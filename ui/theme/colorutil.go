package theme

import (
	"image/color"
	"math"
)

// rgbToHsl converts RGB to HSL color space with float64 precision
// Returns hue (0-360), saturation (0-1), lightness (0-1)
func rgbToHsl(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	delta := max - min

	// Lightness
	l = (max + min) / 2.0

	// Saturation
	if delta == 0 {
		s = 0
	} else {
		s = delta / (1.0 - math.Abs(2.0*l-1.0))
	}

	// Hue
	if delta == 0 {
		h = 0
	} else {
		switch max {
		case rf:
			h = math.Mod((gf-bf)/delta, 6.0)
			if gf < bf {
				h += 6.0
			}
		case gf:
			h = (bf-rf)/delta + 2.0
		case bf:
			h = (rf-gf)/delta + 4.0
		}
		h *= 60.0
	}

	return h, s, l
}

// RgbToHslColor converts a color.Color to HSL values (exported for external use)
func RgbToHslColor(c color.Color) (h, s, l float64) {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit to 8-bit
	return rgbToHsl(uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// HslToRgb converts HSL to RGB color space (exported for external use)
// Takes hue (0-360), saturation (0-1), lightness (0-1)
func HslToRgb(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}

	var r, g, b float64

	if s == 0 {
		// Grayscale
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q

		hk := h / 360.0

		r = hueToRgb(p, q, hk+1.0/3.0)
		g = hueToRgb(p, q, hk)
		b = hueToRgb(p, q, hk-1.0/3.0)
	}

	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}

// hueToRgb is a helper function for HslToRgb
func hueToRgb(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

// boostSaturation increases the saturation of a color by the given factor
func boostSaturation(r, g, b uint8, factor float64) (uint8, uint8, uint8) {
	h, s, l := rgbToHsl(r, g, b)

	// Boost saturation
	s = math.Min(s*factor, 1.0)

	// Convert back to RGB
	c := HslToRgb(h, s, l)
	return c.R, c.G, c.B
}

// brightenColor brightens a color by the given fraction (0-1)
func brightenColor(c color.Color, fraction float64) color.Color {
	r, g, b, a := c.RGBA()
	r, g, b = brightenComponent(r, fraction), brightenComponent(g, fraction), brightenComponent(b, fraction)
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// darkenColor darkens a color by the given fraction (0-1)
func darkenColor(c color.Color, fraction float64) color.Color {
	r, g, b, a := c.RGBA()
	r, g, b = darkenComponent(r, fraction), darkenComponent(g, fraction), darkenComponent(b, fraction)
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func brightenComponent(component uint32, fraction float64) uint32 {
	brightened := min(component+uint32(float64(component)*fraction), 0xffff)
	return brightened
}

func darkenComponent(component uint32, fraction float64) uint32 {
	i := uint32(float64(component) * fraction)
	if i > component {
		return 0
	}
	return component - i
}

// clampFloat constrains a float value between min and max
func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
