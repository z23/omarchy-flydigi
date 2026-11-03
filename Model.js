.pragma library

// Pure helpers for the Flydigi Vader 4 plugin. Nothing here touches Qt, so the
// tests in tests/model.test.mjs run them under node.

var HELPER_API = 1

// The named colors vader4-rgb accepts, in hue order.
var PRESETS = [
  { name: "Red", hex: "#FF0000" },
  { name: "Orange", hex: "#FF5000" },
  { name: "Yellow", hex: "#FFFF00" },
  { name: "Green", hex: "#00FF00" },
  { name: "Cyan", hex: "#00FFFF" },
  { name: "Blue", hex: "#0000FF" },
  { name: "Purple", hex: "#A000FF" },
  { name: "Pink", hex: "#FF2878" },
  { name: "White", hex: "#FFFFFF" }
]

var MODES = [
  { value: "solid", label: "Solid" },
  { value: "stream", label: "Stream" },
  { value: "off", label: "Off" }
]

function parseJson(text) {
  try {
    var v = JSON.parse(String(text || "").trim())
    return v && typeof v === "object" ? v : null
  } catch (e) {
    return null
  }
}

function hex2(n) {
  var v = Math.max(0, Math.min(255, Math.round(n)))
  return (v < 16 ? "0" : "") + v.toString(16).toUpperCase()
}

// A Qt color (r, g, b from 0 to 1) to #RRGGBB.
function colorToHex(c) {
  if (!c || c.r === undefined) return ""
  return "#" + hex2(c.r * 255) + hex2(c.g * 255) + hex2(c.b * 255)
}

// Every color name vader4-rgb accepts.
var NAMED = {
  red: "#FF0000", green: "#00FF00", blue: "#0000FF", white: "#FFFFFF", cyan: "#00FFFF", magenta: "#FF00FF",
  yellow: "#FFFF00", orange: "#FF5000", purple: "#A000FF", pink: "#FF2878", black: "#000000"
}

// A color name, #RGB, RGB, RRGGBB or #RRGGBB to #RRGGBB, or "" when it is none of those.
function normalizeHex(s) {
  var name = String(s || "").trim().toLowerCase()
  if (NAMED.hasOwnProperty(name)) return NAMED[name]
  var h = String(s || "").trim().replace(/^#/, "")
  if (/^[0-9a-fA-F]{3}$/.test(h)) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2]
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return ""
  return "#" + h.toUpperCase()
}

// Black or white, whichever reads better on the color.
function readableOn(hex) {
  var h = normalizeHex(hex)
  if (!h) return "#FFFFFF"
  var r = parseInt(h.slice(1, 3), 16), g = parseInt(h.slice(3, 5), 16), b = parseInt(h.slice(5, 7), 16)
  return (0.299 * r + 0.587 * g + 0.114 * b) > 150 ? "#000000" : "#FFFFFF"
}

function sameColor(a, b) {
  var x = normalizeHex(a)
  return x !== "" && x === normalizeHex(b)
}

// Combines two requested changes. A new effect replaces the old effect and its
// color and speed; brightness travels separately.
function mergeChange(a, b) {
  var out = {}
  var k
  if (a) for (k in a) out[k] = a[k]
  if (!b) return out
  if (b.mode !== undefined) {
    out.mode = b.mode
    delete out.color
    delete out.speed
    if (b.color !== undefined) out.color = b.color
    if (b.speed !== undefined) out.speed = b.speed
  }
  if (b.brightness !== undefined) out.brightness = b.brightness
  return out
}

// The vader4-rgb arguments for a change.
function changeArgs(c) {
  var args = []
  if (!c) return args
  if (c.mode === "solid" && normalizeHex(c.color)) args.push(normalizeHex(c.color))
  else if (c.mode === "off") args.push("off")
  else if (c.mode === "stream") {
    args.push("stream")
    if (typeof c.speed === "number") args.push("-s", Math.max(0, Math.min(1, c.speed)).toFixed(2))
  }
  if (typeof c.brightness === "number") args.push("-b", String(Math.max(0, Math.min(100, Math.round(c.brightness)))))
  return args
}

// What the panel shows: the stored lighting with any change on its way laid over it.
function mergeLighting(lighting, pending) {
  var out = { mode: "", color: "", brightness: -1, speed: -1, zones: [] }
  if (lighting) {
    out.mode = lighting.mode || ""
    out.color = lighting.color || ""
    out.brightness = typeof lighting.brightness === "number" ? lighting.brightness : -1
    out.speed = typeof lighting.speed === "number" ? lighting.speed : -1
    out.zones = lighting.zones || []
  }
  if (pending) {
    if (pending.mode !== undefined) {
      out.mode = pending.mode
      out.color = pending.mode === "solid" ? normalizeHex(pending.color) : ""
      if (pending.mode === "stream" && typeof pending.speed === "number") out.speed = pending.speed
    }
    if (typeof pending.brightness === "number") out.brightness = pending.brightness
  }
  return out
}

var MODE_NAMES = { solid: "Solid", stream: "Stream", off: "Off", breathing: "Breathing", gradient: "Gradient", feedback: "Feedback" }

function modeName(mode) {
  return MODE_NAMES[mode] || (mode ? String(mode) : "Unknown")
}

// One line for the bar tooltip and the panel heading.
function summary(view) {
  if (!view || !view.mode) return "Lighting not read yet"
  var parts = [modeName(view.mode)]
  if (view.mode === "solid" && view.color) parts.push(view.color)
  if (view.mode !== "off" && view.brightness >= 0) parts.push(view.brightness + "%")
  return parts.join(" · ")
}

function connectionLine(status, controller) {
  if (!status || !status.present) return "Not connected"
  var parts = []
  if (controller && controller.link) parts.push(controller.link === "wired" ? "Wired" : "Wireless")
  parts.push(status.mode === "dinput" ? "DInput" : "XInput")
  return parts.join(" · ")
}

// What stands between the user and changing the light bar, or "" when nothing does.
function blocker(helperProblem, status) {
  if (helperProblem === "missing") return "setup"
  if (helperProblem === "outdated") return "outdated"
  if (!status || status.api === undefined) return "starting"
  if (!status.present) return "disconnected"
  if (!status.access) return "permission"
  return ""
}

var BLOCKERS = {
  setup: {
    title: "Finish setup",
    body: "The plugin needs its vader4-rgb helper and a udev rule. Run install.sh from the plugin folder, or press Set up."
  },
  outdated: {
    title: "Update the helper",
    body: "The vader4-rgb on this system predates the plugin. Run install.sh, or press Set up, to replace it."
  },
  starting: { title: "Looking for the controller", body: "" },
  disconnected: {
    title: "Controller not connected",
    body: "Connect it by USB or the 2.4 GHz dongle. Hold the circle button and X for XInput mode (white indicator), or the circle button and A for DInput mode (blue indicator)."
  },
  permission: {
    title: "Permission needed",
    body: "The controller is connected, but this user cannot open it. Run install.sh, or press Set up, to install the udev rule, then replug the controller."
  }
}

function blockerText(kind) {
  return BLOCKERS[kind] || { title: "", body: "" }
}
