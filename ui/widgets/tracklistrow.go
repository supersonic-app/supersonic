package widgets

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

var (
	tracklistUpdateCounter = util.NewEventCounter(20)

	emptyTrack = util.TrackListModel{
		Item: &mediaprovider.Track{
			ID:            "dummy",
			Title:         "—",
			ArtistIDs:     []string{"—"},
			ArtistNames:   []string{"—"},
			Album:         "—",
			Genres:        []string{"—"},
			ComposerNames: []string{"—"},
			ComposerIDs:   []string{"—"},
			Comment:       "—",
			FilePath:      "—",
			ContentType:   "—",
		},
	}
)

const tracklistThumbnailSize = 48

type TracklistColumn struct {
	Name string
	Col  ListColumn
}

const (
	ColumnNum         = "Num"
	ColumnTitle       = "Title"
	ColumnArtist      = "Artist"
	ColumnAlbumArtist = "AlbumArtist"
	ColumnTitleArtist = "Title/Artist"
	ColumnAlbum       = "Album"
	ColumnComposer    = "Composer"
	ColumnGenre       = "Genre"
	ColumnTime        = "Time"
	ColumnYear        = "Year"
	ColumnFavorite    = "Favorite"
	ColumnRating      = "Rating"
	ColumnPlays       = "Plays"
	ColumnLastPlayed  = "LastPlayed"
	ColumnComment     = "Comment"
	ColumnBPM         = "BPM"
	ColumnBitrate     = "Bitrate"
	ColumnSize        = "Size"
	ColumnFileType    = "FileType"
	ColumnDateAdded   = "DateAdded"
	ColumnPath        = "Path"
)

