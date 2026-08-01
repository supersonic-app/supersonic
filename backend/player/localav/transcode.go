//go:build localav

package localav

/*
#include "av_player.h"
#include <stdlib.h>
#include <stdatomic.h>

static int load_progress(void *p) {
    return atomic_load_explicit((_Atomic int *)p, memory_order_acquire);
}

static void store_cancel(void *p) {
    atomic_store_explicit((_Atomic int *)p, 1, memory_order_release);
}

static int call_analyze_waveform(const char *in_url, int64_t duration_ms,
                                  uint8_t *out_peak, uint8_t *out_rms,
                                  void *progress, void *cancel) {
    return av_analyze_waveform(in_url, duration_ms, out_peak, out_rms,
                               (_Atomic int *)progress, (_Atomic int *)cancel);
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// WaveformAnalysis holds the shared state for an in-progress waveform analysis.
type WaveformAnalysis struct {
	Peak     [1024]byte
	RMS      [1024]byte
	progress int32
	cancel   int32
}

// Progress returns the number of completed waveform bins (0–1024).
func (w *WaveformAnalysis) Progress() int {
	return int(C.load_progress(unsafe.Pointer(&w.progress)))
}

// Cancel signals av_analyze_waveform to stop at the next packet boundary.
func (w *WaveformAnalysis) Cancel() {
	C.store_cancel(unsafe.Pointer(&w.cancel))
}

// NewWaveformAnalysis creates a new analysis state.
func NewWaveformAnalysis() *WaveformAnalysis {
	return &WaveformAnalysis{}
}

// AnalyzeWaveform decodes audio at inURL and computes peak/RMS values for
// 1024 waveform bins. Results are written incrementally to wa.Peak and wa.RMS,
// with wa.Progress() updated atomically for progressive rendering.
// Runs synchronously — call from a goroutine.
func AnalyzeWaveform(inURL string, durationMs int64, wa *WaveformAnalysis) error {
	cin := C.CString(inURL)
	defer C.free(unsafe.Pointer(cin))

	ret := C.call_analyze_waveform(cin, C.int64_t(durationMs),
		(*C.uint8_t)(unsafe.Pointer(&wa.Peak[0])),
		(*C.uint8_t)(unsafe.Pointer(&wa.RMS[0])),
		unsafe.Pointer(&wa.progress),
		unsafe.Pointer(&wa.cancel))

	if ret < 0 {
		return fmt.Errorf("av_analyze_waveform: ffmpeg error %d", int(ret))
	}
	return nil
}
