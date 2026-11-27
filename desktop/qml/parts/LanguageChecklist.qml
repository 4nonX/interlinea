import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// Languages to check, found by name or tag; the chosen ones as chips
// above, each to take out again.
ColumnLayout {
    id: list
    property string label
    property var chosen: []
    signal changed(var chosen)
    spacing: Theme.s2
    readonly property var all: Backend.commonLanguages()
    readonly property var shown: all.filter(l => !search.text.trim() || (Backend.languageName(l.code) + " " + l.code).toLowerCase().indexOf(search.text.trim().toLowerCase()) >= 0)
    function toggle(code, on) {
        const next = chosen.filter(c => c !== code)
        if (on) next.push(code)
        chosen = next
        changed(next)
    }
    Flow {
        Layout.fillWidth: true
        visible: list.chosen.length > 0
        spacing: Theme.s1
        Accessible.name: qsTr("Chosen languages")
        Repeater {
            model: list.chosen
            Rectangle {
                required property string modelData
                implicitWidth: cr.implicitWidth + 12
                implicitHeight: 26
                radius: 13
                color: Theme.accentWeak
                Row {
                    id: cr
                    anchors.centerIn: parent
                    spacing: 4
                    Txt { text: Backend.languageName(modelData); color: Theme.accentText; font.pixelSize: Theme.small; wrapMode: Text.NoWrap; anchors.verticalCenter: parent.verticalCenter }
                    IconButton { implicitWidth: 18; implicitHeight: 18; iconName: "x"; tip: Fmt.fill(qsTr("Remove {name}"), { name: Backend.languageName(modelData) }); onClicked: list.toggle(modelData, false) }
                }
            }
        }
    }
    TextField {
        id: search
        Layout.fillWidth: true
        leftPadding: 30
        placeholderText: qsTr("Find a language, such as German or de-AT")
        Accessible.name: qsTr("Find a language")
        Icon { x: 9; anchors.verticalCenter: parent.verticalCenter; name: "search"; color: Theme.text3; size: 15 }
    }
    Rectangle {
        Layout.fillWidth: true
        implicitHeight: 260
        radius: Theme.rControl
        color: Theme.surface
        border.color: Theme.border
        B.ScrollView {
            id: scroll
            anchors.fill: parent
            anchors.margins: Theme.s2
            clip: true
            contentWidth: availableWidth
            B.ScrollBar.vertical: ScrollBar { parent: scroll; x: scroll.width - width + 6; height: scroll.height }
            Flow {
                width: scroll.availableWidth
                spacing: Theme.s1
                Accessible.name: list.label
                Repeater {
                    model: list.shown
                    CheckBox {
                        required property var modelData
                        width: 200
                        text: Backend.languageName(modelData.code) + "  " + modelData.code
                        checked: list.chosen.indexOf(modelData.code) >= 0
                        onToggled: list.toggle(modelData.code, checked)
                    }
                }
                Txt { visible: list.shown.length === 0; role: "muted"; text: qsTr("No language is called that.") }
            }
        }
    }
}
