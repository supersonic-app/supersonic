package dialogs

import (
	// "fmt"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/deluan/sanitize"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/jellyfin"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/util"
)

type SelectPlaylist struct {
	SearchDialog      *SearchDialog
	mp                mediaprovider.MediaProvider
	loggedInUser      string
	allPlaylistResuts []*mediaprovider.SearchResult
	SkipDuplicates    bool
}

func NewSelectPlaylistDialog(mp mediaprovider.MediaProvider, im util.ImageFetcher, loggedInUser string, skipDups bool) *SelectPlaylist {
	sp := &SelectPlaylist{
		mp:             mp,
		loggedInUser:   loggedInUser,
		SkipDuplicates: skipDups,
	}
	sd := NewSearchDialog(
		im,
		lang.L("Add to playlist"),
		lang.L("Cancel"),
		sp.onSearched,
	)
	sd.ActionItem = widget.NewCheckWithData(lang.L("Skip duplicate tracks"), binding.BindBool(&sp.SkipDuplicates))
	sd.PlaceholderText = lang.L("Search playlists or new playlist name")
	sp.SearchDialog = sd
	return sp
}

func (sp *SelectPlaylist) fetchUserOwnedPlaylists() {
	playlists, err := sp.mp.GetPlaylists()
	if err != nil {
		// TODO: surface this error to user
		log.Printf("error getting playlists: %s", err.Error())
	}
	userPlaylists := sharedutil.FilterSlice(playlists, func(playlist *mediaprovider.Playlist) bool {
		if _, isJellyfin := sp.mp.(*jellyfin.JellyfinMediaProvider); isJellyfin {
			// Jellyfin usernames are case-insensitive
			return strings.EqualFold(playlist.Owner, sp.loggedInUser)
		} else {
			// Subsonic usernames are case-sensitive
			return playlist.Owner == sp.loggedInUser
		}
	})
	sp.allPlaylistResuts = sharedutil.MapSlice(userPlaylists, sp.playlistToSearchResult)
}

func (sp *SelectPlaylist) playlistToSearchResult(playlist *mediaprovider.Playlist) *mediaprovider.SearchResult {
	if playlist == nil {
		return nil
	}
	return &mediaprovider.SearchResult{
		Name:       playlist.Name,
		ID:         playlist.ID,
		CoverID:    playlist.CoverArtID,
		Type:       mediaprovider.ContentTypePlaylist,
		Size:       playlist.TrackCount,
		ArtistName: playlist.Name,
	}
}

func (sp *SelectPlaylist) onSearched(query string) []*mediaprovider.SearchResult {
	if sp.allPlaylistResuts == nil {
		sp.fetchUserOwnedPlaylists()
	}
	var results []*mediaprovider.SearchResult
	if query == "" {
		results = sp.allPlaylistResuts
	} else {
		results = sharedutil.FilterSlice(sp.allPlaylistResuts, func(playlist *mediaprovider.SearchResult) bool {
			return strings.Contains(
				sanitize.Accents(strings.ToLower(playlist.Name)),
				sanitize.Accents(strings.ToLower(query)),
			)
		})
		results = append(results, &mediaprovider.SearchResult{
			Name: fmt.Sprintf("%s: %s", lang.L("Create new playlist"), query),
			Type: mediaprovider.ContentTypePlaylist,
		})
	}

	return results
}

func (sp *SelectPlaylist) SetOnDismiss(onDismiss func()) {
	sp.SearchDialog.OnDismiss = onDismiss
}

func (sp *SelectPlaylist) SetOnNavigateTo(onNavigateTo func(mediaprovider.ContentType, string)) {
	sp.SearchDialog.OnNavigateTo = onNavigateTo
}

func (sp *SelectPlaylist) MinSize() fyne.Size {
	return sp.SearchDialog.MinSize()
}

func (sp *SelectPlaylist) GetSearchEntry() fyne.Focusable {
	return sp.SearchDialog.GetSearchEntry()
}
