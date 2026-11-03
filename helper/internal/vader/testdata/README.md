# golden.json

This file pins the rewrite to the bytes the original tool sent to a working Vader 4 Pro. It was generated with the original implementation: z23/flydigictl at commit `daadf96`, a modified copy of pipe01/flydigictl.

A small program built against that code did the following:

1. Decoded a synthetic 490-byte lighting block.
2. Applied each change named in `led[].name`, using the original `SetSteady`, `SetStreamlined`, the original off logic, or a brightness change.
3. Printed the encoded block, plus the XInput and DInput write packets that the original `getLEDConfigDataParcels` produced from it.

It also printed the original info, lighting-read, and dongle command packets for both modes.

`golden_test.go` runs the same changes through this package and compares every byte.
