// Package ui hosts the graphical interface: tray icon, status view (bound to the
// agent state machine via an Observer), configuration, logs, printer list and
// Agent info. It consumes core interfaces only and contains no business or
// printing logic. Owner: UI Engineer.
//
// It must not import adapters for printing, communication or device directly.
package ui
