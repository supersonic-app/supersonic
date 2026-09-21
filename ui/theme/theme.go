package theme

import (
	"bytes"
	"errors"
	"image/color"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/res"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

const (
	ThemeFileDynamic = "dynamic" // Special theme file name for dynamic accent color theme

	ColorNameListHeader        fyne.ThemeColorName = "ListHeader"
	ColorNamePageBackground    fyne.ThemeColorName = "PageBackground"
	ColorNamePageHeader        fyne.ThemeColorName = "PageHeader"
	ColorNameInactiveLyric     fyne.ThemeColorName = "InactiveLyric"
	ColorNameIconButton        fyne.ThemeColorName = "IconButton"
	ColorNameHoveredIconButton fyne.ThemeColorName = "HoveredIconButton"
	ColorNameActiveIconButton  fyne.ThemeColorName = "ActiveIconButton"
	ColorNameNowPlayingPanel   fyne.ThemeColorName = "NowPlayingPanel"

	SizeNameSubSubHeadingText fyne.ThemeSizeName = "subSubHeadingText" // in between Text and SubHeadingText
	SizeNameSubText           fyne.ThemeSizeName = "subText"           // in between Text and Caption
	SizeNameSuffixText        fyne.ThemeSizeName = "suffixText"        // a tiny bit smaller than subText

	SizeNameImageCornerRadius fyne.ThemeSizeName = "imageCornerRadius"

	AnimationDurationShort  = canvas.DurationShort
	AnimationDurationMedium = 225 * time.Millisecond
	AnimationDurationLong   = canvas.DurationStandard

	HeaderImageSize        = 225
	CompactHeaderImageSize = 95
)

var (
	GridViewHoveredIconColor color.Color = color.White
	GridViewIconColor        color.Color = darkenColor(color.White, 0.2)

	AlbumIcon         fyne.Resource = theme.NewThemedResource(res.ResDiscSvg)
	ArtistIcon        fyne.Resource = theme.NewThemedResource(res.ResPeopleSvg)
	AutoplayIcon      fyne.Resource = theme.NewThemedResource(res.ResInfinitySvg)
	CastIcon          fyne.Resource = theme.NewThemedResource(res.ResCastSvg)
	RadioIcon         fyne.Resource = theme.NewThemedResource(res.ResBroadcastSvg)
	FavoriteIcon      fyne.Resource = theme.NewThemedResource(res.ResHeartFilledSvg)
	NotFavoriteIcon   fyne.Resource = theme.NewThemedResource(res.ResHeartOutlineSvg)
	HeadphonesIcon    fyne.Resource = theme.NewThemedResource(res.ResHeadphonesSvg)
	PlaylistIcon      fyne.Resource = theme.NewThemedResource(res.ResPlaylistSvg)
	PlayNextIcon      fyne.Resource = theme.NewThemedResource(res.ResPlaylistAddNextSvg)
	PlayQueueIcon     fyne.Resource = theme.NewThemedResource(res.ResPlayqueueSvg)
	ShareIcon         fyne.Resource = theme.NewThemedResource(res.ResShareSvg)
	ShuffleIcon       fyne.Resource = theme.NewThemedResource(res.ResShuffleSvg)
	TracksIcon        fyne.Resource = theme.NewThemedResource(res.ResMusicnotesSvg)
	GenreIcon         fyne.Resource = theme.NewThemedResource(res.ResTheatermasksSvg)
	FilterIcon        fyne.Resource = theme.NewThemedResource(res.ResFilterSvg)
	RepeatIcon        fyne.Resource = theme.NewThemedResource(res.ResRepeatSvg)
	RepeatOneIcon     fyne.Resource = theme.NewThemedResource(res.ResRepeatoneSvg)
	SidebarIcon       fyne.Resource = theme.NewThemedResource(res.ResSidebarSvg)
	SortIcon          fyne.Resource = theme.NewThemedResource(res.ResUpdownarrowSvg)
	VisualizationIcon fyne.Resource = theme.NewThemedResource(res.ResOscilloscopeSvg)
	LibraryIcon       fyne.Resource = theme.NewThemedResource(res.ResLibrarySvg)
	SaveIcon          fyne.Resource = theme.NewThemedResource(res.ResSaveSvg)
	SaveAsIcon        fyne.Resource = theme.NewThemedResource(res.ResSaveasSvg)
)

type AppearanceMode string

const (
	AppearanceLight AppearanceMode = "Light"
	AppearanceDark  AppearanceMode = "Dark"
	AppearanceAuto  AppearanceMode = "Auto"

	DefaultAppearance AppearanceMode = AppearanceDark
)

var (
	normalFont fyne.Resource
	boldFont   fyne.Resource
)

type MyTheme struct {
	NormalFont   string
	BoldFont     string
	config       *backend.ThemeConfig
	themeFileDir string

	loadedThemeFilename string
	loadedThemeFile     *ThemeFile
	defaultThemeFile    *ThemeFile

	cachedPalette       *Palette
	cachedPaletteConfig struct {
		accentColor string
		saturation  float64
		contrast    float64
		appearance  string
	}
	paletteMu sync.Mutex // protects cachedPalette and cachedPaletteConfig
}

var _ fyne.Theme = (*MyTheme)(nil)

func NewMyTheme(config *backend.ThemeConfig, themeFileDir string) *MyTheme {
	m := &MyTheme{config: config, themeFileDir: themeFileDir}
	var err error
	if m.defaultThemeFile, err = DecodeThemeFile(bytes.NewReader(res.ResDefaultToml.StaticContent)); err != nil {
		log.Fatalf("Failed to load builtin theme: %v", err.Error())
	}
	// Ensure dynamic theme has a default accent color matching the classic Supersonic blue
	if config.AccentColor == "" {
		config.AccentColor = "#286ef4" // Classic Supersonic blue (from Default theme Hyperlink)
	}
	return m
}

// ReloadThemeFile reloads the currently loaded theme file.
func (m *MyTheme) ReloadThemeFile() {
	m.loadedThemeFile = nil
	m.paletteMu.Lock()
	m.cachedPalette = nil
	m.paletteMu.Unlock()
}

// InvalidatePaletteCache clears only the palette cache for instant accent updates
// without the overhead of full theme reload. Use this when only accent settings change.
func (m *MyTheme) InvalidatePaletteCache() {
	m.paletteMu.Lock()
	m.cachedPalette = nil
	m.paletteMu.Unlock()
}

// GetConfig returns the current theme configuration for UI synchronization
func (m *MyTheme) GetConfig() *backend.ThemeConfig {
	return m.config
}

// SetAccentColor sets the accent color immediately without transition.
// This invalidates the cache and applies the new color instantly.
func (m *MyTheme) SetAccentColor(accentHex string) {
	m.config.AccentColor = accentHex

	// Generate new palette and apply immediately
	palette, err := GeneratePalette(accentHex, m.config.Saturation, m.config.Contrast, m.config.Appearance)
	if err != nil {
		log.Printf("Failed to generate palette: %v", err)
		return
	}

	m.paletteMu.Lock()
	m.cachedPalette = palette
	m.paletteMu.Unlock()

	// Notify Fyne to invalidate cache and redraw all widgets
	fyne.Do(func() {
		fyne.CurrentApp().Settings().SetTheme(m)
	})
}

func (m *MyTheme) getColorFromPalette(name fyne.ThemeColorName, palette *Palette) color.Color {
	switch name {
	case ColorNameInactiveLyric:
		return BlendColors(palette.TextPrimary, palette.Surface, 0.33)
	case ColorNameHoveredIconButton:
		// Hover: pure accent color
		return palette.Accent
	case ColorNameActiveIconButton:
		// Active/Selected: brightened accent for clear distinction from hover
		return brightenColor(palette.Accent, 0.25)
	case ColorNameIconButton:
		// Default: use neutral text color (no accent) for consistency
		return palette.TextSecondary
	case ColorNameNowPlayingPanel:
		r, g, b, _ := palette.PageHeader.RGBA()
		return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 180}
	case ColorNameListHeader:
		return palette.ListHeader
	case ColorNamePageBackground:
		return palette.PageBackground
	case ColorNamePageHeader:
		return palette.PageHeader
	case theme.ColorNameBackground:
		return palette.Background
	case theme.ColorNameButton:
		return palette.Surface
	case theme.ColorNameDisabled:
		return palette.TextSecondary
	case theme.ColorNameDisabledButton:
		return palette.Surface
	case theme.ColorNameError:
		return palette.Danger
	case theme.ColorNameFocus:
		// Use SurfaceHover for focus to match selection/hover behavior
		return palette.SurfaceHover
	case theme.ColorNameForeground:
		return palette.TextPrimary
	case theme.ColorNameForegroundOnPrimary:
		// Use TextOnAccent for text/icons on primary/accent buttons
		return palette.TextOnAccent
	case theme.ColorNameHover:
		return palette.SurfaceHover
	case theme.ColorNameHyperlink:
		return palette.Hyperlink
	case theme.ColorNameInputBackground:
		return palette.Surface
	case theme.ColorNameInputBorder:
		return palette.Surface
	case theme.ColorNameMenuBackground:
		// Use MenuBackground for menu background - subtle accent tint
		// that doesn't compete with the stronger SurfaceHover used for focus/selection
		return palette.MenuBackground
	case theme.ColorNameOverlayBackground:
		return palette.Background
	case theme.ColorNamePlaceHolder:
		return palette.TextSecondary
	case theme.ColorNamePressed:
		return palette.Accent
	case theme.ColorNamePrimary:
		// Primary: use accent color directly for checkboxes, tabs, buttons
		return palette.Accent
	case theme.ColorNameScrollBar:
		return palette.Hyperlink
	case theme.ColorNameScrollBarBackground:
		return palette.SurfaceHover
	case theme.ColorNameSelection:
		// Use surfaceHover for selection - it's already calibrated based on surface luminance
		// This ensures good contrast in both light and dark modes
		return palette.SurfaceHover
	case theme.ColorNameSeparator:
		return palette.Surface
	case theme.ColorNameShadow:
		// Use neutral gray for shadows to avoid color tint from accent
		// Shadow should be neutral and not draw attention to itself
		_, _, bgL := RgbToHslColor(palette.Background)
		var gray uint8
		if bgL > 0.5 {
			// Light mode: dark gray shadow
			gray = 0x00
		} else {
			// Dark mode: slightly lighter shadow for visibility
			gray = 0x10
		}
		return color.RGBA{R: gray, G: gray, B: gray, A: 100}
	case theme.ColorNameSuccess:
		return palette.Success
	case theme.ColorNameWarning:
		return palette.Accent
	default:
		return palette.TextPrimary
	}
}

