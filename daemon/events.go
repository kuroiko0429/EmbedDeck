package main

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"
)

func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func watchEvents(ctx context.Context, path func(context.Context) (string, error), events chan<- struct{}) {
	for ctx.Err() == nil {
		p, e := path(ctx)
		if e != nil {
			if !pause(ctx, time.Second) {
				return
			}
			continue
		}
		conn, e := (&net.Dialer{Timeout: 500 * time.Millisecond}).DialContext(ctx, "unix", p)
		if e != nil {
			if !pause(ctx, time.Second) {
				return
			}
			continue
		}
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
			}
		}()
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024), 64*1024)
		for scanner.Scan() {
			name, _, _ := strings.Cut(scanner.Text(), ">>")
			switch name {
			case "workspace", "workspacev2", "focusedmon", "focusedmonv2", "createworkspace", "createworkspacev2", "destroyworkspace", "destroyworkspacev2", "renameworkspace", "moveworkspace", "moveworkspacev2":
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
		close(done)
		conn.Close()
		if !pause(ctx, time.Second) {
			return
		}
	}
}
