import QtQuick
import QtQuick.Controls.Basic as B
import QtQuick.Templates as T
import Interlinea

// A text of several lines, as the web's textarea, scrolling when longer
// than its height.
B.ScrollView {
    id: view
    property alias text: area.text
    property alias placeholderText: area.placeholderText
    property alias area: area
    property bool invalid: false
    property int minimumLines: 3
    implicitWidth: 320
    implicitHeight: Math.max(minimumLines * 20 + 16, Math.min(area.implicitHeight, 240))
    Accessible.name: placeholderText

    T.TextArea {
        id: area
        padding: Theme.s2
        leftPadding: Theme.s2 + 1
        color: Theme.text
        placeholderTextColor: Theme.text3
        selectionColor: Theme.selection
        selectedTextColor: Theme.text
        font.family: Theme.font
        font.pixelSize: Theme.body
        wrapMode: TextEdit.Wrap
        selectByMouse: true
        hoverEnabled: true
        Text {
            x: area.leftPadding
            y: area.topPadding
            width: area.width - area.leftPadding - area.rightPadding
            text: area.placeholderText
            font: area.font
            color: area.placeholderTextColor
            wrapMode: Text.Wrap
            visible: !area.length && !area.preeditText
        }
    }

    background: Rectangle {
        radius: Theme.rControl
        color: Theme.surface
        border.width: area.activeFocus ? 2 : 1
        border.color: view.invalid ? Theme.danger : area.activeFocus ? Theme.accent : area.hovered ? Theme.borderHover : Theme.borderStrong
    }
}
