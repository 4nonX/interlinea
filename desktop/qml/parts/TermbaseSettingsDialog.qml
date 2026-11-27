import QtQuick
import QtQuick.Layouts
import Interlinea

// A termbase's settings, for those who manage it: its name, languages and
// fields; for a master how what projects submit is reviewed, for a
// project termbase the masters it is based on; who may use it on a team
// server; and deleting it.
Dialog {
    id: dlg
    wide: true
    title: qsTr("Termbase settings")
    property var page
    property string name: ""
    // What the page shows; it reloads after every change made here.
    readonly property var detail: page ? page.detail : ({})
    readonly property var entries: page ? page.entries : []
    property var rules: ({})
    property var people: []
    property var masters: []
    property var usedBy: []
    readonly property var def: detail.definition || {}
    readonly property var langs: def.languages || []
    readonly property bool isMaster: detail.kind === "master"
    readonly property bool onServer: { const c = Backend.connection(page ? page.cid : ""); return !!c && !c.local }
    readonly property string base: "/api/termbases/" + Api.seg(name)

    function start() {
        name = page.name
        rules = {}
        people = []
        masters = []
        usedBy = []
        error = ""
        nameField.text = name
        open()
        const cid = page.cid
        if (detail.kind === "master") {
            Api.get(cid, base + "/review-rules", r => { if (r.ok) rules = r.data })
            // Whom to trust is chosen among the people of a team server.
            if (onServer) Api.get(cid, "/api/people", r => { people = r.ok ? r.data : [] })
        } else {
            Api.get(cid, "/api/termbases", r => { masters = (r.ok ? r.data : []).filter(t => t.kind === "master").map(t => t.name) })
        }
        // Which projects use it decides whether it can be deleted.
        Api.get(cid, "/api/projects", r => {
            const out = []
            for (const p of r.ok ? r.data : []) {
                const s = p.settings || {}, t = s.terminology || {}
                const same = x => (x || "").toLowerCase() === name.toLowerCase()
                if (same(t.writeTo) || same(t.termbase) || (t.masters || []).some(same)) out.push(s.name)
            }
            usedBy = out
        })
    }
    function termsIn(lang) { return entries.reduce((n, e) => n + TermData.terms(e, lang).length, 0) }
    function valuesOf(field) {
        let n = 0
        const has = h => TermData.value((h || {}).fields, field) ? 1 : 0
        for (const e of entries) {
            n += has(e)
            for (const l of e.languages || []) { n += has(l); for (const t of l.terms || []) n += has(t) }
        }
        return n
    }
    function rename() {
        const v = nameField.text.trim()
        if (!v || v === name) { nameField.text = name; return }
        const old = name
        Api.put(page.cid, "/api/termbases/" + Api.seg(old) + "/name", { name: v }, r => {
            if (!r.ok) { nameField.text = old; error = r.error.message; return }
            error = ""
            name = v
            page.notice(Fmt.fill(qsTr("Renamed to “{name}”"), { name: v }))
            page.open(page.cid, v)
            page.reload()
        })
    }
    function refresh() { page.reload() }
    function putDefinition(d, done) {
        Api.put(page.cid, base + "/definition", d, r => {
            if (!r.ok) { error = r.error.message; return }
            error = ""
            if (done) page.notice(done)
            page.reload()
        })
    }
    function putRules(next, done) {
        Api.put(page.cid, base + "/review-rules", next, r => {
            if (!r.ok) { error = r.error.message; return }
            error = ""
            rules = r.data
            page.notice(done)
        })
    }
    function setField(i, now, done) {
        const d = TermData.copy(def)
        d.fields[i] = now
        putDefinition(d, done)
    }

    Field { label: qsTr("Name"); TextField { id: nameField; onEditingFinished: dlg.rename() } }

    // Languages.
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
    Txt { role: "h3"; text: qsTr("Languages") }
    Txt { visible: dlg.langs.length === 0; Layout.fillWidth: true; role: "help"; text: qsTr("No languages yet. They are added when you add terms or import a file.") }
    ColumnLayout {
        Layout.fillWidth: true
        spacing: 0
        Repeater {
            model: dlg.langs
            ListRow {
                id: lrow
                required property string modelData
                readonly property int n: dlg.termsIn(modelData)
                Txt { text: Backend.languageName(lrow.modelData); role: "strong"; wrapMode: Text.NoWrap }
                LangChip { code: lrow.modelData }
                Txt { Layout.fillWidth: true; role: "muted"; text: TermData.termCount(lrow.n) }
                actions: ConfirmButton {
                    text: ""
                    iconName: "trash"
                    action: qsTr("Remove")
                    accessibleName: Fmt.fill(qsTr("Remove {language}"), { language: Backend.languageName(lrow.modelData) })
                    question: lrow.n ? Fmt.fill(qsTr("Remove {language} and its {terms}?"), { language: Backend.languageName(lrow.modelData), terms: TermData.termCount(lrow.n) })
                                     : Fmt.fill(qsTr("Remove {language}?"), { language: Backend.languageName(lrow.modelData) })
                    onConfirmed: Api.del(dlg.page.cid, dlg.base + "/languages/" + Api.seg(lrow.modelData), r => {
                        if (!r.ok) { dlg.error = r.error.message; return }
                        dlg.page.notice(lrow.n ? Fmt.fill(qsTr("Removed {language} and {terms}"), { language: Backend.languageName(lrow.modelData), terms: TermData.termCount(lrow.n) })
                                               : Fmt.fill(qsTr("Removed {language}"), { language: Backend.languageName(lrow.modelData) }))
                        dlg.refresh()
                    })
                }
            }
        }
    }
    RowLayout {
        spacing: Theme.s2
        ComboBox {
            id: addLang
            implicitWidth: 300
            readonly property var choices: Backend.commonLanguages().filter(l => dlg.langs.indexOf(l.code) < 0)
            model: [qsTr("Add a language…")].concat(choices.map(l => Backend.languageName(l.code)))
            Accessible.name: qsTr("Language to add")
        }
        Button {
            small: true
            text: qsTr("Add")
            enabled: addLang.currentIndex > 0
            onClicked: {
                const l = addLang.choices[addLang.currentIndex - 1].code
                const d = TermData.copy(dlg.def)
                d.languages = (d.languages || []).concat([l])
                dlg.putDefinition(d, Fmt.fill(qsTr("Added {language}"), { language: Backend.languageName(l) }))
                addLang.currentIndex = 0
            }
        }
    }

    // Fields.
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
    Txt { role: "h3"; text: qsTr("Fields") }
    Txt { visible: !(dlg.def.fields || []).length; Layout.fillWidth: true; role: "help"; text: qsTr("No fields yet: entries hold their terms and each term's status.") }
    Table {
        visible: (dlg.def.fields || []).length > 0
        columns: [{ title: qsTr("Name"), fill: 1 }, { title: qsTr("Applies to"), width: 110 }, { title: qsTr("Values"), fill: 1.2 }, { title: qsTr("Used"), width: 64, align: "right" }, { title: "", width: 48 }]
        Repeater {
            model: dlg.def.fields || []
            TableRow {
                id: frow
                required property var modelData
                required property int index
                readonly property int used: dlg.valuesOf(modelData.name)
                last: index === (dlg.def.fields || []).length - 1
                TextField {
                    text: frow.modelData.name
                    Accessible.name: qsTr("Field name")
                    onEditingFinished: {
                        const v = text.trim()
                        if (!v || v === frow.modelData.name) { text = frow.modelData.name; return }
                        Api.put(dlg.page.cid, dlg.base + "/fields/" + Api.seg(frow.modelData.name), { name: v }, r => {
                            if (!r.ok) { text = frow.modelData.name; dlg.error = r.error.message; return }
                            dlg.page.notice(Fmt.fill(qsTr("Renamed the field “{from}” to “{to}”"), { from: frow.modelData.name, to: v }))
                            dlg.refresh()
                        })
                    }
                }
                Txt { topPadding: 7; text: Fields.levelName(frow.modelData.level || "entry") }
                ColumnLayout {
                    spacing: Theme.s1
                    ComboBox {
                        Layout.fillWidth: true
                        model: Fields.kinds().map(k => k.name)
                        currentIndex: Math.max(0, Fields.kinds().findIndex(k => k.id === (frow.modelData.kind || "text")))
                        Accessible.name: Fmt.fill(qsTr("Values of {field}"), { field: frow.modelData.name })
                        onActivated: i => {
                            const now = Fields.kinds()[i].id
                            if (now === (frow.modelData.kind || "text")) return
                            const changed = TermData.copy(frow.modelData)
                            changed.kind = now
                            if (now !== "picklist") delete changed.values
                            else if (!changed.values) changed.values = []
                            dlg.setField(frow.index, changed, Fmt.fill(qsTr("{field} now takes {kind}"), { field: frow.modelData.name, kind: Fields.kindName(now).toLowerCase() }))
                        }
                    }
                    TextArea {
                        Layout.fillWidth: true
                        visible: frow.modelData.kind === "picklist"
                        minimumLines: 3
                        text: (frow.modelData.values || []).join("\n")
                        Accessible.name: Fmt.fill(qsTr("The values of {field}, one a line"), { field: frow.modelData.name })
                        area.onActiveFocusChanged: if (!area.activeFocus) {
                            const vals = Fields.tidy(text)
                            if (JSON.stringify(vals) === JSON.stringify(frow.modelData.values || [])) return
                            const changed = TermData.copy(frow.modelData)
                            changed.values = vals
                            dlg.setField(frow.index, changed, Fmt.fill(qsTr("Saved the values of {field}"), { field: frow.modelData.name }))
                        }
                    }
                }
                Txt { topPadding: 7; horizontalAlignment: Text.AlignRight; text: Fmt.num(frow.used) }
                ConfirmButton {
                    text: ""
                    iconName: "trash"
                    action: qsTr("Remove")
                    accessibleName: Fmt.fill(qsTr("Remove {field}"), { field: frow.modelData.name })
                    question: frow.used ? Fmt.fill(qsTr("Remove “{field}” and {values}?"), { field: frow.modelData.name, values: Fmt.plural(frow.used, qsTr("{n} value"), qsTr("{n} values")) })
                                        : Fmt.fill(qsTr("Remove “{field}”?"), { field: frow.modelData.name })
                    onConfirmed: Api.del(dlg.page.cid, dlg.base + "/fields/" + Api.seg(frow.modelData.name), r => {
                        if (!r.ok) { dlg.error = r.error.message; return }
                        dlg.page.notice(Fmt.fill(qsTr("Removed the field “{field}”"), { field: frow.modelData.name }))
                        dlg.refresh()
                    })
                }
            }
        }
    }
    AddFieldForm {
        termbase: dlg.name
        taken: (dlg.def.fields || []).map(f => f.name)
        onAdded: field => {
            const d = TermData.copy(dlg.def)
            d.fields = (d.fields || []).concat([field])
            dlg.putDefinition(d, Fmt.fill(qsTr("Added field “{field}” to {termbase}"), { field: field.name, termbase: dlg.name }))
        }
    }

    // Review, for a master.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.isMaster
        spacing: Theme.s3
        Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
        Txt { role: "h3"; text: qsTr("Review") }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Projects submit their new and changed entries to this master, and its reviewers approve them. These rules say what is merged without waiting for them.") }
        CheckBox {
            text: qsTr("Entries the master already has as they are need no review")
            checked: !dlg.rules.keepIdentical
            onToggled: {
                const next = Object.assign({}, dlg.rules)
                if (checked) delete next.keepIdentical
                else next.keepIdentical = true
                dlg.putRules(next, checked ? qsTr("Entries the master already has are merged without review") : qsTr("Entries the master already has wait for review, too"))
            }
        }
        Txt { role: "label"; text: qsTr("Fields that need no approval") }
        Txt { visible: !(dlg.def.fields || []).length; Layout.fillWidth: true; role: "help"; text: qsTr("The termbase has no fields yet. A change to notes or sources only could be merged at once.") }
        Flow {
            Layout.fillWidth: true
            spacing: Theme.s3
            Accessible.name: qsTr("Fields that need no approval")
            Repeater {
                model: dlg.def.fields || []
                CheckBox {
                    required property var modelData
                    readonly property var minor: dlg.rules.minorFields || []
                    text: modelData.name
                    checked: minor.indexOf(modelData.name) >= 0
                    onToggled: {
                        const now = minor.filter(n => n !== modelData.name).concat(checked ? [modelData.name] : [])
                        const next = Object.assign({}, dlg.rules)
                        if (now.length) next.minorFields = now
                        else delete next.minorFields
                        dlg.putRules(next, checked ? Fmt.fill(qsTr("Changes to {field} only are merged without review"), { field: modelData.name })
                                                   : Fmt.fill(qsTr("Changes to {field} wait for review"), { field: modelData.name }))
                    }
                }
            }
        }
        Txt { visible: (dlg.def.fields || []).length > 0; Layout.fillWidth: true; role: "help"; text: qsTr("An entry changed in these fields only is merged at once.") }
        Txt { role: "label"; text: qsTr("Trusted authors") }
        Txt { visible: !(dlg.rules.trusted || []).length; role: "help"; text: qsTr("Nobody yet: every entry waits for a reviewer.") }
        ColumnLayout {
            Layout.fillWidth: true
            spacing: 0
            Repeater {
                model: dlg.rules.trusted || []
                ListRow {
                    id: trow
                    required property var modelData
                    readonly property var tl: (modelData.scope || {}).langs || []
                    readonly property string who: { const p = dlg.people.find(x => x.username === modelData.author); return p ? p.name + " (" + p.username + ")" : modelData.author }
                    Txt { text: trow.who; role: "strong"; wrapMode: Text.NoWrap }
                    Txt { Layout.fillWidth: true; role: "muted"; text: trow.tl.length ? Fmt.fill(qsTr("entries with terms in {languages}"), { languages: trow.tl.join(", ") }) : qsTr("all entries") }
                    actions: Button {
                        small: true; kind: "quiet"
                        text: qsTr("Remove")
                        Accessible.name: Fmt.fill(qsTr("No longer trust {author}"), { author: trow.who })
                        onClicked: {
                            const now = (dlg.rules.trusted || []).filter(t => t.author !== trow.modelData.author)
                            const next = Object.assign({}, dlg.rules)
                            if (now.length) next.trusted = now
                            else delete next.trusted
                            dlg.putRules(next, Fmt.fill(qsTr("The entries of {author} wait for review again"), { author: trow.modelData.author }))
                        }
                    }
                }
            }
        }
        RowLayout {
            spacing: Theme.s3
            Accessible.name: qsTr("Trust an author")
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 240
                label: qsTr("Author")
                Item {
                    implicitHeight: 32
                    ComboBox {
                        id: whoBox
                        anchors.fill: parent
                        visible: dlg.people.length > 0
                        model: [qsTr("Choose a person…")].concat(dlg.people.map(p => p.name + " (" + p.username + ")"))
                    }
                    TextField { id: whoText; anchors.fill: parent; visible: dlg.people.length === 0; placeholderText: qsTr("Name, as in the history") }
                }
            }
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 220
                label: qsTr("For")
                ComboBox { id: trustLang; model: [qsTr("All entries")].concat(dlg.langs.map(l => Fmt.fill(qsTr("Terms in {language}"), { language: Backend.languageName(l) }))) }
            }
            Button {
                Layout.alignment: Qt.AlignBottom
                small: true
                text: qsTr("Trust")
                onClicked: {
                    const author = dlg.people.length ? (whoBox.currentIndex > 0 ? dlg.people[whoBox.currentIndex - 1].username : "") : whoText.text.trim()
                    if (!author) { dlg.error = qsTr("Say whose entries to trust."); return }
                    const rule = { author: author, scope: trustLang.currentIndex > 0 ? { langs: [dlg.langs[trustLang.currentIndex - 1]] } : {} }
                    // One rule an author: a new one replaces the old.
                    const now = (dlg.rules.trusted || []).filter(t => t.author.toLowerCase() !== author.toLowerCase()).concat([rule])
                    dlg.putRules(Object.assign({}, dlg.rules, { trusted: now }), Fmt.fill(qsTr("The entries of {author} are approved without review"), { author: author }))
                    whoBox.currentIndex = 0
                    whoText.text = ""
                    trustLang.currentIndex = 0
                }
            }
        }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Their new and changed entries are approved as they are submitted. An entry that conflicts with the master always waits for a reviewer.") }
    }

    // Based on, for a project termbase.
    ColumnLayout {
        Layout.fillWidth: true
        visible: !dlg.isMaster
        spacing: Theme.s3
        Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
        Txt { role: "h3"; text: Words.get("basedOn") }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("The termbase shows the entries of these masters under its own, and its new and changed entries are submitted to them.") }
        Repeater {
            model: dlg.masters
            CheckBox {
                required property string modelData
                readonly property var ups: dlg.detail.upstreams || []
                text: modelData
                checked: ups.indexOf(modelData) >= 0
                // It needs one master at least.
                enabled: !(checked && ups.length === 1)
                onToggled: {
                    const next = checked ? ups.concat([modelData]) : ups.filter(m => m !== modelData)
                    Api.put(dlg.page.cid, dlg.base + "/upstreams", { upstreams: next }, r => {
                        if (!r.ok) { dlg.error = r.error.message; return }
                        dlg.page.notice(checked ? Fmt.fill(qsTr("Now also based on {master}"), { master: modelData }) : Fmt.fill(qsTr("No longer based on {master}"), { master: modelData }))
                        dlg.page.reload()
                    })
                }
            }
        }
    }

    // Who may use it, on a team server.
    Loader {
        Layout.fillWidth: true
        active: dlg.onServer && dlg.opened
        visible: active
        sourceComponent: AccessPanel {
            title: qsTr("Who may use it")
            cid: dlg.page.cid
            kind: "termbase"
            resource: dlg.name
        }
    }

    // Deleting it.
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
    Txt { role: "h3"; color: Theme.danger; text: qsTr("Delete termbase") }
    readonly property var basedOnThis: (detail.projects || [])
    Txt {
        Layout.fillWidth: true
        visible: dlg.basedOnThis.length > 0
        role: "help"
        text: Fmt.fill(dlg.basedOnThis.length === 1 ? qsTr("{names} is based on this termbase. Delete it, or base it on another master, first.")
                                                    : qsTr("{names} are based on this termbase. Delete them, or base them on another master, first."), { names: Fmt.listOf(dlg.basedOnThis) })
    }
    Txt {
        Layout.fillWidth: true
        visible: dlg.basedOnThis.length === 0 && dlg.usedBy.length > 0
        role: "help"
        text: Fmt.fill(dlg.usedBy.length === 1 ? qsTr("{names} uses this termbase. Remove it from the project first.") : qsTr("{names} use this termbase. Remove it from the projects first."),
                       { names: Fmt.listOf(dlg.usedBy) })
    }
    Txt {
        Layout.fillWidth: true
        visible: dlg.basedOnThis.length === 0 && dlg.usedBy.length === 0
        role: "help"
        text: dlg.isMaster ? Fmt.fill(qsTr("Deletes {entries} and their history. Export the termbase first if you may need it again."), { entries: TermData.entries(dlg.entries.length) })
                           : Fmt.fill(qsTr("Deletes the project's {entries} and their history. The masters keep theirs. Merge what the masters should have first."), { entries: TermData.entries(dlg.entries.length) })
    }
    ConfirmButton {
        visible: dlg.basedOnThis.length === 0 && dlg.usedBy.length === 0
        small: false
        text: qsTr("Delete termbase")
        question: Fmt.fill(qsTr("Delete {name} for good?"), { name: dlg.name })
        action: qsTr("Delete")
        onConfirmed: Api.del(dlg.page.cid, dlg.base, r => {
            if (!r.ok) { dlg.error = r.error.message; return }
            dlg.page.notice(Fmt.fill(qsTr("Deleted {name}"), { name: dlg.name }))
            dlg.page.name = ""
            dlg.page.reload()
            dlg.close()
        })
    }

    buttons: Button { kind: "primary"; small: true; text: qsTr("Done"); onClicked: { dlg.rename(); dlg.close() } }
}
