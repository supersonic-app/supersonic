package util

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/res"
	"github.com/supersonic-app/supersonic/sharedutil"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"golang.org/x/net/html"
)

type DateFormat int

const (
	DateFormatDMY DateFormat = iota
	DateFormatMDY
	DateFormatYMD
)

func FyneDoFunc(f func()) func() {
	return func() {
		fyne.Do(f)
	}
}

func dateFormatForLocale(locale string) DateFormat {
	var region string
	if i := strings.Index(locale, "-"); i > 0 && len(locale) >= 5 {
		region = locale[i+1 : i+3]
	}
	switch strings.ToUpper(region) {
	case "US":
		return DateFormatMDY
	case "CN", "JP", "KR", "HU", "MN", "LT":
		return DateFormatYMD
	default:
		return DateFormatDMY
	}
}

func shortMonthName(month int) string {
	months := [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sept", "Oct", "Nov", "Dec"}
	if month >= 1 && month <= 12 {
		return lang.L(months[month-1])
	}
	return ""
}

func FormatItemDate(date mediaprovider.ItemDate) string {
	var sb strings.Builder
	df := dateFormatForLocale(fyne.CurrentDevice().Locale().String())
	switch df {
	case DateFormatDMY:
		if d := date.Day; d != nil {
			sb.WriteString(fmt.Sprintf("%d ", *d))
		}
		if m := date.Month; m != nil {
			sb.WriteString(shortMonthName(*m) + " ")
		}
		if y := date.Year; y != nil {
			sb.WriteString(fmt.Sprintf("%d", *y))
		}
	case DateFormatMDY:
		if m := date.Month; m != nil {
			sb.WriteString(shortMonthName(*m) + " ")
		}
		if d := date.Day; d != nil {
			sb.WriteString(fmt.Sprintf("%d, ", *d))
		}
		if y := date.Year; y != nil {
			sb.WriteString(fmt.Sprintf("%d", *y))
		}
	case DateFormatYMD:
		if y := date.Year; y != nil {
			sb.WriteString(fmt.Sprintf("%d ", *y))
		}
		if m := date.Month; m != nil {
			sb.WriteString(shortMonthName(*m) + " ")
		}
		if d := date.Day; d != nil {
			sb.WriteString(fmt.Sprintf("%d", *d))
		}
	}
	return sb.String()
}

func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	v := []int{
		t.Local().Year(),
		int(t.Local().Month()),
		t.Local().Day(),
	}
	id := mediaprovider.ItemDate{
		Year:  &v[0],
		Month: &v[1],
		Day:   &v[2],
	}

	return FormatItemDate(id)
}

func LastPlayedDisplayString(t time.Time) string {
	if t.IsZero() {
		return lang.L("never")
	}
	switch d := time.Since(t); {
	case d.Hours() < 1:
		mins := int(d.Minutes())
		return lang.LocalizePluralKey("x_minutes_ago",
			fmt.Sprintf("%d minutes ago", mins), mins,
			map[string]string{"minutes": strconv.Itoa(mins)})
	case d.Hours() < 24:
		hrs := int(d.Hours())
		return lang.LocalizePluralKey("x_hours_ago",
			fmt.Sprintf("%d hours ago", hrs), hrs,
			map[string]string{"hours": strconv.Itoa(hrs)})
	case d.Hours() < 24*31:
		days := int(d.Hours() / 24)
		return lang.LocalizePluralKey("x_days_ago",
			fmt.Sprintf("%d days ago", days), days,
			map[string]string{"days": strconv.Itoa(days)})
	case d.Hours() < 24*365:
		months := int(d.Hours() / (24 * 31))
		return lang.LocalizePluralKey("x_months_ago",
			fmt.Sprintf("%d months ago", months), months,
			map[string]string{"months": strconv.Itoa(months)})
	default:
		years := int(d.Hours() / (24 * 365))
		return lang.LocalizePluralKey("x_years_ago",
			fmt.Sprintf("%d years ago", years), years,
			map[string]string{"years": strconv.Itoa(years)})
	}
}

var BoldRichTextStyle = widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, Inline: true}

func MakeOpaque(c color.Color) color.Color {
	if nrgba, ok := c.(color.NRGBA); ok {
		nrgba.A = 255
		return nrgba
	}
	return c
}

func SecondsToMMSS(s float64) string {
	if s < 0 {
		s = 0
	}
	sec := int(math.Round(s))
	min := sec / 60
	sec -= min * 60

	return fmt.Sprintf("%d:%02d", min, sec)
}

