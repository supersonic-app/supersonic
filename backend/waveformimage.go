package backend

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/supersonic-app/go-mpv"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/util"
)

type WaveformImageGenerator struct {
	audioCache *AudioCache
}

// Buffer pool for waveform analysis to reduce allocations
var audioBufferPool = sync.Pool{
	New: func() any {
		return &audio.IntBuffer{Data: make([]int, 4096)}
	},
}

type WaveformImage = image.NRGBA

func NewWaveformImage() *WaveformImage {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 48))
	centerTop := img.Rect.Dy() / 2 // 24
	centerBottom := centerTop - 1  // 23

	// color in center line
	for x := 0; x < img.Bounds().Dx(); x++ {
		setPixel(img, x, centerTop, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		setPixel(img, x, centerBottom, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	return img
}

type WaveformImageJob struct {
	ItemID   string
	lock     sync.Mutex
	img      *WaveformImage
	err      error
	progress int // first invalid pixel in X direction
	done     bool
	cancel   func()
	canceled bool
}

func (w *WaveformImageJob) Cancel() {
	if w != nil {
		w.canceled = true
		if w.cancel != nil {
			w.cancel()
		}
	}
}

func (w *WaveformImageJob) Canceled() bool {
	return w.canceled
}

func (w *WaveformImageJob) Done() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.err != nil || w.done
}

func (w *WaveformImageJob) Err() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.err
}

func (w *WaveformImageJob) Get() *WaveformImage {
	if w.Done() {
		return w.img
	}
	// return a new *WaveformImage with data copied
	// from the valid region of w.img
	height := w.img.Bounds().Dy()
	result := NewWaveformImage()

	// Copy each scanline from w.img to result
	for y := range height {
		srcOffset := w.img.PixOffset(0, y)
		dstOffset := result.PixOffset(0, y)
		copy(result.Pix[dstOffset:dstOffset+w.progress*4], w.img.Pix[srcOffset:srcOffset+w.progress*4])
	}

	return result
}

func NewWaveformImageGenerator(cache *AudioCache) *WaveformImageGenerator {
	return &WaveformImageGenerator{audioCache: cache}
}

func (w *WaveformImageGenerator) StartWaveformGeneration(item *mediaprovider.Track) *WaveformImageJob {
	ctx, cancel := context.WithCancel(w.audioCache.rootCtx)
	job := &WaveformImageJob{
		img:    NewWaveformImage(),
		ItemID: item.ID,
		cancel: cancel,
	}

	// Set up a pipeline of concurrent tasks that need to complete to generate
	// a waveform image:
	// 1. Begin downloading the file from the server
	// 2. Begin transcoding it to WAV
	// 3. Begin analyzing the resulting WAV file
	// 4. Begin generating the image from the analysis data
	go func() {
		path := w.audioCache.ObtainReferenceToFile(job.ItemID)
		// wait for file to begin downloading if not already
		for path == "" {
			time.Sleep(50 * time.Millisecond)
			if e := ctx.Err(); e != nil {
				job.setError(e)
				return
			}
			path = w.audioCache.ObtainReferenceToFile(job.ItemID)
		}
		// and wait for content to begin being written
		for {
			if s, err := os.Stat(path); err == nil && s.Size() > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
			if e := ctx.Err(); e != nil {
				job.setError(e)
				return
			}
		}

		dir := filepath.Dir(path)
		var transcodeFile string
		for i := 0; true; i++ {
			if i > 0 {
				transcodeFile = filepath.Join(dir, fmt.Sprintf("%s_waveform_%d.wav", filepath.Base(path), i))
			} else {
				transcodeFile = filepath.Join(dir, filepath.Base(path)+"_waveform.wav")
			}
			if _, err := os.Stat(transcodeFile); os.IsNotExist(err) {
				break // found a suitable filename that doesn't exist
			}
		}

		fileDone := func() bool {
			return w.audioCache.IsFullyDownloaded(job.ItemID)
		}

		// If file isn't fully downloaded from server,
		// stream it to MPV via fifo so it doesn't possibly
		// terminate the conversion to WAV early encountering EOF
		if !fileDone() {
			srv, err := util.NewFileStreamerServer(path, fileDone)
			if err != nil {
				job.setError(err)
				return
			}

			path = srv.Addr()

			go srv.Serve()
			time.Sleep(10 * time.Millisecond) // make sure server has time to come up
		}

		// Start converting the file to WAV for analysis
		var wavConvertDone bool
		go func() {
			err := w.convertToWav(ctx, job.ItemID, path, transcodeFile)
			wavConvertDone = true
			if err != nil {
				job.setError(err)
			}
		}()

		// Wait for transcoded WAV file to begin being written
		for {
			if s, err := os.Stat(transcodeFile); err == nil && s.Size() > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
			if e := ctx.Err(); e != nil {
				job.setError(e)
				return
			}
		}

		// Start analyzing the converted wav file
		data := &waveformData{notify: make(chan struct{}, 1)}
		go func() {
			err := analyzeWavFile(ctx, transcodeFile, data, item.Duration.Milliseconds(), func() bool { return wavConvertDone })
			if err != nil {
				job.setError(err)
			}
			data.done = true
			// Final notification that processing is complete
			select {
			case data.notify <- struct{}{}:
			default:
			}
			close(data.notify)
		}()

		// Start generating the waveform image
		go func() {
			generateWaveformImage(ctx, data, job)
			job.done = true
		}()
	}()
	return job
}

