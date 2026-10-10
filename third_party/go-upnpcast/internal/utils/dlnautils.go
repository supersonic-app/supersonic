package utils

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/filetype"
)

const XMLStart = `<?xml version="1.0" encoding="utf-8"?>`

const (
	// dlnaOrgFlagSenderPaced = 1 << 31
	// dlnaOrgFlagTimeBasedSeek = 1 << 30
	// dlnaOrgFlagByteBasedSeek = 1 << 29
	// dlnaOrgFlagPlayContainer = 1 << 28
	// dlnaOrgFlagS0Increase = 1 << 27
	// dlnaOrgFlagSnIncrease = 1 << 26
	// dlnaOrgFlagRtspPause = 1 << 25
	dlnaOrgFlagStreamingTransferMode = 1 << 24
	// dlnaOrgFlagInteractiveTransfertMode = 1 << 23
	dlnaOrgFlagBackgroundTransfertMode = 1 << 22
	dlnaOrgFlagConnectionStall         = 1 << 21
	dlnaOrgFlagDlnaV15                 = 1 << 20
)

var (
	dlnaprofiles = map[string]string{
		"video/x-mkv":             "DLNA.ORG_PN=MATROSKA",
		"video/x-matroska":        "DLNA.ORG_PN=MATROSKA",
		"video/x-msvideo":         "DLNA.ORG_PN=AVI",
		"video/mpeg":              "DLNA.ORG_PN=MPEG1",
		"video/vnd.dlna.mpeg-tts": "DLNA.ORG_PN=MPEG1",
		"video/mp4":               "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5",
		"video/quicktime":         "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5",
		"video/x-m4v":             "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5",
		"video/3gpp":              "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5",
		"video/x-flv":             "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5",
		"video/x-ms-wmv":          "DLNA.ORG_PN=WMVHIGH_FULL",
		"audio/mpeg":              "DLNA.ORG_PN=MP3",
		"image/jpeg":              "JPEG_LRG",
		"image/png":               "PNG_LRG",
	}

	ErrInvalidSeekFlag    = errors.New("invalid seek flag")
	ErrInvalidClockFormat = errors.New("invalid clock format")
)

func defaultStreamingFlags() string {
	return fmt.Sprintf("%.8x%.24x", dlnaOrgFlagStreamingTransferMode|
		dlnaOrgFlagBackgroundTransfertMode|
		dlnaOrgFlagConnectionStall|
		dlnaOrgFlagDlnaV15, 0)
}

func BuildRequestHeader(soapAction string) http.Header {
	return http.Header{
		"SOAPAction":   []string{soapAction},
		"content-type": []string{"text/xml"},
		"charset":      []string{"utf-8"},
		"Connection":   []string{"close"},
	}
}

// BuildContentFeatures builds the content features string
// for the "contentFeatures.dlna.org" header.
func BuildContentFeatures(mediaType string, seek string, transcode bool) (string, error) {
	var cf strings.Builder

	if mediaType != "" {
		dlnaProf, profExists := dlnaprofiles[mediaType]
		if profExists {
			cf.WriteString(dlnaProf + ";")
		}
	}

	// "00" neither time seek range nor range supported
	// "01" range supported
	// "10" time seek range supported
	// "11" both time seek range and range supported
	switch seek {
	case "00":
		cf.WriteString("DLNA.ORG_OP=00;")
	case "01":
		cf.WriteString("DLNA.ORG_OP=01;")
	case "10":
		cf.WriteString("DLNA.ORG_OP=10;")
	case "11":
		cf.WriteString("DLNA.ORG_OP=11;")
	default:
		return "", ErrInvalidSeekFlag
	}

	switch transcode {
	case true:
		cf.WriteString("DLNA.ORG_CI=1;")
	default:
		cf.WriteString("DLNA.ORG_CI=0;")
	}

	cf.WriteString("DLNA.ORG_FLAGS=")
	cf.WriteString(defaultStreamingFlags())

	return cf.String(), nil
}

// GetMimeDetails returns the media mime details.
func GetMimeDetails(f io.ReadCloser) (string, error) {
	defer f.Close()
	head := make([]byte, 261)
	_, err := f.Read(head)
	if err != nil {
		return "", fmt.Errorf("getMimeDetailsFromFile error #2: %w", err)
	}

	kind, err := filetype.Match(head)
	if err != nil {
		return "", fmt.Errorf("getMimeDetailsFromFile error #3: %w", err)
	}

	return fmt.Sprintf("%s/%s", kind.MIME.Type, kind.MIME.Subtype), nil
}

// ClockTimeToSeconds converts relative time to seconds.
func ClockTimeToSeconds(strtime string) (int, error) {
	s := strings.Split(strtime, ":")
	if len(s) != 3 {
		return 0, ErrInvalidClockFormat
	}

	hours, err := strconv.Atoi(s[0])
	if err != nil {
		return 0, ErrInvalidClockFormat
	}

	minutes, err := strconv.Atoi(s[1])
	if err != nil {
		return 0, ErrInvalidClockFormat
	}

	f, err := strconv.ParseFloat(s[2], 32)
	if err != nil {
		return 0, ErrInvalidClockFormat
	}
	seconds := int(math.Round(f))

	return hours*3600 + minutes*60 + seconds, nil
}

// SecondsToClockTime converts seconds to seconds relative time.
func SecondsToClockTime(secs int) string {
	hours := secs / 3600
	secs %= 3600
	minutes := secs / 60
	secs %= 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

// FormatClockTime converts clock time to a more expected format of clock time.
func FormatClockTime(strtime string) (string, error) {
	sec, err := ClockTimeToSeconds(strtime)
	if err != nil {
		return "", ErrInvalidClockFormat
	}

	return SecondsToClockTime(sec), nil
}

// ParseDuration parses a HH:MM:SS or MM:SS formatted string
// into a [time.Duration] value.
func ParseDuration(durStr string) (time.Duration, error) {
	timeformat := ""
	switch strings.Count(durStr, ":") {
	case 2:
		timeformat = "04:05"
	case 3:
		timeformat = "15:04:05"
	default:
		return 0, fmt.Errorf("invalid format: expected MM:SS or HH:MM:SS")
	}

	t, err := time.Parse(timeformat, durStr)
	if err != nil {
		return 0, ErrInvalidClockFormat
	}

	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return t.Sub(midnight), nil
}

func MarshalXMLWithStart(d any) (io.Reader, error) {
	b, err := xml.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("marshaling error: %w", err)
	}

	return io.MultiReader(strings.NewReader(XMLStart), bytes.NewReader(b)), nil
}
