//go:build windows

package localav

/*
// FFmpeg dev headers must be on the include path.
// In CI, MSYS2/MinGW provides these via mingw-w64-x86_64-ffmpeg.
#cgo LDFLAGS: -lavformat -lavcodec -lavfilter -lavutil -lswresample -lswscale
// miniaudio on Windows uses WinAPI only — no extra flags needed.
*/
import "C"
