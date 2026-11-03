// Run with: node --test tests/
import { test } from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import vm from "node:vm"

// Model.js is a QML JavaScript library; drop the pragma and run it in a sandbox.
const source = readFileSync(new URL("../Model.js", import.meta.url), "utf8").replace(/^\.pragma library\s*/, "")
const Model = {}
// Values made inside the sandbox have the sandbox's prototypes; compare plain copies.
const plain = (v) => JSON.parse(JSON.stringify(v))
vm.runInNewContext(source + "\n;Object.assign(exports, { HELPER_API, PRESETS, parseJson, colorToHex, normalizeHex, sameColor, mergeChange, changeArgs, mergeLighting, summary, connectionLine, blocker, blockerText })", { exports: Model })

test("normalizeHex", () => {
  assert.equal(Model.normalizeHex("#00e5ff"), "#00E5FF")
  assert.equal(Model.normalizeHex("0ef"), "#00EEFF")
  assert.equal(Model.normalizeHex(" 00E5FF "), "#00E5FF")
  assert.equal(Model.normalizeHex("#12345"), "")
  assert.equal(Model.normalizeHex("teal"), "")
  assert.equal(Model.normalizeHex("Orange"), "#FF5000")
  assert.equal(Model.normalizeHex(undefined), "")
})

test("colorToHex", () => {
  assert.equal(Model.colorToHex({ r: 0, g: 229 / 255, b: 1, a: 1 }), "#00E5FF")
  assert.equal(Model.colorToHex(null), "")
})

test("presets are the helper's names", () => {
  for (const p of Model.PRESETS) assert.equal(Model.normalizeHex(p.name), p.hex)
})

test("a new effect replaces the old one but keeps brightness", () => {
  let c = Model.mergeChange(null, { mode: "stream", speed: 0.3 })
  c = Model.mergeChange(c, { brightness: 40 })
  c = Model.mergeChange(c, { mode: "solid", color: "#ff0000" })
  assert.deepEqual(plain(c), { mode: "solid", color: "#ff0000", brightness: 40 })
  assert.deepEqual(plain(Model.changeArgs(c)), ["#FF0000", "-b", "40"])
})

test("changeArgs", () => {
  assert.deepEqual(plain(Model.changeArgs({ mode: "stream" })), ["stream"])
  assert.deepEqual(plain(Model.changeArgs({ mode: "stream", speed: 0.333 })), ["stream", "-s", "0.33"])
  assert.deepEqual(plain(Model.changeArgs({ mode: "off" })), ["off"])
  assert.deepEqual(plain(Model.changeArgs({ brightness: 101.4 })), ["-b", "100"])
  assert.deepEqual(plain(Model.changeArgs({ mode: "solid", color: "nope" })), [])
  assert.deepEqual(plain(Model.changeArgs(null)), [])
})

test("mergeLighting lays a pending change over the stored lighting", () => {
  const stored = { mode: "stream", color: null, brightness: 70, speed: 0.5, zones: ["#323200"] }
  assert.deepEqual(plain(Model.mergeLighting(stored, null)), { mode: "stream", color: "", brightness: 70, speed: 0.5, zones: ["#323200"] })
  const v = Model.mergeLighting(stored, { mode: "solid", color: "#00e5ff" })
  assert.equal(v.mode, "solid")
  assert.equal(v.color, "#00E5FF")
  assert.equal(v.brightness, 70)
  assert.equal(Model.mergeLighting(null, { brightness: 20 }).brightness, 20)
})

test("summary and connection lines", () => {
  assert.equal(Model.summary(Model.mergeLighting({ mode: "solid", color: "#00E5FF", brightness: 70 })), "Solid · #00E5FF · 70%")
  assert.equal(Model.summary(Model.mergeLighting({ mode: "off", brightness: 70 })), "Off")
  assert.equal(Model.summary(null), "Lighting not read yet")
  assert.equal(Model.connectionLine({ present: false }), "Not connected")
  assert.equal(Model.connectionLine({ present: true, mode: "xinput" }, { link: "wired" }), "Wired · XInput")
  assert.equal(Model.connectionLine({ present: true, mode: "dinput" }, { link: "wireless" }), "Wireless · DInput")
})

test("blocker order", () => {
  assert.equal(Model.blocker("missing", { api: 1, present: true, access: true }), "setup")
  assert.equal(Model.blocker("", {}), "starting")
  assert.equal(Model.blocker("", { api: 1, present: false }), "disconnected")
  assert.equal(Model.blocker("", { api: 1, present: true, access: false }), "permission")
  assert.equal(Model.blocker("", { api: 1, present: true, access: true }), "")
  assert.ok(Model.blockerText("permission").body.length > 0)
})

test("parseJson", () => {
  assert.deepEqual(plain(Model.parseJson('{"api":1}\n')), { api: 1 })
  assert.equal(Model.parseJson("not json"), null)
  assert.equal(Model.parseJson("3"), null)
})
