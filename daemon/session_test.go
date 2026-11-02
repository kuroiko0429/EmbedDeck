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
	io.WriteString(dev, strings.Repeat("x", 2048)+"\n{\"v\":1,\"type\":\"hello\",\"firmware\":\"0.2.0\"}\n")
	if s := read(); s["workspace"] != float64(2) {
		t.Fatal(s)
	}
	io.WriteString(dev, "{\"v\":1,\"type\":\"encoder\",\"seq\":1,\"delta\":1}\n")
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
