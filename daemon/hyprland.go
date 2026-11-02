package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Hyprland struct{ Socket string }

func request(ctx context.Context, path, cmd string) ([]byte, error) {
	c, e := (&net.Dialer{Timeout: 500 * time.Millisecond}).DialContext(ctx, "unix", path)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	deadline := time.Now().Add(500 * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.SetDeadline(deadline)
	if _, e = io.WriteString(c, cmd); e != nil {
		return nil, e
	}
	b, e := io.ReadAll(io.LimitReader(c, 1<<20))
	if e != nil {
		return nil, e
	}
	if len(b) >= 1<<20 {
		return nil, errors.New("IPC reply too large")
	}
	return b, nil
}
func discover(ctx context.Context) (string, error) {
	root := os.Getenv("XDG_RUNTIME_DIR")
	if root == "" {
		return "", errors.New("XDG_RUNTIME_DIR not set")
	}
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		if filepath.Base(sig) != sig {
			return "", errors.New("invalid instance signature")
		}
		p := filepath.Join(root, "hypr", sig, ".socket.sock")
		_, e := request(ctx, p, "j/activeworkspace")
		return p, e
	}
	paths, _ := filepath.Glob(filepath.Join(root, "hypr", "*", ".socket.sock"))
	var live []string
	for _, p := range paths {
		if _, e := request(ctx, p, "j/activeworkspace"); e == nil {
			live = append(live, p)
		}
	}
	if len(live) != 1 {
		return "", fmt.Errorf("expected one live Hyprland instance, found %d", len(live))
	}
	return live[0], nil
}
func (h Hyprland) path(ctx context.Context) (string, error) {
	if h.Socket != "" {
		return h.Socket, nil
	}
	return discover(ctx)
}
func (h Hyprland) Snapshot(ctx context.Context) (State, error) {
	var s State
	p, e := h.path(ctx)
	if e != nil {
		return s, e
	}
	b, e := request(ctx, p, "j/activeworkspace")
	if e != nil {
		return s, e
	}
	var a struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Layout string `json:"tiledLayout"`
	}
	if e = json.Unmarshal(b, &a); e != nil || a.ID < 1 || a.ID > 10000 {
		return s, errors.New("invalid active workspace")
	}
	b, e = request(ctx, p, "j/workspaces")
	if e != nil {
		return s, e
	}
	var list []struct {
		ID int `json:"id"`
	}
	if e = json.Unmarshal(b, &list); e != nil {
		return s, e
	}
	seen := map[int]bool{}
	for _, w := range list {
		if w.ID > 0 {
			seen[w.ID] = true
		}
	}
	seen[a.ID] = true
	s.Workspaces = []int{}
	for id := range seen {
		s.Workspaces = append(s.Workspaces, id)
	}
	sort.Ints(s.Workspaces)
	if len(s.Workspaces) > 32 {
		s.Workspaces = s.Workspaces[:32]
	}
	s.Workspace = a.ID
	s.Name = safeName(a.Name)
	s.Connected = true
	s.Layout = safeName(a.Layout)
	h.windowState(ctx, p, &s)
	return s, nil
}
func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r != 127 {
			if b.Len()+len(string(r)) > 63 {
				break
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
func (h Hyprland) Focus(ctx context.Context, n int) error {
	if n < 1 || n > 10000 {
		return errors.New("invalid workspace")
	}
	p, e := h.path(ctx)
	if e != nil {
		return e
	}
	b, e := request(ctx, p, fmt.Sprintf("dispatch hl.dsp.focus({ workspace = %d })", n))
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(b)) != "ok" {
		return errors.New("dispatch rejected")
	}
	return nil
}

// Optional window queries fail closed for layout actions, without disabling
// ordinary workspace navigation on an empty desktop or older compositor.
func (h Hyprland) windowState(ctx context.Context, p string, s *State) {
	b, err := request(ctx, p, "j/activewindow")
	if err != nil {
		return
	}
	var w struct {
		Address    string `json:"address"`
		Mapped     bool   `json:"mapped"`
		Hidden     bool   `json:"hidden"`
		Floating   *bool  `json:"floating"`
		Fullscreen *int   `json:"fullscreen"`
		Class      string `json:"class"`
		Size       []int  `json:"size"`
		Monitor    int    `json:"monitor"`
		Workspace  struct {
			ID int `json:"id"`
		} `json:"workspace"`
	}
	if json.Unmarshal(b, &w) != nil || w.Address == "" || !w.Mapped || w.Hidden || len(w.Size) != 2 || w.Size[0] <= 0 || w.Size[0] > 100000 || w.Size[1] <= 0 || w.Size[1] > 100000 {
		return
	}
	windowJSON := append([]byte(nil), b...)
	s.App = asciiLabel(w.Class)
	s.WindowWidth = w.Size[0]
	s.TiledWindow = w.Floating != nil && !*w.Floating && w.Fullscreen != nil && *w.Fullscreen == 0 && w.Workspace.ID == s.Workspace
	b, err = request(ctx, p, "j/monitors")
	if err != nil {
		return
	}
	if editable, _, e := decodeEditable(windowJSON, b, s.Workspace); e == nil {
		applyEditable(s, editable)
	}
	var monitors []struct {
		ID            int `json:"id"`
		Width, Height int
		Scale         float64
		Transform     int
		Reserved      [4]float64
	}
	if json.Unmarshal(b, &monitors) != nil {
		return
	}
	for _, m := range monitors {
		if m.ID != w.Monitor || m.Scale <= 0 {
			continue
		}
		width := m.Width
		if m.Transform%2 != 0 {
			width = m.Height
		}
		// reserved is left, top, right, bottom in logical pixels.
		usable := float64(width)/m.Scale - m.Reserved[0] - m.Reserved[2]
		if usable <= 0 {
			return
		}
		percent := math.Round(float64(s.WindowWidth) / usable * 100)
		s.WidthPercent = int(math.Min(100, math.Max(0, percent)))
		return
	}
}

func asciiLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r <= 126 {
			b.WriteByte(byte(r))
			if b.Len() == 24 {
				break
			}
		}
	}
	return b.String()
}

// Only commands constructed by the controller are accepted; never forward an
// arbitrary device/compositor string into the Lua dispatcher.
func (h Hyprland) Layout(ctx context.Context, command string) error {
	valid := false
	switch command {
	case "focus r", "focus l", "focus u", "focus d", "focusback", "fit active", "colresize +conf", "colresize -conf":
		valid = true
	default:
		for delta := -32; delta <= 32; delta++ {
			if delta != 0 && command == fmt.Sprintf("colresize %+.2f", float64(delta)*0.05) {
				valid = true
				break
			}
		}
	}
	if !valid {
		return errors.New("invalid layout command")
	}
	p, err := h.path(ctx)
	if err != nil {
		return err
	}
	b, err := request(ctx, p, `dispatch hl.dsp.layout("`+command+`")`)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) != "ok" {
		return errors.New("dispatch rejected")
	}
	return nil
}
