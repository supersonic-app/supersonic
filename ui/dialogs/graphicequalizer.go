package dialogs

import (
	"fmt"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/ui/layouts"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

type GraphicEqualizer struct {
	widget.BaseWidget

	OnChanged           func(band int, gain float64)
	OnPreampChanged     func(gain float64)
	OnLoadAutoEQProfile func()
	OnManualAdjustment  func()                  // Called when user manually changes a slider
	OnPresetSelected    func(presetName string) // Called when user selects a preset
	OnPresetDeleted     func(presetName string) // Called when user deletes a preset
	OnEQTypeChanged     func(eqType string)     // Called when EQ type is changed

	bandSliders      []*eqSlider
	preampSlider     *eqSlider
	presetSelect     *widget.Select
	eqTypeSelect     *widget.Select
	autoEQBtn        *widget.Button
	profileLabel     *widget.Label
	container        *fyne.Container
	sliderArea       *fyne.Container // Stores the slider area for dynamic rebuilding
	topBar           *fyne.Container // Stores the top bar
	eqPresets        []backend.EQPreset
	presetManager    *backend.EQPresetManager
	parentWindow     fyne.Window
	isApplyingPreset bool              // Flag to prevent clearing profile during preset application
	currentEQType    string            // Current EQ type ("ISO10Band" or "ISO15Band")
	isDirty          bool              // true when sliders modified since last preset load/save
	loadedPreset     *backend.EQPreset // currently loaded preset (nil if none)
	saveBtn          *ttwidget.Button  // reference for enable/disable control
}

func NewGraphicEqualizer(preamp float64, bandFreqs []string, bandGains []float64, eqType string, presetMgr *backend.EQPresetManager, parentWindow fyne.Window, activePresetName string) *GraphicEqualizer {
	g := &GraphicEqualizer{
		presetManager: presetMgr,
		parentWindow:  parentWindow,
		currentEQType: eqType,
	}
	g.ExtendBaseWidget(g)
	g.loadPresets()
	g.buildSliders(preamp, bandFreqs, bandGains)

	// Set the dropdown to the active preset if one exists
	if activePresetName != "" {
		g.setActivePreset(activePresetName)
		// Populate loadedPreset and detect dirty state on dialog reopen
		for i, p := range g.eqPresets {
			if p.Name == activePresetName {
				g.loadedPreset = &g.eqPresets[i]
				g.isDirty = !g.matchesPreset(p)
				break
			}
		}
	}
	g.updateSaveButtonState()

	return g
}

func (g *GraphicEqualizer) loadPresets() {
	presets, err := g.presetManager.LoadPresets()
	if err != nil {
		// Fallback to empty list if load fails
		g.eqPresets = []backend.EQPreset{}
		return
	}
	g.eqPresets = presets
}

func (g *GraphicEqualizer) buildSliders(preamp float64, bands []string, bandGains []float64) {
	// Build preset selector
	g.updatePresetSelect()

	// Build EQ type selector
	if g.eqTypeSelect == nil {
		g.eqTypeSelect = widget.NewSelect([]string{"ISO 15-Band", "ISO 10-Band"}, func(selected string) {
			// Convert display name to type
			newType := "ISO15Band"
			if selected == "ISO 10-Band" {
				newType = "ISO10Band"
			}

			if newType != g.currentEQType {
				g.currentEQType = newType
				if g.OnEQTypeChanged != nil {
					g.OnEQTypeChanged(newType)
				}
			}
		})
	}
	// Set current selection
	if g.currentEQType == "ISO10Band" {
		g.eqTypeSelect.SetSelected("ISO 10-Band")
	} else {
		g.eqTypeSelect.SetSelected("ISO 15-Band")
	}

	// Reset button
	resetBtn := widget.NewButton(lang.L("Reset"), func() {
		// Find and apply the "Flat" preset
		for _, p := range g.eqPresets {
			if p.Name == "Flat" {
				g.applyPreset(p)
				g.presetSelect.SetSelected(p.Name)
				if g.OnPresetSelected != nil {
					g.OnPresetSelected(p.Name)
				}
				break
			}
		}
	})

	// Save button (overwrites current loaded preset)
	g.saveBtn = ttwidget.NewButtonWithIcon("", myTheme.SaveIcon, func() {
		g.saveCurrentPreset()
	})
	g.saveBtn.Disable() // starts disabled
	g.saveBtn.SetToolTip(lang.L("Save"))

	// Save As button (always enabled, opens name-entry dialog)
	saveAsBtn := ttwidget.NewButtonWithIcon("", myTheme.SaveAsIcon, func() {
		g.showSaveAsDialog()
	})
	saveAsBtn.SetToolTip(lang.L("Save As"))

	// Delete button
	deleteBtn := ttwidget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		g.showDeletePresetDialog()
	})
	deleteBtn.SetToolTip(lang.L("Delete"))

	// AutoEQ button
	g.autoEQBtn = widget.NewButton(lang.L("AutoEQ"), func() {
		if g.OnLoadAutoEQProfile != nil {
			g.OnLoadAutoEQProfile()
		}
	})

	// Profile label (hidden by default)
	g.profileLabel = widget.NewLabel("")
	g.profileLabel.Hide()

	// Set minimum width for preset dropdown
	g.presetSelect.Resize(fyne.NewSize(200, g.presetSelect.MinSize().Height))

	// Top bar with controls - AutoEQ in main row for better discoverability
	topBar := container.NewVBox(
		// Main row: EQ type, preset selector, AutoEQ, and action buttons
		container.NewHBox(
			widget.NewLabel(lang.L("EQ Type:")),
			g.eqTypeSelect,
			widget.NewLabel(lang.L("EQ Preset:")),
			g.presetSelect,
			layout.NewSpacer(),
			g.saveBtn,
			saveAsBtn,
			deleteBtn,
			resetBtn,
			g.autoEQBtn,
		),
		// Second row: Profile label (shown only when AutoEQ profile is active)
		g.profileLabel,
	)

	// Build slider area
	g.sliderArea = g.buildSliderArea(preamp, bands, bandGains)

	// Store topBar and create main container
	g.topBar = topBar
	g.container = container.NewBorder(g.topBar, nil, nil, nil, g.sliderArea)
}

