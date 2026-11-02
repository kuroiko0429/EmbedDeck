package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSessionHelloActionStatusAndCancel(t *testing.T) {
	host, dev := net.Pipe()
	defer dev.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &memoryBackend{state: State{Workspace: 2, Connected: true, Workspaces: []int{1, 2}}}
	c := &Controller{Backend: b, Max: 10}
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, host, c, nil, 20*time.Millisecond) }()
	dev.SetDeadline(time.Now().Add(2 * time.Second))
	r := bufio.NewReader(dev)
	read := func() map[string]any {
		t.Helper()
		line, e := r.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var m map[string]any
		if e = json.Unmarshal(line, &m); e != nil {
			t.Fatal(e)
		}
		return m
	}
	if s := read(); s["workspace"] != float64(2) || s["type"] != "state" {
		t.Fatal(s)
	}
	io.WriteString(dev, strings.Repeat("x", 2048)+"\n{\"v\":3,\"type\":\"hello\",\"firmware\":\"0.4.0\"}\n")
	if s := read(); s["workspace"] != float64(2) {
		t.Fatal(s)
	}
	io.WriteString(dev, "{\"v\":3,\"type\":\"encoder\",\"seq\":1,\"delta\":1}\n")
	for {
		m := read()
		if m["type"] == "ack" {
			if m["ok"] != true {
				t.Fatal(m)
			}
			break
		}
	}
	if m := read(); m["workspace"] != float64(3) {
		t.Fatal(m)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	if len(b.targets) != 1 || b.targets[0] != 3 {
		t.Fatal(b.targets)
	}
}

type chunkPort struct{ net.Conn }

func (p chunkPort) Write(b []byte) (int, error) {
	if len(b) > 3 {
		b = b[:3]
	}
	return p.Conn.Write(b)
}
func (p chunkPort) Read(b []byte) (int, error) {
	if len(b) > 2 {
		b = b[:2]
	}
	return p.Conn.Read(b)
}

func TestSessionBoundedV3StateWithShortReadsWrites(t *testing.T) {
	b := &memoryBackend{state: State{Workspace: 2147483647, Connected: true, Name: safeName(strings.Repeat("<", 63)), Layout: safeName(strings.Repeat("<", 63)), App: strings.Repeat("<", 24), WindowWidth: 100000, WindowHeight: 100000, WindowX: -100000, WindowY: -100000, WindowReady: true, WindowFloating: true, WidthPercent: 100}}
	for i := 0; i < 32; i++ {
		b.state.Workspaces = append(b.state.Workspaces, 2147483647-i)
	}
	c := &Controller{Backend: b, Max: 10000}
	host, dev := net.Pipe()
	defer dev.Close()
	dev.SetDeadline(time.Now().Add(2 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, chunkPort{host}, c, nil, time.Hour) }()
	r := bufio.NewReader(dev)
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if len(line)-1 > 1023 {
		t.Fatal("oversized state", len(line))
	}
	var state map[string]any
	if err = json.Unmarshal(line, &state); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"v", "type", "workspace", "name", "workspaces", "backend", "connected", "error", "mode", "focus_axis", "width_mode", "layout", "app", "window_width", "width_percent", "float_action", "window_ready", "window_floating", "window_x", "window_y", "window_height"} {
		if _, ok := state[key]; !ok {
			t.Fatal("missing", key)
		}
	}
	if state["v"] != float64(3) || state["mode"] != "workspace" || state["app"] != strings.Repeat("<", 24) {
		t.Fatal(state)
	}
	io.WriteString(dev, `{"v":3,"type":"button","seq":5,"button":"user","action":"click"}`+"\n")
	line, err = r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var ack Ack
	if err = json.Unmarshal(line, &ack); err != nil || ack.V != 3 || !ack.OK || ack.Seq != 5 {
		t.Fatal(string(line), err)
	}
	line, err = r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(line, &state); err != nil || state["mode"] != "focus" {
		t.Fatal(string(line), err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session blocked")
	}
	// Same controller, new connection: reconnect does not reset host-owned modes.
	c.Backend = unavailableBackend{}
	host2, dev2 := net.Pipe()
	defer dev2.Close()
	dev2.SetDeadline(time.Now().Add(time.Second))
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	done2 := make(chan error, 1)
	go func() { done2 <- serveSession(ctx2, host2, c, nil, time.Hour) }()
	line, err = bufio.NewReader(dev2).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(line, &state); err != nil || state["mode"] != "focus" || state["connected"] != false {
		t.Fatal(string(line), err)
	}
	cancel2()
	select {
	case <-done2:
	case <-time.After(time.Second):
		t.Fatal("second session blocked")
	}
}

func TestSessionRejectsOldGesturesAndNacksUnavailableLayout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, dev := net.Pipe()
	defer dev.Close()
	dev.SetDeadline(time.Now().Add(2 * time.Second))
	c := &Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true}}, Max: 10, mode: "focus"}
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, host, c, nil, time.Hour) }()
	r := bufio.NewReader(dev)
	if _, err := r.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	io.WriteString(dev, strings.Join([]string{
		`{"v":1,"type":"encoder","seq":1,"delta":1}`,
		`{"v":2,"type":"encoder","seq":6,"delta":1}`,
		`{"v":3,"type":"button","seq":2,"button":"user","action":"press"}`,
		`{"v":3,"type":"button","seq":3,"button":"user","action":"release"}`,
		`{"v":3,"type":"encoder","seq":4,"delta":0}`,
		`{"v":3,"type":"encoder","seq":5,"delta":33}`,
		`{"v":3,"type":"encoder","seq":99,"delta":1}`,
	}, "\n")+"\n")
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var ack Ack
	if err = json.Unmarshal(line, &ack); err != nil || ack.V != 3 || ack.Seq != 99 || ack.OK || ack.Error == "" {
		t.Fatal(string(line), err)
	}
	line, err = r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err = json.Unmarshal(line, &state); err != nil || !state.Connected || state.Mode != "focus" || state.Workspace != 2 {
		t.Fatal(string(line), err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session blocked")
	}
}

func TestSessionBlockedWriteCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	host, dev := net.Pipe()
	defer dev.Close()
	c := &Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true}}, Max: 10}
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, host, c, nil, time.Second) }()
	// Device intentionally never consumes the initial state write.
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("blocked write ignored cancellation")
	}
}

type zeroWriter struct{ net.Conn }

func (p zeroWriter) Write([]byte) (int, error) { return 0, nil }
func TestSessionZeroWriteFails(t *testing.T) {
	host, dev := net.Pipe()
	defer dev.Close()
	c := &Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true}}, Max: 10}
	if err := serveSession(context.Background(), zeroWriter{host}, c, nil, time.Second); err != io.ErrShortWrite {
		t.Fatal(err)
	}
}
