package theme

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// ColorNormalizer provides unified color adjustment for theme consistency
// All extracted or user-selected colors pass through here before palette generation
type ColorNormalizer struct {
	// Minimum luminance for dark modes (ensures visibility on dark bg)
	MinLuminanceDark float64
	// Maximum luminance for light modes (ensures visibility on light bg)
	MaxLuminanceLight float64
	// Target saturation range for accents
	MinSaturation float64
	MaxSaturation float64
}

// NewColorNormalizer creates a normalizer with sensible defaults
func NewColorNormalizer() *ColorNormalizer {
	return &ColorNormalizer{
		MinLuminanceDark:  0.55, // High luminance so accents pop against dark backgrounds
		MaxLuminanceLight: 0.40, // Low luminance so accents pop against light backgrounds
		MinSaturation:     0.25, // Prevent accents from looking too muddy/gray
		MaxSaturation:     1.0,  // Allow full saturation
	}
}

// NormalizeForMode adjusts any hex color to be appropriate for the given appearance mode
// Returns a color guaranteed to have sufficient contrast against backgrounds
func (cn *ColorNormalizer) NormalizeForMode(hexColor string, appearance string) string {
	c, err := hexToColor(hexColor)
	if err != nil {
		// Return a safe default based on mode
		if appearance == string(AppearanceLight) {
			return "#0066CC"
		}
		return "#66B2FF"
	}

	h, s, l := RgbToHslColor(c)
	isLight := appearance == string(AppearanceLight)

	// Normalize luminance based on mode
	if isLight {
		// Light mode: color must be dark enough for visibility
		if l > cn.MaxLuminanceLight {
			l = cn.MaxLuminanceLight - (l-cn.MaxLuminanceLight)*0.5
			if l < 0.2 {
				l = 0.2
			}
		}
		// Ensure minimum saturation for interest
		if s < cn.MinSaturation {
			s = cn.MinSaturation
		}
	} else {
		// Dark/black mode: color must be light enough for visibility
		if l < cn.MinLuminanceDark {
			// Lighten proportionally: very dark gets more boost
			boost := 0.15 + (cn.MinLuminanceDark-l)*0.3
			l = cn.MinLuminanceDark + boost
			if l > 0.7 {
				l = 0.7 // Cap to avoid too-bright accents
			}
		}
		// Boost saturation for vibrancy in dark modes
		if s < cn.MinSaturation {
			s = cn.MinSaturation + 0.1
		}
		if s < 0.5 {
			s = 0.5 + s*0.3 // Boost low saturation colors
		}
	}

	// Clamp to valid ranges
	s = clampFloat(s, 0, 1)
	l = clampFloat(l, 0, 1)

	normalized := HslToRgb(h, s, l)
	return fmt.Sprintf("#%02X%02X%02X", normalized.R, normalized.G, normalized.B)
}

// GetContrastRatio calculates WCAG contrast ratio between two colors
// Returns ratio from 1:1 to 21:1. AA compliance requires >= 4.5
func GetContrastRatio(color1, color2 color.Color) float64 {
	l1 := getRelativeLuminance(color1)
	l2 := getRelativeLuminance(color2)

	// Ensure lighter color is L1
	if l1 < l2 {
		l1, l2 = l2, l1
	}

	return (l1 + 0.05) / (l2 + 0.05)
}

// getRelativeLuminance calculates relative luminance per WCAG 2.1
func getRelativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit to 8-bit, then to sRGB
	r8 := float64(r>>8) / 255.0
	g8 := float64(g>>8) / 255.0
	b8 := float64(b>>8) / 255.0

	// Convert to linear RGB
	rLin := toLinear(r8)
	gLin := toLinear(g8)
	bLin := toLinear(b8)

	return 0.2126*rLin + 0.7152*gLin + 0.0722*bLin
}

// toLinear converts sRGB component to linear
func toLinear(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// EnsureContrast adjusts a color to meet minimum contrast against a background
// Returns the adjusted color and the achieved contrast ratio
func EnsureContrast(foreground, background color.Color, minRatio float64) (color.RGBA, float64) {
	currentRatio := GetContrastRatio(foreground, background)
	if currentRatio >= minRatio {
		c := foreground.(color.RGBA)
		return c, currentRatio
	}

	// Determine if we need to lighten or darken
	fgLum := getRelativeLuminance(foreground)
	bgLum := getRelativeLuminance(background)

	c := foreground.(color.RGBA)
	if bgLum > fgLum {
		// Background is lighter, darken foreground
		for i := 0; i < 10 && currentRatio < minRatio; i++ {
			c = darkenColor(c, 0.15).(color.RGBA)
			currentRatio = GetContrastRatio(c, background)
		}
	} else {
		// Background is darker, lighten foreground
		for i := 0; i < 10 && currentRatio < minRatio; i++ {
			c = brightenColor(c, 0.2).(color.RGBA)
			currentRatio = GetContrastRatio(c, background)
		}
	}

	return c, currentRatio
}

// NormalizeGrayscaleForMode creates an appropriate accent for B&W/grayscale images
// Returns a subtle tint that works with the given appearance mode
func NormalizeGrayscaleForMode(avgLuminance float64, appearance string) string {
	isLight := appearance == string(AppearanceLight)

	// For grayscale images, create subtle warm/cool metallic accent
	var hue float64
	if avgLuminance > 0.5 {
		hue = 35.0 // Warm amber for light images
	} else {
		hue = 205.0 // Cool steel blue for dark images
	}

	saturation := 0.08 // Very subtle
	var lightness float64

	if isLight {
		// Light mode: darker grays for contrast
		lightness = 0.25 + avgLuminance*0.2
	} else {
		// Dark mode: lighter grays for visibility
		lightness = 0.65 + avgLuminance*0.2
	}

	accent := HslToRgb(hue, saturation, lightness)
	return fmt.Sprintf("#%02X%02X%02X", accent.R, accent.G, accent.B)
}

// QuickHexLuminance returns approximate luminance (0-1) for a hex color
// Fast path for decisions without full HSL conversion
func QuickHexLuminance(hexColor string) float64 {
	c, err := hexToColor(hexColor)
	if err != nil {
		return 0.5
	}
	// Use standard luminance formula
	return (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255.0
}

// IsLightColor determines if a color is "light" (better for dark text)
func IsLightColor(c color.Color) bool {
	return getRelativeLuminance(c) > 0.5
}

// BlendWithMode blends two colors, mode-aware (multiply for dark, screen for light effects)
func BlendWithMode(base, overlay color.Color, amount float64, appearance string) color.Color {
	if amount <= 0 {
		return base
	}
	if amount >= 1 {
		return overlay
	}

	// Simple alpha blend as default
	return BlendColors(base, overlay, amount)
}

// ClampColorToGamut ensures RGB values are in valid range
func ClampColorToGamut(c color.RGBA) color.RGBA {
	return color.RGBA{
		R: uint8(clampFloat(float64(c.R), 0, 255)),
		G: uint8(clampFloat(float64(c.G), 0, 255)),
		B: uint8(clampFloat(float64(c.B), 0, 255)),
		A: uint8(clampFloat(float64(c.A), 0, 255)),
	}
}

// ParseHexOrDefault parses a hex color, returning default on error
func ParseHexOrDefault(hex string, defaultHex string) string {
	_, err := hexToColor(hex)
	if err != nil {
		return defaultHex
	}
	return strings.ToUpper(hex)
}
