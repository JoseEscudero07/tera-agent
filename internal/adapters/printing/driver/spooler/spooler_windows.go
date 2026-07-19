//go:build windows

// Package spooler prints on Windows by talking to the print spooler directly
// (winspool.drv), sending RAW bytes without going through the graphical driver.
// This is what thermal/ESC/POS and label printers need. Owner: Printing Engineer.
package spooler

import (
	"context"
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/teraerp/tera-agent/internal/app/ports"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

var (
	winspool             = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinter      = winspool.NewProc("OpenPrinterW")
	procStartDocPrinter  = winspool.NewProc("StartDocPrinterW")
	procStartPagePrinter = winspool.NewProc("StartPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procEnumPrinters     = winspool.NewProc("EnumPrintersW")
	procGetPrinter       = winspool.NewProc("GetPrinterW")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

// PRINTER_STATUS_* bit flags (subset) used to map to a domain Status.
const (
	statusError        = 0x00000002
	statusPaperJam     = 0x00000008
	statusPaperOut     = 0x00000010
	statusOffline      = 0x00000080
	statusBusy         = 0x00000200
	statusPrinting     = 0x00000400
	statusNotAvailable = 0x00001000
	statusProcessing   = 0x00004000
)

// docInfo1 mirrors DOC_INFO_1W.
type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

// printerInfo4 mirrors PRINTER_INFO_4W (lightweight enumeration).
type printerInfo4 struct {
	pPrinterName *uint16
	pServerName  *uint16
	attributes   uint32
}

// Driver sends RAW bytes to a Windows printer via the spooler.
type Driver struct {
	log     ports.Logger
	accepts map[dp.DeviceFormat]bool
}

// NewRaw returns a spooler driver that submits pre-formatted device bytes
// (ESC/POS, ZPL, EPL, raw) directly to the printer.
func NewRaw(log ports.Logger) dp.Driver {
	return &Driver{log: log, accepts: map[dp.DeviceFormat]bool{
		dp.DeviceESCPOS: true, dp.DeviceZPL: true, dp.DeviceEPL: true, dp.DeviceRaw: true,
	}}
}

func (d *Driver) Accepts(f dp.DeviceFormat) bool { return d.accepts[f] }

func (d *Driver) Send(_ context.Context, printerID string, data []byte) error {
	if printerID == "" {
		return fmt.Errorf("winspool: empty printer id")
	}
	if len(data) == 0 {
		return fmt.Errorf("winspool: empty data")
	}

	name, err := syscall.UTF16PtrFromString(printerID)
	if err != nil {
		return err
	}
	var h syscall.Handle
	if r, _, e := procOpenPrinter.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return fmt.Errorf("winspool: OpenPrinter %q: %v", printerID, e)
	}
	defer procClosePrinter.Call(uintptr(h))

	docName, _ := syscall.UTF16PtrFromString("Tera Agent RAW")
	dataType, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{pDocName: docName, pDatatype: dataType}
	if job, _, e := procStartDocPrinter.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di))); job == 0 {
		return fmt.Errorf("winspool: StartDocPrinter: %v", e)
	}
	defer procEndDocPrinter.Call(uintptr(h))

	if r, _, e := procStartPagePrinter.Call(uintptr(h)); r == 0 {
		return fmt.Errorf("winspool: StartPagePrinter: %v", e)
	}
	defer procEndPagePrinter.Call(uintptr(h))

	var written uint32
	if r, _, e := procWritePrinter.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	); r == 0 {
		return fmt.Errorf("winspool: WritePrinter: %v", e)
	}
	if int(written) != len(data) {
		return fmt.Errorf("winspool: short write %d/%d bytes", written, len(data))
	}
	d.log.Info("raw print submitted (winspool)", "printer", printerID, "bytes", len(data))
	return nil
}

// Discovery enumerates Windows printers via EnumPrinters (level 4).
type Discovery struct{ log ports.Logger }

// NewDiscovery returns a winspool-based discovery.
func NewDiscovery(log ports.Logger) dp.Discovery { return &Discovery{log: log} }

func (d *Discovery) List(_ context.Context) ([]dp.Printer, error) {
	flags := uintptr(printerEnumLocal | printerEnumConnections)

	var needed, returned uint32
	// First call sizes the buffer.
	procEnumPrinters.Call(flags, 0, 4, 0, 0,
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, nil
	}

	buf := make([]byte, needed)
	if r, _, e := procEnumPrinters.Call(flags, 0, 4,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned))); r == 0 {
		return nil, fmt.Errorf("winspool: EnumPrinters: %v", e)
	}

	stride := unsafe.Sizeof(printerInfo4{})
	printers := make([]dp.Printer, 0, returned)
	for i := uint32(0); i < returned; i++ {
		pi := (*printerInfo4)(unsafe.Pointer(&buf[int(uintptr(i)*stride)]))
		name := utf16PtrToString(pi.pPrinterName)
		status, _ := d.StatusOf(context.Background(), name)
		printers = append(printers, dp.Printer{
			ID: name, Name: name, Driver: "winspool", Status: status,
		})
	}
	return printers, nil
}

// StatusOf queries PRINTER_INFO_6 (a single status DWORD) via GetPrinter.
func (d *Discovery) StatusOf(_ context.Context, printerID string) (dp.Status, error) {
	name, err := syscall.UTF16PtrFromString(printerID)
	if err != nil {
		return dp.StatusUnknown, nil
	}
	var h syscall.Handle
	if r, _, _ := procOpenPrinter.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return dp.StatusUnknown, nil
	}
	defer procClosePrinter.Call(uintptr(h))

	var needed uint32
	// PRINTER_INFO_6 is a single DWORD (dwStatus); size the buffer first.
	procGetPrinter.Call(uintptr(h), 6, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed < 4 {
		return dp.StatusUnknown, nil
	}
	buf := make([]byte, needed)
	if r, _, _ := procGetPrinter.Call(uintptr(h), 6,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed))); r == 0 {
		return dp.StatusUnknown, nil
	}
	return mapStatus(*(*uint32)(unsafe.Pointer(&buf[0]))), nil
}

func mapStatus(s uint32) dp.Status {
	switch {
	case s == 0:
		return dp.StatusReady
	case s&(statusOffline|statusNotAvailable) != 0:
		return dp.StatusOffline
	case s&(statusError|statusPaperJam|statusPaperOut) != 0:
		return dp.StatusError
	case s&(statusPrinting|statusBusy|statusProcessing) != 0:
		return dp.StatusBusy
	default:
		return dp.StatusReady
	}
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var s []uint16
	ptr := unsafe.Pointer(p)
	for {
		c := *(*uint16)(ptr)
		if c == 0 {
			break
		}
		s = append(s, c)
		ptr = unsafe.Pointer(uintptr(ptr) + 2)
	}
	return string(utf16.Decode(s))
}
