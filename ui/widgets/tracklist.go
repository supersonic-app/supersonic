package widgets

import (
	"log"
	"slices"
	"sort"
	"strings"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/layouts"
	"github.com/supersonic-app/supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type TracklistSort struct {
	SortOrder  SortType
	ColumnName string
}

type TracklistOptions struct {
	// AutoNumber sets whether to auto-number the tracks 1..N in display order,
	// or to use the number from the track's metadata
	AutoNumber bool

	// Reorderable sets whether the tracklist supports drag-and-drop reordering.
	Reorderable bool

	// ShowDiscNumber sets whether to display the disc number as part of the '#' column,
	// (with format %d.%02d). Only applies if AutoNumber==false.
	ShowDiscNumber bool

	// AuxiliaryMenuItems sets additional menu items appended to the context menu
	// must be set before the context menu is shown for the first time
	AuxiliaryMenuItems []*fyne.MenuItem

	// Disables sorting the tracklist by clicking individual columns.
	DisableSorting bool

	// Disables the five star rating widget.
	DisableRating bool

	// Disables the sharing option.
	DisableSharing bool
}

type Tracklist struct {
	widget.BaseWidget

	compactRows bool
	columns     []TracklistColumn

	Options TracklistOptions

	// user action callbacks
	OnPlayTrackAt       func(int)
	OnPlaySelection     func(tracks []*mediaprovider.Track, shuffle bool)
	OnPlaySelectionNext func(trackIDs []*mediaprovider.Track)
	OnAddToQueue        func(trackIDs []*mediaprovider.Track)
	OnAddToPlaylist     func(trackIDs []string)
	OnSetFavorite       func(trackIDs []string, fav bool)
	OnSetRating         func(trackIDs []string, rating int)
	OnDownload          func(tracks []*mediaprovider.Track, downloadName string)
	OnShare             func(trackID string)
	OnPlaySongRadio     func(track *mediaprovider.Track)
	OnReorderTracks     func(trackIDs []string, insertPos int)
	OnShowTrackInfo     func(track *mediaprovider.Track)

	OnShowArtistPage func(artistID string)
	OnShowAlbumPage  func(albumID string)
	OnShowGenrePage  func(genre string)

	OnColumnVisibilityMenuShown func(*widget.PopUp)
	OnVisibleColumnsChanged     func([]string)
	OnTrackShown                func(tracknum int)

	visibleColumns []bool
	sorting        TracklistSort

	tracks          []*util.TrackListModel
	tracksOrigOrder []*util.TrackListModel

	nowPlayingID string
	colLayout    *layouts.ColumnsLayout
	hdr          *ListHeader
	list         *FocusList
	ctxMenu      *util.TrackContextMenu
	loadingDots  *LoadingDots
	container    *fyne.Container
}

