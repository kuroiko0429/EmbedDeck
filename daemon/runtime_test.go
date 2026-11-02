package main

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeInstance(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, "hypr", name)
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, ".socket.sock")
	l, e := net.Listen("unix", p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			buf := make([]byte, 128)
			c.Read(buf)
			io.WriteString(c, `{"id":2}`)
			c.Close()
		}
	}()
	return p
}
func TestDiscoveryRefusesAmbiguity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	p := fakeInstance(t, root, "a")
	got, e := discover(context.Background())
	if e != nil || got != p {
		t.Fatal(got, e)
	}
	fakeInstance(t, root, "b")
	if _, e = discover(context.Background()); e == nil {
		t.Fatal("accepted ambiguous instances")
	}
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "a")
	if got, e = discover(context.Background()); e != nil || got != p {
		t.Fatal(got, e)
	}
}
func TestWatchEventsReconnectAndCancel(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".socket2.sock")
	l, e := net.Listen("unix", p)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() { watchEvents(ctx, func(context.Context) (string, error) { return p, nil }, events); close(done) }()
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		io.WriteString(c, "workspacev2>>2,two\n")
		c.Close()
	}()
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
}
