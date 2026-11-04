package widgets

import (
	"image"
	"slices"
	"strconv"
	"sync"

	"fyne.io/fyne/v2/lang"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/layouts"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

const playQueueListThumbnailSize = 52

type PlayQueueListModel struct {
	Item     mediaprovider.MediaItem
	Selected bool
}

type PlayQueueList struct {
	widget.BaseWidget

	Reorderable    bool
	DisableRating  bool
	DisableSharing bool

	// user action callbacks
	OnPlayItemAt        func(idx int)
	OnPlaySelection     func(items []mediaprovider.MediaItem, shuffle bool)
	OnPlaySelectionNext func(items []mediaprovider.MediaItem)
	OnPlaySongRadio     func(track *mediaprovider.Track)
	OnAddToQueue        func(items []mediaprovider.MediaItem)
	OnAddToPlaylist     func(trackIDs []string)
	OnSetFavorite       func(trackIDs []string, fav bool)
	OnSetRating         func(trackIDs []string, rating int)
	OnRemoveFromQueue   func(idxs []int)
	OnDownload          func(tracks []*mediaprovider.Track, downloadName string)
	OnShowTrackInfo     func(track *mediaprovider.Track)
	OnShare             func(tracks []*mediaprovider.Track)
	OnShowArtistPage    func(artistID string)
	OnReorderItems      func(idxs []int, reorderTo int)

	useNonQueueMenu bool
	menu            *util.TrackContextMenu // ctx menu for when only tracks are selected
	radiosMenu      *widget.PopUpMenu      // ctx menu for when selection contains radios

	nowPlayingID string

	list        *FocusList
	colLayout   *layouts.ColumnsLayout
	tracksMutex sync.RWMutex
	items       []*util.TrackListModel // displayed items: allItems[hiddenCount:]

	// state for the hide-played-tracks filter, see SetQueue. guarded by tracksMutex.
	// an index into items plus hiddenCount is an index into the full queue.
	allItems      []*util.TrackListModel
	nowPlayingIdx int
	hidePlayed    bool
	hiddenCount   int
}

func NewPlayQueueList(im *backend.ImageManager, useNonQueueMenu bool) *PlayQueueList {
	p := &PlayQueueList{useNonQueueMenu: useNonQueueMenu}
	p.ExtendBaseWidget(p)

	// #, Cover, Title/Artist, Time
	coverWidth := NewPlayQueueListRow(p, im, layout.NewSpacer()).cover.MinSize().Width
	p.colLayout = layouts.NewColumnsLayout([]float32{40, coverWidth, -1, 60})

	playIconResource := theme.NewThemedResource(theme.MediaPlayIcon())
	playIconResource.ColorName = theme.ColorNamePrimary
	playIconImg := canvas.NewImageFromResource(playIconResource)
	playIconImg.FillMode = canvas.ImageFillContain
	playIconImg.SetMinSize(fyne.NewSquareSize(theme.IconInlineSize() * 1.5))

	playingIcon := container.NewCenter(playIconImg)

	p.list = NewFocusList(
		p.lenTracks,
		func() fyne.CanvasObject {
			return NewPlayQueueListRow(p, im, playingIcon)
		},
		func(itemID widget.ListItemID, item fyne.CanvasObject) {
			p.tracksMutex.RLock()
			// we could have removed tracks from the list in between
			// Fyne calling the length callback and this update callback
			// so the itemID may be out of bounds. if so, do nothing.
			if itemID >= len(p.items) {
				p.tracksMutex.RUnlock()
				return
			}
			model := p.items[itemID]
			// number rows by their position in the full queue
			trackNum := itemID + 1 + p.hiddenCount
			p.tracksMutex.RUnlock()

			tr := item.(*PlayQueueListRow)
			if tr.trackID != model.Item.Metadata().ID || tr.ListItemID != itemID {
				tr.ListItemID = itemID
			}
			tr.Update(model, trackNum)
		},
	)
	p.list.OnDragBegin = func(id int) {
		if !p.items[id].Selected {
			p.selectTrack(id)
			p.list.Refresh()
		}
	}
	p.list.OnDragEnd = func(dragged, insertPos int) {
		if p.OnReorderItems == nil {
			return
		}
		p.tracksMutex.RLock()
		idxs := p.selectedQueueIdxsLocked()
		insertPos += p.hiddenCount
		if p.hidePlayed && p.nowPlayingIdx >= 0 && p.nowPlayingIdx < len(p.allItems) {
			// dropping above the playing track would move the dragged
			// tracks into the hidden, already-played part of the queue
			insertPos = max(insertPos, p.nowPlayingIdx+1)
		}
		p.tracksMutex.RUnlock()
		p.OnReorderItems(idxs, insertPos)
	}

	return p
}

// SetQueue sets the play queue to display, along with the index of the currently
// playing item (-1 if none). If SetHidePlayed(true) was called, the already-played
// items before the playing one are not displayed; indexes reported to the
// OnPlayItemAt, OnRemoveFromQueue and OnReorderItems callbacks are translated
// back into indexes in the full queue.
func (p *PlayQueueList) SetQueue(items []mediaprovider.MediaItem, nowPlayingIdx int) {
	p.tracksMutex.Lock()
	p.allItems = sharedutil.MapSlice(items, func(item mediaprovider.MediaItem) *util.TrackListModel {
		return &util.TrackListModel{Item: item}
	})
	p.nowPlayingIdx = nowPlayingIdx
	p.applyFilterLocked()
	p.tracksMutex.Unlock()
	p.Refresh()
}

// SetNowPlayingIndex updates the index of the currently playing item within the
// queue set by SetQueue.
func (p *PlayQueueList) SetNowPlayingIndex(nowPlayingIdx int) {
	p.tracksMutex.Lock()
	p.nowPlayingIdx = nowPlayingIdx
	p.tracksMutex.Unlock()
	p.refilter()
}

// SetHidePlayed sets whether the already-played items before the playing one are hidden.
func (p *PlayQueueList) SetHidePlayed(hidePlayed bool) {
	p.tracksMutex.Lock()
	p.hidePlayed = hidePlayed
	p.tracksMutex.Unlock()
	p.refilter()
}

func (p *PlayQueueList) refilter() {
	p.tracksMutex.Lock()
	prev := p.hiddenCount
	p.applyFilterLocked()
	shift := p.hiddenCount - prev
	p.tracksMutex.Unlock()
	if shift == 0 {
		return
	}
	// keep the same tracks in view as rows are hidden or revealed above them
	offset := p.list.GetScrollOffset() - float32(shift)*p.rowPitch()
	p.Refresh()
	p.list.ScrollToOffset(offset)
}

// distance between the tops of adjacent rows, as laid out by widget.List
func (p *PlayQueueList) rowPitch() float32 {
	return p.list.CreateItem().MinSize().Height + p.list.Theme().Size(theme.SizeNamePadding)
}

// caller must hold tracksMutex for writing, and must Refresh afterwards.
// re-slices the existing models, so the selection of displayed items is kept.
func (p *PlayQueueList) applyFilterLocked() {
	p.hiddenCount = 0
	if p.hidePlayed && p.nowPlayingIdx > 0 && p.nowPlayingIdx < len(p.allItems) {
		p.hiddenCount = p.nowPlayingIdx
	}
	// hidden items can't be acted on, so they mustn't stay selected
	util.UnselectAllItems(p.allItems[:p.hiddenCount])
	p.items = p.allItems[p.hiddenCount:]
}

func (p *PlayQueueList) SetTracks(trs []*mediaprovider.Track) {
	p.tracksMutex.Lock()
	p.allItems = util.ToTrackListModels(trs)
	p.applyFilterLocked()
	p.tracksMutex.Unlock()
	p.Refresh()
}

// Queue returns the full play queue, including any items hidden by the
// hide-played-tracks filter. The indexes reported to the OnPlayItemAt,
// OnRemoveFromQueue and OnReorderItems callbacks index into this slice, so it -
// not the displayed items - is what a handler for those must work against.
func (p *PlayQueueList) Queue() []mediaprovider.MediaItem {
	p.tracksMutex.RLock()
	defer p.tracksMutex.RUnlock()
	return sharedutil.MapSlice(p.allItems, func(item *util.TrackListModel) mediaprovider.MediaItem {
		return item.Item
	})
}

// Sets the currently playing item ID and updates the list rendering
func (p *PlayQueueList) SetNowPlaying(itemID string) {
	prevNowPlaying := p.nowPlayingID
	p.tracksMutex.RLock()
	trPrev, idxPrev := util.FindItemByID(p.items, prevNowPlaying)
	tr, idx := util.FindItemByID(p.items, itemID)
	p.tracksMutex.RUnlock()
	p.nowPlayingID = itemID
	if trPrev != nil {
		p.list.RefreshItem(idxPrev)
	}
	if tr != nil {
		p.list.RefreshItem(idx)
	}
}

func (p *PlayQueueList) SelectAll() {
	p.tracksMutex.RLock()
	util.SelectAllItems(p.items)
	p.tracksMutex.RUnlock()
	p.list.Refresh()
}

func (p *PlayQueueList) UnselectAll() {
	p.tracksMutex.RLock()
	util.UnselectAllItems(p.items)
	p.tracksMutex.RUnlock()
	p.list.Refresh()
}

func (p *PlayQueueList) Scroll(amount float32) {
	p.list.ScrollToOffset(p.list.GetScrollOffset() + amount)
}

func (p *PlayQueueList) ScrollToOffset(offset float32) {
	p.list.ScrollToOffset(offset)
}

func (p *PlayQueueList) ScrollToNowPlaying() {
	idx := slices.IndexFunc(p.items, func(item *util.TrackListModel) bool {
		return item.Item.Metadata().ID == p.nowPlayingID
	})
	if idx >= 0 {
		p.list.ScrollTo(idx)
	}
}

func (p *PlayQueueList) Refresh() {
	p.list.EnableDragging = p.Reorderable
	p.BaseWidget.Refresh()
}

func (p *PlayQueueList) lenTracks() int {
	p.tracksMutex.RLock()
	defer p.tracksMutex.RUnlock()
	return len(p.items)
}

func (t *PlayQueueList) onArtistTapped(artistID string) {
	if t.OnShowArtistPage != nil {
		t.OnShowArtistPage(artistID)
	}
}

func (p *PlayQueueList) onPlayTrackAt(idx int) {
	if p.OnPlayItemAt != nil {
		p.OnPlayItemAt(idx + p.hiddenItemCount())
	}
}

func (p *PlayQueueList) onSelectTrack(idx int) {
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		mod := d.CurrentKeyModifiers()
		if mod&fyne.KeyModifierShortcutDefault != 0 {
			p.selectAddOrRemove(idx)
		} else if mod&fyne.KeyModifierShift != 0 {
			p.selectRange(idx)
		} else {
			p.selectTrack(idx)
		}
	} else {
		p.selectTrack(idx)
	}
	p.Refresh()
}

