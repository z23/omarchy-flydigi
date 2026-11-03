// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"errors"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Linux usbfs requests, from <linux/usbdevice_fs.h>.
const (
	usbdevfsBulk             = 0xc0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
	usbdevfsClaimInterface   = 0x8004550f // _IOR('U', 15, unsigned int)
	usbdevfsReleaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	usbdevfsIoctl            = 0xc0105512 // _IOWR('U', 18, struct usbdevfs_ioctl)
	usbdevfsDisconnect       = 0x5516     // _IO('U', 22)
	usbdevfsConnect          = 0x5517     // _IO('U', 23)
)

// XInput mode uses the gamepad interface itself, so xpad has to let go of it
// while the tool talks to the controller. Input stops for that moment.
const (
	xinputInterface = 0
	xinputIn        = 0x81
	xinputOut       = 0x05
)

type usbdevfsBulkTransfer struct {
	ep      uint32
	len     uint32
	timeout uint32 // milliseconds
	_       uint32
	data    uintptr
}

type usbdevfsIoctlArg struct {
	ifno      int32
	ioctlCode int32
	data      uintptr
}

// The request numbers above encode these sizes, which hold on 64-bit Linux.
var (
	_ = [1]struct{}{}[unsafe.Sizeof(usbdevfsBulkTransfer{})-24]
	_ = [1]struct{}{}[unsafe.Sizeof(usbdevfsIoctlArg{})-16]
)

type usbfs struct {
	fd       int
	detached bool
	rbuf     []byte
	wbuf     []byte
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) (int, error) {
	r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return int(r), errno
	}
	return int(r), nil
}

func openUSBFS(node string) (*usbfs, error) {
	fd, err := syscall.Open(node, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, openError(node, err)
	}
	t := &usbfs{fd: fd, rbuf: make([]byte, 64), wbuf: make([]byte, 64)}

	// Ask xpad to let go of the interface. ENODATA means no driver had it.
	if err := t.driverIoctl(usbdevfsDisconnect); err == nil {
		t.detached = true
	} else if !errors.Is(err, syscall.ENODATA) {
		syscall.Close(fd)
		return nil, openError(node, err)
	}
	iface := uint32(xinputInterface)
	if _, err := ioctl(fd, usbdevfsClaimInterface, unsafe.Pointer(&iface)); err != nil {
		t.reattach()
		syscall.Close(fd)
		return nil, openError(node, err)
	}
	return t, nil
}

func (t *usbfs) driverIoctl(code int32) error {
	arg := usbdevfsIoctlArg{ifno: xinputInterface, ioctlCode: code}
	_, err := ioctl(t.fd, usbdevfsIoctl, unsafe.Pointer(&arg))
	return err
}

func (t *usbfs) reattach() {
	if t.detached {
		// EBUSY means a driver is already bound again.
		_ = t.driverIoctl(usbdevfsConnect)
		t.detached = false
	}
}

func (t *usbfs) bulk(ep uint8, buf []byte, timeout time.Duration) (int, error) {
	x := usbdevfsBulkTransfer{
		ep:      uint32(ep),
		len:     uint32(len(buf)),
		timeout: uint32(max(1, timeout.Milliseconds())),
		data:    uintptr(unsafe.Pointer(&buf[0])),
	}
	n, err := ioctl(t.fd, usbdevfsBulk, unsafe.Pointer(&x))
	runtime.KeepAlive(buf)
	return n, err
}

func (t *usbfs) Write(p []byte) error {
	n := copy(t.wbuf, p)
	_, err := t.bulk(xinputOut, t.wbuf[:n], time.Second)
	return err
}

func (t *usbfs) Read(timeout time.Duration) ([]byte, error) {
	n, err := t.bulk(xinputIn, t.rbuf, timeout)
	if errors.Is(err, syscall.ETIMEDOUT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), t.rbuf[:n]...), nil
}

func (t *usbfs) Close() error {
	iface := uint32(xinputInterface)
	_, _ = ioctl(t.fd, usbdevfsReleaseInterface, unsafe.Pointer(&iface))
	t.reattach()
	return syscall.Close(t.fd)
}
