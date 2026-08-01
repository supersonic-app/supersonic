//go:build localav

package backend

import (
	"fmt"

	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/localav"
)

// initLocalPlayer constructs and initializes the localav-backed local player.
func (a *App) initLocalPlayer() error {
	p := localav.New()
	if err := p.Init(); err != nil {
		return fmt.Errorf("failed to initialize localav player: %s", err.Error())
	}
	a.LocalPlayer = p
	return nil
}

// localPlayerSetup performs player-specific post-init setup (localav variant).
func (a *App) localPlayerSetup(p player.LocalPlayer) {
	// localav has no extra setup beyond what setupLocalPlayer does.
}
