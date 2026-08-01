//go:build !localav

package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/util"
	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	mpv "github.com/supersonic-app/go-mpv"
)

func (w *WaveformImageGenerator) StartWaveformGeneration(item *mediaprovider.Track) *WaveformImageJob {
	ctx, cancel := context.WithCancel(w.audioCache.rootCtx)
	job := &WaveformImageJob{
		img:    NewWaveformImage(),
		ItemID: item.ID,
		cancel: cancel,
	}

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
			stat, err := f.Stat()
			if err != nil {
				return fmt.Errorf("stat failed: %w", err)
			}
			currentSize := stat.Size()
			readableBytes := currentSize - int64(samplesPerChunk)*int64(curChunk)*bytesPerSample - 8192
			maxSamples := int(readableBytes / bytesPerSample)

			if maxSamples <= 0 {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			safeSamples := min(maxSamples, cap(buf.Data))
			buf.Data = buf.Data[:safeSamples]
		} else {
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
			sample := float64(buf.Data[i]) / float64(1<<15)
			chunkSamples = append(chunkSamples, sample)

			if len(chunkSamples) >= samplesPerChunk {
				if curChunk < 1024 {
					peak, rms := computePeakAndRMS(chunkSamples)
					data.Peak[curChunk] = float64ToByte(peak)
					data.RMS[curChunk] = float64ToByte(rms)
				}
				curChunk++
				data.progress = curChunk
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
		select {
		case data.notify <- struct{}{}:
		default:
		}
	}

	return nil
}
