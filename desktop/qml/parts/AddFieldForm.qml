import QtQuick
import QtQuick.Layouts
import Interlinea

// Adding a field to a termbase from an entry: its name, where its value
// belongs and the values it takes; every entry gets it.
ColumnLayout {
    id: form
    property string termbase
    property var taken: []
    property bool opened: false
    signal added(var field)
    Layout.fillWidth: !opened ? false : true
    spacing: Theme.s2

    property string level: "entry"
    readonly property var draft: ({ name: name.text, level: level, kind: Fields.kinds()[kindBox.currentIndex].id, values: Fields.tidy(values.text) })
    readonly property string same: taken.find(t => name.text.trim() && t.trim().toLowerCase() === name.text.trim().toLowerCase()) || ""
    readonly property string similar: same || name.text.trim().length < 3 ? "" : (taken.find(t => t.toLowerCase().startsWith(name.text.trim().slice(0, 3).toLowerCase())) || "")

    function reset() { name.text = ""; kindBox.currentIndex = 0; values.text = ""; level = "entry" }
    function submit() {
        if (Fields.problem(draft, taken)) return
        opened = false
        added(Fields.toJson(draft))
        reset()
    }

    Button {
        visible: !form.opened
        kind: "quiet"
        small: true
        iconName: "plus"
        text: qsTr("Add a field")
        onClicked: { form.opened = true; name.forceActiveFocus() }
    }
    Rectangle {
        visible: form.opened
        Layout.fillWidth: true
        implicitHeight: box.implicitHeight + 2 * Theme.s3
        radius: Theme.rControl
        color: Theme.surface2
        border.color: Theme.border
        Accessible.role: Accessible.Grouping
        Accessible.name: qsTr("Add a field")
        ColumnLayout {
            id: box
            x: Theme.s3; y: Theme.s3
            width: parent.width - 2 * Theme.s3
            spacing: Theme.s2
            Field { label: qsTr("Field name"); TextField { id: name; placeholderText: qsTr("For example Source, Region, Register"); onAccepted: form.submit() } }
            Txt { role: "label"; text: qsTr("Where does the value belong?") }
            Repeater {
                model: Fields.levels()
                RadioButton {
                    required property var modelData
                    text: modelData.name
                    description: modelData.hint
                    checked: form.level === modelData.id
                    onClicked: form.level = modelData.id
                }
            }
            Field { label: qsTr("Values"); ComboBox { id: kindBox; model: Fields.kinds().map(k => k.name) } }
            TextArea {
                id: values
                Layout.fillWidth: true
                visible: form.draft.kind === "picklist"
                minimumLines: 4
                placeholderText: qsTr("One value a line\nsuch as\nnoun\nverb")
                Accessible.name: qsTr("The values of the list, one a line")
            }
            Txt {
                Layout.fillWidth: true
                role: "help"
                text: form.same ? Fmt.fill(qsTr("{termbase} already has “{field}”."), { termbase: form.termbase, field: form.same })
                     : form.similar ? Fmt.fill(qsTr("Did you mean the existing field “{field}”?"), { field: form.similar })
                     : qsTr("Every entry in this termbase gets the field. Nothing else changes.")
            }
            Row {
                spacing: Theme.s2
                Button { kind: "primary"; small: true; text: qsTr("Add field"); enabled: Fields.problem(form.draft, form.taken) === ""; onClicked: form.submit() }
                Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: { form.opened = false; form.reset() } }
            }
        }
    }
}
