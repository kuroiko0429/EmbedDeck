package main

import (
	"context"
	"testing"
)

func TestStatusTelemetryNeverDispatches(t *testing.T) {
	b := &memoryBackend{state: State{Workspace: 2, Connected: true}}
	c := Controller{Backend: b, Max: 10}
	for _, s := range []string{`{"v":2,"type":"status","workspace":2,"connected":true,"rx":3,"mode":"workspace","focus_axis":"horizontal","width_mode":"fine"}`, `{"v":2,"type":"status","workspace":0,"connected":false,"rx":0,"mode":"width","focus_axis":"vertical","width_mode":"preset"}`} {
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
	for _, s := range []string{`{"v":2,"type":"status","workspace":0,"connected":true,"rx":1}`, `{"v":2,"type":"status","workspace":2,"connected":true}`, `{"v":2,"type":"status","workspace":2,"connected":"true","rx":1}`} {
		if _, e := decodeFrame([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}
