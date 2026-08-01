//go:build localav

package backend

import (
	"context"
	"os"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player/localav"
	"github.com/dweymouth/supersonic/backend/util"
)

func (w *WaveformImageGenerator) StartWaveformGeneration(item *mediaprovider.Track) *WaveformImageJob {
	ctx, cancel := context.WithCancel(w.audioCache.rootCtx)
	job := &WaveformImageJob{
		img:    NewWaveformImage(),
		ItemID: item.ID,
		cancel: cancel,
	}

	go func() {
		// 1. Obtain reference to cached/downloading file
		path := w.audioCache.ObtainReferenceToFile(job.ItemID)
		for path == "" {
			time.Sleep(50 * time.Millisecond)
			if e := ctx.Err(); e != nil {
				job.setError(e)
				return
			}
			path = w.audioCache.ObtainReferenceToFile(job.ItemID)
		}
		defer w.audioCache.ReleaseReferenceToFile(job.ItemID)

		// Wait for content to begin being written
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

		// 2. If file isn't fully downloaded, stream it via local HTTP server
		//    so FFmpeg doesn't hit a premature EOF
		fileDone := func() bool {
			return w.audioCache.IsFullyDownloaded(job.ItemID)
		}
		inputURL := path
		if !fileDone() {
			srv, err := util.NewFileStreamerServer(path, fileDone)
			if err != nil {
				job.setError(err)
				return
			}
			inputURL = srv.Addr()
			go srv.Serve()
			time.Sleep(10 * time.Millisecond)
		}

		// 3. Start C-side waveform analysis
		wa := localav.NewWaveformAnalysis()
		data := &waveformData{notify: make(chan struct{}, 1)}

		// Analysis runs synchronously in a goroutine;
		// the bridge goroutine below polls progress and copies data.
		analyzeDone := make(chan struct{})
		go func() {
			err := localav.AnalyzeWaveform(inputURL, item.Duration.Milliseconds(), wa)
			if err != nil {
				job.setError(err)
			}
			close(analyzeDone)
		}()

		// Cancel the C-side analysis if the context is cancelled.
		go func() {
			select {
			case <-ctx.Done():
				wa.Cancel()
			case <-analyzeDone:
			}
		}()

		// 4. Bridge C progress to waveformData for image generation.
		// This goroutine owns data.done and data.notify lifecycle.
		go func() {
			lastProgress := 0
			for {
				select {
				case <-analyzeDone:
					// Final copy of any remaining data
					p := wa.Progress()
					if p > lastProgress {
						for i := lastProgress; i < p && i < 1024; i++ {
							data.Peak[i] = wa.Peak[i]
							data.RMS[i] = wa.RMS[i]
						}
						data.progress = p
					}
					data.done = true
					select {
					case data.notify <- struct{}{}:
					default:
					}
					close(data.notify)
					return
				case <-time.After(15 * time.Millisecond):
					p := wa.Progress()
					if p > lastProgress {
						for i := lastProgress; i < p && i < 1024; i++ {
							data.Peak[i] = wa.Peak[i]
							data.RMS[i] = wa.RMS[i]
						}
						data.progress = p
						lastProgress = p
						select {
						case data.notify <- struct{}{}:
						default:
						}
					}
				}
			}
		}()

		// 5. Generate waveform image (shared code)
		generateWaveformImage(ctx, data, job)
		job.done = true
	}()

	return job
}
