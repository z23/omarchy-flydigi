// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.
// Rewritten from vader4-rgb in z23/flydigictl, a modified copy of pipe01/flydigictl.

// Command vader4-rgb reads and writes the light bar on a Flydigi Vader 4 Pro.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/z23/omarchy-flydigi/helper/internal/uevent"
	"github.com/z23/omarchy-flydigi/helper/internal/vader"
)

var version = "2.0.0"

const usage = `vader4-rgb sets the light bar on a Flydigi Vader 4 Pro.

Usage:
  vader4-rgb                  show the controller and its lighting
  vader4-rgb COLOR [-b N]     solid color, optionally at brightness N
  vader4-rgb off              turn the light bar off
  vader4-rgb stream [-s S]    factory moving effect at speed S from 0 to 1
  vader4-rgb -b N             change only the brightness, from 0 to 100
  vader4-rgb backup FILE      save the lighting block exactly as stored
  vader4-rgb restore FILE     write a saved lighting block back
  vader4-rgb status           say whether a controller is connected and this
                              user can open it, without opening it
  vader4-rgb watch            print the status as a JSON line now and each
                              time it changes

COLOR is #RRGGBB, RRGGBB, #RGB, or a name: red, green, blue, white, cyan,
magenta, yellow, orange, purple, pink, black.

Options:
  -b, --brightness N   brightness from 0 to 100
  -s, --speed S        stream speed from 0 to 1
      --json           print JSON instead of text
      --debug          print protocol packets on stderr
  -h, --help           show this help
      --version        print the version

The controller must be in XInput mode (hold the circle button and X until the
indicator is white) or DInput mode (circle and A, blue indicator), connected by
USB or the 2.4 GHz dongle. Changes are saved on the controller. In XInput mode
the kernel gamepad driver lets go of the controller while this runs, so games
see it disconnect and reconnect.

Each read or change is recorded in $XDG_STATE_HOME/vader4-rgb/state.json.
backup keeps effects this tool cannot make, such as ones set in Flydigi Space.

Exit status: 0 done, 1 failed, 2 usage, 3 no controller, 4 no permission,
5 busy, 6 the controller did not answer.
`

func main() { os.Exit(run(os.Args[1:])) }

