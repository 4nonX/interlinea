import QtQuick
import QtQuick.Templates as T
import Interlinea

// A tip in the text colour on the surface's inverse, after a short wait.
T.ToolTip {
    id: control
    delay: 500
    padding: 6
    leftPadding: 8
    rightPadding: 8
    font.family: Theme.font
    font.pixelSize: Theme.tiny
    contentItem: Text {
        text: control.text
        color: Theme.dark ? Theme.text : "#ffffff"
        font: control.font
        wrapMode: Text.Wrap
    }
    background: Rectangle {
        radius: 4
        color: Theme.dark ? "#3a424e" : "#1f2630"
    }
}
