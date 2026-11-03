// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// fakePad answers like a Vader 4 Pro in XInput mode: 32-byte replies with 0xA5
// at byte 14, mixed in with ordinary input reports.
type fakePad struct {
	stored   []byte // the 490-byte lighting area
	queue    [][]byte
	written  [][]byte
	writeLen int // expected data parcels in the current write
	received []byte
	dropAck  map[int]bool // parcel indexes whose first ack is lost
	info     rawInfo
}

func newFakePad() *fakePad {
	stored := make([]byte, lightingChunks*10)
	copy(stored, []byte{1, 2, 0, 0, 0, 30, 70, 5, 5})
	for i := 9; i < 20; i++ {
		stored[i] = 255
	}
	return &fakePad{
		stored:  stored,
		dropAck: map[int]bool{},
		info:    rawInfo{deviceID: 85, mac: []byte{1, 2, 3, 4}, fwLow: 0x04, fwHigh: 0x61, cpu: 1, connection: 1},
	}
}

func (f *fakePad) reply(cmd byte, body ...byte) {
	p := make([]byte, 32)
	p[14], p[15] = 0xA5, cmd
	copy(p[16:], body)
	// An input report first, which the session must skip.
	f.queue = append(f.queue, make([]byte, 20), p)
}

func (f *fakePad) Write(p []byte) error {
	f.written = append(f.written, append([]byte(nil), p...))
	switch {
	case bytes.Equal(p, xinput.info):
		i := f.info
		f.reply(16, append([]byte{i.deviceID}, append(i.mac, i.fwLow, i.fwHigh, i.battery, i.cpu, i.connection, i.motion)...)...)
	case bytes.Equal(p, xinput.readLighting(0)):
		for k := 0; k < lightingChunks; k++ {
			f.reply(39, append([]byte{byte(k)}, f.stored[k*10:k*10+10]...)...)
		}
	case p[0] == 165 && p[1] == 42:
		f.writeLen = int(p[3])
		f.received = nil
		f.reply(42)
	case p[0] == 165 && p[1] == 41:
		k := int(p[13])
		if f.dropAck[k] {
			delete(f.dropAck, k)
			return nil
		}
		if k == len(f.received)/10 {
			f.received = append(f.received, p[2:12]...)
		}
		f.reply(41, byte(k+1))
		if len(f.received)/10 == f.writeLen {
			copy(f.stored, f.received)
		}
	}
	return nil
}

func (f *fakePad) Read(time.Duration) ([]byte, error) {
	if len(f.queue) == 0 {
		return nil, nil
	}
	p := f.queue[0]
	f.queue = f.queue[1:]
	return p, nil
}

func (f *fakePad) Close() error { return nil }

func TestSessionReadWriteRoundTrip(t *testing.T) {
	pad := newFakePad()
	s := &Session{t: pad, d: xinput}
	ctx := context.Background()

	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Model() != "Vader 4 Pro" || info.Link != "wired" || info.Firmware != "6.1.0.4" || info.MAC != "01:02:03:04" {
		t.Fatalf("info %+v", info)
	}

	l, err := s.ReadLighting(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if l.Brightness != 70 || l.Mode != ModeSolid {
		t.Fatalf("read %+v", l)
	}

	l.SetSolid(RGB{0, 0xe5, 0xff})
	pad.dropAck[3] = true // the session must resend parcel 3
	if err := s.WriteLighting(ctx, l); err != nil {
		t.Fatal(err)
	}
	back, err := s.ReadLighting(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := back.Color(); !ok || c.Hex() != "#00E5FF" {
		t.Fatalf("color after write %v %v", c, ok)
	}
}

type silent struct{}

func (silent) Write([]byte) error                 { return nil }
func (silent) Read(time.Duration) ([]byte, error) { return nil, nil }
func (silent) Close() error                       { return nil }

func TestSessionStopsWhenCanceled(t *testing.T) {
	s := &Session{t: silent{}, d: xinput}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Info(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestDecodeInfoWireless(t *testing.T) {
	// What a Vader 4 Pro on the 2.4 GHz dongle sends.
	i := decodeInfo(rawInfo{deviceID: 85, mac: []byte{0x13, 0x36, 0x31, 0x38}, fwLow: 0x33, fwHigh: 0x69, battery: 2, cpu: 2, connection: 0, motion: 1})
	if i.Link != "wireless" || i.Firmware != "6.9.3.3" || i.MAC != "13:36:31:38" || !i.Supported() {
		t.Fatalf("%+v", i)
	}
	i = decodeInfo(rawInfo{deviceID: 80, fwHigh: 0x10})
	if i.Link != "" || i.Supported() {
		t.Fatalf("%+v", i)
	}
}

func TestDecodeLightingTooShort(t *testing.T) {
	if _, err := DecodeLighting(make([]byte, 19)); err == nil {
		t.Fatal("want error")
	}
}
