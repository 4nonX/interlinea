import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import QtCore
import Interlinea

// The editor (web/src/screens/Editor.tsx): a file of a project in one
// target language, segment by segment, with the bar above (the file, how
// far it is, which segments to show, finding, the project's actions), the
// segments, an optional preview of the document, and the panel beside
// them (results, checks, memory search, review, comments, adding a term,
// and the segment's tools).
Item {
    id: page
    signal notice(string text)
    signal back()
    signal openProject(string connectionId, var projectId)
    signal openReview(string connectionId, string termbase)

    property string cid: ""
    property int projectId: 0
    property string file: ""
    property string targetLang: ""
    property var project: ({})
    readonly property var settings: project.settings || {}
    readonly property string sourceLang: settings.sourceLang || ""
    readonly property var conn: Backend.connection(cid)
    readonly property string base: "/api/projects/" + projectId
    readonly property bool manages: !!conn && (conn.local || Roles.allows((project.access || {}).role || "", "manage"))
    readonly property string grammarService: (settings.qa || {}).grammar || ""

    // The segments, and what changes with them: one segment changed is
    // put in place and revision counts up, so rows follow without being
    // made again.
    property var segments: []
    property var rowOf: ({})
    property int revision: 0
    property var pending: ({})
    property var presence: ({})
    property var shownRows: []
    property string show: "all"
    property string find: ""
    property bool table: false

    // The open segment.
    property string activeId: ""
    readonly property int activeRow: activeId in rowOf ? rowOf[activeId] : -1
    readonly property var activeSegment: activeRow >= 0 ? segments[activeRow] : null
    property var te: null              // its TargetEdit
    property var marks: []
    property string sourceSelection: ""
    property var loadedTarget: []
    property var origin: ({})
    property var ignored: []
    property bool insertBest: false
    property int length: 0
    property var results: []
    property int selected: 0
    property var issues: []
    property var grammar: ({ id: "", text: "", found: [], error: "" })
    property var conflicts: ({})
    property string conflictId: ""
    property var completions: []
    property var misspellings: []
    property var services: []
    property string asking: ""
    property string serviceProblem: ""
    property var suggestion: null
    property var quality: null
    property string reviewTermbase: ""
    property int reviewCount: 0
    property bool previewOn: false
    property var layoutBlocks: []
    property string layoutFile: ""
    property bool lookupOpen: false
    property var lookupHits: null
    property string lookupQuery: ""
    property bool captureOpen: false
    property bool confirmWaits: false
    property bool confirmAsked: false
    property bool savingDocument: false
    property int lookupSerial: 0
    property int checkSerial: 0
    property int grammarSerial: 0
    property int searchSerial: 0
    property int qualityAsked: 0

    function plainOf(content) { return Fmt.plain(content) }
    function segmentAt(i, rev) { return segments[i] || {} }
    function targetOf(s, rev) { return pending[s.id] !== undefined ? pending[s.id] : (s.target || []) }
    function say(text) { if (text) notice(text) }
    function isDocument(name) { return /\.(docx|docm|pptx|pptm|ppsx|xlsx|xlsm|html|htm|xhtml|idml|odt|ods|odp|xml|md|markdown|properties|json|yaml|yml|po|pot|resx|srt|vtt|sbv|ass|ssa|sub|ttml|dfxp|itt|stl|scc|txt|csv|tsv|pdf)$/i.test(name) }
    function isBilingual(name) { return /\.(xlf|xliff|sdlxliff|mqxliff|mxliff|txlf|txml|ttx|mqxlz|rtf)$/i.test(name) }
    readonly property var issueCategories: [["accuracy", qsTr("Accuracy")], ["fluency", qsTr("Fluency")], ["terminology", qsTr("Terminology")], ["style", qsTr("Style")],
                                            ["locale", qsTr("Locale conventions")], ["formatting", qsTr("Formatting")], ["other", qsTr("Other")]]
    readonly property var issueSeverities: [["minor", qsTr("Minor"), 1], ["major", qsTr("Major"), 5], ["critical", qsTr("Critical"), 25]]

    // Opens a file of a project in one language; without them, the first
    // file to translate and its first language.
    function open(connectionId, id, fileName, lang) {
        leave()
        cid = connectionId
        projectId = Number(id)
        file = fileName || ""
        targetLang = lang || ""
        project = ({}); segments = []; rowOf = ({}); pending = ({}); presence = ({}); shownRows = []
        activeId = ""; results = []; issues = []; conflicts = ({}); quality = null; suggestion = null
        table = false; previewOn = false; layoutBlocks = []; layoutFile = ""; lookupOpen = false; captureOpen = false
        show = "all"; find = ""; reviewCount = 0
        Api.get(cid, base, r => {
            if (!r.ok) { say(r.error.message); return }
            project = r.data
            const files = (settings.files || []).filter(f => f.role === "translatable")
            if (!file && files.length) file = files[0].name
            if (!targetLang) {
                const f = files.find(x => x.name === file)
                targetLang = f && f.targetLang ? f.targetLang : ((settings.targetLangs || [])[0] || "")
            }
            projectRead()
            load()
        })
    }
    // Says the person left the file, as they leave the page.
    function leave() {
        saveActive()
        if (conn && conn.ready && activeId && file)
            Api.call(cid, "DELETE", base + "/presence", undefined, null, { file: file, lang: targetLang })
    }
    onVisibleChanged: if (!visible) leave()
    function loadProject() {
        Api.get(cid, base, r => { if (r.ok) { project = r.data; projectRead() } else say(r.error.message) })
    }
    function projectRead() {
        loadServices()
        updateReview()
    }
    // A layered project termbase's own entries are its changes to the
    // masters, waiting to be reviewed and merged.
    function updateReview() {
        const t = settings.terminology || {}
        if (t.mode !== "layered" || !t.termbase) { reviewCount = 0; return }
        Api.get(cid, "/api/termbases/" + Api.seg(t.termbase) + "/layered", r => {
            reviewTermbase = t.termbase
            reviewCount = r.ok ? r.data.filter(e => e.termbase === t.termbase).length : 0
        })
    }
    function takeAll(list) {
        const map = {}
        list.forEach((s, i) => { map[s.id] = i })
        segments = list
        rowOf = map
        revision++
    }
    function put(s) {
        const i = rowOf[s.id]
        if (i === undefined) return
        segments[i] = s
        revision++
    }
    function setPending(id, content) { const p = Object.assign({}, pending); p[id] = content; pending = p; revision++ }
    function clearPending(id) { if (pending[id] === undefined) return; const p = Object.assign({}, pending); delete p[id]; pending = p; revision++ }

    function load() {
        Api.get(cid, base + "/segments", r => {
            if (!r.ok) { say(r.error.message); return }
            takeAll(r.data)
            // A Word document whose segments carry the state of a bilingual
            // file is a table for review: it is saved as the file it is, and
            // has no preview of its own.
            if (!table && /\.docx$/i.test(file) && segments.length && typeof segments[0].file === "object") { table = true; previewOn = false }
            applyFilter()
            reloadPresence()
            loadQuality()
            const first = segments.findIndex(s => s.status !== "confirmed" && !s.locked)
            if (segments.length) goToSegment(segments[Math.max(0, first)].id)
            else say(qsTr("No text to translate in this file."))
        }, { file: file, lang: targetLang })
    }

    // Which segments show: all, those to do, those with warnings; and those
    // with the text found. The open one always shows.
    readonly property int withWarnings: { revision; return segments.filter(s => (s.warnings || []).length).length }
    readonly property string folded: Backend.foldForSearch(find.trim())
    function holds(s) {
        if (!folded) return true
        return Backend.foldForSearch(Fmt.plain(s.source)).indexOf(folded) >= 0 || Backend.foldForSearch(Fmt.plain(s.target)).indexOf(folded) >= 0
    }
    readonly property int found: { revision; return folded ? segments.filter(s => holds(s)).length : 0 }
    function applyFilter() {
        const out = []
        segments.forEach((s, i) => {
            if (s.id === activeId) { out.push(i); return }
            if (!holds(s)) return
            if (show === "todo" && s.status === "confirmed") return
            if (show === "warnings" && !(s.warnings || []).length) return
            out.push(i)
        })
        if (JSON.stringify(out) !== JSON.stringify(shownRows)) shownRows = out
    }
    onShowChanged: applyFilter()
    onFindChanged: applyFilter()

    function goToSegment(id) {
        const row = rowOf[id]
        if (row === undefined) return
        if (activeId !== id) activate(row)
        const at = shownRows.indexOf(row)
        if (at >= 0) list.positionViewAtIndex(at, ListView.Contain)
    }
    function move(step) {
        const at = shownRows.indexOf(activeRow)
        const next = at + Number(step)
        if (at < 0 || next < 0 || next >= shownRows.length) return
        goToSegment(segments[shownRows[next]].id)
    }

    // The editor of the open segment comes and goes with its row.
    function attach(editor) {
        te = editor
        editor.setContent(loadedTarget)
        typed()
        editor.textEdit.forceActiveFocus()
    }
    function detach(editor) { if (te === editor) te = null }
    function editable() { return !!activeSegment && !!te && !activeSegment.locked }
    function dirty() { return editable() && !Backend.sameContent(te.content, loadedTarget) }

    function activate(row) {
        saveActive()
        const s = segments[row]
        if (activeId && !(activeId in conflicts)) clearPending(activeId)
        marks = []
        sourceSelection = ""
        loadedTarget = s.target || []
        origin = s.origin || {}
        ignored = s.ignored || []
        results = []
        issues = []
        selected = 0
        suggestion = null
        serviceProblem = ""
        captureOpen = false
        // The best match goes into an empty, untouched segment once its
        // matches are known.
        insertBest = !!Backend.option("autoInsert") && s.status === "new" && !s.locked && !Fmt.plain(loadedTarget).trim()
        activeId = s.id
        if (s.id in conflicts) conflictId = s.id
        applyFilter()
        sendPresence()
        lookup()
        recognize()
        runChecks()
        saveTimer.stop()
    }
    // What is typed: kept as pending, checked, and saved once typing pauses.
    function typed() {
        if (!te || !activeId) return
        setPending(activeId, te.content)
        length = te.plainText.length
        checkTimer.restart()
        if (dirty()) saveTimer.restart()
    }
    function saveActive() {
        saveTimer.stop()
        if (!dirty()) return
        const t = te.content
        const empty = !Fmt.plain(t).trim() && !t.some(r => r.tag)
        save(activeId, t, empty ? "new" : "draft", [], origin)
    }

    // Matches and terms.
    function lookup() {
        if (!activeId) return
        const serial = ++lookupSerial
        Api.post(cid, base + "/lookup", { lang: targetLang, source: activeSegment.source, limit: 9 }, r => {
            if (serial !== lookupSerial) return
            const out = r.ok ? r.data.map(m => Object.assign({ kind: "match" }, m)) : []
            results = out.concat(results.filter(x => x.kind === "term"))
            makeCompletions()
            if (insertBest) {
                insertBest = false
                const best = results[0]
                if (best && best.kind === "match" && editable() && !te.plainText.trim()) {
                    insertResult(1)
                    say(best.context ? qsTr("Context match inserted") : Fmt.fill(qsTr("{score}% match inserted"), { score: best.score }))
                }
            }
        })
    }
    function recognize() {
        if (!activeId) return
        const id = activeId
        Api.post(cid, base + "/recognize", { lang: targetLang, text: Fmt.plain(activeSegment.source) }, r => {
            if (id !== activeId || !r.ok) return
            marks = r.data.map(t => ({ start: t.start16, end: t.end16, cssClass: "term" }))
            results = results.filter(x => x.kind === "match").concat(r.data.map(t => Object.assign({ kind: "term" }, t)))
            makeCompletions()
        })
    }
    // What may complete a word being typed: the terms found for the
    // segment, then the longer words of its memory matches.
    function makeCompletions() {
        if (!Backend.option("autoComplete")) { completions = []; return }
        const words = [], fromMatches = []
        for (const r of results) {
            if (r.unit) { for (const w of Backend.matchAll("[\\p{L}\\p{N}][\\p{L}\\p{N}\\p{M}'\u2019-]{4,}", Fmt.plain(r.unit.target))) fromMatches.push(w); continue }
            for (const t of r.targets || []) if (t.status !== "forbidden") words.push(t.text)
        }
        completions = words.concat(fromMatches)
    }
    function insertResult(n) {
        if (!editable() || n < 1 || n > results.length) return
        const r = results[n - 1]
        if (r.kind === "match") {
            const t = r.unit.target
            te.setContent(t)
            origin = { kind: "tm", memory: r.memory, score: r.score, context: !!r.context, inserted: JSON.stringify(t) }
            typed()
        } else {
            const t = (r.targets || []).find(x => x.status !== "forbidden")
            if (t) te.insertPlain(t.text)
        }
        te.textEdit.forceActiveFocus()
    }
    function copySource() {
        if (!editable()) return
        te.setContent(activeSegment.source)
        origin = { kind: "copy" }
        typed()
        te.textEdit.forceActiveFocus()
    }
    function insertNextTag() {
        if (!editable()) return
        const have = (te.content || []).filter(r => r.tag).map(r => r.tag)
        for (const run of activeSegment.source || []) {
            if (!run.tag) continue
            const i = have.findIndex(t => t.id === run.tag.id && t.kind === run.tag.kind)
            if (i >= 0) { have.splice(i, 1); continue }
            te.insertTag(run.tag)
            te.textEdit.forceActiveFocus()
            return
        }
        say(qsTr("All tags are already in the target."))
    }

    // Checks.
    function runChecks() {
        if (!activeId || !te) return
        const serial = ++checkSerial, id = activeId
        const targets = {}
        targets[id] = te.content
        Api.post(cid, base + "/qa", { file: file, lang: targetLang, segments: [id], targets: targets }, r => {
            if (serial !== checkSerial || id !== activeId || !r.ok) return
            issues = r.data
            spelling()
        })
        checkGrammar(id, te.plainText)
    }
    // Asks the project's grammar service about a segment's translation, and
    // calls then when it has answered or failed.
    function checkGrammar(id, text, then) {
        const serial = ++grammarSerial
        const done = (found, error) => {
            if (serial === grammarSerial) grammar = { id: id, text: text, found: found, error: error }
            if (then) then()
        }
        if (!grammarService || !text.trim()) { done([], ""); return }
        Api.post(cid, "/api/providers/" + Api.seg(grammarService) + "/check", { lang: targetLang, text: text }, r => {
            if (!r.ok) { done([], r.error.message); return }
            done((r.data.matches || []).map(m => ({ kind: "grammar", message: Fmt.fill(qsTr("{message} (“{found}”)"), { message: m.message, found: m.found }),
                                                   found: m.found, replacements: m.replacements || [], offset: m.offset, length: m.length })), "")
        })
    }
    readonly property var grammarIssues: grammarService && te && grammar.id === activeId && grammar.text === (te ? te.plainText : "") ? grammar.found : []
    readonly property var shownIssues: issues.concat(grammarIssues).filter(i => ignored.indexOf(i.message) < 0)
    function spelling() {
        misspellings = shownIssues.filter(i => i.kind === "spelling" && i.found).map(i => ({ word: i.found, suggestions: i.suggestions || [] }))
    }
    onShownIssuesChanged: spelling()
    // The run of text a stretch of the plain text lies in, and where in it.
    function runOfStretch(content, offset, len) {
        let at = 0
        for (let i = 0; i < content.length; i++) {
            if (content[i].tag) continue
            const size = (content[i].text || "").length
            if (offset >= at && offset + len <= at + size) return { run: i, within: offset - at }
            at += size
        }
        return null
    }
    function replaceFound(offset, len, value) {
        if (!editable()) return
        const content = JSON.parse(JSON.stringify(te.content))
        const hit = runOfStretch(content, offset, len)
        if (!hit) return
        const t = content[hit.run].text
        const next = t.slice(0, hit.within) + value + t.slice(hit.within + len)
        if (next) content[hit.run] = { text: next }
        else content.splice(hit.run, 1)
        te.setContent(content)
        typed()
        te.textEdit.forceActiveFocus()
    }
    function ignoreIssue(text) {
        ignored = ignored.concat([text])
        // Kept with the segment, so the warning stays away.
        const s = activeSegment
        if (s) save(activeId, te ? te.content : s.target, s.status, s.warnings || [], origin)
    }

    // Saving a segment from the version it was read in.
    function save(id, target, status, warnings, from, done) {
        const row = rowOf[id]
        if (row === undefined) return
        const seg = segments[row]
        const edit = { target: target, status: status }
        if (warnings && warnings.length) edit.warnings = warnings
        const ign = id === activeId ? ignored : (seg.ignored || [])
        if (ign.length) edit.ignored = ign
        if (from && Object.keys(from).length) edit.origin = from
        if (id === activeId) loadedTarget = target
        Api.call(cid, "PUT", base + "/segments", edit, r => {
            if (!r.ok) {
                if (r.status === 409 || r.status === 412) {
                    Api.get(cid, base + "/segments", all => {
                        if (!all.ok) return
                        const theirs = all.data.find(x => x.id === id)
                        if (theirs) showConflict(id, theirs)
                    }, { file: file, lang: targetLang })
                } else say(Fmt.fill(qsTr("The segment was not saved: {error}"), { error: r.error.message }))
                if (done) done(false)
                return
            }
            put(r.data)
            if (id !== activeId || !te || Backend.sameContent(te.content, r.data.target)) clearPending(id)
            applyFilter()
            if (done) done(true)
        }, { file: file, lang: targetLang, segment: id }, seg.version || 0)
    }
    function showConflict(id, theirs) {
        const c = Object.assign({}, conflicts)
        c[id] = theirs
        conflicts = c
        conflictId = id
        if (id !== activeId) goToSegment(id)
    }
    function keepMine() {
        const id = conflictId, theirs = conflicts[id]
        const c = Object.assign({}, conflicts); delete c[id]; conflicts = c
        if (!theirs || !te) return
        // Saved over theirs, from their version.
        const s = Object.assign({}, segments[rowOf[id]], { version: theirs.version })
        put(s)
        save(id, te.content, "draft", [], origin)
    }
    function takeTheirs() {
        const id = conflictId, theirs = conflicts[id]
        const c = Object.assign({}, conflicts); delete c[id]; conflicts = c
        if (!theirs) return
        put(theirs)
        clearPending(id)
        if (id === activeId && te) { loadedTarget = theirs.target || []; te.setContent(loadedTarget) }
        say(qsTr("Their translation is in."))
    }

    function confirm() {
        if (!activeId) return
        if (activeSegment.locked) { say(qsTr("This segment is locked in the file and cannot be changed.")); return }
        if (!te) return
        const id = activeId, row = activeRow
        const target = te.content
        if (!Fmt.plain(target).trim()) { say(qsTr("The target is empty. Type a translation or insert a match first.")); return }
        saveTimer.stop()
        checkTimer.stop()
        ++checkSerial
        const from = origin
        // The grammar service reads the translation as it is confirmed,
        // unless it has read just that; what it finds is kept with it.
        const said = Fmt.plain(target)
        if (grammarService && !confirmAsked && !(grammar.id === id && grammar.text === said)) {
            if (confirmWaits) return
            confirmWaits = true
            checkGrammar(id, said, () => {
                confirmWaits = false
                confirmAsked = true
                if (id === activeId && te && te.plainText === said) confirm()
                confirmAsked = false
            })
            return
        }
        const targets = {}
        targets[id] = target
        Api.post(cid, base + "/qa", { file: file, lang: targetLang, segments: [id], targets: targets }, r => {
            const warnings = []
            if (r.ok) {
                if (id === activeId) issues = r.data
                const found = r.data.concat(grammar.id === id && grammar.text === said ? grammar.found : [])
                for (const i of found) if (ignored.indexOf(i.message) < 0) warnings.push({ kind: i.kind, text: i.message })
            }
            // Unconfirmed repetitions of the same source get the translation.
            const key = Backend.contentKey(segments[row].source)
            const reps = []
            if (Backend.option("autoPropagate"))
                segments.forEach((s, i) => { if (i !== row && s.status !== "confirmed" && !s.locked && Backend.contentKey(s.source) === key) reps.push(i) })
            save(id, target, "confirmed", warnings, from, ok => {
                if (!ok) return
                for (const i of reps) save(segments[i].id, target, "confirmed", [], { kind: "repetition", from: id })
                if (reps.length) say(Fmt.fill(qsTr("Also filled {repetitions} (segment {numbers})"), { repetitions: Fmt.plural(reps.length, qsTr("{n} repetition"), qsTr("{n} repetitions")), numbers: reps.map(i => i + 1).join(", ") }))
                if (id !== activeId) return
                // The next open segment after it, else the first open one.
                const open = i => i !== row && reps.indexOf(i) < 0 && segments[i].status !== "confirmed" && !segments[i].locked
                for (let i = row + 1; i < segments.length; i++) if (open(i)) { goToSegment(segments[i].id); return }
                for (let i = 0; i < row; i++) if (open(i)) { goToSegment(segments[i].id); return }
                say(qsTr("All segments confirmed"))
            })
        })
    }

    // Terms from the selections.
    property string captureSource: ""
    property string captureTarget: ""
    function openCapture() {
        if (!activeId) return
        lookupOpen = false
        captureSource = sourceSelection
        captureTarget = te ? te.selectedText.trim() : ""
        captureOpen = true
        panel.showCapture()
    }
    function addTerm(src, tgt, note) {
        if (!src || !tgt || !activeId) return
        const fields = note ? [{ field: "note", value: note }] : []
        const entry = { fields: fields, languages: [
            { lang: sourceLang, fields: [{ field: "context", value: Fmt.plain(activeSegment.source) }], terms: [{ text: src, status: "proposed" }] },
            { lang: targetLang, terms: [{ text: tgt, status: "proposed" }] }
        ] }
        Api.post(cid, base + "/terms", entry, r => {
            if (!r.ok) { say(Fmt.fill(qsTr("The term was not added: {error}"), { error: r.error.message })); return }
            say(Fmt.fill(qsTr("Added “{term}” to {termbase}"), { term: src, termbase: r.data.termbase }))
            captureOpen = false
            if (te) te.textEdit.forceActiveFocus()
            recognize()
            updateReview()
        })
    }
    // The memory, searched for words in either language.
    function openLookup(text) {
        captureOpen = false
        let q = text || ""
        if (!q && te) q = te.selectedText
        if (!q) q = sourceSelection
        lookupQuery = q.trim()
        lookupOpen = true
        panel.showLookup()
        searchMemory()
    }
    function searchMemory() {
        const q = lookupQuery.trim()
        lookupHits = null
        if (q.length < 2) return
        const serial = ++searchSerial
        let left = 2
        const hits = []
        for (const inTarget of [false, true]) {
            Api.post(cid, base + "/concordance", { lang: targetLang, query: q, inTarget: inTarget, limit: 20 }, r => {
                if (r.ok) for (const h of r.data) if (!hits.some(o => o.memory === h.memory && o.unit.id === h.unit.id)) hits.push(h)
                if (--left === 0 && serial === searchSerial) lookupHits = hits
            })
        }
    }

    // What others change comes in as it happens.
    function refreshFromServer() {
        Api.get(cid, base + "/segments", r => {
            if (!r.ok) { if (r.status === 404) say(qsTr("This file is no longer in the project.")); return }
            for (const theirs of r.data) {
                const mine = segments[rowOf[theirs.id]]
                if (!mine || (theirs.version || 0) <= (mine.version || 0)) continue
                if (theirs.id === activeId && dirty()) { showConflict(theirs.id, theirs); continue }
                put(theirs)
                if (theirs.id === activeId && te) { loadedTarget = theirs.target || []; te.setContent(loadedTarget); clearPending(theirs.id) }
            }
            applyFilter()
        }, { file: file, lang: targetLang })
    }
    function reloadPresence() {
        Api.get(cid, base + "/presence", r => {
            if (!r.ok) return
            const me = conn ? (conn.user || {}).id : 0
            const by = {}
            for (const p of r.data) if (p.userId !== me) (by[p.segment] = by[p.segment] || []).push(p.userName)
            presence = by
        }, { file: file, lang: targetLang })
    }
    function sendPresence() {
        if (!conn || !conn.ready || !activeId) return
        Api.put(cid, base + "/presence", { file: file, lang: targetLang, segment: activeId }, null)
    }
    readonly property var progress: {
        revision
        let total = 0, done = 0
        for (const s of segments) { const w = Backend.wordCount(s.source || []); total += w; if (s.status === "confirmed") done += w }
        return { total: total, done: done }
    }

    // Words the project spells right.
    function addWord(word) {
        const st = JSON.parse(JSON.stringify(settings))
        const qa = st.qa || {}
        qa.words = (qa.words || []).filter(w => w !== word).concat([word])
        st.qa = qa
        Api.put(cid, base + "/settings", st, r => {
            if (!r.ok) { say(Fmt.fill(qsTr("“{word}” was not added: {error}"), { word: word, error: r.error.message })); return }
            project = r.data
            say(Fmt.fill(qsTr("“{word}” is spelled right in this project from now on"), { word: word }))
            runChecks()
        }, project.version || 0)
    }
    // Translation services.
    function loadServices() {
        Api.get(cid, "/api/providers", r => { if (r.ok) services = r.data.filter(p => p.kind !== "check").map(p => p.name) })
    }
    function askService(name) {
        if (!activeId || asking) return
        const id = activeId
        asking = name
        serviceProblem = ""
        Api.post(cid, "/api/providers/" + Api.seg(name) + "/translate", { sourceLang: sourceLang, targetLang: targetLang, segments: [{ id: id, source: activeSegment.source }] }, r => {
            asking = ""
            if (id !== activeId) return
            const ts = r.ok ? (r.data.translations || []) : []
            if (!ts.length) { serviceProblem = r.ok ? Fmt.fill(qsTr("{name} did not translate the segment."), { name: name }) : r.error.message; suggestion = null }
            else suggestion = Object.assign({ name: name }, ts[0])
        })
    }
    function insertSuggestion() {
        if (!editable() || !suggestion) return
        te.setContent(suggestion.target)
        typed()
        te.textEdit.forceActiveFocus()
    }

    // Comments and review issues: the segment as the server has it now
    // brings them; the translation being worked on stays.
    function takeComments(segment) {
        const row = rowOf[segment.id]
        if (row === undefined) return
        const mine = Object.assign({}, segments[row])
        const issuesChanged = JSON.stringify(mine.issues || []) !== JSON.stringify(segment.issues || [])
        for (const key of ["comments", "issues"]) { if ((segment[key] || []).length) mine[key] = segment[key]; else delete mine[key] }
        put(mine)
        if (issuesChanged) loadQuality()
    }
    function refreshComments() {
        Api.get(cid, base + "/segments", r => { if (r.ok) for (const s of r.data) takeComments(s) }, { file: file, lang: targetLang })
    }
    function addComment(text) {
        const said = text.trim()
        if (!activeId || !said) return
        Api.post(cid, base + "/comments", { text: said }, r => {
            if (!r.ok) { say(r.error.message); return }
            panel.commentSent()
            takeComments(r.data)
        }, { file: file, lang: targetLang, segment: activeId })
    }
    function deleteComment(commentId) {
        Api.call(cid, "DELETE", base + "/comments", undefined, r => { if (!r.ok) say(r.error.message); else takeComments(r.data) },
                 { file: file, lang: targetLang, segment: activeId, comment: commentId })
    }
    function addIssue(category, severity, note) {
        if (!activeId) return
        const body = { category: category, severity: severity }
        if (note.trim()) body.note = note.trim()
        Api.post(cid, base + "/issues", body, r => {
            if (!r.ok) { say(r.error.message); return }
            panel.issueSent()
            takeComments(r.data)
        }, { file: file, lang: targetLang, segment: activeId })
    }
    function deleteIssue(issueId) {
        Api.call(cid, "DELETE", base + "/issues", undefined, r => { if (!r.ok) say(r.error.message); else takeComments(r.data) },
                 { file: file, lang: targetLang, segment: activeId, issue: issueId })
    }
    // The file's score, as the server counts it.
    function loadQuality() {
        const asked = ++qualityAsked
        Api.get(cid, base + "/review", r => {
            if (!r.ok || asked !== qualityAsked) return
            quality = (r.data.files || []).find(f => f.file === file) || null
        }, { lang: targetLang })
    }

    // Find and replace in the translations; a place lies between two tags,
    // so the tags stay where they are.
    function replaceCount(text, with_, matchCase, wholeWords) {
        let places = 0, segs = 0, locked = 0
        if (text) {
            for (const s of segments) {
                const n = Backend.replaceInContent(s.id === activeId && te ? te.content : (s.target || []), text, with_, matchCase, wholeWords).count
                if (!n) continue
                if (s.locked) locked++
                else { places += n; segs++ }
            }
        }
        return { places: places, segments: segs, locked: locked }
    }
    function replaceAll(text, with_, matchCase, wholeWords) {
        if (!text) return
        const changes = []
        let places = 0
        for (const s of segments) {
            if (s.locked) continue
            const r = Backend.replaceInContent(s.id === activeId && te ? te.content : (s.target || []), text, with_, matchCase, wholeWords)
            if (r.count) { places += r.count; changes.push({ id: s.id, target: r.content }) }
        }
        if (!changes.length) return
        // One after the other, so a segment someone else changed stops nothing else.
        const step = i => {
            if (i >= changes.length) {
                say(Fmt.fill(qsTr("Replaced {places} in {segments}"), { places: Fmt.plural(places, qsTr("{n} place"), qsTr("{n} places")), segments: Fmt.plural(changes.length, qsTr("{n} segment"), qsTr("{n} segments")) }))
                return
            }
            const c = changes[i]
            if (c.id === activeId && te) te.setContent(c.target)
            // A segment that changes is a draft again; one emptied is new.
            save(c.id, c.target, c.target.length ? "draft" : "new", [], {}, () => step(i + 1))
        }
        step(0)
    }

    // Splitting and joining.
    readonly property var cut: {
        revision
        const s = activeSegment || {}, row = activeRow
        const next = row >= 0 && row + 1 < segments.length ? segments[row + 1] : null
        const kept = x => !!(x && typeof x.file === "object" && x.file.fixed)
        const fixed = kept(s) || /\.(sdlxliff|mqxliff|mxliff|txml|ttx|mqxlz|rtf)$/i.test(file)
        const cuts = row >= 0 && !s.locked && !fixed
        const joinedFrom = (s.joined || []).length
        const part = (s.split || {}).unit || "", nextPart = next ? (next.split || {}).unit || "" : ""
        const joins = cuts && !!next && !next.locked && !kept(next) && ((part && part === nextPart) || (s.block && s.block === next.block))
        const note = fixed ? qsTr("The program this file comes from keeps its own record of every segment, so its segments stay as they are.")
                   : part && joinedFrom > 0 ? Fmt.fill(qsTr("A part of {n} segments of the file that were joined."), { n: joinedFrom })
                   : joinedFrom > 0 ? Fmt.fill(qsTr("Joined from {n} segments of the file. Split it where they met to have them apart again."), { n: joinedFrom })
                   : part ? qsTr("A part of a segment of the file that was split.") : ""
        return { split: cuts, join: joins, note: note, shown: isDocument(file) || isBilingual(file) }
    }
    function takeSegments(list, row) {
        saveTimer.stop()
        activeId = ""
        takeAll(list)
        applyFilter()
        if (segments.length) goToSegment(segments[Math.max(0, Math.min(row, segments.length - 1))].id)
    }
    function resegment(how, before, failed) {
        if (!activeId) return
        const id = activeId
        const go = () => {
            const row = rowOf[id]
            if (row === undefined || id !== activeId) return
            Api.call(cid, "POST", base + "/segments/" + how, how === "split" ? { before: before } : null, r => {
                if (!r.ok) { if (failed) failed(r.error.message); else say(r.error.message); return }
                takeSegments(r.data, row)
                say(how === "split" ? qsTr("Split in two segments") : qsTr("Joined with the next segment"))
                if (how === "split") splitDialog.close()
            }, { file: file, lang: targetLang, segment: id, version: segments[row].version || 0 })
        }
        // What was typed is saved first, so it is what gets split or joined.
        if (dirty()) {
            saveTimer.stop()
            const t = te.content
            save(id, t, !Fmt.plain(t).trim() && !t.some(x => x.tag) ? "new" : "draft", [], origin, ok => { if (ok) go(); else if (failed) failed(qsTr("The segment was not saved.")) })
        } else go()
    }

    function openSplit() { if (cut.split) splitDialog.start() }

    // The subtitle being typed: its lines, how long, how fast.
    function subtitleCounter() {
        const s = activeSegment
        if (!s || !s.timing || !te) return ""
        const lim = (settings.qa || {}).subtitles || {}
        let maxLine = lim.lineLength > 0 ? lim.lineLength : 42
        if (s.timing.lineLimit > 0 && s.timing.lineLimit < maxLine) maxLine = s.timing.lineLimit
        const maxLines = lim.lines > 0 ? lim.lines : 2
        const maxCps = lim.cps > 0 ? lim.cps : 17
        // The lines: the text cut at the tags the source has as breaks, and
        // at line breaks typed in.
        const breaks = (s.source || []).filter(r => r.tag && r.tag.type === "break").map(r => r.tag.id)
        const lines = [""]
        for (const r of te.content) {
            if (r.tag) { if (r.tag.type === "break" || (r.tag.kind === "standalone" && breaks.indexOf(r.tag.id) >= 0)) lines.push(""); continue }
            const parts = (r.text || "").split("\n")
            lines[lines.length - 1] += parts[0]
            for (let i = 1; i < parts.length; i++) lines.push(parts[i])
        }
        const shown = lines.map(l => l.trim()).filter(l => l)
        let longest = 0, chars = 0
        for (const l of shown) { const n = [...l].length; longest = Math.max(longest, n); chars += n }
        const count = Math.max(1, shown.length)
        const seconds = ((s.timing.end || 0) - (s.timing.start || 0)) / 1000
        const part = (text, over) => over ? "<font color=\"" + Theme.danger + "\"><b>" + text + "</b></font>" : text
        const out = [part(Fmt.fill(qsTr("Longest line {n} of {max}"), { n: longest, max: maxLine }), longest > maxLine),
                     part(count === 1 ? Fmt.fill(qsTr("1 line of {max}"), { max: maxLines }) : Fmt.fill(qsTr("{n} lines of {max}"), { n: count, max: maxLines }), count > maxLines)]
        if (seconds > 0) {
            const cps = chars / seconds
            out.push(part(Fmt.fill(qsTr("{cps} of {max} characters per second in {seconds} s"), { cps: Fmt.dec(cps, 1), max: Fmt.dec(maxCps, maxCps % 1 ? 1 : 0), seconds: Fmt.dec(seconds, 2) }), cps > maxCps))
        }
        return out.join(" · ")
    }

    // Saving the translated document, as the file came.
    readonly property var fileInfo: (settings.files || []).find(f => f.name === file) || {}
    readonly property bool asXliff: !(isDocument(file) && !table) && (isBilingual(file) || table)
    readonly property string saveLabel: fileInfo.package ? qsTr("Save return package")
                                       : asXliff && /\.rtf$/i.test(file) ? qsTr("Save RTF")
                                       : asXliff && /\.docx$/i.test(file) ? qsTr("Save DOCX")
                                       : asXliff ? qsTr("Save XLIFF") : qsTr("Save translated document")
    property string fetched: ""
    function saveDocument() {
        saveActive()
        savingDocument = true
        const pkg = !!fileInfo.package
        Backend.fetch(cid, pkg ? base + "/return-package" : base + "/files/translated", pkg ? { lang: targetLang } : { name: file, lang: targetLang }).done.connect(r => {
            savingDocument = false
            if (!r.ok) { say(Fmt.fill(qsTr("Could not save the document: {error}"), { error: r.error.message })); return }
            if (!r.temp) { say(Fmt.fill(qsTr("Could not save the document: {error}"), { error: r.saveError })); return }
            fetched = r.temp
            docDialog.selectedFile = StandardPaths.writableLocation(StandardPaths.DocumentsLocation) + "/" + (r.fileName || Backend.fileName(file))
            docDialog.open()
        })
    }
    FileDialog {
        id: docDialog
        title: page.saveLabel
        fileMode: FileDialog.SaveFile
        currentFolder: StandardPaths.writableLocation(StandardPaths.DocumentsLocation)
        onAccepted: {
            const problem = Backend.keep(page.fetched, selectedFile)
            if (problem) { page.say(Fmt.fill(qsTr("Could not save the document: {error}"), { error: problem })); return }
            const open = page.segments.filter(s => s.status !== "confirmed" && !s.locked).length
            page.say(Fmt.fill(qsTr("Saved {file}."), { file: Backend.fileName(Backend.localPath(selectedFile)) })
                     + (open ? " " + Fmt.plural(open, qsTr("1 segment is not confirmed yet."), qsTr("{n} segments are not confirmed yet.")) : ""))
        }
    }
    function saveReview() {
        Backend.fetch(cid, base + "/review", { format: "csv" }).done.connect(r => {
            if (!r.ok || !r.temp) { say(Fmt.fill(qsTr("Could not save the review: {error}"), { error: r.ok ? r.saveError : r.error.message })); return }
            fetched = r.temp
            reviewDialog.selectedFile = StandardPaths.writableLocation(StandardPaths.DocumentsLocation) + "/" + (settings.name || "") + " review.csv"
            reviewDialog.open()
        })
    }
    FileDialog {
        id: reviewDialog
        title: qsTr("Save the project's review as CSV")
        fileMode: FileDialog.SaveFile
        currentFolder: StandardPaths.writableLocation(StandardPaths.DocumentsLocation)
        onAccepted: {
            const problem = Backend.keep(page.fetched, selectedFile)
            page.say(problem ? Fmt.fill(qsTr("Could not save the review: {error}"), { error: problem }) : Fmt.fill(qsTr("Saved {file}"), { file: Backend.fileName(Backend.localPath(selectedFile)) }))
        }
    }

    // The preview: the document's paragraphs, each segment in its
    // translation, or in the source, grey and underlined, while it has none.
    function loadLayout() {
        if (layoutFile === file) return
        layoutFile = file
        layoutBlocks = []
        const f = file
        Api.get(cid, base + "/files/layout", r => { if (f === file) layoutBlocks = r.ok ? (r.data.blocks || []) : [] }, { name: f })
    }
    onPreviewOnChanged: if (previewOn) loadLayout()
    function showPreview(on) { previewOn = !!on && isDocument(file) && !table }

    Timer { id: checkTimer; interval: 500; onTriggered: page.runChecks() }
    Timer { id: saveTimer; interval: 1500; onTriggered: page.saveActive() }
    Timer { id: presenceTimer; interval: 30000; repeat: true; running: page.visible && page.activeId !== ""; onTriggered: page.sendPresence() }
    Timer { id: refreshTimer; interval: 250; onTriggered: page.refreshFromServer() }
    Connections {
        target: Backend
        function onEvent(c, type, data) {
            if (c !== page.cid || !data || Number(data.projectId) !== page.projectId) return
            if (type === "presence") { page.reloadPresence(); return }
            if (type !== "project") return
            const kind = data.kind, here = data.file === page.file && data.targetLang === page.targetLang
            if (kind === "segment-changed" && here) {
                const mine = page.segments[page.rowOf[data.segment]] || {}
                if ((data.version || 0) > (mine.version || 0)) refreshTimer.restart()
            } else if ((kind === "segment-commented" || kind === "segment-reviewed") && here) {
                // Only the comments or the issues changed: the translations here stay as they are.
                page.refreshComments()
                if (kind === "segment-reviewed") page.loadQuality()
            } else if (kind === "file-resegmented" && here) {
                // Someone split or joined segments: the file's list is another one.
                const mine = page.segments[page.rowOf[data.segment]]
                if (mine && (mine.version || 0) >= (data.version || 0)) return
                Api.get(page.cid, page.base + "/segments", r => {
                    if (!r.ok) return
                    // What is being typed stays, if its segment is still the one it was.
                    const was = page.activeId, version = (page.activeSegment || {}).version, typing = page.dirty(), typedNow = typing ? page.te.content : []
                    page.takeSegments(r.data, Math.max(0, page.activeRow))
                    if (was in page.rowOf) {
                        page.goToSegment(was)
                        if (typing && page.te && page.segments[page.rowOf[was]].version === version) page.te.setContent(typedNow)
                    }
                }, { file: page.file, lang: page.targetLang })
            } else if (kind === "settings-changed") page.loadProject()
            else if (kind === "file-removed" || kind === "project-deleted") refreshTimer.restart()
        }
        function onReset(c) { if (c === page.cid) refreshTimer.restart() }
    }

    // The editor's keys (where the usual translation tools agree).
    readonly property bool keys: visible && activeId !== ""
    Shortcut { enabled: page.keys; sequences: ["Ctrl+Return", "Ctrl+Enter"]; onActivated: page.confirm() }
    Shortcut { enabled: page.keys; sequence: "Alt+Down"; onActivated: page.move(1) }
    Shortcut { enabled: page.keys; sequence: "Alt+Up"; onActivated: page.move(-1) }
    Repeater {
        model: 9
        Item { required property int index; Shortcut { enabled: page.keys; sequence: "Ctrl+" + (index + 1); onActivated: page.insertResult(index + 1) } }
    }
    Shortcut { enabled: page.keys; sequence: "Ctrl+Down"; onActivated: if (page.results.length) page.selected = Math.min(page.selected + 1, page.results.length - 1) }
    Shortcut { enabled: page.keys; sequence: "Ctrl+Up"; onActivated: if (page.results.length) page.selected = Math.max(page.selected - 1, 0) }
    Shortcut { enabled: page.keys; sequence: "Ctrl+Space"; onActivated: if (page.results.length) page.insertResult(Math.min(page.selected, page.results.length - 1) + 1) }
    Shortcut { enabled: page.keys; sequence: "Ctrl+Ins"; onActivated: page.copySource() }
    Shortcut { enabled: page.keys; sequence: "Ctrl+E"; onActivated: page.openCapture() }
    Shortcut { enabled: page.keys; sequences: ["Ctrl+K", "F3"]; onActivated: page.openLookup("") }
    Shortcut { enabled: page.keys; sequence: "F9"; onActivated: page.insertNextTag() }
    Shortcut { enabled: page.visible && page.segments.length > 0; sequence: "Ctrl+H"; onActivated: replaceDialog.start() }
    Shortcut { enabled: page.keys && page.cut.shown && page.cut.split; sequence: "Alt+Shift+S"; onActivated: page.openSplit() }
    Shortcut { enabled: page.keys && page.cut.shown && page.cut.join; sequence: "Alt+Shift+J"; onActivated: page.resegment("join") }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        // .editor-bar
        Rectangle {
            Layout.fillWidth: true
            implicitHeight: bar.implicitHeight + 2 * Theme.s3
            color: Theme.surface
            Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
            Flow {
                id: bar
                x: Theme.s5; y: Theme.s3
                width: parent.width - 2 * Theme.s5
                spacing: Theme.s2
                // The file and how far it is; on a narrow window the filter
                // and the search go on a line of their own (flex-wrap).
                GridLayout {
                    width: bar.width
                    readonly property bool wide: bar.width >= 980
                    columns: wide ? 2 : 1
                    columnSpacing: Theme.s4
                    rowSpacing: Theme.s2
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Theme.s4
                    Button { kind: "quiet"; small: true; iconName: "arrow-left"; text: qsTr("Projects"); onClicked: { page.leave(); page.back() } }
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.minimumWidth: 100
                        spacing: 0
                        Txt { role: "h3"; text: page.settings.name || ""; elide: Text.ElideRight; Layout.fillWidth: true; wrapMode: Text.NoWrap }
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: Theme.s3
                            readonly property var files: (page.settings.files || []).filter(f => f.role === "translatable").map(f => f.name)
                            readonly property var targets: page.settings.targetLangs || []
                            ComboBox {
                                visible: parent.files.length > 1
                                implicitHeight: 26
                                implicitWidth: 240
                                model: parent.files.map(f => Backend.fileName(f))
                                currentIndex: parent.files.indexOf(page.file)
                                Accessible.name: qsTr("File")
                                onActivated: i => { const f = parent.files[i]; if (f !== page.file) page.open(page.cid, page.projectId, f, page.targetLang) }
                            }
                            Txt { visible: parent.files.length <= 1; role: "sub"; text: Backend.fileName(page.file); Layout.fillWidth: true; Layout.maximumWidth: implicitWidth; Layout.minimumWidth: Math.min(implicitWidth, 90); elide: Text.ElideMiddle; wrapMode: Text.NoWrap }
                            Row {
                                spacing: Theme.s1
                                LangChip { code: page.sourceLang; anchors.verticalCenter: parent.verticalCenter }
                                Txt { text: "→"; role: "sub"; anchors.verticalCenter: parent.verticalCenter }
                                LangChip { visible: parent.parent.targets.length <= 1; code: page.targetLang; anchors.verticalCenter: parent.verticalCenter }
                                ComboBox {
                                    visible: parent.parent.targets.length > 1
                                    implicitHeight: 26
                                    implicitWidth: 200
                                    model: parent.parent.targets.map(t => Backend.languageName(t))
                                    currentIndex: parent.parent.targets.indexOf(page.targetLang)
                                    Accessible.name: qsTr("Target language")
                                    onActivated: i => { const l = parent.parent.targets[i]; if (l !== page.targetLang) page.open(page.cid, page.projectId, page.file, l) }
                                }
                            }
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: !parent.wide
                    spacing: Theme.s4
                    // .editor-progress
                    // Narrower windows take from the track and the field first.
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.preferredWidth: 180
                        Layout.maximumWidth: 180
                        Layout.minimumWidth: 80
                        spacing: 3
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: 6
                            radius: 3
                            color: Theme.dark ? Theme.border : "#e3e7ed"
                            Accessible.role: Accessible.ProgressBar
                            Accessible.name: Fmt.fill(qsTr("{percent} percent confirmed"), { percent: Fmt.percent(page.progress.done, page.progress.total) })
                            Rectangle { height: parent.height; radius: 3; color: Theme.ok; width: parent.width * (page.progress.total ? page.progress.done / page.progress.total : 0) }
                        }
                        Txt {
                            Layout.fillWidth: true
                            elide: Text.ElideRight
                            wrapMode: Text.NoWrap
                            color: Theme.text3
                            font.pixelSize: Theme.tiny
                            text: Fmt.fill(qsTr("{words} of {total} confirmed"), { words: Fmt.plural(page.progress.done, qsTr("{n} word"), qsTr("{n} words")), total: Fmt.num(page.progress.total) })
                        }
                    }
                    Segmented {
                        name: qsTr("Show segments")
                        model: [qsTr("All"), qsTr("To do"), page.withWarnings ? Fmt.fill(qsTr("Warnings ({n})"), { n: page.withWarnings }) : qsTr("Warnings")]
                        current: ["all", "todo", "warnings"].indexOf(page.show)
                        onChanged: i => page.show = ["all", "todo", "warnings"][i]
                    }
                    TextField {
                        Layout.fillWidth: true
                        Layout.preferredWidth: 200
                        Layout.maximumWidth: 200
                        Layout.minimumWidth: 100
                        implicitHeight: 28
                        placeholderText: qsTr("Find in the segments")
                        text: page.find
                        onTextEdited: page.find = text
                    }
                    Txt {
                        visible: page.folded !== ""
                        role: "sub"
                        text: page.found === 0 ? qsTr("Nothing found") : Fmt.plural(page.found, qsTr("{n} segment"), qsTr("{n} segments"))
                    }
                }
                }
                Button {
                    visible: page.reviewCount > 0
                    small: true
                    iconName: "git-merge"
                    text: Fmt.fill(qsTr("Review and merge ({n})"), { n: page.reviewCount })
                    onClicked: page.openReview(page.cid, page.reviewTermbase)
                }
                Button { small: true; iconName: "settings"; text: qsTr("Project settings"); onClicked: { page.leave(); page.openProject(page.cid, page.projectId) } }
                Button { small: true; iconName: "replace"; text: qsTr("Replace"); onClicked: replaceDialog.start() }
                Button { small: true; iconName: "memories"; text: qsTr("Memories"); onClicked: memoriesDialog.open() }
                Button { small: true; iconName: "bar-chart"; text: qsTr("Analysis"); onClicked: analysisDialog.start() }
                Button {
                    visible: page.isDocument(page.file) && !page.table
                    small: true
                    iconName: "eye"
                    text: qsTr("Preview")
                    checkable: true
                    checked: page.previewOn
                    onClicked: page.previewOn = !page.previewOn
                }
                Button {
                    visible: page.isDocument(page.file) || page.isBilingual(page.file) || page.table
                    small: true
                    iconName: "download"
                    text: page.savingDocument ? qsTr("Saving…") : page.saveLabel
                    enabled: !page.savingDocument
                    onClicked: page.saveDocument()
                }
            }
        }

        RowLayout {
            id: bodyRow
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            ListView {
                id: list
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                model: page.shownRows
                boundsBehavior: Flickable.StopAtBounds
                cacheBuffer: 1200
                reuseItems: false
                Accessible.role: Accessible.Table
                Accessible.name: qsTr("Segments")
                delegate: SegmentRow {
                    required property int modelData
                    editor: page
                    index: modelData
                }
                ScrollBar.vertical: ScrollBar {}
            }

            // .preview-pane
            Rectangle {
                visible: page.previewOn
                // The segments and the preview share what the panel leaves.
                Layout.preferredWidth: Math.max(320, (bodyRow.width - 360) / 2)
                Layout.fillHeight: true
                color: Theme.surface2
                Accessible.name: qsTr("Preview of the translated document")
                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: Theme.s3
                    anchors.leftMargin: Theme.s4
                    anchors.rightMargin: Theme.s4
                    spacing: Theme.s2
                    Txt { Layout.fillWidth: true; role: "muted"; text: qsTr("Layout from the document. Untranslated text appears in the source language, grey and underlined. The saved file is exact.") }
                    Flickable {
                        id: sheetView
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        clip: true
                        contentHeight: sheet.height + 2
                        ScrollBar.vertical: ScrollBar {}
                        // A sheet of paper, as the document is: white in either theme.
                        Rectangle {
                            id: sheet
                            width: Math.min(680, sheetView.width)
                            x: (sheetView.width - width) / 2
                            height: previewText.implicitHeight + 64
                            color: "#ffffff"
                            border.color: Theme.border
                            radius: 2
                            Text {
                                id: previewText
                                x: 32; y: 32
                                width: parent.width - 64
                                wrapMode: Text.Wrap
                                textFormat: Text.RichText
                                color: "#111111"
                                font.pixelSize: 13
                                text: { page.revision; return page.previewOn ? Backend.previewHtml(page.segments, page.layoutBlocks, page.activeId) : "" }
                                onLinkActivated: link => { if (link.startsWith("seg:")) page.goToSegment(link.slice(4)) }
                                HoverHandler { cursorShape: previewText.hoveredLink ? Qt.PointingHandCursor : Qt.ArrowCursor }
                            }
                        }
                    }
                }
            }

            EditorPanel {
                id: panel
                editor: page
                Layout.preferredWidth: 360
                Layout.fillHeight: true
            }
        }
    }

    SplitDialog { id: splitDialog; objectName: "splitDialog"; editor: page }
    ReplaceDialog { id: replaceDialog; objectName: "replaceDialog"; editor: page }
    EditorMemoriesDialog { id: memoriesDialog; objectName: "memoriesDialog"; editor: page }
    EditorAnalysisDialog { id: analysisDialog; objectName: "analysisDialog"; editor: page }
}
