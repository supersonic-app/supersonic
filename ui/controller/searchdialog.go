package controller

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/dialogs"
)

func (c *Controller) ShowQuickSearch() {
	qs := dialogs.NewQuickSearch(c.App.ServerManager.Server, c.App.ImageManager)
	pop := widget.NewModalPopUp(container.NewPadded(qs.SearchDialog), c.MainWindow.Canvas())
	qs.SetOnDismiss(func() {
		pop.Hide()
		c.doModalClosed()
	})

	qs.SetOnNavigateTo(func(contentType mediaprovider.ContentType, id string) {
		pop.Hide()
		c.doModalClosed()
		switch contentType {
		case mediaprovider.ContentTypeAlbum:
			c.NavigateTo(AlbumRoute(id))
		case mediaprovider.ContentTypeArtist:
			c.NavigateTo(ArtistRoute(id))
		case mediaprovider.ContentTypeTrack:
			go c.App.PlaybackManager.PlayTrack(id)
		case mediaprovider.ContentTypePlaylist:
			c.NavigateTo(PlaylistRoute(id))
		case mediaprovider.ContentTypeGenre:
			c.NavigateTo(GenreRoute(id))
		case mediaprovider.ContentTypeRadioStation:
			if rp, ok := c.App.ServerManager.Server.(mediaprovider.RadioProvider); ok {
				if radio, err := rp.GetRadioStation(id); err == nil {
					go c.App.PlaybackManager.PlayRadioStation(radio)
				}
			}
		}
	})
	qs.OnPlay = func(t mediaprovider.ContentType, id string, item any, shuffle bool) {
		switch t {
		case mediaprovider.ContentTypeTrack:
			c.App.PlaybackManager.LoadTracks(
				[]*mediaprovider.Track{item.(*mediaprovider.Track)},
				backend.Replace, false /*shuffle*/)
			c.App.PlaybackManager.PlayFromBeginning()
		case mediaprovider.ContentTypeAlbum:
			go c.App.PlaybackManager.PlayAlbum(id, 0, shuffle)
		case mediaprovider.ContentTypeArtist:
			go c.App.PlaybackManager.PlayArtistDiscography(id, shuffle)
		case mediaprovider.ContentTypePlaylist:
			go c.App.PlaybackManager.PlayPlaylist(id, 0, shuffle)
		case mediaprovider.ContentTypeGenre:
			go c.App.PlaybackManager.PlayRandomSongs(id /*genre name*/)
		case mediaprovider.ContentTypeRadioStation:
			go c.App.PlaybackManager.PlayRadioStation(item.(*mediaprovider.RadioStation))
		}
	}
	qs.OnAddToQueue = c.handleSearchDialogOnAddToQueue
	qs.OnAddToPlaylist = c.handleSearchDialogOnAddToPlaylist
	qs.OnDownload = func(track *mediaprovider.Track) {
		c.ShowDownloadDialog([]*mediaprovider.Track{track}, track.Metadata().Name)
	}
	qs.OnPlaySongRadio = func(track *mediaprovider.Track) {
		go func() {
			tracks, err := c.GetSongRadioTracks(track)
			if err != nil {
				c.App.PlaybackManager.LoadTracks(tracks, backend.Replace, false)
				c.App.PlaybackManager.PlayFromBeginning()
			}
		}()
	}
	qs.OnSetFavorite = func(trackID string, fav bool) {
		go c.SetTrackFavorites([]string{trackID}, fav)
	}
	qs.OnSetRating = func(trackID string, rating int) {
		go c.SetTrackRatings([]string{trackID}, rating)
	}
	qs.OnShare = func(trackID string) {
		c.ShowShareDialog(trackID)
	}
	qs.OnShowTrackInfo = func(track *mediaprovider.Track) {
		c.ShowTrackInfoDialog(track)
	}

	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	min := qs.MinSize()
	height := fyne.Max(min.Height, fyne.Min(min.Height*1.5, c.MainWindow.Canvas().Size().Height*0.7))
	qs.SearchDialog.Show()
	pop.Resize(fyne.NewSize(min.Width, height))
	pop.Show()
	c.MainWindow.Canvas().Focus(qs.GetSearchEntry())
}

