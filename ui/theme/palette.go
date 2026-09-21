package theme

import (
	"encoding/hex"
	"fmt"
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
)

// SliderRanges defines the min/max ranges for saturation and contrast sliders per base mode
type SliderRanges struct {
	SatMin, SatMax           float64
	ContrastMin, ContrastMax float64
}

// GetSliderRanges returns the clamp ranges for saturation and contrast based on appearance
// This is the single source of truth used by both UI sliders and palette generation
func GetSliderRanges(appearance string) SliderRanges {
	switch appearance {
	case string(AppearanceLight):
		return SliderRanges{SatMin: 0.5, SatMax: 1.5, ContrastMin: 0.5, ContrastMax: 1.0}
	case string(AppearanceDark):
		return SliderRanges{SatMin: 0.5, SatMax: 1.5, ContrastMin: 0, ContrastMax: 1.0}
	default: // Auto - use dark ranges as default
		return SliderRanges{SatMin: 0.5, SatMax: 1.5, ContrastMin: 0, ContrastMax: 1.0}
	}
}

// Palette represents a generated color palette from an accent color
type Palette struct {
	Accent         color.Color
	Background     color.Color
	Surface        color.Color
	SurfaceHover   color.Color
	MenuBackground color.Color // Subtle accent tint for menus
	TextPrimary    color.Color
	TextSecondary  color.Color
	TextOnAccent   color.Color
	Success        color.Color
	Danger         color.Color
	PageBackground color.Color
	PageHeader     color.Color
	ListHeader     color.Color
	Hyperlink      color.Color
}

// GeneratePalette creates a unified color palette from the given configuration
// Uses consistent formulas across all appearances for coherence.
// Note: Caching is handled by MyTheme.cachedPalette, not here, to avoid double caching.
func GeneratePalette(accentHex string, saturation, contrast float64, appearance string) (*Palette, error) {
	// Normalize accent color for the mode (ensures visibility)
	normalizer := NewColorNormalizer()
	normalizedHex := normalizer.NormalizeForMode(accentHex, appearance)
	accent, _ := hexToColor(normalizedHex)
	accentH, accentS, accentL := RgbToHslColor(accent)

	// Apply saturation/contrast sliders to accent
	if saturation != 1.0 {
		// Allow oversaturation (>1.0) for more vibrant colors
		accentS = math.Max(0, accentS*saturation)
	}
	if contrast != 1.0 {
		// Contrast affects lightness: >1 = more extreme, <1 = closer to mid
		midpoint := 0.5
		accentL = midpoint + (accentL-midpoint)*contrast
		accentL = clampFloat(accentL, 0.15, 0.85)
	}
	// Determine mode characteristics
	isLight := appearance == string(AppearanceLight)
	isDark := appearance == string(AppearanceDark) || (appearance == string(AppearanceAuto) && IsDarkMode(fyne.CurrentApp()))

	// Apply luminosity correction for legibility only when contrast is low/medium
	// When contrast > 1.0, user wants extreme colors, so respect their choice
	if contrast <= 1.0 {
		if isLight {
			accentL = math.Min(accentL, 0.45)
		} else {
			accentL = math.Max(accentL, 0.55)
		}
	}
	accent = HslToRgb(accentH, math.Min(accentS, 1.0), accentL)

	// Unified palette generation using consistent ratios
	// All values derived from accent hue with mode-specific lightness
	var bgL, surfL, pageBgL, pageHeaderL, listHeaderL, textPrimaryL, textSecondaryL float64
	var bgSat float64

	if isLight {
		// Light mode: light backgrounds, darker text for contrast
		bgL = 0.94
		surfL = 1.0 // Pure white for surfaces
		pageBgL = 0.96
		pageHeaderL = 0.90
		listHeaderL = 0.92
		textPrimaryL = 0.08   // Darker for better contrast (was 0.12)
		textSecondaryL = 0.28 // Darker gray (was 0.40)
		bgSat = 0.03
	} else if isDark {
		// Dark mode (includes both Dark and Auto when system is dark)
		bgL = 0.02
		surfL = 0.10
		pageBgL = 0.04
		pageHeaderL = 0.14
		listHeaderL = 0.12
		textPrimaryL = 0.95
		textSecondaryL = 0.65
		bgSat = 0.04
	} else {
		// Fallback - shouldn't happen with proper Appearance values
		// Use dark greys as safe default
		bgL = 0.10
		surfL = 0.16
		pageBgL = 0.08
		pageHeaderL = 0.20
		listHeaderL = 0.18
		textPrimaryL = 0.94
		textSecondaryL = 0.62
		bgSat = 0.06
	}

	// Apply contrast adjustment to background lightness
	// Higher contrast = more separation from middle grey
	contrastDelta := (contrast - 1.0) * 0.08
	if isLight {
		// Light mode: higher contrast = lighter backgrounds
		bgL = clampFloat(bgL+contrastDelta, 0.85, 1.0)
		surfL = clampFloat(surfL+contrastDelta*0.5, 0.90, 1.0)
	} else {
		// Dark mode: higher contrast = darker backgrounds
		bgL = clampFloat(bgL-contrastDelta, 0.0, 0.25)
		surfL = clampFloat(surfL-contrastDelta*0.5, 0.05, 0.30)
	}

	// Generate colors from accent hue with calculated lightness
	bg := HslToRgb(accentH, bgSat, bgL)
	surf := HslToRgb(accentH, bgSat, surfL)
	pageBg := HslToRgb(accentH, bgSat, pageBgL)
	pageHeader := HslToRgb(accentH, bgSat, pageHeaderL)
	listHeader := HslToRgb(accentH, bgSat*0.8, listHeaderL)

	// Text colors: accent hue with low saturation for subtle tint
	textSat := 0.08
	if isLight {
		textSat = 0.12 // Slightly more saturation in light mode for visibility
	}
	textPrimaryRGB := HslToRgb(accentH, textSat, textPrimaryL)
	textSecondaryRGB := HslToRgb(accentH, textSat*0.7, textSecondaryL)

	// Ensure WCAG contrast compliance
	textPrimary, _ := EnsureContrast(textPrimaryRGB, surf, 4.5)
	textSecondary, _ := EnsureContrast(textSecondaryRGB, surf, 3.0) // AA for large text

	// Calculate text on accent
	textOnAccent := calculateTextOnAccent(accent)

	// Surface hover: blend accent with surface
	hoverRatio := 0.22
	if !isLight {
		hoverRatio = 0.28 // Stronger hover in dark modes
	}
	surfaceHover := BlendColors(accent, surf, hoverRatio)

	// Menu background: subtler blend
	menuBackground := BlendColors(accent, surf, hoverRatio*0.5)

	// Success/Danger: keep standard but tint slightly with accent hue
	success := HslToRgb(140.0, 0.60, 0.45) // Green
	danger := HslToRgb(0.0, 0.70, 0.50)    // Red

	// Hyperlink: use the processed accent directly (luminosity correction already applied)
	hyperlink := accent

	palette := &Palette{
		Accent:         accent,
		Background:     bg,
		Surface:        surf,
		SurfaceHover:   surfaceHover,
		MenuBackground: menuBackground,
		TextPrimary:    textPrimary,
		TextSecondary:  textSecondary,
		TextOnAccent:   textOnAccent,
		Success:        success,
		Danger:         danger,
		PageBackground: pageBg,
		PageHeader:     pageHeader,
		ListHeader:     listHeader,
		Hyperlink:      hyperlink,
	}

	return palette, nil
}

