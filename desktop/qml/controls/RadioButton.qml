import QtQuick
import QtQuick.Templates as T
import Interlinea

// A radio button of 16 px with its text; description under the text.
T.RadioButton {
    id: control
    property string description: ""
    property bool bold: false
    implicitWidth: implicitContentWidth + leftPadding + rightPadding
    implicitHeight: Math.max(implicitContentHeight, 20)
    spacing: Theme.s2
    leftPadding: 16 + spacing
    padding: 0
    hoverEnabled: true
    font.family: Theme.font
    font.pixelSize: Theme.body
    opacity: enabled ? 1 : 0.55
    Accessible.description: description

    indicator: Rectangle {
        x: 0
        y: 2
        width: 16
        height: 16
        radius: 8
        color: Theme.surface
        border.color: control.checked ? Theme.accent : control.hovered ? Theme.borderHover : Theme.borderStrong
        border.width: control.checked ? 5 : 1
        FocusRing { target: control; radius_: 10 }
    }

    // As wide as its longest line, so the control sizes itself in a
    // Flow; narrower when a layout gives it less, and then it wraps.
    contentItem: Item {
        implicitWidth: Math.max(label.implicitWidth, more.visible ? more.implicitWidth : 0)
        implicitHeight: lines.implicitHeight
        Column {
            id: lines
            width: parent.width
            spacing: 2
            Txt {
                id: label
                width: parent.width
                text: control.text
                role: control.bold ? "strong" : ""
            }
            Txt {
                id: more
                width: parent.width
                visible: control.description !== ""
                text: control.description
                role: "muted"
            }
        }
    }
}
