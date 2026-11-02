package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// All desktop integration tests use private fake Unix sockets, never live IPC.
func fakeDesktop(t *testing.T, reply func(string) string) Hyprland {
	t.Helper()
	dir, err := os.MkdirTemp("", "ed-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "desktop.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(time.Second))
			b := make([]byte, 4096)
			n, _ := conn.Read(b)
			io.WriteString(conn, reply(string(b[:n])))
			conn.Close()
		}
	}()
	return Hyprland{Socket: path}
}

func TestDesktopActualWindowState(t *testing.T) {
	window := `{"address":"0xabc","mapped":true,"hidden":false,"floating":false,"fullscreen":0,"class":"terminal🙂\nabcdefghijklmnopqrstuvwx","size":[900,700],"monitor":3,"workspace":{"id":2}}`
	h := fakeDesktop(t, func(command string) string {
		switch command {
		case "j/activeworkspace":
			return `{"id":2,"name":"two","tiledLayout":"hyprscrolling"}`
		case "j/workspaces":
			return `[{"id":2}]`
		case "j/activewindow":
			return window
		case "j/monitors":
			return `[{"id":3,"width":2400,"height":1600,"scale":1.25,"transform":0,"reserved":[40,30,80,20]}]`
		}
		return "unknown"
	})
	s, err := h.Snapshot(context.Background())
	if err != nil || s.Layout != "hyprscrolling" || s.App != "terminalabcdefghijklmnop" || s.WindowWidth != 900 || s.WidthPercent != 50 || !s.TiledWindow {
		t.Fatal(s, err)
	}
	for _, bad := range []string{
		`{}`, `null`,
		strings.Replace(window, `"mapped":true`, `"mapped":false`, 1),
		strings.Replace(window, `"floating":false,`, ``, 1),
		strings.Replace(window, `"fullscreen":0,`, ``, 1),
		strings.Replace(window, `"hidden":false`, `"hidden":true`, 1),
		strings.Replace(window, `"floating":false`, `"floating":true`, 1),
		strings.Replace(window, `"fullscreen":0`, `"fullscreen":1`, 1),
		strings.Replace(window, `"fullscreen":0`, `"fullscreen":2`, 1),
		strings.Replace(window, `"id":2`, `"id":3`, 1),
		strings.Replace(window, `"size":[900,700]`, `"size":[0,700]`, 1),
		strings.Replace(window, `"address":"0xabc"`, `"address":""`, 1),
	} {
		t.Run(bad, func(t *testing.T) {
			h2 := fakeDesktop(t, func(command string) string {
				if command == "j/activewindow" {
					return bad
				}
				switch command {
				case "j/activeworkspace":
					return `{"id":2,"tiledLayout":"hyprscrolling"}`
				case "j/workspaces":
					return `[]`
				case "j/monitors":
					return `[]`
				}
				return ""
			})
			s, e := h2.Snapshot(context.Background())
			if e != nil || !s.Connected || s.TiledWindow {
				t.Fatal(s, e)
			}
		})
	}
}

func TestDesktopWidthRotationAndClamping(t *testing.T) {
	for _, tc := range []struct {
		width, transform int
		scale            float64
		want             int
	}{{900, 1, 1.25, 50}, {5000, 1, 1.25, 100}, {900, 0, 0, 0}, {100001, 0, 1, 0}} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			h := fakeDesktop(t, func(command string) string {
				switch command {
				case "j/activeworkspace":
					return `{"id":2,"tiledLayout":"hyprscrolling"}`
				case "j/workspaces":
					return `[]`
				case "j/activewindow":
					return fmt.Sprintf(`{"address":"0xabc","mapped":true,"size":[%d,700],"monitor":3,"workspace":{"id":2}}`, tc.width)
				case "j/monitors":
					return fmt.Sprintf(`[{"id":3,"width":1600,"height":2400,"transform":%d,"scale":%g,"reserved":[40,30,80,20]}]`, tc.transform, tc.scale)
				}
				return ""
			})
			s, err := h.Snapshot(context.Background())
			if err != nil || s.WidthPercent != tc.want || s.WindowWidth > 100000 {
				t.Fatal(s, err)
			}
		})
	}
}

func TestLayoutExactIPCAndRejectUntrustedCommand(t *testing.T) {
	commands := make(chan string, 20)
	h := fakeDesktop(t, func(command string) string { commands <- command; return "ok" })
	for _, command := range []string{"focus r", "focus l", "focus u", "focus d", "focusback", "colresize +0.05", "colresize -1.60", "colresize +conf", "colresize -conf", "fit active"} {
		if err := h.Layout(context.Background(), command); err != nil {
			t.Fatal(err)
		}
		if got := <-commands; got != `dispatch hl.dsp.layout("`+command+`")` {
			t.Fatal(got)
		}
	}
	for _, command := range []string{`focus r"); os.execute("bad")`, "colresize 0.05", "colresize +0.00", "colresize +1.65", "colresize +0.051", "fit all", "focus x"} {
		if err := h.Layout(context.Background(), command); err == nil {
			t.Fatal("accepted", command)
		}
	}
	select {
	case command := <-commands:
		t.Fatal("untrusted command sent", command)
	default:
	}
	rejected := fakeDesktop(t, func(string) string { return "error" })
	if err := rejected.Layout(context.Background(), "focus r"); err == nil {
		t.Fatal("accepted rejection")
	}
}

func TestIPCCancelInterruptsRead(t *testing.T) {
	dir, err := os.MkdirTemp("", "ed-cancel-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	done := make(chan error, 1)
	go func() { _, err := request(ctx, path, "j/activewindow"); done <- err }()
	conn := <-accepted
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation accepted")
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("cancel failed to interrupt IPC read")
	}
}
