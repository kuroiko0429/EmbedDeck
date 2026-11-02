package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if e = json.Unmarshal(b, &a); e != nil || a.ID < 1 {
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