func run(argv []string) int {
	o, err := parseArgs(argv)
	if err != nil {
		return fail(o, err)
	}
	if o.help || (len(o.args) == 1 && o.args[0] == "help") {
		fmt.Print(usage)
		return 0
	}
	if o.version {
		fmt.Printf("vader4-rgb %s (omarchy-flydigi, api %d)\n", version, apiVersion)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	if len(o.args) > 0 && (o.args[0] == "backup" || o.args[0] == "restore") {
		if len(o.args) != 2 || o.brightness >= 0 || o.speed >= 0 {
			return fail(o, usagef("%s takes one file name", o.args[0]))
		}
		var r *result
		if o.args[0] == "backup" {
			r, err = runBackup(ctx, o, o.args[1])
		} else {
			r, err = runRestore(ctx, o, o.args[1])
		}
		if err != nil {
			return fail(o, err)
		}
		if o.args[1] == "-" {
			return 0 // stdout already holds the backup
		}
		return report(o, r)
	}
	if len(o.args) > 0 && (o.args[0] == "status" || o.args[0] == "watch") {
		if len(o.args) > 1 || o.brightness >= 0 || o.speed >= 0 {
			return fail(o, usagef("%s takes no other arguments", o.args[0]))
		}
		if o.args[0] == "watch" {
			return runWatch()
		}
		return runStatus(o)
	}

	req, err := parseRequest(o)
	if err != nil {
		return fail(o, err)
	}
	r, err := runLighting(ctx, o, req)
	if err != nil {
		return fail(o, err)
	}
	return report(o, r)
}

func report(o options, r *result) int {
	if o.json {
		printJSON(os.Stdout, r)
	} else {
		printText(os.Stdout, r)
	}
	return 0
}

type request struct {
	off, stream, color bool
	rgb                vader.RGB
	brightness         int
	speed              float64
}

func (r request) changing() bool { return r.off || r.stream || r.color || r.brightness >= 0 }

func parseRequest(o options) (request, error) {
	req := request{brightness: o.brightness, speed: o.speed}
	switch len(o.args) {
	case 0:
	case 1:
		switch o.args[0] {
		case "off":
			req.off = true
		case "stream", "streamlined":
			req.stream = true
		default:
			c, err := parseColor(o.args[0])
			if err != nil {
				return req, usageError{err.Error()}
			}
			req.color, req.rgb = true, c
		}
	default:
		return req, usagef("expected one color or command, got %d", len(o.args))
	}
	if req.speed >= 0 && !req.stream {
		return req, usagef("--speed only applies to stream")
	}
	return req, nil
}

// withController finds the controller, locks it against other copies of this
// tool, opens it, and asks who it is before handing it to fn.
func withController(ctx context.Context, o options, fn func(context.Context, *vader.Session, *result) error) (*result, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	devs, err := vader.Discover(vader.DefaultRoots)
	if err != nil {
		return nil, err
	}
	if len(devs) == 0 {
		return nil, vader.ErrNotFound
	}
	d := devs[0]
	if !d.Access {
		return nil, fmt.Errorf("%w (%s)", vader.ErrNoAccess, d.Node)
	}
	unlock, err := lockController(10 * time.Second)
	if err != nil {
		return nil, err
	}
	defer unlock()

	s, err := vader.Open(d)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if o.debug {
		s.Debug = os.Stderr
	}

	info, err := s.Info(ctx)
	if err != nil {
		return nil, err
	}
	r := &result{API: apiVersion, OK: true, Controller: describeController(info, d)}
	if err := fn(ctx, s, r); err != nil {
		return nil, err
	}
	r.UpdatedAt = time.Now().Format(time.RFC3339)
	if r.Lighting != nil {
		if err := saveState(r); err != nil && o.debug {
			fmt.Fprintf(os.Stderr, "vader4-rgb: record state: %s\n", err)
		}
	}
	return r, nil
}

// writeAndVerify stores l and reads it back. Once packets are going out it
// finishes even if asked to stop, so the controller is not left with half a
// lighting block.
func writeAndVerify(ctx context.Context, s *vader.Session, l *vader.Lighting, r *result) (*vader.Lighting, error) {
	wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	err := s.WriteLighting(wctx, l)
	wcancel()
	if err != nil {
		return nil, err
	}
	r.Changed = true
	back, err := s.ReadLighting(ctx)
	if err != nil {
		r.Note = "Sent. The controller did not send the new setting back."
		return l, nil
	}
	return back, nil
}

func runLighting(ctx context.Context, o options, req request) (*result, error) {
	return withController(ctx, o, func(ctx context.Context, s *vader.Session, r *result) error {
		l, err := s.ReadLighting(ctx)
		if err != nil {
			return err
		}
		if req.changing() {
			applyRequest(l, req, r)
			if l, err = writeAndVerify(ctx, s, l, r); err != nil {
				return err
			}
		}
		r.Lighting = describeLighting(l)
		return nil
	})
}

type backupFile struct {
	Tool     string `json:"tool"`
	Model    string `json:"model"`
	DeviceID int    `json:"deviceId"`
	Firmware string `json:"firmware"`
	MAC      string `json:"mac"`
	SavedAt  string `json:"savedAt"`
	// Lighting is the raw lighting area, hex encoded.
	Lighting string `json:"lighting"`
}

func runBackup(ctx context.Context, o options, path string) (*result, error) {
	return withController(ctx, o, func(ctx context.Context, s *vader.Session, r *result) error {
		block, err := s.ReadLightingBlock(ctx)
		if err != nil {
			return err
		}
		l, err := vader.DecodeLighting(block)
		if err != nil {
			return err
		}
		c := r.Controller
		b, _ := json.MarshalIndent(backupFile{
			Tool: "vader4-rgb", Model: c.Model, DeviceID: c.DeviceID, Firmware: c.Firmware, MAC: c.MAC,
			SavedAt: time.Now().Format(time.RFC3339), Lighting: hex.EncodeToString(block),
		}, "", "  ")
		if path == "-" {
			_, err = os.Stdout.Write(append(b, '\n'))
		} else {
			err = os.WriteFile(path, append(b, '\n'), 0o644)
		}
		if err != nil {
			return err
		}
		r.Lighting = describeLighting(l)
		if path != "-" {
			r.Note = "Saved the lighting to " + path + "."
		}
		return nil
	})
}

func runRestore(ctx context.Context, o options, path string) (*result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bf backupFile
	if err := json.Unmarshal(raw, &bf); err != nil || bf.Tool != "vader4-rgb" {
		return nil, usagef("%s is not a vader4-rgb backup", path)
	}
	block, err := hex.DecodeString(bf.Lighting)
	if err != nil {
		return nil, usagef("%s has a damaged lighting block", path)
	}
	saved, err := vader.DecodeLighting(block)
	if err != nil {
		return nil, usagef("%s: %s", path, err)
	}
	return withController(ctx, o, func(ctx context.Context, s *vader.Session, r *result) error {
		if r.Controller.DeviceID != bf.DeviceID {
			return usagef("%s was saved from a %s (id %d), and this is a %s (id %d)", path, bf.Model, bf.DeviceID, r.Controller.Model, r.Controller.DeviceID)
		}
		back, err := writeAndVerify(ctx, s, saved, r)
		if err != nil {
			return err
		}
		if r.Note == "" && !bytesEqualPrefix(back.Encode(), saved.Encode()) {
			r.Note = "Written, but the controller sent back different lighting."
		}
		r.Lighting = describeLighting(back)
		return nil
	})
}

func bytesEqualPrefix(a, b []byte) bool {
	n := min(len(a), len(b))
	return string(a[:n]) == string(b[:n])
}

func applyRequest(l *vader.Lighting, req request, r *result) {
	if req.brightness >= 0 {
		l.Brightness = byte(req.brightness)
	}
	if (req.color || req.stream) && req.brightness < 0 && l.Brightness == 0 {
		l.Brightness = 100
		r.Note = "Brightness was 0, so it was raised to 100."
	}
	switch {
	case req.off:
		l.SetOff()
	case req.color:
		l.SetSolid(req.rgb)
	case req.stream:
		speed := 0.5
		if req.speed >= 0 {
			speed = req.speed
		} else if l.LoopTime <= 100 {
			// The solid preset leaves the stream period alone, so this is the
			// speed the effect last ran at.
			speed = float64(100-l.LoopTime) / 100
		}
		l.SetStream(speed)
	}
}

func currentStatus() statusJSON {
	st := statusJSON{API: apiVersion}
	devs, err := vader.Discover(vader.DefaultRoots)
	if err != nil || len(devs) == 0 {
		return st
	}
	d := devs[0]
	st.Present, st.Access, st.Mode, st.Port, st.Node, st.Product = true, d.Access, d.Mode, d.Port, d.Node, d.Product
	return st
}

func runStatus(o options) int {
	st := currentStatus()
	if o.json {
		printJSON(os.Stdout, st)
	} else if !st.Present {
		fmt.Println("No Flydigi controller in XInput or DInput mode is connected.")
	} else {
		access := "yes"
		if !st.Access {
			access = "no, install the udev rule"
		}
		fmt.Printf("Controller : %s, %s mode, USB port %s\n", st.Product, st.Mode, st.Port)
		fmt.Printf("Device     : %s\n", st.Node)
		fmt.Printf("Access     : %s\n", access)
	}
	switch {
	case !st.Present:
		return exitNotFound
	case !st.Access:
		return exitNoAccess
	}
	return 0
}

// runWatch prints the status now and again whenever a USB or hidraw event
// changes it. It runs until killed or until stdout goes away.
func runWatch() int {
	// Exit with the process that started this one, such as the Omarchy shell.
	_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0)

	last := ""
	emit := func() {
		b, _ := json.Marshal(currentStatus())
		if string(b) == last {
			return
		}
		last = string(b)
		if _, err := fmt.Fprintf(os.Stdout, "%s\n", b); err != nil {
			os.Exit(exitFailure)
		}
	}
	emit()

	l, err := uevent.Listen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vader4-rgb: udev events unavailable (%s); checking every 3 seconds\n", err)
	}
	for {
		if l == nil {
			time.Sleep(3 * time.Second)
			emit()
			continue
		}
		got, err := l.Wait(time.Hour)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vader4-rgb: udev events stopped (%s); checking every 3 seconds\n", err)
			l.Close()
			l = nil
			continue
		}
		if !got {
			continue
		}
		// One plug sends a burst of events, and the pad can re-enumerate from
		// DInput to XInput a second later. Let the burst settle first.
		for {
			more, err := l.Wait(400 * time.Millisecond)
			if err != nil || !more {
				break
			}
		}
		emit()
	}
}
