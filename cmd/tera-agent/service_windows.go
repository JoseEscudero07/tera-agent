//go:build windows

package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "TeraAgent"
	serviceDisplay = "Tera Agent"
	serviceDesc    = "Tera Agent - ERP printing & device agent"
)

// spoolerService es la dependencia dura del Agent en Windows: sin el spooler de
// impresión no hay impresoras que descubrir ni cola donde encolar. Declararla
// evita el arranque en frío donde el Agent gana la carrera al spooler y arranca
// sin ver ninguna impresora.
const spoolerService = "Spooler"

// resetPeriodSeconds es la ventana tras la que el SCM olvida los fallos
// acumulados. 24h significa: si el Agent aguanta un día entero, el siguiente
// fallo vuelve a contar como el primero (y se reintenta rápido otra vez).
const resetPeriodSeconds = 86400

// recoveryActions define qué hace el SCM cuando el Agent muere. Un POS debe
// volver a imprimir solo, sin que nadie reinicie el equipo: reintentos con
// espera creciente y, a partir del tercero, cada minuto indefinidamente.
func recoveryActions() []mgr.RecoveryAction {
	return []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}
}

// isWindowsService reports whether the process was started by the Windows SCM.
func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// teraService adapts the agent run loop to the Windows service control protocol.
type teraService struct {
	run  func()
	stop func()
}

func (s *teraService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	go s.run()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			s.stop()
			return false, 0
		}
	}
	return false, 0
}

// runWindowsService runs the agent under the SCM. run should block until stop
// is called.
func runWindowsService(run, stop func()) error {
	return svc.Run(serviceName, &teraService{run: run, stop: stop})
}

// controlService installs/uninstalls/starts/stops the Windows service.
func controlService(action, configPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()

	switch action {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if s, err := m.OpenService(serviceName); err == nil {
			s.Close()
			return fmt.Errorf("service %q already installed", serviceName)
		}
		s, err := m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: serviceDisplay,
			Description: serviceDesc,
			StartType:   mgr.StartAutomatic,
			// El spooler tarda en estar listo tras el boot; retrasar el arranque
			// evita descubrir cero impresoras en el primer ciclo.
			DelayedAutoStart: true,
			Dependencies:     []string{spoolerService},
		}, "run", "--config", configPath)
		if err != nil {
			return err
		}
		defer s.Close()

		// Auto-recuperación. No es fatal si falla (el servicio ya está creado y
		// funcional), pero sí hay que avisar: sin esto un crash deja el POS sin
		// imprimir hasta que alguien reinicie a mano.
		if err := s.SetRecoveryActions(recoveryActions(), resetPeriodSeconds); err != nil {
			fmt.Fprintf(os.Stderr, "aviso: no se pudieron configurar los reintentos automáticos: %v\n", err)
			return nil
		}
		// Por defecto el SCM solo reacciona a crashes; un exit(1) limpio no
		// cuenta como fallo. Para un agente residente cualquier salida no
		// solicitada es un fallo y debe reintentarse.
		if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
			fmt.Fprintf(os.Stderr, "aviso: no se pudo activar el reintento ante salidas no-crash: %v\n", err)
		}
		return nil

	case "uninstall":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return fmt.Errorf("service %q not installed", serviceName)
		}
		defer s.Close()
		return s.Delete()

	case "start":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return err
		}
		defer s.Close()
		return s.Start()

	case "stop":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return err
		}
		defer s.Close()
		_, err = s.Control(svc.Stop)
		return err

	default:
		return fmt.Errorf("unknown service action %q (use install|uninstall|start|stop)", action)
	}
}
