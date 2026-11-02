package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"go.bug.st/serial"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const defaultPort = "/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00"

func serialMode() *serial.Mode {
	return &serial.Mode{BaudRate: 115200, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit, InitialStatusBits: &serial.ModemOutputBits{DTR: false, RTS: false}}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("embeddeck", flag.ContinueOnError)
	fs.SetOutput(out)
	port := fs.String("port", defaultPort, "USB CDC serial port (stable by-id path recommended)")
	max := fs.Int("max-workspace", 10, "highest workspace number (1..10000)")
	probe := fs.Bool("probe", false, "read actual Hyprland state once; never open serial or dispatch")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *max < 1 || *max > 10000 {
		return errors.New("max-workspace must be 1..10000")
	}
	if *port == "" {
		return errors.New("port must not be empty")
	}
	backend := Hyprland{}
	c := &Controller{Backend: backend, Max: *max}
	if *probe {
		s := c.Refresh(ctx)
		encoder := json.NewEncoder(out)
		encoder.SetEscapeHTML(false)
		if e := encoder.Encode(s); e != nil {
			return e
		}
		if !s.Connected {
			return errors.New("hyprland unavailable")
		}
		return nil
	}
	events := make(chan struct{}, 1)
	watchDone := make(chan struct{})
	watchCtx, stop := context.WithCancel(ctx)
	defer func() { stop(); <-watchDone }()
	go func() {
		defer close(watchDone)
		watchEvents(watchCtx, func(ctx context.Context) (string, error) {
			p, e := discover(ctx)
			return filepath.Join(filepath.Dir(p), ".socket2.sock"), e
		}, events)
	}()
	for ctx.Err() == nil {
		p, e := openSerialPort(*port)
		if e != nil {
			log.Printf("serial unavailable: %v", e)
		} else {
			log.Printf("serial connected: %s", *port)
			e = serveSession(ctx, p, c, events, time.Second)
			if ctx.Err() == nil {
				log.Printf("serial disconnected: %v", e)
			}
		}
		if !pause(ctx, time.Second) {
			break
		}
	}
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e := run(ctx, os.Args[1:], os.Stdout); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return
		}
		log.Print(e)
		os.Exit(1)
	}
}
