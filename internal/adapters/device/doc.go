// Package device hosts the hardware drivers that implement domain/device.Driver:
// barcode scanner, cash drawer, scale, customer display, serial, USB HID and
// RFID. Each device is an interchangeable driver; platform-specific files use
// build tags. Owner: Device Engineer.
//
// It must not import comms, ui or printing packages.
package device