func (m *MyTheme) AppearanceMode() AppearanceMode {
	v := DefaultAppearance // default if config has invalid or missing setting
	if slices.Contains(
		[]string{string(AppearanceLight), string(AppearanceDark), string(AppearanceAuto)},
		m.config.Appearance) {
		v = AppearanceMode(m.config.Appearance)
	}
	return v
}

func (m *MyTheme) Color(name fyne.ThemeColorName, defVariant fyne.ThemeVariant) color.Color {
	// Use custom accent palette if Dynamic theme is selected
	if m.config.ThemeFile == ThemeFileDynamic {
		m.paletteMu.Lock()
		// Check if we need to regenerate the palette
		if m.cachedPalette == nil ||
			m.cachedPaletteConfig.accentColor != m.config.AccentColor ||
			m.cachedPaletteConfig.saturation != m.config.Saturation ||
			m.cachedPaletteConfig.contrast != m.config.Contrast ||
			m.cachedPaletteConfig.appearance != m.config.Appearance {

			palette, err := GeneratePalette(m.config.AccentColor, m.config.Saturation, m.config.Contrast, m.config.Appearance)
			if err != nil {
				log.Printf("failed to generate palette: %v", err)
				// Fall through to TOML theme
			} else {
				m.cachedPalette = palette
				m.cachedPaletteConfig.accentColor = m.config.AccentColor
				m.cachedPaletteConfig.saturation = m.config.Saturation
				m.cachedPaletteConfig.contrast = m.config.Contrast
				m.cachedPaletteConfig.appearance = m.config.Appearance
			}
		}
		palette := m.cachedPalette
		m.paletteMu.Unlock()

		// Use cached palette outside lock
		if palette != nil {
			return m.getColorFromPalette(name, palette)
		}
	}

	// load theme file if necessary
	if m.loadedThemeFile == nil || m.config.ThemeFile != m.loadedThemeFilename {
		t, err := ReadThemeFile(path.Join(m.themeFileDir, m.config.ThemeFile))
		if err == nil {
			m.loadedThemeFile = t
		} else {
			log.Printf("failed to load theme file %q: %s", m.config.ThemeFile, err.Error())
			m.loadedThemeFile = m.defaultThemeFile
		}
		m.loadedThemeFilename = m.config.ThemeFile
	}

	variant := m.getVariant(defVariant)
	thFile := m.loadedThemeFile
	if !thFile.SupportsVariant(variant) {
		thFile = m.defaultThemeFile
	}
	colors := thFile.DarkColors
	defColors := m.defaultThemeFile.DarkColors
	if variant == theme.VariantLight {
		colors = thFile.LightColors
		defColors = m.defaultThemeFile.LightColors
	}
	switch name {
	case ColorNameInactiveLyric:
		// average the Foreground and Disabled colors
		foreground := colorOrDefault(colors.Foreground, defColors.Foreground, theme.ColorNameForeground, variant)
		disabled := colorOrDefault(colors.Disabled, defColors.Disabled, theme.ColorNameDisabled, variant)
		return BlendColors(foreground, disabled, 0.33)
	case ColorNameHoveredIconButton:
		foreground := colorOrDefault(colors.Foreground, defColors.Foreground, theme.ColorNameForeground, variant)
		if variant == theme.VariantDark {
			// For dark theme, use a lighter version of the foreground color for hover
			return brightenColor(foreground, 0.33)
		}
		// For light theme, use a darker version of the foreground color for hover
		return darkenColor(foreground, 0.33)
	case ColorNameActiveIconButton:
		// For theme files, use Primary color for active/selected icons (backward compatible)
		return colorOrDefault(colors.Primary, defColors.Primary, theme.ColorNamePrimary, variant)
	case ColorNameIconButton:
		foreground := colorOrDefault(colors.Foreground, defColors.Foreground, theme.ColorNameForeground, variant)
		if variant == theme.VariantDark {
			return darkenColor(foreground, 0.05)
		}
		return brightenColor(foreground, 0.2)
	case ColorNameNowPlayingPanel:
		pageHeader := colorOrDefault(colors.PageHeader, defColors.PageHeader, name, variant).(color.RGBA)
		pageHeader.A = 180
		return pageHeader
	case ColorNameListHeader:
		return colorOrDefault(colors.ListHeader, defColors.ListHeader, name, variant)
	case ColorNamePageBackground:
		return colorOrDefault(colors.PageBackground, defColors.PageBackground, name, variant)
	case ColorNamePageHeader:
		return colorOrDefault(colors.PageHeader, defColors.PageHeader, name, variant)
	case theme.ColorNameBackground:
		return colorOrDefault(colors.Background, defColors.Background, name, variant)
	case theme.ColorNameButton:
		return colorOrDefault(colors.Button, defColors.Button, name, variant)
	case theme.ColorNameDisabled:
		return colorOrDefault(colors.Disabled, defColors.Disabled, name, variant)
	case theme.ColorNameDisabledButton:
		return colorOrDefault(colors.DisabledButton, defColors.DisabledButton, name, variant)
	case theme.ColorNameError:
		return colorOrDefault(colors.Error, defColors.Error, name, variant)
	case theme.ColorNameFocus:
		return colorOrDefault(colors.Focus, defColors.Focus, name, variant)
	case theme.ColorNameForeground:
		return colorOrDefault(colors.Foreground, defColors.Foreground, name, variant)
	case theme.ColorNameHover:
		return colorOrDefault(colors.Hover, defColors.Hover, name, variant)
	case theme.ColorNameHyperlink:
		return colorOrDefault(colors.Hyperlink, defColors.Hyperlink, name, variant)
	case theme.ColorNameInputBackground:
		return colorOrDefault(colors.InputBackground, defColors.InputBackground, name, variant)
	case theme.ColorNameInputBorder:
		return colorOrDefault(colors.InputBorder, defColors.InputBorder, name, variant)
	case theme.ColorNameMenuBackground:
		return colorOrDefault(colors.MenuBackground, defColors.MenuBackground, name, variant)
	case theme.ColorNameOverlayBackground:
		return colorOrDefault(colors.OverlayBackground, defColors.OverlayBackground, name, variant)
	case theme.ColorNamePlaceHolder:
		return colorOrDefault(colors.Placeholder, defColors.Placeholder, name, variant)
	case theme.ColorNamePressed:
		return colorOrDefault(colors.Pressed, defColors.Pressed, name, variant)
	case theme.ColorNamePrimary:
		return colorOrDefault(colors.Primary, defColors.Primary, name, variant)
	case theme.ColorNameScrollBar:
		return colorOrDefault(colors.ScrollBar, defColors.ScrollBar, name, variant)
	case theme.ColorNameSelection:
		return colorOrDefault(colors.Selection, defColors.Selection, name, variant)
	case theme.ColorNameSeparator:
		return colorOrDefault(colors.Separator, defColors.Separator, name, variant)
	case theme.ColorNameShadow:
		return colorOrDefault(colors.Shadow, defColors.Shadow, name, variant)
	case theme.ColorNameSuccess:
		return colorOrDefault(colors.Success, defColors.Success, name, variant)
	case theme.ColorNameWarning:
		return colorOrDefault(colors.Warning, defColors.Warning, name, variant)
	default:
		return colorOrDefault("", "", name, variant)
	}
}

