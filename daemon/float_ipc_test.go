package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func floatWindow() map[string]any {
	return map[string]any{"address": "0xabc", "mapped": true, "hidden": false, "floating": true, "fullscreen": 0, "class": "term", "at": []int{-900, 80}, "size": []int{500, 300}, "monitor": 3, "workspace": map[string]any{"id": 2}}
}
func floatMonitor() map[string]any {
	return map[string]any{"id": 3, "x": -1000, "y": 0, "width": 1200, "height": 800, "scale": 1.25, "transform": 0, "reserved": []int{20, 30, 40, 50}, "focused": true, "activeWorkspace": map[string]any{"id": 2}}
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func floatDesktop(t *testing.T, w, m map[string]any, dispatch func(string) string) Hyprland {
	return fakeDesktop(t, func(cmd string) string {
		switch cmd {
		case "j/activeworkspace":
			return `{"id":2}`
		case "j/workspaces":
			return `[]`
		case "j/activewindow":
			return jsonText(w)
		case "j/clients":
			return "[" + jsonText(w) + "]"
		case "j/monitors":
			return "[" + jsonText(m) + "]"
		}
		return dispatch(cmd)
	})
}
func TestFloatNativeIPCAndActualReadback(t *testing.T) {
	for _, tc := range []struct {
		action string
		delta  int
		want   string
	}{
		{"move_x", 32, `dispatch hl.dsp.window.move({ x = -580, y = 80, relative = false, window = "address:0xabc" })`},
		{"move_y", -32, `dispatch hl.dsp.window.move({ x = -900, y = 30, relative = false, window = "address:0xabc" })`},
		{"resize_w", 32, `dispatch hl.dsp.window.resize({ x = 820, y = 300, relative = false, window = "address:0xabc" })`},
		{"resize_h", 32, `dispatch hl.dsp.window.resize({ x = 500, y = 510, relative = false, window = "address:0xabc" })`},
		{"toggle", 0, `dispatch hl.dsp.window.float({ action = "disable", window = "address:0xabc" })`},
	} {
		t.Run(tc.action, func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			commands := make(chan string, 4)
			h := floatDesktop(t, w, m, func(cmd string) string {
				commands <- cmd
				if tc.action == "toggle" {
					w["floating"] = false
				}
				w["size"] = []int{477, 299}
				return "ok"
			})
			if err := h.Float(context.Background(), tc.action, tc.delta); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; got != tc.want {
				t.Fatalf("%s != %s", got, tc.want)
			}
			s, err := h.Snapshot(context.Background())
			if err != nil || s.WindowWidth != 477 || s.WindowHeight != 299 || !s.WindowReady {
				t.Fatal(s, err)
			}
		})
	}
}
func TestFloatRejectsUnsafeGeometryAndState(t *testing.T) {
	for _, mutate := range []func(map[string]any, map[string]any){
		func(w, m map[string]any) { w["address"] = `0xabc\");evil()` }, func(w, m map[string]any) { delete(w, "hidden") }, func(w, m map[string]any) { w["mapped"] = false }, func(w, m map[string]any) { w["hidden"] = true }, func(w, m map[string]any) { w["fullscreen"] = 1 }, func(w, m map[string]any) { w["floating"] = false }, func(w, m map[string]any) { w["at"] = []int{100001, 0} }, func(w, m map[string]any) { w["size"] = []int{500, 100001} }, func(w, m map[string]any) { w["workspace"] = map[string]any{"id": 3} }, func(w, m map[string]any) { m["focused"] = false }, func(w, m map[string]any) { m["scale"] = 0 }, func(w, m map[string]any) { m["reserved"] = []int{1} }, func(w, m map[string]any) { m["transform"] = 8 }, func(w, m map[string]any) { delete(m, "x") }, func(w, m map[string]any) { m["width"] = 0 }, func(w, m map[string]any) { m["reserved"] = []int{-1, 0, 0, 0} },
	} {
		w, m := floatWindow(), floatMonitor()
		mutate(w, m)
		h := floatDesktop(t, w, m, func(cmd string) string { t.Errorf("unsafe dispatch: %s", cmd); return "ok" })
		if err := h.Float(context.Background(), "move_x", 1); err == nil {
			t.Fatal("unsafe edit accepted", w, m)
		}
	}
}
func TestFloatToggleTiledAndReadbackGuards(t *testing.T) {
	for _, outcome := range []string{"enabled", "unchanged", "disappeared", "fullscreen", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			w["floating"] = false
			h := floatDesktop(t, w, m, func(cmd string) string {
				if !strings.Contains(cmd, `action = "enable"`) {
					t.Error(cmd)
				}
				switch outcome {
				case "enabled":
					w["floating"] = true
				case "disappeared":
					w["address"] = "0xdef"
				case "fullscreen":
					w["floating"] = true
					w["fullscreen"] = 1
				case "rejected":
					return "error"
				}
				return "ok"
			})
			err := h.Float(context.Background(), "toggle", 0)
			if (err == nil) != (outcome == "enabled") {
				t.Fatal(outcome, err)
			}
		})
	}
}

