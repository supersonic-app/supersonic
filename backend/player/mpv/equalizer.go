package mpv

// Equalizer types are now defined in the parent player package.
// Type aliases are provided here for backwards compatibility with
// existing call sites that use mpv.Equalizer, mpv.ISO15BandEqualizer, etc.

import "github.com/dweymouth/supersonic/backend/player"

type (
	Equalizer        = player.Equalizer
	EqualizerBand    = player.EqualizerBand
	EqualizerCurve   = player.EqualizerCurve
	WidthType        = player.WidthType
	ISO15BandEqualizer = player.ISO15BandEqualizer
	ISO10BandEqualizer = player.ISO10BandEqualizer
)

const (
	WidthTypeHz     = player.WidthTypeHz
	WidthTypeKhz    = player.WidthTypeKhz
	WidthTypeQ      = player.WidthTypeQ
	WidthTypeOctave = player.WidthTypeOctave
	WidthTypeSlope  = player.WidthTypeSlope
)
