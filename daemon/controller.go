package main

import (
	"context"
	"errors"
	"fmt"
)

type State struct {
	V              int    `json:"v"`
	Type           string `json:"type"`
	Workspace      int    `json:"workspace"`
	Name           string `json:"name"`
	Workspaces     []int  `json:"workspaces"`
	Backend        string `json:"backend"`
	Connected      bool   `json:"connected"`
	Error          string `json:"error"`
	Mode           string `json:"mode"`
	FocusAxis      string `json:"focus_axis"`
	WidthMode      string `json:"width_mode"`
	Layout         string `json:"layout"`
	App            string `json:"app"`
	WindowWidth    int    `json:"window_width"`
	FloatAction    string `json:"float_action"`
	WindowFloating bool   `json:"window_floating"`
	WindowReady    bool   `json:"window_ready"`
	WindowX        int    `json:"window_x"`
	WindowY        int    `json:"window_y"`
	WindowHeight   int    `json:"window_height"`
	WidthPercent   int    `json:"width_percent"`
	TiledWindow    bool   `json:"-"`
}
type Backend interface {
	Snapshot(context.Context) (State, error)
	Focus(context.Context, int) error
}
type Controller struct {
	Backend                                 Backend
	Max                                     int
	current, previous                       int
	State                                   State
	mode, focusAxis, widthMode, floatAction string
}

func (c *Controller) Refresh(ctx context.Context) State {
	s, e := c.Backend.Snapshot(ctx)
	if e != nil {
		s = State{Error: "hyprland unavailable"}
	}
	if c.mode == "" {
		c.mode = "workspace"
	}
	if c.focusAxis == "" {
		c.focusAxis = "horizontal"
	}
	if c.widthMode == "" {
		c.widthMode = "fine"
	}
	if c.floatAction == "" {
		c.floatAction = "move_x"
	}
	s.FloatAction = c.floatAction
	s.Mode, s.FocusAxis, s.WidthMode = c.mode, c.focusAxis, c.widthMode
	s.V = 3
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
	if f.Type == "hello" || f.Type == "status" {
		return nil
	}
	s := c.Refresh(ctx)
	if f.Type == "button" && f.Button == "user" {
		if f.Action == "hold" {
			c.mode = "workspace"
		} else if f.Action == "click" {
			switch c.mode {
			case "workspace":
				c.mode = "focus"
			case "focus":
				c.mode = "width"
			case "width":
				c.mode = "float"
			case "float":
				c.mode = "workspace"
			}
		} else {
			return errFrame
		}
		c.Refresh(ctx)
		return nil
	}
	if f.Type == "button" && f.Button == "knob" && f.Action == "click" && c.mode != "workspace" {
		if c.mode == "float" {
			switch c.floatAction {
			case "move_x":
				c.floatAction = "move_y"
			case "move_y":
				c.floatAction = "resize_w"
			case "resize_w":
				c.floatAction = "resize_h"
			default:
				c.floatAction = "move_x"
			}
		} else if c.mode == "focus" {
			if c.focusAxis == "horizontal" {
				c.focusAxis = "vertical"
			} else {
				c.focusAxis = "horizontal"
			}
		} else {
			if c.widthMode == "fine" {
				c.widthMode = "preset"
			} else {
				c.widthMode = "fine"
			}
		}
		c.Refresh(ctx)
		return nil
	}
	if c.mode == "float" {
		return c.handleFloat(ctx, f, s)
	}
	if c.mode != "workspace" {
		return c.handleLayout(ctx, f, s)
	}
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
		if f.Action == "hold" {
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

// Layout is optional: numeric workspace navigation does not require the plugin.
type layoutBackend interface {
	Layout(context.Context, string) error
}

func (c *Controller) handleLayout(ctx context.Context, f Frame, s State) error {
	b, ok := c.Backend.(layoutBackend)
	if !ok {
		return errors.New("layout backend unavailable")
	}
	command, count := "", 1
	if f.Type == "button" && f.Button == "knob" && f.Action == "hold" {
		if c.mode == "focus" {
			command = "focusback"
		} else {
			command = "fit active"
		}
	} else if f.Type == "encoder" {
		delta := *f.Delta
		count = delta
		if count < 0 {
			count = -count
		}
		if c.mode == "focus" {
			command = "focus r"
			if delta < 0 {
				command = "focus l"
			}
			if c.focusAxis == "vertical" {
				command = "focus d"
				if delta < 0 {
					command = "focus u"
				}
			}
		} else if c.widthMode == "preset" {
			command = "colresize +conf"
			if delta < 0 {
				command = "colresize -conf"
			}
		} else {
			command = fmt.Sprintf("colresize %+.2f", float64(delta)*0.05)
			count = 1
		}
	} else {
		return errFrame
	}
	for i := 0; i < count; i++ {
		if !s.Connected {
			return errors.New("hyprland unavailable")
		}
		if s.Layout != "hyprscrolling" {
			return errors.New("hyprscrolling layout required")
		}
		if !s.TiledWindow {
			return errors.New("focused tiled window required")
		}
		if err := b.Layout(ctx, command); err != nil {
			return errors.New("layout dispatch failed")
		}
		s = c.Refresh(ctx)
		if !s.Connected || !s.TiledWindow || s.Layout != "hyprscrolling" {
			return errors.New("layout state unavailable")
		}
	}
	return nil
}
