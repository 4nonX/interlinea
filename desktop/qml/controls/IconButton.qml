import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's .icon-button: a square icon with its tip and name.
T.Button {
    id: control
    property string iconName
    property string tip: ""
    property int iconSize: 15
    implicitWidth: 28
    implicitHeight: 28
    hoverEnabled: true
    focusPolicy: Qt.StrongFocus
    Accessible.name: tip
    contentItem: Item {
        Icon { anchors.centerIn: parent; name: control.iconName; color: control.hovered ? Theme.text : Theme.text2; size: control.iconSize }
    }
    background: Rectangle {
        radius: 4
        color: control.hovered || control.down ? Theme.hover : "transparent"
        FocusRing { target: control; radius_: 6 }
    }
    ToolTip { visible: control.hovered && control.tip !== ""; text: control.tip }
}
