package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStateWireBounds(t *testing.T) {
	name := safeName(strings.Repeat("🙂", 80) + "\n\x1b")
	if len(name) > 63 || !utf8.ValidString(name) {
		t.Fatalf("name bytes %d", len(name))
	}
	s := State{V: 1, Type: "state", Backend: "hyprland", Connected: true, Workspace: 2147483647, Name: safeName(strings.Repeat("<", 100)), Workspaces: []int{}}
	for i := 0; i < 32; i++ {
		s.Workspaces = append(s.Workspaces, 2147483647-i)
	}
	b, _ := json.Marshal(s)
	if len(b) > 1023 {
		t.Fatalf("frame bytes %d", len(b))
	}
}

type unavailableBackend struct{}

func (unavailableBackend) Snapshot(context.Context) (State, error) {
	return State{Workspace: 2, Connected: true}, errors.New("private\n\x1bsecret")
}
func (unavailableBackend) Focus(context.Context, int) error { return nil }
func TestDisconnectedStateSanitized(t *testing.T) {
	c := Controller{Backend: unavailableBackend{}, Max: 10}
	s := c.Refresh(context.Background())
	if s.Connected || s.Workspace != 0 || s.Error != "hyprland unavailable" {
		t.Fatal(s)
	}
}
