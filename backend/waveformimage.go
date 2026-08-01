package backend

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/go-audio/audio"
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

func setPixel(img *image.NRGBA, x, y int, c color.NRGBA) {
	offset := img.PixOffset(x, y)
	img.Pix[offset+0] = c.R
	img.Pix[offset+1] = c.G
	img.Pix[offset+2] = c.B
	img.Pix[offset+3] = c.A
}
