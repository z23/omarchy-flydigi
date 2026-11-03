// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// USB modes the configuration protocol works in. Switch mode and Bluetooth do not speak it.
const (
	ModeXInput = "xinput"
	ModeDInput = "dinput"
)

// Device is a connected controller found in sysfs. Finding one never opens it.
type Device struct {
	Mode    string `json:"mode"`
	Port    string `json:"port"`
	Node    string `json:"node"`
	Product string `json:"product"`
	Access  bool   `json:"access"`
}

// Roots lets tests point discovery at a fake sysfs and /dev.
type Roots struct {
	Sys string
	Dev string
}

// DefaultRoots are the real system paths.
var DefaultRoots = Roots{Sys: "/sys", Dev: "/dev"}

var (
	// ErrNotFound means no controller is connected in a mode this tool can talk to.
	ErrNotFound = errors.New("no Flydigi controller in XInput or DInput mode is connected")
	// ErrNoAccess means the controller is connected but this user cannot open it.
	ErrNoAccess = errors.New("this user cannot open the controller")
	// ErrBusy means another program holds the controller's config interface.
	ErrBusy = errors.New("another program is using the controller")
)

// Discover lists connected Flydigi controllers, XInput first.
func Discover(r Roots) ([]Device, error) {
	ports, err := filepath.Glob(filepath.Join(r.Sys, "bus/usb/devices/*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(ports)
	var out []Device
	for _, dir := range ports {
		port := filepath.Base(dir)
		if strings.Contains(port, ":") {
			continue // an interface, not a device
		}
		id := sysText(dir, "idVendor") + ":" + sysText(dir, "idProduct")
		if id != "045e:028e" && id != "04b4:2412" {
			continue
		}
		label := strings.TrimSpace(sysText(dir, "manufacturer") + " " + sysText(dir, "product"))
		lower := strings.ToLower(label)
		// 045e:028e is also a real Xbox 360 pad and 04b4 is Cypress; only take Flydigi ones.
		if !strings.Contains(lower, "flydigi") && !strings.Contains(lower, "vader") {
			continue
		}
		d := Device{Port: port, Product: sysText(dir, "product")}
		if id == "045e:028e" {
			bus, _ := strconv.Atoi(sysText(dir, "busnum"))
			dev, _ := strconv.Atoi(sysText(dir, "devnum"))
			if bus == 0 || dev == 0 {
				continue
			}
			d.Mode = ModeXInput
			d.Node = filepath.Join(r.Dev, fmt.Sprintf("bus/usb/%03d/%03d", bus, dev))
		} else {
			node := dinputHidraw(r, dir, port)
			if node == "" {
				continue
			}
			d.Mode = ModeDInput
			d.Node = node
		}
		d.Access = syscall.Access(d.Node, 6) == nil // read and write
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Mode == ModeXInput && out[j].Mode != ModeXInput })
	return out, nil
}

// dinputHidraw finds the hidraw node of the config interface, interface 2.
func dinputHidraw(r Roots, dir, port string) string {
	ifaces, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), port+":*"))
	for _, iface := range ifaces {
		if sysText(iface, "bInterfaceNumber") != "02" {
			continue
		}
		nodes, _ := filepath.Glob(filepath.Join(iface, "*", "hidraw", "hidraw*"))
		if len(nodes) > 0 {
			return filepath.Join(r.Dev, filepath.Base(nodes[0]))
		}
	}
	return ""
}

func sysText(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Open claims the controller's config interface and starts a session.
func Open(d Device) (*Session, error) {
	var (
		t   Transport
		err error
		dia *dialect
	)
	switch d.Mode {
	case ModeXInput:
		t, err = openUSBFS(d.Node)
		dia = xinput
	case ModeDInput:
		t, err = openHidraw(d.Node)
		dia = dinput
	default:
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Session{t: t, d: dia}, nil
}

// Close releases the controller and hands it back to its kernel driver.
func (s *Session) Close() error { return s.t.Close() }

func openError(node string, err error) error {
	switch {
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return fmt.Errorf("%w (%s)", ErrNoAccess, node)
	case errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ENODEV):
		return ErrNotFound
	case errors.Is(err, syscall.EBUSY):
		return ErrBusy
	}
	return fmt.Errorf("open %s: %w", node, err)
}
