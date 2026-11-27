import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's number input: a number between from and to, typed or stepped
// with the arrow keys or the small buttons; edited(value) when it changes.
TextField {
    id: field
    property real value: 0
    property real from: 0
    property real to: 100
    property real step: 1
    property int decimals: 0
    signal edited(real value)
    implicitWidth: 96
    rightPadding: 22
    inputMethodHints: Qt.ImhFormattedNumbersOnly
    horizontalAlignment: TextInput.AlignRight
    text: Number(value).toFixed(decimals)
    invalid: !ok
    readonly property bool ok: {
        const n = Number(text.replace(",", "."))
        return text.trim() !== "" && !isNaN(n) && n >= from && n <= to
    }
    function take(n) {
        const v = Math.min(to, Math.max(from, Number(n.toFixed(decimals))))
        if (v !== value) edited(v)
        // Shows the value as it is now, and follows it again.
        text = Qt.binding(() => Number(value).toFixed(decimals))
    }
    onEditingFinished: if (ok) take(Number(text.replace(",", "."))); else text = Qt.binding(() => Number(value).toFixed(decimals))
    Keys.onUpPressed: take((ok ? Number(text.replace(",", ".")) : value) + step)
    Keys.onDownPressed: take((ok ? Number(text.replace(",", ".")) : value) - step)
    Column {
        anchors.right: parent.right
        anchors.rightMargin: 3
        anchors.verticalCenter: parent.verticalCenter
        IconButton { implicitWidth: 16; implicitHeight: 12; iconSize: 11; iconName: "chevron-up"; focusPolicy: Qt.NoFocus; Accessible.name: qsTr("More"); onClicked: field.take((field.ok ? Number(field.text.replace(",", ".")) : field.value) + field.step) }
        IconButton { implicitWidth: 16; implicitHeight: 12; iconSize: 11; iconName: "chevron-down"; focusPolicy: Qt.NoFocus; Accessible.name: qsTr("Less"); onClicked: field.take((field.ok ? Number(field.text.replace(",", ".")) : field.value) - field.step) }
    }
}