func (p *PlayQueueList) selectTrack(idx int) {
	p.tracksMutex.RLock()
	defer p.tracksMutex.RUnlock()
	util.SelectItem(p.items, idx)
}

func (p *PlayQueueList) selectAddOrRemove(idx int) {
	p.tracksMutex.RLock()
	defer p.tracksMutex.RUnlock()
	p.items[idx].Selected = !p.items[idx].Selected
}

func (p *PlayQueueList) selectRange(idx int) {
	p.tracksMutex.RLock()
	defer p.tracksMutex.RUnlock()
	util.SelectItemRange(p.items, idx)
}

func (p *PlayQueueList) onShowContextMenu(e *fyne.PointEvent, trackIdx int) {
	p.selectTrack(trackIdx)
	p.list.Refresh()
	selected := p.selectedItems()

	allTracks := true
	for _, item := range selected {
		if item.Metadata().Type == mediaprovider.MediaItemTypeRadioStation {
			allTracks = false
			break
		}
	}

	if allTracks {
		p.ensureTracksMenu()
		p.menu.SetRatingDisabled(p.DisableRating)
		p.menu.SetInfoDisabled(len(selected) != 1)
		p.menu.SetShareDisabled(p.DisableSharing || len(selected) != 1)
		p.menu.ShowAtPosition(e.AbsolutePosition, fyne.CurrentApp().Driver().CanvasForObject(p))
	} else {
		p.ensureRadiosMenu()
		p.radiosMenu.ShowAtPosition(e.AbsolutePosition)
	}
}

