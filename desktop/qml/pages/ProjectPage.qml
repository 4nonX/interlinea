import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import QtCore
import Interlinea

// A project's page (web/src/screens/ProjectSettings.tsx and
// ServerProjectPage.tsx): its details, files, languages, memories,
// terminology, segmentation, checks and analysis, pre-translation, on a
// team server the work given out and who is working now, saving it as a
// template, and deleting it. What is changed is saved together from the
// bar at the foot.
Item {
    id: project
    signal notice(string text)
    signal back()
    signal gone()
    signal openEditor(string connectionId, var projectId, string file, string lang)

    property string cid: ""
    property int projectId: 0
    property var data_: ({})
    property var progress: []
    property var templates: []
    property var assignments: []
    property var presence: []
    property var people: []
    property var analysis: []
    property string analysisLang: ""
    property bool analysing: false
    property bool loaded: false
    property bool dirty: false
    property bool saving: false
    property string saveError: ""
    property string busy: ""
    property bool isGone: false

    readonly property var conn: Backend.connection(cid)
    readonly property bool local: !!conn && conn.local
    readonly property var s: data_.settings || {}
    readonly property string name: s.name || ""
    readonly property var access: data_.access || {}
    readonly property bool manages: Roles.allows(access.role || "", "manage") || (!!conn && conn.admin) || local
    readonly property string base: "/api/projects/" + projectId

    function say(text) { if (text) notice(text) }
    // Opens a project; the page starts from scratch.
    function open(connectionId, id) {
        cid = connectionId
        projectId = Number(id)
        data_ = ({}); progress = []; templates = []; assignments = []; presence = []; people = []
        analysis = []; analysisLang = ""; analysing = false
        loaded = false; dirty = false; saving = false; saveError = ""; busy = ""; isGone = false
        reload()
    }
    // The project is gone (deleted here or elsewhere): said once.
    function leave() {
        if (isGone) return
        isGone = true
        gone()
    }
    function reload() {
        if (!conn || !conn.ready || !projectId) return
        const id = projectId
        Api.get(cid, base, r => {
            if (id !== projectId) return
            if (!r.ok) { if (r.status === 404) leave(); else say(r.error.message); return }
            data_ = r.data
            // Settings being edited are not replaced under the person's hands.
            if (!dirty) loadForm()
            Api.get(cid, base + "/progress", p => { if (p.ok && id === projectId) progress = p.data })
            Api.get(cid, "/api/templates", t => { if (id === projectId) templates = t.ok ? t.data : [] })
            if (local) return
            Api.get(cid, base + "/assignments", a => { if (a.ok && id === projectId) assignments = a.data })
            Api.get(cid, base + "/presence", a => { if (id === projectId) presence = a.ok ? a.data : [] })
            if (manages) Api.get(cid, "/api/people", a => { if (a.ok && id === projectId) people = a.data })
        })
    }
    function loadForm() {
        form.load(cid, error => {
            if (error) { say(error); return }
            if (dirty) return
            loaded = false
            const fixed = []
            for (const f of s.files || []) if (f.targetLang && fixed.indexOf(f.targetLang) < 0) fixed.push(f.targetLang)
            form.fixed = fixed
            form.suggested = (s.targetLangs || []).slice()
            form.setSettings(s, false)
            form.readOnly = !manages
            loaded = true
        })
    }
    function discard() {
        dirty = false
        saveError = ""
        loaded = false
        form.setSettings(s, false)
        form.readOnly = !manages
        loaded = true
    }

    // Saving: the memories made here first, then the project termbase where
    // the masters changed, then the settings themselves.
    function save() {
        if (saving) return
        saving = true
        saveError = ""
        const failed = why => { saving = false; saveError = Fmt.fill(qsTr("Could not save: {error}"), { error: why }) }
        form.createNewMemories(error => {
            if (error) { failed(error); return }
            const st = form.settings()
            const was = s
            // Files are added and removed on their own.
            st.files = was.files || []
            const t = st.terminology || {}
            const mode = t.mode || "own"
            const masters = t.masters || []
            let termbase = mode === "direct" ? "" : ((was.terminology || {}).termbase || "")
            const withTermbase = () => { t.termbase = termbase; st.terminology = t; putSettings(st) }
            if (mode === "direct") { delete t.termbase; st.terminology = t; putSettings(st) }
            else if (!termbase) {
                // A project that now keeps its own terms gets its termbase.
                termbase = st.name + " (project)"
                const body = { definition: { name: termbase } }
                if (mode === "layered") body.upstreams = masters
                else body.definition.languages = [st.sourceLang].concat(st.targetLangs || [])
                Api.post(cid, "/api/termbases", body, r => {
                    if (!r.ok && r.error.code !== "exists") { failed(r.error.message); return }
                    withTermbase()
                })
            } else if (mode === "layered" && JSON.stringify(masters) !== JSON.stringify((was.terminology || {}).masters || [])) {
                // A layered one follows the masters chosen.
                Api.put(cid, "/api/termbases/" + Api.seg(termbase) + "/upstreams", { upstreams: masters }, r => {
                    if (!r.ok) { failed(r.error.message); return }
                    withTermbase()
                })
            } else withTermbase()
        })
    }
    function putSettings(st) {
        const before = s.targetLangs || []
        Api.put(cid, base + "/settings", st, r => {
            saving = false
            if (!r.ok) {
                const conflict = r.error.code === "conflict"
                const rest = form.showProblems(r.error.problems)
                saveError = conflict ? qsTr("The project changed in the meantime. It is loaded again; make your change once more.")
                          : !(r.error.problems || []).length ? Fmt.fill(qsTr("Could not save: {error}"), { error: r.error.message })
                          : !rest.length ? qsTr("Could not save: see what is marked above.")
                          : Fmt.fill(qsTr("Could not save: {error}"), { error: rest.join(" ") })
                if (conflict) { dirty = false; reload() }
                return
            }
            dirty = false
            saveError = ""
            const keepAccess = access
            const next = r.data
            if (!next.access) next.access = keepAccess
            data_ = next
            const fresh = (next.settings.targetLangs || []).filter(t => before.indexOf(t) < 0).map(t => Backend.languageName(t))
            say(fresh.length ? Fmt.fill(qsTr("Saved the settings of “{name}”; {languages} added to every file"), { name: next.settings.name, languages: fresh.join(", ") })
                             : Fmt.fill(qsTr("Saved the settings of “{name}”"), { name: next.settings.name }))
            // The analysis was of the settings before.
            analysis = []
            loadForm()
            reload()
        }, data_.version || 0)
    }

    // A file from the project, saved where the person says.
    property string fetched: ""
    function download(path, query, what) {
        Backend.fetch(cid, path, query).done.connect(r => {
            if (!r.ok) { say(r.error.message); return }
            if (!r.temp) { say(Fmt.fill(qsTr("{what} could not be saved: {error}"), { what: what, error: r.saveError })); return }
            fetched = r.temp
            saveDialog.title = Fmt.fill(qsTr("Save {what}"), { what: what })
            saveDialog.selectedFile = StandardPaths.writableLocation(StandardPaths.DocumentsLocation) + "/" + (r.fileName || "download")
            saveDialog.open()
        })
    }
    FileDialog {
        id: saveDialog
        fileMode: FileDialog.SaveFile
        currentFolder: StandardPaths.writableLocation(StandardPaths.DocumentsLocation)
        onAccepted: {
            const problem = Backend.keep(project.fetched, selectedFile)
            project.say(problem ? Fmt.fill(qsTr("{file} could not be saved: {error}"), { file: Backend.localPath(selectedFile), error: problem })
                                : Fmt.fill(qsTr("Saved {file}"), { file: Backend.localPath(selectedFile) }))
        }
    }

    // Files are added one after the other: each is read where the project
    // is kept first, and what can't be translated goes in for reference.
    function addFiles(urls) {
        const files = Backend.filesOf(urls)
        if (!files.length) return
        let added = 0
        const next = i => {
            if (i >= files.length) {
                busy = ""
                if (added) say(Fmt.fill(qsTr("Added {files}"), { files: Fmt.plural(added, qsTr("{n} file"), qsTr("{n} files")) }))
                reload()
                return
            }
            const f = files[i]
            if ((s.files || []).some(x => x.name.toLowerCase() === f.name.toLowerCase())) {
                say(Fmt.fill(qsTr("{file} is in the project already; remove it first to replace it"), { file: f.name }))
                next(i + 1)
                return
            }
            if (f.problem) { say(Fmt.fill(qsTr("{file} can't be read: {error}"), { file: f.path, error: f.problem })); next(i + 1); return }
            busy = Fmt.fill(qsTr("Reading {file}…"), { file: f.name })
            Backend.upload(cid, "POST", "/api/files/inspect", f.path, "application/octet-stream", { name: f.name }).done.connect(inn => {
                if (!inn.ok) { say(inn.error.message); next(i + 1); return }
                if (inn.data.format === "package") { say(Fmt.fill(qsTr("{file} is a package: a project is made from it, it is not added to one"), { file: f.name })); next(i + 1); return }
                const body = { name: f.name, data: Backend.base64File(f.path) }
                if (inn.data.translatable) body.role = "translatable"
                else { body.role = "reference"; body.format = (f.name.split(".").pop() || "").toLowerCase() }
                busy = Fmt.fill(qsTr("Adding {file}…"), { file: f.name })
                Api.post(cid, base + "/files", body, r => {
                    if (r.ok) {
                        // The next file is added to the project as it is now.
                        const next_ = r.data
                        if (!next_.access) next_.access = access
                        data_ = next_
                        added++
                    } else say(r.error.message)
                    next(i + 1)
                }, { version: data_.version || 0 })
            })
        }
        next(0)
    }
    function removeFile(file) {
        Api.call(cid, "DELETE", base + "/files", undefined, r => { say(r.ok ? Fmt.fill(qsTr("Removed {file}"), { file: file }) : r.error.message); reload() }, { name: file }, data_.version || 0)
    }
    function runAnalysis() {
        analysing = true
        Api.post(cid, base + "/analysis", {}, r => {
            analysing = false
            if (r.ok) analysis = r.data
            else say(r.error.message)
        })
    }
    // The analysis as CSV, written here.
    function analysisCsv() {
        const quote = x => "\"" + String(x).replace(/"/g, "\"\"") + "\""
        let csv = "\"Language\",\"File\",\"Match\",\"Segments\",\"Words\",\"Characters\"\r\n"
        for (const rep of analysis)
            for (const f of rep.files || [])
                for (const r of f.rows || [])
                    csv += [quote(rep.targetLang), quote(f.name), quote(r.name), r.segments || 0, r.words || 0, r.characters || 0].join(",") + "\r\n"
        return csv
    }
    function pretranslate() {
        Api.post(cid, base + "/pretranslate", {}, r => {
            if (!r.ok) { say(r.error.message); return }
            const filled = r.data.reduce((n, x) => n + (x.filled || 0), 0)
            say(filled ? Fmt.fill(qsTr("Pre-translated {segments}"), { segments: Fmt.plural(filled, qsTr("{n} segment"), qsTr("{n} segments")) }) : qsTr("No empty segment has a match good enough to insert"))
            reload()
        })
    }
    function saveTemplate(tname) {
        if (!tname) return
        // Saving over a template of the same name needs its version.
        const old = templates.find(t => t.name.toLowerCase() === tname.toLowerCase())
        const version = old ? old.version || 0 : 0
        Api.put(cid, "/api/templates", form.templateOf(tname, s), r => {
            say(!r.ok ? r.error.message : version ? Fmt.fill(qsTr("Updated the template “{name}”"), { name: tname }) : Fmt.fill(qsTr("Saved the template “{name}”"), { name: tname }))
            Api.get(cid, "/api/templates", t => { if (t.ok) templates = t.data })
        }, version)
    }
    function deleteProject() {
        Api.del(cid, base, r => { if (!r.ok) { say(r.error.message); return } leave() })
    }

    ProjectForm {
        id: form
        objectName: "projectForm"
        onChanged: if (project.loaded) project.dirty = true
    }
    Timer { id: soon; interval: 400; onTriggered: project.reload() }
    Connections {
        target: Backend
        function onEvent(c, type, data) {
            if (c !== project.cid) return
            if ((type === "project" || type === "presence") && Number((data || {}).projectId) === project.projectId) soon.restart()
            if (type === "account") soon.restart()
        }
        function onReset(c) { if (c === project.cid) soon.restart() }
    }

    Page {
        id: page
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: bar.visible ? bar.top : parent.bottom
        title: Fmt.fill(qsTr("{name}: settings"), { name: project.name })
        maxWidth: 760
        Accessible.name: qsTr("Project")
        headerActions: [
            Button { kind: "quiet"; iconName: "arrow-left"; text: qsTr("Projects"); Accessible.name: qsTr("Back to the projects"); onClicked: project.back() },
            Button { iconName: "pencil-line"; text: qsTr("Open in the editor"); onClicked: project.openEditor(project.cid, project.projectId, "", "") }
        ]

        Txt {
            Layout.fillWidth: true
            role: "help"
            text: {
                let where = project.local ? qsTr("On this computer") : Fmt.fill(qsTr("On the team server {server}"), { server: project.conn ? project.conn.displayName : "" })
                if (project.s.client) where += Fmt.fill(qsTr(", for {client}"), { client: project.s.client })
                where += "."
                const made = project.data_.createdAt, by = project.data_.createdBy || ""
                if (made) where += " " + (by ? Fmt.fill(qsTr("Made by {who} on {date}."), { who: by, date: Fmt.date(made) }) : Fmt.fill(qsTr("Made on {date}."), { date: Fmt.date(made) }))
                return where
            }
        }

        Panel { title: qsTr("Details"); FormDetails { form: form } }

        // Files: what each is for and how far it is, the original and the
        // translation to save, and files to add.
        Panel {
            id: filesPanel
            title: qsTr("Files")
            spacing: Theme.s3
            readonly property var files: project.s.files || []
            readonly property var targets: project.s.targetLangs || []
            Txt { visible: filesPanel.files.length === 0; role: "muted"; text: qsTr("The project has no files yet.") }
            Table {
                id: filesTable
                visible: filesPanel.files.length > 0
                name: qsTr("Files")
                columns: [{ title: qsTr("File"), fill: 1 }, { title: qsTr("Words"), width: 72, align: "right" }, { title: qsTr("Done"), width: 110 }, { title: "", width: 320 }]
                Repeater {
                    model: filesPanel.files
                    TableRow {
                        id: frow
                        required property var modelData
                        required property int index
                        readonly property string fname: modelData.name
                        readonly property string role: modelData.role
                        readonly property var langs: project.progress.filter(p => p.file === fname)
                        readonly property int words: langs.reduce((n, p) => Math.max(n, p.words || 0), 0)
                        last: index === filesPanel.files.length - 1
                        CellTitle {
                            title: frow.fname
                            sub: frow.role === "reference" ? qsTr("Reference file")
                               : frow.role === "package" ? Fmt.fill(qsTr("The {kind} the project was made from"), { kind: /\.mqout$/i.test(frow.fname) ? qsTr("handoff package") : qsTr("project package") })
                               : frow.modelData.targetLang ? Fmt.fill(qsTr("Translated into {language} only"), { language: Backend.languageName(frow.modelData.targetLang) })
                               : filesPanel.targets.length > 1 ? qsTr("Translated into every target language")
                               : Fmt.fill(qsTr("Translated into {language}"), { language: filesPanel.targets.map(t => Backend.languageName(t)).join(", ") })
                        }
                        Txt { horizontalAlignment: Text.AlignRight; text: frow.words ? Fmt.num(frow.words) : "–" }
                        // How far it is in each language, each a way into the editor.
                        Column {
                            spacing: 4
                            Txt { visible: frow.langs.length === 0; role: "muted"; text: "–" }
                            Repeater {
                                model: frow.langs
                                Row {
                                    required property var modelData
                                    readonly property int pct: modelData.segments ? Math.round(100 * modelData.confirmed / modelData.segments) : 100
                                    spacing: 6
                                    LangChip { code: modelData.lang; anchors.verticalCenter: parent.verticalCenter }
                                    Button {
                                        kind: "quiet"
                                        small: true
                                        leftPadding: 2; rightPadding: 2
                                        text: parent.pct + "%"
                                        Accessible.name: Fmt.fill(qsTr("Translate {file} into {language}: {confirmed} of {segments} segments confirmed"),
                                                                  { file: frow.fname, language: Backend.languageName(modelData.lang), confirmed: modelData.confirmed, segments: modelData.segments })
                                        ToolTip { visible: parent.hovered; text: Fmt.fill(qsTr("{confirmed} of {segments} segments confirmed. Opens the file in the editor."), { confirmed: modelData.confirmed, segments: modelData.segments }) }
                                        onClicked: project.openEditor(project.cid, project.projectId, frow.fname, modelData.lang)
                                    }
                                }
                            }
                        }
                        // What can be saved or taken away.
                        RowLayout {
                            spacing: Theme.s1
                            Item { Layout.fillWidth: true }
                            Button {
                                kind: "quiet"; small: true; iconName: "download"
                                text: qsTr("Original")
                                Accessible.name: Fmt.fill(qsTr("Save the original of {file}"), { file: frow.fname })
                                onClicked: project.download(project.base + "/files/content", { name: frow.fname }, Fmt.fill(qsTr("the original of {file}"), { file: frow.fname }))
                            }
                            Button {
                                visible: frow.role === "translatable"
                                kind: "quiet"; small: true; iconName: "download"
                                text: qsTr("Translation")
                                Accessible.name: Fmt.fill(qsTr("Save the translation of {file}"), { file: frow.fname })
                                onClicked: {
                                    if (frow.langs.length > 1) { langMenu.popup(); return }
                                    project.download(project.base + "/files/translated", { name: frow.fname, lang: (frow.langs[0] || {}).lang || "" },
                                                     Fmt.fill(qsTr("the translation of {file}"), { file: frow.fname }))
                                }
                                Menu {
                                    id: langMenu
                                    Repeater {
                                        model: frow.langs
                                        MenuItem {
                                            required property var modelData
                                            text: Backend.languageName(modelData.lang)
                                            onTriggered: project.download(project.base + "/files/translated", { name: frow.fname, lang: modelData.lang },
                                                                          Fmt.fill(qsTr("the translation of {file}"), { file: frow.fname }))
                                        }
                                    }
                                }
                            }
                            ConfirmButton {
                                visible: project.manages
                                text: qsTr("Remove")
                                iconName: "trash"
                                action: qsTr("Remove")
                                accessibleName: Fmt.fill(qsTr("Remove {file}"), { file: frow.fname })
                                question: project.local ? Fmt.fill(qsTr("Remove {file} with its translations?"), { file: frow.fname })
                                                        : Fmt.fill(qsTr("Remove {file} with its translations and the work given out in it, for everyone?"), { file: frow.fname })
                                onConfirmed: project.removeFile(frow.fname)
                            }
                        }
                    }
                }
            }
            Button {
                visible: (project.s.files || []).some(f => f.role === "package")
                small: true
                iconName: "download"
                text: qsTr("Save the return package")
                onClicked: project.download(project.base + "/return-package", {}, qsTr("the return package"))
            }
            Txt { visible: project.manages; role: "label"; text: qsTr("Add files") }
            DropZone {
                visible: project.manages
                Accessible.name: qsTr("Add files")
                onDropped: urls => { if (!project.busy) project.addFiles(urls) }
                Icon { name: "upload"; color: Theme.text2; size: 18; Layout.alignment: Qt.AlignVCenter }
                Txt {
                    Layout.fillWidth: true
                    role: "muted"
                    text: project.busy || qsTr("Drop files or folders here. DOCX, PPTX, XLSX, IDML and web page files keep their formatting; of a PDF the text is read, and its translation is a DOCX document; bilingual files (.sdlxliff, .mqxliff, .mqxlz, .mxliff, .txml, .ttx, .xlf) keep their translations and states, and so do bilingual tables for review in .docx and .rtf.")
                }
                Button { small: true; iconName: "file-text"; text: qsTr("Add files"); enabled: !project.busy; Layout.alignment: Qt.AlignVCenter; onClicked: filesDialog.open() }
                Button { small: true; iconName: "folder-open"; text: qsTr("Add a folder"); enabled: !project.busy; Layout.alignment: Qt.AlignVCenter; onClicked: folderDialog.open() }
            }
            Txt { visible: project.manages; Layout.fillWidth: true; role: "help"; text: qsTr("Reference files (a style guide, an earlier version) travel with the project for the translators to read; they are not translated.") }
        }

        Panel {
            title: qsTr("Languages")
            spacing: Theme.s2
            Txt { role: "label"; text: qsTr("Source language") }
            RowLayout { spacing: Theme.s2; Txt { text: Backend.languageName(project.s.sourceLang || "") } LangChip { code: project.s.sourceLang || "" } }
            Txt { Layout.fillWidth: true; role: "help"; text: qsTr("The source language stays: every file was segmented by its rules.") }
            Txt { Layout.topMargin: Theme.s2; role: "label"; text: qsTr("Target languages") }
            FormTargets { form: form }
            Txt { Layout.fillWidth: true; role: "help"; text: qsTr("A language you add gets every file to translate, with nothing translated yet. A language you take out loses its translations.") }
        }

        Panel {
            title: qsTr("Memories")
            FormMemories { form: form }
            // The project's own memories, and adding their units to a main
            // memory once the project is done.
            ColumnLayout {
                id: ownMemories
                readonly property var own: {
                    const out = []
                    for (const t of project.s.targetLangs || [])
                        for (const m of (project.s.memories || {})[t] || [])
                            if (m.memory === project.name + " (" + t + ")") {
                                const c = (form.catalog.memories || []).find(x => x.name === m.memory)
                                if (c) out.push(c)
                            }
                    return out
                }
                visible: own.length > 0 && project.manages
                Layout.fillWidth: true
                spacing: Theme.s2
                Txt { role: "label"; text: qsTr("Project memories") }
                Repeater {
                    model: ownMemories.own
                    RowLayout {
                        id: pm
                        required property var modelData
                        readonly property var mains: (form.catalog.memories || []).filter(c => !ownMemories.own.some(o => o.name === c.name)
                                                                                         && form.samePrimary(c.sourceLang, modelData.sourceLang) && form.samePrimary(c.targetLang, modelData.targetLang)).map(c => c.name)
                        property bool adding: false
                        spacing: Theme.s2
                        Txt { textFormat: Text.StyledText; text: "<b>" + pm.modelData.name + "</b> <font color=\"" + Theme.text3 + "\">" + Fmt.plural(pm.modelData.units || 0, qsTr("{n} unit"), qsTr("{n} units")) + "</font>" }
                        Txt { visible: pm.mains.length === 0; role: "muted"; text: Fmt.fill(qsTr("No main memory for {language} to add them to."), { language: Backend.languageName(pm.modelData.targetLang) }) }
                        ComboBox {
                            id: mainBox
                            visible: pm.mains.length > 0
                            implicitWidth: 240
                            model: pm.mains
                            Accessible.name: Fmt.fill(qsTr("Main memory for the units of {memory}"), { memory: pm.modelData.name })
                        }
                        Button {
                            visible: pm.mains.length > 0
                            small: true
                            text: qsTr("Add its units")
                            enabled: (pm.modelData.units || 0) > 0 && !pm.adding
                            Accessible.name: Fmt.fill(qsTr("Add the units of {memory} to the main memory"), { memory: pm.modelData.name })
                            onClicked: {
                                const mname = pm.modelData.name, main = pm.mains[mainBox.currentIndex]
                                pm.adding = true
                                Backend.fetch(project.cid, "/api/memories/" + Api.seg(mname) + "/export", {}).done.connect(tmx => {
                                    if (!tmx.ok || !tmx.temp) { pm.adding = false; project.say(tmx.ok ? tmx.saveError : tmx.error.message); return }
                                    Backend.upload(project.cid, "POST", "/api/memories/" + Api.seg(main) + "/import", tmx.temp, "application/xml", {}).done.connect(r => {
                                        pm.adding = false
                                        project.say(r.ok ? Fmt.fill(qsTr("Added {units} of {memory} to {main}"), { units: Fmt.plural(r.data.stored || 0, qsTr("{n} unit"), qsTr("{n} units")), memory: mname, main: main }) : r.error.message)
                                        project.loadForm()
                                    })
                                })
                            }
                        }
                    }
                }
                Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Units already in the main memory with the same translation are updated, not added twice.") }
            }
        }

        Panel { title: qsTr("Terminology"); FormTerminology { form: form } }
        Panel { title: qsTr("Segmentation"); FormSegmentation { form: form } }

        Panel {
            title: qsTr("Checks and analysis")
            spacing: Theme.s4
            FormChecks { form: form }
            // The match analysis of the files as they are, against the saved settings.
            ColumnLayout {
                id: analysisPart
                Layout.fillWidth: true
                spacing: Theme.s2
                readonly property var langs: project.analysis.map(a => a.targetLang)
                readonly property string shown: langs.indexOf(project.analysisLang) >= 0 ? project.analysisLang : (langs[0] || "")
                readonly property var rep: project.analysis.find(a => a.targetLang === shown) || null
                Txt { role: "label"; text: qsTr("Match analysis") }
                RowLayout {
                    spacing: Theme.s2
                    Button {
                        small: true
                        iconName: "bar-chart"
                        text: project.analysing ? qsTr("Analysing…") : project.analysis.length ? qsTr("Analyse again") : qsTr("Analyse the files")
                        enabled: !project.analysing
                        onClicked: project.runAnalysis()
                    }
                    Button { visible: project.analysis.length > 0; kind: "quiet"; small: true; iconName: "download"; text: qsTr("Save as CSV"); onClicked: csvDialog.open() }
                }
                Txt { visible: project.analysis.length === 0; Layout.fillWidth: true; role: "help"; text: qsTr("Counts the words to translate by how well the project's memories match them, with the saved settings.") }
                Tabs {
                    visible: analysisPart.langs.length > 1
                    Layout.fillWidth: true
                    inset: 0
                    small: true
                    name: qsTr("Target language")
                    model: analysisPart.langs.map(l => Backend.languageName(l))
                    current: Math.max(0, analysisPart.langs.indexOf(analysisPart.shown))
                    onChanged: i => project.analysisLang = analysisPart.langs[i]
                }
                Txt {
                    visible: !!analysisPart.rep
                    Layout.fillWidth: true
                    role: "help"
                    readonly property var mems: ((project.s.memories || {})[analysisPart.shown] || []).filter(m => m.lookup).map(m => m.memory)
                    text: mems.length === 0 ? qsTr("No memory for this language, so only repetitions count.")
                        : Fmt.fill(qsTr("Against {memories}, counted in this order: context matches first, then repetitions within a file and across files, then the best match."), { memories: mems.join(qsTr(" and ")) })
                }
                AnalysisTable {
                    visible: !!analysisPart.rep
                    report: analysisPart.rep || {}
                    name: Fmt.fill(qsTr("Match analysis, {language}"), { language: Backend.languageName(analysisPart.shown) })
                }
            }
        }

        Panel {
            title: qsTr("Pre-translation")
            FormPrepare { form: form }
            Button { visible: project.manages; text: qsTr("Pre-translate now"); enabled: !project.dirty; onClicked: project.pretranslate() }
            Txt {
                visible: project.manages
                Layout.fillWidth: true
                role: "help"
                text: project.dirty ? qsTr("Save your changes first; pre-translation uses the saved settings.")
                                    : Fmt.fill(qsTr("Fills the empty segments of every file and language with matches of {score}% or more from the memories set for lookup."), { score: (project.s.prepare || {}).minScore || 100 })
            }
        }

        // The work given out, on a team server.
        Panel {
            visible: !project.local
            title: qsTr("Work")
            spacing: Theme.s3
            ProjectWork { project: project }
        }
        Panel {
            visible: !project.local
            title: qsTr("Working now")
            Txt { visible: project.presence.length === 0; role: "muted"; text: qsTr("Nobody has the project open.") }
            Repeater {
                model: project.presence
                CellTitle {
                    required property var modelData
                    title: modelData.userName || ""
                    sub: Fmt.fill(qsTr("{file}, {language}, segment {segment}, since {since}"), { file: modelData.file || "", language: modelData.targetLang || "", segment: modelData.segment || "", since: Fmt.time(modelData.since) })
                }
            }
        }
        Loader {
            Layout.fillWidth: true
            active: !project.local && project.manages && project.projectId > 0
            visible: active
            sourceComponent: AccessPanel { title: qsTr("People with access"); cid: project.cid; kind: "project"; resource: String(project.projectId) }
        }

        Panel {
            visible: project.manages
            title: qsTr("Template")
            Field {
                id: templateField
                label: qsTr("Template name")
                readonly property bool clash: project.templates.some(t => t.name.toLowerCase() === templateName.text.trim().toLowerCase())
                RowLayout {
                    spacing: Theme.s2
                    TextField {
                        id: templateName
                        implicitWidth: 320
                        Accessible.name: qsTr("Template name")
                        text: project.s.client || project.name
                    }
                    Button {
                        text: templateField.clash ? qsTr("Replace template") : qsTr("Save as template")
                        enabled: templateName.text.trim() !== "" && !project.dirty
                        onClicked: project.saveTemplate(templateName.text.trim())
                    }
                }
            }
            Txt {
                Layout.fillWidth: true
                role: "help"
                text: project.dirty ? qsTr("Save your changes first; the template takes the saved settings.")
                                    : qsTr("Languages, memories, termbases, checks and pre-translation; not the files, name or due date. New projects can start from it.")
            }
        }

        Panel {
            visible: project.manages
            title: qsTr("Delete the project")
            Txt {
                Layout.fillWidth: true
                role: "help"
                text: project.local ? qsTr("The project, its files and translations are deleted. Its memories and termbase stay; delete them on their own if nothing else uses them.")
                                    : qsTr("The project, its files and translations and the work given out in it are removed from the server for everyone. Its memories and termbase stay; delete them on their own if nothing else uses them.")
            }
            ConfirmButton {
                small: false
                iconName: "trash"
                text: qsTr("Delete project")
                action: qsTr("Delete")
                question: project.local ? Fmt.fill(qsTr("Delete “{name}” and its translations?"), { name: project.name })
                                        : Fmt.fill(qsTr("Delete “{name}” on {server} for everyone?"), { name: project.name, server: project.conn ? project.conn.displayName : "" })
                onConfirmed: project.deleteProject()
            }
        }
    }

    // .save-bar: what was changed is saved together, from the foot of the page.
    Rectangle {
        id: bar
        visible: project.manages
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: barRow.implicitHeight + 20
        color: Theme.surface
        Rectangle { width: parent.width; height: 1; color: Theme.border }
        // Under the page's column, as wide as it.
        RowLayout {
            id: barRow
            anchors.verticalCenter: parent.verticalCenter
            anchors.horizontalCenter: parent.horizontalCenter
            width: Math.min(parent.width - 2 * Theme.s6, page.maxWidth)
            spacing: Theme.s2
            Txt {
                Layout.fillWidth: true
                visible: project.saveError !== ""
                role: "error"
                text: project.saveError
                Accessible.name: qsTr("Why the changes were not saved")
            }
            Item { Layout.fillWidth: true; visible: project.saveError === "" }
            Button { kind: "quiet"; text: qsTr("Discard changes"); enabled: project.dirty && !project.saving; onClicked: project.discard() }
            Button { kind: "primary"; text: project.saving ? qsTr("Saving…") : qsTr("Save changes"); enabled: project.dirty && !project.saving; onClicked: project.save() }
        }
    }

    FileDialog {
        id: filesDialog
        title: qsTr("Files to add")
        fileMode: FileDialog.OpenFiles
        onAccepted: project.addFiles(Array.from(selectedFiles))
    }
    FolderDialog {
        id: folderDialog
        title: qsTr("A folder of files")
        onAccepted: project.addFiles([selectedFolder])
    }
    FileDialog {
        id: csvDialog
        title: qsTr("Save the analysis")
        fileMode: FileDialog.SaveFile
        nameFilters: [qsTr("CSV files (*.csv)")]
        currentFolder: StandardPaths.writableLocation(StandardPaths.DocumentsLocation)
        selectedFile: StandardPaths.writableLocation(StandardPaths.DocumentsLocation) + "/" + Backend.fileSafe(project.name + " analysis") + ".csv"
        onAccepted: {
            const problem = Backend.writeText(selectedFile, "﻿" + project.analysisCsv())
            project.say(problem ? Fmt.fill(qsTr("The analysis could not be saved: {error}"), { error: problem }) : Fmt.fill(qsTr("Saved {file}"), { file: Backend.localPath(selectedFile) }))
        }
    }
}
