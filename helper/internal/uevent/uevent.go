// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

// Package uevent listens for device events from udev, so hotplug can be
// followed without polling.
package uevent

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"time"
	"unsafe"
)

// udev rebroadcasts kernel events on group 2 after its rules have run, so by the
// time a message arrives the device node already has its final permissions.
const udevGroup = 2

// Listener receives udev events.
type Listener struct {
	fd  int
	buf []byte
}

// Listen subscribes to udev events. It needs no privileges.
func Listen() (*Listener, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: udevGroup}); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &Listener{fd: fd, buf: make([]byte, 64*1024)}, nil
}

// Wait blocks until an event for a USB device or a hidraw node arrives, or the
// timeout passes. It reports whether such an event arrived.
func (l *Listener) Wait(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return false, nil
		}
		var set syscall.FdSet
		word := int(unsafe.Sizeof(set.Bits[0])) * 8
		set.Bits[l.fd/word] |= 1 << (uint(l.fd) % uint(word))
		tv := syscall.NsecToTimeval(left.Nanoseconds())
		n, err := syscall.Select(l.fd+1, &set, nil, nil, &tv)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
		m, _, err := syscall.Recvfrom(l.fd, l.buf, 0)
		if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.ENOBUFS) {
			// ENOBUFS: events were dropped. Report one so the caller rescans.
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if Relevant(l.buf[:m]) {
			return true, nil
		}
	}
}

// Relevant reports whether a udev message is about a USB device or hidraw node.
func Relevant(msg []byte) bool {
	props := msg
	// A udev message starts with "libudev\0", a magic number, the header size,
	// and the offset of its NUL-separated KEY=VALUE properties.
	if bytes.HasPrefix(msg, []byte("libudev\x00")) && len(msg) >= 20 {
		off := int(binary.LittleEndian.Uint32(msg[16:20]))
		if off <= 0 || off > len(msg) {
			return false
		}
		props = msg[off:]
	}
	for _, kv := range bytes.Split(props, []byte{0}) {
		if bytes.Equal(kv, []byte("SUBSYSTEM=usb")) || bytes.Equal(kv, []byte("SUBSYSTEM=hidraw")) {
			return true
		}
	}
	return false
}

// Close stops listening.
func (l *Listener) Close() error { return syscall.Close(l.fd) }
