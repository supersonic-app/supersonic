package player

// Equalizer types based on the FFmpeg 'equalizer' filter.
// Defined here (rather than in the mpv sub-package) so that both the mpv
// and localav player implementations can share the same types without
// creating an import cycle.

import (
	"fmt"
	"math"
	"strings"
)

type Equalizer interface {
	IsEnabled() bool
	Preamp() float64
	Curve() EqualizerCurve
	Type() string
	// Returns the band frequencies as strings friendly for display.
	BandFrequencies() []string
}

type ISO15BandEqualizer struct {
	Disabled  bool
	EQPreamp  float64
	BandGains [15]float64
}

var (
	iso15Bands = []string{"25", "40", "63", "100", "160", "250", "400", "630", "1k", "1.6k", "2.5k", "4k", "6.3k", "10k", "16k"}
	iso15FMult = math.Pow(2, 2./3)
)

var _ Equalizer = (*ISO15BandEqualizer)(nil)

func (i *ISO15BandEqualizer) IsEnabled() bool { return !i.Disabled }
func (i *ISO15BandEqualizer) Preamp() float64  { return i.EQPreamp }

func (i *ISO15BandEqualizer) Curve() EqualizerCurve {
	fC := float64(25)
	curve := make([]EqualizerBand, 0, len(i.BandGains))
	for _, bandGain := range i.BandGains {
		curve = append(curve, EqualizerBand{
			Frequency: int(math.Round(fC)),
			Width:     2. / 3,
			WidthType: WidthTypeOctave,
			Gain:      bandGain,
		})
		fC *= iso15FMult
	}
	return curve
}

func (*ISO15BandEqualizer) BandFrequencies() []string {
	ret := make([]string, len(iso15Bands))
	copy(ret, iso15Bands)
	return ret
}

func (*ISO15BandEqualizer) Type() string { return "ISO15Band" }

type ISO10BandEqualizer struct {
	Disabled  bool
	EQPreamp  float64
	BandGains [10]float64
}

var (
	iso10Bands = []string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}
	iso10FMult = 2.0
)

var _ Equalizer = (*ISO10BandEqualizer)(nil)

func (i *ISO10BandEqualizer) IsEnabled() bool { return !i.Disabled }
func (i *ISO10BandEqualizer) Preamp() float64  { return i.EQPreamp }

func (i *ISO10BandEqualizer) Curve() EqualizerCurve {
	fC := float64(31.25)
	curve := make([]EqualizerBand, 0, len(i.BandGains))
	for _, bandGain := range i.BandGains {
		curve = append(curve, EqualizerBand{
			Frequency: int(math.Round(fC)),
			Width:     1.0,
			WidthType: WidthTypeOctave,
			Gain:      bandGain,
		})
		fC *= iso10FMult
	}
	return curve
}

func (*ISO10BandEqualizer) BandFrequencies() []string {
	ret := make([]string, len(iso10Bands))
	copy(ret, iso10Bands)
	return ret
}

func (*ISO10BandEqualizer) Type() string { return "ISO10Band" }

type WidthType int

const (
	WidthTypeHz WidthType = iota
	WidthTypeKhz
	WidthTypeQ
	WidthTypeOctave
	WidthTypeSlope
)

func (w WidthType) String() string {
	switch w {
	case WidthTypeHz:
		return "h"
	case WidthTypeKhz:
		return "k"
	case WidthTypeQ:
		return "q"
	case WidthTypeOctave:
		return "o"
	case WidthTypeSlope:
		return "s"
	}
	return "x"
}

type EqualizerBand struct {
	Frequency int
	Gain      float64
	Width     float64
	WidthType WidthType
}

func (e EqualizerBand) String() string {
	if math.Abs(e.Gain) < 0.02 {
		return ""
	}
	return fmt.Sprintf("equalizer=f=%d:g=%0.2f:t=%s:w=%0.2f",
		e.Frequency, e.Gain, e.WidthType.String(), e.Width)
}

// QFactor returns the Q factor for this band, converting from whatever WidthType is used.
func (e EqualizerBand) QFactor() float64 {
	switch e.WidthType {
	case WidthTypeQ:
		return e.Width
	case WidthTypeOctave:
		return 1.0 / (2.0 * math.Sinh(math.Log(2)/2.0*e.Width))
	case WidthTypeHz:
		if e.Width > 0 {
			return float64(e.Frequency) / e.Width
		}
		return 1.0
	default:
		return 1.0
	}
}

type EqualizerCurve []EqualizerBand

func (e EqualizerCurve) String() string {
	var sb strings.Builder
	first := true
	for _, band := range e {
		if s := band.String(); s != "" {
			if !first {
				sb.WriteString(",")
			}
			sb.WriteString(s)
			first = false
		}
	}
	return sb.String()
}