func NewTracklist(tracks []*mediaprovider.Track, im *backend.ImageManager, useCompactRows bool) *Tracklist {
	initTracklistColumns()
	playIcon := theme.NewThemedResource(theme.MediaPlayIcon())
	playIcon.ColorName = theme.ColorNamePrimary

	t := &Tracklist{compactRows: useCompactRows}
	t.ExtendBaseWidget(t)
	t.columns = ExpandedTracklistRowColumns
	colWidths := ExpandedTracklistRowColumnWidths
	var playingIcon fyne.CanvasObject
	if useCompactRows {
		t.columns = CompactTracklistRowColumns
		colWidths = CompactTracklistRowColumnWidths
		playIconImg := canvas.NewImageFromResource(playIcon)
		playIconImg.FillMode = canvas.ImageFillContain
		playIconImg.SetMinSize(fyne.NewSquareSize(theme.IconInlineSize() * 1.2))
		playingIcon = container.NewCenter(container.NewHBox(util.NewHSpace(4), playIconImg))
	} else {
		playIconImg := canvas.NewImageFromResource(playIcon)
		playIconImg.FillMode = canvas.ImageFillContain
		playIconImg.SetMinSize(fyne.NewSquareSize(theme.IconInlineSize() * 1.5))
		playingIcon = container.NewCenter(playIconImg)
	}
	t.visibleColumns = make([]bool, len(t.columns))

	if len(tracks) > 0 {
		t._setTracks(tracks)
	}

	t.colLayout = layouts.NewColumnsLayout(colWidths)
	t.hdr = NewListHeader(sharedutil.MapSlice(t.columns,
		func(t TracklistColumn) ListColumn { return t.Col }), t.colLayout)
	t.hdr.OnColumnSortChanged = t.onSorted
	t.hdr.OnColumnVisibilityChanged = t.setColumnVisible
	t.hdr.OnColumnVisibilityMenuShown = func(pop *widget.PopUp) {
		if t.OnColumnVisibilityMenuShown != nil {
			t.OnColumnVisibilityMenuShown(pop)
		}
	}

	t.list = NewFocusList(
		func() int { return len(t.tracks) },
		func() fyne.CanvasObject {
			var tr TracklistRow
			if t.compactRows {
				tr = NewCompactTracklistRow(t, playingIcon)
			} else {
				tr = NewExpandedTracklistRow(t, im, playingIcon)
			}
			tr.SetOnTapped(func() {
				t.onSelectTrack(tr.ItemID())
			})
			tr.SetOnTappedSecondary(t.onShowContextMenu)
			tr.SetOnDoubleTapped(func() {
				t.onPlayTrackAt(tr.ItemID())
			})
			tr.SetOnFocusNeighbor(func(up bool) {
				t.list.FocusNeighbor(tr.ItemID(), up)
			})
			return tr
		},
		func(itemID widget.ListItemID, item fyne.CanvasObject) {
			// we could have removed tracks from the list in between
			// Fyne calling the length callback and this update callback
			// so the itemID may be out of bounds. if so, do nothing.
			if itemID >= len(t.tracks) {
				return
			}
			model := t.tracks[itemID]

			tr := item.(TracklistRow)
			if tr.TrackID() != model.Item.Metadata().ID || tr.ItemID() != itemID {
				tr.SetItemID(itemID)
			}
			i := -1 // signal that we want to display the actual track num.
			if t.Options.AutoNumber {
				i = itemID + 1
			}
			tr.Update(model, i, func() {})
			if t.OnTrackShown != nil {
				t.OnTrackShown(itemID)
			}
		})
	t.list.OnDragBegin = func(id int) {
		if !t.tracks[id].Selected {
			t.selectTrack(id)
			t.list.Refresh()
		}
	}
	t.list.OnDragEnd = func(dragged, insertPos int) {
		if t.OnReorderTracks != nil {
			t.OnReorderTracks(t.SelectedTrackIDs(), insertPos)
		}
	}
	t.loadingDots = NewLoadingDots()
	t.container = container.NewBorder(t.hdr, nil, nil, nil,
		container.NewStack(t.list, container.NewCenter(t.loadingDots)))
	return t
}

func (t *Tracklist) Reset() {
	t.SetLoading(false)
	t.Clear()
	t.Options = TracklistOptions{}
	t.ctxMenu = nil
	t.SetSorting(TracklistSort{})
}

func (t *Tracklist) SetLoading(loading bool) {
	if loading {
		t.loadingDots.Start()
	} else {
		t.loadingDots.Stop()
	}
}

func (t *Tracklist) SetHeaderTransparent(transparent bool) {
	if t.hdr != nil {
		t.hdr.SetTransparentBackground(transparent)
	}
}

func (t *Tracklist) ScrollBy(amount float32) {
	t.list.ScrollToOffset(t.list.GetScrollOffset() + amount)
}

func (t *Tracklist) GetScrollOffset() float32 {
	return t.list.GetScrollOffset()
}

func (t *Tracklist) ScrollToOffset(offset float32) {
	t.list.ScrollToOffset(offset)
}

// Gets the track at the given index.
func (t *Tracklist) TrackAt(idx int) *mediaprovider.Track {
	if idx >= len(t.tracks) {
		log.Println("error: Tracklist.TrackAt: index out of range")
		return nil
	}
	return t.tracks[idx].Track()
}