func SecondsToTimeString(s float64) string {
	if s < 3600 /*1 hour*/ {
		return SecondsToMMSS(s)
	}
	sec := int64(s)
	days := sec / 86400
	sec -= days * 86400
	hr := sec / 3600
	sec -= hr * 3600
	min := sec / 60
	sec -= min * 60

	var str string
	if days > 0 {
		daysStr := lang.L("days")
		if days == 1 {
			daysStr = lang.L("day")
		}
		str = fmt.Sprintf("%d %s ", days, daysStr)
	}
	if hr > 0 {
		hrStr := lang.L("hrs")
		if hr == 1 {
			hrStr = lang.L("hr")
		}
		str += fmt.Sprintf("%d %s ", hr, hrStr)
	}
	if min > 0 {
		str += fmt.Sprintf("%d %s ", min, lang.L("min"))
	}
	if sec > 0 {
		str += fmt.Sprintf("%d %s ", sec, lang.L("sec"))
	}
	return str[:len(str)-1]
}

func BytesToSizeString(bytes int64) string {
	var num float64
	var suffix string
	switch b := float64(bytes); {
	case b > 1000000000:
		suffix = "GB"
		num = b / 1000000000
	case b > 1000000:
		suffix = "MB"
		num = b / 1000000
	case b > 1000:
		suffix = "KB"
		num = b / 1000
	default:
		suffix = "B"
		num = b
	}
	return fmt.Sprintf(fmtStringForThreeSigFigs(num)+" %s", num, suffix)
}

func fmtStringForThreeSigFigs(num float64) string {
	switch {
	case num >= 100:
		return "%0.0f"
	case num >= 10:
		return "%0.1f"
	default:
		return "%0.2f"
	}
}

func ImageAspect(im image.Image) float32 {
	b := im.Bounds()
	return float32(b.Max.X-b.Min.X) / float32(b.Max.Y-b.Min.Y)
}

// Debouncer returns a function that will call callOnDone when
// it has not been invoked since the last dur interval.
func NewDebouncer(dur time.Duration, callOnDone func()) func() {
	var mu sync.Mutex
	var timer *time.Timer
	return func() {
		mu.Lock()
		defer mu.Unlock()

		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(dur, func() {
			fyne.Do(callOnDone)
		})
	}
}

func PlaintextFromHTMLString(s string) string {
	tokr := html.NewTokenizer(strings.NewReader(s))

	var text string
	var isLink bool
	var done bool
	for !done {
		tt := tokr.Next()
		switch {
		case tt == html.ErrorToken:
			done = true
		case tt == html.StartTagToken:
			t := tokr.Token()
			isLink = t.Data == "a"
		case tt == html.EndTagToken:
			isLink = false
		case tt == html.TextToken:
			t := tokr.Token()
			// for now, skip displaying Navidrome's "Read more on Last.FM" link
			if !isLink {
				text = text + t.Data
			}
		}
	}
	return text
}

func DisplayReleaseType(releaseTypes mediaprovider.ReleaseTypes) string {
	baseType := lang.L("Album")
	switch {
	case releaseTypes&mediaprovider.ReleaseTypeAudiobook > 0:
		baseType = lang.L("Audiobook")
	case releaseTypes&mediaprovider.ReleaseTypeAudioDrama > 0:
		baseType = lang.L("Audio Drama")
	case releaseTypes&mediaprovider.ReleaseTypeBroadcast > 0:
		baseType = lang.L("Broadcast")
	case releaseTypes&mediaprovider.ReleaseTypeDJMix > 0:
		baseType = lang.L("DJ-Mix")
	case releaseTypes&mediaprovider.ReleaseTypeEP > 0:
		baseType = lang.L("EP")
	case releaseTypes&mediaprovider.ReleaseTypeFieldRecording > 0:
		baseType = lang.L("Field Recording")
	case releaseTypes&mediaprovider.ReleaseTypeInterview > 0:
		baseType = lang.L("Interview")
	case releaseTypes&mediaprovider.ReleaseTypeMixtape > 0:
		baseType = lang.L("Mixtape")
	case releaseTypes&mediaprovider.ReleaseTypeSingle > 0:
		baseType = lang.L("Single")
	case releaseTypes&mediaprovider.ReleaseTypeSoundtrack > 0:
		baseType = lang.L("Soundtrack")
	}

	var modifiers []string
	if releaseTypes&mediaprovider.ReleaseTypeLive > 0 {
		modifiers = append(modifiers, lang.L("Live"))
	}
	if releaseTypes&mediaprovider.ReleaseTypeDemo > 0 {
		modifiers = append(modifiers, lang.L("Demo"))
	}
	if releaseTypes&mediaprovider.ReleaseTypeRemix > 0 {
		modifiers = append(modifiers, lang.L("Remix"))
	}
	if releaseTypes&mediaprovider.ReleaseTypeSpokenWord > 0 {
		modifiers = append(modifiers, lang.L("Spoken Word"))
	}
	if releaseTypes&mediaprovider.ReleaseTypeCompilation > 0 {
		modifiers = append(modifiers, lang.L("Compilation"))
	}

	modifiers = append(modifiers, baseType)
	return strings.Join(modifiers, " ")
}

