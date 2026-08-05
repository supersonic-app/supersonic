package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/ui/dialogs"
)

func (m *Controller) PromptForFirstServer() {
	d := dialogs.NewAddEditServerDialog(lang.L("Connect to Server"), false, nil, m.MainWindow.Canvas().Focus)
	pop := widget.NewModalPopUp(container.NewPadded(d), m.MainWindow.Canvas())
	d.OnSubmit = func() {
		d.DisableSubmit()
		go func() {
			if m.testConnectionAndUpdateDialogText(d) {
				// connection is good
				fyne.Do(func() {
					pop.Hide()
					m.doModalClosed()
				})
				conn := backend.ServerConnection{
					ServerType:    d.ServerType,
					Hostname:      d.Host,
					AltHostname:   d.AltHost,
					Username:      d.Username,
					LegacyAuth:    d.LegacyAuth,
					SkipSSLVerify: d.SkipSSLVerify,
				}
				server := m.App.ServerManager.AddServer(d.Nickname, conn)
				if err := m.trySetPasswordAndConnectToServer(server, d.Password); err != nil {
					log.Printf("error connecting to server: %s", err.Error())
				}
			} else {
				fyne.Do(d.EnableSubmit)
			}
		}()
	}
	m.haveModal = true
	pop.Show()
}

// DoConnectToServerWorkflow does the workflow for connecting to the last active server on startup
func (c *Controller) DoConnectToServerWorkflow(server *backend.ServerConfig) {
	// GetServerPassword may block on keyring unlock (showing a system dialog),
	// so run it in a goroutine to avoid freezing the Fyne event loop.
	go func() {
		pass, err := c.App.ServerManager.GetServerPassword(server.ID)
		if err != nil {
			log.Printf("error getting password from keyring: %v", err)
			fyne.Do(c.PromptForLoginAndConnect)
			return
		}

		// Password retrieved; show the dialog and connect from the Fyne thread
		fyne.Do(func() {
			c.doConnectWithPassword(server, pass)
		})
	}()
}

func (c *Controller) doConnectWithPassword(server *backend.ServerConfig, pass string) {
	canceled := false
	ctx, cancel := context.WithCancel(context.Background())
	dlg := dialog.NewCustom(lang.L("Connecting"), lang.L("Cancel"),
		widget.NewLabel(fmt.Sprintf(lang.L("Connecting to")+" %s", server.Nickname)), c.MainWindow)
	dlg.SetOnClosed(func() {
		canceled = true
		cancel()
	})
	c.haveModal = true
	dlg.Show()

	// try to connect
	go func() {
		defer cancel() // make sure to free up ctx resources if user does not cancel

		if err := c.tryConnectToServer(ctx, server, pass); err != nil {
			fyne.Do(func() {
				dlg.Hide()
				c.haveModal = false
				if canceled {
					c.PromptForLoginAndConnect()
				} else {
					// connection failure
					dlg := dialog.NewError(err, c.MainWindow)
					dlg.SetOnClosed(func() {
						c.PromptForLoginAndConnect()
					})
					c.haveModal = true
					dlg.Show()
				}
			})
		} else {
			fyne.Do(func() {
				dlg.Hide()
				c.haveModal = false
			})
		}
	}()
}

