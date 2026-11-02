package main

import (
	"context"
	"errors"
)

type State struct {
	V          int    `json:"v"`
	Type       string `json:"type"`
	Workspace  int    `json:"workspace"`
	Name       string `json:"name"`
	Workspaces []int  `json:"workspaces"`
	Backend    string `json:"backend"`
	Connected  bool   `json:"connected"`
	Error      string `json:"error"`
}
type Backend interface {
	Snapshot(context.Context) (State, error)
	Focus(context.Context, int) error
}
type Controller struct {
	Backend           Backend
	Max               int
	current, previous int
	State             State
}

func (c *Controller) Refresh(ctx context.Context) State {
	s, e := c.Backend.Snapshot(ctx)
	if e != nil {
		s = State{Error: "hyprland unavailable"}
	}
	s.V = 1
	s.Type = "state"
	s.Backend = "hyprland"
	if s.Workspaces == nil {
		s.Workspaces = []int{}
	}
	if s.Connected && s.Workspace > 0 && s.Workspace != c.current {
		c.previous = c.current
		c.current = s.Workspace
	}
	c.State = s
	return s
}
func (c *Controller) Handle(ctx context.Context, f Frame) error {
	if f.Type == "hello" || f.Type == "status" || (f.Type == "button" && f.Action == "release") {
		return nil
	}
	s := c.Refresh(ctx)
	if !s.Connected || s.Workspace < 1 {
		return errors.New("hyprland unavailable")
	}
	n := s.Workspace
	switch f.Type {
	case "encoder":
		n += *f.Delta
		if n < 1 {
			n = 1
		}
		if n > c.Max {
			n = c.Max
		}
	case "button":
		if f.Button == "user" {
			n = 1
		} else if c.previous >= 1 && c.previous <= c.Max {
			n = c.previous
		}
	default:
		return errFrame
	}
	if n == s.Workspace {
		return nil
	}
	if e := c.Backend.Focus(ctx, n); e != nil {
		return errors.New("workspace dispatch failed")
	}
	s = c.Refresh(ctx)
	if !s.Connected || s.Workspace != n {
		return errors.New("workspace not confirmed")
	}
	return nil
}
