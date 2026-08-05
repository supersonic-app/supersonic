package res

const (
	AppName          = "supersonic"
	AppID            = "io.github.supersonic-app.supersonic"
	DisplayName      = "Supersonic"
	AppVersion       = "0.22.0"
	AppVersionTag    = "v" + AppVersion
	ConfigFile       = "config.toml"
	GithubURL        = "https://github.com/supersonic-app/supersonic"
	LatestReleaseURL = GithubURL + "/releases/latest"
	KofiURL          = "https://ko-fi.com/dweymouth"
	Copyright        = "Copyright © 2022–2026 Drew Weymouth and contributors"
)

var (
	WhatsAdded = `
## Added
* IPC endpoint for getting the current track/radio station
* Support for the OpenSubsonic playbackReport extension
* "All Tracks" option on the Startup page
* Cover art support for internet radio stations
* Updated Polish, Spanish, Greek, Turkish, and French translations
* Migrate to Fyne 2.8, adding Wayland support
* Match window border color to app theme rather than OS theme
* Artist biography text now scrolls instead of being truncated`

	WhatsFixed = `
## Fixed
* UI freeze after switching workspaces on Wayland
* Segfault when clicking the tray icon on Wayland
* Crash/freeze when system keyring unlock dialog blocked the main event loop on startup
* Race condition when using an alternate hostname caused 401 auth errors
* UI scale setting not applied correctly when using a non-English language
* DLNA cast DIDL-Lite metadata missing artist, album, cover art, and <res> audio attributes
* Stale cover art and Now Playing background persisting on tracks without art
* Autoplay incorrectly enqueuing tracks while a radio station is playing`
)
