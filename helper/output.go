// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/z23/omarchy-flydigi/helper/internal/vader"
)

// apiVersion changes when the JSON shape changes in a way the plugin must know about.
const apiVersion = 1

type controllerJSON struct {
	Model     string `json:"model"`
	DeviceID  int    `json:"deviceId"`
	Supported bool   `json:"supported"`
	Firmware  string `json:"firmware"`
	MAC       string `json:"mac"`
	Link      string `json:"link"`
	Mode      string `json:"mode"`
	Port      string `json:"port"`
}

type lightingJSON struct {
	Mode       string   `json:"mode"`
	ModeID     int      `json:"modeId"`
	Brightness int      `json:"brightness"`
	Speed      *float64 `json:"speed"`
	Color      *string  `json:"color"`
	Zones      []string `json:"zones"`
}

type errorJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type result struct {
	API        int             `json:"api"`
	OK         bool            `json:"ok"`
	Changed    bool            `json:"changed"`
	Controller *controllerJSON `json:"controller,omitempty"`
	Lighting   *lightingJSON   `json:"lighting,omitempty"`
	Note       string          `json:"note,omitempty"`
	UpdatedAt  string          `json:"updatedAt,omitempty"`
	Error      *errorJSON      `json:"error,omitempty"`
}

type statusJSON struct {
	API     int    `json:"api"`
	Present bool   `json:"present"`
	Access  bool   `json:"access"`
	Mode    string `json:"mode"`
	Port    string `json:"port"`
	Node    string `json:"node"`
	Product string `json:"product"`
}

func describeController(info *vader.Info, d vader.Device) *controllerJSON {
	return &controllerJSON{
		Model: info.Model(), DeviceID: info.DeviceID, Supported: info.Supported(),
		Firmware: info.Firmware, MAC: info.MAC, Link: info.Link, Mode: d.Mode, Port: d.Port,
	}
}

func describeLighting(l *vader.Lighting) *lightingJSON {
	out := &lightingJSON{Mode: l.Mode.String(), ModeID: int(l.Mode), Brightness: int(l.Brightness), Zones: []string{}}
	for _, z := range l.ZoneColors() {
		out.Zones = append(out.Zones, z.Hex())
	}
	if s, ok := l.Speed(); ok {
		out.Speed = &s
	}
	if c, ok := l.Color(); ok && l.Mode != vader.ModeOff {
		h := c.Hex()
		out.Color = &h
	}
	return out
}

// Exit codes the plugin can tell apart without parsing text.
const (
	exitFailure    = 1
	exitUsage      = 2
	exitNotFound   = 3
	exitNoAccess   = 4
	exitBusy       = 5
	exitNoResponse = 6
)

func classify(err error) (code string, exit int, hint string) {
	var u usageError
	switch {
	case errors.As(err, &u):
		return "usage", exitUsage, "Run vader4-rgb --help."
	case errors.Is(err, vader.ErrNotFound):
		return "not_found", exitNotFound, "Connect the controller by USB or the 2.4 GHz dongle. Hold the circle button and X for XInput mode (white indicator), or the circle button and A for DInput mode (blue indicator)."
	case errors.Is(err, vader.ErrNoAccess):
		return "no_access", exitNoAccess, "Install the udev rule with install.sh from the omarchy-flydigi plugin, then unplug and replug the controller."
	case errors.Is(err, vader.ErrBusy):
		return "busy", exitBusy, "Another program, or another vader4-rgb, is using the controller. Try again in a moment."
	case errors.Is(err, vader.ErrNoResponse), errors.Is(err, context.DeadlineExceeded):
		return "no_response", exitNoResponse, "Unplug and replug the controller, then try again."
	case errors.Is(err, context.Canceled):
		return "canceled", exitFailure, ""
	}
	return "failed", exitFailure, ""
}

func printJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "%s\n", b)
}

func printText(w io.Writer, r *result) {
	c, l := r.Controller, r.Lighting
	if c != nil {
		fmt.Fprintf(w, "Controller : %s (id %d)\n", c.Model, c.DeviceID)
		fmt.Fprintf(w, "Firmware   : %s\n", c.Firmware)
		link := c.Link
		if link == "" {
			link = "unknown"
		}
		fmt.Fprintf(w, "Link       : %s, %s mode\n", link, map[string]string{vader.ModeXInput: "XInput", vader.ModeDInput: "DInput"}[c.Mode])
		if !c.Supported {
			fmt.Fprintln(w, "This is not reporting as a Vader 4 Pro. The lighting commands use the same Flydigi protocol.")
		}
	}
	if r.Note != "" {
		fmt.Fprintln(w, r.Note)
	}
	if r.Changed {
		fmt.Fprintln(w, "Saved.")
	}
	if l != nil {
		fmt.Fprintf(w, "Lighting   : %s\n", l.Mode)
		fmt.Fprintf(w, "Brightness : %d\n", l.Brightness)
		if l.Speed != nil {
			fmt.Fprintf(w, "Speed      : %.2f\n", *l.Speed)
		}
		if len(l.Zones) > 0 {
			fmt.Fprintf(w, "Zones      : %s\n", strings.Join(l.Zones, " "))
		}
	}
}

func fail(o options, err error) int {
	code, exit, hint := classify(err)
	if o.json {
		printJSON(os.Stdout, result{API: apiVersion, Error: &errorJSON{Code: code, Message: err.Error(), Hint: hint}})
		return exit
	}
	fmt.Fprintf(os.Stderr, "vader4-rgb: %s\n", err)
	if hint != "" && code != "usage" {
		fmt.Fprintf(os.Stderr, "\n%s\n", hint)
	}
	return exit
}
