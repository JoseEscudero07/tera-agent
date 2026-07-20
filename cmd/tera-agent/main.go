// Command tera-agent is the entry point of the Tera Agent.
//
// Commands:
//
//	tera-agent printers                 List installed printers
//	tera-agent print   --printer NAME --file F [flags]   Print a document (PDF/img/text)
//	tera-agent raw     --printer NAME --file F           Send bytes unmodified
//	tera-agent text    --printer NAME --text "..."       Print text (ESC/POS)
//	tera-agent run     [--config config.yaml]            Run as a resident agent
//
// main contains no business logic; it wires flags to the print engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/teraerp/tera-agent/internal/adapters/api/httpapi"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/ui"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
	"github.com/teraerp/tera-agent/internal/infra/di"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		return
	}
	switch args[0] {
	case "printers":
		exitOn(cmdPrinters(args[1:]))
	case "print":
		exitOn(cmdPrint(args[1:]))
	case "raw":
		exitOn(cmdRaw(args[1:]))
	case "text":
		exitOn(cmdText(args[1:]))
	case "run":
		cmdRun(args[1:])
	case "serve":
		cmdServe(args[1:])
	case "ui":
		cmdUI(args[1:])
	case "service":
		exitOn(cmdService(args[1:]))
	case "version", "--version", "-v":
		fmt.Println("tera-agent", di.AgentVersion)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "tera-agent: unknown command %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
}

// cmdPrinters lists installed printers with system, status and detected type.
func cmdPrinters(args []string) error {
	fs := flag.NewFlagSet("printers", flag.ExitOnError)
	level := fs.String("log", "warn", "log level (debug|info|warn|error)")
	_ = fs.Parse(args)

	log := logger.New(*level)
	_, _, disc := di.BuildPrinting(log, di.PrintOptions{})
	printers, err := disc.List(context.Background())
	if err != nil {
		return err
	}
	if len(printers) == 0 {
		fmt.Println("No printers found.")
		return nil
	}
	fmt.Printf("%-28s %-8s %-10s %s\n", "NAME", "SYSTEM", "STATUS", "TYPE")
	for _, p := range printers {
		fmt.Printf("%-28s %-8s %-10s %s\n", p.Name, di.OSName(), p.Status, p.Driver)
	}
	return nil
}

// cmdPrint prints a document (PDF/image/text) through the print engine.
func cmdPrint(args []string) error {
	fs := flag.NewFlagSet("print", flag.ExitOnError)
	printer := fs.String("printer", "", "target printer id (required)")
	file := fs.String("file", "", "file to print (default: stdin)")
	format := fs.String("format", "", "source format override (pdf|png|jpeg|text)")
	paper := fs.Int("paper", 80, "thermal paper width in mm (58|80)")
	width := fs.Int("width", 0, "printer width in dots (overrides --paper)")
	density := fs.Int("density", 3, "print density 1..5 (3 = normal)")
	cut := fs.Bool("cut", true, "cut paper at the end")
	drawer := fs.Bool("drawer", false, "kick the cash drawer")
	out := fs.String("out", "", "write encoded output to this file (dry run, no printing)")
	level := fs.String("log", "info", "log level")
	_ = fs.Parse(args)

	data, err := readInput(*file)
	if err != nil {
		return err
	}
	return runEngine(engineArgs{
		printer: *printer,
		format:  resolveFormat(*format, *file),
		data:    data,
		width:   resolveWidth(*paper, *width),
		density: *density,
		cut:     *cut,
		drawer:  *drawer,
		out:     *out,
		level:   *level,
	})
}

// cmdText prints plain text as ESC/POS.
func cmdText(args []string) error {
	fs := flag.NewFlagSet("text", flag.ExitOnError)
	printer := fs.String("printer", "", "target printer id (required)")
	text := fs.String("text", "", "text to print")
	file := fs.String("file", "", "text file to print (alternative to --text)")
	paper := fs.Int("paper", 80, "thermal paper width in mm (58|80)")
	width := fs.Int("width", 0, "printer width in dots (overrides --paper)")
	cut := fs.Bool("cut", true, "cut paper at the end")
	drawer := fs.Bool("drawer", false, "kick the cash drawer")
	out := fs.String("out", "", "write encoded output to this file (dry run)")
	level := fs.String("log", "info", "log level")
	_ = fs.Parse(args)

	var data []byte
	switch {
	case *text != "":
		data = []byte(*text)
	case *file != "":
		b, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		data = b
	default:
		return fmt.Errorf("text: provide --text or --file")
	}

	return runEngine(engineArgs{
		printer: *printer,
		format:  dp.FormatText,
		data:    data,
		width:   resolveWidth(*paper, *width),
		density: 3,
		cut:     *cut,
		drawer:  *drawer,
		out:     *out,
		level:   *level,
	})
}