func (c *Controller) handleSearchDialogOnAddToQueue(t mediaprovider.ContentType, id string, item any, next bool) {
	insertMode := backend.Append
	if next {
		insertMode = backend.InsertNext
	}
	switch t {
	case mediaprovider.ContentTypeTrack:
		c.App.PlaybackManager.LoadTracks(
			[]*mediaprovider.Track{item.(*mediaprovider.Track)},
			insertMode, false /*shuffle*/)
	case mediaprovider.ContentTypeAlbum:
		go c.App.PlaybackManager.LoadAlbum(id, insertMode, false)
	case mediaprovider.ContentTypeArtist:
		go func() {
			tracks := c.GetArtistTracks(id)
			c.App.PlaybackManager.LoadTracks(tracks, insertMode, false)
		}()
	case mediaprovider.ContentTypePlaylist:
		go c.App.PlaybackManager.LoadPlaylist(id, insertMode, false)
	case mediaprovider.ContentTypeGenre:
		go func() {
			tr, err := c.App.ServerManager.Server.GetRandomTracks(id /*genre name*/, c.App.Config.Application.EnqueueBatchSize)
			if err != nil {
				c.App.PlaybackManager.LoadTracks(tr, insertMode, false /*shuffle*/)
			}
		}()
	case mediaprovider.ContentTypeRadioStation:
		c.App.PlaybackManager.LoadRadioStation(item.(*mediaprovider.RadioStation), insertMode)
	}
}

func (c *Controller) handleSearchDialogOnAddToPlaylist(t mediaprovider.ContentType, id string, item any) {
	switch t {
	case mediaprovider.ContentTypeTrack:
		c.DoAddTracksToPlaylistWorkflow([]string{id})
	case mediaprovider.ContentTypeAlbum:
		go func() {
			album, err := c.App.ServerManager.Server.GetAlbum(id)
			if err == nil && len(album.Tracks) > 0 {
				trackIDs := sharedutil.MapSlice(album.Tracks, func(t *mediaprovider.Track) string {
					return t.ID
				})
				fyne.Do(func() { c.DoAddTracksToPlaylistWorkflow(trackIDs) })
			}
		}()
	case mediaprovider.ContentTypeArtist:
		go func() {
			tracks := c.GetArtistTracks(id)
			if len(tracks) > 0 {
				trackIDs := sharedutil.MapSlice(tracks, func(t *mediaprovider.Track) string {
					return t.ID
				})
				fyne.Do(func() { c.DoAddTracksToPlaylistWorkflow(trackIDs) })
			}
		}()
	case mediaprovider.ContentTypePlaylist:
		go func() {
			playlist, err := c.App.ServerManager.Server.GetPlaylist(id)
			if err == nil && len(playlist.Tracks) > 0 {
				trackIDs := sharedutil.MapSlice(playlist.Tracks, func(t *mediaprovider.Track) string {
					return t.ID
				})
				fyne.Do(func() { c.DoAddTracksToPlaylistWorkflow(trackIDs) })
			}
		}()
	case mediaprovider.ContentTypeGenre:
		go func() {
			tracks, err := c.App.ServerManager.Server.GetRandomTracks(id /*genre name*/, c.App.Config.Application.EnqueueBatchSize)
			if err == nil && len(tracks) > 0 {
				trackIDs := sharedutil.MapSlice(tracks, func(t *mediaprovider.Track) string {
					return t.ID
				})
				fyne.Do(func() { c.DoAddTracksToPlaylistWorkflow(trackIDs) })
			}
		}()
	case mediaprovider.ContentTypeRadioStation:
		log.Println("Cannot add radio station to playlist")
	}
}
