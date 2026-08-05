package browsing

import (
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/util"
)

type NavigationHandler interface {
	SetPage(Page)
	CurrentPage() controller.Route
}

type Router struct {
	App        *backend.App
	Controller *controller.Controller
	Nav        NavigationHandler
	widgetPool *util.WidgetPool

	OnNavigateTo func(controller.Route)
}

func NewRouter(app *backend.App, controller *controller.Controller, nav NavigationHandler) Router {
	r := Router{
		App:        app,
		Controller: controller,
		Nav:        nav,
		widgetPool: util.NewWidgetPool(app.BackgroundContext()),
	}
	return r
}

func (r Router) CreatePage(rte controller.Route) Page {
	_, canRate := r.App.ServerManager.Server.(mediaprovider.SupportsRating)
	_, canShare := r.App.ServerManager.Server.(mediaprovider.SupportsSharing)
	switch rte.Page {
	case controller.Album:
		return NewAlbumPage(rte.Arg, &r.App.Config.AlbumPage, r.widgetPool, r.App.PlaybackManager, r.App.ServerManager.Server, r.App.ImageManager, r.Controller)
	case controller.Albums:
		return NewAlbumsPage(&r.App.Config.AlbumsPage, r.widgetPool, r.Controller, r.App.PlaybackManager, r.App.ServerManager.Server, r.App.ImageManager)
	case controller.Artist:
		return NewArtistPage(rte.Arg, &r.App.Config.ArtistPage, r.widgetPool, r.App.PlaybackManager, r.App.ServerManager.Server, r.App.ImageManager, r.Controller)
	case controller.Artists:
		return NewArtistsPage(&r.App.Config.ArtistsPage, r.widgetPool, r.Controller, r.App.PlaybackManager, r.App.ServerManager.Server, r.App.ImageManager)
	case controller.Favorites:
		return NewFavoritesPage(&r.App.Config.FavoritesPage, r.widgetPool, r.Controller, r.App.ServerManager.Server, r.App.PlaybackManager, r.App.ImageManager)
	case controller.Genre:
		return NewGenrePage(rte.Arg, &r.App.Config.AlbumsPage, r.widgetPool, r.Controller, r.App.PlaybackManager, r.App.ServerManager.Server, r.App.ImageManager)
	case controller.Genres:
		return NewGenresPage(r.Controller, r.App.ServerManager.Server)
	case controller.NowPlaying:
		return NewNowPlayingPage(&r.App.Config.NowPlayingConfig, r.Controller, r.widgetPool, r.App.ServerManager, r.App.LyricsManager, r.App.ImageManager, r.App.PlaybackManager, r.App.ServerManager.Server, canRate, canShare, r.App.Config)
	case controller.Playlist:
		return NewPlaylistPage(rte.Arg, &r.App.Config.PlaylistPage, r.widgetPool, r.Controller, r.App.ServerManager, r.App.PlaybackManager, r.App.ImageManager)
	case controller.Playlists:
		return NewPlaylistsPage(r.Controller, r.widgetPool, &r.App.Config.PlaylistsPage, r.App.ServerManager.Server)
	case controller.Tracks:
		return NewTracksPage(r.Controller, &r.App.Config.TracksPage, r.widgetPool, r.App.ServerManager.Server, r.App.ImageManager)
	case controller.Radios:
		var rp mediaprovider.RadioProvider
		rp, _ = r.App.ServerManager.Server.(mediaprovider.RadioProvider)
		return NewRadiosPage(r.Controller, rp, r.App.PlaybackManager)
	}
	return nil
}

func (r *Router) NavigateTo(rte controller.Route) {
	if rte != r.Nav.CurrentPage() {
		r.Nav.SetPage(r.CreatePage(rte))
	}
	if r.OnNavigateTo != nil {
		r.OnNavigateTo(rte)
	}
}
