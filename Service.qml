import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Commons as Commons
import "Model.js" as Model

// The plugin's one shared instance. Every call to vader4-rgb goes through here,
// so the bar widgets on several monitors never talk to the controller at once.
//
// Nothing here opens the controller on its own except a change you ask for, a
// first read when the panel opens with no lighting on record, and theme
// following if you turn it on. Hotplug comes from `vader4-rgb watch`, which only
// reads sysfs, and the last lighting comes from the state file the helper writes.
Item {
  id: root

  // Injected by the shell.
  property var shell: null
  property var manifest: null
  property string omarchyPath: ""

  readonly property string pluginDir: manifest && manifest.__sourceDir ? String(manifest.__sourceDir) : ""
  readonly property string homeDir: Quickshell.env("HOME") || ""
  readonly property string statePath: (Quickshell.env("XDG_STATE_HOME") || (homeDir + "/.local/state")) + "/vader4-rgb/state.json"

  // ---- The helper binary
  property string helper: ""
  property string helperVersion: ""
  // "" when the helper is fine, "missing", or "outdated" for a vader4-rgb
  // without plugin support.
  property string helperProblem: ""
  property bool helperChecked: false

  // ---- Connection, from `vader4-rgb watch`
  property var status: ({})
  readonly property bool present: !!status.present
  readonly property bool accessible: present && !!status.access

  // ---- Last known controller and lighting. The controller stores its own
  //      lighting, so this stays true while it is unplugged.
  property var controller: null
  property var lighting: null
  property string updatedAt: ""

  // ---- Work in flight
  readonly property bool busy: runner.running
  // "read" or "write", for the panel's status line.
  property string busyKind: ""
  property var pendingChange: null
  property bool refreshQueued: false
  // Shown in place of the stored lighting until the change lands.
  property var optimistic: null
  property string lastError: ""
  property string lastErrorCode: ""
  property string lastHint: ""

  // Pushed in by the bar widget from its settings.
  property bool followTheme: false

  readonly property string accentHex: Model.colorToHex(Commons.Color.accent)
  readonly property var view: Model.mergeLighting(lighting, optimistic)
  readonly property string blocker: Model.blocker(helperChecked ? helperProblem : "", status)

  function describe() {
    return JSON.stringify({
      helper: root.helper, helperVersion: root.helperVersion, helperProblem: root.helperProblem,
      blocker: root.blocker, status: root.status, controller: root.controller, lighting: root.lighting,
      view: root.view, updatedAt: root.updatedAt, busy: root.busy, followTheme: root.followTheme,
      accent: root.accentHex, lastError: root.lastError, lastErrorCode: root.lastErrorCode
    })
  }

  // ------------------------------------------------------------ requests

  // Each returns false when the change cannot be made, such as with no
  // controller connected.
  function setColor(hex) {
    var h = Model.normalizeHex(hex)
    return h !== "" && request({ mode: "solid", color: h })
  }
  function setOff() { return request({ mode: "off" }) }
  function setStream(speed) { return request(typeof speed === "number" ? { mode: "stream", speed: speed } : { mode: "stream" }) }
  function setBrightness(value) { return request({ brightness: Math.round(value) }) }

  // Changes made while one is running are merged and sent as one, so dragging
  // through colors costs one extra write rather than one per click.
  function request(change) {
    if (blocker !== "") return false
    root.optimistic = Model.mergeChange(root.optimistic, change)
    root.pendingChange = Model.mergeChange(root.pendingChange, change)
    if (!runner.running) startNext()
    return true
  }

  // Reads the controller. In XInput mode it disconnects for a moment.
  function refresh() {
    if (blocker !== "") return
    if (runner.running) {
      root.refreshQueued = true
      return
    }
    run([])
  }

  // A first read when the panel opens and nothing is on record yet.
  function ensureLighting() {
    if (!root.lighting && root.accessible && !runner.running) refresh()
  }

  function startNext() {
    if (root.pendingChange) {
      var args = Model.changeArgs(root.pendingChange)
      root.pendingChange = null
      if (args.length > 0) {
        run(args)
        return
      }
    }
    if (root.refreshQueued) {
      root.refreshQueued = false
      run([])
      return
    }
    root.optimistic = null
  }

  property bool _exited: false
  property bool _streamDone: false
  property int _exitCode: 0

  function run(args) {
    if (!root.helper) return
    root._exited = false
    root._streamDone = false
    root.busyKind = args.length > 0 ? "write" : "read"
    runner.command = [root.helper, "--json"].concat(args)
    runner.running = true
  }

  // stdout can finish before or after the process reports its exit.
  function finishRun() {
    if (!root._exited || !root._streamDone) return
    root._exited = false
    var r = Model.parseJson(runnerOut.text)
    if (r && r.ok) {
      root.applyResult(r)
      root.lastError = ""
      root.lastErrorCode = ""
      root.lastHint = ""
    } else {
      root.optimistic = null
      root.pendingChange = null
      if (r && r.error) {
        root.lastError = String(r.error.message || "")
        root.lastErrorCode = String(r.error.code || "failed")
        root.lastHint = String(r.error.hint || "")
      } else {
        root.lastError = String(runnerErr.text || "").trim() || ("vader4-rgb exited with status " + root._exitCode)
        root.lastErrorCode = "failed"
        root.lastHint = ""
      }
    }
    startNext()
  }

  function applyResult(r) {
    if (!r || !r.ok || !r.lighting) return
    root.controller = r.controller || root.controller
    root.lighting = r.lighting
    root.updatedAt = String(r.updatedAt || "")
  }

  Process {
    id: runner
    running: false
    command: []
    stdout: StdioCollector {
      id: runnerOut
      waitForEnd: true
      onStreamFinished: {
        root._streamDone = true
        root.finishRun()
      }
    }
    stderr: StdioCollector { id: runnerErr; waitForEnd: true }
    onExited: function(exitCode) {
      root._exitCode = exitCode
      root._exited = true
      root.finishRun()
    }
  }

  // ------------------------------------------------------------- the helper

  // Picks the first vader4-rgb that speaks the plugin's JSON. An older one from
  // z23/flydigictl has the same name but no --json, status or watch.
  function findHelper() {
    finder.running = false
    finder.command = ["sh", "-c",
      'found=""\n'
      + 'for p in "$(command -v vader4-rgb 2>/dev/null)" "$@"; do\n'
      + '  [ -n "$p" ] && [ -x "$p" ] || continue\n'
      + '  found="$p"\n'
      + '  v=$("$p" --version 2>/dev/null) || continue\n'
      + '  case "$v" in *"omarchy-flydigi, api "*) printf "ok\\n%s\\n%s\\n" "$p" "$v"; exit 0 ;; esac\n'
      + 'done\n'
      + '[ -n "$found" ] && { printf "outdated\\n%s\\n" "$found"; exit 0; }\n'
      + 'printf "missing\\n"\n',
      "sh", root.homeDir + "/.local/bin/vader4-rgb", root.pluginDir + "/bin/vader4-rgb"]
    finder.running = true
  }

  Process {
    id: finder
    running: false
    command: []
    stdout: StdioCollector { id: finderOut; waitForEnd: true }
    onExited: {
      var lines = String(finderOut.text || "").split("\n")
      var kind = lines[0] || "missing"
      var api = /api (\d+)/.exec(lines[2] || "")
      root.helperChecked = true
      if (kind === "ok" && api && Number(api[1]) >= Model.HELPER_API) {
        root.helperProblem = ""
        root.helperVersion = lines[2]
        if (root.helper !== lines[1]) {
          root.helper = lines[1]
          root.restartWatch()
        } else if (!watcher.running) {
          root.restartWatch()
        }
      } else {
        root.helperProblem = kind === "missing" ? "missing" : "outdated"
        root.helper = ""
        watcher.running = false
      }
    }
  }

  // Opens a terminal running install.sh. It builds the helper and asks sudo
  // for the udev rule, so it needs a person in front of it.
  function runSetup() {
    if (!root.pluginDir) return
    Quickshell.execDetached(["omarchy-launch-floating-terminal-with-presentation",
      Util.shellQuote(root.pluginDir + "/install.sh")])
  }

  // ------------------------------------------------------------- hotplug

  property int _watchRestarts: 0

  function restartWatch() {
    watcher.running = false
    if (!root.helper) return
    watcher.command = [root.helper, "watch"]
    watcher.running = true
  }

  Process {
    id: watcher
    running: false
    command: []
    stdout: SplitParser {
      onRead: function(line) {
        var s = Model.parseJson(line)
        if (s && s.api !== undefined) {
          root._watchRestarts = 0
          root.status = s
        }
      }
    }
    onExited: {
      if (!root.helper) return
      // Back off if it keeps dying, rather than spinning.
      watchRestart.interval = Math.min(60000, 1000 * Math.pow(2, root._watchRestarts))
      root._watchRestarts++
      watchRestart.restart()
    }
  }

  Timer {
    id: watchRestart
    repeat: false
    onTriggered: if (root.helper && !watcher.running) root.restartWatch()
  }

  // ---------------------------------------------------- the state file

  // FileView only notices a file appearing if its folder already exists, so
  // make the folder before pointing it there.
  property bool stateDirReady: false

  Process {
    id: makeStateDir
    running: false
    command: ["mkdir", "-p", root.statePath.replace(/\/[^\/]*$/, "")]
    onExited: root.stateDirReady = true
  }

  FileView {
    id: stateFile
    path: root.stateDirReady ? root.statePath : ""
    watchChanges: true
    printErrors: false
    // text() is stale in the change signal itself; reload, then read in onLoaded.
    onFileChanged: reload()
    onLoaded: {
      var r = Model.parseJson(text())
      if (r && r.ok && String(r.updatedAt || "") !== root.updatedAt) root.applyResult(r)
    }
  }

  // ---------------------------------------------------- theme following

  onAccentHexChanged: scheduleThemeSync()
  onFollowThemeChanged: scheduleThemeSync()
  onAccessibleChanged: scheduleThemeSync()

  function scheduleThemeSync() {
    if (root.followTheme && root.accessible) themeSync.restart()
  }

  // A theme switch can repaint the accent more than once; wait for it to settle.
  Timer {
    id: themeSync
    interval: 1500
    repeat: false
    onTriggered: {
      if (!root.followTheme || !root.accessible || !root.accentHex) return
      var v = root.view
      if (v.mode === "solid" && Model.sameColor(v.color, root.accentHex)) return
      root.setColor(root.accentHex)
    }
  }

  Component.onCompleted: {
    makeStateDir.running = true
    findHelper()
  }
}
