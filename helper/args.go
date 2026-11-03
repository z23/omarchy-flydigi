// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

type options struct {
	json       bool
	debug      bool
	help       bool
	version    bool
	brightness int     // -1 keeps the current brightness
	speed      float64 // -1 keeps the current speed
	args       []string
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

// parseArgs accepts flags before, between, and after the positional words, as
// in `vader4-rgb cyan -b 70`.
func parseArgs(argv []string) (options, error) {
	o := options{brightness: -1, speed: -1}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			o.args = append(o.args, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			o.args = append(o.args, a)
			continue
		}
		name, value, hasValue := strings.Cut(a, "=")
		if !hasValue && len(name) > 2 && name[1] != '-' {
			// -b70
			name, value, hasValue = name[:2], name[2:], true
		}
		takeValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(argv) {
				return "", usagef("%s needs a value", name)
			}
			i++
			return argv[i], nil
		}
		switch name {
		case "--json":
			o.json = true
		case "--debug":
			o.debug = true
		case "-h", "--help":
			o.help = true
		case "--version":
			o.version = true
		case "-b", "--brightness":
			v, err := takeValue()
			if err != nil {
				return o, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 100 {
				return o, usagef("brightness must be a whole number from 0 to 100, not %q", v)
			}
			o.brightness = n
		case "-s", "--speed":
			v, err := takeValue()
			if err != nil {
				return o, err
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0 || f > 1 {
				return o, usagef("speed must be from 0 to 1, not %q", v)
			}
			o.speed = f
		default:
			return o, usagef("unknown option %s", name)
		}
		if hasValue && name != "-b" && name != "--brightness" && name != "-s" && name != "--speed" {
			return o, usagef("%s takes no value", name)
		}
	}
	return o, nil
}
