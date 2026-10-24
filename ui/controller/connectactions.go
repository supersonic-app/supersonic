package controller

import (
	"log"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

func (m *Controller) ConnectTracklistActionsWithReplayGainAlbum(tracklist *widgets.Tracklist) {
	m.connectTracklistActionsWithReplayGainMode(tracklist, player.ReplayGainAlbum)
}

func (m *Controller) ConnectTracklistActions(tracklist *widgets.Tracklist) {
	m.connectTracklistActionsWithReplayGainMode(tracklist, player.ReplayGainTrack)
}

func (m *Controller) connectTracklistActionsWithReplayGainMode(tracklist *widgets.Tracklist, mode player.ReplayGainMode) {
	tracklist.OnAddToPlaylist = m.DoAddTracksToPlaylistWorkflow
	tracklist.OnPlaySelectionNext = func(tracks []*mediaprovider.Track) {
		m.App.PlaybackManager.LoadTracks(tracks, backend.InsertNext, false)
	}
	tracklist.OnAddToQueue = func(tracks []*mediaprovider.Track) {
		m.App.PlaybackManager.LoadTracks(tracks, backend.Append, false)
	}
	tracklist.OnPlayTrackAt = func(idx int) {
		m.App.PlaybackManager.LoadTracksAndPlayAtIdx(tracklist.GetTracks(), false, idx)
		if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
			m.App.PlaybackManager.SetReplayGainMode(mode)
		}
	}
	tracklist.OnPlaySelection = func(tracks []*mediaprovider.Track, shuffle bool) {
		m.App.PlaybackManager.LoadTracks(tracks, backend.Replace, shuffle)
		if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
			m.App.PlaybackManager.SetReplayGainMode(mode)
		}
		m.App.PlaybackManager.PlayFromBeginning()
	}
	tracklist.OnSetFavorite = m.SetTrackFavorites
	tracklist.OnSetRating = m.SetTrackRatings
	tracklist.OnShowAlbumPage = func(albumID string) {
		m.NavigateTo(AlbumRoute(albumID))
	}
	tracklist.OnShowArtistPage = func(artistID string) {
		m.NavigateTo(ArtistRoute(artistID))
	}
	tracklist.OnShowGenrePage = func(genre string) {
		m.NavigateTo(GenreRoute(genre))
	}
	tracklist.OnColumnVisibilityMenuShown = func(pop *widget.PopUp) {
		m.ClosePopUpOnEscape(pop)
	}
	tracklist.OnDownload = m.ShowDownloadDialog
	tracklist.OnShare = func(trackID string) {
		m.ShowShareDialog(trackID)
	}
	tracklist.OnShowTrackInfo = m.ShowTrackInfoDialog
	tracklist.OnPlaySongRadio = func(track *mediaprovider.Track) {
		go func() {
			tracks, err := m.GetSongRadioTracks(track)
			if err != nil {
				log.Println("Error getting song radio: ", err)
				return
			}
			m.App.PlaybackManager.LoadTracks(tracks, backend.Replace, false)
			if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
				m.App.PlaybackManager.SetReplayGainMode(mode)
			}
			m.App.PlaybackManager.PlayFromBeginning()
		}()
	}
}

func (m *Controller) ConnectAlbumGridActions(grid *widgets.GridView) {
	grid.OnAddToQueue = func(albumID string) {
		go m.App.PlaybackManager.LoadAlbum(albumID, backend.Append, false)
	}
	grid.OnPlayNext = func(albumID string) {
		go m.App.PlaybackManager.LoadAlbum(albumID, backend.InsertNext, false)
	}
	grid.OnPlay = func(albumID string, shuffle bool) {
		go m.App.PlaybackManager.PlayAlbum(albumID, 0, shuffle)
	}
	grid.OnFavorite = func(albumID string, favorite bool) {
		m.App.ServerManager.Server.SetFavorite(mediaprovider.RatingFavoriteParameters{
			AlbumIDs: []string{albumID},
		}, favorite)
	}
	grid.OnShowItemPage = func(albumID string) {
		m.NavigateTo(AlbumRoute(albumID))
	}
	grid.OnShowSecondaryPage = func(artistID string) {
		m.NavigateTo(ArtistRoute(artistID))
	}
	grid.OnAddToPlaylist = m.onAddAlbumToPlaylist
	grid.OnDownload = m.onDownloadAlbum
	grid.OnShare = func(albumID string) {
		m.ShowShareDialog(albumID)
	}
}

