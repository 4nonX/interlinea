import QtQuick
import QtQuick.Templates as T
import Interlinea

T.MenuItem {
    id: control
    implicitWidth: row.implicitWidth + leftPadding + rightPadding + 24
    implicitHeight: 30
    leftPadding: Theme.s2 + 2
    rightPadding: Theme.s2 + 2
    hoverEnabled: true
    font.family: Theme.font
    font.pixelSize: Theme.body
    opacity: enabled ? 1 : 0.5
    readonly property string keys: action && action.shortcut ? String(action.shortcut) : ""
    contentItem: Item {
        implicitWidth: row.implicitWidth
        Row {
            id: row
            anchors.verticalCenter: parent.verticalCenter
            spacing: Theme.s2
            Item {
                width: 16; height: 16
                anchors.verticalCenter: parent.verticalCenter
                Icon { anchors.centerIn: parent; name: control.checkable && control.checked ? "check" : (control.icon.name || ""); color: Theme.text2; size: 14 }
            }
            Text {
                text: control.text.replace(/&(.)/, "$1")
                color: Theme.text
                font: control.font
                anchors.verticalCenter: parent.verticalCenter
            }
            Item { width: 24; height: 1 }
            Text {
                text: control.keys
                color: Theme.text3
                font.family: Theme.font
                font.pixelSize: Theme.small
                anchors.verticalCenter: parent.verticalCenter
            }
        }
        Icon {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            name: control.subMenu ? "chevron-right" : ""
            color: Theme.text2
            size: 14
        }
    }
    background: Rectangle {
        radius: 4
        color: control.highlighted ? Theme.accentWeak : "transparent"
    }
}
