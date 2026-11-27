import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// New project (web/src/screens/NewProject.tsx): where it starts from and
// where it is kept; its files and languages; its memories; its
// terminology; and a review with the checks, pre-translation, the match
// analysis and saving the settings as a template.
Page {
    id: page
    title: qsTr("New project")
    maxWidth: 760
    headerActions: [
        Button { kind: "quiet"; text: qsTr("Cancel"); Accessible.name: qsTr("Cancel the new project"); onClicked: page.cancelled() }
    ]

    signal notice(string text)
    signal cancelled()
    signal created(string connectionId, var projectId)

    readonly property int maxFile: 200 * 1024 * 1024
    // Below this the person confirms the language (langdetect.Confident).
    readonly property real confident: 0.75

    property int step: 0
    property string cid: ""
    // Chosen by the person, rather than the first place that was ready.
    property bool placeChosen: false
    property bool loading: false
    property var defaults: ({})
    property var templates: []
    property var projects: []
    property string startKind: "blank"
    property string startId: ""
    // The files: { path, name, size, inspection, role, read }.
    property var docs: []
    property bool nameTouched: false
    property string autoName: ""
    property bool sourceTouched: false
    property var previewData: []
    property bool previewing: false
    property string previewError: ""
    property string createError: ""
    property bool creating: false
    property int previewSeq: 0
    property bool checksOpen: false
    property bool segmentationOpen: false
    property bool asTemplate: false
    property string templateName: ""
    property string reviewLang: ""

    readonly property var conn: Backend.connection(cid)
    readonly property var readyPlaces: Backend.connections.filter(c => c.ready)
    readonly property var s: form.s

    ProjectForm {
        id: form
        objectName: "newProjectForm"
        onChanged: {
            // A name the person typed stays theirs.
            if ((form.s.name || "") !== page.autoName) page.nameTouched = true
            if (page.step === 4) previewTimer.restart()
        }
    }
    Timer { id: previewTimer; interval: 350; onTriggered: page.preview() }

    function strings(v) { return v || [] }
    // "Mietvertrag_Hofmann.docx" names the project "Mietvertrag Hofmann".
    function nameFromFile(file) {
        let f = file.split("/").pop()
        const dot = f.lastIndexOf(".")
        if (dot > 0) f = f.slice(0, dot)
        return f.replace(/_+/g, " ").trim()
    }
    function packageKind(name) { return /\.mqout$/i.test(name) ? qsTr("handoff package") : qsTr("project package") }
    function bilingualName(format) {
        return ({ sdlxliff: ".sdlxliff", mqxliff: ".mqxliff", mxliff: ".mxliff", txml: ".txml", ttx: ".ttx", sdlreview: qsTr("Review table (.docx)"),
                  mqrtf: qsTr("Two-column table (.rtf)"), mqdocx: qsTr("Two-column table (.docx)"), dvrtf: qsTr("External view (.rtf)"),
                  "xliff-2": "XLIFF 2", "xliff-1.2": "XLIFF 1.2" })[format] || "XLIFF"
    }
    // What a document format is called and what it keeps of the file
    // besides its text; empty for any other format.
    function documentFormatText(format) {
        const formatting = qsTr("formatting kept"), markup = qsTr("markup kept"), keys = qsTr("keys and placeholders kept")
        const names = {
            docx: [qsTr("DOCX document"), formatting], pptx: [qsTr("PPTX presentation"), formatting], xlsx: [qsTr("XLSX workbook"), formatting],
            html: [qsTr("Web page"), markup], idml: [qsTr("IDML document"), qsTr("layout kept")], odt: [qsTr("ODT document"), formatting],
            ods: [qsTr("ODS spreadsheet"), formatting], odp: [qsTr("ODP presentation"), formatting], xml: [qsTr("XML file"), markup],
            md: [qsTr("Markdown file"), markup], properties: [qsTr("Properties file"), keys], json: [qsTr("JSON file"), keys],
            yaml: [qsTr("YAML file"), keys], po: [qsTr("PO file"), keys], resx: [qsTr("RESX file"), keys],
            srt: [qsTr("SRT subtitles"), qsTr("times kept")], vtt: [qsTr("WebVTT subtitles"), qsTr("times kept")], sbv: [qsTr("SBV subtitles"), qsTr("times kept")],
            ass: [qsTr("ASS subtitles"), qsTr("times and styles kept")], sub: [qsTr("SUB subtitles"), qsTr("frames kept")],
            ttml: [qsTr("TTML subtitles"), qsTr("times and styles kept")], stl: [qsTr("EBU STL subtitles"), qsTr("times and positions kept")],
            scc: [qsTr("SCC captions"), qsTr("times and positions kept")], txt: [qsTr("Text file"), qsTr("line breaks kept")],
            pdf: [qsTr("PDF document"), qsTr("text only, saved as a DOCX document")]
        }
        const n = names[format]
        return n ? n[0] + ", " + n[1] : ""
    }
    // The most common of a list, the first on a tie.
    function byCount(all) {
        const n = {}, order = []
        for (const x of all) { if (!x) continue; if (!(x in n)) { order.push(x); n[x] = 0 } n[x]++ }
        return order.sort((a, b) => n[b] - n[a])
    }

    // Opens the page for a new project, with any files and folders given.
    function start(urls) {
        if (typeof urls === "string") urls = urls ? [urls] : []
        step = 0
        placeChosen = false
        docs = []
        nameTouched = false
        autoName = ""
        sourceTouched = false
        startKind = "blank"
        startId = ""
        asTemplate = false
        templateName = ""
        checksOpen = false
        segmentationOpen = false
        previewData = []
        previewError = ""
        createError = ""
        creating = false
        form.setSettings({}, true)
        reloadWhere()
        if (urls && urls.length) addFiles(urls)
    }

    // Where the project is kept decides what it can use: the memories,
    // termbases, templates and earlier projects there. The files are read
    // there, too.
    function reloadWhere() {
        if (!conn || !conn.ready) cid = readyPlaces.length ? readyPlaces[0].id : ""
        defaults = ({}); templates = []; projects = []
        if (!cid) return
        loading = true
        const id = cid
        let left = 4, error = ""
        const done = () => {
            if (--left > 0 || id !== cid) return
            loading = false
            if (error) notice(error)
            applyStart()
            for (let i = 0; i < docs.length; i++) inspect(i)
            filesChanged()
        }
        Api.get(id, "/api/projects/defaults", r => { if (r.ok) defaults = r.data; else error = r.error.message; done() })
        Api.get(id, "/api/templates", r => { templates = r.ok ? r.data : []; done() })
        Api.get(id, "/api/projects", r => { projects = r.ok ? r.data : []; done() })
        form.load(id, e => { if (e) error = e; done() })
    }
    // The settings the project starts with: the defaults, or those of a
    // template or an earlier project, with what was typed here kept.
    function applyStart() {
        const had = form.settings()
        const st = form.copy(defaults)
        st.analysis = Object.assign({}, st.analysis || {}, { context: Backend.option("contextRule") || "previous" })
        let from = null
        if (startKind === "template") from = form.copy(templates.find(t => t.name === startId) || null)
        else if (startKind === "project") {
            const p = projects.find(p => String(p.id) === startId)
            if (p) {
                from = form.copy(p.settings)
                // Its memories, but not its own project memories, and a
                // project termbase of its own.
                const mems = from.memories || {}
                for (const lang of Object.keys(mems)) mems[lang] = (mems[lang] || []).filter(m => m.memory !== from.name + " (" + lang + ")")
                from.memories = mems
            }
        }
        if (from) {
            for (const key of ["client", "sourceLang", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare"])
                if (from[key] !== undefined) st[key] = from[key]
            const term = st.terminology || {}
            delete term.termbase
            delete term.addLanguages
            st.terminology = term
            sourceTouched = true
        } else {
            // A blank project keeps the languages chosen so far.
            st.sourceLang = had.sourceLang || ""
            if (had.targetLangs) st.targetLangs = had.targetLangs
            if (had.client) st.client = had.client
        }
        st.name = had.name || ""
        st.due = had.due || ""
        st.description = had.description || ""
        st.terminology = Object.assign({}, st.terminology || {}, { location: "server" })
        st.files = []
        form.setSettings(st, true)
        // Target languages of earlier projects are offered first.
        let recent = []
        for (const p of projects) recent = recent.concat((p.settings || {}).targetLangs || [])
        form.suggested = byCount(recent)
        filesChanged()
    }
    function choose(kind, id) {
        if (startKind === kind && startId === id) return
        startKind = kind
        startId = id
        applyStart()
    }
    function setStep(to) {
        if (to < 0 || to > 4 || to === step) return
        // A target language picked and not added yet is added on leaving its step.
        if (step === 1) targetsPart.addPending()
        step = to
        if (step === 4) preview()
    }

    // Files.
    function addFiles(urls) {
        const list = docs.slice()
        for (const f of Backend.filesOf(urls)) {
            if (list.some(d => d.name.toLowerCase() === f.name.toLowerCase())) continue
            const d = { path: f.path, name: f.name, size: f.size, inspection: {}, role: "reference", read: false }
            if (f.size > maxFile) { d.inspection = { problem: qsTr("The file is larger than 200 MB.") }; d.read = true }
            else if (f.problem) { d.inspection = { problem: f.problem }; d.read = true }
            list.push(d)
        }
        docs = list
        for (let i = 0; i < docs.length; i++) if (!docs[i].read) inspect(i)
        filesChanged()
    }
    function setDoc(path, change) {
        docs = docs.map(d => { if (d.path !== path) return d; const x = Object.assign({}, d); change(x); return x })
    }
    function inspect(i) {
        const d = docs[i]
        if (!cid || !d || d.inspection.problem) return
        const path = d.path, where = cid
        setDoc(path, x => { x.read = false })
        Backend.upload(cid, "POST", "/api/files/inspect", path, "application/octet-stream", { name: d.name }).done.connect(r => {
            if (where !== cid) return
            setDoc(path, x => {
                x.inspection = r.ok ? r.data : { problem: r.error.message }
                x.read = true
                x.role = !x.inspection.translatable ? "reference" : x.inspection.format === "package" ? "package" : "translatable"
            })
            filesChanged()
        })
    }
    function bilingualSources() {
        const out = []
        for (const d of docs) {
            if (d.role === "reference") continue
            const src = Backend.normalizeLanguage(((d.inspection.bilingual || {}).sourceLang) || ((d.inspection.package || {}).sourceLang) || "")
            if (src && out.indexOf(src) < 0) out.push(src)
        }
        return out
    }
    function fixedTargets() {
        const out = []
        for (const d of docs) {
            if (d.role === "reference") continue
            const ts = [((d.inspection.bilingual || {}).targetLang) || ""].concat((d.inspection.package || {}).targetLangs || [])
            for (const t of ts) { const n = Backend.normalizeLanguage(t); if (n && out.indexOf(n) < 0) out.push(n) }
        }
        return out
    }
    // What the text of the files says, the surest of them.
    function detection() {
        let best = null
        for (const d of docs) {
            if (d.role !== "translatable") continue
            const det = d.inspection.detection
            if (det && (!best || det.confidence > best.confidence)) best = det
        }
        return best
    }
    // What follows the files until the person sets it: the project's name,
    // its source language and the target languages bilingual files need.
    function filesChanged() {
        if (!nameTouched) {
            let name = ""
            if (docs.length) {
                const pkg = (docs[0].inspection.package || {}).name || ""
                name = nameFromFile(pkg || docs[0].name)
            }
            autoName = name
            if ((form.s.name || "") !== name) form.put("name", name)
        }
        const said = bilingualSources()
        const best = detection()
        const fallback = byCount(projects.map(p => (p.settings || {}).sourceLang))[0] || ""
        if (said.length === 1) { if (form.source !== said[0]) form.put("sourceLang", said[0]) }
        else if (!sourceTouched) {
            const guess = best ? best.lang || "" : ""
            if ((guess || fallback) && form.source !== (guess || fallback)) form.put("sourceLang", guess || fallback)
        }
        const fixed = fixedTargets()
        let targets = form.targets.filter(t => t !== form.source)
        for (const t of fixed) if (targets.indexOf(t) < 0) targets.push(t)
        form.fixed = fixed
        if (JSON.stringify(targets) !== JSON.stringify(form.targets)) form.setTargets(targets)
        if (step === 4) previewTimer.restart()
    }

    readonly property bool reading: docs.some(d => !d.read)
    readonly property bool anyToTranslate: docs.some(d => d.role !== "reference")
    readonly property var fileProblems: {
        const out = []
        const sources = bilingualSources()
        if (sources.length > 1)
            out.push(Fmt.fill(qsTr("The bilingual files have different source languages ({languages}). A project has one source language."), { languages: sources.map(l => Backend.languageName(l)).join(", ") }))
        if (docs.filter(d => d.role === "package").length > 1)
            out.push(qsTr("A project holds the files of one package, so it can be saved back as one return package."))
        const f = form.problemsAt("files")
        if (f) out.push(f)
        return out
    }
    readonly property bool filesReady: !!cid && !loading && anyToTranslate && !reading && fileProblems.length === 0 && !!form.source
                                        && (form.targets.length > 0 || targetsPart.pending !== "") && !!(form.s.name || "").trim()
    readonly property var pageProblems: {
        if (!asTemplate) return []
        if (!templateName.trim()) return [qsTr("Give the template a name.")]
        if (templates.some(t => t.name.toLowerCase() === templateName.trim().toLowerCase())) return [qsTr("A template with this name exists already.")]
        return []
    }
    readonly property var formProblems: {
        const all = []
        for (const f of ["name", "client", "due", "files", "sourceLang", "targetLangs", "memories", "terminology", "qa", "analysis", "prepare"]) {
            const p = form.problemsAt(f)
            for (const line of p ? p.split("\n") : []) if (all.indexOf(line) < 0) all.push(line)
        }
        return all
    }

    // The request that makes the project.
    function request() {
        const st = form.settings()
        const name = st.name, source = st.sourceLang
        let newMemories = form.newMemories()
        // The project's own memories come first for each language, so they
        // win ties.
        if (st.projectMemory) {
            const mems = st.memories || {}
            for (const t of st.targetLangs || []) {
                const mname = name + " (" + t + ")"
                newMemories.push({ Name: mname, SourceLang: source, TargetLang: t })
                mems[t] = [{ memory: mname, lookup: true, concordance: true, update: true, penalty: 0 }].concat(mems[t] || [])
            }
            st.memories = mems
        }
        const req = { memories: newMemories }
        if ((st.terminology || {}).mode !== "direct") req.termbase = { name: name + " (project)" }
        req.files = docs.filter(d => !d.inspection.problem || d.read).map(d => {
            const f = { name: d.name, role: d.role, data: Backend.base64File(d.path) }
            if (d.role === "reference") f.format = (d.name.split(".").pop() || "").toLowerCase()
            return f
        }).filter(f => f.data !== "")
        st.files = []
        req.settings = st
        return req
    }
    function fieldsOf(problems) { return (problems || []).map(p => ({ field: p.field.startsWith("settings.") ? p.field.slice(9) : p.field, message: p.message })) }
    // Asks the core what the project would be: the setup's problems, the
    // analysis of the files, and what pre-translation would fill.
    function preview() {
        if (!cid || step !== 4 || creating) return
        previewing = true
        const seq = ++previewSeq
        Api.post(cid, "/api/projects/preview", request(), r => {
            if (seq !== previewSeq) return
            previewing = false
            previewError = ""
            createError = ""
            let ps = []
            if (r.ok) previewData = r.data
            else {
                previewData = []
                ps = fieldsOf(r.error.problems)
                if (!ps.length) previewError = r.error.message
            }
            form.showProblems(ps)
            // What pre-translation would do, with the preparation.
            const said = previewData.map(p => Fmt.fill(qsTr("{language}: {filled} filled, {confirmed} confirmed"),
                                                       { language: Backend.languageName(p.lang), filled: Fmt.plural(p.filled || 0, qsTr("{n} segment"), qsTr("{n} segments")), confirmed: Fmt.num(p.confirmed || 0) }))
            form.summary = said.length && (form.s.prepare || {}).pretranslate ? said.join(". ") + "." : ""
        })
    }
    function create() {
        if (!cid || creating) return
        creating = true
        createError = ""
        const req = request()
        const st = req.settings, id = cid
        Api.post(cid, "/api/projects", req, r => {
            if (!r.ok) {
                creating = false
                const ps = fieldsOf(r.error.problems)
                form.showProblems(ps)
                createError = ps.length ? "" : r.error.message
                return
            }
            const project = r.data.id
            // What follows the project: the memories filled from TMX files,
            // the template, and pre-translation. The project is made either
            // way; what went wrong is said on the project's page.
            form.importNewMemories(() => {
                const finish = () => {
                    if (!(st.prepare || {}).pretranslate) { creating = false; page.created(id, project); return }
                    Api.post(id, "/api/projects/" + project + "/pretranslate", {}, () => { creating = false; page.created(id, project) })
                }
                if (!asTemplate) { finish(); return }
                Api.put(id, "/api/templates", form.templateOf(templateName.trim(), form.settings()), () => finish(), 0)
            })
        })
    }

    // Reading again when where the project can be kept changes.
    Connections {
        target: Backend
        function onConnectionsChanged() { if (!page.conn || !page.conn.ready) page.reloadWhere() }
    }
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() {
                // This computer comes first once it is ready, unless the person chose another place.
                if (modelData.local && modelData.ready && !page.placeChosen && page.cid !== modelData.id) { page.cid = modelData.id; page.reloadWhere() }
                else if (!page.cid || (page.cid === modelData.id && !modelData.ready)) page.reloadWhere()
            }
        }
    }

    Stepper {
        steps: [qsTr("Start"), qsTr("Files and languages"), qsTr("Memories"), qsTr("Terminology"), qsTr("Review")]
        current: page.step
        clickable: true
        onStepClicked: i => page.setStep(i)
    }

    // Start.
    Panel {
        visible: page.step === 0
        title: qsTr("Start")
        spacing: Theme.s4
        Txt {
            visible: !page.cid
            Layout.fillWidth: true
            // State 0: the core is starting.
            role: Backend.local.state === 0 ? "muted" : "error"
            text: Backend.local.state === 0 ? qsTr("Starting the work on this computer…")
                                                                    : qsTr("Nowhere to keep a project: work on this computer can't be opened and no team server is signed in.")
        }
        ColumnLayout {
            visible: !!page.cid
            Layout.fillWidth: true
            spacing: Theme.s2
            Txt { role: "label"; text: qsTr("Start from") }
            Choice {
                Layout.fillWidth: true
                text: qsTr("A blank project")
                description: qsTr("Languages from the files, memories and termbases suggested for the client.")
                checked: page.startKind === "blank"
                onToggled: if (checked) page.choose("blank", "")
            }
            Repeater {
                model: page.templates
                Choice {
                    required property var modelData
                    Layout.fillWidth: true
                    text: Fmt.fill(qsTr("Template: {name}"), { name: modelData.name })
                    description: {
                        let memories = 0
                        for (const k in (modelData.memories || {})) memories += modelData.memories[k].length
                        const parts = []
                        if (modelData.client) parts.push(modelData.client)
                        parts.push((modelData.sourceLang || "") + " → " + (modelData.targetLangs || []).join(" "))
                        parts.push(Fmt.plural(memories, qsTr("{n} memory"), qsTr("{n} memories")))
                        return parts.join(", ")
                    }
                    checked: page.startKind === "template" && page.startId === modelData.name
                    onToggled: if (checked) page.choose("template", modelData.name)
                }
            }
            Choice {
                visible: page.projects.length > 0
                Layout.fillWidth: true
                text: qsTr("Like an earlier project")
                description: qsTr("Its languages, memories, termbases and checks; new files and a new name.")
                checked: page.startKind === "project"
                onToggled: if (checked) page.choose("project", String(page.projects[0].id))
                RowLayout {
                    visible: page.startKind === "project"
                    width: parent.width
                    spacing: Theme.s2
                    Txt { text: qsTr("Project") }
                    ComboBox {
                        Layout.fillWidth: true
                        model: page.projects.map(p => p.settings.client ? p.settings.name + " (" + p.settings.client + ")" : p.settings.name)
                        currentIndex: Math.max(0, page.projects.findIndex(p => String(p.id) === page.startId))
                        Accessible.name: qsTr("Project")
                        onActivated: i => page.choose("project", String(page.projects[i].id))
                    }
                }
            }
            Txt {
                visible: page.templates.length === 0
                Layout.fillWidth: true
                role: "help"
                text: qsTr("You can save any project's settings as a template, here at the end or later from the project's settings.")
            }
        }
        // Where it is kept, when there is more than one place.
        ColumnLayout {
            visible: page.readyPlaces.length > 1
            Layout.fillWidth: true
            spacing: Theme.s2
            Txt { role: "label"; text: qsTr("Where the project is kept") }
            Repeater {
                model: page.readyPlaces
                Choice {
                    required property var modelData
                    Layout.fillWidth: true
                    text: modelData.local ? qsTr("On this computer") : Fmt.fill(qsTr("On the team server {server}"), { server: modelData.displayName })
                    description: modelData.local ? qsTr("Only you work on it, with the memories and termbases kept on this computer.")
                                                 : qsTr("Shared: you give work to translators and reviewers there, and everyone's translations are saved on the server. It uses the server's memories and termbases.")
                    checked: modelData.id === page.cid
                    onToggled: {
                        if (!checked || modelData.id === page.cid) return
                        page.cid = modelData.id
                        page.placeChosen = true
                        // Templates and earlier projects are those of the place.
                        page.startKind = "blank"
                        page.startId = ""
                        page.reloadWhere()
                    }
                }
            }
        }
        StepActions {
            Button { kind: "primary"; text: qsTr("Continue to files and languages"); enabled: !!page.cid && !page.loading; onClicked: page.setStep(1) }
        }
    }

    // Files and languages.
    Panel {
        visible: page.step === 1
        title: qsTr("Files and languages")
        spacing: Theme.s4
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            Txt { role: "label"; text: qsTr("Files") }
            Table {
                visible: page.docs.length > 0
                name: qsTr("Files")
                columns: [{ title: qsTr("File"), fill: 1 }, { title: qsTr("Words"), width: 80, align: "right" }, { title: qsTr("Use"), width: 140 }, { title: "", width: 48 }]
                Repeater {
                    model: page.docs
                    TableRow {
                        id: frow
                        required property var modelData
                        required property int index
                        readonly property var d: modelData
                        readonly property var inn: d.inspection || {}
                        readonly property bool canTranslate: !!inn.translatable
                        last: index === page.docs.length - 1
                        ColumnLayout {
                            spacing: Theme.s1
                            CellTitle {
                                Layout.fillWidth: true
                                title: frow.d.name
                                sub: {
                                    if (!frow.d.read) return qsTr("Reading…")
                                    const pkg = frow.inn.package, b = frow.inn.bilingual
                                    if (pkg) return Fmt.fill(qsTr("{kind}, {source} to {targets}, {files}"), {
                                        kind: page.packageKind(frow.d.name), source: Backend.languageName(Backend.normalizeLanguage(pkg.sourceLang || "")),
                                        targets: Fmt.listOf((pkg.targetLangs || []).map(t => Backend.languageName(Backend.normalizeLanguage(t)))),
                                        files: Fmt.plural((pkg.files || []).length, qsTr("{n} file"), qsTr("{n} files")) })
                                    if (b) {
                                        let sub = page.bilingualName(b.format)
                                        if (b.targetLang && b.sourceLang)
                                            sub += Fmt.fill(qsTr(", {source} to {target}"), { source: Backend.languageName(Backend.normalizeLanguage(b.sourceLang)), target: Backend.languageName(Backend.normalizeLanguage(b.targetLang)) })
                                        return sub + Fmt.fill(qsTr(", {translated} of {segments} translated"), { translated: Fmt.num(b.translated || 0), segments: Fmt.plural(frow.inn.segments || 0, qsTr("{n} segment"), qsTr("{n} segments")) })
                                    }
                                    return page.documentFormatText(frow.inn.format || "") || qsTr("No text Interlinea can read")
                                }
                                HoverHandler { id: tipHover }
                                ToolTip { visible: tipHover.hovered; text: frow.d.path }
                            }
                            Repeater { model: frow.inn.warnings || []; Note { required property string modelData; kind: "warn"; text: modelData } }
                            Note {
                                visible: frow.d.read && !frow.canTranslate
                                kind: "warn"
                                text: Fmt.fill(qsTr("{why} It is added as a reference file."), {
                                    why: frow.inn.problem ? Fmt.fill(qsTr("It could not be read: {problem}"), { problem: frow.inn.problem })
                                                          : qsTr("Interlinea can't read text from this file (for example a scan, or a format it has no filter for). It can travel with the project for reference.") })
                            }
                        }
                        Txt { horizontalAlignment: Text.AlignRight; text: frow.inn.words ? Fmt.num(frow.inn.words) : "–" }
                        ComboBox {
                            implicitHeight: 28
                            model: [qsTr("Translate"), qsTr("Reference")]
                            currentIndex: frow.d.role === "reference" ? 1 : 0
                            enabled: frow.d.read && frow.canTranslate
                            Accessible.name: Fmt.fill(qsTr("Use of {file}"), { file: frow.d.name })
                            onActivated: i => {
                                page.setDoc(frow.d.path, x => { x.role = i === 1 ? "reference" : x.inspection.format === "package" ? "package" : "translatable" })
                                page.filesChanged()
                            }
                        }
                        IconButton {
                            iconName: "x"
                            tip: Fmt.fill(qsTr("Remove {file}"), { file: frow.d.name })
                            onClicked: { const p = frow.d.path; page.docs = page.docs.filter(x => x.path !== p); page.filesChanged() }
                        }
                    }
                }
            }
            DropZone {
                Accessible.name: qsTr("Add files")
                onDropped: urls => page.addFiles(urls)
                Icon { name: "upload"; color: Theme.text2; size: 18; Layout.alignment: Qt.AlignVCenter }
                Txt {
                    Layout.fillWidth: true
                    role: "muted"
                    text: qsTr("Drop files or folders here. DOCX, PPTX, XLSX, IDML and web page files keep their formatting; of a PDF the text is read, and its translation is a DOCX document; bilingual files (.sdlxliff, .mqxliff, .mqxlz, .mxliff, .txml, .ttx, .xlf) and packages (.sdlppx, .mqout) keep their translations and states, and so do bilingual tables for review in .docx and .rtf.")
                }
                Button { small: true; iconName: "file-text"; text: qsTr("Add files"); Layout.alignment: Qt.AlignVCenter; onClicked: filesDialog.open() }
                Button { small: true; iconName: "folder-open"; text: qsTr("Add a folder"); Layout.alignment: Qt.AlignVCenter; onClicked: folderDialog.open() }
            }
            Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Reference files (a style guide, an earlier version) travel with the project for the translators to read; they are not translated.") }
            Txt { visible: page.fileProblems.length > 0; Layout.fillWidth: true; role: "error"; text: page.fileProblems.join("\n") }
        }

        // The source language.
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            readonly property var said: page.bilingualSources()
            readonly property var det: page.detection()
            readonly property bool any: page.docs.some(d => d.role === "translatable")
            Txt { role: "label"; text: qsTr("Source language") }
            RowLayout {
                visible: parent.said.length === 1
                spacing: Theme.s2
                Txt { text: Backend.languageName(form.source) }
                LangChip { code: form.source }
            }
            Txt { visible: parent.said.length === 1; role: "help"; text: qsTr("From the bilingual files, which are made for it.") }
            Note {
                visible: parent.said.length !== 1 && parent.any
                readonly property var det: parent.det || {}
                readonly property string code: det.lang || ""
                readonly property real conf: det.confidence || 0
                readonly property int pct: Math.round(conf * 100)
                readonly property int basis: det.basis || 0
                readonly property string src: form.source
                readonly property var said: {
                    if (page.sourceTouched || page.startKind !== "blank") {
                        const differs = code && conf >= page.confident && !form.samePrimary(code, src)
                        return { kind: differs ? "warn" : "info", text: Fmt.fill(qsTr("Set to {language}."), { language: Backend.languageName(src) })
                                 + (differs ? " " + Fmt.fill(qsTr("The files look {language} ({percent}%). Check the source language."), { language: Backend.languageName(code), percent: pct }) : "") }
                    }
                    if (det.reason === "empty")
                        return { kind: "warn", text: !src ? qsTr("No text found in the files (for example a scanned PDF), so the language can't be detected. Choose the source language.")
                                                          : Fmt.fill(qsTr("No text found in the files (for example a scanned PDF), so the language can't be detected. Preselected {language}, the source language of most of your projects. Change it if it's wrong."), { language: Backend.languageName(src) }) }
                    if (det.reason === "short" || !code) {
                        const little = Fmt.fill(qsTr("Too little text to tell ({words})."), { words: Fmt.plural(basis, qsTr("{n} word"), qsTr("{n} words")) })
                        return { kind: "warn", text: little + " " + (!src ? qsTr("Choose the source language.") : Fmt.fill(qsTr("Preselected {language}, the source language of most of your projects."), { language: Backend.languageName(src) })) }
                    }
                    if (conf < page.confident) {
                        const alt = (det.alternatives || []).map(a => Backend.languageName(a.lang) + " " + Math.round(a.confidence * 100) + "%")
                        return { kind: "warn", text: alt.length ? Fmt.fill(qsTr("Probably {language} ({percent}%), but it could also be {others}. Please check."), { language: Backend.languageName(code), percent: pct, others: alt.join(", ") })
                                                                : Fmt.fill(qsTr("Probably {language} ({percent}%). Please check."), { language: Backend.languageName(code), percent: pct }) }
                    }
                    return { kind: "ok", text: Fmt.fill(qsTr("Detected {language}, {percent}% confident, from {basis} of the files."), {
                        language: Backend.languageName(code), percent: pct,
                        basis: det.unit === "characters" ? Fmt.plural(basis, qsTr("{n} character"), qsTr("{n} characters")) : Fmt.plural(basis, qsTr("{n} word"), qsTr("{n} words")) }) }
                }
                kind: said.kind
                text: said.text
            }
            LanguageField {
                id: sourceField
                visible: parent.said.length !== 1
                implicitWidth: 320
                Accessible.name: qsTr("Source language")
                Component.onCompleted: pick(form.source)
                onLanguageChanged: {
                    if (!activeFocus || !language || language === form.source) return
                    page.sourceTouched = true
                    form.put("sourceLang", language)
                    page.filesChanged()
                }
                // What the files say shows here until the person types.
                Connections { target: form; function onSChanged() { if (!sourceField.activeFocus && sourceField.currentValue !== form.source) sourceField.pick(form.source) } }
            }
            Txt { visible: parent.said.length !== 1; Layout.fillWidth: true; role: "help"; text: qsTr("Text shows the language, not the region. Check the variant (for example German (Austria)) if the client needs it.") }
            Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: form.problemsAt("sourceLang") }
        }

        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            Txt { role: "label"; text: qsTr("Target languages") }
            FormTargets { id: targetsPart; form: form }
            Txt { visible: form.targets.length > 1; Layout.fillWidth: true; role: "help"; text: qsTr("Each file is translated into every target language; a bilingual file only into the language it was made for.") }
        }
        FormDetails { form: form }
        StepActions {
            Button { kind: "quiet"; text: qsTr("Back"); onClicked: page.setStep(0) }
            Button {
                kind: "primary"
                text: qsTr("Continue to memories")
                enabled: page.filesReady
                ToolTip {
                    visible: parent.hovered && !page.filesReady
                    text: !page.anyToTranslate ? qsTr("Add a file to translate first.") : page.reading ? qsTr("The files are still being read.")
                        : qsTr("Give the project a name, its source language and a target language first.")
                }
                onClicked: page.setStep(2)
            }
        }
    }

    // Memories.
    Panel {
        visible: page.step === 2
        title: qsTr("Memories")
        spacing: Theme.s4
        FormMemories { form: form }
        StepActions {
            Button { kind: "quiet"; text: qsTr("Back"); onClicked: page.setStep(1) }
            Button { kind: "primary"; text: qsTr("Continue to terminology"); enabled: page.filesReady; onClicked: page.setStep(3) }
        }
    }

    // Terminology.
    Panel {
        visible: page.step === 3
        title: qsTr("Terminology")
        spacing: Theme.s4
        FormTerminology { form: form }
        StepActions {
            Button { kind: "quiet"; text: qsTr("Back"); onClicked: page.setStep(2) }
            Button { kind: "primary"; text: qsTr("Continue to review"); enabled: page.filesReady; onClicked: page.setStep(4) }
        }
    }

    // Review.
    Panel {
        visible: page.step === 4
        title: qsTr("Review")
        spacing: Theme.s4
        readonly property var st: form.s
        GridLayout {
            Layout.fillWidth: true
            columns: 2
            columnSpacing: Theme.s5
            rowSpacing: Theme.s2
            Accessible.name: qsTr("Summary")
            readonly property string grey: Theme.text3
            function esc(t) { return (t || "").replace(/&/g, "&amp;").replace(/</g, "&lt;") }
            function muted(t) { return "<font color=\"" + grey + "\">" + esc(t) + "</font>" }

            Txt { Layout.alignment: Qt.AlignTop; role: "muted"; text: qsTr("Project") }
            Txt {
                Layout.fillWidth: true
                textFormat: Text.StyledText
                text: parent.esc(form.s.name) + (form.s.client ? parent.muted(Fmt.fill(qsTr(" for {client}"), { client: form.s.client })) : "")
                      + (form.s.due ? parent.muted(Fmt.fill(qsTr(", due {date}"), { date: Fmt.time(form.s.due) })) : "")
            }
            Txt { Layout.alignment: Qt.AlignTop; role: "muted"; text: qsTr("Files") }
            Txt {
                Layout.fillWidth: true
                textFormat: Text.StyledText
                text: {
                    let translatable = 0, reference = 0, pkg = ""
                    for (const d of page.docs) {
                        if (d.role === "reference") reference++
                        else if (d.role === "package") { pkg = d.name; translatable += ((d.inspection.package || {}).files || []).length }
                        else translatable++
                    }
                    let t = parent.esc(Fmt.fill(qsTr("{files} to translate"), { files: Fmt.plural(translatable, qsTr("{n} file"), qsTr("{n} files")) }))
                    if (reference) t += ", " + parent.esc(Fmt.plural(reference, qsTr("{n} reference file"), qsTr("{n} reference files")))
                    if (pkg) t += parent.muted(Fmt.fill(qsTr(" from the {kind} {name}"), { kind: page.packageKind(pkg), name: pkg }))
                    return t
                }
            }
            Txt { Layout.alignment: Qt.AlignTop; role: "muted"; text: qsTr("Languages") }
            Txt {
                Layout.fillWidth: true
                text: Fmt.fill(qsTr("{source} into {targets}"), { source: Backend.languageName(form.source), targets: form.targets.map(t => Backend.languageName(t)).join(", ") })
            }
            Txt { Layout.alignment: Qt.AlignTop; role: "muted"; text: qsTr("Memories") }
            Txt {
                Layout.fillWidth: true
                textFormat: Text.StyledText
                text: form.targets.map(t => {
                    const names = form.memoriesFor(t).map(m => m.penalty ? m.memory + " (−" + m.penalty + ")" : m.memory)
                    return "<b>" + parent.esc(t) + "</b> " + parent.esc(names.length ? names.join(", ") : qsTr("none")) + (form.s.projectMemory ? parent.muted(qsTr(", and a project memory")) : "")
                }).join("<br>")
            }
            Txt { Layout.alignment: Qt.AlignTop; role: "muted"; text: qsTr("Terminology") }
            Txt {
                Layout.fillWidth: true
                text: {
                    const term = form.s.terminology || {}
                    const masters = term.masters || []
                    const kept = page.conn && !page.conn.local ? Fmt.fill(qsTr("on {server}"), { server: page.conn.displayName }) : qsTr("on this computer")
                    if (term.mode === "direct") return Fmt.fill(qsTr("New terms go straight into {master}."), { master: term.writeTo || masters.join(", ") })
                    if (term.mode === "layered") {
                        let t = Fmt.fill(qsTr("A new project termbase {where}, based on {masters}."), { where: kept, masters: masters.join(", ") }) + " "
                              + (term.autoUpdate ? qsTr("Takes in master changes each time the project opens.") : qsTr("Master changes wait until you take them in."))
                        if ((term.addLanguages || []).length) t += " " + Fmt.fill(qsTr("Languages are added to {masters}."), { masters: term.addLanguages.join(", ") })
                        return t
                    }
                    return Fmt.fill(qsTr("A new project termbase {where}, no master."), { where: kept })
                }
            }
        }

        // Checks and context matches, folded away until asked for.
        RowLayout {
            spacing: Theme.s3
            Button {
                kind: "quiet"
                leftPadding: 0
                iconName: page.checksOpen ? "chevron-down" : "chevron-right"
                text: qsTr("Checks and context matches")
                Accessible.name: page.checksOpen ? qsTr("Hide the checks and context matches") : qsTr("Show the checks and context matches")
                onClicked: page.checksOpen = !page.checksOpen
            }
            Txt {
                role: "muted"
                text: Fmt.fill(qsTr("{on} of {of} checks on, context matches {how}"), { on: form.checksOn, of: form.checksOf,
                               how: ((form.s.analysis || {}).context === "both") ? qsTr("by the segments before and after") : qsTr("by the segment before") })
            }
        }
        FormChecks { visible: page.checksOpen; form: form }
        // Segmentation, folded away until asked for.
        RowLayout {
            spacing: Theme.s3
            Button {
                kind: "quiet"
                leftPadding: 0
                iconName: page.segmentationOpen ? "chevron-down" : "chevron-right"
                text: qsTr("Segmentation")
                Accessible.name: page.segmentationOpen ? qsTr("Hide the segmentation rules") : qsTr("Show the segmentation rules")
                onClicked: page.segmentationOpen = !page.segmentationOpen
            }
            Txt {
                role: "muted"
                readonly property int own: (form.s.segmentation || []).length
                text: own === 0 ? qsTr("as usual for the language") : Fmt.plural(own, qsTr("{n} rule of your own"), qsTr("{n} rules of your own"))
            }
        }
        FormSegmentation { visible: page.segmentationOpen; form: form }
        FormPrepare { form: form }

        // The match analysis.
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            readonly property var langs: page.previewData.map(p => p.lang)
            readonly property string shown: langs.indexOf(page.reviewLang) >= 0 ? page.reviewLang : (langs[0] || "")
            readonly property var pv: page.previewData.find(p => p.lang === shown) || null
            Txt { role: "label"; text: qsTr("Match analysis") }
            Txt {
                visible: page.previewData.length === 0
                role: "muted"
                text: page.previewing ? qsTr("Analysing the files…") : qsTr("The analysis follows once the project can be created.")
            }
            Tabs {
                visible: parent.langs.length > 1
                Layout.fillWidth: true
                inset: 0
                small: true
                name: qsTr("Target language")
                model: parent.langs.map(l => Backend.languageName(l))
                current: Math.max(0, parent.langs.indexOf(parent.shown))
                onChanged: i => page.reviewLang = parent.langs[i]
            }
            Txt {
                visible: !!parent.pv
                Layout.fillWidth: true
                role: "help"
                text: !parent.pv ? "" : (parent.pv.memories || []).length === 0 ? qsTr("No memory for this language, so only repetitions count.")
                    : Fmt.fill(qsTr("Against {memories}, counted in this order: context matches first, then repetitions within a file and across files, then the best match."), { memories: parent.pv.memories.join(qsTr(" and ")) })
            }
            AnalysisTable {
                visible: !!parent.pv
                report: parent.pv ? parent.pv.report || {} : {}
                name: Fmt.fill(qsTr("Match analysis, {language}"), { language: Backend.languageName(parent.shown) })
            }
        }

        // Saving the settings as a template.
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            CheckBox {
                text: qsTr("Save these settings as a template")
                checked: page.asTemplate
                onToggled: {
                    page.asTemplate = checked
                    if (checked && !page.templateName) page.templateName = form.s.client || form.s.name || ""
                }
            }
            Field {
                visible: page.asTemplate
                Layout.leftMargin: 26
                label: qsTr("Template name")
                help: qsTr("Languages, memories, termbases, checks and pre-translation; not the files, name or due date.")
                error: page.pageProblems.join("\n")
                TextField { text: page.templateName; invalid: page.pageProblems.length > 0; onTextEdited: page.templateName = text }
            }
        }

        // Why the project can't be made yet.
        Note {
            visible: page.formProblems.length > 0 || page.previewError !== "" || page.createError !== ""
            kind: "warn"
            text: page.createError ? Fmt.fill(qsTr("The project was not made: {error}"), { error: page.createError })
                : page.previewError ? Fmt.fill(qsTr("The project can't be checked: {error}"), { error: page.previewError })
                : qsTr("The project can't be created yet") + "\n• " + page.formProblems.join("\n• ")
        }
        StepActions {
            Button { kind: "quiet"; text: qsTr("Back"); onClicked: page.setStep(3) }
            Button {
                kind: "primary"
                text: page.creating ? qsTr("Creating…") : page.conn && !page.conn.local ? qsTr("Create on the server") : qsTr("Create project")
                enabled: !page.previewing && !page.creating && page.previewError === "" && page.previewData.length > 0 && page.formProblems.length === 0 && page.pageProblems.length === 0
                onClicked: page.create()
            }
        }
    }

    FileDialog {
        id: filesDialog
        title: qsTr("Files for the project")
        fileMode: FileDialog.OpenFiles
        onAccepted: page.addFiles(Array.from(selectedFiles))
    }
    FolderDialog {
        id: folderDialog
        title: qsTr("A folder of files")
        onAccepted: page.addFiles([selectedFolder])
    }
}
