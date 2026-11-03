// Copyright (C) 2026 z23. Part of omarchy-flydigi, released under GPL-3.0-only.

package vader

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// testdata/golden.json was produced by the pipe01/flydigictl implementation
// this package replaces, so these tests pin the rewrite to the bytes the old
// tool sent to a working controller.
type golden struct {
	LED []struct {
		Name       string   `json:"name"`
		Read       string   `json:"read"`
		Encoded    string   `json:"encoded"`
		XInput     []string `json:"xinputParcels"`
		DInput     []string `json:"dinputParcels"`
		Mode       int      `json:"mode"`
		Brightness int      `json:"brightness"`
		Zones      []string `json:"zones"`
	} `json:"led"`
	Commands map[string]string `json:"commands"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	b, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var goldenMutations = map[string]func(*Lighting){
	"roundtrip":      func(*Lighting) {},
	"solid-00e5ff":   func(l *Lighting) { l.SetSolid(RGB{0, 0xe5, 0xff}) },
	"stream-0.5":     func(l *Lighting) { l.SetStream(0.5) },
	"stream-keep-16": func(l *Lighting) { l.SetStream(float64(100-l.LoopTime) / 100) },
	"off":            func(l *Lighting) { l.SetOff() },
	"off-rgbnum0":    func(l *Lighting) { l.SetOff() },
	"brightness-40":  func(l *Lighting) { l.Brightness = 40 },
	"solid-then-off": func(l *Lighting) { l.SetSolid(RGB{255, 80, 0}); l.SetOff() },
}

func TestLightingMatchesOriginal(t *testing.T) {
	g := loadGolden(t)
	if len(g.LED) != len(goldenMutations) {
		t.Fatalf("golden has %d cases, test knows %d", len(g.LED), len(goldenMutations))
	}
	for _, c := range g.LED {
		t.Run(c.Name, func(t *testing.T) {
			mutate, ok := goldenMutations[c.Name]
			if !ok {
				t.Fatalf("no mutation for %s", c.Name)
			}
			l, err := DecodeLighting(unhex(t, c.Read))
			if err != nil {
				t.Fatal(err)
			}
			mutate(l)
			enc := l.Encode()
			if got := hex.EncodeToString(enc); got != c.Encoded {
				t.Fatalf("encoded\n got %s\nwant %s", got, c.Encoded)
			}
			if int(l.Mode) != c.Mode || int(l.Brightness) != c.Brightness {
				t.Fatalf("mode %d brightness %d, want %d %d", l.Mode, l.Brightness, c.Mode, c.Brightness)
			}
			zones := l.ZoneColors()
			if len(zones) != len(c.Zones) {
				t.Fatalf("zones %v, want %v", zones, c.Zones)
			}
			for i, z := range zones {
				if z.Hex() != c.Zones[i] {
					t.Fatalf("zone %d %s, want %s", i, z.Hex(), c.Zones[i])
				}
			}
			for _, d := range []struct {
				dia  *dialect
				want []string
			}{{xinput, c.XInput}, {dinput, c.DInput}} {
				got := d.dia.lightingParcels(enc, 0)
				if len(got) != len(d.want) {
					t.Fatalf("%s: %d parcels, want %d", d.dia.name, len(got), len(d.want))
				}
				for i := range got {
					if hex.EncodeToString(got[i]) != d.want[i] {
						t.Fatalf("%s parcel %d\n got %x\nwant %s", d.dia.name, i, got[i], d.want[i])
					}
				}
			}
		})
	}
}

func TestCommandsMatchOriginal(t *testing.T) {
	g := loadGolden(t)
	for name, got := range map[string][]byte{
		"xinputInfo":    xinput.info,
		"xinputDongle":  xinput.dongle,
		"xinputReadLED": xinput.readLighting(0),
		"dinputInfo":    dinput.info,
		"dinputDongle":  dinput.dongle,
		"dinputReadLED": dinput.readLighting(0),
	} {
		if hex.EncodeToString(got) != g.Commands[name] {
			t.Errorf("%s: got %x, want %s", name, got, g.Commands[name])
		}
	}
}