type waveformData struct {
	Peak [1024]byte
	RMS  [1024]byte

	progress int // first invalid index for Peak/RMS data
	done     bool
	notify   chan struct{} // signals when new data is available
}

func generateWaveformImage(ctx context.Context, data *waveformData, job *WaveformImageJob) {
	centerY := job.img.Rect.Dy() / 2 // 24
	top := centerY - 1               // 23
	bottom := centerY                // 24

	opaqueColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	translucentColor := color.NRGBA{R: 255, G: 255, B: 255, A: 128}

	for x := range 1024 {
		// Wait for data to be available instead of polling
		for data.progress <= x && !data.done {
			select {
			case <-ctx.Done():
				return // expired
			case <-data.notify:
				// New data available or processing complete
			}
		}

		if data.progress <= x {
			return // done but data not available for this x
		}

		rmsPixels := int(data.RMS[x]) * centerY / 255
		peakPixels := int(data.Peak[x]) * centerY / 255

		// Always draw at least 2 center pixels
		setPixel(job.img, x, top, opaqueColor)
		setPixel(job.img, x, bottom, opaqueColor)

		// Draw RMS pixels (solid)
		for i := 1; i < rmsPixels; i++ {
			setPixel(job.img, x, top-i, opaqueColor)
			setPixel(job.img, x, bottom+i, opaqueColor)
		}

		// Draw Peak extension (translucent)
		for i := max(1, rmsPixels); i < peakPixels; i++ {
			setPixel(job.img, x, top-i, translucentColor)
			setPixel(job.img, x, bottom+i, translucentColor)
		}
		job.progress = x + 1
	}
}

// assumes mono, 16 bit
func analyzeWavFile(ctx context.Context, transcodeFile string, data *waveformData, millisecs int64, fileDone func() bool) error {
	f, err := os.Open(transcodeFile)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(transcodeFile)

	decoder := wav.NewDecoder(f)
	if !decoder.IsValidFile() {
		return errors.New("invalid wav file")
	}

	format := decoder.Format()

	totalSamples := format.SampleRate * int(millisecs) / 1000
	samplesPerChunk := totalSamples / 1024

	if err := decoder.FwdToPCM(); err != nil {
		return err
	}

	// Get buffer from pool to reduce allocations
	buf := audioBufferPool.Get().(*audio.IntBuffer)
	defer audioBufferPool.Put(buf)

	curChunk := 0
	chunkSamples := make([]float64, 0, samplesPerChunk)
	bytesPerSample := int64(2 * format.NumChannels) // 16-bit = 2 bytes per channel

	// file read loop
	doneReading := false
	for !doneReading {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		fileIsDone := fileDone()

		if !fileIsDone {
			// Check how many samples we can safely read without encountering EOF
			// and adjust read buffer size accordingly

			stat, err := f.Stat()
			if err != nil {
				return fmt.Errorf("stat failed: %w", err)
			}
			currentSize := stat.Size()

			// how many bytes can we read without nearing EOF
			readableBytes := currentSize - int64(samplesPerChunk)*int64(curChunk)*bytesPerSample - 8192 // buffer for safety

			// Estimate how many samples we can read
			maxSamples := int(readableBytes / bytesPerSample)

			if maxSamples <= 0 {
				// Wait for more data to be written to file
				time.Sleep(50 * time.Millisecond)
				continue
			}

			// Resize buffer to fit only what’s safe
			safeSamples := min(maxSamples, cap(buf.Data))
			buf.Data = buf.Data[:safeSamples]
		} else {
			// File is done being written, resize read buf to the max
			buf.Data = buf.Data[:cap(buf.Data)]
		}

		n, err := decoder.PCMBuffer(buf)
		if n == 0 || err == io.EOF {
			if fileIsDone {
				doneReading = true
			}
			if err == io.EOF && !fileDone() {
				return errors.New("WAV read got premature EOF")
			}
		}
		if err != nil {
			return err
		}

		// Process samples
		for i := range n {
			sample := float64(buf.Data[i]) / float64(1<<15) // Normalize to [-1, 1]
			chunkSamples = append(chunkSamples, sample)

			if len(chunkSamples) >= samplesPerChunk {
				if curChunk < 1024 {
					peak, rms := computePeakAndRMS(chunkSamples)
					data.Peak[curChunk] = float64ToByte(peak)
					data.RMS[curChunk] = float64ToByte(rms)
				}
				curChunk++
				data.progress = curChunk
				// Notify that new data is available (non-blocking)
				select {
				case data.notify <- struct{}{}:
				default:
				}
				chunkSamples = chunkSamples[:0]
				if curChunk >= 1024 {
					break
				}
			}
		}
	}

	// analyze the last chunk if it's partially filled with samples
	if curChunk < 1024 && len(chunkSamples) > 0 {
		peak, rms := computePeakAndRMS(chunkSamples)
		data.Peak[curChunk] = float64ToByte(peak)
		data.RMS[curChunk] = float64ToByte(rms)
		data.progress = curChunk + 1
		// Notify that final data is available (non-blocking)
		select {
		case data.notify <- struct{}{}:
		default:
		}
	}

	return nil
}

