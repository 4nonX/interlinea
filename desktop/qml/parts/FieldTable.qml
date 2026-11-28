import QtQuick
import QtQuick.Layouts
import Interlinea

// The fields of a termbase being made: each can be renamed, moved to
// another level, given another kind or taken out, and more added.
ColumnLayout {
    id: table
    property var fields: []
    property var inherited: []
    signal changed(var fields)
    spacing: Theme.s2
    function set(i, key, value) {
        const next = TermData.copy(fields)
        next[i][key] = value
        fields = next
        changed(next)
    }
    Txt { visible: (table.fields || []).length === 0; Layout.fillWidth: true; role: "help"; text: qsTr("No fields: entries hold their terms and each term's status.") }
    Table {
        visible: (table.fields || []).length > 0
        columns: [{ title: qsTr("Name"), fill: 1 }, { title: qsTr("Belongs to"), width: 150 }, { title: qsTr("Values"), fill: 1 }, { title: "", width: 48 }]
        Repeater {
            model: table.fields
            TableRow {
                id: frow
                required property var modelData
                required property int index
                readonly property string label: (modelData.name || "").trim() || Fmt.fill(qsTr("field {n}"), { n: index + 1 })
                last: index === (table.fields || []).length - 1
                TextField {
                    text: frow.modelData.name
                    placeholderText: qsTr("Name of the field")
                    Accessible.name: Fmt.fill(qsTr("Name of field {n}"), { n: frow.index + 1 })
                    onEditingFinished: if (text !== frow.modelData.name) table.set(frow.index, "name", text)
                }
                ComboBox {
                    model: Fields.levels().map(l => l.name)
                    currentIndex: Math.max(0, Fields.levels().findIndex(l => l.id === frow.modelData.level))
                    Accessible.name: Fmt.fill(qsTr("Where {field} belongs"), { field: frow.label })
                    onActivated: i => table.set(frow.index, "level", Fields.levels()[i].id)
                }
                ColumnLayout {
                    spacing: Theme.s1
                    ComboBox {
                        Layout.fillWidth: true
                        model: Fields.kinds().map(k => k.name)
                        currentIndex: Math.max(0, Fields.kinds().findIndex(k => k.id === frow.modelData.kind))
                        Accessible.name: Fmt.fill(qsTr("Values of {field}"), { field: frow.label })
                        onActivated: i => table.set(frow.index, "kind", Fields.kinds()[i].id)
                    }
                    TextArea {
                        Layout.fillWidth: true
                        visible: frow.modelData.kind === "picklist"
                        minimumLines: 3
                        text: (frow.modelData.values || []).join("\n")
                        placeholderText: qsTr("One value a line\nsuch as\nnoun\nverb")
                        Accessible.name: Fmt.fill(qsTr("The values of {field}, one a line"), { field: frow.label })
                        area.onActiveFocusChanged: if (!area.activeFocus) table.set(frow.index, "values", Fields.tidy(text))
                    }
                }
                IconButton {
                    iconName: "trash"
                    tip: Fmt.fill(qsTr("Remove {field}"), { field: frow.label })
                    onClicked: { const next = table.fields.filter((f, k) => k !== frow.index); table.fields = next; table.changed(next) }
                }
            }
        }
    }
    Button {
        small: true
        iconName: "plus"
        text: qsTr("Add a field")
        onClicked: { const next = table.fields.concat([{ name: "", level: "entry", kind: "text", values: [] }]); table.fields = next; table.changed(next) }
    }
}
