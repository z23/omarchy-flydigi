// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.
// Packet layouts come from pipe01/flydigictl.

package vader

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Transport moves raw reports to and from the controller's config interface.
type Transport interface {
	Write(p []byte) error
	// Read returns the next report, or nil and no error when none arrived in time.
	Read(timeout time.Duration) ([]byte, error)
	Close() error
}

type replyKind int

const (
	replyNone replyKind = iota
	replyInfo
	replyDongle
	replyLightingChunk
	replyAck
)

type reply struct {
	kind  replyKind
	info  rawInfo
	fw    [2]byte // dongle firmware, low then high
	index int     // lighting chunk index
	data  []byte  // lighting chunk payload
	ack   int
}

type rawInfo struct {
	deviceID, fwLow, fwHigh, battery, cpu, connection, motion byte
	mac                                                       []byte
}

// dialect is the packet format of one USB mode.
type dialect struct {
	name            string
	info, dongle    []byte
	readLighting    func(configID byte) []byte
	lightingParcels func(data []byte, configID byte) [][]byte
	parse           func(p []byte) reply
}

const lightingChunks = 49

// XInput mode: 15-byte packets on the gamepad interface, with a checksum in the last byte.
var xinput = &dialect{
	name:   "xinput",
	info:   xinputPacket(16),
	dongle: xinputPacket(17),
	readLighting: func(configID byte) []byte {
		return xinputPacket(38, configID)
	},
	lightingParcels: func(data []byte, configID byte) [][]byte {
		return parcels(data, func(n int) []byte {
			return checksummed([]byte{165, 42, configID, byte(n), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		}, func(i int, chunk []byte) []byte {
			p := append([]byte{165, 41}, chunk...)
			return checksummed(append(p, 160, byte(i), 0))
		})
	},
	parse: func(p []byte) reply {
		// Input reports share this endpoint. Flydigi replies carry 0xA5 at byte 14.
		if len(p) < 28 || p[14] != 0xA5 {
			return reply{}
		}
		switch p[15] {
		case 16:
			return reply{kind: replyInfo, info: rawInfo{
				deviceID: p[16], mac: p[17:21], fwLow: p[21], fwHigh: p[22],
				battery: p[23], cpu: p[24], connection: p[25], motion: p[26],
			}}
		case 17:
			return reply{kind: replyDongle, fw: [2]byte{p[16], p[17]}}
		case 39:
			return reply{kind: replyLightingChunk, index: int(p[16]), data: p[17:27]}
		case 41:
			return reply{kind: replyAck, ack: int(p[16])}
		case 42:
			return reply{kind: replyAck, ack: 0}
		}
		return reply{}
	},
}

// DInput mode: HID output report 5 on interface 2, no checksum.
var dinput = &dialect{
	name:   "dinput",
	info:   dinputPacket(236),
	dongle: dinputPacket(17),
	readLighting: func(configID byte) []byte {
		return dinputPacket(229, configID)
	},
	lightingParcels: func(data []byte, configID byte) [][]byte {
		return parcels(data, func(n int) []byte {
			return []byte{5, 231, configID, byte(n), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		}, func(i int, chunk []byte) []byte {
			p := append([]byte{5, 51}, chunk...)
			return append(p, 160, byte(i))
		})
	},
	parse: func(p []byte) reply {
		if len(p) < 16 {
			return reply{}
		}
		switch {
		case p[15] == 229:
			return reply{kind: replyLightingChunk, index: int(p[3]), data: p[5:15]}
		case p[0] == 4 && p[1] == 17:
			return reply{kind: replyDongle, fw: [2]byte{p[2], p[3]}}
		case p[15] == 234 || p[15] == 231 || p[15] == 51:
			return reply{kind: replyAck, ack: int(p[3])}
		case p[15] == 236:
			return reply{kind: replyInfo, info: rawInfo{
				deviceID: p[3], mac: p[5:9], fwLow: p[9], fwHigh: p[10],
				battery: p[11], cpu: p[12], connection: p[13], motion: p[14],
			}}
		}
		return reply{}
	},
}

func xinputPacket(cmd byte, args ...byte) []byte {
	p := make([]byte, 15)
	p[0], p[1] = 165, cmd
	copy(p[2:], args)
	return checksummed(p)
}

func dinputPacket(cmd byte, args ...byte) []byte {
	p := make([]byte, 12)
	p[0], p[1] = 5, cmd
	copy(p[2:], args)
	return p
}

// checksummed stores the byte sum of everything before the last byte in the last byte.
func checksummed(p []byte) []byte {
	var sum byte
	for _, v := range p[:len(p)-1] {
		sum += v
	}
	p[len(p)-1] = sum
	return p
}

// parcels splits data into a start packet followed by ten-byte chunks, the
// last one zero padded.
func parcels(data []byte, start func(n int) []byte, chunk func(i int, data []byte) []byte) [][]byte {
	n := (len(data) + 9) / 10
	out := make([][]byte, 0, n+1)
	out = append(out, start(n))
	for i := 0; i < n; i++ {
		c := make([]byte, 10)
		copy(c, data[i*10:])
		out = append(out, chunk(i, c))
	}
	return out
}

// Info describes the connected controller.
type Info struct {
	DeviceID int
	Firmware string
	MAC      string
	// Link is "wired", "wireless", or "" when the controller does not say.
	Link string
}

// Model names the controller from its protocol device id.
func (i *Info) Model() string {
	switch i.DeviceID {
	case 85:
		return "Vader 4 Pro"
	case 80, 81:
		return "Vader 3 Pro"
	}
	return "Flydigi controller"
}

// Supported reports whether this is the controller the tool is built for.
func (i *Info) Supported() bool { return i.DeviceID == 85 }

func decodeInfo(r rawInfo) *Info {
	fwL, fwL2 := r.fwLow&15, r.fwLow>>4
	fwH, fwH2 := r.fwHigh&15, r.fwHigh>>4
	info := &Info{
		DeviceID: int(r.deviceID),
		Firmware: fmt.Sprintf("%d.%d.%d.%d", fwH2, fwH, fwL2, fwL),
		MAC:      net.HardwareAddr(r.mac).String(),
	}
	wch := r.cpu > 0 || (fwH2 >= 6 && fwH >= 1)
	if wch {
		if r.connection == 1 {
			info.Link = "wired"
		} else {
			info.Link = "wireless"
		}
	}
	return info
}

// ErrNoResponse means the controller stopped answering.
var ErrNoResponse = errors.New("the controller did not answer")

const (
	attempts       = 3
	attemptTimeout = 3 * time.Second
	readSlice      = 100 * time.Millisecond
)

// Session runs request and reply exchanges over one open transport.
type Session struct {
	t     Transport
	d     *dialect
	Debug io.Writer
}

func (s *Session) send(p []byte) error {
	if s.Debug != nil {
		fmt.Fprintf(s.Debug, "%s > %s\n", s.d.name, hex.EncodeToString(p))
	}
	return s.t.Write(p)
}

// await reads reports until match accepts one or the timeout passes.
func (s *Session) await(ctx context.Context, timeout time.Duration, match func(reply) bool) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return false, nil
		}
		p, err := s.t.Read(min(left, readSlice))
		if err != nil {
			return false, err
		}
		if p == nil {
			continue
		}
		r := s.d.parse(p)
		if s.Debug != nil && r.kind != replyNone {
			fmt.Fprintf(s.Debug, "%s < %s\n", s.d.name, hex.EncodeToString(p))
		}
		if r.kind != replyNone && match(r) {
			return true, nil
		}
	}
}