func (j *WaveformImageJob) setError(err error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.err = err
}

func computePeakAndRMS(chunk []float64) (peak float64, rms float64) {
	var sumSquares float64
	peak = 0.0
	for _, v := range chunk {
		if v > peak {
			peak = v
		} else if v < -peak {
			peak = -v
		}
		sumSquares += float64(v * v)
	}
	rms = math.Sqrt(sumSquares / float64(len(chunk)))
	return peak, rms
}

func float64ToByte(val float64) byte {
	if val > 1.0 {
		val = 1.0
	}
	if val < 0.0 {
		val = 0.0
	}
	return byte(val * 255)
}

func (w *WaveformImageGenerator) convertToWav(ctx context.Context, id, inPath, outPath string) error {
	m := mpv.Create()
	m.SetOptionString("video", "no")
	m.SetOptionString("audio-display", "no")
	m.SetOptionString("terminal", "no")
	m.SetOptionString("idle", "yes")
	m.SetOptionString("ao-pcm-file", outPath)
	m.SetOptionString("ao", "pcm")
	m.SetOption("volume", mpv.FORMAT_INT64, 100)
	// no need to preserve full sample resolution just for waveform image
	// let's make less data to process and smaller on-disk file
	m.SetOption("audio-samplerate", mpv.FORMAT_INT64, 22050)
	m.SetOptionString("audio-channels", "mono")
	m.SetOptionString("audio-format", "s16")
	if err := m.Initialize(); err != nil {
		return err
	}

	defer m.TerminateDestroy()

	m.Command([]string{"loadfile", inPath, "replace"})
	defer w.audioCache.ReleaseReferenceToFile(id)

	// Wait for MPV idle or ctx expiry
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// use small timeout to allow detecting ctx expiry
			// without too much delay
			e := m.WaitEvent(0.05 /*timeout seconds*/)
			if e.Event_Id == mpv.EVENT_IDLE {
				if _, err := os.Stat(outPath); os.IsNotExist(err) {
					log.Printf("WARNING! file %s does not exist after MPV convert", outPath)
				}
				return nil
			}
			ia := m.GetPropertyString("idle-active")
			if ia == "yes" || ia == "true" {
				if _, err := os.Stat(outPath); os.IsNotExist(err) {
					log.Printf("WARNING! file %s does not exist after MPV convert", outPath)
				}
				return nil
			}
		}
	}
}

func setPixel(img *image.NRGBA, x, y int, c color.NRGBA) {
	offset := img.PixOffset(x, y)
	img.Pix[offset+0] = c.R
	img.Pix[offset+1] = c.G
	img.Pix[offset+2] = c.B
	img.Pix[offset+3] = c.A
}
