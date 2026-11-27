import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// Exporting a termbase (web/src/screens/ExportTermbase.tsx): the format,
// from settings kept with the termbase or ready-made ones; what goes in
// (languages, entries, statuses, fields, a filter); for a glossary its
// layout; and keeping the settings for the next time.
Dialog {
    id: dlg
    wide: true
    property var page
    property var options: Fields.defaultOptions("pdf")
    property var definition: ({})
    property var kept: []
    property var ready: []
    property string preset: ""
    property bool shownOnly: false
    property bool exporting: false
    property int step: 0
    readonly property string format: options.format || "pdf"
    readonly property bool glossary: Fields.glossary(format)
    readonly property var steps: [qsTr("Format"), qsTr("Content")].concat(glossary ? [qsTr("Layout")] : [])
    readonly property var allLanguages: definition.languages || []
    readonly property var languages: options.languages || allLanguages
    readonly property var shown: page ? page.shown.map(e => e.id) : []
    readonly property int total: page ? page.entries.length : 0
    title: Fmt.fill(qsTr("Export {name}"), { name: page ? page.name : "" })

    onOpened: {
        options = Fields.defaultOptions("pdf")
        preset = ""
        shownOnly = false
        step = 0
        error = ""
        definition = page.detail.definition || {}
        kept = definition.exports || []
        // The settings kept with the termbase as they are now.
        Api.get(page.cid, page.path(), r => { if (r.ok) { definition = r.data.definition || {}; kept = definition.exports || [] } })
        Api.get(page.cid, "/api/export-presets", r => { if (r.ok) ready = r.data })
    }
    function set(key, value) {
        const o = Object.assign({}, options)
        if (value === undefined || value === null || value === "") delete o[key]
        else o[key] = value
        options = o
    }
    function setFormat(id) {
        preset = ""
        if (Fields.glossary(id) && !glossary) {
            const o = Fields.defaultOptions(id)
            for (const k of ["languages", "statuses", "fields", "allFields", "filterField", "filterValue", "changedSince"])
                if (k in options) o[k] = options[k]
            options = o
        } else {
            set("format", id)
        }
    }
    function choosePreset(name) {
        preset = name
        const p = kept.concat(ready).find(x => x.name === name)
        if (!p) return
        const o = TermData.copy(p.options || {})
        // Languages or a sort the termbase lacks are left to it.
        if (o.languages) {
            const keep = o.languages.filter(l => allLanguages.indexOf(l) >= 0)
            if (keep.length) o.languages = keep
            else delete o.languages
        }
        if (o.sortBy && allLanguages.indexOf(o.sortBy) < 0) delete o.sortBy
        options = o
    }
    function putExports(next, said) {
        const def = Object.assign({}, definition, { exports: next })
        Api.put(page.cid, page.path("/definition"), def, r => {
            if (!r.ok) { error = r.error.message; return }
            definition = def
            kept = next
            page.notice(said)
        })
    }
    function blocking() {
        if (!languages.length) return qsTr("Choose a language at least.")
        if (glossary && options.sortBy && languages.indexOf(options.sortBy) < 0) return qsTr("Sort by one of the languages exported.")
        if ((shownOnly ? shown.length : total) === 0) return qsTr("There is no entry to export.")
        return ""
    }
    function fileName() {
        const base = (page ? page.name : "").replace(/[\\/:*?"<>|]+/g, " ").trim()
        if (glossary) return base + "." + format
        const f = Fields.exportFormats().find(x => x.id === format)
        return base + (f ? f.ext : "")
    }
    function exportTo(url) {
        error = ""
        const o = Object.assign({}, options)
        if (shownOnly) o.entries = shown
        exporting = true
        const path = Backend.localPath(url)
        Backend.postToFile(page.cid, page.path("/export"), { options: o }, path).done.connect(r => {
            exporting = false
            if (!r.ok) { error = Fmt.fill(qsTr("The export failed: {error}"), { error: r.error.message }); return }
            if (!r.saved) { error = Fmt.fill(qsTr("{file} could not be saved: {error}"), { file: Backend.fileName(path), error: r.saveError }); return }
            const summary = (r.headers || {})["x-interlinea-summary"] || ""
            const missing = ((r.headers || {})["x-interlinea-missing"] || "").split(" ").filter(x => x).slice(0, 12)
                            .map(c => String.fromCodePoint(parseInt(c.slice(2), 16))).join(" ")
            page.notice(missing ? Fmt.fill(qsTr("Exported {what} to {file}. No font has {chars}: they show as boxes."), { what: summary, file: Backend.fileName(path), chars: missing })
                                : Fmt.fill(qsTr("Exported {what} to {file}"), { what: summary, file: Backend.fileName(path) }))
            dlg.close()
        })
    }
    FileDialog {
        id: saveDialog
        fileMode: FileDialog.SaveFile
        title: dlg.title
        onAccepted: dlg.exportTo(selectedFile)
    }

    Stepper { steps: dlg.steps; current: dlg.step; onStepClicked: i => dlg.step = i }

    // Format.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.step === 0
        spacing: Theme.s3
        Field {
            visible: dlg.kept.length + dlg.ready.length > 0
            label: qsTr("Start from")
            ComboBox {
                readonly property var names: [""].concat(dlg.kept.map(p => p.name)).concat(dlg.ready.map(p => p.name))
                model: names.map(n => n || qsTr("Settings of my own"))
                currentIndex: Math.max(0, names.indexOf(dlg.preset))
                onActivated: i => { if (names[i]) dlg.choosePreset(names[i]); else dlg.preset = "" }
            }
        }
        Button {
            visible: !!dlg.preset && dlg.kept.some(p => p.name === dlg.preset) && dlg.page.may("manage")
            kind: "quiet"; small: true
            text: Fmt.fill(qsTr("Remove “{name}”"), { name: dlg.preset })
            onClicked: {
                const gone = dlg.preset
                dlg.preset = ""
                dlg.putExports(dlg.kept.filter(p => p.name !== gone), Fmt.fill(qsTr("Removed the export settings “{name}”"), { name: gone }))
            }
        }
        Repeater {
            model: [{ title: qsTr("To read and print"), items: Fields.readFormats().map(f => ({ id: f.id, label: "." + f.id, note: f.note })) },
                    { title: qsTr("To exchange with other tools"), items: Fields.exportFormats() }]
            ColumnLayout {
                required property var modelData
                Layout.fillWidth: true
                spacing: Theme.s2
                Txt { role: "label"; text: modelData.title }
                GridLayout {
                    Layout.fillWidth: true
                    columns: 3
                    columnSpacing: Theme.s2
                    rowSpacing: Theme.s2
                    Repeater {
                        model: modelData.items
                        Choice {
                            required property var modelData
                            Layout.fillWidth: true
                            Layout.preferredWidth: 1
                            Layout.fillHeight: true
                            text: modelData.label
                            description: modelData.note
                            checked: dlg.format === modelData.id
                            onClicked: dlg.setFormat(modelData.id)
                        }
                    }
                }
            }
        }
    }

    // Content.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.step === 1
        spacing: Theme.s3
        Txt { role: "label"; text: qsTr("Languages") }
        Flow {
            Layout.fillWidth: true
            spacing: Theme.s3
            Accessible.name: qsTr("Languages exported")
            Repeater {
                model: dlg.allLanguages
                CheckBox {
                    required property string modelData
                    text: Backend.languageName(modelData)
                    checked: dlg.languages.indexOf(modelData) >= 0
                    onToggled: {
                        const next = dlg.allLanguages.filter(l => l === modelData ? checked : dlg.languages.indexOf(l) >= 0)
                        dlg.set("languages", next.length === dlg.allLanguages.length ? undefined : next)
                    }
                }
            }
        }
        Txt { visible: dlg.shown.length < dlg.total; role: "label"; text: qsTr("Entries") }
        RadioButton {
            visible: dlg.shown.length < dlg.total
            text: Fmt.fill(qsTr("All {entries}"), { entries: TermData.entries(dlg.total) })
            checked: !dlg.shownOnly
            onClicked: dlg.shownOnly = false
        }
        RadioButton {
            visible: dlg.shown.length < dlg.total
            text: dlg.shown.length === 1 ? qsTr("The 1 entry the search shows") : Fmt.fill(qsTr("The {n} entries the search shows"), { n: Fmt.num(dlg.shown.length) })
            checked: dlg.shownOnly
            onClicked: dlg.shownOnly = true
        }
        Txt { role: "label"; text: qsTr("Terms") }
        Row {
            spacing: Theme.s3
            Accessible.name: qsTr("Statuses exported")
            Repeater {
                model: ["approved", "proposed", "forbidden"]
                CheckBox {
                    required property string modelData
                    readonly property var all: dlg.options.statuses || ["approved", "proposed", "forbidden"]
                    text: TermData.statusLabel(modelData)
                    checked: all.indexOf(modelData) >= 0
                    onToggled: {
                        const next = ["approved", "proposed", "forbidden"].filter(s => s === modelData ? checked : all.indexOf(s) >= 0)
                        dlg.set("statuses", next.length === 3 ? undefined : next)
                    }
                }
            }
        }
        Txt { visible: (dlg.definition.fields || []).length > 0; role: "label"; text: qsTr("Fields") }
        Flow {
            Layout.fillWidth: true
            visible: (dlg.definition.fields || []).length > 0
            spacing: Theme.s3
            Accessible.name: qsTr("Fields exported")
            Repeater {
                model: dlg.definition.fields || []
                CheckBox {
                    required property var modelData
                    readonly property var all: dlg.options.allFields !== false ? (dlg.definition.fields || []).map(f => f.name) : (dlg.options.fields || [])
                    text: modelData.name
                    checked: all.indexOf(modelData.name) >= 0
                    onToggled: {
                        const names = (dlg.definition.fields || []).map(f => f.name)
                        const next = names.filter(n => n === modelData.name ? checked : all.indexOf(n) >= 0)
                        const o = Object.assign({}, dlg.options)
                        if (next.length === names.length) { o.allFields = true; delete o.fields }
                        else { o.allFields = false; o.fields = next }
                        dlg.options = o
                    }
                }
            }
        }
        RowLayout {
            spacing: Theme.s4
            Field {
                Layout.fillWidth: false
                label: qsTr("Only entries with")
                RowLayout {
                    spacing: Theme.s2
                    readonly property var field: (dlg.definition.fields || []).find(f => f.name === dlg.options.filterField) || null
                    ComboBox {
                        implicitWidth: 180
                        readonly property var names: [""].concat((dlg.definition.fields || []).map(f => f.name))
                        model: names.map(n => n || qsTr("Any entry"))
                        currentIndex: Math.max(0, names.indexOf(dlg.options.filterField || ""))
                        Accessible.name: qsTr("Only entries with")
                        onActivated: i => { dlg.set("filterField", names[i]); dlg.set("filterValue", undefined) }
                    }
                    ComboBox {
                        visible: !!parent.field && parent.field.kind === "picklist"
                        implicitWidth: 160
                        readonly property var values: parent.field ? [""].concat(parent.field.values || []) : [""]
                        model: values.map(v => v || qsTr("Any value"))
                        currentIndex: Math.max(0, values.indexOf(dlg.options.filterValue || ""))
                        Accessible.name: Fmt.fill(qsTr("Value of {field}"), { field: dlg.options.filterField || "" })
                        onActivated: i => dlg.set("filterValue", values[i])
                    }
                    TextField {
                        visible: !!parent.field && parent.field.kind !== "picklist"
                        implicitWidth: 160
                        text: dlg.options.filterValue || ""
                        placeholderText: qsTr("Any value")
                        Accessible.name: Fmt.fill(qsTr("Value of {field}"), { field: dlg.options.filterField || "" })
                        onEditingFinished: dlg.set("filterValue", text.trim())
                    }
                }
            }
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 160
                label: qsTr("Changed since")
                TextField {
                    text: dlg.options.changedSince || ""
                    placeholderText: qsTr("Any day")
                    invalid: text !== "" && !/^\d{4}-\d{2}-\d{2}$/.test(text)
                    onEditingFinished: if (!invalid) dlg.set("changedSince", text.trim())
                }
            }
        }
    }

    // Layout of a glossary.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.step === 2 && dlg.glossary
        spacing: Theme.s3
        Txt { role: "label"; text: qsTr("Layout") }
        RowLayout {
            spacing: Theme.s2
            Repeater {
                model: [{ id: "dictionary", t: qsTr("Dictionary"), d: qsTr("Headwords in two columns, each language on a line under them.") },
                        { id: "table", t: qsTr("Table"), d: qsTr("A column for each language, a row for each entry.") }]
                Choice {
                    required property var modelData
                    Layout.fillWidth: true
                    Layout.preferredWidth: 1
                    text: modelData.t
                    description: modelData.d
                    checked: (dlg.options.layout || "dictionary") === modelData.id
                    onClicked: dlg.set("layout", modelData.id)
                }
            }
        }
        RowLayout {
            spacing: Theme.s4
            Field {
                label: qsTr("Sorted by")
                ComboBox {
                    readonly property var codes: dlg.languages
                    model: codes.map(l => Backend.languageName(l))
                    currentIndex: Math.max(0, codes.indexOf(dlg.options.sortBy || ""))
                    onActivated: i => dlg.set("sortBy", codes[i])
                }
            }
            Field { label: qsTr("Title"); TextField { text: dlg.options.title || ""; onTextEdited: dlg.set("title", text) } }
        }
        Txt { role: "label"; text: qsTr("Show") }
        Repeater {
            model: [{ k: "letters", t: qsTr("Initial letters before their headwords") }, { k: "perTerm", t: qsTr("Every synonym as a headword of its own") },
                    { k: "groupHomonyms", t: qsTr("Entries with the same headword under it, numbered") }, { k: "labels", t: qsTr("The names of fields before their values") },
                    { k: "status", t: qsTr("Proposed and forbidden terms marked") }]
            CheckBox { required property var modelData; text: modelData.t; checked: !!dlg.options[modelData.k]; onToggled: dlg.set(modelData.k, checked ? true : undefined) }
        }
        Txt { visible: dlg.format === "pdf"; role: "label"; text: qsTr("Page") }
        Row {
            visible: dlg.format === "pdf"
            spacing: Theme.s4
            RadioButton { text: qsTr("A4"); checked: dlg.options.page !== "letter"; onClicked: dlg.set("page", "a4") }
            RadioButton { text: qsTr("Letter"); checked: dlg.options.page === "letter"; onClicked: dlg.set("page", "letter") }
        }
    }

    // Keeping the settings, at the last step, for those who manage it.
    Field {
        visible: dlg.step === dlg.steps.length - 1 && !!dlg.page && dlg.page.may("manage")
        label: qsTr("Keep these settings as")
        help: Fmt.fill(qsTr("Kept with {name}, for everyone who exports it."), { name: dlg.page ? dlg.page.name : "" })
        RowLayout {
            spacing: Theme.s2
            TextField { id: keepName; Layout.fillWidth: true; placeholderText: qsTr("For example Client glossary") }
            Button {
                small: true
                text: qsTr("Keep")
                enabled: keepName.text.trim() !== ""
                onClicked: {
                    const name = keepName.text.trim()
                    const next = dlg.kept.filter(p => p.name !== name).concat([{ name: name, options: dlg.options }])
                    dlg.putExports(next, Fmt.fill(qsTr("Kept the export settings “{name}” with {termbase}"), { name: name, termbase: dlg.page.name }))
                    dlg.preset = name
                    keepName.text = ""
                }
            }
        }
    }

    buttons: [
        Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button { visible: dlg.step > 0; small: true; text: qsTr("Back"); onClicked: dlg.step-- },
        Button { visible: dlg.step < dlg.steps.length - 1; small: true; text: qsTr("Continue"); onClicked: dlg.step++ },
        Button {
            kind: "primary"; small: true
            text: dlg.exporting ? qsTr("Exporting…") : Fmt.fill(qsTr("Export to {format}"), { format: dlg.glossary ? "." + dlg.format : Fields.formatLabel(dlg.format) })
            enabled: !dlg.exporting && dlg.blocking() === ""
            ToolTip { visible: parent.hovered && dlg.blocking() !== ""; text: dlg.blocking() }
            onClicked: { saveDialog.selectedFile = "file:" + dlg.fileName(); saveDialog.open() }
        }
    ]
}