func (m *Controller) PromptForLoginAndConnect() {
	d := dialogs.NewLoginDialog(m.App.Config.Servers, m.App.ServerManager.GetServerPassword)
	pop := widget.NewModalPopUp(container.NewPadded(d), m.MainWindow.Canvas())
	d.OnSubmit = func(server *backend.ServerConfig, password string) {
		d.DisableSubmit()
		d.SetInfoText(lang.L("Testing connection") + "...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := m.App.ServerManager.TestConnectionAndAuth(ctx, server.ServerConnection, password)
			fyne.Do(func() {
				if err == backend.ErrUnreachable {
					d.SetErrorText(lang.L("Server unreachable"))
				} else if err != nil {
					d.SetErrorText(lang.L("Authentication failed"))
				} else {
					pop.Hide()
					m.trySetPasswordAndConnectToServer(server, password)
					m.doModalClosed()
				}
				d.EnableSubmit()
			})
		}()
	}
	d.OnEditServer = func(server *backend.ServerConfig) {
		pop.Hide()
		editD := dialogs.NewAddEditServerDialog(lang.L("Edit server"), true, server, m.MainWindow.Canvas().Focus)
		editPop := widget.NewModalPopUp(container.NewPadded(editD), m.MainWindow.Canvas())
		editD.OnSubmit = func() {
			d.DisableSubmit()
			go func() {
				success := m.testConnectionAndUpdateDialogText(editD)
				fyne.Do(func() {
					if success {
						// connection is good
						editPop.Hide()
						server.Hostname = editD.Host
						server.AltHostname = editD.AltHost
						server.Nickname = editD.Nickname
						server.Username = editD.Username
						server.LegacyAuth = editD.LegacyAuth
						server.SkipSSLVerify = editD.SkipSSLVerify
						m.trySetPasswordAndConnectToServer(server, editD.Password)
						m.doModalClosed()
					}
					d.EnableSubmit()
				})
			}()
		}
		editD.OnCancel = func() {
			editPop.Hide()
			pop.Show()
		}
		editPop.Show()
	}
	d.OnNewServer = func() {
		pop.Hide()
		newD := dialogs.NewAddEditServerDialog(lang.L("Add Server"), true, nil, m.MainWindow.Canvas().Focus)
		newPop := widget.NewModalPopUp(container.NewPadded(newD), m.MainWindow.Canvas())
		newD.OnSubmit = func() {
			d.DisableSubmit()
			go func() {
				success := m.testConnectionAndUpdateDialogText(newD)
				fyne.Do(func() {
					if success {
						// connection is good
						newPop.Hide()
						conn := backend.ServerConnection{
							ServerType:    newD.ServerType,
							Hostname:      newD.Host,
							AltHostname:   newD.AltHost,
							Username:      newD.Username,
							LegacyAuth:    newD.LegacyAuth,
							SkipSSLVerify: newD.SkipSSLVerify,
						}
						server := m.App.ServerManager.AddServer(newD.Nickname, conn)
						m.trySetPasswordAndConnectToServer(server, newD.Password)
						m.doModalClosed()
					}
					d.EnableSubmit()
				})
			}()
		}
		newD.OnCancel = func() {
			newPop.Hide()
			pop.Show()
		}
		newPop.Show()
	}
	d.OnDeleteServer = func(server *backend.ServerConfig) {
		pop.Hide()
		dialog.ShowConfirm(lang.L("Confirm Delete Server"),
			fmt.Sprintf(lang.L("Are you sure you want to delete the server")+" %q?", server.Nickname),
			func(ok bool) {
				if ok {
					m.App.ServerManager.DeleteServer(server.ID)
					m.App.DeleteServerCacheDir(server.ID)
					d.SetServers(m.App.Config.Servers)
				}
				if len(m.App.Config.Servers) == 0 {
					m.PromptForFirstServer()
				} else {
					pop.Show()
				}
			}, m.MainWindow)
	}
	m.haveModal = true
	pop.Show()
}

func (c *Controller) trySetPasswordAndConnectToServer(server *backend.ServerConfig, password string) error {
	if err := c.App.ServerManager.SetServerPassword(server, password); err != nil {
		log.Printf("error setting keyring credentials: %v", err)
		// Don't return an error; fall back to just using the password in-memory
		// User will need to log in with the password on subsequent runs.
	}
	return c.tryConnectToServer(context.Background(), server, password)
}

// try to connect to the given server, with the configured timeout added to the context
func (c *Controller) tryConnectToServer(ctx context.Context, server *backend.ServerConfig, password string) error {
	timeout := time.Duration(c.App.Config.Application.RequestTimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := c.App.ServerManager.TestConnectionAndAuth(ctx, server.ServerConnection, password); err != nil {
		return err
	}
	if err := c.App.ServerManager.ConnectToServer(server, password); err != nil {
		log.Printf("error connecting to server: %v", err)
		return err
	}
	return nil
}

// should be called from goroutine
func (c *Controller) testConnectionAndUpdateDialogText(dlg *dialogs.AddEditServerDialog) bool {
	fyne.Do(func() { dlg.SetInfoText(lang.L("Testing connection") + "...") })
	conn := backend.ServerConnection{
		ServerType:    dlg.ServerType,
		Hostname:      dlg.Host,
		AltHostname:   dlg.AltHost,
		Username:      dlg.Username,
		LegacyAuth:    dlg.LegacyAuth,
		SkipSSLVerify: dlg.SkipSSLVerify,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.App.ServerManager.TestConnectionAndAuth(ctx, conn, dlg.Password)
	if err == backend.ErrUnreachable {
		fyne.Do(func() {
			dlg.SetErrorText(lang.L("Could not reach server") + fmt.Sprintf(" (%s?)", lang.L("wrong URL")))
		})
		return false
	} else if err != nil {
		fyne.Do(func() {
			dlg.SetErrorText(lang.L("Authentication failed") + fmt.Sprintf(" (%s)", lang.L("wrong username/password")))
		})
		return false
	}
	return true
}
