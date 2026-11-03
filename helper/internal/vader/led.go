// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.
// The lighting layout and presets come from pipe01/flydigictl.

package vader

import (
	"fmt"
	"math"
)

// RGB is one LED color.
type RGB struct{ R, G, B uint8 }

// Hex formats the color as #RRGGBB.
func (c RGB) Hex() string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

// Mode is the light bar effect stored on the controller.
type Mode uint8

const (
	ModeOff Mode = iota
	ModeStream
	ModeBreathing
	ModeGradient
	ModeFeedback
	ModeSolid
)

var modeNames = [...]string{"off", "stream", "breathing", "gradient", "feedback", "solid"}

func (m Mode) String() string {
	if int(m) < len(modeNames) {
		return modeNames[m]
	}
	return fmt.Sprintf("mode-%d", m)
}

const (
	lightingHeaderLen = 20
	framesPerZone     = 10
	presetZoneSlots   = 16
	// A 490-byte read holds 15 whole zones; the sixteenth is cut short.
	readZones = 15
	// The Vader 4 Pro light bar has five zones.
	LightBarZones = 5
)

// Lighting is the controller's LED configuration block. Zones holds the
// animation frames of each zone; a nil zone is not sent at all, which is how
// Flydigi's own steady preset is laid out.
type Lighting struct {
	Version    [2]byte
	Type       byte
	LoopStart  byte
	LoopEnd    byte
	LoopTime   byte // stream effect period: 100 - speed*100
	Brightness byte // 0 to 100
	ZoneCount  byte
	Mode       Mode
	Reserve    [11]byte
	Zones      [][]RGB
}

// DecodeLighting parses the block the controller sends for a lighting read.
func DecodeLighting(data []byte) (*Lighting, error) {
	if len(data) < lightingHeaderLen {
		return nil, fmt.Errorf("lighting block is %d bytes, want at least %d", len(data), lightingHeaderLen)
	}
	l := &Lighting{
		Type:       data[2],
		LoopStart:  data[3],
		LoopEnd:    data[4],
		LoopTime:   data[5],
		Brightness: data[6],
		ZoneCount:  data[7],
		Mode:       Mode(data[8]),
	}
	copy(l.Version[:], data[:2])
	copy(l.Reserve[:], data[9:lightingHeaderLen])
	for off := lightingHeaderLen; len(l.Zones) < readZones && off+framesPerZone*3 <= len(data); off += framesPerZone * 3 {
		zone := make([]RGB, framesPerZone)
		for j := range zone {
			p := off + j*3
			zone[j] = RGB{data[p], data[p+1], data[p+2]}
		}
		l.Zones = append(l.Zones, zone)
	}
	return l, nil
}

// Encode lays the block out the way the controller expects it on a write.
func (l *Lighting) Encode() []byte {
	out := make([]byte, 0, lightingHeaderLen+len(l.Zones)*framesPerZone*3)
	out = append(out, l.Version[0], l.Version[1], l.Type, l.LoopStart, l.LoopEnd, l.LoopTime, l.Brightness, l.ZoneCount, byte(l.Mode))
	out = append(out, l.Reserve[:]...)
	for _, zone := range l.Zones {
		for _, c := range zone {
			out = append(out, c.R, c.G, c.B)
		}
	}
	return out
}

// SetSolid applies Flydigi's steady preset: the color in the first frame of
// each light bar zone, and no data for the zones after them.
func (l *Lighting) SetSolid(c RGB) {
	l.Mode = ModeSolid
	l.ZoneCount = LightBarZones
	l.LoopEnd = 0
	l.Type = 0
	l.Zones = make([][]RGB, presetZoneSlots)
	for i := 0; i < LightBarZones; i++ {
		zone := make([]RGB, framesPerZone)
		zone[0] = c
		l.Zones[i] = zone
	}
}

// SetStream applies the factory moving effect at a speed from 0 to 1.
func (l *Lighting) SetStream(speed float64) {
	scaled := int(math.Round(speed * 100))
	scaled = max(0, min(100, scaled))
	l.Mode = ModeStream
	l.LoopEnd = 5
	l.LoopTime = byte(100 - scaled)
	l.ZoneCount = LightBarZones
	l.Zones = make([][]RGB, presetZoneSlots)
	for i := range l.Zones {
		zone := make([]RGB, framesPerZone)
		if i < len(streamFrames) {
			copy(zone, streamFrames[i][:])
		}
		l.Zones[i] = zone
	}
}

// SetOff turns the light bar off and blanks the first frame of each zone.
func (l *Lighting) SetOff() {
	l.Mode = ModeOff
	for _, zone := range l.Zones[:l.activeZones()] {
		if len(zone) > 0 {
			zone[0] = RGB{}
		}
	}
}

func (l *Lighting) activeZones() int {
	n := int(l.ZoneCount)
	if n <= 0 || n > len(l.Zones) {
		n = len(l.Zones)
	}
	return n
}

// ZoneColors returns the first frame of each light bar zone.
func (l *Lighting) ZoneColors() []RGB {
	n := min(l.activeZones(), LightBarZones)
	out := make([]RGB, 0, n)
	for _, zone := range l.Zones[:n] {
		if len(zone) == 0 {
			out = append(out, RGB{})
			continue
		}
		out = append(out, zone[0])
	}
	return out
}

// Color returns the light bar color when every zone shows the same one.
func (l *Lighting) Color() (RGB, bool) {
	zones := l.ZoneColors()
	if len(zones) == 0 {
		return RGB{}, false
	}
	for _, c := range zones[1:] {
		if c != zones[0] {
			return RGB{}, false
		}
	}
	return zones[0], true
}

// Speed returns the stream effect speed from 0 to 1.
func (l *Lighting) Speed() (float64, bool) {
	if l.Mode != ModeStream || l.LoopTime > 100 {
		return 0, false
	}
	return float64(100-l.LoopTime) / 100, true
}

// The first five zones of Flydigi's moving rainbow preset.
var streamFrames = [LightBarZones][framesPerZone]RGB{
	{{50, 50, 0}, {0, 100, 0}, {0, 50, 50}, {0, 0, 100}, {50, 0, 50}, {100, 0, 0}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}},
	{{24, 76, 0}, {0, 76, 24}, {0, 0, 100}, {24, 0, 76}, {76, 0, 24}, {76, 24, 0}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}},
	{{0, 100, 0}, {0, 50, 50}, {0, 50, 50}, {50, 0, 50}, {100, 0, 0}, {50, 50, 0}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}},
	{{100, 0, 0}, {50, 50, 0}, {0, 100, 0}, {0, 50, 50}, {0, 0, 100}, {50, 0, 50}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}},
	{{76, 24, 0}, {24, 76, 0}, {0, 76, 24}, {0, 76, 24}, {24, 0, 76}, {76, 0, 24}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}, {0, 116, 255}},
}
