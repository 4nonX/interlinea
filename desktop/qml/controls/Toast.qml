import QtQuick
import Interlinea

// The web's .toast: a notice over the bottom of the window, gone after a
// while; screen readers hear it.
Rectangle {
    id: toast
    property string last: ""
    // What it said, the latest last.
    property var said: []
    function say(text) {
        if (!text) return
        last = text
        said = said.concat([text]).slice(-50)
        label.text = text
        visible = true
        opacity = 1
        timer.restart()
    }
    visible: false
    z: 1000
    anchors.horizontalCenter: parent ? parent.horizontalCenter : undefined
    anchors.bottom: parent ? parent.bottom : undefined
    anchors.bottomMargin: Theme.s5
    width: Math.min(label.implicitWidth + 32, 560)
    height: label.implicitHeight + 20
    radius: 8
    color: Theme.dark ? "#3a424e" : "#1f2630"
    Accessible.role: Accessible.AlertMessage
    Accessible.name: label.text
    Text {
        id: label
        x: 16
        y: 10
        width: Math.min(implicitWidth, 528)
        color: "#ffffff"
        font.family: Theme.font
        font.pixelSize: Theme.body
        font.weight: 500
        wrapMode: Text.Wrap
    }
    Behavior on opacity { NumberAnimation { duration: 150 } }
    Timer { id: timer; interval: 4000; onTriggered: toast.visible = false }
}
