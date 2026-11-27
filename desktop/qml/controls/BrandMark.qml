import QtQuick
import Interlinea

// The app's mark: three lines on the accent, as the web's .brand-mark.
Rectangle {
    width: 28
    height: 28
    radius: 7
    color: Theme.accent
    Accessible.ignored: true
    Item {
        anchors.centerIn: parent
        width: 20
        height: 20
        Rectangle { x: 2; y: 4; width: 16; height: 2; radius: 1; color: "#ffffff" }
        Rectangle { x: 2; y: 9; width: 11; height: 2; radius: 1; color: "#ffffff"; opacity: 0.55 }
        Rectangle { x: 2; y: 14; width: 16; height: 2; radius: 1; color: "#ffffff" }
    }
}
