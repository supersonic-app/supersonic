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
	"time"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/res"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

const (
	ColorNameListHeader        fyne.ThemeColorName = "ListHeader"
	ColorNamePageBackground    fyne.ThemeColorName = "PageBackground"
	ColorNamePageHeader        fyne.ThemeColorName = "PageHeader"
	ColorNameInactiveLyric     fyne.ThemeColorName = "InactiveLyric"
	ColorNameIconButton        fyne.ThemeColorName = "IconButton"
	ColorNameHoveredIconButton fyne.ThemeColorName = "HoveredIconButton"
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
}

var _ fyne.Theme = (*MyTheme)(nil)

func NewMyTheme(config *backend.ThemeConfig, themeFileDir string) *MyTheme {
	m := &MyTheme{config: config, themeFileDir: themeFileDir}
	var err error
	if m.defaultThemeFile, err = DecodeThemeFile(bytes.NewReader(res.ResDefaultToml.StaticContent)); err != nil {
		log.Fatalf("Failed to load builtin theme: %v", err.Error())
	}
	return m
}

// ReloadThemeFile reloads the currently loaded theme file.
func (m *MyTheme) ReloadThemeFile() {
	m.loadedThemeFile = nil
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
