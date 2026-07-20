//go:build windows

package main

import (
	"context"
	_ "embed"
	"os/exec"

	"fyne.io/systray"

	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/infra/di"
)

//go:embed assets/tray.ico
var trayIcon []byte

func traySupported() bool { return true }

// runWithTray runs the agent while showing a system-tray icon whose menu
// reflects the connection state and lets the user open the data folder or quit.
func runWithTray(ctx context.Context, stop func(), app *di.App) {
	onReady := func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Tera Agent")
		systray.SetTooltip("Tera Agent")

		mStatus := systray.AddMenuItem("Estado: iniciando…", "Estado del agente")
		mStatus.Disable()
		systray.AddSeparator()
		mPanel := systray.AddMenuItem("Abrir panel", "Abrir el panel de Tera Agent")
		mData := systray.AddMenuItem("Abrir carpeta de datos", "Logs y estado")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Salir", "Detener el agente")

		app.Machine.Subscribe(func(_, to agent.State) {
			mStatus.SetTitle("Estado: " + string(to))
			systray.SetTooltip("Tera Agent — " + string(to))
		})

		// Run the agent and the desktop panel (web UI) in the background.
		go runLoop(ctx, app)
		go func() { _ = buildUIServer(app).Run(ctx, uiAddr) }()

		go func() {
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-mQuit.ClickedCh:
					stop()
					systray.Quit()
					return
				case <-mPanel.ClickedCh:
					openBrowser("http://" + uiAddr)
				case <-mData.ClickedCh:
					if app.DataDir != "" {
						_ = exec.Command("explorer", app.DataDir).Start()
					}
				}
			}
		}()
	}
	systray.Run(onReady, stop)
}
