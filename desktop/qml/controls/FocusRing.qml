import QtQuick
import Interlinea

// The web's :focus-visible ring: 2 px of the accent, 2 px out.
Rectangle {
    property Item target: parent
    property int radius_: Theme.rControl + 2
    property int inset: -3
    visible: target && target.visualFocus === true
    anchors.fill: parent
    anchors.margins: inset
    radius: radius_
    color: "transparent"
    border.color: Theme.accent
    border.width: 2
    z: 10
}