func colorOrDefault(colorStr, defColorStr string, name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if c, err := ColorStringToColor(colorStr); err == nil {
		return c
	}
	if c, err := ColorStringToColor(defColorStr); err == nil {
		return c
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (m *MyTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Returns a map [themeFileName] -> displayName
func (m *MyTheme) ListThemeFiles() map[string]string {
	// Use filepath.Join to create a cross-platform file path
	pattern := filepath.Join(m.themeFileDir, "/*.toml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("Failed to glob files: %v", err)
	}

	result := make(map[string]string)
	for _, filePath := range files {
		// Clean the path to avoid issues with slashes
		cleanPath := filepath.Clean(filePath)

		// Now read the theme file, using the cleaned path
		if themeFile, err := ReadThemeFile(cleanPath); err == nil {
			result[filepath.Base(cleanPath)] = themeFile.SupersonicTheme.Name
		} else {
			log.Printf("Failed to load theme file: %s, error: %v", cleanPath, err)
		}
	}
	return result
}

func (m *MyTheme) Font(style fyne.TextStyle) fyne.Resource {
	switch style {
	case fyne.TextStyle{}:
		if m.NormalFont != "" && normalFont == nil {
			if content, err := readTTFFile(m.NormalFont); err != nil {
				m.NormalFont = ""
				m.BoldFont = ""
			} else {
				normalFont = fyne.NewStaticResource("normalFont", content)
			}
		}
		if normalFont != nil {
			return normalFont
		}
	case fyne.TextStyle{Bold: true}:
		if m.BoldFont != "" && boldFont == nil {
			if content, err := os.ReadFile(m.BoldFont); err != nil {
				m.BoldFont = ""
			} else {
				normalFont = fyne.NewStaticResource("boldFont", content)
			}
		}
		if boldFont != nil {
			return boldFont
		}
		if normalFont != nil {
			return normalFont
		}
	}
	return theme.DefaultTheme().Font(style)
}

func (m *MyTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case SizeNameSubSubHeadingText:
		return 15.5
	case SizeNameSubText:
		return 13
	case SizeNameSuffixText:
		return 12
	case theme.SizeNameScrollBar:
		return 14
	case SizeNameImageCornerRadius:
		if m.config.UseRoundedImageCorners {
			return theme.InputRadiusSize()
		}
		return 0
	default:
		return theme.DefaultTheme().Size(name)
	}
}

func (m *MyTheme) getVariant(defVariant fyne.ThemeVariant) fyne.ThemeVariant {
	v := DefaultAppearance // default if config has invalid or missing setting
	if slices.Contains(
		[]string{string(AppearanceLight), string(AppearanceDark), string(AppearanceAuto)},
		m.config.Appearance) {
		v = AppearanceMode(m.config.Appearance)
	}

	if AppearanceMode(v) == AppearanceDark {
		return theme.VariantDark
	} else if AppearanceMode(v) == AppearanceLight {
		return theme.VariantLight
	}
	return defVariant
}

// IsDarkMode returns true if the current theme is in dark mode.
// It checks the Supersonic config first (Appearance setting), falling back to OS detection.
func IsDarkMode(app fyne.App) bool {
	appTheme := app.Settings().Theme()
	if themePtr, ok := appTheme.(*MyTheme); ok {
		cfg := themePtr.GetConfig()
		appearanceLower := strings.ToLower(cfg.Appearance)
		if appearanceLower == "dark" {
			return true
		} else if appearanceLower == "light" {
			return false
		}
		// "auto" - fall back to OS detection
	}
	// Fallback to OS detection
	return app.Settings().ThemeVariant() == theme.VariantDark
}

func BlendColors(a, b color.Color, fractionA float64) color.Color {
	ra, ga, ba, aa := a.RGBA()
	rb, gb, bb, ab := b.RGBA()

	fractionB := 1 - fractionA
	rAvg := uint8(float64(ra/257)*fractionA + float64(rb/257)*fractionB)
	gAvg := uint8(float64(ga/257)*fractionA + float64(gb/257)*fractionB)
	bAvg := uint8(float64(ba/257)*fractionA + float64(bb/257)*fractionB)
	aAvg := uint8(float64(aa/257)*fractionA + float64(ab/257)*fractionB)
	return color.RGBA{R: rAvg, G: gAvg, B: bAvg, A: aAvg}
}

func readTTFFile(filepath string) ([]byte, error) {
	if !strings.HasSuffix(filepath, ".ttf") {
		err := errors.New("only .ttf fonts are supported")
		log.Printf("error loading custom font %q: %s", filepath, err.Error())
		return nil, err
	}
	content, err := os.ReadFile(filepath)
	if err != nil {
		log.Printf("error loading custom font %q: %s", filepath, err.Error())
	}
	return content, err
}
