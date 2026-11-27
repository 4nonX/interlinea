import QtQuick
import QtQuick.Layouts
import Interlinea

// Pre-translation: which memory matches go into empty segments, and which
// of them are confirmed.
ColumnLayout {
    id: prep
    property var form
    readonly property var p: form.s.prepare || {}
    readonly property bool on: !!p.pretranslate
    function set(key, value) { form.edit("prepare", x => { x[key] = value }) }
    spacing: Theme.s2
    enabled: !form.readOnly
    Layout.fillWidth: true

    Txt { role: "label"; text: prep.form.isNew ? qsTr("Pre-translation") : qsTr("Matches to insert") }
    CheckBox {
        visible: prep.form.isNew
        text: qsTr("Fill empty segments with memory matches")
        checked: prep.on
        onToggled: prep.set("pretranslate", checked)
    }
    ColumnLayout {
        visible: prep.on || !prep.form.isNew
        Layout.fillWidth: true
        Layout.leftMargin: prep.form.isNew ? 26 : 0
        spacing: Theme.s2
        RowLayout {
            spacing: Theme.s2
            Txt { text: qsTr("Lowest match to insert") }
            NumberField {
                from: 50; to: 100
                value: prep.p.minScore || 100
                Accessible.name: qsTr("Lowest match to insert")
                onEdited: v => prep.set("minScore", v)
            }
            Txt { text: "%" }
        }
        Txt { visible: text !== ""; role: "error"; text: prep.form.problemsAt("prepare.minScore") }
        CheckBox { text: qsTr("Confirm segments filled with a context match"); checked: !!prep.p.confirmContext; onToggled: prep.set("confirmContext", checked) }
        CheckBox { text: qsTr("Confirm segments filled with a 100% match"); checked: !!prep.p.confirmExact; onToggled: prep.set("confirmExact", checked) }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Other matches are inserted as drafts for you to check. Locked and translated segments are left alone.") }
        Note { visible: prep.form.summary !== ""; text: prep.form.summary }
    }
}