func (t *Tracklist) SetVisibleColumns(cols []string) {
	t.visibleColumns[0] = true
	t.visibleColumns[1] = true

	for i := 2; i < len(t.columns); i++ {
		t.visibleColumns[i] = false
		t.hdr.SetColumnVisible(i, false)
	}
	for _, col := range cols {
		if num := t.ColNumber(col); num < 0 {
			log.Printf("Unknown tracklist column %q", col)
		} else {
			t.visibleColumns[num] = true
			t.hdr.SetColumnVisible(num, true)
		}
	}
}

func (t *Tracklist) VisibleColumns() []string {
	var cols []string
	for i := 2; i < len(t.visibleColumns); i++ {
		if t.visibleColumns[i] {
			cols = append(cols, string(t.colName(i)))
		}
	}
	return cols
}

func (t *Tracklist) setColumnVisible(colNum int, vis bool) {
	if colNum >= len(t.visibleColumns) {
		log.Printf("error: Tracklist.SetColumnVisible: column index %d out of range", colNum)
		return
	}
	t.visibleColumns[colNum] = vis
	t.list.Refresh()
	if t.OnVisibleColumnsChanged != nil {
		t.OnVisibleColumnsChanged(t.VisibleColumns())
	}
}

func (t *Tracklist) Sorting() TracklistSort {
	return t.sorting
}

func (t *Tracklist) SetSorting(sorting TracklistSort) {
	if sorting.ColumnName == "" {
		// nil case - reset current sort
		if slices.ContainsFunc(t.columns, func(c TracklistColumn) bool {
			return c.Name == t.sorting.ColumnName
		}) {
			t.hdr.SetSorting(ListHeaderSort{ColNumber: t.ColNumber(t.sorting.ColumnName), Type: SortNone})
		}
		return
	}
	// actual sorting will be handled in callback from header
	t.hdr.SetSorting(ListHeaderSort{ColNumber: t.ColNumber(sorting.ColumnName), Type: sorting.SortOrder})
}

// Sets the currently playing track ID and updates the list rendering
func (t *Tracklist) SetNowPlaying(trackID string) {
	prevNowPlaying := t.nowPlayingID
	trPrev, idxPrev := util.FindItemByID(t.tracks, prevNowPlaying)
	tr, idx := util.FindItemByID(t.tracks, trackID)
	t.nowPlayingID = trackID
	if trPrev != nil {
		t.list.RefreshItem(idxPrev)
	}
	if tr != nil {
		t.list.RefreshItem(idx)
	}
}

// Increments the play count of the given track and updates the list rendering
func (t *Tracklist) IncrementPlayCount(trackID string) {
	tr, idx := util.FindItemByID(t.tracks, trackID)
	if tr != nil {
		tr.(*mediaprovider.Track).PlayCount += 1
		t.list.RefreshItem(idx)
	}
}

// Remove all tracks from the tracklist. Does not issue Refresh call.
func (t *Tracklist) Clear() {
	t.tracks = nil
	t.tracksOrigOrder = nil
}

// Sets the tracks in the tracklist.
func (t *Tracklist) SetTracks(trs []*mediaprovider.Track) {
	t._setTracks(trs)
	t.Refresh()
}

func (t *Tracklist) _setTracks(trs []*mediaprovider.Track) {
	t.tracksOrigOrder = util.ToTrackListModels(trs)
	t.doSortTracks()
}

// Returns the tracks in the tracklist in the current display order.
func (t *Tracklist) GetTracks() []*mediaprovider.Track {
	return sharedutil.MapSlice(t.tracks, func(tm *util.TrackListModel) *mediaprovider.Track {
		return tm.Track()
	})
}

// Append more tracks to the tracklist.
func (t *Tracklist) AppendTracks(trs []*mediaprovider.Track) {
	t.tracksOrigOrder = append(t.tracksOrigOrder, util.ToTrackListModels(trs)...)
	t.doSortTracks()
	t.list.Refresh()
}

