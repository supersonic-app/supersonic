package browsing

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/boxes-ltd/imaging"
	"github.com/cenkalti/dominantcolor"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

// BackgroundManager handles background blur/gradient for pages
type BackgroundManager struct {
	BackgroundImgA     *canvas.Image
	BackgroundImgB     *canvas.Image
	BackgroundGradient *canvas.LinearGradient

	CachedBlurredImage  image.Image
	CachedDominantColor color.Color
	mu                  sync.Mutex // protects CachedBlurredImage, CachedDominantColor, and generation

	// Context for cancelling ongoing background processing
	ctx    context.Context
	cancel context.CancelFunc

	// Generation counter to prevent race conditions between goroutines
	generation uint64
}

// NewBackgroundManager creates background widgets
func NewBackgroundManager() *BackgroundManager {
	b := &BackgroundManager{
		BackgroundImgA:     canvas.NewImageFromImage(nil),
		BackgroundImgB:     canvas.NewImageFromImage(nil),
		BackgroundGradient: canvas.NewLinearGradient(color.Transparent, color.Transparent, 0),
	}
	b.BackgroundImgA.Hidden = true
	b.BackgroundImgB.Hidden = true
	// Use Stretch with pre-processed "cover" sized images
	b.BackgroundImgA.FillMode = canvas.ImageFillStretch
	b.BackgroundImgB.FillMode = canvas.ImageFillStretch
	b.ctx, b.cancel = context.WithCancel(context.Background())
	return b
}

// ApplyBackground applies background based on mode: "blur", "gradient", or "disabled"
// When mode is empty, it defaults to gradient behavior (for nowplaying page compatibility)
func (b *BackgroundManager) ApplyBackground(img image.Image, mode string) {
	if img == nil || mode == "disabled" {
		b.HideImages()
		return
	}

	// Cancel any ongoing processing before starting new work
	if b.cancel != nil {
		b.cancel()
	}
	b.ctx, b.cancel = context.WithCancel(context.Background())

	// Clear cache if image changed and increment generation
	b.mu.Lock()
	if img != b.CachedBlurredImage {
		b.CachedBlurredImage = nil
		b.CachedDominantColor = nil
	}
	b.generation++
	currentGen := b.generation
	b.mu.Unlock()

	// If blur is requested, apply blurred background
	if mode == "blur" {
		b.mu.Lock()
		cachedImg := b.CachedBlurredImage
		b.mu.Unlock()
		if cachedImg != nil {
			b.applyBlurredBackground(cachedImg)
			return
		}
		go func(ctx context.Context, sourceImg image.Image, gen uint64) {
			// Performance optimization: resize to small size first, then blur
			// Use 16:9 aspect ratio to match screen proportions and avoid stretching
			// Fyne will stretch this small image to fill the screen
			resized := imaging.Fill(sourceImg, 128, 72, imaging.Center, imaging.NearestNeighbor)
			flipped := imaging.FlipH(resized)
			adjusted := adjustBrightnessForTheme(flipped)

			select {
			case <-ctx.Done():
				return
			default:
			}

			blurred := imaging.Blur(adjusted, 5.0)

			// Double-check context before writing to cache
			select {
			case <-ctx.Done():
				return
			default:
			}

			b.mu.Lock()
			// Only cache if generation hasn't changed (no newer request started)
			if b.generation == gen {
				b.CachedBlurredImage = blurred
			}
			b.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			default:
				fyne.Do(func() {
					b.applyBlurredBackground(blurred)
				})
			}
		}(b.ctx, img, currentGen)
	} else {
		// Default to gradient (handles mode="gradient" and mode="" for nowplaying)
		b.mu.Lock()
		cachedColor := b.CachedDominantColor
		b.mu.Unlock()
		if cachedColor != nil {
			// Even for cached colors, re-apply theme adjustment
			// Theme may have changed since color was cached
			adjusted := adjustColorForTheme(cachedColor)
			b.applyGradientBackground(adjusted)
			return
		}
		go func(ctx context.Context, sourceImg image.Image, gen uint64) {
			c := dominantcolor.Find(sourceImg)
			// Adjust the extracted color based on theme
			adjusted := adjustColorForTheme(c)

			// Double-check context before writing to cache
			select {
			case <-ctx.Done():
				return
			default:
			}

			b.mu.Lock()
			// Only cache if generation hasn't changed (no newer request started)
			if b.generation == gen {
				b.CachedDominantColor = c
			}
			b.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			default:
				fyne.Do(func() {
					b.applyGradientBackground(adjusted)
				})
			}
		}(b.ctx, img, currentGen)
	}
}

