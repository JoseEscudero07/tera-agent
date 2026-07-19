//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "TeraAgent"
	serviceDisplay = "Tera Agent"
	serviceDesc    = "Tera Agent - ERP printing & device agent"
)

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
		}, "run", "--config", configPath)
		if err != nil {
			return err
		}
		defer s.Close()
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