// hexToColor parses a hex color string (#RRGGBB or #RRGGBBAA)
func hexToColor(hexStr string) (color.RGBA, error) {
	hexStr = strings.TrimSpace(hexStr)
	if !strings.HasPrefix(hexStr, "#") {
		return color.RGBA{}, fmt.Errorf("invalid hex color format")
	}
	hexStr = hexStr[1:]

	var r, g, b, a uint8
	var err error

	switch len(hexStr) {
	case 6:
		r, err = parseHexByte(hexStr[0:2])
		if err != nil {
			return color.RGBA{}, err
		}
		g, err = parseHexByte(hexStr[2:4])
		if err != nil {
			return color.RGBA{}, err
		}
		b, err = parseHexByte(hexStr[4:6])
		if err != nil {
			return color.RGBA{}, err
		}
		a = 255
	case 8:
		r, err = parseHexByte(hexStr[0:2])
		if err != nil {
			return color.RGBA{}, err
		}
		g, err = parseHexByte(hexStr[2:4])
		if err != nil {
			return color.RGBA{}, err
		}
		b, err = parseHexByte(hexStr[4:6])
		if err != nil {
			return color.RGBA{}, err
		}
		a, err = parseHexByte(hexStr[6:8])
		if err != nil {
			return color.RGBA{}, err
		}
	default:
		return color.RGBA{}, fmt.Errorf("invalid hex color length")
	}

	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}

func parseHexByte(s string) (uint8, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// calculateTextOnAccent determines the best text color (black or white) based on luminance
func calculateTextOnAccent(c color.RGBA) color.Color {
	// Calculate luminance using standard formula
	luminance := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)

	if luminance > 128 {
		return color.Black
	}
	return color.White
}