// Info asks the controller who it is.
func (s *Session) Info(ctx context.Context) (*Info, error) {
	for i := 0; i < attempts; i++ {
		if err := s.send(s.d.info); err != nil {
			return nil, err
		}
		var got *Info
		ok, err := s.await(ctx, attemptTimeout, func(r reply) bool {
			if r.kind == replyInfo {
				got = decodeInfo(r.info)
				return true
			}
			return false
		})
		if err != nil {
			return nil, err
		}
		if ok {
			return got, nil
		}
	}
	return nil, fmt.Errorf("read controller info: %w", ErrNoResponse)
}

// ReadLighting reads and decodes the lighting block stored on the controller.
func (s *Session) ReadLighting(ctx context.Context) (*Lighting, error) {
	block, err := s.ReadLightingBlock(ctx)
	if err != nil {
		return nil, err
	}
	return DecodeLighting(block)
}

// ReadLightingBlock reads the raw 490-byte lighting area.
func (s *Session) ReadLightingBlock(ctx context.Context) ([]byte, error) {
	for i := 0; i < attempts; i++ {
		if err := s.send(s.d.readLighting(0)); err != nil {
			return nil, err
		}
		buf := make([]byte, lightingChunks*10)
		next := 0
		ok, err := s.await(ctx, attemptTimeout, func(r reply) bool {
			if r.kind != replyLightingChunk || r.index != next {
				return false
			}
			copy(buf[next*10:], r.data)
			next++
			return next == lightingChunks
		})
		if err != nil {
			return nil, err
		}
		if ok {
			// Let any trailing replies drain before the next request.
			if _, err := s.await(ctx, readSlice, func(reply) bool { return false }); err != nil {
				return nil, err
			}
			return buf, nil
		}
	}
	return nil, fmt.Errorf("read lighting: %w", ErrNoResponse)
}

// WriteLighting stores a lighting block on the controller. Each packet is
// resent up to three times until the controller acknowledges it; the start
// packet is packet 0 and data packet k needs an acknowledgement of at least k+1.
func (s *Session) WriteLighting(ctx context.Context, l *Lighting) error {
	for i, p := range s.d.lightingParcels(l.Encode(), 0) {
		acked := false
		for try := 0; try < attempts && !acked; try++ {
			if err := s.send(p); err != nil {
				return err
			}
			var err error
			acked, err = s.await(ctx, attemptTimeout, func(r reply) bool {
				return r.kind == replyAck && r.ack >= i
			})
			if err != nil {
				return err
			}
		}
		if !acked {
			return fmt.Errorf("write lighting packet %d: %w", i, ErrNoResponse)
		}
	}
	return nil
}
