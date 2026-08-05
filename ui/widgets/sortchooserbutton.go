package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/ui/theme"
)

type SortChooserButton struct {
	widget.BaseWidget

	Sorts     []string
	AlignLeft bool

	OnChanged func(selIndex int)

	disabled      bool
	selectedIndex int
	btn           *IconButton
}

func NewSortChooserButton(sorts []string, onChanged func(selIdx int)) *SortChooserButton {
	s := &SortChooserButton{Sorts: sorts, OnChanged: onChanged}
	s.ExtendBaseWidget(s)
	return s
}

func (s *SortChooserButton) SelectedIndex() int {
	return s.selectedIndex
}

func (s *SortChooserButton) SetSelectedIndex(idx int) {
	if idx < 0 || idx > len(s.Sorts)-1 {
		return
	}

	s.selectedIndex = idx
}

func (s *SortChooserButton) Disable() {
	if !s.disabled {
		s.disabled = true
		s.Refresh()
	}
}

func (s *SortChooserButton) Enable() {
	if s.disabled {
		s.disabled = false
		s.Refresh()
	}
}

func (s *SortChooserButton) Disabled() bool {
	return s.disabled
}

func (s *SortChooserButton) Refresh() {
	if s.btn != nil {
		if s.disabled {
			s.btn.Disable()
		} else {
			s.btn.Enable()
		}
	}
	s.BaseWidget.Refresh()
}

func (s *SortChooserButton) CreateRenderer() fyne.WidgetRenderer {
	s.btn = NewIconButton(theme.SortIcon, s.showMenu)
	s.btn.SetToolTip(lang.L("Sort"))
	return widget.NewSimpleRenderer(s.btn)
}

func (s *SortChooserButton) showMenu() {
	m := fyne.NewMenu("")
	for i, lbl := range s.Sorts {
		_i := i
		item := fyne.NewMenuItem(lbl, func() {
			s.selectedIndex = _i
			if s.OnChanged != nil {
				s.OnChanged(_i)
			}
		})
		if i == s.selectedIndex {
			item.Checked = true
		}
		m.Items = append(m.Items, item)
	}
	btnPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(s)
	btnSize := s.Size()
	pop := widget.NewPopUpMenu(m, fyne.CurrentApp().Driver().CanvasForObject(s))
	menuW := pop.MinSize().Width
	if s.AlignLeft {
		pop.ShowAtPosition(fyne.NewPos(btnPos.X+btnSize.Width-menuW, btnPos.Y+btnSize.Height))
	} else {
		pop.ShowAtPosition(fyne.NewPos(btnPos.X, btnPos.Y+btnSize.Height))
	}
}
