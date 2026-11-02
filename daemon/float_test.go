package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestFloatProtocolV3(t *testing.T) {
	raw := `{"v":3,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"float","focus_axis":"horizontal","width_mode":"fine","float_action":"move_x","window_floating":true,"window_ready":true,"window_x":-100,"window_y":20,"window_width":800,"window_height":600}`
	if _, err := decodeFrame([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	json.Unmarshal([]byte(raw), &obj)
	for _, key := range []string{"float_action", "window_floating", "window_ready", "window_x", "window_y", "window_width", "window_height"} {
		copy := map[string]any{}
		for k, v := range obj {
			copy[k] = v
		}
		delete(copy, key)
		b, _ := json.Marshal(copy)
		if _, err := decodeFrame(b); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, v := range []int{1, 2} {
		obj["v"] = v
		b, _ := json.Marshal(obj)
		if _, err := decodeFrame(b); err == nil {
			t.Fatalf("v%d accepted", v)
		}
	}
}

type floatFake struct {
	memoryBackend
	actions []string
	deltas  []int
	fail    bool
}

func (b *floatFake) Float(_ context.Context, action string, delta int) error {
	b.actions = append(b.actions, action)
	b.deltas = append(b.deltas, delta)
	if b.fail {
		return errors.New("failed")
	}
	if action == "toggle" {
		b.state.WindowFloating = !b.state.WindowFloating
	}
	b.state.WindowX = 37
	return nil
}
func TestFloatController(t *testing.T) {
	ctx := context.Background()
	b := &floatFake{memoryBackend: memoryBackend{state: State{Connected: true, Workspace: 2, WindowReady: true, WindowFloating: true, Layout: "dwindle"}}}
	c := Controller{Backend: b, Max: 10}
	for i := 0; i < 3; i++ {
		if err := c.Handle(ctx, gesture("user", "click")); err != nil {
			t.Fatal(err)
		}
	}
	if c.State.Mode != "float" || c.State.FloatAction != "move_x" {
		t.Fatal(c.State)
	}
	for _, action := range []string{"move_x", "move_y", "resize_w", "resize_h"} {
		if err := c.Handle(ctx, turn(-2)); err != nil || c.State.FloatAction != action || c.State.WindowX != 37 {
			t.Fatal(c.State, err)
		}
		if err := c.Handle(ctx, gesture("knob", "click")); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(b.actions, []string{"move_x", "move_y", "resize_w", "resize_h"}) || !reflect.DeepEqual(b.deltas, []int{-2, -2, -2, -2}) {
		t.Fatal(b.actions, b.deltas)
	}
	if err := c.Handle(ctx, gesture("knob", "hold")); err != nil || c.State.WindowFloating {
		t.Fatal(c.State, err)
	}
	if err := c.Handle(ctx, turn(1)); err == nil {
		t.Fatal("tiled edit accepted")
	}
	if err := c.Handle(ctx, gesture("knob", "hold")); err != nil || !c.State.WindowFloating {
		t.Fatal(c.State, err)
	}
	c.Handle(ctx, gesture("knob", "click"))
	c.Handle(ctx, gesture("user", "hold"))
	if c.State.Mode != "workspace" || c.State.FloatAction != "move_y" {
		t.Fatal(c.State)
	}
	for _, bad := range []State{{Connected: false, WindowReady: true, WindowFloating: true}, {Connected: true, WindowFloating: true}, {Connected: true, WindowReady: false}} {
		b.state = bad
		c.mode = "float"
		before := len(b.actions)
		for _, f := range []Frame{turn(1), gesture("knob", "hold")} {
			if err := c.Handle(ctx, f); err == nil {
				t.Fatal("unsafe action accepted", bad)
			}
		}
		if len(b.actions) != before {
			t.Fatal(b.actions)
		}
	}
}

func TestFloatProtocolBounds(t *testing.T) {
	base := map[string]any{"v": 3, "type": "status", "workspace": 2, "connected": true, "rx": 1, "mode": "float", "focus_axis": "horizontal", "width_mode": "fine", "float_action": "resize_h", "window_floating": false, "window_ready": true, "window_x": -100000, "window_y": 100000, "window_width": 1, "window_height": 100000}
	if _, err := decodeFrame([]byte(jsonText(base))); err != nil {
		t.Fatal(err)
	}
	for key, values := range map[string][]any{"float_action": {"toggle", "shell", ""}, "window_x": {-100001, 100001, 0.5}, "window_y": {-100001, 100001}, "window_width": {-1, 0, 100001}, "window_height": {-1, 0, 100001}, "window_ready": {nil, "true", 1}, "window_floating": {nil, 1}} {
		original := base[key]
		for _, value := range values {
			base[key] = value
			if _, err := decodeFrame([]byte(jsonText(base))); err == nil {
				t.Fatal(key, value)
			}
		}
		base[key] = original
	}
	for _, kind := range []string{"hello", "encoder", "button", "status"} {
		for _, version := range []int{1, 2} {
			raw := map[string]any{"v": version, "type": kind, "firmware": "0.4.0", "seq": 1, "delta": 1, "button": "knob", "action": "click"}
			if _, err := decodeFrame([]byte(jsonText(raw))); err == nil {
				t.Fatal(raw)
			}
		}
	}
}
func TestFloatReconnectAndOptionalBackend(t *testing.T) {
	ctx := context.Background()
	c := Controller{Backend: unavailableBackend{}, Max: 10}
	c.mode = "float"
	c.Refresh(ctx)
	c.Handle(ctx, gesture("knob", "click"))
	if c.State.FloatAction != "move_y" {
		t.Fatal(c.State)
	}
	c.Backend = &memoryBackend{state: State{Connected: true, Workspace: 2, WindowReady: true, WindowFloating: true}}
	if c.Refresh(ctx).FloatAction != "move_y" || c.State.Mode != "float" {
		t.Fatal(c.State)
	}
	if err := c.Handle(ctx, turn(1)); err == nil {
		t.Fatal("optional backend accepted")
	}
	restarted := Controller{Backend: unavailableBackend{}, Max: 10}
	if restarted.Refresh(ctx).FloatAction != "move_x" {
		t.Fatal(restarted.State)
	}
	b := &floatFake{memoryBackend: memoryBackend{state: State{Connected: true, Workspace: 2, WindowReady: true, WindowFloating: true}}, fail: true}
	c.Backend = b
	if err := c.Handle(ctx, turn(1)); err == nil {
		t.Fatal("dispatch failure accepted")
	}
}

func TestFloatSessionEndToEndV3(t *testing.T) {
	w, m := floatWindow(), floatMonitor()
	commands := make(chan string, 4)
	backend := floatDesktop(t, w, m, func(cmd string) string { commands <- cmd; w["at"] = []int{-890, 80}; return "ok" })
	host, dev := net.Pipe()
	defer dev.Close()
	dev.SetDeadline(time.Now().Add(2 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := &Controller{Backend: backend, Max: 10, mode: "float"}
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, host, controller, nil, time.Hour) }()
	reader := bufio.NewReader(dev)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var before State
	if json.Unmarshal(line, &before) != nil || !before.WindowReady || before.V != 3 {
		t.Fatal(string(line))
	}
	io.WriteString(dev, `{"v":3,"type":"encoder","seq":4,"delta":1}`+"\n")
	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var ack Ack
	if json.Unmarshal(line, &ack) != nil || !ack.OK || ack.V != 3 {
		t.Fatal(string(line))
	}
	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var after State
	if json.Unmarshal(line, &after) != nil || after.WindowX != -890 || after.Mode != "float" || after.FloatAction != "move_x" {
		t.Fatal(string(line))
	}
	if got := <-commands; got != `dispatch hl.dsp.window.move({ x = -890, y = 80, relative = false, window = "address:0xabc" })` {
		t.Fatal(got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session did not stop")
	}
}
