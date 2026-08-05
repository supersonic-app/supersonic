//go:build darwin

package ui

/*
#include <stdlib.h>
void installReopenDelegate();
void dockMenuBegin();
void dockMenuAddItem(const char* title, int index);
void dockMenuAddSeparator();
void dockMenuCommit();
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
)

var darwinAppDelegateReopenWindow fyne.Window

// darwinQuitting is set to true when applicationShouldTerminate: fires so
// the close intercept knows to actually close instead of hide.
var darwinQuitting bool

func isRealQuit() bool {
	return darwinQuitting
}

func installReopenHandler(w fyne.Window) {
	darwinAppDelegateReopenWindow = w
	C.installReopenDelegate()
}

//export appReopened
func appReopened() {
	if darwinAppDelegateReopenWindow == nil {
		return
	}
	go func() {
		fyne.Do(darwinAppDelegateReopenWindow.Show)
	}()
}

//export appShouldTerminate
func appShouldTerminate() {
	darwinQuitting = true
}

// darwinDockMenuCallbacks is indexed by the NSMenuItem tag set in
// dockMenuAddItem, dispatched back to Go via dockMenuItemClicked.
var darwinDockMenuCallbacks []func()

func installDockMenu(menu *fyne.Menu) {
	darwinDockMenuCallbacks = darwinDockMenuCallbacks[:0]

	C.dockMenuBegin()
	for _, item := range menu.Items {
		if item.IsSeparator {
			C.dockMenuAddSeparator()
			continue
		}
		cTitle := C.CString(item.Label)
		C.dockMenuAddItem(cTitle, C.int(len(darwinDockMenuCallbacks)))
		C.free(unsafe.Pointer(cTitle))
		darwinDockMenuCallbacks = append(darwinDockMenuCallbacks, item.Action)
	}
	C.dockMenuCommit()
}

//export dockMenuItemClicked
func dockMenuItemClicked(index C.int) {
	i := int(index)
	if i < 0 || i >= len(darwinDockMenuCallbacks) {
		return
	}
	if cb := darwinDockMenuCallbacks[i]; cb != nil {
		go func() { fyne.Do(cb) }()
	}
}
