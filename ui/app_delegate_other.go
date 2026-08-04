//go:build !darwin

package ui

import "fyne.io/fyne/v2"

func installReopenHandler(w fyne.Window) {
}

func isRealQuit() bool {
	return false
}

func setAppDockIconVisible(visible bool) {
}

func installDockMenu(menu *fyne.Menu) {
}
