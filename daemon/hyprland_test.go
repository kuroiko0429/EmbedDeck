package main

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestHyprlandExactIPC(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".socket.sock")
	l, e := net.Listen("unix", path)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	commands := make(chan string, 3)
	go func() {
		for _, reply := range []string{`{"id":2,"name":"two"}`, `[{"id":2},{"id":1},{"id":-99},{"id":2}]`, `ok`} {
			c, e := l.Accept()
			if e != nil {
				return
			}
			c.SetDeadline(time.Now().Add(time.Second))
			buf := make([]byte, 4096)
			n, _ := c.Read(buf)
			commands <- string(buf[:n])
			io.WriteString(c, reply)
			c.Close()
		}
	}()
	h := Hyprland{Socket: path}
	s, e := h.Snapshot(context.Background())
	if e != nil || !s.Connected || s.Workspace != 2 || s.Name != "two" || len(s.Workspaces) != 2 || s.Workspaces[0] != 1 {
		t.Fatalf("%+v %v", s, e)
	}
	if e = h.Focus(context.Background(), 3); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"j/activeworkspace", "j/workspaces", "dispatch hl.dsp.focus({ workspace = 3 })"} {
		select {
		case got := <-commands:
			if got != want {
				t.Fatalf("%q != %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}
