package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const maxFrame = 1023

var errFrame = errors.New("invalid protocol frame")

type Frame struct {
	V         int     `json:"v"`
	Type      string  `json:"type"`
	Firmware  string  `json:"firmware,omitempty"`
	Seq       *uint32 `json:"seq,omitempty"`
	Delta     *int    `json:"delta,omitempty"`
	Button    string  `json:"button,omitempty"`
	Action    string  `json:"action,omitempty"`
	Workspace *int    `json:"workspace,omitempty"`
	Connected *bool   `json:"connected,omitempty"`
	RX        *uint32 `json:"rx,omitempty"`
}

func decodeFrame(b []byte) (Frame, error) {
	var f Frame
	if len(b) > maxFrame || !utf8.Valid(b) {
		return f, errFrame
	}
	// Reject duplicate keys as well as unknown keys and null required values.
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return f, errFrame
	}
	keys := map[string]bool{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return f, errFrame
		}
		s, ok := k.(string)
		if !ok || keys[s] {
			return f, errFrame
		}
		keys[s] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(raw, []byte("null")) {
			return f, errFrame
		}
	}
	if _, e = d.Token(); e != nil {
		return f, errFrame
	}
	if _, e = d.Token(); e != io.EOF {
		return f, errFrame
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&f) != nil || f.V != 1 {
		return f, errFrame
	}
	allowed := map[string]bool{"v": true, "type": true}
	switch f.Type {
	case "status":
		allowed["workspace"] = true
		allowed["connected"] = true
		allowed["rx"] = true
		if f.Workspace == nil || f.Connected == nil || f.RX == nil || *f.Workspace < 0 || (*f.Connected && *f.Workspace < 1) {
			return f, errFrame
		}
	case "hello":
		allowed["firmware"] = true
		if f.Firmware == "" {
			return f, errFrame
		}
	case "encoder":
		allowed["seq"] = true
		allowed["delta"] = true
		if f.Seq == nil || f.Delta == nil || *f.Delta == 0 || *f.Delta < -32 || *f.Delta > 32 {
			return f, errFrame
		}
	case "button":
		allowed["seq"] = true
		allowed["button"] = true
		allowed["action"] = true
		if f.Seq == nil || (f.Button != "knob" && f.Button != "user") || (f.Action != "press" && f.Action != "release") {
			return f, errFrame
		}
	default:
		return f, errFrame
	}
	for k := range keys {
		if !allowed[k] {
			return f, errFrame
		}
	}
	return f, nil
}
