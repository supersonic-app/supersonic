//go:build !localav

package backend

import (
	"fmt"

	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/mpv"
)

// initLocalPlayer constructs and initializes the mpv-backed local player.
func (a *App) initLocalPlayer() error {
	p := mpv.NewWithClientName(a.appName)
	c := a.Config.LocalPlayback
	c.InMemoryCacheSizeMB = clamp(c.InMemoryCacheSizeMB, 10, 500)
	if err := p.Init(c.InMemoryCacheSizeMB); err != nil {
		return fmt.Errorf("failed to initialize mpv player: %s", err.Error())
	}
	a.LocalPlayer = p
	return nil
}

// localPlayerSetup performs player-specific post-init setup (mpv variant).
// Called by setupLocalPlayer after the common device/volume/EQ setup.
func (a *App) localPlayerSetup(p player.LocalPlayer) {
	// mpv has no extra setup beyond what setupLocalPlayer does.
}