func (t *Tracklist) SelectAll() {
	util.SelectAllItems(t.tracks)
	t.list.Refresh()
}

func (t *Tracklist) UnselectAll() {
	t.unselectAll()
	t.list.Refresh()
}

func (t *Tracklist) unselectAll() {
	util.UnselectAllItems(t.tracks)
}

func (t *Tracklist) SelectAndScrollToTrack(trackID string) {
	idx := -1
	for i, tr := range t.tracks {
		if tr.Item.Metadata().ID == trackID {
			idx = i
			tr.Selected = true
		} else {
			tr.Selected = false
		}
	}
	if idx >= 0 {
		t.list.ScrollTo(idx)
	}
}

func (t *Tracklist) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}

func (t *Tracklist) Refresh() {
	t.list.EnableDragging = t.Options.Reorderable
	t.hdr.DisableSorting = t.Options.DisableSorting
	t.BaseWidget.Refresh()
}

// do nothing Tapped handler so that tapping the separator between rows
// doesn't fall through to the page (which calls UnselectAll on tracklist)
func (t *Tracklist) Tapped(*fyne.PointEvent) {}

func (t *Tracklist) stringSort(fieldFn func(*util.TrackListModel) string) {
	new := make([]*util.TrackListModel, len(t.tracksOrigOrder))
	copy(new, t.tracksOrigOrder)
	sort.SliceStable(new, func(i, j int) bool {
		cmp := strings.Compare(fieldFn(new[i]), fieldFn(new[j]))
		if t.sorting.SortOrder == SortDescending {
			return cmp > 0
		}
		return cmp < 0
	})
	t.tracks = new
}

func (t *Tracklist) intSort(fieldFn func(*util.TrackListModel) int64) {
	new := make([]*util.TrackListModel, len(t.tracksOrigOrder))
	copy(new, t.tracksOrigOrder)
	sort.SliceStable(new, func(i, j int) bool {
		if t.sorting.SortOrder == SortDescending {
			return fieldFn(new[i]) > fieldFn(new[j])
		}
		return fieldFn(new[i]) < fieldFn(new[j])
	})
	t.tracks = new
}

func (t *Tracklist) doSortTracks() {
	if t.sorting.SortOrder == SortNone {
		t.tracks = t.tracksOrigOrder
		return
	}
	switch t.sorting.ColumnName {
	case ColumnNum:
		if t.sorting.SortOrder == SortDescending {
			t.tracks = sharedutil.Reversed(t.tracksOrigOrder)
		} else {
			t.tracks = t.tracksOrigOrder
		}
	case ColumnTitle, ColumnTitleArtist:
		t.stringSort(func(tr *util.TrackListModel) string { return tr.Track().Title })
	case ColumnArtist:
		t.stringSort(func(tr *util.TrackListModel) string { return strings.Join(tr.Track().ArtistNames, ", ") })
	case ColumnAlbumArtist:
		t.stringSort(func(tr *util.TrackListModel) string { return strings.Join(tr.Track().AlbumArtistNames, ", ") })
	case ColumnAlbum:
		t.stringSort(func(tr *util.TrackListModel) string { return tr.Track().Album })
	case ColumnGenre:
		t.stringSort(func(tr *util.TrackListModel) string { return strings.Join(tr.Track().Genres, ", ") })
	case ColumnPath:
		t.stringSort(func(tr *util.TrackListModel) string { return tr.Track().FilePath })
	case ColumnRating:
		t.intSort(func(tr *util.TrackListModel) int64 { return int64(tr.Track().Rating) })
	case ColumnTime:
		t.intSort(func(tr *util.TrackListModel) int64 { return tr.Track().Duration.Milliseconds() })
	case ColumnYear:
		t.intSort(func(tr *util.TrackListModel) int64 { return int64(tr.Track().Year) })
	case ColumnSize:
		t.intSort(func(tr *util.TrackListModel) int64 { return tr.Track().Size })
	case ColumnPlays:
		t.intSort(func(tr *util.TrackListModel) int64 { return int64(tr.Track().PlayCount) })
	case ColumnLastPlayed:
		t.intSort(func(tr *util.TrackListModel) int64 { return tr.Track().LastPlayed.Unix() })
	case ColumnComment:
		t.stringSort(func(tr *util.TrackListModel) string { return tr.Track().Comment })
	case ColumnFileType:
		t.stringSort(func(tr *util.TrackListModel) string { return tr.Track().Extension })
	case ColumnBPM:
		t.intSort(func(tr *util.TrackListModel) int64 { return int64(tr.Track().BPM) })
	case ColumnBitrate:
		t.intSort(func(tr *util.TrackListModel) int64 { return int64(tr.Track().BitRate) })
	case ColumnFavorite:
		t.intSort(func(tr *util.TrackListModel) int64 {
			if tr.Track().Favorite {
				return 1
			}
			return 0
		})
	case ColumnDateAdded:
		t.intSort(func(tr *util.TrackListModel) int64 { return tr.Track().DateAdded.Unix() })
	}
}

