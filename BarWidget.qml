import QtQuick
import Quickshell
import qs.Commons
import qs.Commons as Commons
import qs.Ui
import "Model.js" as Model

// Controller icon for the bar, and the host for the light bar panel. Left click
// opens the panel. The icon hides while no controller is connected, unless the
// plugin still needs setting up or "Show when disconnected" is on.
BarWidget {
  id: root
  moduleName: "io.github.z23.flydigi"

  readonly property var service: bar && bar.shell ? bar.shell.serviceFor("io.github.z23.flydigi") : null
  readonly property bool followTheme: String(setting("followTheme", "Off")) === "On"
  readonly property bool showWhenDisconnected: String(setting("showWhenDisconnected", "Off")) === "On"
  readonly property string blocker: service ? service.blocker : "starting"
  readonly property bool connected: !!service && service.present
  readonly property bool needsSetup: blocker === "setup" || blocker === "outdated" || blocker === "permission"
  readonly property bool shown: connected || needsSetup || showWhenDisconnected || opened

  readonly property string tooltip: {
    var name = service && service.controller ? service.controller.model : "Flydigi Vader 4"
    if (needsSetup) return name + " · " + Model.blockerText(blocker).title
    if (!connected) return name + " · Not connected"
    if (service.busy) return name + " · Saving…"
    return name + " · " + Model.summary(service.view)
  }

  // Theme following lives in the service, which has one copy for all monitors.
  Binding {
    target: root.service
    property: "followTheme"
    value: root.followTheme
    when: !!root.service
  }

  // ---- Panel. Bar.findPanelWidget routes summon/hide/toggle to the widget
  //      root, so it forwards the panel's lifecycle.
  readonly property bool opened: panelLoader.item ? panelLoader.item.opened === true : false
  readonly property bool popoutSwitchClosing: panelLoader.item ? panelLoader.item.popoutSwitchClosing === true : false

  function open() { if (panelLoader.item) panelLoader.item.open() }
  function close() { if (panelLoader.item) panelLoader.item.close() }
  function togglePanel() { if (panelLoader.item) panelLoader.item.toggle() }
  function closeForPopoutSwitch() { if (panelLoader.item) panelLoader.item.closeForPopoutSwitch() }

  function injectPanel() {
    var target = panelLoader.item
    if (!target) return
    if ("bar" in target) target.bar = root.bar
    if ("settings" in target) target.settings = root.settings
    if ("anchorItem" in target) target.anchorItem = button
    if ("hostWidget" in target) target.hostWidget = root
    if ("service" in target) target.service = root.service
  }

  readonly property real openPanelIndicatorWidth: button.labelWidth
  readonly property real openPanelIndicatorHeight: Math.max(Style.space(10), Math.round(Style.bar.iconSlot * 0.55))

  visible: shown
  implicitWidth: shown ? button.implicitWidth : 0
  implicitHeight: button.implicitHeight

  onBarChanged: injectPanel()
  onSettingsChanged: injectPanel()
  onServiceChanged: injectPanel()

  Loader {
    id: panelLoader
    active: true
    source: Qt.resolvedUrl("Panel.qml")
    visible: false
    onLoaded: {
      root.injectPanel()
      Qt.callLater(root.injectPanel)
    }
  }

  // omarchy-shell io.github.z23.flydigi toggle | color '#00e5ff' | off | stream | refresh | recheck
  ShellIpc {
    target: "io.github.z23.flydigi"

    function open(): void { root.open() }
    function close(): void { root.close() }
    function show(): void { root.open() }
    function hide(): void { root.close() }
    function toggle(): void { root.togglePanel() }
    function color(hex: string): string { return root.service && root.service.setColor(hex) ? "ok" : "error" }
    function off(): string { return root.service && root.service.setOff() ? "ok" : "error" }
    function stream(): string { return root.service && root.service.setStream() ? "ok" : "error" }
    function refresh(): string { if (!root.service) return "error"; root.service.refresh(); return "ok" }
    // install.sh calls this when it finishes, so the plugin picks up the new helper.
    function recheck(): string { if (!root.service) return "error"; root.service.findHelper(); return "ok" }
    // What the plugin knows, as JSON. Never opens the controller.
    function state(): string { return root.service ? root.service.describe() : "{}" }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: "\uf11b"
    dimmed: !root.connected || root.blocker !== ""
    tooltipText: root.tooltip
    onPressed: function(b) {
      if (b === Qt.LeftButton) root.togglePanel()
    }
  }
}
