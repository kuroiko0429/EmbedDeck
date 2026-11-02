package main

import (
	"context"
	"strings"
	"testing"
)

func TestFloatOperationConfirmationIsIndependentOfCurrentFocus(t *testing.T) {
	for _, scenario := range []string{"focus-before-toggle", "focus-after-move"} {
		t.Run(scenario, func(t *testing.T) {
			a, b, monitor := floatWindow(), floatWindow(), floatMonitor()
			b["address"], b["size"] = "0xdef", []int{777, 300}
			a["floating"] = scenario != "focus-before-toggle"
			b["floating"] = scenario == "focus-before-toggle"
			active, reads := a, 0
			commands := make(chan string, 2)
			backend := fakeDesktop(t, func(cmd string) string {
				switch cmd {
				case "j/activeworkspace":
					return `{"id":2,"name":"2","tiledLayout":"dwindle"}`
				case "j/workspaces":
					return `[]`
				case "j/activewindow":
					reads++
					if scenario == "focus-before-toggle" && reads >= 2 {
						active = b
					}
					return jsonText(active)
				case "j/monitors":
					return "[" + jsonText(monitor) + "]"
				case "j/clients":
					return "[" + jsonText(a) + "," + jsonText(b) + "]"
				}
				commands <- cmd
				if scenario == "focus-before-toggle" {
					if cmd != `dispatch hl.dsp.window.float({ action = "disable", window = "address:0xdef" })` {
						t.Error(cmd)
					}
					b["floating"] = false
				} else {
					if !strings.Contains(cmd, `window = "address:0xabc"`) {
						t.Error(cmd)
					}
					a["at"] = []int{-890, 80}
					active = b
				}
				return "ok"
			})
			c := Controller{Backend: backend, Max: 10, mode: "float"}
			input := turn(1)
			if scenario == "focus-before-toggle" {
				input = gesture("knob", "hold")
			}
			if err := c.Handle(context.Background(), input); err != nil {
				t.Fatalf("backend-confirmed pinned operation falsely failed after focus change: %v", err)
			}
			if c.State.WindowWidth != 777 || c.State.WindowFloating || !c.State.WindowReady {
				t.Fatalf("publication must describe current focused B, not the operated target: %+v", c.State)
			}
			if len(commands) != 1 {
				t.Fatalf("unexpected desktop dispatches: %d", len(commands))
			}
		})
	}
}

func TestFloatStatusReadyRequiresPositiveDimensions(t *testing.T) {
	base := map[string]any{"v": 3, "type": "status", "workspace": 2, "connected": true, "rx": 1,
		"mode": "float", "focus_axis": "horizontal", "width_mode": "fine", "float_action": "move_x",
		"window_floating": true, "window_ready": true, "window_x": 0, "window_y": 0, "window_width": 600, "window_height": 400}
	for _, key := range []string{"window_width", "window_height"} {
		copy := map[string]any{}
		for k, v := range base {
			copy[k] = v
		}
		copy[key] = 0
		if _, err := decodeFrame([]byte(jsonText(copy))); err == nil {
			t.Fatalf("zero %s accepted for ready window", key)
		}
		copy["window_ready"] = false
		if _, err := decodeFrame([]byte(jsonText(copy))); err != nil {
			t.Fatalf("not-ready zero dimension rejected: %v", err)
		}
	}
}
