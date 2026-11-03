import QtQuick
import Quickshell
import qs.Commons
import qs.Commons as Commons
import qs.Ui
import "Model.js" as Model

// The light bar panel: effect, color, brightness, stream speed, and theme
// following. BarWidget.qml owns the bar icon and hands this panel the button
// to anchor against and the shared service.
Panel {
  id: root
  moduleName: "io.github.z23.flydigi"
  ipcTarget: "io.github.z23.flydigi"
  manageIpc: false

  property var anchorItem: null
  property var service: null
  // The bar tracks the widget mounted in its slot, not this nested panel, so
  // that widget is the panel's identity for the popout coordinator.
  property var hostWidget: null
  readonly property var barIdentity: hostWidget || root

  readonly property color foreground: bar ? bar.foreground : Commons.Color.foreground
  readonly property color dim: Qt.darker(foreground, 1.5)
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family

  readonly property var view: service ? service.view : Model.mergeLighting(null, null)
  readonly property string blocker: service ? service.blocker : "starting"
  readonly property bool ready: blocker === ""
  readonly property bool busy: !!service && service.busy
  readonly property bool followTheme: String(setting("followTheme", "Off")) === "On"
  readonly property string accentHex: service ? service.accentHex : Model.colorToHex(Commons.Color.accent)
  // The last solid color, so picking Solid after Stream or Off brings it back.
  property string lastSolid: ""

  readonly property var swatches: [{ name: "Theme accent", hex: root.accentHex, theme: true }].concat(Model.PRESETS)

  onViewChanged: if (view.mode === "solid" && view.color) root.lastSolid = view.color

  function open() {
    if (root.service) {
      if (root.service.helperProblem !== "") root.service.findHelper()
      root.service.ensureLighting()
    }
    root.controller.show()
  }

  function close() {
    root.controller.hide()
  }

  function toggle() {
    if (root.opened) root.close()
    else root.open()
  }

  function switchPanel(direction) {
    if (root.bar && typeof root.bar.switchPanelFrom === "function")
      return root.bar.switchPanelFrom(root.barIdentity, direction)
    return false
  }

  // Applied locally first so the switch moves on the click itself; the
  // shell.json write comes back through the bar as the same value.
  function persistSettings(values) {
    var entry = { id: root.moduleName }
    for (var existing in root.settings) if (existing !== "id") entry[existing] = root.settings[existing]
    for (var key in values) entry[key] = values[key]
    root.settings = entry
    if (root.hostWidget && "settings" in root.hostWidget) root.hostWidget.settings = entry
    if (root.bar && root.bar.shell && typeof root.bar.shell.updateEntryInline === "function")
      root.bar.shell.updateEntryInline(root.moduleName, entry)
  }

  function chooseMode(mode) {
    if (!root.service) return
    if (mode === "solid") root.service.setColor(root.lastSolid || root.accentHex)
    else if (mode === "stream") root.service.setStream()
    else if (mode === "off") root.service.setOff()
  }

  function statusText() {
    if (!root.service) return ""
    if (root.busy) return root.service.busyKind === "read" ? "Reading the controller…" : "Saving to the controller…"
    if (root.service.lastError !== "")
      return root.service.lastError + (root.service.lastHint ? ". " + root.service.lastHint : "")
    if (!root.ready) return ""
    if (root.service.status.mode === "xinput")
      return "Saved on the controller. In XInput mode each change disconnects it for a moment."
    return "Saved on the controller."
  }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.barIdentity
    bar: root.bar
    open: root.opened
    centerOnBar: false
    focusTarget: panel.loadedContent
    contentWidth: panel.fittedContentWidth(Style.space(368))
    contentHeight: panel.fittedContentHeight(panel.loadedContentHeight)

    content: PanelKeyCatcher {
      id: keyCatcher
      implicitHeight: column.implicitHeight
      anchors.fill: parent
      blocked: hexField.activeFocus
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }
      onTextKey: function(t) {
        if ((t === "r" || t === "R") && root.service) root.service.refresh()
      }

      Column {
        id: column
        width: parent.width
        spacing: Style.space(12)

        PanelHero {
          id: hero
          width: parent.width
          title: root.service && root.service.controller ? root.service.controller.model : "Flydigi Vader 4"
          meta: root.service ? Model.connectionLine(root.service.status, root.service.controller) : ""
          detail: root.service && root.service.controller && root.service.controller.firmware ? "fw " + root.service.controller.firmware : ""
          foreground: root.foreground
          fontFamily: root.fontFamily
          iconOpacity: root.service && root.service.present ? 1.0 : 0.5

          iconComponent: Component {
            Text {
              textFormat: Text.PlainText
              text: "\uf11b"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.display
            }
          }

          trailingControl: Component {
            PanelActionButton {
              visible: root.ready
              enabled: !root.busy
              iconText: "\uf021"
              tooltipText: "Read the lighting from the controller (R)"
              foreground: root.foreground
              fontFamily: root.fontFamily
              onClicked: if (root.service) root.service.refresh()
            }
          }
        }

        // ---- What stands in the way, when something does.
        Column {
          width: parent.width
          spacing: Style.space(8)
          visible: root.blocker !== "" && root.blocker !== "starting"

          PanelSeparator { width: parent.width; foreground: root.foreground }

          Text {
            width: parent.width
            textFormat: Text.PlainText
            text: Model.blockerText(root.blocker).title
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.subtitle
            font.bold: true
          }

          Text {
            width: parent.width
            textFormat: Text.PlainText
            wrapMode: Text.WordWrap
            text: Model.blockerText(root.blocker).body
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
          }

          Button {
            visible: root.blocker === "setup" || root.blocker === "outdated" || root.blocker === "permission"
            text: "Set up"
            iconText: "\uf120"
            bordered: true
            foreground: root.foreground
            fontFamily: root.fontFamily
            onClicked: {
              if (root.service) root.service.runSetup()
              root.close()
            }
          }
        }

        // ---- The light bar.
        Column {
          width: parent.width
          spacing: Style.space(10)
          visible: root.ready

          PanelSeparator { width: parent.width; foreground: root.foreground }

          PanelSectionHeader {
            text: "Light bar"
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          ButtonGroup {
            options: Model.MODES
            value: root.view.mode
            foreground: root.foreground
            fontFamily: root.fontFamily
            onChanged: function(v) { root.chooseMode(v) }
          }

          Text {
            visible: root.view.mode !== "" && root.view.mode !== "solid" && root.view.mode !== "stream" && root.view.mode !== "off"
            width: parent.width
            textFormat: Text.PlainText
            wrapMode: Text.WordWrap
            text: Model.modeName(root.view.mode) + " was set elsewhere, such as Flydigi Space. Pick an effect to replace it."
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }

          Flow {
            width: parent.width
            spacing: Style.space(4)

            Repeater {
              model: root.swatches
              delegate: Item {
                id: swatch
                required property var modelData
                readonly property bool selected: root.view.mode === "solid" && Model.sameColor(root.view.color, modelData.hex)
                width: Style.space(28)
                height: width

                Rectangle {
                  anchors.fill: parent
                  radius: Style.cornerRadius > 0 ? width / 2 : 0
                  color: "transparent"
                  border.width: swatch.selected ? Math.max(2, Style.space(2)) : (swatchMouse.containsMouse ? 1 : 0)
                  border.color: root.foreground
                }

                Rectangle {
                  anchors.fill: parent
                  anchors.margins: Style.space(4)
                  radius: Style.cornerRadius > 0 ? width / 2 : 0
                  color: swatch.modelData.hex || "transparent"
                  border.width: 1
                  border.color: Qt.rgba(root.foreground.r, root.foreground.g, root.foreground.b, 0.25)

                  Text {
                    anchors.centerIn: parent
                    visible: !!swatch.modelData.theme
                    textFormat: Text.PlainText
                    text: "\uf1fc"
                    color: Model.readableOn(swatch.modelData.hex)
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.caption
                  }
                }

                MouseArea {
                  id: swatchMouse
                  anchors.fill: parent
                  hoverEnabled: true
                  enabled: root.ready
                  cursorShape: Qt.PointingHandCursor
                  onClicked: if (root.service) root.service.setColor(swatch.modelData.hex)

                  PanelToolTip {
                    visible: swatchMouse.containsMouse
                    text: swatch.modelData.name + " " + swatch.modelData.hex
                    fontFamily: root.fontFamily
                  }
                }
              }
            }
          }

          Row {
            spacing: Style.space(8)

            TextField {
              id: hexField
              width: Style.space(110)
              placeholderText: "#RRGGBB"
              foreground: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              enabled: root.ready
              readonly property bool valid: text === "" || Model.normalizeHex(text) !== ""
              color: valid ? root.foreground : Commons.Color.urgent
              onAccepted: {
                if (root.service && root.service.setColor(text)) text = ""
              }
              Keys.onEscapePressed: function(event) {
                text = ""
                keyCatcher.forceActiveFocus()
                event.accepted = true
              }
            }

            Text {
              anchors.verticalCenter: parent.verticalCenter
              textFormat: Text.PlainText
              text: "Enter applies"
              visible: hexField.activeFocus
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }

          Item {
            width: parent.width
            height: Style.space(28)
            visible: root.view.mode !== "off"

            Text {
              id: brightnessLabel
              anchors.left: parent.left
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(80)
              textFormat: Text.PlainText
              text: "Brightness"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }

            PanelSlider {
              id: brightnessSlider
              bar: root.bar
              anchors.left: brightnessLabel.right
              anchors.right: brightnessValue.left
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              minimum: 0
              maximum: 100
              step: 1
              integer: true
              enabled: root.ready
              value: Math.max(0, root.view.brightness)
              onReleased: function(v) { if (root.service) root.service.setBrightness(v) }
            }

            Text {
              id: brightnessValue
              anchors.right: parent.right
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(36)
              horizontalAlignment: Text.AlignRight
              textFormat: Text.PlainText
              text: root.view.brightness < 0 && !brightnessSlider.dragging ? "–" : Math.round(brightnessSlider.dragging ? brightnessSlider.liveValue : root.view.brightness) + "%"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
          }

          Item {
            width: parent.width
            height: Style.space(28)
            visible: root.view.mode === "stream"

            Text {
              id: speedLabel
              anchors.left: parent.left
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(80)
              textFormat: Text.PlainText
              text: "Speed"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }

            PanelSlider {
              id: speedSlider
              bar: root.bar
              anchors.left: speedLabel.right
              anchors.right: speedValue.left
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              minimum: 0
              maximum: 1
              step: 0.05
              enabled: root.ready
              value: root.view.speed >= 0 ? root.view.speed : 0.5
              onReleased: function(v) { if (root.service) root.service.setStream(v) }
            }

            Text {
              id: speedValue
              anchors.right: parent.right
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(36)
              horizontalAlignment: Text.AlignRight
              textFormat: Text.PlainText
              text: Math.round((speedSlider.dragging ? speedSlider.liveValue : speedSlider.value) * 100) + "%"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
          }

          PanelSeparator { width: parent.width; foreground: root.foreground }

          Toggle {
            width: parent.width
            label: "Match the theme accent"
            description: "Sets the light bar to the accent when the theme changes or the controller connects."
            checked: root.followTheme
            foreground: root.foreground
            fontFamily: root.fontFamily
            onClicked: root.persistSettings({ followTheme: root.followTheme ? "Off" : "On" })
          }
        }

        Text {
          width: parent.width
          visible: text !== ""
          textFormat: Text.PlainText
          wrapMode: Text.WordWrap
          text: root.statusText()
          color: root.service && root.service.lastError !== "" && !root.busy ? Commons.Color.urgent : root.dim
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
        }
      }
    }
  }
}
