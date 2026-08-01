//go:build linux

package localav

/*
#cgo pkg-config: libavformat libavcodec libavfilter libavutil libswresample
#cgo LDFLAGS: -lpthread -ldl -lm
*/
import "C"
