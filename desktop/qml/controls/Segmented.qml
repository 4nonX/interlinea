import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's .segmented: a few choices side by side in a grey track, the
// chosen one white.
Rectangle {
    id: seg
    property var model: []
    property int current: 0
    property string name: ""
    signal changed(int index)
    implicitWidth: row.implicitWidth + 4
    implicitHeight: 30
    radius: Theme.rControl
    color: Theme.dark ? "#2a3038" : "#eceff3"
    Accessible.role: Accessible.Grouping
    Accessible.name: name
    Row {
        id: row
        x: 2; y: 2
        height: parent.height - 4
        Repeater {
            model: seg.model
            T.AbstractButton {
                id: b
                required property var modelData
                required property int index
                height: row.height
                implicitWidth: t.implicitWidth + 20
                hoverEnabled: true
                focusPolicy: Qt.StrongFocus
                Accessible.role: Accessible.RadioButton
                Accessible.name: modelData
                Accessible.checked: seg.current === index
                onClicked: if (seg.current !== index) { seg.current = index; seg.changed(index) }
                contentItem: Text {
                    id: t
                    text: b.modelData
                    color: seg.current === b.index || b.hovered ? Theme.text : Theme.text2
                    font.family: Theme.font
                    font.pixelSize: Theme.small
                    font.weight: 550
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    radius: 5
                    color: seg.current === b.index ? Theme.surface : "transparent"
                    border.color: seg.current === b.index ? Theme.border : "transparent"
                    FocusRing { target: b; radius_: 7 }
                }
            }
        }
    }
}
