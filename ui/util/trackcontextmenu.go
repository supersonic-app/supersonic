package util

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
)

type TrackContextMenu struct {
	ratingSubmenu     *fyne.MenuItem
	shareMenuItem     *fyne.MenuItem
	songRadioMenuItem *fyne.MenuItem
	infoMenuItem      *fyne.MenuItem

	OnPlay          func(shuffle bool)
	OnAddToQueue    func(next bool)
	OnPlaySongRadio func()
	OnDownload      func()
	OnAddToPlaylist func()
	OnShowInfo      func()
	OnShare         func()
	OnFavorite      func(fav bool)
	OnSetRating     func(rating int)

	menu *fyne.Menu
}

func NewTrackContextMenu(disablePlaybackMenu bool, auxItems []*fyne.MenuItem) *TrackContextMenu {
	tcm := &TrackContextMenu{}

	tcm.menu = fyne.NewMenu("")
	tcm.songRadioMenuItem = fyne.NewMenuItem(lang.L("Play song radio"), func() {
		if tcm.OnPlaySongRadio != nil {
			tcm.OnPlaySongRadio()
		}
	})
	tcm.songRadioMenuItem.Icon = myTheme.RadioIcon
	if !disablePlaybackMenu {
		play := fyne.NewMenuItem(lang.L("Play"), func() {
			if tcm.OnPlay != nil {
				tcm.OnPlay(false)
			}
		})
		play.Icon = theme.MediaPlayIcon()
		shuffle := fyne.NewMenuItem(lang.L("Shuffle"), func() {
			if tcm.OnPlay != nil {
				tcm.OnPlay(true)
			}
		})
		shuffle.Icon = myTheme.ShuffleIcon
		playNext := fyne.NewMenuItem(lang.L("Play next"), func() {
			if tcm.OnAddToQueue != nil {
				tcm.OnAddToQueue(true)
			}
		})
		playNext.Icon = myTheme.PlayNextIcon
		add := fyne.NewMenuItem(lang.L("Add to queue"), func() {
			if tcm.OnAddToQueue != nil {
				tcm.OnAddToQueue(false)
			}
		})
		add.Icon = theme.ContentAddIcon()
		tcm.menu.Items = append(tcm.menu.Items,
			play, shuffle, playNext, add, tcm.songRadioMenuItem)
	}
	playlist := fyne.NewMenuItem(lang.L("Add to playlist")+"...", func() {
		if tcm.OnAddToPlaylist != nil {
			tcm.OnAddToPlaylist()
		}
	})
	playlist.Icon = myTheme.PlaylistIcon
	download := fyne.NewMenuItem(lang.L("Download")+"...", func() {
		if tcm.OnDownload != nil {
			tcm.OnDownload()
		}
	})
	download.Icon = theme.DownloadIcon()
	tcm.infoMenuItem = fyne.NewMenuItem(lang.L("Show info")+"...", func() {
		if tcm.OnShowInfo != nil {
			tcm.OnShowInfo()
		}
	})
	tcm.infoMenuItem.Icon = theme.InfoIcon()
	favorite := fyne.NewMenuItem(lang.L("Set favorite"), func() {
		if tcm.OnFavorite != nil {
			tcm.OnFavorite(true)
		}
	})
	favorite.Icon = myTheme.FavoriteIcon
	unfavorite := fyne.NewMenuItem(lang.L("Unset favorite"), func() {
		if tcm.OnFavorite != nil {
			tcm.OnFavorite(false)
		}
	})
	unfavorite.Icon = myTheme.NotFavoriteIcon
	tcm.menu.Items = append(tcm.menu.Items, fyne.NewMenuItemSeparator(),
		playlist, download, tcm.infoMenuItem)
	tcm.shareMenuItem = fyne.NewMenuItem(lang.L("Share")+"...", func() {
		if tcm.OnShare != nil {
			tcm.OnShare()
		}
	})
	tcm.shareMenuItem.Icon = myTheme.ShareIcon
	tcm.menu.Items = append(tcm.menu.Items, tcm.shareMenuItem)
	if disablePlaybackMenu {
		tcm.menu.Items = append(tcm.menu.Items, tcm.songRadioMenuItem)
	}
	tcm.menu.Items = append(tcm.menu.Items, fyne.NewMenuItemSeparator())
	tcm.menu.Items = append(tcm.menu.Items, favorite, unfavorite)
	tcm.ratingSubmenu = NewRatingSubmenu(func(rating int) {
		if tcm.OnSetRating != nil {
			tcm.OnSetRating(rating)
		}
	})
	tcm.menu.Items = append(tcm.menu.Items, tcm.ratingSubmenu)
	if len(auxItems) > 0 {
		tcm.menu.Items = append(tcm.menu.Items, fyne.NewMenuItemSeparator())
		tcm.menu.Items = append(tcm.menu.Items, auxItems...)
	}

	return tcm
}

func (tcm *TrackContextMenu) SetRatingDisabled(disabled bool) {
	tcm.ratingSubmenu.Disabled = disabled
}

func (tcm *TrackContextMenu) SetShareDisabled(disabled bool) {
	tcm.shareMenuItem.Disabled = disabled
}

func (tcm *TrackContextMenu) SetInfoDisabled(disabled bool) {
	tcm.infoMenuItem.Disabled = disabled
}

func (tcm *TrackContextMenu) ShowAtPosition(pos fyne.Position, canvas fyne.Canvas) {
	widget.ShowPopUpMenuAtPosition(tcm.menu, canvas, pos)
}
