package main

import (
	"context"
	"testing"
)

type memoryBackend struct {
	state   State
	targets []int
	fail    bool
}

func (b *memoryBackend) Snapshot(context.Context) (State, error) { return b.state, nil }
func (b *memoryBackend) Focus(_ context.Context, n int) error {
	b.targets = append(b.targets, n)
	b.state.Workspace = n
	return nil
}
func TestNavigationConfirmedPrevious(t *testing.T) {
	b := &memoryBackend{state: State{Workspace: 2, Connected: true}}
	c := Controller{Backend: b, Max: 10}
	ctx := context.Background()
	c.Refresh(ctx)
	for _, step := range []struct {
		line string
		want int
	}{
		{`{"v":3,"type":"hello","firmware":"0.4.0"}`, 2},
		{`{"v":3,"type":"encoder","seq":1,"delta":32}`, 10},
		{`{"v":3,"type":"button","seq":2,"button":"knob","action":"click"}`, 2},
		{`{"v":3,"type":"button","seq":3,"button":"knob","action":"click"}`, 10},
		{`{"v":3,"type":"encoder","seq":4,"delta":-32}`, 1},
		{`{"v":3,"type":"hello","firmware":"0.4.0"}`, 1},
		{`{"v":3,"type":"encoder","seq":6,"delta":1}`, 2},
		{`{"v":3,"type":"button","seq":7,"button":"knob","action":"hold"}`, 1},
	} {
		f, _ := decodeFrame([]byte(step.line))
		if err := c.Handle(ctx, f); err != nil {
			t.Fatal(err)
		}
		if b.state.Workspace != step.want {
			t.Fatalf("%s got %d", step.line, b.state.Workspace)
		}
	}
	if len(b.targets) != 6 {
		t.Fatalf("unexpected dispatches %v", b.targets)
	}
	// External changes count as confirmed history too.
	b.state.Workspace = 7
	c.Refresh(ctx)
	f, _ := decodeFrame([]byte(`{"v":3,"type":"button","seq":8,"button":"knob","action":"click"}`))
	c.Handle(ctx, f)
	if b.state.Workspace != 1 {
		t.Fatal(b.state)
	}
}
