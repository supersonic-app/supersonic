package widgets

import (
	"regexp"
	"strconv"

	fynelyrics "github.com/supersonic-app/fyne-lyrics"
)

// inlineWordTagRe matches enhanced-LRC inline tags <MM:SS.CC> (2 or 3 digit
// fraction).
var inlineWordTagRe = regexp.MustCompile(`<(\d{2}):(\d{2})\.(\d{2,3})>`)

// parseLyricSegments parses enhanced-LRC inline <MM:SS.CC> tags into timed
// segments. Text without tags becomes a single untimed segment.
func parseLyricSegments(text string) []fynelyrics.LyricSegment {
	matches := inlineWordTagRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return []fynelyrics.LyricSegment{{Text: text, Start: -1}}
	}
	var segs []fynelyrics.LyricSegment
	if matches[0][0] > 0 {
		segs = append(segs, fynelyrics.LyricSegment{Text: text[:matches[0][0]], Start: -1})
	}
	for i, m := range matches {
		mm, _ := strconv.Atoi(text[m[2]:m[3]])
		ss, _ := strconv.Atoi(text[m[4]:m[5]])
		frac := text[m[6]:m[7]]
		f, _ := strconv.Atoi(frac)
		fracSec := float64(f) / 100
		if len(frac) == 3 {
			fracSec = float64(f) / 1000
		}
		start := float64(mm)*60 + float64(ss) + fracSec
		textStart := m[1]
		textEnd := len(text)
		if i+1 < len(matches) {
			textEnd = matches[i+1][0]
		}
		segText := text[textStart:textEnd]
		if segText == "" {
			continue
		}
		segs = append(segs, fynelyrics.LyricSegment{Text: segText, Start: start})
	}
	return segs
}
