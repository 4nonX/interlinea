import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's text input: 32 px, a 1 px border, the accent ring on focus;
// red when invalid.
T.TextField {
    id: control
    property bool invalid: false
    implicitWidth: 240
    implicitHeight: Theme.controlHeight
    leftPadding: Theme.s2 + 1
    rightPadding: Theme.s2 + 1
    verticalAlignment: TextInput.AlignVCenter
    color: Theme.text
    placeholderTextColor: Theme.text3
    selectionColor: Theme.selection
    selectedTextColor: Theme.text
    font.family: Theme.font
    font.pixelSize: Theme.body
    hoverEnabled: true
    selectByMouse: true
    opacity: enabled ? 1 : 0.55
    Accessible.name: placeholderText

    Text {
        x: control.leftPadding
        width: control.width - control.leftPadding - control.rightPadding
        height: control.height
        verticalAlignment: Text.AlignVCenter
        text: control.placeholderText
        font: control.font
        color: control.placeholderTextColor
        elide: Text.ElideRight
        visible: !control.length && !control.preeditText
    }

    background: Rectangle {
        radius: Theme.rControl
        color: Theme.surface
        border.width: control.activeFocus ? 2 : 1
        border.color: control.invalid ? Theme.danger : control.activeFocus ? Theme.accent : control.hovered ? Theme.borderHover : Theme.borderStrong
    }
}