// cmdRaw sends bytes to a printer unmodified.
func cmdRaw(args []string) error {
	fs := flag.NewFlagSet("raw", flag.ExitOnError)
	printer := fs.String("printer", "", "target printer id (required)")
	file := fs.String("file", "", "file with raw bytes (default: stdin)")
	level := fs.String("log", "info", "log level")
	_ = fs.Parse(args)

	if *printer == "" {
		return fmt.Errorf("raw: --printer is required")
	}
	data, err := readInput(*file)
	if err != nil {
		return err
	}
	return di.RawDriver(logger.New(*level)).Send(context.Background(), *printer, data)
}

// cmdRun starts the resident agent (local mode until a backend is configured).
// When launched by the Windows Service Control Manager it runs as a service.
func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to the configuration file")
	tray := fs.Bool("tray", false, "show a system-tray icon (Windows)")
	_ = fs.Parse(args)

	app, err := di.Build(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tera-agent: startup failed:", err)
		os.Exit(1)
	}
	app.Machine.Subscribe(func(from, to agent.State) {
		app.Log.Info("state changed", "from", string(from), "to", string(to))
	})

	// Windows service: the SCM controls the lifetime.
	if isWindowsService() {
		ctx, cancel := context.WithCancel(context.Background())
		if err := runWindowsService(func() { runLoop(ctx, app) }, cancel); err != nil {
			app.Log.Error("service", "err", err)
			os.Exit(1)
		}
		return
	}

	// Interactive: Ctrl-C / SIGTERM stop the agent.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *tray && traySupported() {
		runWithTray(ctx, stop, app)
		return
	}
	runLoop(ctx, app)
}

// runLoop runs the agent until ctx is cancelled.
func runLoop(ctx context.Context, app *di.App) {
	if app.LocalMode {
		transport := app.NewTransport()
		if err := transport.Connect(ctx); err != nil {
			app.Log.Error("transport", "err", err)
			return
		}
		defer transport.Close()

		if app.HTTPAddr != "" {
			app.Log.Info("tera-agent running (local mode) with HTTP print service")
			srv := httpapi.New(app.Engine, app.Profiles, app.Discovery, app.Log, app.HTTPToken)
			if err := srv.Run(ctx, app.HTTPAddr); err != nil {
				app.Log.Error("http service", "err", err)
			}
			return
		}
		app.Log.Info("tera-agent running (local mode). Printing available via CLI; backend integration pending.")
		<-ctx.Done()
		return
	}
	if err := app.Lifecycle.Run(ctx); err != nil {
		app.Log.Error("agent stopped", "err", err)
	}
}

// cmdServe starts only the local HTTP print service (Django integration/testing).
func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:9100", "listen address (use 0.0.0.0:9100 for LAN)")
	token := fs.String("token", "", "require 'Authorization: Bearer <token>' when set")
	density := fs.Int("density", 3, "print density 1..5")
	level := fs.String("log", "info", "log level")
	_ = fs.Parse(args)

	log := logger.New(*level)
	engine, profiles, disc := di.BuildPrinting(log, di.PrintOptions{Density: *density})
	srv := httpapi.New(engine, profiles, disc, log, *token)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx, *addr); err != nil {
		log.Error("http service", "err", err)
		os.Exit(1)
	}
}

