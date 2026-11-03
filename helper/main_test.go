// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/z23/omarchy-flydigi/helper/internal/vader"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		argv []string
		want options
		err  bool
	}{
		{argv: nil, want: options{brightness: -1, speed: -1}},
		{argv: []string{"cyan", "-b", "70"}, want: options{brightness: 70, speed: -1, args: []string{"cyan"}}},
		{argv: []string{"--json", "#00e5ff", "--brightness=40"}, want: options{json: true, brightness: 40, speed: -1, args: []string{"#00e5ff"}}},
		{argv: []string{"stream", "-s0.25"}, want: options{brightness: -1, speed: 0.25, args: []string{"stream"}}},
		{argv: []string{"-b", "101"}, err: true},
		{argv: []string{"-b"}, err: true},
		{argv: []string{"-s", "2", "stream"}, err: true},
		{argv: []string{"--json=yes"}, err: true},
		{argv: []string{"--colour"}, err: true},
		{argv: []string{"--", "-b"}, want: options{brightness: -1, speed: -1, args: []string{"-b"}}},
	}
	for _, c := range cases {
		got, err := parseArgs(c.argv)
		if c.err {
			if err == nil {
				t.Errorf("%v: want error", c.argv)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.argv, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.argv, got, c.want)
		}
	}
}

func TestParseRequest(t *testing.T) {
	r, err := parseRequest(options{brightness: -1, speed: -1, args: []string{"orange"}})
	if err != nil || !r.color || r.rgb != (vader.RGB{R: 255, G: 80}) {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := parseRequest(options{brightness: -1, speed: 0.5, args: []string{"red"}}); err == nil {
		t.Fatal("speed without stream should fail")
	}
	if _, err := parseRequest(options{brightness: -1, speed: -1, args: []string{"red", "blue"}}); err == nil {
		t.Fatal("two colors should fail")
	}
	if r, _ := parseRequest(options{brightness: 30, speed: -1}); !r.changing() {
		t.Fatal("brightness alone is a change")
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]vader.RGB{
		"#00e5ff": {G: 0xe5, B: 0xff},
		"00E5FF":  {G: 0xe5, B: 0xff},
		"#0ef":    {G: 0xee, B: 0xff},
		"Purple":  {R: 160, B: 255},
	} {
		got, err := parseColor(in)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"", "#12345", "#gggggg", "teal"} {
		if _, err := parseColor(in); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestApplyRequest(t *testing.T) {
	l := &vader.Lighting{Brightness: 0, LoopTime: 16, ZoneCount: 5}
	r := &result{}
	applyRequest(l, request{stream: true, brightness: -1, speed: -1}, r)
	if l.Brightness != 100 || r.Note == "" {
		t.Fatalf("brightness %d note %q", l.Brightness, r.Note)
	}
	if s, _ := l.Speed(); s != 0.84 {
		t.Fatalf("speed %v, want the last stream speed 0.84", s)
	}

	l = &vader.Lighting{Brightness: 0}
	r = &result{}
	applyRequest(l, request{color: true, rgb: vader.RGB{R: 1}, brightness: 0, speed: -1}, r)
	if l.Brightness != 0 || r.Note != "" {
		t.Fatalf("an explicit -b 0 must stay 0, got %d", l.Brightness)
	}
}

func TestClassify(t *testing.T) {
	for err, want := range map[error]int{
		vader.ErrNotFound:   exitNotFound,
		vader.ErrNoAccess:   exitNoAccess,
		vader.ErrBusy:       exitBusy,
		vader.ErrNoResponse: exitNoResponse,
		usagef("x"):         exitUsage,
		errors.New("other"): exitFailure,
	} {
		if _, got, _ := classify(err); got != want {
			t.Errorf("%v: exit %d, want %d", err, got, want)
		}
	}
}

func TestRestoreRejectsBadFilesBeforeOpeningAnything(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"not-json":   "hello",
		"other-tool": `{"tool":"something","lighting":"00"}`,
		"bad-hex":    `{"tool":"vader4-rgb","lighting":"zz"}`,
		"too-short":  `{"tool":"vader4-rgb","lighting":"0102"}`,
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := runRestore(context.Background(), options{}, path)
		var u usageError
		if !errors.As(err, &u) {
			t.Errorf("%s: want a usage error, got %v", name, err)
		}
	}
	if _, err := runRestore(context.Background(), options{}, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file: want error")
	}
}
