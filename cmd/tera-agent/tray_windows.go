//go:build windows

package main

import (
	"context"
	_ "embed"
	"os/exec"
	"strconv"
	"time"

	"fyne.io/systray"

	"github.com/teraerp/tera-agent/internal/adapters/ui"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
	"github.com/teraerp/tera-agent/internal/infra/di"
)

//go:embed assets/tray.ico
var trayIcon []byte

// maxPrinterSlots caps how many printers the tray submenu shows. systray has no
// way to clear a submenu, so we pre-allocate a fixed pool of slots and toggle
// their visibility on each refresh.
const maxPrinterSlots = 8

func traySupported() bool { return true }

// runWithTray runs the agent while showing a system-tray icon whose menu mirrors
// the desktop panel: connection state, company/equipment, last sync, detected
// printers, last print, and the actions (test, history, config, panel, quit).
func runWithTray(ctx context.Context, stop func(), app *di.App) {
	onReady := func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Tera Agent")
		systray.SetTooltip("Tera Agent")

		// --- Cabecera e identidad (solo lectura) ---
		mHeader := systray.AddMenuItem("TERA AGENT", "")
		mHeader.Disable()
		mStatus := systray.AddMenuItem("● —", "Estado de conexión")
		mStatus.Disable()
		mEmpresa := systray.AddMenuItem("Empresa: —", "")
		mEmpresa.Disable()
		mEquipo := systray.AddMenuItem("Equipo: —", "")
		mEquipo.Disable()
		mSync := systray.AddMenuItem("Última sincronización: —", "")
		mSync.Disable()

		// --- Impresoras (submenú con slots fijos) ---
		systray.AddSeparator()
		mPrinters := systray.AddMenuItem("Impresoras", "Impresoras detectadas")
		printerSlots := make([]*systray.MenuItem, maxPrinterSlots)
		for i := range printerSlots {
			printerSlots[i] = mPrinters.AddSubMenuItem("", "")
			printerSlots[i].Disable()
			printerSlots[i].Hide()
		}

		// --- Última impresión ---
		systray.AddSeparator()
		mLast := systray.AddMenuItem("Última impresión: —", "")
		mLast.Disable()

		// --- Acciones ---
		systray.AddSeparator()
		mTest := systray.AddMenuItem("Probar", "Imprimir un ticket de prueba")
		mHist := systray.AddMenuItem("Historial", "Abrir el historial en el panel")
		mConfig := systray.AddMenuItem("Configuración", "Abrir la configuración en el panel")
		mPanel := systray.AddMenuItem("Abrir panel", "Abrir el panel de Tera Agent")
		mData := systray.AddMenuItem("Abrir carpeta de datos", "Logs y estado")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Salir", "Detener el agente")

		// Estado en vivo (mismo mapeo de colores/etiquetas que el panel web).
		applyState := func(st agent.State) {
			dot, label := trayStatus(st)
			mStatus.SetTitle(dot + " " + label)
			systray.SetTooltip("Tera Agent — " + label)
		}
		applyState(app.Machine.Current())
		app.Machine.Subscribe(func(_, to agent.State) { applyState(to) })

		// Refresco periódico de identidad e impresoras.
		refresh := func() {
			id, ts := app.Info.Snapshot()
			mEmpresa.SetTitle("Empresa: " + orDash(id.Empresa))
			mEquipo.SetTitle("Equipo: " + orDash(firstNonEmpty(id.Equipo, app.Cfg.AgentID)))
			mSync.SetTitle("Última sincronización: " + humanSinceTray(ts))

			list, _ := app.Discovery.List(ctx)
			for i, slot := range printerSlots {
				switch {
				case i < len(list):
					mark := "○"
					if list[i].Status == dp.StatusReady {
						mark = "✓"
					}
					slot.SetTitle(mark + " " + list[i].Name)
					slot.Show()
				case i == 0 && len(list) == 0:
					slot.SetTitle("Sin impresoras")
					slot.Show()
				default:
					slot.Hide()
				}
			}
		}
		refresh()

		// Ejecuta el agente y el panel web en segundo plano.
		go runLoop(ctx, app)
		go func() { _ = buildUIServer(app).Run(ctx, uiAddr) }()

		go func() {
			ticker := time.NewTicker(4 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-ticker.C:
					refresh()
				case <-mTest.ClickedCh:
					go trayTestPrint(ctx, app)
				case <-mHist.ClickedCh:
					openBrowser("http://" + uiAddr + "/?view=historial")
				case <-mConfig.ClickedCh:
					openBrowser("http://" + uiAddr + "/?view=config")
				case <-mPanel.ClickedCh:
					openBrowser("http://" + uiAddr)
				case <-mData.ClickedCh:
					if app.DataDir != "" {
						_ = exec.Command("explorer", app.DataDir).Start()
					}
				case <-mQuit.ClickedCh:
					stop()
					systray.Quit()
					return
				}
			}
		}()
	}
	systray.Run(onReady, stop)
}

// trayTestPrint prints a test page on the default (or first discovered)
// printer, reusing the same routine as the web panel's "Probar" button.
func trayTestPrint(ctx context.Context, app *di.App) {
	printer := app.Cfg.DefaultPrinter
	if printer == "" {
		if ps, _ := app.Discovery.List(ctx); len(ps) > 0 {
			printer = ps[0].Name
		}
	}
	if printer == "" {
		app.Log.Warn("tray: no hay impresora para la prueba")
		return
	}
	if err := ui.PrintTestPage(ctx, app.Engine, app.Profiles, app.Cfg, printer); err != nil {
		app.Log.Error("tray: prueba de impresión falló", "err", err)
	}
}

// trayStatus maps a lifecycle state to a coloured dot + Spanish label, matching
// the web panel's status vocabulary.
func trayStatus(st agent.State) (dot, label string) {
	switch st {
	case agent.StateConnected:
		return "🟢", "Conectado"
	case agent.StateStarting:
		return "🟡", "Iniciando"
	case agent.StateConnecting:
		return "🟡", "Conectando"
	case agent.StateAuthenticating:
		return "🟡", "Autenticando"
	case agent.StateRegistering:
		return "🟡", "Registrando"
	case agent.StateReconnecting:
		return "🟡", "Reconectando"
	case agent.StateDisconnected:
		return "🔴", "Desconectado"
	case agent.StateOffline:
		return "🔴", "Sin conexión"
	case agent.StateError:
		return "🔴", "Error"
	case agent.StateShuttingDown:
		return "🔴", "Cerrando"
	default:
		return "●", string(st)
	}
}

// firstNonEmpty returns a if non-empty, else b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func humanSinceTray(ts time.Time) string {
	if ts.IsZero() {
		return "—"
	}
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		return "hace " + strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return "hace " + strconv.Itoa(int(d.Minutes())) + "m"
	default:
		return "hace " + strconv.Itoa(int(d.Hours())) + "h"
	}
}
