package main

import (
	"context"
	"testing"
)

func TestStatusTelemetryNeverDispatches(t *testing.T) {
	b := &memoryBackend{state: State{Workspace: 2, Connected: true}}
	c := Controller{Backend: b, Max: 10}
	for _, s := range []string{`{"v":3,"type":"status","workspace":2,"connected":true,"rx":3,"mode":"workspace","focus_axis":"horizontal","width_mode":"fine","float_action":"move_x","window_floating":false,"window_ready":false,"window_x":0,"window_y":0,"window_width":0,"window_height":0}`, `{"v":3,"type":"status","workspace":0,"connected":false,"rx":0,"mode":"width","focus_axis":"vertical","width_mode":"preset","float_action":"move_x","window_floating":false,"window_ready":false,"window_x":0,"window_y":0,"window_width":0,"window_height":0}`} {
		f, e := decodeFrame([]byte(s))
		if e != nil {
			t.Fatal(e)
		}
		if e = c.Handle(context.Background(), f); e != nil {
			t.Fatal(e)
		}
	}
	if len(b.targets) != 0 {
		t.Fatal(b.targets)
	}
	for _, s := range []string{`{"v":3,"type":"status","workspace":0,"connected":true,"rx":1}`, `{"v":3,"type":"status","workspace":2,"connected":true}`, `{"v":3,"type":"status","workspace":2,"connected":"true","rx":1}`} {
		if _, e := decodeFrame([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}