func (g *GraphicEqualizer) buildSliderArea(preamp float64, bands []string, bandGains []float64) *fyne.Container {
	// Range labels
	rng := container.NewVBox(
		newCaptionTextSizeLabel("+12", fyne.TextAlignTrailing),
		layout.NewSpacer(),
		newCaptionTextSizeLabel("0 dB", fyne.TextAlignTrailing),
		layout.NewSpacer(),
		newCaptionTextSizeLabel("-12", fyne.TextAlignTrailing),
	)

	g.bandSliders = make([]*eqSlider, len(bands))
	bandSlidersCtr := container.New(layouts.NewGridLayoutWithColumnsAndPadding(len(bands)+2, -16))

	// Preamp slider
	pre := newCaptionTextSizeLabel(lang.L("EQ Preamp"), fyne.TextAlignCenter)
	g.preampSlider = newEQSlider()
	g.preampSlider.SetValue(preamp)
	g.preampSlider.OnChanged = func(f float64) {
		if g.OnPreampChanged != nil {
			g.OnPreampChanged(f)
		}
		g.preampSlider.UpdateToolTip()
		if !g.isApplyingPreset {
			g.isDirty = true
			g.updateSaveButtonState()
			if g.OnManualAdjustment != nil {
				g.OnManualAdjustment()
			}
		}
	}
	g.preampSlider.UpdateToolTip()
	bandSlidersCtr.Add(container.NewBorder(nil, pre, nil, nil, g.preampSlider))
	bandSlidersCtr.Add(container.NewBorder(nil, widget.NewLabel(""), nil, nil, rng))

	// Band sliders
	for i, band := range bands {
		s := newEQSlider()
		if i < len(bandGains) {
			s.SetValue(bandGains[i])
			s.UpdateToolTip()
		}
		_i := i
		s.OnChanged = func(f float64) {
			if g.OnChanged != nil {
				g.OnChanged(_i, f)
			}
			g.bandSliders[_i].UpdateToolTip()
			if !g.isApplyingPreset {
				g.isDirty = true
				g.updateSaveButtonState()
				if g.OnManualAdjustment != nil {
					g.OnManualAdjustment()
				}
			}
		}
		l := newCaptionTextSizeLabel(band, fyne.TextAlignCenter)
		c := container.NewBorder(nil, l, nil, nil, s)
		bandSlidersCtr.Add(c)
		g.bandSliders[i] = s
	}

	return container.NewStack(
		container.NewBorder(nil, widget.NewLabel(""), nil, nil,
			container.NewBorder(nil, nil, util.NewHSpace(5), util.NewHSpace(5),
				container.NewVBox(
					layout.NewSpacer(),
					myTheme.NewThemedRectangle(theme.ColorNameInputBackground),
					layout.NewSpacer(),
				),
			),
		),
		bandSlidersCtr,
	)
}

