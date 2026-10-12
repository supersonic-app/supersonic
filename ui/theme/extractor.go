package theme

import (
	"fmt"
	"image"
	"math"
	"sort"
)

// ColorExtractor extracts vibrant accent colors from images
type ColorExtractor struct {
	// SampleRate: process 1 pixel every N pixels (default 10)
	SampleRate int
}

// NewColorExtractor creates a new extractor with default settings
func NewColorExtractor() *ColorExtractor {
	return &ColorExtractor{
		SampleRate: 10, // Sample every 10th pixel for performance
	}
}

// ExtractAccentFromImage extracts a vibrant accent color from an image.
// appearance is used for grayscale images to generate an appropriate accent ("Dark", "Light", or "Auto").
// Returns hex color string (e.g., "#FF6A00") or error.
func (ce *ColorExtractor) ExtractAccentFromImage(img image.Image, appearance string) (string, error) {
	if img == nil {
		return "", fmt.Errorf("nil image")
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width == 0 || height == 0 {
		return "", fmt.Errorf("empty image")
	}

	// Collect color samples
	type colorScore struct {
		r, g, b    uint8
		h, s, l    float64
		saturation float64
		vibrancy   float64
	}

	var samples []colorScore

	// Sample pixels with stride
	stride := ce.SampleRate
	if stride < 1 {
		stride = 1
	}

	for y := 0; y < height; y += stride {
		for x := 0; x < width; x += stride {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// Convert from 16-bit to 8-bit
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)

			h, s, l := rgbToHsl(r8, g8, b8)

			// Calculate vibrancy: prefer saturated colors with medium luminance
			// Penalize grays (low saturation), blacks (low luminance), and whites (high luminance)
			vibrancy := s * (1.0 - math.Abs(l-0.5)*2.0) // Max at l=0.5, zero at l=0 or l=1

			samples = append(samples, colorScore{
				r: r8, g: g8, b: b8,
				h: h, s: s, l: l,
				saturation: s,
				vibrancy:   vibrancy,
			})
		}
	}

	if len(samples) == 0 {
		return "", fmt.Errorf("no samples collected")
	}

	// Check if image is essentially monochrome (B&W or desaturated)
	// Calculate average saturation and luminance across all samples
	var totalSaturation, totalLuminance float64
	for _, s := range samples {
		totalSaturation += s.saturation
		totalLuminance += s.l
	}
	avgSaturation := totalSaturation / float64(len(samples))
	avgLuminance := totalLuminance / float64(len(samples))

	// If image is essentially black & white (low saturation), use normalizer for coherent accent
	if avgSaturation < 0.25 {
		// Return placeholder - actual color will be normalized by caller based on theme mode
		return NormalizeGrayscaleForMode(avgLuminance, appearance), nil
	}

	// Filter out low-quality colors (grays, near-blacks, near-whites)
	var candidates []colorScore
	for _, s := range samples {
		// Require minimum saturation and reasonable luminance
		if s.saturation > 0.25 && s.l > 0.15 && s.l < 0.85 {
			candidates = append(candidates, s)
		}
	}

	// If no good candidates, relax constraints
	if len(candidates) == 0 {
		for _, s := range samples {
			if s.saturation > 0.15 && s.l > 0.1 && s.l < 0.9 {
				candidates = append(candidates, s)
			}
		}
	}

	// Still no candidates? Use any non-extreme color
	if len(candidates) == 0 {
		for _, s := range samples {
			if s.l > 0.05 && s.l < 0.95 {
				candidates = append(candidates, s)
			}
		}
	}

	if len(candidates) == 0 {
		// Fallback to average of all samples
		var sumR, sumG, sumB int64
		for _, s := range samples {
			sumR += int64(s.r)
			sumG += int64(s.g)
			sumB += int64(s.b)
		}
		avgR := uint8(sumR / int64(len(samples)))
		avgG := uint8(sumG / int64(len(samples)))
		avgB := uint8(sumB / int64(len(samples)))
		return fmt.Sprintf("#%02X%02X%02X", avgR, avgG, avgB), nil
	}

	// Group colors by hue buckets for finding dominant colors
	const hueBuckets = 12 // 30 degrees each
	buckets := make([][]colorScore, hueBuckets)

	for _, c := range candidates {
		bucket := int(c.h/30.0) % hueBuckets
		buckets[bucket] = append(buckets[bucket], c)
	}

	// Score each bucket by total vibrancy
	type bucketScore struct {
		index    int
		vibrancy float64
		count    int
	}

	var scores []bucketScore
	for i, bucket := range buckets {
		if len(bucket) > 0 {
			var totalVibrancy float64
			for _, c := range bucket {
				totalVibrancy += c.vibrancy
			}
			scores = append(scores, bucketScore{
				index:    i,
				vibrancy: totalVibrancy,
				count:    len(bucket),
			})
		}
	}

	// Sort by vibrancy descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].vibrancy > scores[j].vibrancy
	})

	// Get the best bucket and average its colors
	if len(scores) == 0 {
		return "", fmt.Errorf("no color buckets found")
	}

	bestBucket := buckets[scores[0].index]

	// Average colors in the best bucket, weighted by vibrancy
	var sumR, sumG, sumB, weight float64
	for _, c := range bestBucket {
		w := c.vibrancy + 0.1 // Base weight to avoid zero
		sumR += float64(c.r) * w
		sumG += float64(c.g) * w
		sumB += float64(c.b) * w
		weight += w
	}

	if weight == 0 {
		return "", fmt.Errorf("zero weight")
	}

	avgR := uint8(sumR / weight)
	avgG := uint8(sumG / weight)
	avgB := uint8(sumB / weight)

	// Boost saturation slightly for more vibrant accent
	avgR, avgG, avgB = boostSaturation(avgR, avgG, avgB, 1.15)

	return fmt.Sprintf("#%02X%02X%02X", avgR, avgG, avgB), nil
}
