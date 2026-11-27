import QtQuick
import QtQuick.Templates as T
import Interlinea

// The menu bar over the window, in the colour of the navigation.
T.MenuBar {
    id: control
    implicitWidth: contentItem.implicitWidth
    implicitHeight: 30
    font.family: Theme.font
    font.pixelSize: Theme.small
    delegate: MenuBarItem {}
    contentItem: Row {
        leftPadding: Theme.s2
        spacing: 0
        Repeater { model: control.contentModel }
    }
    background: Rectangle {
        color: Theme.rail
        Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
    }
}
