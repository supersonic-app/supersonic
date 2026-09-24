package mpv

/*
#cgo pkg-config: mpv
#include <mpv/client.h>

static int supersonic_end_file_reason(void *data) {
	if (data == NULL) {
		return -1;
	}
	return ((mpv_event_end_file *)data)->reason;
}

static int supersonic_end_file_error(void *data) {
	if (data == NULL) {
		return 0;
	}
	return ((mpv_event_end_file *)data)->error;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/supersonic-app/supersonic/backend/player"
)

const prematureEOFToleranceSeconds = 2

func playbackErrorFromEndFileEvent(data unsafe.Pointer, status player.Status) error {
	reason := C.supersonic_end_file_reason(data)
	if reason == C.MPV_END_FILE_REASON_ERROR {
		message := C.GoString(C.mpv_error_string(C.supersonic_end_file_error(data)))
		if message == "" {
			message = "unknown error"
		}
		return errors.New(message)
	}
	if reason == C.MPV_END_FILE_REASON_EOF && isPrematureEOF(status) {
		return fmt.Errorf("playback ended at %.2fs before the expected duration %.2fs", status.TimePos, status.Duration)
	}
	return nil
}

func isPrematureEOF(status player.Status) bool {
	return status.Duration > 0 && status.TimePos >= 0 &&
		status.Duration-status.TimePos > prematureEOFToleranceSeconds
}