func TestFloatWorkareaEdgesRotationTinyAndNoop(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		delta        int
		at, size     []int
		mutate       func(map[string]any)
		want         string
	}{
		{"min-width", "resize_w", -32, []int{-900, 80}, []int{170, 300}, nil, `dispatch hl.dsp.window.resize({ x = 160, y = 300, relative = false, window = "address:0xabc" })`},
		{"min-height", "resize_h", -32, []int{-900, 80}, []int{500, 110}, nil, `dispatch hl.dsp.window.resize({ x = 500, y = 100, relative = false, window = "address:0xabc" })`},
		{"left-noop", "move_x", -32, []int{-980, 30}, []int{500, 300}, nil, `dispatch hl.dsp.window.move({ x = -980, y = 30, relative = false, window = "address:0xabc" })`},
		{"rotate", "move_x", 32, []int{-900, 80}, []int{500, 300}, func(m map[string]any) { m["width"] = 800; m["height"] = 1200; m["transform"] = 1 }, `dispatch hl.dsp.window.move({ x = -580, y = 80, relative = false, window = "address:0xabc" })`},
		{"tiny", "resize_w", -32, []int{-980, 30}, []int{40, 40}, func(m map[string]any) { m["width"] = 125; m["height"] = 125 }, `dispatch hl.dsp.window.resize({ x = 40, y = 20, relative = false, window = "address:0xabc" })`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			w["at"] = tc.at
			w["size"] = tc.size
			if tc.mutate != nil {
				tc.mutate(m)
			}
			commands := make(chan string, 1)
			h := floatDesktop(t, w, m, func(cmd string) string {
				commands <- cmd
				var width, height int
				if n, err := fmt.Sscanf(cmd, "dispatch hl.dsp.window.resize({ x = %d, y = %d,", &width, &height); err == nil && n == 2 {
					w["size"] = []int{width, height}
				}
				return "ok"
			})
			if err := h.Float(context.Background(), tc.action, tc.delta); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
func TestFloatInvalidInputNeverConnects(t *testing.T) {
	h := Hyprland{Socket: "/does/not/exist"}
	for _, tc := range []struct {
		action string
		delta  int
	}{{"move_x", 0}, {"move_y", 33}, {"resize_w", -33}, {"shell", 1}, {"toggle", 1}} {
		if err := h.Float(context.Background(), tc.action, tc.delta); err == nil || err.Error() != "invalid floating action" {
			t.Fatal(tc, err)
		}
	}
}
func TestFloatReadbackPinnedWhenFocusChanges(t *testing.T) {
	w, m := floatWindow(), floatMonitor()
	focused := w
	h := fakeDesktop(t, func(cmd string) string {
		switch cmd {
		case "j/activeworkspace":
			return `{"id":2}`
		case "j/activewindow":
			return jsonText(focused)
		case "j/monitors":
			return "[" + jsonText(m) + "]"
		case "j/clients":
			return "[" + jsonText(w) + "]"
		}
		focused = floatWindow()
		focused["address"] = "0xdef"
		return "ok"
	})
	if err := h.Float(context.Background(), "move_x", 1); err != nil {
		t.Fatal(err)
	}
}

func TestFloatResizePreservesMinimumUnlessMonitorTiny(t *testing.T) {
	w, m := floatWindow(), floatMonitor()
	w["at"] = []int{-100, 80}
	w["size"] = []int{20, 300}
	commands := make(chan string, 1)
	h := floatDesktop(t, w, m, func(cmd string) string { commands <- cmd; return "ok" })
	if err := h.Float(context.Background(), "resize_w", -1); err == nil {
		t.Fatal("subminimum resize on normal monitor accepted")
	}
	select {
	case cmd := <-commands:
		t.Fatal("unsafe resize", cmd)
	default:
	}
}

func TestFloatMalformedIPCAndToggleGuards(t *testing.T) {
	for _, badCommand := range []string{"j/activeworkspace", "j/activewindow", "j/monitors", "j/clients"} {
		t.Run(badCommand, func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			h := fakeDesktop(t, func(cmd string) string {
				if cmd == badCommand {
					return `{"broken":`
				}
				switch cmd {
				case "j/activeworkspace":
					return `{"id":2}`
				case "j/activewindow":
					return jsonText(w)
				case "j/monitors":
					return "[" + jsonText(m) + "]"
				case "j/clients":
					return "[" + jsonText(w) + "]"
				}
				return "ok"
			})
			if err := h.Float(context.Background(), "move_x", 1); err == nil {
				t.Fatal("invalid IPC accepted")
			}
		})
	}
	empty, fullscreen, hidden, unmapped := map[string]any{}, floatWindow(), floatWindow(), floatWindow()
	fullscreen["fullscreen"] = 1
	hidden["hidden"] = true
	unmapped["mapped"] = false
	for _, w := range []map[string]any{empty, fullscreen, hidden, unmapped} {
		h := floatDesktop(t, w, floatMonitor(), func(cmd string) string { t.Errorf("unsafe toggle %s", cmd); return "ok" })
		if err := h.Float(context.Background(), "toggle", 0); err == nil {
			t.Fatal("unsafe toggle accepted")
		}
	}
}
