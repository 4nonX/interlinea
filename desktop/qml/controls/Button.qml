import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's .btn: normal, primary (the page's main step), quiet (a step
// on a row or a field), danger (takes something away); small for rows.
T.Button {
    id: control
    property string kind: "normal"
    property bool small: false
    property string iconName: ""
    // A key that does the same, shown in a chip after the text (the web's kbd).
    property string keys: ""

    readonly property bool primary: kind === "primary"
    readonly property bool quiet: kind === "quiet"
    readonly property bool danger: kind === "danger"

    implicitHeight: small ? Theme.smallHeight : Theme.controlHeight
    implicitWidth: Math.max(implicitHeight, row.implicitWidth + leftPadding + rightPadding)
    leftPadding: small ? 10 : Theme.s3
    rightPadding: small ? 10 : Theme.s3
    focusPolicy: Qt.StrongFocus
    hoverEnabled: true
    font.family: Theme.font
    font.pixelSize: small ? Theme.small : Theme.body
    font.weight: 550
    opacity: enabled ? 1 : 0.55
    Accessible.name: text.replace("&", "")

    readonly property color fg: primary ? "#ffffff"
                              : danger ? Theme.danger
                              : quiet || checked ? Theme.accentText : Theme.text

    contentItem: Item {
        implicitWidth: row.implicitWidth
        implicitHeight: row.implicitHeight
        Row {
            id: row
            anchors.centerIn: parent
            spacing: 6
            Icon {
                name: control.iconName
                color: control.fg
                size: control.small ? 14 : 16
                anchors.verticalCenter: parent.verticalCenter
            }
            Text {
                text: control.text.replace(/&(.)/, "$1")
                visible: text !== ""
                color: control.fg
                font: control.font
                anchors.verticalCenter: parent.verticalCenter
                elide: Text.ElideRight
            }
            Rectangle {
                visible: control.keys !== ""
                anchors.verticalCenter: parent.verticalCenter
                implicitWidth: kbd.implicitWidth + 10
                implicitHeight: kbd.implicitHeight + 2
                radius: 4
                color: control.primary ? Qt.rgba(1, 1, 1, 0.12) : Theme.surface2
                border.color: control.primary ? Qt.rgba(1, 1, 1, 0.45) : Theme.border
                Text {
                    id: kbd
                    anchors.centerIn: parent
                    text: control.keys
                    color: control.primary ? "#ffffff" : Theme.text2
                    font.family: Theme.font
                    font.pixelSize: Theme.tiny
                    font.weight: 550
                }
            }
        }
    }

    background: Rectangle {
        radius: Theme.rControl
        color: control.primary ? (control.down ? Theme.accentPressed : control.hovered ? Theme.accentHover : Theme.accent)
             : control.quiet ? (control.hovered || control.down ? Theme.accentWeak : "transparent")
             : control.danger ? (control.down ? Theme.dangerBg : control.hovered ? Theme.dangerBg : Theme.surface)
             : control.checked ? Theme.accentWeak
             : (control.down ? (Theme.dark ? "#2a313a" : "#eef1f5") : control.hovered ? Theme.surface2 : Theme.surface)
        border.width: control.quiet ? 0 : 1
        border.color: control.primary ? (control.hovered ? Theme.accentHover : Theme.accent)
                    : control.danger ? Theme.danger
                    : control.checked ? Theme.accent
                    : control.hovered ? Theme.borderHover : Theme.borderStrong
        FocusRing { target: control }
    }
}