var (
	ExpandedTracklistRowColumns []TracklistColumn

	ExpandedTracklistRowColumnWidths []float32

	CompactTracklistRowColumns []TracklistColumn

	CompactTracklistRowColumnWidths []float32

	initTracklistColumns = sync.OnceFunc(func() {
		title := lang.L("Title")
		artist := lang.L("Artist")
		album := lang.L("Album")
		albumArtist := lang.L("Album artist")
		composer := lang.L("Composer")
		genre := lang.L("Genre")
		time := lang.L("Time")
		year := lang.L("Year")
		fav := lang.L("Fav.")
		rating := lang.L("Rating")
		plays := lang.L("Plays")
		lastPlayed := lang.L("Last played")
		comment := lang.L("Comment")
		bpm := lang.L("BPM")
		bitrate := lang.L("Bit rate")
		size := lang.L("Size")
		fileType := lang.L("File type")
		dateAdded := lang.L("Date added")
		filepath := lang.L("File path")

		CompactTracklistRowColumns = []TracklistColumn{
			{Name: ColumnNum, Col: ListColumn{Text: "#", Alignment: fyne.TextAlignTrailing, CanToggleVisible: false}},
			{Name: ColumnTitle, Col: ListColumn{Text: title, Alignment: fyne.TextAlignLeading, CanToggleVisible: false}},
			{Name: ColumnArtist, Col: ListColumn{Text: artist, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnAlbum, Col: ListColumn{Text: album, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnAlbumArtist, Col: ListColumn{Text: albumArtist, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnComposer, Col: ListColumn{Text: composer, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnGenre, Col: ListColumn{Text: genre, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnTime, Col: ListColumn{Text: time, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnYear, Col: ListColumn{Text: year, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnFavorite, Col: ListColumn{Text: " " + fav, Alignment: fyne.TextAlignCenter, CanToggleVisible: true}},
			{Name: ColumnRating, Col: ListColumn{Text: rating, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnPlays, Col: ListColumn{Text: plays, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnLastPlayed, Col: ListColumn{Text: lastPlayed, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnComment, Col: ListColumn{Text: comment, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnBPM, Col: ListColumn{Text: bpm, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnBitrate, Col: ListColumn{Text: bitrate, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnSize, Col: ListColumn{Text: size, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnFileType, Col: ListColumn{Text: fileType, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnDateAdded, Col: ListColumn{Text: dateAdded, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnPath, Col: ListColumn{Text: filepath, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
		}

		ExpandedTracklistRowColumns = []TracklistColumn{
			{Name: ColumnNum, Col: ListColumn{Text: "#", Alignment: fyne.TextAlignTrailing, CanToggleVisible: false}},
			{Name: ColumnTitleArtist, Col: ListColumn{Text: title + " / " + artist, Alignment: fyne.TextAlignLeading, CanToggleVisible: false}},
			{Name: ColumnAlbum, Col: ListColumn{Text: album, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnAlbumArtist, Col: ListColumn{Text: albumArtist, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnComposer, Col: ListColumn{Text: composer, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnGenre, Col: ListColumn{Text: genre, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnTime, Col: ListColumn{Text: time, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnYear, Col: ListColumn{Text: year, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnFavorite, Col: ListColumn{Text: fav, Alignment: fyne.TextAlignCenter, CanToggleVisible: true}},
			{Name: ColumnRating, Col: ListColumn{Text: rating, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnPlays, Col: ListColumn{Text: plays, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnLastPlayed, Col: ListColumn{Text: lastPlayed, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnComment, Col: ListColumn{Text: comment, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnBPM, Col: ListColumn{Text: bpm, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnBitrate, Col: ListColumn{Text: bitrate, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnSize, Col: ListColumn{Text: size, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnFileType, Col: ListColumn{Text: fileType, Alignment: fyne.TextAlignTrailing, CanToggleVisible: true}},
			{Name: ColumnDateAdded, Col: ListColumn{Text: dateAdded, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
			{Name: ColumnPath, Col: ListColumn{Text: filepath, Alignment: fyne.TextAlignLeading, CanToggleVisible: true}},
		}

		var sortIconWidth float32 = 15

		numColWidth := widget.NewLabel("9999").MinSize().Width
		timeColWidth := fyne.Max(
			widget.NewLabel(time).MinSize().Width+sortIconWidth,
			widget.NewLabel("99:99").MinSize().Width)
		yearColWidth := fyne.Max(
			widget.NewLabel(year).MinSize().Width+sortIconWidth,
			widget.NewLabel("2000").MinSize().Width)
		favColWidth := fyne.Max(55,
			widget.NewLabel(fav).MinSize().Width+sortIconWidth)
		ratingColWidth := fyne.Max(100,
			widget.NewLabel(rating).MinSize().Width+sortIconWidth)
		playsColWidth := fyne.Max(
			widget.NewLabel("9999").MinSize().Width,
			widget.NewLabel(plays).MinSize().Width+sortIconWidth)
		lastPlayedColWidth := fyne.Max(
			widget.NewLabel(lang.LocalizePluralKey("x_minutes_ago", "59 minutes ago", 59, map[string]string{"minutes": "59"})).MinSize().Width,
			widget.NewLabel(lastPlayed).MinSize().Width+sortIconWidth,
		)
		dateAddedColWidth := fyne.Max(
			widget.NewLabel("2006 Jan 30").MinSize().Width,
			widget.NewLabel(dateAdded).MinSize().Width+sortIconWidth,
		)
		bpmColWidth := fyne.Max(
			widget.NewLabel(bpm+"   ").MinSize().Width,
			widget.NewLabel("9999").MinSize().Width)
		bitrateColWidth := fyne.Max(
			widget.NewLabel("1000").MinSize().Width,
			widget.NewLabel(bitrate).MinSize().Width+sortIconWidth)
		sizeColWidth := fyne.Max(
			widget.NewLabel("99.9 MB").MinSize().Width,
			widget.NewLabel(size).MinSize().Width+sortIconWidth)
		fileTypeColWidth := fyne.Max(
			widget.NewLabel("FLAC").MinSize().Width,
			widget.NewLabel(fileType).MinSize().Width+sortIconWidth)

		// #, Title, Artist, Album, AlbumArtist, Composer, Genre, Time, Year, Favorite, Rating, Plays, LastPlayed, Comment, BPM, Bitrate, Size, FileType, DateAdded, Path
		CompactTracklistRowColumnWidths = []float32{numColWidth, -1, -1, -1, -1, -1, -1, timeColWidth, yearColWidth, favColWidth, ratingColWidth, playsColWidth, lastPlayedColWidth, -1, bpmColWidth, bitrateColWidth, sizeColWidth, fileTypeColWidth, dateAddedColWidth, -1}
		// #, Title/Artist, Album, AlbumArtist, Composer, Genre, Time, Year, Favorite, Rating, Plays, LastPlayed, Comment, BPM, Bitrate, Size, FileType, DateAdded, Path
		ExpandedTracklistRowColumnWidths = []float32{numColWidth, -1, -1, -1, -1, -1, timeColWidth, yearColWidth, favColWidth, ratingColWidth, playsColWidth, lastPlayedColWidth, -1, bpmColWidth, bitrateColWidth, sizeColWidth, fileTypeColWidth, dateAddedColWidth, -1}
	})
)

type tracklistRowBase struct {
	FocusListRowBase

	OnTappedSecondary func(e *fyne.PointEvent, trackIdx int)

	// set by extending widget
	playingIcon fyne.CanvasObject

	// when replacing the content of col 0 with playing icon,
	// the original content is saved here for resetting
	originalNumColContent fyne.CanvasObject

	// internal state
	tracklist  *Tracklist
	trackNum   int
	trackID    string
	coverID    string
	isPlaying  bool
	isFavorite bool
	playCount  int

	nextUpdateModel  *util.TrackListModel
	nextUpdateRowNum int

	num         *widget.Label
	name        *ttwidget.RichText
	artist      *MultiHyperlink
	album       *MultiHyperlink // for disabled support, if albumID is ""
	albumArtist *MultiHyperlink
	composer    *MultiHyperlink
	genre       *MultiHyperlink
	dur         *widget.Label
	year        *widget.Label
	favorite    *fyne.Container
	rating      *StarRating
	bitrate     *widget.Label
	plays       *widget.Label
	lastPlayed  *widget.Label
	comment     *ttwidget.Label
	bpm         *widget.Label
	size        *widget.Label
	fileType    *widget.Label
	dateAdded   *widget.Label
	path        *ttwidget.Label

	// must be injected by extending widget
	setColVisibility func(int, bool) bool
	layout           func(fyne.Size)
}

type TracklistRow interface {
	fyne.CanvasObject
	FocusListRow

	SetOnTappedSecondary(func(_ *fyne.PointEvent, trackNum int))

	TrackID() string
	Update(model *util.TrackListModel, rowNum int, onDone func())
}

type ExpandedTracklistRow struct {
	tracklistRowBase

	img         *ImagePlaceholder
	imageLoader util.ThumbnailLoader
}

type CompactTracklistRow struct {
	tracklistRowBase
}

var (
	_ TracklistRow = (*CompactTracklistRow)(nil)
	_ TracklistRow = (*ExpandedTracklistRow)(nil)
)

func NewExpandedTracklistRow(tracklist *Tracklist, im *backend.ImageManager, playingIcon fyne.CanvasObject) *ExpandedTracklistRow {
	t := &ExpandedTracklistRow{}
	t.ExtendBaseWidget(t)
	t.tracklistRowBase.create(tracklist)
	t.playingIcon = playingIcon
	t.img = NewImagePlaceholder(myTheme.TracksIcon, tracklistThumbnailSize)
	t.img.ScaleMode = canvas.ImageScaleFastest

	t.imageLoader = util.NewThumbnailLoader(im, func(i image.Image) {
		t.img.SetImage(i, false)
	})
	t.imageLoader.OnBeforeLoad = func() {
		t.img.SetImage(nil, false)
	}

	titleArtistImg := container.NewBorder(nil, nil,
		container.New(layout.NewCustomPaddedLayout(2, 2, 2, -4), t.img) /*left*/, nil,
		container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-16),
			t.name, t.artist))

	v := makeVerticallyCentered // func alias
	container := container.New(tracklist.colLayout,
		v(t.num), titleArtistImg, v(t.album), v(t.albumArtist), v(t.composer), v(t.genre), v(t.dur), v(t.year), v(t.favorite), v(t.rating), v(t.plays), v(t.lastPlayed), v(t.comment), v(t.bpm), v(t.bitrate), v(t.size), v(t.fileType), v(t.dateAdded), v(t.path))
	t.Content = container
	t.setColVisibility = func(colNum int, vis bool) bool {
		c := container.Objects[colNum].(*fyne.Container)
		wasHidden := c.Hidden
		c.Hidden = vis
		return c.Hidden != wasHidden
	}
	t.layout = func(s fyne.Size) {
		container.Layout.Layout(container.Objects, s)
	}
	return t
}

func (t *ExpandedTracklistRow) Update(tm *util.TrackListModel, rowNum int, _ func()) {
	if t.trackID != tm.Track().ID && t.img.HaveImage() {
		t.img.SetImage(nil, false)
	}
	t.tracklistRowBase.Update(tm, rowNum, func() {
		t.imageLoader.Load(t.coverID)
	})
}

func NewCompactTracklistRow(tracklist *Tracklist, playingIcon fyne.CanvasObject) *CompactTracklistRow {
	t := &CompactTracklistRow{}
	t.ExtendBaseWidget(t)

	t.tracklistRowBase.create(tracklist)
	t.playingIcon = playingIcon

	container := container.New(tracklist.colLayout,
		t.num, t.name, t.artist, t.album, t.albumArtist, t.composer, t.genre, t.dur, t.year, t.favorite, t.rating, t.plays, t.lastPlayed, t.comment, t.bpm, t.bitrate, t.size, t.fileType, t.dateAdded, t.path)
	t.Content = container

	colHiddenPtrMap := map[int]*bool{
		2:  &t.artist.Hidden,
		3:  &t.album.Hidden,
		4:  &t.albumArtist.Hidden,
		5:  &t.composer.Hidden,
		6:  &t.genre.Hidden,
		7:  &t.dur.Hidden,
		8:  &t.year.Hidden,
		9:  &t.favorite.Hidden,
		10: &t.rating.Hidden,
		11: &t.plays.Hidden,
		12: &t.lastPlayed.Hidden,
		13: &t.comment.Hidden,
		14: &t.bpm.Hidden,
		15: &t.bitrate.Hidden,
		16: &t.size.Hidden,
		17: &t.fileType.Hidden,
		18: &t.dateAdded.Hidden,
		19: &t.path.Hidden,
	}
	t.setColVisibility = func(colNum int, vis bool) bool {
		ptr, ok := colHiddenPtrMap[colNum]
		if !ok {
			return false // column is always visible
		}
		wasHidden := *ptr
		*ptr = vis
		return vis != wasHidden
	}
	t.layout = func(s fyne.Size) {
		container.Layout.Layout(container.Objects, s)
	}
	return t
}

func (t *tracklistRowBase) create(tracklist *Tracklist) {
	t.tracklist = tracklist
	t.num = util.NewTrailingAlignLabel()
	t.name = util.NewTruncatingTooltipRichText()
	t.name.OnMouseIn = t.MouseIn
	t.name.OnMouseOut = t.MouseOut
	t.artist = NewMultiHyperlink()
	t.artist.OnTapped = tracklist.onArtistTapped
	t.artist.OnMouseIn = t.MouseIn
	t.artist.OnMouseOut = t.MouseOut
	t.album = NewMultiHyperlink()
	t.album.OnTapped = func(id string) { tracklist.onAlbumTapped(id) }
	t.album.OnMouseIn = t.MouseIn
	t.album.OnMouseOut = t.MouseOut
	t.albumArtist = NewMultiHyperlink()
	t.albumArtist.OnTapped = tracklist.onArtistTapped
	t.albumArtist.OnMouseIn = t.MouseIn
	t.albumArtist.OnMouseOut = t.MouseOut
	t.composer = NewMultiHyperlink()
	t.composer.OnTapped = func(id string) { tracklist.onArtistTapped(id) }
	t.composer.OnMouseIn = t.MouseIn
	t.composer.OnMouseOut = t.MouseOut
	t.genre = NewMultiHyperlink()
	t.genre.OnTapped = func(id string) { tracklist.onGenreTapped(id) }
	t.genre.OnMouseIn = t.MouseIn
	t.genre.OnMouseOut = t.MouseOut
	t.dur = util.NewTrailingAlignLabel()
	t.year = util.NewTrailingAlignLabel()
	favorite := NewFavoriteIcon()
	favorite.OnTapped = t.toggleFavorited
	t.favorite = container.NewCenter(favorite)
	t.rating = NewStarRating()
	t.rating.IsDisabled = t.tracklist.Options.DisableRating
	t.rating.StarSize = 16
	t.rating.OnRatingChanged = t.setTrackRating
	t.plays = util.NewTrailingAlignLabel()
	t.lastPlayed = util.NewTruncatingLabel()
	t.comment = util.NewTruncatingTooltipLabel()
	t.comment.OnMouseIn = t.MouseIn
	t.comment.OnMouseOut = t.MouseOut
	t.bpm = util.NewTrailingAlignLabel()
	t.bitrate = util.NewTrailingAlignLabel()
	t.size = util.NewTrailingAlignLabel()
	t.fileType = util.NewTrailingAlignLabel()
	t.dateAdded = util.NewTruncatingLabel()
	t.path = util.NewTruncatingTooltipLabel()
	t.path.OnMouseIn = t.MouseIn
	t.path.OnMouseOut = t.MouseOut
}

func (t *tracklistRowBase) SetOnTappedSecondary(f func(*fyne.PointEvent, int)) {
	t.OnTappedSecondary = f
}

func (t *tracklistRowBase) TrackID() string {
	return t.trackID
}

func (t *tracklistRowBase) MinSize() fyne.Size {
	// because tracklist rows have variable column visibility,
	// we return a zero width here so that the tracklist
	// can size based on the header instead.
	s := t.BaseWidget.MinSize()
	return fyne.NewSize(0, s.Height)
}

func (t *tracklistRowBase) Update(tm *util.TrackListModel, rowNum int, onUpdate func()) {
	// Show only columns configured to be visible
	for i := 2; i < len(t.tracklist.columns); i++ {
		t.setColVisibility(i, !t.tracklist.visibleColumns[i])
	}
	t.layout(t.Size())

	// Ealy return if nothing else needs updating
	if !t.needsUpdate(tm, rowNum) {
		return
	}

	// Throttle updates when many are happening in a short time
	if tracklistUpdateCounter.NumEventsSince(time.Now().Add(-300*time.Millisecond)) > 10 {
		t.doUpdate(&emptyTrack, 1)
		if t.nextUpdateModel == nil {
			// queue to run later
			go func() {
				<-time.After(10 * time.Millisecond)
				fyne.Do(func() {
					if t.nextUpdateModel != nil {
						t.doUpdate(t.nextUpdateModel, t.nextUpdateRowNum)
						onUpdate()
					}
					t.nextUpdateModel = nil
				})
			}()
		}
		t.nextUpdateModel = tm
		t.nextUpdateRowNum = rowNum
	} else {
		t.nextUpdateModel = nil
		t.doUpdate(tm, rowNum)
		onUpdate()
	}
}

func (t *tracklistRowBase) needsUpdate(tm *util.TrackListModel, rowNum int) bool {
	tr := tm.Track()
	if tm.Selected != t.Selected {
		return true
	}
	if t.trackID != tr.ID || t.trackNum != rowNum {
		return true
	}
	if t.playCount != tr.PlayCount || t.isFavorite != tr.Favorite || t.rating.Rating != tr.Rating {
		return true
	}
	if t.rating.IsDisabled != t.tracklist.Options.DisableRating {
		return true
	}
	if isPlaying := t.tracklist.nowPlayingID == tr.ID; isPlaying != t.isPlaying {
		return true
	}
	return false
}

func (t *tracklistRowBase) doUpdate(tm *util.TrackListModel, rowNum int) {
	fullRefresh := false
	if tm.Selected != t.Selected {
		t.Selected = tm.Selected
		fullRefresh = true
	}

	// Update info that can change if this row is bound to
	// a new track (*mediaprovider.Track)
	tr := tm.Track()
	if id := tr.ID; id != t.trackID {
		t.EnsureUnfocused()
		t.trackID = id
		t.coverID = tr.CoverArtID

		t.name.Segments[0].(*widget.TextSegment).Text = tr.Title
		t.name.SetToolTip(tr.Title)
		t.artist.BuildSegments(tr.ArtistNames, tr.ArtistIDs)
		t.album.BuildSegments([]string{tr.Album}, []string{tr.AlbumID})
		t.albumArtist.BuildSegments(tr.AlbumArtistNames, tr.AlbumArtistIDs)
		t.composer.BuildSegments(tr.ComposerNames, tr.ComposerIDs)
		t.genre.BuildSegments(tr.Genres, tr.Genres)
		t.dur.Text = util.SecondsToMMSS(tr.Duration.Seconds())
		if tr.Year != 0 {
			t.year.Text = strconv.Itoa(tr.Year)
		} else {
			t.year.Text = ""
		}
		t.plays.Text = strconv.Itoa(int(tr.PlayCount))
		t.lastPlayed.Text = util.LastPlayedDisplayString(tr.LastPlayed)
		t.comment.Text = strings.ReplaceAll(tr.Comment, "\n", " ")
		t.comment.SetToolTip(tr.Comment)
		t.bpm.Text = strconv.Itoa(tr.BPM)
		t.bitrate.Text = strconv.Itoa(tr.BitRate)
		t.size.Text = util.BytesToSizeString(tr.Size)
		t.fileType.Text = tr.Extension
		t.dateAdded.Text = util.FormatDate(tr.DateAdded)
		t.path.Text = tr.FilePath
		t.path.SetToolTip(tr.FilePath)
		fullRefresh = true
	}

	// Update track num if needed
	// (which can change based on bound *mediaprovider.Track or tracklist.AutoNumber)
	if t.trackNum != rowNum {
		discNum := -1
		var str string
		if rowNum < 0 {
			rowNum = tr.TrackNumber
			if t.tracklist.Options.ShowDiscNumber {
				discNum = tr.DiscNumber
			}
		}
		t.trackNum = rowNum
		if discNum >= 0 {
			str = fmt.Sprintf("%d.%02d", discNum, rowNum)
		} else {
			str = strconv.Itoa(rowNum)
		}
		t.num.SetText(str)
	}

	// Update play count if needed
	if tr.PlayCount != t.playCount {
		t.playCount = tr.PlayCount
		t.plays.SetText(strconv.Itoa(int(tr.PlayCount)))
	}

	// Render whether track is playing or not
	if isPlaying := t.tracklist.nowPlayingID == tr.ID; isPlaying != t.isPlaying {
		t.isPlaying = isPlaying
		t.name.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = isPlaying
		t.name.Refresh()

		if isPlaying {
			t.originalNumColContent = t.Content.(*fyne.Container).Objects[0]
			t.Content.(*fyne.Container).Objects[0] = t.playingIcon
		} else {
			t.Content.(*fyne.Container).Objects[0] = t.originalNumColContent
		}
		canvas.Refresh(t)
	}

	// Update favorite column
	if tr.Favorite != t.isFavorite {
		t.isFavorite = tr.Favorite
		t.favorite.Objects[0].(*FavoriteIcon).Favorite = tr.Favorite
		canvas.Refresh(t.favorite)
	}

	// Update rating column
	if t.rating.Rating != tr.Rating {
		t.rating.Rating = tr.Rating
		t.rating.Refresh()
	}
	if t.rating.IsDisabled != t.tracklist.Options.DisableRating {
		t.rating.IsDisabled = t.tracklist.Options.DisableRating
		t.rating.Refresh()
	}

	if fullRefresh {
		tracklistUpdateCounter.Add()
		t.Refresh()
	}
}

func (t *tracklistRowBase) toggleFavorited() {
	t.isFavorite = !t.isFavorite
	favIcon := t.favorite.Objects[0].(*FavoriteIcon)
	favIcon.Favorite = t.isFavorite
	t.favorite.Refresh()
	t.tracklist.onSetFavorite(t.trackID, t.isFavorite)
}

func (t *tracklistRowBase) setTrackRating(rating int) {
	t.tracklist.onSetRating(t.trackID, rating)
}

func (t *tracklistRowBase) TappedSecondary(e *fyne.PointEvent) {
	if t.OnTappedSecondary != nil {
		t.OnTappedSecondary(e, t.ListItemID)
	}
}

func makeVerticallyCentered(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), obj, layout.NewSpacer())
}
