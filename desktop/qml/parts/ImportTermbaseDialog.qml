import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// Importing a termbase file (TermbaseDialogs.tsx, ImportDialog): the file
// and where it goes; for a table what each column holds; where each of the
// file's languages goes and the status of terms without one; and what the
// import does, with how existing entries are found and treated.
Dialog {
    id: dlg
    wide: true
    property var lists: ({})
    signal imported(string connectionId, string name, string message)

    property string place: ""
    property string termbase: ""
    property bool intoNew: true
    property string file: ""
    property string fileName: ""
    property var inspection: ({})
    property var read_: ({})
    property var mapping: ({})
    property var langMap: ({})
    property var fileLangs: []
    property string fallback: "proposed"
    property string match: "term"
    property string matchLang: ""
    property string whenMatched: "merge"
    property var plan: null
    property var destination: ({})
    property int destinationEntries: 0
    property int step: 0
    property bool busy: false
    readonly property bool tabular: !!inspection.tabular
    readonly property var steps: [qsTr("File")].concat(tabular ? [qsTr("Columns")] : []).concat([qsTr("Languages"), qsTr("Review")])
    readonly property string stepName: tabular ? ["file", "columns", "languages", "review"][step] : ["file", "languages", "review"][step]
    readonly property var places: Backend.connections.filter(c => c.ready)
    title: termbase && !intoNew ? Fmt.fill(qsTr("Import into {name}"), { name: termbase }) : qsTr("Import a termbase")

    function start(connectionId, name) {
        place = connectionId || Backend.local.id
        termbase = name || ""
        intoNew = !name
        file = ""; fileName = ""; inspection = {}; read_ = {}; mapping = {}; langMap = {}; fileLangs = []
        fallback = "proposed"; match = "term"; matchLang = ""; whenMatched = "merge"; plan = null; step = 0; error = ""
        newName.text = ""
        destination = {}
        destinationEntries = 0
        if (termbase)
            Api.get(place, "/api/termbases/" + Api.seg(termbase), r => {
                if (!r.ok) return
                destination = r.data
                destinationEntries = r.data.entries || 0
            })
        open()
    }
    function options(withLanguages) {
        const q = { filename: fileName }
        if (tabular && Object.keys(mapping).length) q.mapping = mapping
        if (withLanguages && Object.keys(langMap).length) q.languages = langMap
        return q
    }
    function destinationLanguages() {
        if (intoNew) return []
        return ((destination.definition || {}).languages) || []
    }
    // Where each of the file's languages goes: the termbase's language that
    // is the same, else one of the same language, else the listed language.
    function suggest(fl, dest) {
        const same = dest.find(l => l.toLowerCase() === fl.toLowerCase())
        if (same) return same
        const common = Backend.commonLanguages().map(l => l.code)
        const primary = t => t.split("-")[0].toLowerCase()
        if (fl.indexOf("-") < 0) {
            return dest.find(l => primary(l) === primary(fl)) || common.find(l => primary(l) === primary(fl)) || fl
        }
        return common.find(l => l.toLowerCase() === fl.toLowerCase()) || fl
    }
    function inspect(withMapping, then) {
        error = ""
        busy = true
        Backend.upload(place, "POST", "/api/termfiles/inspect", file, "application/octet-stream", withMapping ? options(false) : { filename: fileName }).done.connect(r => {
            busy = false
            if (!r.ok) { error = Fmt.fill(qsTr("{file} could not be read: {error}"), { file: fileName, error: r.error.message }); return }
            then(r.data)
        })
    }
    function choose(url) {
        file = Backend.localPath(url)
        fileName = Backend.fileName(file)
        inspect(false, o => {
            inspection = o
            mapping = o.mapping || {}
            read_ = o
            if (!o.tabular && !((o.read || {}).entries)) {
                error = Fmt.fill(qsTr("{file} could not be read: the file has no entries with terms"), { file: fileName })
                return
            }
            if (!newName.text.trim() && o.name) newName.text = o.name
        })
    }
    function readLanguages() {
        if (!Object.keys(langMap).length) {
            const dest = destinationLanguages()
            const m = {}
            fileLangs = read_.languages || []
            for (const l of fileLangs) m[l] = suggest(l, dest)
            langMap = m
        }
        busy = true
        Backend.upload(place, "POST", "/api/termfiles/inspect", file, "application/octet-stream", options(true)).done.connect(r => {
            busy = false
            if (!r.ok) { error = Fmt.fill(qsTr("{file} could not be read: {error}"), { file: fileName, error: r.error.message }); return }
            read_ = r.data
        })
    }
    function loadPlan() {
        const rep = read_.read || {}
        const incoming = (rep.entries || 0) - (read_.dropped || 0)
        if (intoNew) {
            plan = { added: incoming, actions: incoming, plan: (read_.sample || []).map(s => ({ kind: "add", entry: s })) }
            return
        }
        const none = destinationEntries === 0
        const q = Object.assign(options(true), { dryRun: "1", status: fallback, match: none ? "none" : match, whenMatched: none ? "add" : whenMatched })
        if (!none && match === "term" && matchLang) q.lang = matchLang
        plan = null
        Backend.upload(place, "POST", "/api/termbases/" + Api.seg(termbase) + "/import", file, "application/octet-stream", q).done.connect(r => {
            plan = r.ok ? r.data : {}
            if (!r.ok) error = Fmt.fill(qsTr("The import could not be planned: {error}"), { error: r.error.message })
        })
    }
    function doImport() {
        error = ""
        busy = true
        const cid = place
        const send = (name, created) => {
            const none = intoNew || destinationEntries === 0
            const q = Object.assign(options(true), { status: fallback, match: none ? "none" : match, whenMatched: none ? "add" : whenMatched })
            if (!none && match === "term" && matchLang) q.lang = matchLang
            const route = "/api/termbases/" + Api.seg(name)
            Backend.upload(cid, "POST", route + "/import", file, "application/octet-stream", q).done.connect(r => {
                busy = false
                if (!r.ok) {
                    // A termbase made for an import that failed is taken back.
                    if (created) Api.del(cid, route)
                    error = Fmt.fill(qsTr("The import failed: {error}"), { error: r.error.message })
                    return
                }
                const o = r.data, parts = []
                if (o.added) parts.push(Fmt.fill(qsTr("{n} added"), { n: o.added }))
                if (o.merged) parts.push(Fmt.fill(qsTr("{n} merged"), { n: o.merged }))
                if (o.overwritten) parts.push(Fmt.fill(qsTr("{n} replaced"), { n: o.overwritten }))
                if ((o.skipped || 0) + (o.unchanged || 0)) parts.push(Fmt.fill(qsTr("{n} left as they were"), { n: (o.skipped || 0) + (o.unchanged || 0) }))
                dlg.imported(cid, name, Fmt.fill(qsTr("Imported {file} into {name}: {what}"), { file: fileName, name: name, what: parts.length ? parts.join(", ") : qsTr("nothing changed") }))
                dlg.close()
            })
        }
        if (!intoNew) { send(termbase, false); return }
        const name = newName.text.trim() || (read_.name || "").trim() || qsTr("Imported termbase")
        Api.post(cid, "/api/termbases", { definition: { name: name } }, r => {
            if (!r.ok) { busy = false; error = r.error.message; return }
            send(r.data.name || name, true)
        })
    }
    function next() {
        if (stepName === "file") {
            step++
            if (stepName === "languages") { langMap = {}; readLanguages() }
        } else if (stepName === "columns") {
            inspect(true, o => {
                if (!((o.read || {}).entries)) { error = Fmt.fill(qsTr("{file} could not be read: the file has no entries with terms"), { file: fileName }); return }
                read_ = o
                langMap = {}
                step++
                readLanguages()
            })
        } else if (stepName === "languages") {
            step++
            loadPlan()
        } else {
            doImport()
        }
    }

    Stepper { steps: dlg.steps; current: dlg.step; clickable: false }

    // The file and where it goes.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "file"
        spacing: Theme.s3
        Txt { visible: !!dlg.termbase; role: "label"; text: qsTr("Import into") }
        RadioButton { visible: !!dlg.termbase; text: dlg.termbase; checked: !dlg.intoNew; onClicked: dlg.intoNew = false }
        RadioButton { visible: !!dlg.termbase; text: qsTr("A new master termbase"); checked: dlg.intoNew; onClicked: dlg.intoNew = true }
        RowLayout {
            visible: dlg.intoNew
            spacing: Theme.s4
            Field { label: qsTr("Name"); help: qsTr("from the file if empty"); TextField { id: newName } }
            ColumnLayout {
                visible: dlg.places.length > 1
                spacing: Theme.s2
                Txt { role: "label"; text: qsTr("Store on") }
                Segmented {
                    name: qsTr("Store on")
                    model: dlg.places.map(c => c.local ? qsTr("This computer") : dlg.places.length > 2 ? c.displayName : qsTr("Server"))
                    current: Math.max(0, dlg.places.findIndex(c => c.id === dlg.place))
                    onChanged: i => dlg.place = dlg.places[i].id
                }
            }
        }
        DropZone {
            onDropped: urls => dlg.choose(urls[0])
            Icon { name: "file-up"; color: Theme.accent; size: 22 }
            ColumnLayout {
                Layout.fillWidth: true
                spacing: 2
                Txt { Layout.fillWidth: true; text: dlg.fileName || qsTr("Choose a termbase file"); role: "strong" }
                Txt { Layout.fillWidth: true; role: "muted"; text: qsTr("TBX (2 or 3), an .sdltb termbase or its XML export, an XLSX workbook, CSV or tab-separated text. A termbase definition (.xdt) brings only the languages.") }
            }
            Button { small: true; iconName: "file-up"; text: dlg.busy ? qsTr("Reading…") : qsTr("Choose a termbase file"); onClicked: fileDialog.open() }
        }
        Txt {
            Layout.fillWidth: true
            visible: !!dlg.inspection.format && !dlg.tabular
            role: "muted"
            text: dlg.inspection.format ? Fmt.fill(qsTr("{format}: {entries}, {terms}."), { format: Fields.formatLabel(dlg.inspection.format),
                  entries: TermData.entries((dlg.inspection.read || {}).entries || 0), terms: TermData.termCount((dlg.inspection.read || {}).terms || 0) }) : ""
        }
    }
    FileDialog {
        id: fileDialog
        title: qsTr("Choose a termbase file")
        nameFilters: [qsTr("Termbase files (*.tbx *.tbxm *.xml *.mtf *.sdltb *.xdt *.csv *.tsv *.tab *.txt *.xlsx)"), qsTr("All files (*)")]
        onAccepted: dlg.choose(selectedFile)
    }

    // The columns of a table.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "columns"
        spacing: Theme.s3
        Txt {
            Layout.fillWidth: true
            role: "muted"
            text: Fmt.fill(qsTr("{name}: {plural} with entries. Each row is one entry. Check what each column holds; the guess comes from the column names."),
                           { name: dlg.fileName, plural: Fmt.plural((dlg.inspection.rows || 0), qsTr("{n} row"), qsTr("{n} rows")) })
        }
        RowLayout {
            spacing: Theme.s4
            CheckBox {
                text: qsTr("The first row holds column names")
                checked: dlg.mapping.header !== false
                onToggled: dlg.mapping = Object.assign({}, dlg.mapping, { header: checked })
            }
            Txt { text: qsTr("Synonyms in one cell separated by"); role: "muted"; wrapMode: Text.NoWrap }
            TextField {
                implicitWidth: 60
                text: dlg.mapping.separator || ""
                Accessible.name: qsTr("Synonyms in one cell separated by")
                onTextEdited: dlg.mapping = Object.assign({}, dlg.mapping, { separator: text })
            }
        }
        Table {
            columns: [{ title: qsTr("Column"), fill: 1 }, { title: qsTr("Holds"), width: 150 }, { title: "", fill: 1.4 }]
            Repeater {
                model: dlg.mapping.columns || []
                TableRow {
                    id: colRow
                    required property var modelData
                    required property int index
                    readonly property var col: modelData
                    readonly property string title: col.header || Fmt.fill(qsTr("Column {n}"), { n: index + 1 })
                    last: index === (dlg.mapping.columns || []).length - 1
                    function set(changes) {
                        const cols = TermData.copy(dlg.mapping.columns)
                        cols[index] = Object.assign(cols[index], changes)
                        dlg.mapping = Object.assign({}, dlg.mapping, { columns: cols })
                    }
                    CellTitle { title: colRow.title; sub: (colRow.col.sample || []).slice(0, 3).join(", ") }
                    ComboBox {
                        readonly property var roles: ["term", "status", "field", "id", "ignore"]
                        model: [qsTr("Terms"), qsTr("Status"), qsTr("Field"), qsTr("Entry ID"), qsTr("Leave out")]
                        currentIndex: Math.max(0, roles.indexOf(colRow.col.role))
                        Accessible.name: Fmt.fill(qsTr("Column {n} holds"), { n: colRow.index + 1 })
                        onActivated: i => {
                            const r = roles[i], ch = { role: r }
                            if (r === "field" && !colRow.col.field) { ch.field = colRow.title; ch.level = colRow.col.lang ? "language" : "entry" }
                            colRow.set(ch)
                        }
                    }
                    RowLayout {
                        spacing: Theme.s2
                        LanguageField {
                            visible: colRow.col.role === "term" || colRow.col.role === "status" || (colRow.col.role === "field" && colRow.col.level !== "entry")
                            Layout.fillWidth: true
                            language: colRow.col.lang || ""
                            placeholder: qsTr("Language…")
                            Accessible.name: Fmt.fill(qsTr("Language of column {n}"), { n: colRow.index + 1 })
                            onLanguageChanged: if (language !== (colRow.col.lang || "")) colRow.set({ lang: language })
                        }
                        TextField {
                            visible: colRow.col.role === "field"
                            Layout.fillWidth: true
                            text: colRow.col.field || ""
                            placeholderText: qsTr("Field name")
                            Accessible.name: Fmt.fill(qsTr("Field name of column {n}"), { n: colRow.index + 1 })
                            onEditingFinished: colRow.set({ field: text })
                        }
                        ComboBox {
                            visible: colRow.col.role === "field"
                            implicitWidth: 140
                            readonly property var levels: ["entry", "language", "term"]
                            model: [qsTr("Whole entry"), qsTr("One language"), qsTr("The terms")]
                            currentIndex: Math.max(0, levels.indexOf(colRow.col.level || "entry"))
                            Accessible.name: Fmt.fill(qsTr("Column {n} applies to"), { n: colRow.index + 1 })
                            onActivated: i => colRow.set({ level: levels[i] })
                        }
                    }
                }
            }
        }
        Txt {
            visible: !(dlg.mapping.columns || []).some(c => c.role === "term" && c.lang)
            role: "error"
            text: qsTr("Mark at least one column as terms and give it a language.")
        }
    }

    // Languages.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "languages"
        spacing: Theme.s3
        readonly property var rep: dlg.read_.read || {}
        Txt {
            Layout.fillWidth: true
            role: "muted"
            text: Fmt.fill(qsTr("{format}: {entries}, {terms}."), { format: Fields.formatLabel(dlg.inspection.format || ""), entries: TermData.entries(parent.rep.entries || 0), terms: TermData.termCount(parent.rep.terms || 0) })
                  + ((parent.rep.skipped || 0) > 0 ? " " + Fmt.fill(qsTr("{n} rows without terms skipped."), { n: parent.rep.skipped }) : "")
        }
        Table {
            name: qsTr("Languages of the file")
            columns: [{ title: qsTr("In the file"), width: 160 }, { title: qsTr("In the termbase"), fill: 1 }]
            Repeater {
                model: dlg.fileLangs
                TableRow {
                    id: lrow
                    required property string modelData
                    required property int index
                    last: index === dlg.fileLangs.length - 1
                    Txt { text: lrow.modelData; font.family: "monospace" }
                    ComboBox {
                        readonly property var dest: dlg.destinationLanguages()
                        readonly property var codes: [""].concat(dest).concat(Backend.commonLanguages().map(l => l.code).filter(c => dest.indexOf(c) < 0))
                                                     .concat(dlg.langMap[lrow.modelData] && codes0.indexOf(dlg.langMap[lrow.modelData]) < 0 ? [dlg.langMap[lrow.modelData]] : [])
                        readonly property var codes0: [""].concat(dest).concat(Backend.commonLanguages().map(l => l.code))
                        model: codes.map(c => !c ? qsTr("Leave out") : !dlg.intoNew && dest.indexOf(c) < 0 ? Fmt.fill(qsTr("{language} (new)"), { language: Backend.languageName(c) }) : Backend.languageName(c))
                        currentIndex: Math.max(0, codes.indexOf(dlg.langMap[lrow.modelData] || ""))
                        Accessible.name: Fmt.fill(qsTr("Termbase language for {language}"), { language: lrow.modelData })
                        onActivated: i => { dlg.langMap = Object.assign({}, dlg.langMap, { [lrow.modelData]: codes[i] }); dlg.readLanguages() }
                    }
                }
            }
        }
        Note {
            visible: (dlg.read_.dropped || 0) > 0
            kind: "warn"
            text: Fmt.fill(dlg.read_.dropped === 1 ? qsTr("{n} entry has no terms in the languages you import and will be left out.") : qsTr("{n} entries have no terms in the languages you import and will be left out."), { n: dlg.read_.dropped || 0 })
        }
        Txt { role: "label"; text: qsTr("Terms without a status") }
        RadioButton { text: qsTr("Approved"); checked: dlg.fallback === "approved"; onClicked: dlg.fallback = "approved" }
        RadioButton { text: qsTr("Proposed, for lists nobody has checked"); checked: dlg.fallback !== "approved"; onClicked: dlg.fallback = "proposed" }
    }

    // Review.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "review"
        spacing: Theme.s3
        RowLayout {
            visible: !dlg.intoNew && dlg.destinationEntries > 0
            spacing: Theme.s5
            ColumnLayout {
                Layout.alignment: Qt.AlignTop
                spacing: Theme.s2
                Txt { role: "label"; text: qsTr("Find existing entries by") }
                Repeater {
                    model: [{ id: "term", t: qsTr("A shared term") }, { id: "id", t: qsTr("The entry ID (for a file exported from this termbase)") }, { id: "none", t: qsTr("Nothing: add every entry") }]
                    RadioButton { required property var modelData; text: modelData.t; checked: dlg.match === modelData.id; onClicked: { dlg.match = modelData.id; dlg.loadPlan() } }
                }
                ComboBox {
                    visible: dlg.match === "term"
                    model: [qsTr("In any language")].concat(dlg.destinationLanguages().map(l => Fmt.fill(qsTr("In {language}"), { language: Backend.languageName(l) })))
                    Accessible.name: qsTr("Language of the shared term")
                    onActivated: i => { dlg.matchLang = i ? dlg.destinationLanguages()[i - 1] : ""; dlg.loadPlan() }
                }
            }
            ColumnLayout {
                Layout.alignment: Qt.AlignTop
                visible: dlg.match !== "none"
                spacing: Theme.s2
                Txt { role: "label"; text: qsTr("When an entry exists") }
                Repeater {
                    model: [{ id: "merge", t: qsTr("Merge: add the new terms and field values") }, { id: "overwrite", t: qsTr("Replace it with the entry from the file") },
                            { id: "skip", t: qsTr("Keep it and leave the file's entry out") }, { id: "add", t: qsTr("Add the file's entry as a separate entry") }]
                    RadioButton { required property var modelData; text: modelData.t; checked: dlg.whenMatched === modelData.id; onClicked: { dlg.whenMatched = modelData.id; dlg.loadPlan() } }
                }
            }
        }
        Txt { role: "h3"; visible: dlg.plan !== null; text: qsTr("What the import does") }
        Txt { visible: dlg.plan === null; role: "help"; text: qsTr("Working out what the import does…") }
        Row {
            visible: !!dlg.plan
            spacing: Theme.s3
            Repeater {
                model: dlg.plan ? [{ t: qsTr("New entries"), n: dlg.plan.added || 0 }, { t: qsTr("Merged"), n: dlg.plan.merged || 0 },
                                   { t: qsTr("Replaced"), n: dlg.plan.overwritten || 0 }, { t: qsTr("Unchanged"), n: (dlg.plan.unchanged || 0) + (dlg.plan.skipped || 0) }] : []
                Rectangle {
                    required property var modelData
                    width: 130; height: 64
                    radius: Theme.rControl
                    color: Theme.surface2
                    border.color: Theme.border
                    Column {
                        anchors.centerIn: parent
                        Txt { anchors.horizontalCenter: parent.horizontalCenter; text: Fmt.num(modelData.n); font.pixelSize: 20; font.weight: 650 }
                        Txt { anchors.horizontalCenter: parent.horizontalCenter; text: modelData.t; role: "sub"; wrapMode: Text.NoWrap }
                    }
                }
            }
        }
        Table {
            visible: !!dlg.plan && (dlg.plan.plan || []).length > 0
            name: qsTr("Entries of the file and what happens to them")
            columns: [{ title: qsTr("Entry in the file"), fill: 1 }, { title: qsTr("Result"), width: 110 }, { title: qsTr("Existing entry"), fill: 1 }]
            Repeater {
                model: dlg.plan ? (dlg.plan.plan || []).slice(0, 12) : []
                TableRow {
                    required property var modelData
                    required property int index
                    last: index === Math.min(12, (dlg.plan.plan || []).length) - 1
                    Txt { text: modelData.entry || "" }
                    Badge {
                        text: modelData.kind === "add" ? qsTr("New") : modelData.kind === "merge" ? qsTr("Merged") : modelData.kind === "overwrite" ? qsTr("Replaced") : qsTr("Unchanged")
                        kind: modelData.kind === "add" ? "ok" : modelData.kind === "merge" || modelData.kind === "overwrite" ? "warn" : ""
                    }
                    Txt { text: modelData.kind === "add" ? qsTr("none") : (modelData.existing || ""); role: modelData.kind === "add" ? "muted" : "" }
                }
            }
        }
        Txt {
            visible: !!dlg.plan && (dlg.plan.actions || 0) > 12
            role: "help"
            text: dlg.plan ? Fmt.plural((dlg.plan.actions || 0) - 12, qsTr("1 more entry"), qsTr("{n} more entries")) : ""
        }
    }

    buttons: [
        Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button { visible: dlg.step > 0; small: true; text: qsTr("Back"); onClicked: { dlg.error = ""; dlg.step--; } },
        Button {
            kind: "primary"; small: true
            text: dlg.stepName === "review" ? qsTr("Import") : qsTr("Continue")
            enabled: !dlg.busy && (dlg.stepName !== "file" || !!dlg.inspection.format)
                     && (dlg.stepName !== "columns" || (dlg.mapping.columns || []).some(c => c.role === "term" && c.lang))
                     && (dlg.stepName !== "languages" || ((dlg.read_.read || {}).entries || 0) - (dlg.read_.dropped || 0) > 0)
            onClicked: dlg.next()
        }
    ]
}