func (p *PlayQueueList) ensureTracksMenu() {
	if p.menu != nil {
		return
	}
	var auxItems []*fyne.MenuItem
	if !p.useNonQueueMenu {
		remove := fyne.NewMenuItem(lang.L("Remove from queue"), func() {
			if p.OnRemoveFromQueue != nil {
				idxs := p.selectedQueueIdxs()
				p.OnRemoveFromQueue(idxs)
			}
		})
		remove.Icon = theme.ContentRemoveIcon()
		auxItems = append(auxItems, remove)
	}
	p.menu = util.NewTrackContextMenu(!p.useNonQueueMenu, auxItems)
	p.menu.OnPlay = func(shuffle bool) {
		p.OnPlaySelection(p.selectedItems(), shuffle)
	}
	p.menu.OnAddToQueue = func(next bool) {
		if next {
			p.OnPlaySelectionNext(p.selectedItems())
		} else {
			p.OnAddToQueue(p.selectedItems())
		}
	}
	p.menu.OnPlaySongRadio = func() {
		p.OnPlaySongRadio(p.selectedTracks()[0])
	}
	p.menu.OnDownload = func() {
		p.OnDownload(p.selectedTracks(), "Selected tracks")
	}
	p.menu.OnAddToPlaylist = func() {
		p.OnAddToPlaylist(p.selectedItemIDs())
	}
	p.menu.OnShowInfo = func() {
		p.OnShowTrackInfo(p.selectedTracks()[0])
	}
	p.menu.OnShare = func() {
		p.OnShare(p.selectedTracks())
	}
	p.menu.OnSetRating = func(rating int) {
		p.OnSetRating(p.selectedItemIDs(), rating)
	}
	p.menu.OnFavorite = func(fav bool) {
		p.OnSetFavorite(p.selectedItemIDs(), fav)
	}
}

