import QtQuick
import QtQuick.Templates as T
import Interlinea

// A thin scroll bar in the border colour, as the web's.
T.ScrollBar {
    id: control
    implicitWidth: 10
    implicitHeight: 10
    padding: 2
    minimumSize: 0.08
    visible: policy !== T.ScrollBar.AlwaysOff && size < 1
    contentItem: Rectangle {
        implicitWidth: 6
        implicitHeight: 6
        radius: 3
        color: control.pressed ? Theme.text3 : Theme.borderStrong
        opacity: control.active || control.hovered ? 1 : 0.7
    }
}
