import QtQuick
import QtQuick.Layouts
import Interlinea

// A term of an entry: its text, status and fields, to change, edit and
// remove; .term in the web app.
Rectangle {
    id: item
    property var term: ({})
    property string lang
    property int index
    property var terms: []
    property var termFields: []
    property string termbase: ""   // set for those who manage the termbase
    signal removeField(string name)
    property bool editing: false
    signal change(string summary, var apply, bool rebuild)
    signal removed(string text)
    readonly property string text: term.text || ""
    readonly property string status: term.status || "proposed"
    Layout.fillWidth: true
    implicitHeight: body.implicitHeight + 2 * Theme.s3
    radius: Theme.rControl
    color: Theme.surface2
    border.color: Theme.border
    Accessible.role: Accessible.Grouping
    Accessible.name: text

    function submit() {
        const d = draft.text.trim()
        if (!d || TermData.hasTerm(terms, d, index)) return
        editing = false
        if (d === text) return
        const lang = item.lang, i = item.index
        item.change(Fmt.fill(qsTr("Changed “{from}” to “{to}”"), { from: text, to: d }), e => {
            const ts = TermData.copy(TermData.terms(e, lang))
            if (i < ts.length) ts[i].text = d
            return TermData.withTerms(e, lang, ts)
        }, true)
    }

    ColumnLayout {
        id: body
        x: Theme.s3; y: Theme.s3
        width: parent.width - 2 * Theme.s3
        spacing: Theme.s2
        RowLayout {
            visible: !item.editing
            spacing: Theme.s2
            Txt {
                Layout.fillWidth: true
                text: item.text
                font.pixelSize: 16
                font.weight: 600
                horizontalAlignment: Backend.rtl(item.lang) ? Text.AlignRight : Text.AlignLeft
            }
            Badge { text: TermData.statusLabel(item.status); kind: item.status === "approved" ? "ok" : item.status === "forbidden" ? "danger" : "warn" }
            ComboBox {
                implicitWidth: 120
                implicitHeight: 28
                readonly property var states: ["approved", "proposed", "forbidden"]
                model: states.map(s => TermData.statusLabel(s))
                currentIndex: Math.max(0, states.indexOf(item.status))
                Accessible.name: Fmt.fill(qsTr("Status of {term}"), { term: item.text })
                onActivated: i => {
                    const s = states[i]
                    if (s === item.status) return
                    const lang = item.lang, k = item.index
                    item.change(Fmt.fill(qsTr("Set “{term}” to {status}"), { term: item.text, status: s }), e => {
                        const ts = TermData.copy(TermData.terms(e, lang))
                        if (k < ts.length) ts[k].status = s
                        return TermData.withTerms(e, lang, ts)
                    }, true)
                }
            }
            IconButton {
                iconName: "pencil"
                tip: Fmt.fill(qsTr("Edit {term}"), { term: item.text })
                onClicked: { item.editing = true; draft.text = item.text; draft.forceActiveFocus(); draft.selectAll() }
            }
            ConfirmButton {
                text: ""
                iconName: "trash"
                action: qsTr("Remove")
                accessibleName: Fmt.fill(qsTr("Remove {term}"), { term: item.text })
                question: Fmt.fill(qsTr("Remove “{term}”?"), { term: item.text })
                onConfirmed: {
                    const lang = item.lang, k = item.index
                    item.change(Fmt.fill(qsTr("Removed “{term}”"), { term: item.text }), e => {
                        const ts = TermData.copy(TermData.terms(e, lang))
                        if (k < ts.length) ts.splice(k, 1)
                        return TermData.withTerms(e, lang, ts)
                    }, true)
                    item.removed(item.text)
                }
            }
        }
        ColumnLayout {
            visible: item.editing
            spacing: Theme.s1
            RowLayout {
                spacing: Theme.s1
                TextField {
                    id: draft
                    Layout.fillWidth: true
                    invalid: TermData.hasTerm(item.terms, text.trim(), item.index)
                    Accessible.name: qsTr("Term")
                    horizontalAlignment: Backend.rtl(item.lang) ? TextInput.AlignRight : TextInput.AlignLeft
                    onAccepted: item.submit()
                }
                Button { kind: "primary"; small: true; text: qsTr("Save"); enabled: draft.text.trim() !== "" && !draft.invalid; onClicked: item.submit() }
                Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: item.editing = false }
            }
            Txt {
                visible: draft.invalid
                role: "error"
                text: Fmt.fill(qsTr("This entry already has “{term}” in {language}."), { term: draft.text.trim(), language: Backend.languageName(item.lang) })
            }
        }
        // The term's own fields.
        Repeater {
            model: item.termFields
            FieldValue {
                required property var modelData
                field: modelData
                small: true
                termbase: item.termbase
                onRemoveField: item.removeField(modelData.name)
                value: TermData.value(item.term.fields, modelData.name)
                onSaved: v => {
                    const lang = item.lang, k = item.index, name = modelData.name
                    item.change(Fmt.fill(v ? qsTr("Set {field} of “{term}”") : qsTr("Cleared {field} of “{term}”"), { field: name, term: item.text }),
                                e => TermData.withTermField(e, lang, k, name, v), false)
                }
            }
        }
    }
}
