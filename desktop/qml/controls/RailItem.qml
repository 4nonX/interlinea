import QtQuick
import QtQuick.Templates as T
import Interlinea

// An entry of the navigation: its icon and name; the page shown is white
// with its icon in the accent.
T.AbstractButton {
    id: control
    property string iconName
    property bool current: false
    property string keys: ""
    implicitHeight: 34
    implicitWidth: 180
    leftPadding: Theme.s2
    rightPadding: Theme.s2
    hoverEnabled: true
    focusPolicy: Qt.StrongFocus
    font.family: Theme.font
    font.pixelSize: Theme.body
    font.weight: 500
    Accessible.role: Accessible.PageTab
    Accessible.name: text
    Accessible.selected: current
    contentItem: Row {
        spacing: Theme.s2
        Icon {
            name: control.iconName
            color: control.current ? Theme.accent : Theme.text2
            size: 16
            anchors.verticalCenter: parent.verticalCenter
        }
        Text {
            text: control.text
            color: control.current || control.hovered ? Theme.text : Theme.text2
            font: control.font
            anchors.verticalCenter: parent.verticalCenter
        }
    }
    background: Item {
        Rectangle {
            anchors.fill: parent
            anchors.topMargin: 1
            radius: Theme.rControl
            color: Theme.shadow
            opacity: control.current ? 0.35 : 0
        }
        Rectangle {
            anchors.fill: parent
            radius: Theme.rControl
            color: control.current ? Theme.surface : control.hovered ? Theme.hover : "transparent"
        }
        FocusRing { target: control; visible: control.visualFocus }
    }
    ToolTip {
        visible: control.hovered && control.keys !== ""
        text: control.text + " (" + control.keys + ")"
    }
}
