// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"errors"
	"syscall"
	"time"
	"unsafe"
)

// DInput mode has a separate HID config interface, so the gamepad keeps working
// while the tool talks to it.
type hidraw struct {
	fd  int
	buf []byte
}

func openHidraw(node string) (*hidraw, error) {
	fd, err := syscall.Open(node, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, openError(node, err)
	}
	return &hidraw{fd: fd, buf: make([]byte, 64)}, nil
}

func (h *hidraw) Write(p []byte) error {
	_, err := syscall.Write(h.fd, p)
	return err
}

func (h *hidraw) Read(timeout time.Duration) ([]byte, error) {
	var set syscall.FdSet
	word := int(unsafe.Sizeof(set.Bits[0])) * 8
	set.Bits[h.fd/word] |= 1 << (uint(h.fd) % uint(word))
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())
	n, err := syscall.Select(h.fd+1, &set, nil, nil, &tv)
	if errors.Is(err, syscall.EINTR) || n == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := syscall.Read(h.fd, h.buf)
	if errors.Is(err, syscall.EAGAIN) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), h.buf[:m]...), nil
}

func (h *hidraw) Close() error { return syscall.Close(h.fd) }
