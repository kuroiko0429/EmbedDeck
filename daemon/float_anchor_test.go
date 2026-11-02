package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestFloatResizeKeepsAnchorWhenCompositorRecenters(t *testing.T) {
	for _, action := range []string{"resize_w", "resize_h"} {
		t.Run(action, func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			w["at"] = []int{-980, 30}
			commands := make(chan string, 4)
			h := floatDesktop(t, w, m, func(cmd string) string {
				commands <- cmd
				if strings.HasPrefix(cmd, "dispatch hl.dsp.window.resize(") {
					if action == "resize_w" {
						w["size"], w["at"] = []int{520, 300}, []int{-990, 30}
					} else {
						w["size"], w["at"] = []int{500, 320}, []int{-980, 20}
					}
				} else if strings.HasPrefix(cmd, "dispatch hl.dsp.window.move(") {
					w["at"] = []int{-980, 30}
				} else {
					t.Errorf("unexpected dispatcher %q", cmd)
				}
				return "ok"
			})
			if err := h.Float(context.Background(), action, 2); err != nil {
				t.Fatal(err)
			}
			s, err := h.Snapshot(context.Background())
			if err != nil || s.WindowX != -980 || s.WindowY != 30 {
				t.Fatalf("resize recentering escaped the left/top workarea: state=%+v err=%v", s, err)
			}
			if len(commands) != 2 {
				t.Fatalf("expected resize then pinned anchor correction, got %d dispatches", len(commands))
			}
			<-commands
			if got := <-commands; got != `dispatch hl.dsp.window.move({ x = -980, y = 30, relative = false, window = "address:0xabc" })` {
				t.Fatal(got)
			}
		})
	}
}

func TestFloatResizeRejectsActualConstraintOverflow(t *testing.T) {
	w, m := floatWindow(), floatMonitor()
	h := floatDesktop(t, w, m, func(cmd string) string {
		if !strings.HasPrefix(cmd, "dispatch hl.dsp.window.resize(") {
			t.Errorf("unexpected correction %q", cmd)
		}
		w["size"] = []int{9000, 300}
		return "ok"
	})
	if err := h.Float(context.Background(), "resize_w", 1); err == nil {
		t.Fatal("acknowledged application-constrained overflow beyond usable monitor")
	}
}

func TestFloatResizeCorrectionMustBeConfirmed(t *testing.T) {
	for _, rejected := range []bool{true, false} {
		t.Run(fmt.Sprint(rejected), func(t *testing.T) {
			w, m := floatWindow(), floatMonitor()
			h := floatDesktop(t, w, m, func(cmd string) string {
				if strings.HasPrefix(cmd, "dispatch hl.dsp.window.resize(") {
					w["size"], w["at"] = []int{520, 300}, []int{-910, 80}
					return "ok"
				}
				if rejected {
					return "error"
				}
				return "ok" // compositor ignores correction: must not claim anchor held
			})
			if err := h.Float(context.Background(), "resize_w", 2); err == nil {
				t.Fatal("unconfirmed resize anchor accepted")
			}
		})
	}
}
