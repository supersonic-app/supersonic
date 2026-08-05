package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/res"
)

type WhatsNewDialog struct {
	widget.BaseWidget
}

func NewWhatsNewDialog() *WhatsNewDialog {
	w := &WhatsNewDialog{}
	w.ExtendBaseWidget(w)
	return w
}

const donationTextTemplate = `Thank you for using Supersonic!
If you're enjoying the app and want to [show your support](%s),
donations are always appreciated and help motivate me to continue
delivering new features and improvements!`

func (w *WhatsNewDialog) CreateRenderer() fyne.WidgetRenderer {
	donationPrompt := widget.NewRichTextFromMarkdown(fmt.Sprintf(donationTextTemplate, res.KofiURL))
	donationPrompt.Wrapping = fyne.TextWrapWord
	objects := container.NewVBox(donationPrompt)
	if res.WhatsAdded != "" {
		added := widget.NewRichTextFromMarkdown(res.WhatsAdded)
		added.Wrapping = fyne.TextWrapWord
		objects.Add(added)
	}
	if res.WhatsFixed != "" {
		fixed := widget.NewRichTextFromMarkdown(res.WhatsFixed)
		fixed.Wrapping = fyne.TextWrapWord
		objects.Add(fixed)
	}

	c := container.NewVScroll(objects)
	return widget.NewSimpleRenderer(c)
}

func (w *WhatsNewDialog) MinSize() fyne.Size {
	return fyne.NewSize(425, 275)
}
