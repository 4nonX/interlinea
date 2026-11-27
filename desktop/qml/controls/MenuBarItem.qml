import QtQuick
import QtQuick.Templates as T
import Interlinea

T.MenuBarItem {
    id: control
    implicitWidth: label.implicitWidth + 2 * Theme.s3
    implicitHeight: 30
    hoverEnabled: true
    font: menuBar ? menuBar.font : undefined
    contentItem: Text {
        id: label
        text: control.text.replace(/&(.)/, "$1")
        color: Theme.text
        font: control.font
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
    }
    background: Rectangle {
        anchors.fill: parent
        anchors.margins: 3
        radius: 4
        color: (control.highlighted && control.menu && control.menu.visible) || control.hovered ? Theme.hover : "transparent"
    }
}