func (t *Tracklist) onSorted(sort ListHeaderSort) {
	t.sorting = TracklistSort{ColumnName: t.colName(sort.ColNumber), SortOrder: sort.Type}
	t.doSortTracks()
	t.Refresh()
}

func (t *Tracklist) onPlayTrackAt(idx int) {
	if t.OnPlayTrackAt != nil {
		t.OnPlayTrackAt(idx)
	}
}

func (t *Tracklist) onSelectTrack(idx int) {
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		mod := d.CurrentKeyModifiers()
		if mod&fyne.KeyModifierShortcutDefault != 0 {
			t.selectAddOrRemove(idx)
		} else if mod&fyne.KeyModifierShift != 0 {
			t.selectRange(idx)
		} else {
			t.selectTrack(idx)
		}
	} else {
		t.selectTrack(idx)
	}
	t.list.Refresh()
}

func (t *Tracklist) selectAddOrRemove(idx int) {
	t.tracks[idx].Selected = !t.tracks[idx].Selected
}

func (t *Tracklist) selectTrack(idx int) bool {
	return util.SelectItem(t.tracks, idx)
}

func (t *Tracklist) selectRange(idx int) {
	util.SelectItemRange(t.tracks, idx)
}

func (t *Tracklist) onShowContextMenu(e *fyne.PointEvent, trackIdx int) {
	if t.selectTrack(trackIdx) {
		t.list.Refresh()
	}
	if t.ctxMenu == nil {
		t.ctxMenu = util.NewTrackContextMenu(false, t.Options.AuxiliaryMenuItems)
		t.ctxMenu.OnPlay = func(shuffle bool) {
			t.OnPlaySelection(t.SelectedTracks(), shuffle)
		}
		t.ctxMenu.OnAddToQueue = func(next bool) {
			if next {
				t.OnPlaySelectionNext(t.SelectedTracks())
			} else {
				t.OnAddToQueue(t.SelectedTracks())
			}
		}
		t.ctxMenu.OnPlaySongRadio = func() {
			t.onPlaySongRadio(t.SelectedTracks())
		}
		t.ctxMenu.OnAddToPlaylist = func() {
			t.OnAddToPlaylist(t.SelectedTrackIDs())
		}
		t.ctxMenu.OnDownload = func() {
			t.onDownload(t.SelectedTracks(), "Selected tracks")
		}
		t.ctxMenu.OnShowInfo = func() {
			t.OnShowTrackInfo(t.SelectedTracks()[0])
		}
		t.ctxMenu.OnFavorite = func(fav bool) {
			t.onSetFavorites(t.SelectedTracks(), fav, true /*needRefresh*/)
		}
		t.ctxMenu.OnShare = func() {
			t.onShare(t.SelectedTracks())
		}
		t.ctxMenu.OnSetRating = func(rating int) {
			t.onSetRatings(t.SelectedTracks(), rating, true /*needRefresh*/)
		}
	}
	t.ctxMenu.SetRatingDisabled(t.Options.DisableRating)
	t.ctxMenu.SetShareDisabled(t.Options.DisableSharing || len(t.SelectedTracks()) != 1)
	t.ctxMenu.SetInfoDisabled(len(t.SelectedTracks()) != 1)
	t.ctxMenu.ShowAtPosition(e.AbsolutePosition, fyne.CurrentApp().Driver().CanvasForObject(t))
}