// cmdUI runs the agent and serves the local desktop web UI, opening it in a
// browser window.
func cmdUI(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to the configuration file")
	addr := fs.String("addr", "127.0.0.1:9180", "UI listen address")
	noOpen := fs.Bool("no-open", false, "do not open the browser automatically")
	_ = fs.Parse(args)

	app, err := di.Build(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tera-agent: startup failed:", err)
		os.Exit(1)
	}
	app.Machine.Subscribe(func(from, to agent.State) {
		app.Log.Info("state changed", "from", string(from), "to", string(to))
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runLoop(ctx, app) // connect to backend (or local mode) in the background

	srv := ui.New(ui.Deps{
		Machine:    app.Machine,
		Info:       app.Info,
		Discovery:  app.Discovery,
		Engine:     app.Engine,
		Profiles:   app.Profiles,
		Cfg:        app.Cfg,
		Version:    di.AgentVersion,
		DataDir:    app.DataDir,
		Log:        app.Log,
		SaveConfig: app.SaveConfig,
	})
	if !*noOpen {
		go openBrowser("http://" + *addr)
	}
	if err := srv.Run(ctx, *addr); err != nil {
		app.Log.Error("ui", "err", err)
		os.Exit(1)
	}
}

// openBrowser opens url in the default browser (best effort).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// cmdService manages the Windows service (install|uninstall|start|stop).
func cmdService(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("service: expected install|uninstall|start|stop")
	}
	action := args[0]
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	configPath := fs.String("config", `C:\ProgramData\TeraAgent\config.yaml`, "config path for the service")
	_ = fs.Parse(args[1:])
	return controlService(action, *configPath)
}

// engineArgs bundles the parameters for a print through the engine.
type engineArgs struct {
	printer string
	format  dp.SourceFormat
	data    []byte
	width   int
	density int
	cut     bool
	drawer  bool
	out     string
	level   string
}

func runEngine(a engineArgs) error {
	if a.printer == "" {
		return fmt.Errorf("print: --printer is required")
	}
	log := logger.New(a.level)
	engine, profiles, _ := di.BuildPrinting(log, di.PrintOptions{Density: a.density, OutFile: a.out})

	// The profile normally comes from the ERP; here we build it from flags so the
	// engine can run without a backend.
	profiles.Set(dp.PrinterProfile{
		PrinterID:      a.printer,
		NativeFormats:  []dp.DeviceFormat{dp.DeviceESCPOS},
		WidthDots:      a.width,
		DPI:            203,
		SupportsCut:    true,
		SupportsDrawer: true,
	})

	return engine.Print(context.Background(), dp.PrintJob{
		PrinterID: a.printer,
		Format:    a.format,
		Content:   a.data,
		Options:   dp.Options{Copies: 1, Cut: a.cut, OpenDrawer: a.drawer},
	})
}

// readInput reads a file, or stdin when file is empty.
func readInput(file string) ([]byte, error) {
	if file != "" {
		return os.ReadFile(file)
	}
	return io.ReadAll(os.Stdin)
}

// resolveWidth converts a paper size (mm) to dots, honoring an explicit override.
func resolveWidth(paperMM, widthDots int) int {
	if widthDots > 0 {
		return widthDots
	}
	if paperMM == 58 {
		return 384 // 58mm @203dpi printable
	}
	return 576 // 80mm @203dpi printable
}

// resolveFormat picks the source format from an explicit flag or the file name.
func resolveFormat(explicit, file string) dp.SourceFormat {
	switch strings.ToLower(explicit) {
	case "pdf":
		return dp.FormatPDF
	case "png":
		return dp.FormatPNG
	case "jpeg", "jpg":
		return dp.FormatJPEG
	case "text", "txt":
		return dp.FormatText
	}
	switch strings.ToLower(filepath.Ext(file)) {
	case ".pdf":
		return dp.FormatPDF
	case ".png":
		return dp.FormatPNG
	case ".jpg", ".jpeg":
		return dp.FormatJPEG
	case ".txt":
		return dp.FormatText
	default:
		return dp.FormatPDF
	}
}

func usage() {
	fmt.Print(`tera-agent — ERP printing & device agent

Commands:
  printers                                   List installed printers
  print  --printer NAME --file F [flags]     Print PDF/image/text (PDF -> ESC/POS)
  raw    --printer NAME --file F             Send bytes unmodified
  text   --printer NAME --text "..."         Print text as ESC/POS
  run    [--config config.yaml] [--tray]     Run as a resident agent (local mode)
  ui     [--config config.yaml] [--addr ..]  Run the agent + desktop web UI
  serve  [--addr 127.0.0.1:9100] [--token X] Start the HTTP print web service
  service install|uninstall|start|stop       Manage the Windows service

Common print flags:
  --paper 58|80     Thermal width in mm (default 80)
  --width N         Width in dots (overrides --paper: 58mm=384, 80mm=576)
  --density 1..5    Print density (default 3)
  --cut --drawer    Cut paper / kick cash drawer
  --out FILE        Dry run: write encoded bytes to FILE (no printing)

Examples:
  tera-agent printers
  tera-agent print --printer XP-80 --file factura.pdf
  tera-agent raw   --printer XP-80 --file ticket.bin
  tera-agent text  --printer XP-80 --text "Hola mundo"
`)
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "tera-agent:", err)
		os.Exit(1)
	}
}
