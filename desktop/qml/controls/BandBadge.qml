import QtQuick
import Interlinea

// A match band's badge: its short name on its colour.
Rectangle {
    id: badge
    property string band: "new"
    property string text: Bands.shortName(band)
    readonly property var c: Bands.colors(band)
    implicitWidth: Math.max(36, label.implicitWidth + 10)
    implicitHeight: 20
    radius: 4
    color: c.background
    border.color: c.border
    border.width: 1
    Accessible.role: Accessible.StaticText
    Accessible.name: text
    Text {
        id: label
        anchors.centerIn: parent
        text: badge.text
        color: badge.c.text
        font.family: Theme.font
        font.pixelSize: Theme.tiny
        font.weight: 600
    }
}
