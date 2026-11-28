import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// The chosen entry (the right pane of Terminology): its head with who
// changed it last, its fields, each language with its terms, and its
// history; read only for those who may not edit.
Rectangle {
    id: detail
    property var tb: null           // the page: what it knows of the termbase
    property var entry: null
    property bool masterView: false // an entry of the master under a project termbase
    property int tab: 0
    property var history: null
    readonly property bool edit: tb.may("edit")
    readonly property var langs: {
        const out = tb.languages.slice()
        for (const l of TermData.languages(entry)) if (out.indexOf(l) < 0) out.push(l)
        return out
    }
    readonly property var headTerm: entry ? TermData.head(entry, tb.languages[0] || "") : ({ text: "", lang: "" })
    radius: Theme.rPanel
    color: Theme.surface
    border.color: Theme.border
    clip: true

    onEntryChanged: if (tab === 1 && entry) loadHistory()
    function loadHistory() {
        history = null
        if (!entry || masterView) return
        const id = entry.id
        Api.get(tb.cid, tb.path("/entries/" + Api.seg(id) + "/history"), r => {
            if (!detail.entry || detail.entry.id !== id) return
            history = r.ok ? r.data : { error: r.error.message }
        })
    }
    function fieldsAt(level) { return tb.fields.filter(f => f.level === level) }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 1
        spacing: 0
        visible: !!detail.entry

        // The head.
        ColumnLayout {
            Layout.fillWidth: true
            Layout.margins: Theme.s4
            Layout.bottomMargin: Theme.s3
            spacing: Theme.s1
            RowLayout {
                spacing: Theme.s2
                EntryTerms {
                    Layout.fillWidth: true
                    entry: detail.entry
                    languages: tb.languages
                    big: true
                    Accessible.role: Accessible.Heading
                }
                ConfirmButton {
                    visible: detail.edit && !detail.masterView
                    text: qsTr("Delete entry")
                    iconName: "trash"
                    question: qsTr("Delete this entry? It can be brought back from Deleted, without its history.")
                    action: qsTr("Delete")
                    onConfirmed: tb.deleteEntry(detail.entry, detail.headTerm.text)
                }
            }
            Flow {
                Layout.fillWidth: true
                spacing: Theme.s2
                readonly property string when: detail.entry ? Fmt.time(detail.entry.changedAt || detail.entry.createdAt) : ""
                readonly property string by: detail.entry ? (detail.entry.changedBy || detail.entry.createdBy || "") : ""
                // Each as wide as it is, and no wider than the pane: in a
                // narrow one it goes onto a second line.
                Txt {
                    visible: !detail.masterView && parent.when !== ""
                    width: Math.min(implicitWidth, parent.width)
                    role: "sub"
                    text: parent.by ? Fmt.fill(qsTr("Last changed {when} by {who}"), { when: parent.when, who: parent.by }) : Fmt.fill(qsTr("Last changed {when}"), { when: parent.when })
                }
                Txt {
                    visible: detail.masterView
                    width: Math.min(implicitWidth, parent.width)
                    role: "sub"
                    text: detail.entry ? (parent.when ? Fmt.fill(qsTr("In {master}, last changed {when}"), { master: detail.entry.termbase, when: parent.when })
                                                      : Fmt.fill(qsTr("In {master}"), { master: detail.entry.termbase })) : ""
                }
                Txt {
                    visible: !detail.masterView && !!detail.entry && !!tb.bases[detail.entry.id]
                    width: Math.min(implicitWidth, parent.width)
                    role: "sub"
                    text: detail.entry ? Words.get("basedOn") + " " + (tb.bases[detail.entry.id] || "") : ""
                }
                Badge { visible: !detail.masterView && tb.isProject && !!detail.entry && !tb.bases[detail.entry.id]; text: qsTr("Not in a master yet") }
            }
        }
        Tabs {
            visible: !detail.masterView
            Layout.fillWidth: true
            name: qsTr("Entry views")
            current: detail.tab
            model: [qsTr("Entry"), { text: Words.get("history") + " (" + Math.max(1, detail.entry ? (detail.entry.version || 1) : 1) + ")", icon: "history" }]
            onChanged: i => { detail.tab = i; if (i === 1) detail.loadHistory() }
        }
        Rectangle { visible: detail.masterView; Layout.fillWidth: true; height: 1; color: Theme.border }

        B.ScrollView {
            id: scroll
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            contentWidth: availableWidth
            B.ScrollBar.vertical: ScrollBar { parent: scroll; x: scroll.width - width; height: scroll.height }
            ColumnLayout {
                width: scroll.availableWidth - 2 * Theme.s4
                x: Theme.s4
                spacing: Theme.s3

                Item { implicitHeight: Theme.s2 }

                // The master's entry, to take into the project or leave out.
                Note {
                    visible: detail.masterView && detail.edit
                    text: qsTr("This is the master's entry. To change it, take it into the project: your version is used here and goes to the master's reviewers when you merge.")
                }
                Row {
                    visible: detail.masterView && detail.edit
                    spacing: Theme.s2
                    Button { small: true; iconName: "pencil"; text: qsTr("Change in this project"); onClicked: tb.override(detail.entry) }
                    ConfirmButton {
                        text: qsTr("Leave out of this project")
                        question: qsTr("Leave this entry out of the project? Merging asks the master's reviewers to delete it there.")
                        action: qsTr("Leave out")
                        onConfirmed: tb.leaveOut(detail.entry)
                    }
                }

                // To read only.
                EntryRead {
                    visible: (detail.masterView || !detail.edit) && detail.tab === 0
                    entry: detail.entry
                }

                // To edit: the entry's fields, then each language.
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: !detail.masterView && detail.edit && detail.tab === 0
                    spacing: Theme.s3
                    Repeater {
                        model: detail.fieldsAt("entry")
                        FieldValue {
                            required property var modelData
                            field: modelData
                            termbase: tb.may("manage") ? tb.name : ""
                            onRemoveField: tb.removeField(modelData.name)
                            value: TermData.value(detail.entry ? detail.entry.fields : [], modelData.name)
                            onSaved: v => {
                                const name = modelData.name
                                tb.change(detail.entry.id, Fmt.fill(v ? qsTr("Set {field}") : qsTr("Cleared {field}"), { field: name }),
                                          e => TermData.withEntryField(e, name, v), false)
                            }
                        }
                    }
                    AddFieldForm {
                        termbase: tb.name
                        taken: tb.fields.map(f => f.name)
                        onAdded: field => tb.addField(field)
                    }
                    Repeater {
                        model: detail.langs
                        ColumnLayout {
                            id: block
                            required property string modelData
                            readonly property string lang: modelData
                            readonly property var section: TermData.language(detail.entry, lang)
                            readonly property var terms: section.terms || []
                            Layout.fillWidth: true
                            spacing: Theme.s2
                            Accessible.name: Backend.languageName(lang)
                            Rectangle { Layout.fillWidth: true; Layout.topMargin: Theme.s2; height: 1; color: Theme.border }
                            Row {
                                Layout.topMargin: Theme.s2
                                spacing: Theme.s2
                                Txt { text: Backend.languageName(block.lang); role: "strong"; wrapMode: Text.NoWrap }
                                LangChip { code: block.lang }
                            }
                            Repeater {
                                model: detail.fieldsAt("language")
                                FieldValue {
                                    required property var modelData
                                    field: modelData
                                    small: true
                                    termbase: tb.may("manage") ? tb.name : ""
                                    onRemoveField: tb.removeField(modelData.name)
                                    value: TermData.value(block.section.fields, modelData.name)
                                    onSaved: v => {
                                        const name = modelData.name, lang = block.lang
                                        tb.change(detail.entry.id, Fmt.fill(v ? qsTr("Set {field} ({language})") : qsTr("Cleared {field} ({language})"),
                                                                            { field: name, language: Backend.languageName(lang) }),
                                                  e => TermData.withLanguageField(e, lang, name, v), false)
                                    }
                                }
                            }
                            Txt { visible: block.terms.length === 0; role: "muted"; text: qsTr("No term yet") }
                            Repeater {
                                model: block.terms
                                TermItem {
                                    required property var modelData
                                    required property int index
                                    term: modelData
                                    lang: block.lang
                                    terms: block.terms
                                    termFields: detail.fieldsAt("term")
                                    termbase: tb.may("manage") ? tb.name : ""
                                    onRemoveField: name => tb.removeField(name)
                                    onChange: (summary, apply, rebuild) => tb.change(detail.entry.id, summary, apply, rebuild)
                                    onRemoved: text => tb.notice(Fmt.fill(qsTr("Removed “{term}”"), { term: text }))
                                }
                            }
                            // A term or a synonym.
                            ColumnLayout {
                                id: addTerm
                                property bool opened: false
                                Layout.fillWidth: true
                                spacing: Theme.s1
                                function submit() {
                                    const d = newTerm.text.trim()
                                    if (!d || TermData.hasTerm(block.terms, d, -1)) return
                                    const lang = block.lang, s = ["proposed", "approved", "forbidden"][newStatus.currentIndex]
                                    tb.change(detail.entry.id, Fmt.fill(qsTr("Added “{term}”"), { term: d }), e => {
                                        const ts = TermData.copy(TermData.terms(e, lang))
                                        ts.push({ text: d, status: s })
                                        return TermData.withTerms(e, lang, ts)
                                    }, true)
                                    if (tb.languages.indexOf(lang) < 0) tb.addLanguage(lang)
                                    newTerm.text = ""
                                    opened = false
                                }
                                Button {
                                    visible: !addTerm.opened
                                    kind: "quiet"
                                    small: true
                                    iconName: "plus"
                                    text: block.terms.length ? qsTr("Add a synonym") : qsTr("Add a term")
                                    Accessible.name: Fmt.fill(block.terms.length ? qsTr("Add a synonym in {language}") : qsTr("Add a term in {language}"), { language: Backend.languageName(block.lang) })
                                    onClicked: { addTerm.opened = true; newTerm.forceActiveFocus() }
                                }
                                RowLayout {
                                    visible: addTerm.opened
                                    spacing: Theme.s1
                                    TextField {
                                        id: newTerm
                                        Layout.fillWidth: true
                                        implicitHeight: 28
                                        placeholderText: Fmt.fill(qsTr("Term in {language}"), { language: Backend.languageName(block.lang) })
                                        invalid: text.trim() !== "" && TermData.hasTerm(block.terms, text.trim(), -1)
                                        horizontalAlignment: Backend.rtl(block.lang) ? TextInput.AlignRight : TextInput.AlignLeft
                                        onAccepted: addTerm.submit()
                                    }
                                    ComboBox {
                                        id: newStatus
                                        implicitWidth: 120
                                        implicitHeight: 28
                                        model: ["proposed", "approved", "forbidden"].map(s => TermData.statusLabel(s))
                                        Accessible.name: qsTr("Status")
                                    }
                                    Button { kind: "primary"; small: true; text: qsTr("Add"); enabled: newTerm.text.trim() !== "" && !newTerm.invalid; onClicked: addTerm.submit() }
                                    Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: { newTerm.text = ""; addTerm.opened = false } }
                                }
                                Txt {
                                    visible: addTerm.opened && newTerm.invalid
                                    role: "error"
                                    text: Fmt.fill(qsTr("This entry already has “{term}”."), { term: newTerm.text.trim() })
                                }
                            }
                        }
                    }
                    ComboBox {
                        readonly property var choices: Backend.commonLanguages().filter(l => detail.langs.indexOf(l.code) < 0)
                        visible: choices.length > 0
                        Layout.maximumWidth: 420
                        Layout.fillWidth: true
                        Layout.topMargin: Theme.s2
                        model: [qsTr("Add a language to the termbase…")].concat(choices.map(l => Backend.languageName(l.code)))
                        currentIndex: 0
                        Accessible.name: Fmt.fill(qsTr("Add a language to {name}"), { name: tb.name })
                        onActivated: i => { if (i > 0) tb.addLanguage(choices[i - 1].code); currentIndex = 0 }
                    }
                }

                // The history: the newest first, each to go back to.
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: detail.tab === 1 && !detail.masterView
                    spacing: 0
                    Txt { visible: !!detail.history && !!detail.history.error; role: "error"; text: detail.history && detail.history.error ? detail.history.error : "" }
                    Repeater {
                        model: Array.isArray(detail.history) ? detail.history.slice().reverse() : []
                        Item {
                            id: ver
                            required property var modelData
                            required property int index
                            readonly property int total: detail.history.length
                            readonly property int at: total - 1 - index
                            readonly property var then: modelData.entry || {}
                            Layout.fillWidth: true
                            implicitHeight: hv.implicitHeight + 2 * Theme.s3
                            Rectangle { x: 4; width: 2; height: parent.height; color: Theme.border }
                            Rectangle { x: 0; y: Theme.s3 + 4; width: 10; height: 10; radius: 5; color: ver.index === 0 ? Theme.accent : Theme.surface; border.color: ver.index === 0 ? Theme.accent : Theme.borderStrong }
                            ColumnLayout {
                                id: hv
                                x: Theme.s4 + 4
                                y: Theme.s3
                                width: parent.width - x
                                spacing: 2
                                Txt { Layout.fillWidth: true; role: "strong"; text: ver.modelData.summary || (ver.at === 0 ? qsTr("Created") : qsTr("Changed")) }
                                Txt { Layout.fillWidth: true; role: "sub"; text: [ver.modelData.by || "", Fmt.time(ver.modelData.at)].filter(x => x).join(", ") }
                                Txt {
                                    Layout.fillWidth: true
                                    role: "muted"
                                    font.pixelSize: Theme.small
                                    text: TermData.languages(ver.then).map(l => ({ l: l, ts: TermData.terms(ver.then, l).map(t => t.text) }))
                                                  .filter(x => x.ts.length).map(x => x.l + ": " + x.ts.join(", ")).join("; ")
                                }
                                ConfirmButton {
                                    visible: ver.index > 0 && detail.edit
                                    danger: false
                                    text: qsTr("Restore this version")
                                    question: qsTr("Set the entry back to this version? What it is now stays in the history.")
                                    action: qsTr("Restore")
                                    onConfirmed: tb.revert(detail.entry.id, ver.modelData.version, Fmt.time(ver.modelData.at), () => { detail.tab = 0 })
                                }
                            }
                        }
                    }
                }
                Item { implicitHeight: Theme.s4 }
            }
        }
    }
}
