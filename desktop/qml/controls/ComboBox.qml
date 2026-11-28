import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's select: 32 px, a border, the chevron at the right, the list
// in a popup with the panel's shadow.
T.ComboBox {
    id: control
    implicitWidth: 240
    implicitHeight: Theme.controlHeight
    leftPadding: Theme.s2 + 2
    rightPadding: 28
    hoverEnabled: true
    font.family: Theme.font
    font.pixelSize: Theme.body
    opacity: enabled ? 1 : 0.55
    Accessible.name: displayText

    // A row's text is the combo box's own for it (textAt): right for
    // every kind of list, strings, objects or a model with roles, where a
    // required modelData is not given and the row would stay empty.
    delegate: T.ItemDelegate {
        required property int index
        width: ListView.view ? ListView.view.width : implicitWidth
        implicitHeight: 30
        leftPadding: Theme.s2 + 2
        rightPadding: Theme.s2
        highlighted: control.highlightedIndex === index
        hoverEnabled: true
        contentItem: Text {
            text: control.textAt(index)
            color: Theme.text
            font.family: Theme.font
            font.pixelSize: Theme.body
            font.weight: control.currentIndex === index ? 600 : 400
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }
        background: Rectangle {
            radius: 4
            color: highlighted ? Theme.accentWeak : "transparent"
        }
    }

    indicator: Icon {
        x: control.width - width - Theme.s2
        y: (control.height - height) / 2
        name: "chevron-down"
        color: Theme.text2
        size: 14
    }

    contentItem: Text {
        text: control.displayText
        color: Theme.text
        font: control.font
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }

    background: Rectangle {
        radius: Theme.rControl
        color: Theme.surface
        border.width: control.visualFocus ? 2 : 1
        border.color: control.visualFocus || control.down ? Theme.accent : control.hovered ? Theme.borderHover : Theme.borderStrong
    }

    popup: T.Popup {
        y: control.height + 2
        width: Math.max(control.width, 160)
        implicitHeight: Math.min(contentItem.implicitHeight + 8, 320)
        padding: 4
        contentItem: ListView {
            clip: true
            implicitHeight: contentHeight
            model: control.delegateModel
            currentIndex: control.highlightedIndex
            T.ScrollBar.vertical: ScrollBar {}
        }
        background: PopupSurface {}
    }
}
