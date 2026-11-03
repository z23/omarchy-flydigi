// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/z23/omarchy-flydigi/helper/internal/vader"
)

// stateDir holds state.json, the last lighting this tool read or wrote. The
// Omarchy plugin watches it, so a change made from a terminal shows up in the
// bar without the plugin opening the controller.
func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "vader4-rgb")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "vader4-rgb")
}

func saveState(r *result) error {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "state.json"))
}

// lockController serializes access to the controller between processes, so a
// click in the bar and a terminal command cannot interleave packets.
func lockController(timeout time.Duration) (func(), error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, fmt.Sprintf("vader4-rgb-%d.lock", os.Getuid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, vader.ErrBusy
			}
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