func (p *PlayQueueList) ensureRadiosMenu() {
	if p.radiosMenu != nil {
		return
	}
	remove := fyne.NewMenuItem(lang.L("Remove from queue"), func() {
		if p.OnRemoveFromQueue != nil {
			idxs := p.selectedQueueIdxs()
			p.OnRemoveFromQueue(idxs)
		}
	})
	remove.Icon = theme.ContentRemoveIcon()
	p.radiosMenu = widget.NewPopUpMenu(
		fyne.NewMenu("", remove),
		fyne.CurrentApp().Driver().CanvasForObject(p),
	)
}

func (t *PlayQueueList) selectedItems() []mediaprovider.MediaItem {
	t.tracksMutex.RLock()
	defer t.tracksMutex.RUnlock()
	return util.SelectedItems(t.items)
}

func (t *PlayQueueList) selectedTracks() []*mediaprovider.Track {
	t.tracksMutex.RLock()
	defer t.tracksMutex.RUnlock()
	return util.SelectedTracks(t.items)
}

func (t *PlayQueueList) selectedItemIDs() []string {
	t.tracksMutex.RLock()
	defer t.tracksMutex.RUnlock()
	return util.SelectedItemIDs(t.items)
}

// number of leading queue items currently hidden. Any index that crosses into
// the playback engine must have this added to it.
func (t *PlayQueueList) hiddenItemCount() int {
	t.tracksMutex.RLock()
	defer t.tracksMutex.RUnlock()
	return t.hiddenCount
}

// indexes of the selected rows, translated into indexes in the full play queue.
// Must be used for any callback whose indexes are interpreted by the playback engine.
func (t *PlayQueueList) selectedQueueIdxs() []int {
	t.tracksMutex.RLock()
	defer t.tracksMutex.RUnlock()
	return t.selectedQueueIdxsLocked()
}

// caller must hold tracksMutex
func (t *PlayQueueList) selectedQueueIdxsLocked() []int {
	idxs := util.SelectedIndexes(t.items)
	for i := range idxs {
		idxs[i] += t.hiddenCount
	}
	return idxs
}

func (p *PlayQueueList) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.list)
}

