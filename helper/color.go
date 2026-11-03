// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/z23/omarchy-flydigi/helper/internal/vader"
)

var colorNames = map[string]vader.RGB{
	"red":     {R: 255},
	"green":   {G: 255},
	"blue":    {B: 255},
	"white":   {R: 255, G: 255, B: 255},
	"cyan":    {G: 255, B: 255},
	"magenta": {R: 255, B: 255},
	"yellow":  {R: 255, G: 255},
	"orange":  {R: 255, G: 80},
	"purple":  {R: 160, B: 255},
	"pink":    {R: 255, G: 40, B: 120},
	"black":   {},
}

func parseColor(s string) (vader.RGB, error) {
	if c, ok := colorNames[strings.ToLower(s)]; ok {
		return c, nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return vader.RGB{}, fmt.Errorf("color %q is not a name or #RRGGBB", s)
	}
	n, err := strconv.ParseUint(hex, 16, 24)
	if err != nil {
		return vader.RGB{}, fmt.Errorf("color %q is not a name or #RRGGBB", s)
	}
	return vader.RGB{R: byte(n >> 16), G: byte(n >> 8), B: byte(n)}, nil
}
