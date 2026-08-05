package controller

import (
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/dialogs"
	"github.com/supersonic-app/supersonic/ui/util"
)

// Show dialog to select playlist.
// Depending on the results of that dialog, potentially create a new playlist
// Add tracks to the user-specified playlist
func (m *Controller) DoAddTracksToPlaylistWorkflow(trackIDs []string) {
	sp := dialogs.NewSelectPlaylistDialog(m.App.ServerManager.Server, m.App.ImageManager,
		m.App.ServerManager.LoggedInUser, m.App.Config.Application.AddToPlaylistSkipDuplicates)
	pop := widget.NewModalPopUp(container.NewPadded(sp.SearchDialog), m.MainWindow.Canvas())
	sp.SetOnDismiss(func() {
		pop.Hide()
		m.doModalClosed()
	})
	sp.SetOnNavigateTo(func(contentType mediaprovider.ContentType, id string) {
		notifySuccess := func(n int) {
			fyne.Do(func() {
				msg := lang.LocalizePluralKey("playlist.addedtracks",
					"Added tracks to playlist", n, map[string]string{"trackCount": strconv.Itoa(n)})
				m.ToastProvider.ShowSuccessToast(msg)
			})
		}
		notifyError := util.FyneDoFunc(func() {
			m.ToastProvider.ShowErrorToast(
				lang.L("An error occurred adding tracks to the playlist"),
			)
		})
		pop.Hide()
		m.App.Config.Application.AddToPlaylistSkipDuplicates = sp.SkipDuplicates
		if id == "" /* creating new playlist */ {
			go func() {
				err := m.App.ServerManager.Server.CreatePlaylistWithTracks(sp.SearchDialog.SearchQuery(), trackIDs)
				if err == nil {
					notifySuccess(len(trackIDs))
				} else {
					log.Printf("error adding tracks to playlist: %s", err.Error())
					notifyError()
				}
			}()
		} else {
			m.App.Config.Application.DefaultPlaylistID = id
			if sp.SkipDuplicates {
				go func() {
					currentTrackIDs := make(map[string]struct{})
					if selectedPlaylist, err := m.App.ServerManager.Server.GetPlaylist(id); err != nil {
						log.Printf("error getting playlist: %s", err.Error())
						notifyError()
					} else {
						for _, track := range selectedPlaylist.Tracks {
							currentTrackIDs[track.ID] = struct{}{}
						}
						filterTrackIDs := sharedutil.FilterSlice(trackIDs, func(trackID string) bool {
							_, ok := currentTrackIDs[trackID]
							return !ok
						})
						err := m.App.ServerManager.Server.AddPlaylistTracks(id, filterTrackIDs)
						if err == nil {
							notifySuccess(len(filterTrackIDs))
						} else {
							log.Printf("error adding tracks to playlist: %s", err.Error())
							notifyError()
						}
					}
				}()
			} else {
				go func() {
					err := m.App.ServerManager.Server.AddPlaylistTracks(id, trackIDs)
					if err == nil {
						notifySuccess(len(trackIDs))
					} else {
						log.Printf("error adding tracks to playlist: %s", err.Error())
						notifyError()
					}
				}()
			}
		}
	})
	m.ClosePopUpOnEscape(pop)
	m.haveModal = true
	min := sp.MinSize()
	height := fyne.Max(min.Height, fyne.Min(min.Height*1.5, m.MainWindow.Canvas().Size().Height*0.7))
	sp.SearchDialog.Show()
	pop.Resize(fyne.NewSize(min.Width, height))
	pop.Show()
	m.MainWindow.Canvas().Focus(sp.GetSearchEntry())
}

func (m *Controller) DoEditPlaylistWorkflow(playlist *mediaprovider.Playlist) {
	canMakePublic := m.App.ServerManager.Server.CanMakePublicPlaylist()
	dlg := dialogs.NewEditPlaylistDialog(playlist, canMakePublic)
	pop := widget.NewModalPopUp(container.NewPadded(dlg), m.MainWindow.Canvas())
	m.ClosePopUpOnEscape(pop)
	dlg.OnCanceled = func() {
		pop.Hide()
		m.doModalClosed()
	}
	dlg.OnDeletePlaylist = func() {
		pop.Hide()
		dialog.ShowCustomConfirm(lang.L("Confirm Delete Playlist"), lang.L("OK"), lang.L("Cancel"), layout.NewSpacer(), /*custom content*/
			func(ok bool) {
				if !ok {
					pop.Show()
				} else {
					m.doModalClosed()
					go func() {
						if err := m.App.ServerManager.Server.DeletePlaylist(playlist.ID); err != nil {
							log.Printf("error deleting playlist: %s", err.Error())
						} else if rte := m.CurPageFunc(); rte.Page == Playlist && rte.Arg == playlist.ID {
							// navigate to playlists page if user is still on the page of the deleted playlist
							fyne.Do(func() { m.NavigateTo(PlaylistsRoute()) })
						}
					}()
				}
			}, m.MainWindow)
	}
	dlg.OnUpdateMetadata = func() {
		pop.Hide()
		m.doModalClosed()
		go func() {
			s := m.App.ServerManager.GetServer()
			if s == nil {
				return // logged out
			}
			err := s.EditPlaylist(playlist.ID, dlg.Name, dlg.Description, dlg.IsPublic)
			if err != nil {
				fyne.Do(func() { m.ToastProvider.ShowErrorToast(lang.L("Error updating playlist")) })
				log.Printf("error updating playlist: %s", err.Error())
			} else if rte := m.CurPageFunc(); rte.Page == Playlist && rte.Arg == playlist.ID {
				// if user is on playlist page, reload to get the updates
				fyne.Do(m.ReloadFunc)
			}
		}()
	}
	m.haveModal = true
	pop.Show()
}

func (m *Controller) DoCreatePlaylistWorkflow() {
	canMakePublic := m.App.ServerManager.Server.CanMakePublicPlaylist()
	dlg := dialogs.NewCreatePlaylistDialog(canMakePublic)
	pop := widget.NewModalPopUp(container.NewPadded(dlg), m.MainWindow.Canvas())
	m.ClosePopUpOnEscape(pop)
	dlg.OnCanceled = func() {
		pop.Hide()
		m.doModalClosed()
	}
	dlg.OnUpdateMetadata = func() {
		pop.Hide()
		m.doModalClosed()
		go func() {
			s := m.App.ServerManager.GetServer()
			if s == nil {
				return // logged out
			}
			err := s.CreatePlaylist(dlg.Name, dlg.Description, dlg.IsPublic)
			if err != nil {
				fyne.Do(func() { m.ToastProvider.ShowErrorToast(lang.L("Error creating playlist")) })
				log.Printf("error creating playlist: %s", err.Error())
			} else {
				fyne.Do(func() {
					// Right now, this workflow is only initiated by the "New Playlist" button
					// on the playlists page. Reload it so the new playlist shows up.
					m.ReloadFunc()
					m.ToastProvider.ShowSuccessToast(lang.L("Successfully created playlist"))
				})
			}
		}()
	}
	m.haveModal = true
	pop.Show()
}
