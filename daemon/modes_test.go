package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func gesture(button, action string) Frame {
	return Frame{V: 2, Type: "button", Button: button, Action: action}
}
func turn(delta int) Frame { return Frame{V: 2, Type: "encoder", Delta: &delta} }

func TestModeCycleOfflineAndHostOwned(t *testing.T) {
	ctx := context.Background()
	c := Controller{Backend: unavailableBackend{}, Max: 10}
	s := c.Refresh(ctx)
	if s.Mode != "workspace" || s.FocusAxis != "horizontal" || s.WidthMode != "fine" {
		t.Fatal(s)
	}
	for _, want := range []string{"focus", "width", "workspace", "focus"} {
		if err := c.Handle(ctx, gesture("user", "click")); err != nil {
			t.Fatal(err)
		}
		if s = c.Refresh(ctx); s.Mode != want || s.Connected {
			t.Fatal(s)
		}
	}
	if err := c.Handle(ctx, gesture("knob", "click")); err != nil {
		t.Fatal(err)
	}
	if s = c.Refresh(ctx); s.FocusAxis != "vertical" {
		t.Fatal(s)
	}
	if err := c.Handle(ctx, gesture("user", "hold")); err != nil {
		t.Fatal(err)
	}
	if s = c.Refresh(ctx); s.Mode != "workspace" || s.FocusAxis != "vertical" {
		t.Fatal(s)
	}
	// Firmware readback is telemetry, never a request to change host modes.
	f, err := decodeFrame([]byte(`{"v":2,"type":"status","workspace":0,"connected":false,"rx":3,"mode":"width","focus_axis":"horizontal","width_mode":"preset"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(ctx, f); err != nil {
		t.Fatal(err)
	}
	if s = c.Refresh(ctx); s.Mode != "workspace" || s.FocusAxis != "vertical" || s.WidthMode != "fine" {
		t.Fatal(s)
	}
}

func TestUserHoldHasNoWorkspaceAction(t *testing.T) {
	b := &memoryBackend{state: State{Workspace: 7, Connected: true}}
	c := Controller{Backend: b, Max: 10}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := c.Handle(ctx, gesture("user", "click")); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Handle(ctx, gesture("knob", "click")); err != nil {
		t.Fatal(err)
	}
	if c.Refresh(ctx).WidthMode != "preset" {
		t.Fatal(c.State)
	}
	if err := c.Handle(ctx, gesture("user", "hold")); err != nil {
		t.Fatal(err)
	}
	if c.Refresh(ctx).Mode != "workspace" || len(b.targets) != 0 || b.state.Workspace != 7 {
		t.Fatal(c.State, b.targets)
	}
}

type scrollingBackend struct {
	memoryBackend
	commands      []string
	dispatchErr   error
	afterDispatch func()
}

func (b *scrollingBackend) Layout(_ context.Context, command string) error {
	b.commands = append(b.commands, command)
	if b.afterDispatch != nil {
		b.afterDispatch()
	}
	return b.dispatchErr
}
func scrollingController() (*Controller, *scrollingBackend) {
	b := &scrollingBackend{memoryBackend: memoryBackend{state: State{Workspace: 2, Connected: true, Layout: "hyprscrolling", TiledWindow: true}}}
	return &Controller{Backend: b, Max: 10}, b
}
func TestLayoutActualReadbackAndFailedQuery(t *testing.T) {
	ctx := context.Background()
	c, b := scrollingController()
	c.mode = "width"
	b.state.WindowWidth = 900
	b.afterDispatch = func() { b.state.WindowWidth = 946; b.state.WidthPercent = 51 }
	if err := c.Handle(ctx, turn(1)); err != nil || c.State.WindowWidth != 946 || c.State.WidthPercent != 51 {
		t.Fatal(c.State, err)
	}
	b.afterDispatch = func() { b.state.TiledWindow = false; b.state.WindowWidth = 0 }
	if err := c.Handle(ctx, turn(1)); err == nil {
		t.Fatal("missing focused window readback accepted")
	}
}

func TestFocusAxesAndFocusBack(t *testing.T) {
	ctx := context.Background()
	c, b := scrollingController()
	c.Handle(ctx, gesture("user", "click"))
	for _, f := range []Frame{turn(2), turn(-1), gesture("knob", "click"), turn(1), turn(-2), gesture("knob", "hold")} {
		if err := c.Handle(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"focus r", "focus r", "focus l", "focus d", "focus u", "focus u", "focusback"}
	if !reflect.DeepEqual(b.commands, want) || len(b.targets) != 0 {
		t.Fatal(b.commands, b.targets)
	}
}
func TestWidthFinePresetAndFit(t *testing.T) {
	ctx := context.Background()
	c, b := scrollingController()
	c.Handle(ctx, gesture("user", "click"))
	c.Handle(ctx, gesture("user", "click"))
	for _, f := range []Frame{turn(1), turn(-3), turn(32), gesture("knob", "click"), turn(2), turn(-2), gesture("knob", "hold"), gesture("knob", "click"), turn(-1)} {
		if err := c.Handle(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"colresize +0.05", "colresize -0.15", "colresize +1.60", "colresize +conf", "colresize +conf", "colresize -conf", "colresize -conf", "fit active", "colresize -0.05"}
	if !reflect.DeepEqual(b.commands, want) || len(b.targets) != 0 {
		t.Fatal(b.commands, b.targets)
	}
}
func TestScrollingGuardsAndErrors(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []State{
		{Workspace: 2, Connected: true, Layout: "dwindle", TiledWindow: true},
		{Workspace: 2, Connected: true, Layout: "hyprscrolling"},
		{Workspace: 2, Layout: "hyprscrolling", TiledWindow: true},
	} {
		for _, mode := range []string{"focus", "width"} {
			c, b := scrollingController()
			b.state = bad
			c.mode = mode
			for _, f := range []Frame{turn(1), gesture("knob", "hold")} {
				if err := c.Handle(ctx, f); err == nil {
					t.Fatal("accepted invalid state", bad)
				}
			}
			if len(b.commands) != 0 || len(b.targets) != 0 {
				t.Fatal(b.commands, b.targets)
			}
		}
	}
	c := Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true, Layout: "hyprscrolling", TiledWindow: true}}, Max: 10, mode: "focus"}
	if err := c.Handle(ctx, turn(1)); err == nil {
		t.Fatal("backend without layout support accepted")
	}
	c2, b := scrollingController()
	c2.mode = "focus"
	b.dispatchErr = errors.New("rejected")
	if err := c2.Handle(ctx, turn(3)); err == nil || len(b.commands) != 1 {
		t.Fatal(err, b.commands)
	}
	// Dispatch acceptance is success even when a single-window focus is a no-op.
	b.dispatchErr = nil
	if err := c2.Handle(ctx, turn(1)); err != nil {
		t.Fatal(err)
	}
	if c2.State.Workspace != 2 {
		t.Fatal(c2.State)
	}
}
