// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package uevent

import (
	"encoding/binary"
	"testing"
)

func udevMessage(props string) []byte {
	header := make([]byte, 40)
	copy(header, "libudev\x00")
	binary.BigEndian.PutUint32(header[8:], 0xfeedcafe)
	binary.LittleEndian.PutUint32(header[12:], 40)
	binary.LittleEndian.PutUint32(header[16:], 40)
	binary.LittleEndian.PutUint32(header[20:], uint32(len(props)))
	// A non-zero byte right before the properties, as the bloom filter can be.
	header[39] = 0x7f
	return append(header, props...)
}

func TestRelevant(t *testing.T) {
	cases := map[string]bool{
		"SUBSYSTEM=usb\x00ACTION=add\x00DEVTYPE=usb_device\x00":         true,
		"ACTION=remove\x00SUBSYSTEM=hidraw\x00DEVNAME=/dev/hidraw5\x00": true,
		"ACTION=add\x00SUBSYSTEM=usbmisc\x00":                           false,
		"ACTION=change\x00SUBSYSTEM=power_supply\x00":                   false,
	}
	for props, want := range cases {
		if got := Relevant(udevMessage(props)); got != want {
			t.Errorf("udev %q: %v", props, got)
		}
	}
	// Kernel-format messages, in case one reaches the socket.
	if !Relevant([]byte("add@/devices/usb3/3-8\x00ACTION=add\x00SUBSYSTEM=usb\x00")) {
		t.Error("kernel message")
	}
	if Relevant([]byte("libudev\x00short")) {
		t.Error("truncated header")
	}
}
