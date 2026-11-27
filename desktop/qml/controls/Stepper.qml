import QtQuick
import Interlinea

// The web's .stepper: the numbered steps of a wizard, the current one in
// the accent, those done with a check and a link back to them.
Row {
    id: stepper
    property var steps: []
    property int current: 0
    property bool clickable: true
    signal stepClicked(int index)
    spacing: Theme.s4
    Accessible.role: Accessible.List
    Accessible.name: qsTr("Steps")
    Repeater {
        model: stepper.steps
        Row {
            id: step
            required property string modelData
            required property int index
            readonly property bool done: index < stepper.current
            readonly property bool now: index === stepper.current
            spacing: Theme.s2
            Accessible.role: Accessible.ListItem
            Accessible.name: now ? Fmt.fill(qsTr("Step {n}, {name}, current"), { n: index + 1, name: modelData }) : Fmt.fill(qsTr("Step {n}, {name}"), { n: index + 1, name: modelData })
            Rectangle {
                width: 22; height: 22; radius: 11
                anchors.verticalCenter: parent.verticalCenter
                color: step.now ? Theme.accent : step.done ? Theme.okBg : Theme.badge
                Text {
                    anchors.centerIn: parent
                    visible: !step.done
                    text: step.index + 1
                    color: step.now ? "#ffffff" : Theme.text2
                    font.family: Theme.font
                    font.pixelSize: Theme.tiny
                    font.weight: 600
                }
                Icon { anchors.centerIn: parent; visible: step.done; name: "check"; color: Theme.ok; size: 13 }
            }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: step.modelData
                color: step.now ? Theme.text : step.done && stepper.clickable ? Theme.accentText : Theme.text2
                font.family: Theme.font
                font.pixelSize: Theme.small
                font.weight: step.now ? 600 : 500
                font.underline: step.done && stepper.clickable && area.containsMouse
                MouseArea {
                    id: area
                    anchors.fill: parent
                    enabled: step.done && stepper.clickable
                    hoverEnabled: true
                    cursorShape: Qt.PointingHandCursor
                    onClicked: stepper.stepClicked(step.index)
                }
            }
        }
    }
}
