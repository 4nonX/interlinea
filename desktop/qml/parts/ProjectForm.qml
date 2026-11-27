import QtQuick
import Interlinea

// A project's settings while they are made or changed (New project, and
// a project's Settings): what the connection offers (memories, termbases,
// dictionaries, checks, grammar services), the settings themselves, the
// memories to make with the project, and the problems the core found.
// The sections (FormDetails, FormTargets, FormMemories, FormTerminology,
// FormChecks, FormPrepare, FormSegmentation) show and change it.
QtObject {
    id: form
    property string cid: ""
    property var s: ({})
    property bool isNew: false
    property bool readOnly: false
    property var catalog: ({})
    property var dicts: []
    property var kinds: []
    property var grammar: []
    property var problems: []
    // Languages the files suggest, and those a bilingual file fixes.
    property var suggested: []
    property var fixed: []
    // Memories made with the project: { name, sourceLang, targetLang, tmx }.
    property var created: []
    property string creatingFor: ""
    property bool checksTouched: false
    property string summary: ""
    signal changed()

    readonly property var conn: Backend.connection(cid)
    readonly property string host: conn && !conn.local ? conn.displayName : ""
    readonly property string source: s.sourceLang || ""
    readonly property var targets: s.targetLangs || []

    function copy(o) { return JSON.parse(JSON.stringify(o === undefined ? null : o)) }
    function samePrimary(a, b) { return (a || "").split("-")[0].toLowerCase() === (b || "").split("-")[0].toLowerCase() }

    // What the connection offers, read before the form is shown.
    function load(connectionId, done) {
        cid = connectionId
        let left = 4, error = ""
        const got = {}
        const finish = () => {
            if (--left > 0) return
            if (!error) { catalog = got.catalog; dicts = got.dicts; kinds = got.kinds; grammar = got.grammar }
            done(error)
        }
        Api.get(cid, "/api/catalog", r => { if (r.ok) got.catalog = r.data; else error = r.error.message; finish() })
        Api.get(cid, "/api/dictionaries", r => { if (r.ok) got.dicts = r.data; else error = r.error.message; finish() })
        Api.get(cid, "/api/providers", r => {
            if (r.ok) got.grammar = r.data.filter(p => p.kind === "check").map(p => p.name)
            else error = r.error.message
            finish()
        })
        Api.get(cid, "/api/qa/kinds", r => { if (r.ok) got.kinds = r.data; else error = r.error.message; finish() })
    }
    function setSettings(settings, asNew) {
        isNew = asNew
        checksTouched = false
        problems = []
        created = []
        creatingFor = ""
        const next = copy(settings)
        s = next
        pickDictionaries()
    }
    // The settings to send: trimmed, with memories and dictionaries only
    // for the target languages, and terminology as its mode needs it.
    function settings() {
        const out = copy(s)
        out.name = (out.name || "").trim()
        out.client = (out.client || "").trim()
        out.description = (out.description || "").trim()
        const mems = {}
        for (const t of targets) mems[t] = (out.memories || {})[t] || []
        out.memories = mems
        const qa = out.qa || {}
        const dicts = {}
        for (const t of targets) if ((qa.dictionaries || {})[t]) dicts[t] = qa.dictionaries[t]
        qa.dictionaries = dicts
        out.qa = qa
        const term = out.terminology || {}
        if (term.mode === "own") term.masters = []
        if (term.mode !== "direct") delete term.writeTo
        if (term.mode !== "layered") term.autoUpdate = false
        // The app's project termbases are kept where the project is.
        term.location = "server"
        out.terminology = term
        return out
    }
    // A change: the key set to a value, or a part changed by a function.
    function put(key, value) {
        const next = copy(s)
        if (value === undefined) delete next[key]
        else next[key] = value
        s = next
        changed()
    }
    function edit(key, change) {
        const next = copy(s)
        const part = next[key] === undefined || next[key] === null ? {} : next[key]
        change(part)
        next[key] = part
        s = next
        changed()
    }
    function setTargets(list) {
        const next = copy(s)
        next.targetLangs = list
        s = next
        dropStaleMemories()
        changed()
    }
    function addTarget(lang) {
        if (!lang || targets.indexOf(lang) >= 0 || lang === source) return
        setTargets(targets.concat([lang]))
        pickDictionaries()
    }

    // Problems the core found, by the field they are about.
    function problemsAt(field) {
        return problems.filter(p => p.field === field || p.field.startsWith(field + ".") || p.field.startsWith(field + "[")).map(p => p.message).join("\n")
    }
    // The fields the sections show problems under.
    function shownFields() {
        const out = ["name", "client", "due", "targetLangs", "terminology.masters", "terminology.addLanguages", "terminology.mode", "terminology.writeTo",
                     "qa.subtitles", "qa.words", "qa.grammar", "prepare.minScore"]
        for (const t of targets) out.push("memories." + t, "qa.dictionaries." + t)
        for (let i = 0; i < ((s.qa || {}).rules || []).length; i++) out.push("qa.rules[" + i + "]")
        for (let i = 0; i < (s.segmentation || []).length; i++) out.push("segmentation[" + i + "]")
        return out
    }
    // Shows the problems; returns those no section shows.
    function showProblems(list) {
        problems = list || []
        const shown = shownFields()
        return problems.filter(p => !shown.some(f => p.field === f || p.field.startsWith(f + ".") || p.field.startsWith(f + "["))).map(p => p.message)
    }

    // The dictionaries that spell a language: the same region first, then
    // the language; those installed before those that come with Interlinea.
    function dictionariesFor(lang) {
        const out = [[], [], [], []]
        for (const d of dicts) {
            if (!samePrimary(d.lang, lang)) continue
            out[(d.lang === lang ? 0 : 2) + (d.included ? 1 : 0)].push(d)
        }
        return out[0].concat(out[1], out[2], out[3])
    }
    // A new project spells each target language with the dictionary that
    // fits it, until the checks are changed by hand.
    function pickDictionaries() {
        if (!isNew || checksTouched) return
        const qa = copy(s.qa || {})
        const chosen = qa.dictionaries || {}
        let changedAny = false
        for (const t of targets) {
            const set = chosen[t] || ""
            if (set && dicts.some(d => d.name === set)) continue
            const fit = dictionariesFor(t)
            if (!fit.length) continue
            chosen[t] = fit[0].name
            changedAny = true
        }
        if (!changedAny) return
        qa.dictionaries = chosen
        const next = copy(s)
        next.qa = qa
        s = next
    }
    // Languages no longer targets lose their memories and dictionaries.
    function dropStaleMemories() {
        const next = copy(s)
        const mems = next.memories || {}
        for (const k of Object.keys(mems)) if (targets.indexOf(k) < 0) delete mems[k]
        next.memories = mems
        const qa = next.qa || {}
        const d = qa.dictionaries || {}
        for (const k of Object.keys(d)) if (targets.indexOf(k) < 0) delete d[k]
        qa.dictionaries = d
        next.qa = qa
        s = next
        pickDictionaries()
        // A memory made here for a language that is gone is not made.
        created = created.filter(n => targets.indexOf(n.targetLang) >= 0)
        if (targets.indexOf(creatingFor) < 0) creatingFor = ""
    }

    function memoryInfo(name) {
        const m = (catalog.memories || []).find(x => x.name === name)
        if (m) return m
        const n = created.find(x => x.name === name)
        return n ? { name: n.name, sourceLang: n.sourceLang, targetLang: n.targetLang, isNew: true, tmx: n.tmx } : null
    }
    function memoriesFor(lang) { return (s.memories || {})[lang] || [] }
    function setMemories(lang, list) { edit("memories", m => { m[lang] = list }) }
    function addMemory(lang, name) {
        // Updated only when there is no project memory to collect translations.
        const collect = isNew && !!s.projectMemory
        setMemories(lang, memoriesFor(lang).concat([{ memory: name, lookup: true, concordance: true, update: !collect, penalty: 0 }]))
    }
    function removeMemory(lang, i) {
        const name = memoriesFor(lang)[i].memory
        const list = memoriesFor(lang).filter((_, k) => k !== i)
        // A memory made here and taken out again is not made.
        const all = Object.assign({}, s.memories || {}, { [lang]: list })
        const elsewhere = Object.keys(all).some(k => (all[k] || []).some(x => x.memory === name))
        if (!elsewhere) created = created.filter(n => n.name !== name)
        setMemories(lang, list)
    }
    function makeMemory(name, lang, tmx) {
        created = created.concat([{ name: name, sourceLang: source, targetLang: lang, tmx: tmx }])
        creatingFor = ""
        addMemory(lang, name)
    }
    function nameTaken(name) {
        const n = name.trim().toLowerCase()
        return (catalog.memories || []).some(m => m.name.toLowerCase() === n) || created.some(m => m.name.toLowerCase() === n)
    }
    // As the request to make a project names them.
    function newMemories() { return created.map(n => ({ Name: n.name, SourceLang: n.sourceLang, TargetLang: n.targetLang })) }
    // Makes the memories (for a project made without them) and reads in
    // their TMX files; done(error).
    function createNewMemories(done) {
        const all = created.slice()
        const next = i => {
            if (i >= all.length) { importNewMemories(done); return }
            const n = all[i]
            Api.post(cid, "/api/memories", { name: n.name, sourceLang: n.sourceLang, targetLang: n.targetLang }, r => {
                // One made by an earlier try that failed later is there already.
                if (!r.ok && r.error.code !== "exists") { done(Fmt.fill(qsTr("The memory {name} was not made: {error}"), { name: n.name, error: r.error.message })); return }
                // It is on the connection now: the catalog knows it.
                const c = copy(catalog)
                c.memories = (c.memories || []).concat([{ name: n.name, sourceLang: n.sourceLang, targetLang: n.targetLang, units: 0 }])
                catalog = c
                next(i + 1)
            })
        }
        next(0)
    }
    function importNewMemories(done) {
        const files = created.filter(n => !!n.tmx)
        created = []
        const errors = []
        const next = i => {
            if (i >= files.length) { done(errors.join(" ")); return }
            const n = files[i]
            Backend.upload(cid, "POST", "/api/memories/" + Api.seg(n.name) + "/import", n.tmx, "application/xml", {}).done.connect(r => {
                if (!r.ok) errors.push(Fmt.fill(qsTr("{file} was not read into {memory}: {error}"), { file: Backend.fileName(n.tmx), memory: n.name, error: r.error.message }))
                next(i + 1)
            })
        }
        next(0)
    }

    // The checks: how many there are and how many are on.
    readonly property int checksOf: kinds.length + 1
    readonly property int checksOn: {
        const qa = s.qa || {}
        const disabled = qa.disabled || [], enabled = qa.enabled || []
        let n = qa.uneditedFuzzy ? 1 : 0
        for (const k of kinds) if (k.default ? disabled.indexOf(k.kind) < 0 : enabled.indexOf(k.kind) >= 0) n++
        return n
    }
    function editQa(change) { checksTouched = true; edit("qa", change) }

    // A template made of a project's settings: without what belongs to the
    // project itself, its termbase and its own memories.
    function templateOf(name, settings) {
        const t = { name: name }
        for (const k of ["client", "sourceLang", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare", "segmentation"])
            if (settings[k] !== undefined) t[k] = copy(settings[k])
        if (t.terminology) delete t.terminology.termbase
        const mems = t.memories || {}
        for (const lang of Object.keys(mems)) mems[lang] = (mems[lang] || []).filter(m => m.memory !== settings.name + " (" + lang + ")")
        t.memories = mems
        return t
    }
}