// RebuildForEQType rebuilds the sliders for a new EQ type
func (g *GraphicEqualizer) RebuildForEQType(eqType string, bandGains []float64) {
	// Determine band frequencies for the new type
	var bands []string
	if eqType == "ISO10Band" {
		bands = []string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}
	} else {
		bands = []string{"25", "40", "63", "100", "160", "250", "400", "630", "1k", "1.6k", "2.5k", "4k", "6.3k", "10k", "16k"}
	}

	// Get current preamp value
	currentPreamp := 0.0
	if g.preampSlider != nil {
		currentPreamp = g.preampSlider.Value
	}

	// Rebuild the slider area
	newSliderArea := g.buildSliderArea(currentPreamp, bands, bandGains)

	// Replace the old slider area in the container
	g.sliderArea = newSliderArea
	g.container.Objects = []fyne.CanvasObject{g.topBar, g.sliderArea}
	g.container.Refresh()

	// Clear loaded preset when EQ type changes
	g.loadedPreset = nil
	g.isDirty = false
	g.updateSaveButtonState()
}

func (g *GraphicEqualizer) updatePresetSelect() {
	presetNames := make([]string, len(g.eqPresets))
	for i, p := range g.eqPresets {
		displayName := p.Name
		if !p.IsBuiltin {
			displayName = p.Name + " *" // Mark custom presets with asterisk
		}
		presetNames[i] = displayName
	}

	if g.presetSelect == nil {
		g.presetSelect = widget.NewSelect(presetNames, func(selected string) {
			// Remove asterisk marker if present
			cleanName := selected
			if len(selected) > 2 && selected[len(selected)-2:] == " *" {
				cleanName = selected[:len(selected)-2]
			}
			for _, p := range g.eqPresets {
				if p.Name == cleanName {
					g.applyPreset(p)
					if g.OnPresetSelected != nil {
						g.OnPresetSelected(cleanName)
					}
					break
				}
			}
		})
		g.presetSelect.PlaceHolder = lang.L("EQ Preset")
	} else {
		g.presetSelect.Options = presetNames
		g.presetSelect.Refresh()
	}
}

// setActivePreset sets the dropdown selection to match the given preset name
func (g *GraphicEqualizer) setActivePreset(presetName string) {
	if presetName == "" {
		return
	}

	// Find the preset and determine display name (with asterisk for custom)
	for _, p := range g.eqPresets {
		if p.Name == presetName {
			displayName := p.Name
			if !p.IsBuiltin {
				displayName = p.Name + " *"
			}
			g.presetSelect.SetSelected(displayName)
			return
		}
	}
}

