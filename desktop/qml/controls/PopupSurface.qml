import QtQuick
import Interlinea

// A surface over the page (menus, lists of a choice, tips): the panel's
// border and the web's --shadow-pop.
Item {
    Rectangle {
        anchors.fill: parent
        anchors.topMargin: 3
        anchors.margins: -2
        radius: Theme.rPanel + 2
        color: Theme.shadow
        opacity: 0.35
    }
    Rectangle {
        anchors.fill: parent
        radius: 8
        color: Theme.surface
        border.color: Theme.border
    }
}
