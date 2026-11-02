package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"
)

type Ack struct {
	V     int    `json:"v"`
	Type  string `json:"type"`
	Seq   uint32 `json:"seq"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// A single event-loop owns all writes and controller state. Closing the port
// interrupts reads/writes on cancellation or a bounded write timeout.
func serveSession(ctx context.Context, p io.ReadWriteCloser, c *Controller, events <-chan struct{}, heartbeat time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer p.Close()
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			p.Close()
		case <-closed:
		}
	}()
	frames := make(chan Frame, 32)
	readErr := make(chan error, 1)
	readerDone := make(chan struct{})
	defer func() { cancel(); p.Close(); <-readerDone }()
	go func() {
		defer close(readerDone)
		r := bufio.NewReaderSize(p, 256)
		for {
			b, e := readFrame(r)
			if errors.Is(e, errFrame) {
				continue
			}
			if e != nil {
				readErr <- e
				return
			}
			f, e := decodeFrame(b)
			if e != nil {
				continue
			}
			select {
			case frames <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	write := func(v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if len(b) > 1023 {
			return errors.New("outbound frame too large")
		}
		b = append(b, '\n')
		timer := time.AfterFunc(time.Second, func() { p.Close() })
		defer timer.Stop()
		for len(b) > 0 {
			n, e := p.Write(b)
			if e != nil {
				return e
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			b = b[n:]
		}
		return nil
	}
	state := func() error { return write(c.Refresh(ctx)) }
	if e := state(); e != nil {
		return e
	}
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-readErr:
			return e
		case <-events:
			if e := state(); e != nil {
				return e
			}
		case <-tick.C:
			if e := state(); e != nil {
				return e
			}
		case f := <-frames:
			if f.Type == "status" {
				log.Printf("device state: workspace=%d connected=%t rx=%d", *f.Workspace, *f.Connected, *f.RX)
				continue
			}
			e := c.Handle(ctx, f)
			if f.Seq != nil && !(f.Type == "button" && f.Action == "release") {
				log.Printf("action: seq=%d type=%s button=%s workspace=%d ok=%t error=%v", *f.Seq, f.Type, f.Button, c.State.Workspace, e == nil, e)
				a := Ack{V: 1, Type: "ack", Seq: *f.Seq, OK: e == nil}
				if e != nil {
					a.Error = e.Error()
				}
				if e = write(a); e != nil {
					return e
				}
			}
			if e := state(); e != nil {
				return e
			}
		}
	}
}
