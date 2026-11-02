//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// linuxSerialPort keeps its descriptor nonblocking and registered with Go's
// runtime poller. Do not replace this with a blocking serial-library Write:
// closing a blocking Linux tty fd need not interrupt an in-flight syscall.
type linuxSerialPort struct{ file *os.File }

func openSerialPort(name string) (io.ReadWriteCloser, error) {
	p, err := openLinuxSerial(name, false)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// allowNoModem is exclusively for private PTY tests, whose modem ioctls return
// ENOTTY. Production must successfully clear both DTR and RTS before use.
func openLinuxSerial(name string, allowNoModem bool) (*linuxSerialPort, error) {
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open serial: %w", err)
	}
	file := os.NewFile(uintptr(fd), name)
	// NewFile registers an already-nonblocking fd with netpoll. Fail closed
	// if the device cannot be polled: a Write must never fall back to a
	// blocking syscall that a concurrent Close cannot interrupt.
	if err = file.SetWriteDeadline(time.Time{}); err != nil {
		return nil, errors.Join(fmt.Errorf("serial write deadlines: %w", err), file.Close())
	}
	p := &linuxSerialPort{file: file}
	fail := func(stage string, err error) (*linuxSerialPort, error) {
		return nil, errors.Join(fmt.Errorf("%s: %w", stage, err), p.Close())
	}
	if err = unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		return fail("serial exclusive access", err)
	}
	settings, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return fail("get serial termios", err)
	}
	// Raw 115200 8N1, receiver enabled, no hardware/software flow control,
	// no HUPCL, and no input/output translations or canonical/echo processing.
	settings.Iflag = 0
	settings.Oflag = 0
	settings.Lflag = 0
	settings.Cflag = unix.B115200 | unix.CS8 | unix.CLOCAL | unix.CREAD
	settings.Ispeed = unix.B115200
	settings.Ospeed = unix.B115200
	settings.Cc[unix.VMIN] = 1
	settings.Cc[unix.VTIME] = 0
	if err = unix.IoctlSetTermios(fd, unix.TCSETS, settings); err != nil {
		return fail("set serial termios", err)
	}
	bits, err := unix.IoctlGetInt(fd, unix.TIOCMGET)
	if err != nil {
		if !allowNoModem || !errors.Is(err, unix.ENOTTY) {
			return fail("get serial modem bits", err)
		}
	} else if err = unix.IoctlSetPointerInt(fd, unix.TIOCMSET, bits&^(unix.TIOCM_DTR|unix.TIOCM_RTS)); err != nil {
		return fail("clear serial DTR/RTS", err)
	}
	return p, nil
}

func (p *linuxSerialPort) Read(b []byte) (int, error) { return p.file.Read(b) }

// serveSession owns the single writer. Each Write has its own one-second
// deadline; its existing watchdog bounds the entire frame across short writes.
func (p *linuxSerialPort) Write(b []byte) (int, error) {
	if err := p.file.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return 0, err
	}
	return p.file.Write(b)
}

func (p *linuxSerialPort) Close() error {
	// RawConn.Control pins the fd against concurrent Close without waiting for
	// Read/Write's locks. Clear tty exclusivity while the fd is still valid;
	// a PTY master (or another existing opener) may otherwise keep it alive.
	var ioctlErr error
	raw, err := p.file.SyscallConn()
	if err == nil {
		err = raw.Control(func(fd uintptr) { ioctlErr = unix.IoctlSetInt(int(fd), unix.TIOCNXCL, 0) })
	}
	return errors.Join(err, ioctlErr, p.file.Close())
}
