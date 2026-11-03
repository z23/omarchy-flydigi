// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	r := Roots{Sys: filepath.Join(root, "sys"), Dev: filepath.Join(root, "dev")}
	usb := filepath.Join(r.Sys, "bus/usb/devices")

	// A Flydigi pad in XInput mode, with an open device node.
	writeFiles(t, filepath.Join(usb, "3-8"), map[string]string{
		"idVendor": "045e", "idProduct": "028e", "manufacturer": "Flydigi", "product": "Flydigi VADER3", "busnum": "3", "devnum": "12",
	})
	writeFiles(t, filepath.Join(r.Dev, "bus/usb/003"), map[string]string{"012": ""})

	// A real Xbox 360 pad: same ids, not Flydigi.
	writeFiles(t, filepath.Join(usb, "1-1"), map[string]string{
		"idVendor": "045e", "idProduct": "028e", "manufacturer": "©Microsoft Corporation", "product": "Controller", "busnum": "1", "devnum": "2",
	})

	// A Flydigi pad in DInput mode whose node this user cannot open.
	writeFiles(t, filepath.Join(usb, "2-4"), map[string]string{
		"idVendor": "04b4", "idProduct": "2412", "manufacturer": "Flydigi", "product": "Flydigi VADER3",
	})
	writeFiles(t, filepath.Join(usb, "2-4:1.0"), map[string]string{"bInterfaceNumber": "00"})
	writeFiles(t, filepath.Join(usb, "2-4:1.0", "0003:04B4:2412.0001", "hidraw", "hidraw3"), nil)
	writeFiles(t, filepath.Join(usb, "2-4:1.2"), map[string]string{"bInterfaceNumber": "02"})
	writeFiles(t, filepath.Join(usb, "2-4:1.2", "0003:04B4:2412.0003", "hidraw", "hidraw5"), nil)
	writeFiles(t, r.Dev, map[string]string{"hidraw5": ""})
	if err := os.Chmod(filepath.Join(r.Dev, "hidraw5"), 0o000); err != nil {
		t.Fatal(err)
	}

	devs, err := Discover(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("found %+v", devs)
	}
	x, d := devs[0], devs[1]
	if x.Mode != ModeXInput || x.Port != "3-8" || x.Node != filepath.Join(r.Dev, "bus/usb/003/012") || !x.Access {
		t.Fatalf("xinput %+v", x)
	}
	if d.Mode != ModeDInput || d.Node != filepath.Join(r.Dev, "hidraw5") {
		t.Fatalf("dinput %+v", d)
	}
	if os.Geteuid() != 0 && d.Access {
		t.Fatalf("dinput node should not be accessible: %+v", d)
	}
}

func TestDiscoverNothing(t *testing.T) {
	devs, err := Discover(Roots{Sys: t.TempDir(), Dev: t.TempDir()})
	if err != nil || len(devs) != 0 {
		t.Fatalf("%v %v", devs, err)
	}
}
