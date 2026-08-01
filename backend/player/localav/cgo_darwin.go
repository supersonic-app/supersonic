//go:build darwin

package localav

/*
#cgo CFLAGS: -I/opt/homebrew/include -I/opt/local/include
#cgo LDFLAGS: -L/opt/homebrew/lib -L/opt/local/lib
#cgo LDFLAGS: -lavformat -lavcodec -lavfilter -lavutil -lswresample -lswscale
#cgo LDFLAGS: -framework CoreAudio -framework AudioToolbox -framework CoreFoundation
*/
import "C"