// HideImages hides the background images
func (b *BackgroundManager) HideImages() {
	if !b.BackgroundImgA.Hidden {
		b.BackgroundImgA.Hide()
		b.BackgroundImgB.Hide()
	}
}

// adjustBrightnessForTheme adjusts image brightness based on current theme
// Uses the palette's background lightness as reference for coherent styling
func adjustBrightnessForTheme(img image.Image) image.Image {
	// Use centralized theme detection
	isDark := myTheme.IsDarkMode(fyne.CurrentApp())

	// Get the current theme to access palette
	appTheme := fyne.CurrentApp().Settings().Theme()
	var targetLuminance float64

	if themePtr, ok := appTheme.(*myTheme.MyTheme); ok && themePtr.GetConfig().ThemeFile == "dynamic" {
		// Use palette reference for coherent adjustment
		cfg := themePtr.GetConfig()
		palette, err := myTheme.GeneratePalette(cfg.AccentColor,
			cfg.Saturation,
			cfg.Contrast,
			cfg.Appearance)
		if err == nil && palette != nil {
			// Target: make image align with page background luminance
			r, g, b, _ := palette.PageBackground.RGBA()
			targetLuminance = (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
		}
	}

	if targetLuminance == 0 {
		// Fallback to default behavior
		if isDark {
			return imaging.AdjustBrightness(img, -40.0)
		}
		return imaging.AdjustBrightness(img, 30.0)
	}

	// Calculate adjustment needed to match target luminance
	// Assuming average image has ~0.5 luminance
	adjustment := (targetLuminance - 0.5) * 100 // Scale to imaging adjustment range
	return imaging.AdjustBrightness(img, adjustment)
}

// adjustColorForTheme adjusts the extracted dominant color for theme coherence
// It creates a soft glow by locking the luminance away from text colors,
// guaranteeing legibility without needing complex clash-detection hacks.
func adjustColorForTheme(c color.Color) color.Color {
	// Use centralized theme detection from theme package
	isDark := myTheme.IsDarkMode(fyne.CurrentApp())

	cH, cS, _ := myTheme.RgbToHslColor(c)

	// Premium UI: Smooth, tasteful color tint instead of raw dominant color
	// Cap saturation so it's elegant (not garish or neon)
	targetS := math.Min(cS, 0.65)
	if targetS < 0.15 {
		targetS = 0.15 // Prevent it from being entirely grayscale
	}

	// Strict Luminance for guaranteed contrast and premium "glow" look
	var targetL float64
	if isDark {
		targetL = 0.16 // Dark, rich glow (contrasts with white text)
	} else {
		targetL = 0.88 // Soft, pastel glow (contrasts with black text)
	}

	result := myTheme.HslToRgb(cH, targetS, targetL)
	return result
}

// SetGradientEndColor sets the end color of the gradient (should be theme background)
func (b *BackgroundManager) SetGradientEndColor(c color.Color) {
	b.BackgroundGradient.EndColor = c
	b.BackgroundGradient.Refresh()
}

// ensureGradientEndColor sets the EndColor to theme background if it's currently zero/transparent
func (b *BackgroundManager) ensureGradientEndColor() {
	cr, cg, cb, ca := b.BackgroundGradient.EndColor.RGBA()
	// Check if color is zero/transparent (all components zero)
	if cr == 0 && cg == 0 && cb == 0 && ca == 0 {
		b.BackgroundGradient.EndColor = theme.Color(theme.ColorNameBackground)
	}
}

// applyBlurredBackground applies a pre-processed blurred image with smooth transition
func (b *BackgroundManager) applyBlurredBackground(blurred image.Image) {
	// Check if coming from gradient mode (images were hidden)
	fromGradient := b.BackgroundImgA.Hidden && b.BackgroundImgB.Hidden

	// Make gradient transparent so images show through
	b.BackgroundGradient.StartColor = color.Transparent
	b.BackgroundGradient.Refresh()

	if fromGradient {
		// Coming from gradient: just show the image immediately, no crossfade
		b.BackgroundImgA.Hidden = false
		b.BackgroundImgB.Hidden = false
		b.BackgroundImgA.Image = blurred
		b.BackgroundImgB.Image = blurred
		b.BackgroundImgA.Translucency = 0.0
		b.BackgroundImgB.Translucency = 0.0
		b.BackgroundImgA.Refresh()
		b.BackgroundImgB.Refresh()
	} else {
		// Already showing blurred: crossfade to new image
		b.BackgroundImgA.Hidden = false
		b.BackgroundImgB.Hidden = false
		b.BackgroundImgA.Image = b.BackgroundImgB.Image
		b.BackgroundImgB.Image = blurred
		b.BackgroundImgA.Translucency = 0.0
		b.BackgroundImgB.Translucency = 1.0
		b.BackgroundImgA.Refresh()
		b.BackgroundImgB.Refresh()

		fyne.NewAnimation(myTheme.AnimationDurationMedium, func(f float32) {
			b.BackgroundImgA.Translucency = float64(f)
			b.BackgroundImgB.Translucency = float64(1 - f)
			b.BackgroundImgA.Refresh()
			b.BackgroundImgB.Refresh()
		}).Start()
	}
}

// applyGradientBackground applies a gradient with the given dominant color
// The EndColor of the gradient should be set by the caller (page) to match the theme background
func (b *BackgroundManager) applyGradientBackground(c color.Color) {
	b.ensureGradientEndColor()
	// Compare colors by RGBA values (handles different color types)
	cr, cg, cb, ca := c.RGBA()
	sr, sg, sb, sa := b.BackgroundGradient.StartColor.RGBA()
	if cr == sr && cg == sg && cb == sb && ca == sa && b.BackgroundImgA.Hidden && b.BackgroundImgB.Hidden {
		return
	}

	// Ensure images stay visible during transition
	wasHiddenA := b.BackgroundImgA.Hidden
	wasHiddenB := b.BackgroundImgB.Hidden

	b.BackgroundImgA.Hidden = false
	b.BackgroundImgB.Hidden = false

	// Ensure gradient is visible
	b.BackgroundGradient.Hidden = false
	// Set gradient StartColor to the dominant color
	b.BackgroundGradient.StartColor = c
	b.BackgroundGradient.Refresh()

	// If images were hidden, make them fully transparent so gradient shows through immediately
	if wasHiddenA {
		b.BackgroundImgA.Translucency = 1.0
	}
	if wasHiddenB {
		b.BackgroundImgB.Translucency = 1.0
	}

	// Animate images fading out to reveal the gradient smoothly
	startTranslucencyA := b.BackgroundImgA.Translucency
	startTranslucencyB := b.BackgroundImgB.Translucency

	fyne.NewAnimation(myTheme.AnimationDurationMedium, func(f float32) {
		// Fade out images to reveal gradient underneath
		if !wasHiddenA {
			b.BackgroundImgA.Translucency = startTranslucencyA + (1-startTranslucencyA)*float64(f)
		}
		if !wasHiddenB {
			b.BackgroundImgB.Translucency = startTranslucencyB + (1-startTranslucencyB)*float64(f)
		}
		b.BackgroundImgA.Refresh()
		b.BackgroundImgB.Refresh()

		// Hide images completely at end
		if f >= 0.99 {
			b.BackgroundImgA.Hide()
			b.BackgroundImgB.Hide()
		}
	}).Start()
}