func (m *Controller) ConnectGroupedReleasesActions(grid *widgets.GroupedReleases) {
	grid.OnAddToQueue = func(albumID string) {
		go m.App.PlaybackManager.LoadAlbum(albumID, backend.Append, false)
	}
	grid.OnPlayNext = func(albumID string) {
		go m.App.PlaybackManager.LoadAlbum(albumID, backend.InsertNext, false)
	}
	grid.OnPlay = func(albumID string, shuffle bool) {
		go m.App.PlaybackManager.PlayAlbum(albumID, 0, shuffle)
	}
	grid.OnFavorite = func(albumID string, favorite bool) {
		m.App.ServerManager.Server.SetFavorite(mediaprovider.RatingFavoriteParameters{
			AlbumIDs: []string{albumID},
		}, favorite)
	}
	grid.OnShowItemPage = func(albumID string) {
		m.NavigateTo(AlbumRoute(albumID))
	}
	grid.OnShowSecondaryPage = func(artistID string) {
		m.NavigateTo(ArtistRoute(artistID))
	}
	grid.OnAddToPlaylist = m.onAddAlbumToPlaylist
	grid.OnDownload = m.onDownloadAlbum
	grid.OnShare = func(albumID string) {
		m.ShowShareDialog(albumID)
	}
}

func (m *Controller) onAddAlbumToPlaylist(albumID string) {
	go func() {
		album, err := m.App.ServerManager.Server.GetAlbum(albumID)
		if err != nil {
			log.Printf("error loading album: %s", err.Error())
			return
		}
		fyne.Do(func() {
			m.DoAddTracksToPlaylistWorkflow(sharedutil.TracksToIDs(album.Tracks))
		})
	}()
}

func (m *Controller) onDownloadAlbum(albumID string) {
	go func() {
		album, err := m.App.ServerManager.Server.GetAlbum(albumID)
		if err != nil {
			log.Printf("error loading album: %s", err.Error())
			return
		}
		fyne.Do(func() {
			m.ShowDownloadDialog(album.Tracks, album.Name)
		})
	}()
}

func (m *Controller) ConnectArtistGridActions(grid *widgets.GridView) {
	grid.OnShowItemPage = func(id string) { m.NavigateTo(ArtistRoute(id)) }
	grid.OnPlayNext = func(artistID string) {
		go m.App.PlaybackManager.LoadTracks(m.GetArtistTracks(artistID), backend.InsertNext, false)
	}
	grid.OnPlay = func(artistID string, shuffle bool) { go m.App.PlaybackManager.PlayArtistDiscography(artistID, shuffle) }
	grid.OnAddToQueue = func(artistID string) {
		go m.App.PlaybackManager.LoadTracks(m.GetArtistTracks(artistID), backend.Append, false)
	}
	grid.OnAddToPlaylist = func(artistID string) {
		m.DoAddTracksToPlaylistWorkflow(
			sharedutil.TracksToIDs(m.GetArtistTracks(artistID)))
	}
	grid.OnFavorite = func(artistID string, favorite bool) {
		m.App.ServerManager.Server.SetFavorite(mediaprovider.RatingFavoriteParameters{
			ArtistIDs: []string{artistID},
		}, favorite)
	}
	grid.OnDownload = func(artistID string) {
		go func() {
			tracks := m.GetArtistTracks(artistID)
			tist, err := m.App.ServerManager.Server.GetArtist(artistID)
			if err != nil {
				log.Printf("error getting artist: %v", err.Error())
				return
			}
			fyne.Do(func() {
				m.ShowDownloadDialog(tracks, tist.Name)
			})
		}()
	}
	grid.OnShare = func(artistID string) {
		m.ShowShareDialog(artistID)
	}
}

func (c *Controller) ConnectPlayQueuelistActions(list *widgets.PlayQueueList) {
	list.OnReorderItems = func(idxs []int, insertPos int) {
		// idxs and insertPos index the full queue, not the displayed items
		newTracks := sharedutil.ReorderItems(list.Queue(), idxs, insertPos)
		c.App.PlaybackManager.UpdatePlayQueue(newTracks)
	}
	list.OnDownload = c.ShowDownloadDialog
	list.OnShowTrackInfo = c.ShowTrackInfoDialog
	list.OnShare = func(tracks []*mediaprovider.Track) {
		if len(tracks) > 0 {
			c.ShowShareDialog(tracks[0].ID)
		}
	}
	list.OnAddToPlaylist = c.DoAddTracksToPlaylistWorkflow
	list.OnPlayItemAt = func(tracknum int) {
		c.App.PlaybackManager.PlayTrackAt(tracknum)
	}
	list.OnShowArtistPage = func(artistID string) {
		c.NavigateTo(ArtistRoute(artistID))
	}
	list.OnRemoveFromQueue = func(idxs []int) {
		list.UnselectAll()
		c.App.PlaybackManager.RemoveTracksFromQueue(idxs)
	}
	list.OnSetRating = c.SetTrackRatings
	list.OnSetFavorite = c.SetTrackFavorites
	list.OnPlaySongRadio = func(track *mediaprovider.Track) {
		go func() {
			if err := c.App.PlaybackManager.PlaySimilarSongs(track.ID); err != nil {
				fyne.Do(func() {
					c.ToastProvider.ShowErrorToast(lang.L("Unable to play song radio"))
				})
			}
		}()
	}
	list.OnShowTrackInfo = c.ShowTrackInfoDialog
}