type PlayQueueListRow struct {
	FocusListRowBase

	OnTappedSecondary func(e *fyne.PointEvent, trackIdx int)

	imageLoader   util.ThumbnailLoader
	playQueueList *PlayQueueList
	trackID       string
	isPlaying     bool

	playingIcon fyne.CanvasObject
	num         *widget.Label
	cover       *ImagePlaceholder
	title       *ttwidget.Label
	artist      *MultiHyperlink
	time        *widget.Label
}

func NewPlayQueueListRow(playQueueList *PlayQueueList, im *backend.ImageManager, playingIcon fyne.CanvasObject) *PlayQueueListRow {
	p := &PlayQueueListRow{
		playingIcon:   playingIcon,
		playQueueList: playQueueList,
		num:           widget.NewLabel(""),
		cover:         NewImagePlaceholder(myTheme.TracksIcon, playQueueListThumbnailSize),
		title:         util.NewTruncatingTooltipLabel(),
		artist:        NewMultiHyperlink(),
		time:          util.NewTrailingAlignLabel(),
	}
	p.ExtendBaseWidget(p)

	p.cover.ScaleMode = canvas.ImageScaleFastest
	p.title.OnMouseIn = p.MouseIn
	p.title.OnMouseOut = p.MouseOut
	p.artist.OnTapped = playQueueList.onArtistTapped
	p.artist.OnMouseIn = p.MouseIn
	p.artist.OnMouseOut = p.MouseOut
	p.OnDoubleTapped = func() {
		playQueueList.onPlayTrackAt(p.ItemID())
	}
	p.OnTapped = func() {
		playQueueList.onSelectTrack(p.ItemID())
	}
	p.OnTappedSecondary = playQueueList.onShowContextMenu
	p.OnFocusNeighbor = func(up bool) {
		playQueueList.list.FocusNeighbor(p.ItemID(), up)
	}

	p.imageLoader = util.NewThumbnailLoader(im, func(i image.Image) {
		p.cover.SetImage(i, false)
	})
	p.imageLoader.OnBeforeLoad = func() {
		p.cover.SetImage(nil, false)
	}

	p.Content = container.New(playQueueList.colLayout,
		container.NewCenter(p.num),
		container.NewPadded(p.cover),
		container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-15),
			p.title, p.artist),
		container.NewCenter(p.time),
	)
	return p
}

func (p *PlayQueueListRow) TappedSecondary(e *fyne.PointEvent) {
	if p.OnTappedSecondary != nil {
		p.OnTappedSecondary(e, p.ListItemID)
	}
}

func (p *PlayQueueListRow) Update(tm *util.TrackListModel, rowNum int) {
	if tm.Selected != p.Selected {
		p.Selected = tm.Selected
	}

	if num := strconv.Itoa(rowNum); p.num.Text != num {
		p.num.Text = num
	}

	// Update info that can change if this row is bound to
	// a new track (*mediaprovider.Track)
	meta := tm.Item.Metadata()
	if meta.ID != p.trackID {
		if meta.Type == mediaprovider.MediaItemTypeRadioStation {
			p.cover.PlaceholderIcon = myTheme.RadioIcon
		} else {
			p.cover.PlaceholderIcon = myTheme.TracksIcon
		}
		p.imageLoader.Load(meta.CoverArtID)
		p.EnsureUnfocused()
		p.trackID = meta.ID
		p.title.Text = meta.Name
		p.title.SetToolTip(meta.Name)
		p.artist.BuildSegments(meta.Artists, meta.ArtistIDs)
		p.time.Text = util.SecondsToMMSS(meta.Duration.Seconds())
	}

	// Render whether track is playing or not
	if isPlaying := p.playQueueList.nowPlayingID == meta.ID; isPlaying != p.isPlaying {
		p.isPlaying = isPlaying
		p.title.TextStyle.Bold = isPlaying

		if isPlaying {
			p.Content.(*fyne.Container).Objects[0] = p.playingIcon
		} else {
			p.Content.(*fyne.Container).Objects[0] = container.NewCenter(p.num)
		}
	}

	// we always need to refresh in case of light/dark change
	// even if no info changed in the update, since the
	// PlayQueueList is used in the pop up queue which may be
	// hidden and re-shown after a theme variant change
	p.Refresh()
}
