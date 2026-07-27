module github.com/dweymouth/supersonic

go 1.25.0

require (
	fyne.io/fyne/v2 v2.7.2
	github.com/20after4/configdir v0.1.1
	github.com/Microsoft/go-winio v0.6.2
	github.com/boxes-ltd/imaging v1.7.5
	github.com/cenkalti/dominantcolor v1.0.3
	github.com/charlievieth/strcase v0.0.6
	github.com/deluan/sanitize v0.0.0-20241120162836-fdfd8fdfaa55
	github.com/dweymouth/fyne-advanced-list v0.0.0-20250211191927-58ea85eec72c
	github.com/dweymouth/fyne-tooltip v0.4.0
	github.com/dweymouth/go-jellyfin v0.0.0-20260422144755-ac6d50639b1a
	github.com/go-audio/audio v1.0.0
	github.com/go-audio/wav v1.1.0
	github.com/godbus/dbus/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/pelletier/go-toml/v2 v2.4.2
	github.com/quarckster/go-mpris-server v1.0.3
	github.com/supersonic-app/fyne-lyrics v0.0.0-20250614151306-b1880a70a410
	github.com/supersonic-app/go-mpv v0.1.1-0.20250822102843-7a8cde5f5449
	github.com/supersonic-app/go-subsonic v0.0.0-20260416152144-7a5f505a273c
	github.com/supersonic-app/go-upnpcast v0.1.1-0.20260517163705-d76cd97c192f
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/net v0.57.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	golang.org/x/text v0.40.0
)

require (
	fyne.io/systray v1.12.2 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/FyshOS/fancyfs v0.0.1 // indirect
	github.com/anthonynsimon/bild v0.14.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fredbi/uri v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fyne-io/gl-js v0.2.1-0.20260315212741-029c47fd27e8 // indirect
	github.com/fyne-io/glfw-js v0.4.0 // indirect
	github.com/fyne-io/image v0.1.1 // indirect
	github.com/fyne-io/oksvg v0.2.0 // indirect
	github.com/go-audio/riff v1.0.0 // indirect
	github.com/go-gl/gl v0.0.0-20260331235117-4566fea9a276 // indirect
	github.com/go-gl/glfw/v3.4/glfw v0.1.0-pre.1.0.20260627172858-eb9c312d9d47 // indirect
	github.com/go-text/render v0.2.1 // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hack-pad/go-indexeddb v0.3.2 // indirect
	github.com/hack-pad/safejs v0.1.1 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/jeandeaual/go-locale v0.0.0-20250612000132-0ef82f21eade // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	github.com/koron/go-ssdp v0.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/nicksnyder/go-i18n/v2 v2.6.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rymdport/portal v0.4.2 // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/yuin/goldmark v1.8.2 // indirect
	golang.org/x/image v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace fyne.io/fyne/v2 v2.7.2 => github.com/dweymouth/fyne/v2 v2.3.0-rc1.0.20260707001049-35cc7c250c08

replace github.com/go-audio/wav v1.1.0 => github.com/dweymouth/go-wav v0.0.0-20250719173115-e60429a83eb0