func (g *GraphicEqualizer) applyPreset(preset backend.EQPreset) {
	g.isApplyingPreset = true
	defer func() { g.isApplyingPreset = false }()

	// If preset type differs from current type, switch type first
	if preset.Type != "" && preset.Type != g.currentEQType {
		g.currentEQType = preset.Type
		// Update the type selector UI
		if preset.Type == "ISO10Band" {
			g.eqTypeSelect.SetSelected("ISO 10-Band")
		} else {
			g.eqTypeSelect.SetSelected("ISO 15-Band")
		}
		// Notify about type change
		if g.OnEQTypeChanged != nil {
			g.OnEQTypeChanged(preset.Type)
		}
	}

	// Apply preamp
	g.preampSlider.SetValue(preset.Preamp)
	g.preampSlider.UpdateToolTip()
	if g.OnPreampChanged != nil {
		g.OnPreampChanged(preset.Preamp)
	}

	// Apply band gains
	for i, gain := range preset.Bands {
		if i < len(g.bandSliders) {
			g.bandSliders[i].SetValue(gain)
			g.bandSliders[i].UpdateToolTip()
			if g.OnChanged != nil {
				g.OnChanged(i, gain)
			}
		}
	}

	// Track the loaded preset and clear dirty state
	presetCopy := preset
	g.loadedPreset = &presetCopy
	g.isDirty = false
	g.updateSaveButtonState()
}

func (g *GraphicEqualizer) getCurrentSettings() backend.EQPreset {
	bands := make([]float64, len(g.bandSliders))
	for i, slider := range g.bandSliders {
		bands[i] = slider.Value
	}
	return backend.EQPreset{
		Type:   g.currentEQType,
		Preamp: g.preampSlider.Value,
		Bands:  bands,
	}
}

func (g *GraphicEqualizer) updateSaveButtonState() {
	if g.saveBtn == nil {
		return
	}
	if g.loadedPreset != nil && !g.loadedPreset.IsBuiltin && g.isDirty {
		g.saveBtn.Enable()
	} else {
		g.saveBtn.Disable()
	}
}

func (g *GraphicEqualizer) saveCurrentPreset() {
	if g.loadedPreset == nil || g.loadedPreset.IsBuiltin {
		return
	}
	g.savePresetWithName(g.loadedPreset.Name)
}

func (g *GraphicEqualizer) savePresetWithName(name string) {
	preset := g.getCurrentSettings()
	preset.Name = name
	preset.IsBuiltin = false

	if err := g.presetManager.SavePreset(preset); err != nil {
		dialog.ShowError(err, g.parentWindow)
		return
	}

	// Update loaded preset and clear dirty state
	g.loadedPreset = &preset
	g.isDirty = false
	g.updateSaveButtonState()

	// Reload presets and update UI
	g.loadPresets()
	g.updatePresetSelect()

	// Select the newly saved preset
	g.presetSelect.SetSelected(preset.Name + " *")
	if g.OnPresetSelected != nil {
		g.OnPresetSelected(preset.Name)
	}
}

func (g *GraphicEqualizer) showSaveAsDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(lang.L("Preset name"))

	// Pre-fill with loaded preset name if it's a custom preset
	if g.loadedPreset != nil && !g.loadedPreset.IsBuiltin {
		nameEntry.SetText(g.loadedPreset.Name)
	}

	formDialog := dialog.NewForm(
		lang.L("Save Preset As"),
		lang.L("Save"),
		lang.L("Cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(lang.L("Name"), nameEntry),
		},
		func(confirmed bool) {
			if !confirmed || nameEntry.Text == "" {
				return
			}

			name := nameEntry.Text

			// Check if name matches a builtin preset
			for _, p := range g.eqPresets {
				if p.Name == name && p.IsBuiltin {
					dialog.ShowInformation(
						lang.L("Invalid Name"),
						lang.L("Cannot use the name of a builtin preset"),
						g.parentWindow,
					)
					return
				}
			}

			// Check if name matches an existing custom preset
			for _, p := range g.eqPresets {
				if p.Name == name && !p.IsBuiltin {
					dialog.ShowConfirm(
						lang.L("Overwrite Preset"),
						fmt.Sprintf(lang.L("Preset '%s' already exists. Overwrite?"), name),
						func(overwrite bool) {
							if overwrite {
								g.savePresetWithName(name)
							}
						},
						g.parentWindow,
					)
					return
				}
			}

			g.savePresetWithName(name)
		},
		g.parentWindow,
	)

	formDialog.Resize(fyne.NewSize(400, 150))
	formDialog.Show()
}

