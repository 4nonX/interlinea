import QtQuick
import Interlinea

// The web's .badge, .tag and .status: 20 px high, round ends, 12 px text.
// kind: "" (grey), "accent", "ok", "warn", "danger", "server".
Rectangle {
    id: badge
    property string text
    property string kind: ""
    property string iconName: ""
    property bool mono: false
    implicitWidth: row.implicitWidth + 14
    implicitHeight: 20
    radius: 10
    color: kind === "accent" ? Theme.accentWeak : kind === "ok" ? Theme.okBg : kind === "warn" ? Theme.warnBg
         : kind === "danger" ? Theme.dangerBg : kind === "server" ? Theme.serverBg : Theme.badge
    readonly property color fg: kind === "accent" ? Theme.accentText : kind === "ok" ? Theme.ok : kind === "warn" ? Theme.warn
                               : kind === "danger" ? Theme.danger : kind === "server" ? Theme.server : Theme.text2
    Accessible.role: Accessible.StaticText
    Accessible.name: text
    Row {
        id: row
        anchors.centerIn: parent
        spacing: 4
        Icon { name: badge.iconName; color: badge.fg; size: 13; anchors.verticalCenter: parent.verticalCenter }
        Text {
            text: badge.text
            color: badge.fg
            font.family: badge.mono ? "monospace" : Theme.font
            font.pixelSize: Theme.tiny
            font.weight: 550
            anchors.verticalCenter: parent.verticalCenter
        }
    }
}
