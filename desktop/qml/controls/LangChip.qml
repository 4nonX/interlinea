import QtQuick
import Interlinea

// A language code in a small bordered chip, as the web's .lang-chip; its
// name for screen readers and as the tip.
Rectangle {
    id: chip
    property string code
    implicitWidth: label.implicitWidth + 12
    implicitHeight: 18
    radius: 4
    color: Theme.surface2
    border.color: Theme.border
    Accessible.role: Accessible.StaticText
    Accessible.name: Backend.languageName(code)
    Text {
        id: label
        anchors.centerIn: parent
        text: chip.code
        color: Theme.text2
        font.family: "monospace"
        font.pixelSize: 11
        font.weight: 600
    }
    HoverHandler { id: hover }
    ToolTip { visible: hover.hovered; text: Backend.languageName(chip.code) }
}