func NewRatingSubmenu(onSetRating func(int)) *fyne.MenuItem {
	newRatingMenuItem := func(rating int) *fyne.MenuItem {
		label := fmt.Sprintf("(%s)", lang.L("none"))
		if rating > 0 {
			label = strconv.Itoa(rating)
		}
		return fyne.NewMenuItem(label, func() {
			onSetRating(rating)
		})
	}
	ratingMenu := fyne.NewMenuItem(lang.L("Set rating"), nil)
	ratingMenu.Icon = theme.NewThemedResource(res.ResStarOutlineSvg)
	ratingMenu.ChildMenu = fyne.NewMenu("", []*fyne.MenuItem{
		newRatingMenuItem(0),
		newRatingMenuItem(1),
		newRatingMenuItem(2),
		newRatingMenuItem(3),
		newRatingMenuItem(4),
		newRatingMenuItem(5),
	}...)
	return ratingMenu
}

func AddHeaderBackground(obj fyne.CanvasObject) *fyne.Container {
	return AddHeaderBackgroundWithColorName(obj, myTheme.ColorNamePageHeader)
}

func AddHeaderBackgroundWithColorName(obj fyne.CanvasObject, colorName fyne.ThemeColorName) *fyne.Container {
	bgrnd := myTheme.NewThemedRectangle(colorName)
	bgrnd.CornerRadius = theme.InputRadiusSize()
	return container.NewStack(bgrnd,
		container.New(&layout.CustomPaddedLayout{LeftPadding: 10, RightPadding: 10, TopPadding: 10, BottomPadding: 10},
			obj))
}

func NewTruncatingRichText() *widget.RichText {
	rt := widget.NewRichTextWithText("")
	rt.Truncation = fyne.TextTruncateEllipsis
	return rt
}

func NewTruncatingLabel() *widget.Label {
	rt := widget.NewLabel("")
	rt.Truncation = fyne.TextTruncateEllipsis
	return rt
}

func NewTruncatingTooltipRichText() *ttwidget.RichText {
	rt := ttwidget.NewRichTextWithText("")
	rt.Truncation = fyne.TextTruncateEllipsis
	return rt
}

func NewTruncatingTooltipLabel() *ttwidget.Label {
	rt := ttwidget.NewLabel("")
	rt.Truncation = fyne.TextTruncateEllipsis
	return rt
}

func NewTrailingAlignLabel() *widget.Label {
	rt := widget.NewLabel("")
	rt.Alignment = fyne.TextAlignTrailing
	return rt
}

func SaveWindowSize(w fyne.Window, wPtr, hPtr *int) {
	// round sizes to even to avoid Wayland issues with 2x scaling factor
	// https://github.com/supersonic-app/supersonic/issues/212
	*wPtr = int(math.RoundToEven(float64(w.Canvas().Size().Width)))
	*hPtr = int(math.RoundToEven(float64(w.Canvas().Size().Height)))
}

func LocalizeSlice(s []string) []string {
	return sharedutil.MapSlice(s, func(s string) string { return lang.L(s) })
}

type ToolTipRichText struct {
	ttwidget.RichText

	OnMouseIn  func(e *desktop.MouseEvent)
	OnMouseOut func()
	OnTapped   func(e *fyne.PointEvent)
}

type Space struct {
	widget.BaseWidget

	Width  float32
	Height float32
}

func NewHSpace(w float32) *Space {
	s := &Space{Width: w}
	s.ExtendBaseWidget(s)
	return s
}

func NewVSpace(h float32) *Space {
	s := &Space{Height: h}
	s.ExtendBaseWidget(s)
	return s
}

func (h *Space) MinSize() fyne.Size {
	return fyne.NewSize(h.Width, h.Height)
}

func (h *Space) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(layout.NewSpacer())
}
