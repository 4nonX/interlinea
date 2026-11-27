import QtQuick
import Interlinea

// The web's ConfirmButton: a quiet button that asks before it acts.
// Pressed, it shows the question in red with the confirming button and
// Cancel; danger: false makes the step the page's main button instead.
Item {
    id: confirm
    property string text
    property string question
    property string action: text
    property string iconName: ""
    property bool small: true
    property bool danger: true
    property bool asking: false
    property bool available: true
    property string accessibleName: text
    signal confirmed()
    implicitWidth: asking ? askRow.implicitWidth : button.implicitWidth
    implicitHeight: asking ? askRow.implicitHeight : button.implicitHeight

    Button {
        id: button
        visible: !confirm.asking
        text: confirm.text
        iconName: confirm.iconName
        kind: "quiet"
        small: confirm.small
        enabled: confirm.available
        Accessible.name: confirm.accessibleName
        onClicked: {
            confirm.asking = true
            yes.forceActiveFocus()
        }
    }
    Row {
        id: askRow
        visible: confirm.asking
        spacing: Theme.s1
        Txt {
            anchors.verticalCenter: parent.verticalCenter
            text: confirm.question
            color: confirm.danger ? Theme.danger : Theme.text
            wrapMode: Text.NoWrap
            font.pixelSize: confirm.small ? Theme.small : Theme.body
            width: Math.min(implicitWidth, 520)
            elide: Text.ElideRight
        }
        Button {
            id: yes
            text: confirm.action
            kind: confirm.danger ? "danger" : "primary"
            small: true
            Accessible.description: confirm.question
            onClicked: {
                confirm.asking = false
                confirm.confirmed()
            }
        }
        Button {
            text: qsTr("Cancel")
            kind: "quiet"
            small: true
            onClicked: {
                confirm.asking = false
                button.forceActiveFocus()
            }
        }
    }
}