// matchesPreset compares current slider values against a preset
func (g *GraphicEqualizer) matchesPreset(preset backend.EQPreset) bool {
	if g.preampSlider == nil {
		return false
	}
	if math.Abs(g.preampSlider.Value-preset.Preamp) > 0.05 {
		return false
	}
	if len(g.bandSliders) != len(preset.Bands) {
		return false
	}
	for i, slider := range g.bandSliders {
		if math.Abs(slider.Value-preset.Bands[i]) > 0.05 {
			return false
		}
	}
	return true
}

// ClearLoadedPresetState clears the loaded preset and dirty state
func (g *GraphicEqualizer) ClearLoadedPresetState() {
	g.loadedPreset = nil
	g.isDirty = false
	g.updateSaveButtonState()
}

func (g *GraphicEqualizer) showDeletePresetDialog() {
	selected := g.presetSelect.Selected
	if selected == "" {
		dialog.ShowInformation(lang.L("No Preset Selected"), lang.L("Please select a preset to delete"), g.parentWindow)
		return
	}

	// Remove asterisk marker if present
	cleanName := selected
	if len(selected) > 2 && selected[len(selected)-2:] == " *" {
		cleanName = selected[:len(selected)-2]
	}

	// Find the preset
	var presetToDelete *backend.EQPreset
	for i, p := range g.eqPresets {
		if p.Name == cleanName {
			presetToDelete = &g.eqPresets[i]
			break
		}
	}

	if presetToDelete == nil || presetToDelete.IsBuiltin {
		dialog.ShowInformation(lang.L("Cannot Delete"), lang.L("Cannot delete builtin presets"), g.parentWindow)
		return
	}

	dialog.ShowConfirm(
		lang.L("Delete Preset"),
		fmt.Sprintf(lang.L("Delete preset '%s'?"), cleanName),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			if err := g.presetManager.DeletePreset(cleanName); err != nil {
				dialog.ShowError(err, g.parentWindow)
				return
			}

			// Notify about deletion
			if g.OnPresetDeleted != nil {
				g.OnPresetDeleted(cleanName)
			}

			// Reload presets and update UI
			g.loadPresets()
			g.updatePresetSelect()
			g.presetSelect.ClearSelected()
		},
		g.parentWindow,
	)
}

// SetProfileLabel displays the name of the applied AutoEQ profile
func (g *GraphicEqualizer) SetProfileLabel(profileName string) {
	if profileName == "" {
		g.profileLabel.SetText("")
		g.profileLabel.Hide()
	} else {
		g.profileLabel.SetText(fmt.Sprintf("%s: %s", lang.L("Profile"), profileName))
		g.profileLabel.Show()
	}
}

// ClearProfileLabel hides the profile label (called on manual adjustment)
func (g *GraphicEqualizer) ClearProfileLabel() {
	g.SetProfileLabel("")
}

// ClearPresetSelection clears the preset dropdown selection
func (g *GraphicEqualizer) ClearPresetSelection() {
	g.presetSelect.ClearSelected()
}

func newCaptionTextSizeLabel(text string, alignment fyne.TextAlign) *widget.RichText {
	l := widget.NewRichTextWithText(text)
	ts := l.Segments[0].(*widget.TextSegment)
	ts.Style.SizeName = theme.SizeNameCaptionText
	ts.Style.Alignment = alignment
	return l
}

func (g *GraphicEqualizer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}

type eqSlider struct {
	ttwidget.Slider
}

func newEQSlider() *eqSlider {
	s := &eqSlider{
		Slider: ttwidget.Slider{
			Slider: widget.Slider{
				Orientation: widget.Vertical,
				Min:         -12,
				Max:         12,
				Step:        0.1,
			},
		},
	}
	s.UpdateToolTip()
	s.ExtendBaseWidget(s)
	return s
}

func (s *eqSlider) UpdateToolTip() {
	s.SetToolTip(fmt.Sprintf("%0.1f dB", s.Value))
}

// We implement our own double tapping so that the Tapped behavior
// can be triggered instantly.
func (s *eqSlider) DoubleTapped(e *fyne.PointEvent) {
	s.SetValue(0)
}
