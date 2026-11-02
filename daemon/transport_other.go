//go:build !linux

package main

import (
	"io"

	"go.bug.st/serial"
)

// The deadline and Close-interruption guarantee is Linux-only. Other platforms
// retain the existing serial implementation and its platform-specific behavior.
func openSerialPort(name string) (io.ReadWriteCloser, error) {
	return serial.Open(name, serialMode())
}