func (t *Tracklist) onSetFavorite(trackID string, fav bool) {
	item, _ := util.FindItemByID(t.tracks, trackID)
	t.onSetFavorites([]*mediaprovider.Track{item.(*mediaprovider.Track)}, fav, false)
}

func (t *Tracklist) onSetFavorites(tracks []*mediaprovider.Track, fav bool, needRefresh bool) {
	for _, tr := range tracks {
		tr.Favorite = fav
	}
	if needRefresh {
		t.Refresh()
	}
	// notify listener
	if t.OnSetFavorite != nil {
		t.OnSetFavorite(sharedutil.TracksToIDs(tracks), fav)
	}
}

func (t *Tracklist) onSetRating(trackID string, rating int) {
	// update our own track model
	item, _ := util.FindItemByID(t.tracks, trackID)
	t.onSetRatings([]*mediaprovider.Track{item.(*mediaprovider.Track)}, rating, false)
}

func (t *Tracklist) onSetRatings(tracks []*mediaprovider.Track, rating int, needRefresh bool) {
	for _, tr := range tracks {
		tr.Rating = rating
	}
	if needRefresh {
		t.Refresh()
	}
	// notify listener
	if t.OnSetRating != nil {
		t.OnSetRating(sharedutil.TracksToIDs(tracks), rating)
	}
}

func (t *Tracklist) onArtistTapped(artistID string) {
	if t.OnShowArtistPage != nil {
		t.OnShowArtistPage(artistID)
	}
}

func (t *Tracklist) onAlbumTapped(albumID string) {
	if t.OnShowAlbumPage != nil {
		t.OnShowAlbumPage(albumID)
	}
}

func (t *Tracklist) onGenreTapped(genre string) {
	if t.OnShowGenrePage != nil {
		t.OnShowGenrePage(genre)
	}
}

func (t *Tracklist) onDownload(tracks []*mediaprovider.Track, downloadName string) {
	if t.OnDownload != nil {
		t.OnDownload(tracks, downloadName)
	}
}

func (t *Tracklist) onShare(tracks []*mediaprovider.Track) {
	if t.OnShare != nil {
		if len(tracks) > 0 {
			t.OnShare(tracks[0].ID)
		}
	}
}

func (t *Tracklist) onPlaySongRadio(tracks []*mediaprovider.Track) {
	if t.OnPlaySongRadio != nil {
		if len(tracks) > 0 {
			t.OnPlaySongRadio(tracks[0])
		}
	}
}

func (t *Tracklist) SelectedTracks() []*mediaprovider.Track {
	return util.SelectedTracks(t.tracks)
}

func (t *Tracklist) SelectedTrackIDs() []string {
	return util.SelectedItemIDs(t.tracks)
}

// SelectedTrackIndexes returns the indexes of the selected tracks in the
// original sort order (ie if tracklist is sorted by some column), the indexes
// returned will correspond to the order of tracks when the list was initialized.
func (t *Tracklist) SelectedTrackIndexes() []int {
	idx := -1
	return sharedutil.FilterMapSlice(t.tracksOrigOrder, func(t *util.TrackListModel) (int, bool) {
		idx++
		return idx, t.Selected
	})
}

func (t *Tracklist) ColNumber(colName string) int {
	i := slices.IndexFunc(t.columns, func(c TracklistColumn) bool {
		return c.Name == colName
	})
	if i < 0 {
		log.Printf("error: Tracklist: invalid column name %s", colName)
	}
	return i
}

func (t *Tracklist) colName(i int) string {
	if i < len(t.columns) {
		return t.columns[i].Name
	}
	log.Println("notReached: Tracklist.colName")
	return ""
}
