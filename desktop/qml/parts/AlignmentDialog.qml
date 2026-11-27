import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import QtQuick.Controls as C
import Interlinea

// A document and its translation, done before, into units of a memory
// (web/src/screens/Alignment.tsx, align.ts): the core pairs their
// sentences, and the pairs are checked here, as lines between the two
// texts, before they are added. Into one memory, or into one chosen here
// of those given, or a new one.
Dialog {
    id: dlg
    wide: true
    signal added(int units, string memory)

    readonly property int lookBelow: 50
    readonly property int gutter: 96
    readonly property real near: 7
    readonly property real portRadius: 5
    readonly property real portOut: 6

    property string cid: ""
    property bool choice: false
    property var memories: []
    property var taken: []
    property string memoryName: ""
    property string sourceLang: "de-DE"
    property string targetLang: "en-GB"
    property string newName: ""
    property bool isNew: false
    property bool made: false
    property var paths: ["", ""]
    property bool busy: false
    property bool hasResult: false
    property bool overwrite: false
    // 0: not asked; 1: asked about closing; 2: about other documents.
    property int leaving: 0

    // The pairing: the segments of each side, the lines [source, target],
    // those that are the translator's own, the scores by unit, and the
    // order each side is read in.
    property var segs: [[], []]
    property var links: []
    property var mine: []
    property var scores: ({})
    property var order: [[], []]
    property var pendingOrder: [[], []]
    property var past: []
    property var future: []
    property int pickedSide: -1
    property int picked: -1
    property int runSide: -1
    property var run: []
    property int runAnchor: -1
    property int focusSide: -1
    property int focusIndex: -1
    property int rev: 0

    readonly property string memory: isNew ? newName.trim() : memoryName
    title: choice && !hasResult ? qsTr("Align documents") : Fmt.fill(qsTr("Align documents into {memory}"), { memory: memory })
    closePolicy: C.Popup.NoAutoClose

    // Opens it: into is the memories ({name, sourceLang, targetLang}) the
    // units may go into; with memory named, into that one only.
    function start(connectionId, into, takenNames, memoryNamed) {
        cid = connectionId
        choice = !memoryNamed
        memories = into || []
        taken = takenNames || []
        newName = ""
        made = false
        paths = ["", ""]
        hasResult = false
        busy = false
        leaving = 0
        overwrite = false
        past = []; future = []
        error = ""
        const first = memoryNamed ? (memories.find(m => m.name === memoryNamed) || memories[0] || {}) : memories[0]
        isNew = choice && !first
        memoryName = first ? (memoryNamed || first.name) : ""
        sourceLang = first ? first.sourceLang : "de-DE"
        targetLang = first ? first.targetLang : "en-GB"
        open()
    }

    // Pairs as keys, so they can be compared and found.
    function has(list, l) { return list.some(x => x[0] === l[0] && x[1] === l[1]) }
    function without(list, l) { return list.filter(x => !(x[0] === l[0] && x[1] === l[1])) }
    function keyOf(a, b) { return a.join(",") + "|" + b.join(",") }
    function sortedLines(list) { return list.slice().sort((x, y) => x[0] - y[0] || x[1] - y[1]).map(l => l[0] + "|" + l[1]) }
    function changed() { rev++ }

    function chooseMemory(name) {
        if (!choice || hasResult) return
        const m = memories.find(x => x.name === name)
        isNew = !m
        memoryName = m ? m.name : ""
        if (m) { sourceLang = m.sourceLang; targetLang = m.targetLang }
    }
    readonly property string destinationProblem: {
        if (!isNew) return ""
        const name = newName.trim()
        if (!name) return qsTr("Give the memory a name.")
        if (!made && taken.some(t => t.toLowerCase() === name.toLowerCase())) return qsTr("A memory with this name exists already.")
        if (sourceLang === targetLang) return qsTr("Source and target language must differ.")
        return ""
    }
    function setFile(side, path) { const p = paths.slice(); p[Number(side)] = path; paths = p }

    // The units the lines make, in the order of the document: segments
    // joined by lines, directly or through others, are one.
    function units() {
        const ns = segs[0].length
        const parent = []
        for (let i = 0; i < ns + segs[1].length; i++) parent.push(i)
        const find = x => { while (parent[x] !== x) { parent[x] = parent[parent[x]]; x = parent[x] } return x }
        for (const l of links) { const a = find(l[0]), b = find(ns + l[1]); if (a !== b) parent[a] = b }
        const at = {}, out = []
        for (const l of links) {
            const root = find(l[0])
            if (!(root in at)) { at[root] = out.length; out.push({ source: [], target: [] }) }
            const u = out[at[root]]
            if (u.source.indexOf(l[0]) < 0) u.source.push(l[0])
            if (u.target.indexOf(l[1]) < 0) u.target.push(l[1])
        }
        for (const u of out) { u.source.sort((a, b) => a - b); u.target.sort((a, b) => a - b) }
        out.sort((a, b) => a.source[0] - b.source[0])
        return out
    }
    // The units whose lines are all the translator's own: what stays when
    // the rest is paired again.
    function fixedUnits() {
        return units().filter(u => links.every(l => u.source.indexOf(l[0]) < 0 || has(mine, l)))
    }
    function lookUnits(us) {
        const checked = fixedUnits().map(f => f.source[0])
        return us.filter(u => { const k = keyOf(u.source, u.target); return k in scores && scores[k] < lookBelow && checked.indexOf(u.source[0]) < 0 })
    }
    function alone(side, index) { return !links.some(l => l[side] === index) }
    function fixedArray() { return fixedUnits().map(u => ({ source: u.source, target: u.target })) }

    readonly property var shown: {
        rev
        const us = hasResult ? units() : []
        const look = hasResult ? lookUnits(us) : []
        let lone = 0
        for (let s = 0; s < 2; s++) for (let i = 0; i < segs[s].length; i++) if (alone(s, i)) lone++
        const lookLines = links.filter(l => look.some(u => u.source.indexOf(l[0]) >= 0))
        return { units: us, look: look, lone: lone, lookLines: lookLines, checked: hasResult ? fixedUnits().length : 0 }
    }
    readonly property string status: {
        const s = shown
        let out = Fmt.fill(qsTr("{units} from {sentences} and {segments}."), { units: Fmt.plural(s.units.length, qsTr("{n} unit"), qsTr("{n} units")),
                                                                              sentences: Fmt.num(segs[0].length), segments: Fmt.plural(segs[1].length, qsTr("{n} segment"), qsTr("{n} segments")) })
        if (s.checked > 0) out += " " + Fmt.plural(s.checked, qsTr("1 unit is checked."), qsTr("{n} units are checked."))
        if (s.lone > 0) out += " " + Fmt.plural(s.lone, qsTr("1 segment has no pair and is left out."), qsTr("{n} segments have no pair and are left out."))
        if (s.look.length > 0) out += " " + Fmt.plural(s.look.length, qsTr("1 pair needs a look: the lengths or numbers do not fit."), qsTr("{n} pairs need a look: the lengths or numbers do not fit."))
        if (s.lone === 0 && s.look.length === 0) out += " " + qsTr("All of them fit.")
        return out
    }

    // Undo and redo: the lines, what they were paired from and the order read.
    function snap() { return { links: links.slice(), mine: mine.slice(), scores: Object.assign({}, scores), order: [order[0].slice(), order[1].slice()] } }
    function remember() { past = past.concat([snap()]); future = [] }
    function restore(s) {
        links = s.links; mine = s.mine; scores = s.scores; order = s.order
        picked = -1; pickedSide = -1; runSide = -1; run = []; runAnchor = -1
        changed()
    }
    function undo() { if (!hasResult || busy || !past.length) return; future = future.concat([snap()]); const p = past.slice(); restore(p.pop()); past = p }
    function redo() { if (!hasResult || busy || !future.length) return; past = past.concat([snap()]); const f = future.slice(); restore(f.pop()); future = f }

    // Makes the lines of a segment's unit the translator's own.
    function own(side, index) {
        let m = mine.slice()
        for (const u of units()) {
            if ((side === 0 ? u.source : u.target).indexOf(index) < 0) continue
            for (const l of links) if (u.source.indexOf(l[0]) >= 0 && !has(m, l)) m.push(l)
        }
        mine = m
    }
    // A line added is its maker's, and so is what it joins.
    function draw(l) { if (!has(links, l)) links = links.concat([l]); own(0, l[0]) }
    function confirmPicked() { if (!hasResult || picked < 0) return; remember(); own(pickedSide, picked); picked = -1; pickedSide = -1; changed() }
    function confirmAll() { if (!hasResult || mine.length === links.length) return; remember(); mine = links.slice(); picked = -1; pickedSide = -1; changed() }
    // A segment chosen, then one of the other text: the line between them
    // is drawn, or taken away if it is there.
    function choose(side, index) {
        side = Number(side); index = Number(index)
        if (!hasResult || index < 0 || index >= segs[side].length) return
        if (picked >= 0 && pickedSide !== side) {
            const l = side === 1 ? [picked, index] : [index, picked]
            remember()
            if (has(links, l)) { links = without(links, l); mine = without(mine, l) }
            else draw(l)
            picked = -1; pickedSide = -1
        } else if (picked === index && pickedSide === side) {
            picked = -1; pickedSide = -1
        } else {
            pickedSide = side; picked = index
        }
        changed()
    }
    // A line pulled from a segment to one of the other text is drawn.
    function pull(from, index, other) {
        from = Number(from); index = Number(index); other = Number(other)
        if (!hasResult) return
        const l = from === 0 ? [index, other] : [other, index]
        if (l[0] < 0 || l[0] >= segs[0].length || l[1] < 0 || l[1] >= segs[1].length) return
        if (!(has(links, l) && has(mine, l))) { remember(); draw(l) }
        picked = -1; pickedSide = -1
        changed()
    }
    function takeLine(source, target) {
        source = Number(source); target = Number(target)
        const l = [source, target]
        if (!has(links, l)) return
        remember()
        links = without(links, l); mine = without(mine, l)
        changed()
    }

    // Moving sections of a side into the other's order.
    function asRead(side, which) { const o = order[side]; return which.slice().sort((a, b) => o.indexOf(a) - o.indexOf(b)) }
    function movingFrom(side, index) { return runSide === side && run.indexOf(index) >= 0 ? asRead(side, run) : [index] }
    function movedIn(o, items, before) {
        if (before >= 0 && items.indexOf(before) >= 0) return o
        const rest = o.filter(i => items.indexOf(i) < 0), inRun = o.filter(i => items.indexOf(i) >= 0)
        const at = before < 0 ? rest.length : rest.indexOf(before)
        if (at < 0) return o
        return rest.slice(0, at).concat(inRun, rest.slice(at))
    }
    function inDocumentOrder(o) { return o.every((v, k) => v === k) }
    function move(side, items, before) {
        if (!hasResult || busy || !items.length) return
        const next = movedIn(order[side], items, before)
        if (JSON.stringify(next) === JSON.stringify(order[side])) return
        focusSide = side
        focusIndex = items[0]
        request(fixedArray(), side === 0 ? next : order[0], side === 1 ? next : order[1])
    }
    function nudge(side, index, by) {
        side = Number(side); index = Number(index); by = Number(by)
        if (!hasResult || index < 0 || index >= order[side].length) return
        const items = movingFrom(side, index), o = order[side]
        const first = o.indexOf(items[0]), last = o.indexOf(items[items.length - 1])
        if (by < 0 && first > 0) move(side, items, o[first - 1])
        else if (by > 0 && last < o.length - 1) move(side, items, last + 2 < o.length ? o[last + 2] : -1)
    }
    function mark(side, index, extend) {
        side = Number(side); index = Number(index)
        if (!hasResult || index < 0 || index >= order[side].length) return
        if (extend && runSide === side) {
            const o = order[side], a = o.indexOf(runAnchor), b = o.indexOf(index)
            run = o.slice(Math.min(a, b), Math.max(a, b) + 1)
        } else if (runSide === side && run.length === 1 && run[0] === index) {
            runSide = -1; run = []; runAnchor = -1
        } else {
            runSide = side; run = [index]; runAnchor = index
        }
        changed()
    }
    function extendRun(side, k, by) {
        const to = k + by
        if (to < 0 || to >= order[side].length) return
        const next = order[side][to]
        if (runSide !== side) mark(side, order[side][k], false)
        mark(side, next, true)
        const g = board.grip(side, next)
        if (g) g.forceActiveFocus()
    }

    // Sends both files to be paired, around the pairs that are fixed, with
    // each side read in the order given (none: the documents' own).
    function pair() { request([], [], []) }
    function pairAgain() { const f = fixedArray(); if (hasResult && f.length) request(f, order[0], order[1]) }
    function request(fixed, sourceOrder, targetOrder) {
        if (busy || !paths[0] || !paths[1] || destinationProblem) return
        const body = {}
        const langs = [sourceLang, targetLang]
        for (let s = 0; s < 2; s++) {
            const name = Backend.fileName(Backend.localPath(paths[s]))
            const read = Backend.readBase64(paths[s])
            if (read.error !== undefined) { error = Fmt.fill(qsTr("{file} could not be read: {error}."), { file: name, error: read.error }); return }
            body[s === 0 ? "source" : "target"] = { name: name, lang: langs[s], data: read.data }
        }
        if (fixed.length) body.fixed = fixed
        if (!inDocumentOrder(sourceOrder) || !inDocumentOrder(targetOrder)) body.order = { source: sourceOrder, target: targetOrder }
        pendingOrder = [sourceOrder, targetOrder]
        busy = true
        error = ""
        Api.post(cid, "/api/align", body, r => {
            busy = false
            if (!r.ok) { error = r.error.message; return }
            const o = r.data
            // Paired again, the lines before can be gone back to; paired
            // anew, there is nothing before.
            if (hasResult) remember()
            else { past = []; future = [] }
            segs = [o.source || [], o.target || []]
            const ord = [[], []]
            for (let s = 0; s < 2; s++) {
                ord[s] = pendingOrder[s]
                if (ord[s].length !== segs[s].length) { ord[s] = []; for (let i = 0; i < segs[s].length; i++) ord[s].push(i) }
            }
            order = ord
            runSide = -1; run = []; runAnchor = -1
            const l = [], m = [], sc = {}
            for (const p of o.pairs || []) {
                const a = p.sourceSegments || [], b = p.targetSegments || []
                if (!p.fixed) sc[keyOf(a, b)] = p.score
                if (!a.length || !b.length) continue
                const link = (s, t) => { if (!has(l, [s, t])) l.push([s, t]); if (p.fixed && !has(m, [s, t])) m.push([s, t]) }
                // Every segment of a pair hangs together: each sentence on
                // the first translation, each translation on the last sentence.
                for (const s of a) link(s, b[0])
                for (const t of b) link(a[a.length - 1], t)
            }
            links = l; mine = m; scores = sc
            picked = -1; pickedSide = -1
            hasResult = true
            changed()
            if (focusSide >= 0) {
                const fs = focusSide, fi = focusIndex
                focusSide = -1; focusIndex = -1
                Qt.callLater(() => { const g = board.grip(fs, fi); if (g) g.forceActiveFocus() })
            }
        })
    }

    // The text of segments joined: a space between them, unless a script
    // written without spaces meets there.
    function spaceless(ch) { return Backend.matchAll("^[\\p{Han}\\p{Hiragana}\\p{Katakana}\\p{Thai}\\p{Lao}\\p{Khmer}\\p{Myanmar}\\s]", ch).length > 0 }
    function joined(side, which) {
        const out = []
        let before = ""
        for (const i of which) {
            const next = segs[side][i] || []
            const after = Fmt.plain(next)
            const last = [...before].pop() || "", first = [...after][0] || ""
            if (before && after && !spaceless(last) && !spaceless(first)) { out.push({ text: " " }); before += " " }
            for (const runPart of next) out.push(runPart)
            before += after
        }
        return out
    }
    function add() {
        if (busy || !hasResult || !units().length) return
        busy = true
        error = ""
        if (!isNew || made) { sendUnits(); return }
        // The new memory is made first, and once.
        Api.post(cid, "/api/memories", { name: memory, sourceLang: sourceLang, targetLang: targetLang }, r => {
            if (!r.ok) {
                busy = false
                error = r.status === 403 ? qsTr("You may not make memories on the server. Ask an administrator for the manage role on all memories.") : r.error.message
                return
            }
            made = true
            newName = r.data.name || memory
            sendUnits()
        })
    }
    function sendUnits() {
        const body = units().map(u => ({ source: joined(0, asRead(0, u.source)), target: joined(1, asRead(1, u.target)) }))
        const n = body.length
        Api.post(cid, "/api/memories/" + Api.seg(memory) + "/units/batch", { units: body }, r => {
            busy = false
            if (!r.ok) { error = r.error.message; return }
            added(n, memory)
            close()
        }, overwrite ? { policy: "overwrite" } : undefined)
    }

    // Leaving with work on the lines asks first.
    readonly property string leaveQuestion: leaving === 1 ? qsTr("Close without adding? What you did to the lines is lost.")
                                          : leaving === 2 ? qsTr("Choose other documents? What you did to the lines is lost.") : ""
    function otherDocuments() { hasResult = false; past = []; future = []; error = "" }
    function askOther() { if (hasResult && (past.length || future.length)) leaving = 2; else otherDocuments() }
    function cancel() {
        if (leaving) { stay(); return }
        if (hasResult && !busy && (past.length || future.length)) { leaving = 1; return }
        close()
    }
    function stay() { leaving = 0 }
    function leave() { const how = leaving; leaving = 0; if (how === 1) close(); else if (how === 2) otherDocuments() }
    Shortcut { enabled: dlg.opened; sequence: "Escape"; onActivated: dlg.cancel() }

    // Brings the next segment without a line, or pair that needs a look, into view.
    function lookPlaces() {
        const ys = []
        for (let s = 0; s < 2; s++) for (let i = 0; i < segs[s].length; i++) if (alone(s, i)) { const b = board.box(s, i); if (b) ys.push(b.y + b.height / 2) }
        for (const u of shown.look) { const b = board.box(0, u.source[0]); if (b) ys.push(b.y + b.height / 2) }
        return ys.sort((a, b) => a - b)
    }
    function next() {
        const ys = lookPlaces()
        if (!ys.length) return
        const half = view.height / 2, from = view.contentY + half
        let y = ys[0]
        for (const v of ys) if (v > from + 1) { y = v; break }
        view.contentY = Math.max(0, Math.min(y - half, view.contentHeight - view.height))
    }

    Shortcut { enabled: dlg.opened && dlg.hasResult; sequence: "Ctrl+Z"; onActivated: dlg.undo() }
    Shortcut { enabled: dlg.opened && dlg.hasResult; sequences: ["Ctrl+Y", "Ctrl+Shift+Z"]; onActivated: dlg.redo() }

    // The first step: the memory and the two documents.
    Txt {
        visible: !dlg.hasResult
        Layout.fillWidth: true
        role: "help"
        text: qsTr("Choose a document and its translation. Their sentences are paired, and you check the pairs before they go into the memory. The two files need not be of the same format: DOCX, PPTX, XLSX, ODT, HTML, IDML, XML, Markdown, PDF, subtitles, software strings or text.")
    }
    Field {
        visible: !dlg.hasResult && dlg.choice
        Layout.fillWidth: true
        label: qsTr("Memory the units go into")
        ComboBox {
            width: parent.width
            model: dlg.memories.map(m => Fmt.fill(qsTr("{name} ({source} to {target})"), { name: m.name, source: m.sourceLang, target: m.targetLang })).concat([qsTr("A new memory")])
            currentIndex: dlg.isNew ? dlg.memories.length : Math.max(0, dlg.memories.findIndex(m => m.name === dlg.memoryName))
            onActivated: i => dlg.chooseMemory(i < dlg.memories.length ? dlg.memories[i].name : "")
        }
    }
    GridLayout {
        visible: !dlg.hasResult && dlg.choice && dlg.isNew
        Layout.fillWidth: true
        columns: 2
        columnSpacing: Theme.s5 - 8
        rowSpacing: Theme.s3
        Field {
            Layout.fillWidth: true
            Layout.preferredWidth: 1
            label: qsTr("Name of the new memory")
            TextField { width: parent.width; text: dlg.newName; onTextEdited: dlg.newName = text }
        }
        Item { Layout.fillWidth: true; Layout.preferredWidth: 1 }
        Field {
            Layout.fillWidth: true
            Layout.preferredWidth: 1
            label: qsTr("Source language")
            LanguageField { width: parent.width; language: dlg.sourceLang; onLanguageChanged: if (language) dlg.sourceLang = language }
        }
        Field {
            Layout.fillWidth: true
            Layout.preferredWidth: 1
            label: qsTr("Target language")
            LanguageField { width: parent.width; language: dlg.targetLang; onLanguageChanged: if (language) dlg.targetLang = language }
        }
    }
    Repeater {
        model: dlg.hasResult ? [] : [0, 1]
        Field {
            id: fileField
            required property int modelData
            readonly property string fieldLabel: modelData === 0 ? Fmt.fill(qsTr("Document in {language}"), { language: Backend.languageName(dlg.sourceLang) })
                                                                : Fmt.fill(qsTr("Its translation in {language}"), { language: Backend.languageName(dlg.targetLang) })
            Layout.fillWidth: true
            label: fieldLabel
            RowLayout {
                width: parent.width
                spacing: 12
                Button {
                    small: true
                    text: qsTr("Choose File")
                    Accessible.name: fileField.fieldLabel
                    onClicked: { fileDialog.side = fileField.modelData; fileDialog.title = fileField.fieldLabel; fileDialog.open() }
                }
                Txt {
                    Layout.fillWidth: true
                    role: dlg.paths[fileField.modelData] ? "" : "muted"
                    text: dlg.paths[fileField.modelData] ? Backend.fileName(Backend.localPath(dlg.paths[fileField.modelData])) : qsTr("No file chosen")
                    elide: Text.ElideMiddle
                    wrapMode: Text.NoWrap
                }
            }
        }
    }
    Txt {
        visible: !dlg.hasResult && dlg.destinationProblem !== "" && dlg.newName !== ""
        Layout.fillWidth: true
        role: "error"
        text: dlg.destinationProblem
    }
    FileDialog {
        id: fileDialog
        property int side: 0
        nameFilters: [qsTr("Documents (*.docx *.docm *.pptx *.pptm *.ppsx *.xlsx *.xlsm *.html *.htm *.xhtml *.idml *.odt *.ods *.odp *.xml *.md *.markdown *.properties *.json *.yaml *.yml *.po *.resx *.srt *.vtt *.sbv *.ass *.ssa *.sub *.ttml *.dfxp *.itt *.stl *.scc *.pdf *.txt)"), qsTr("All files (*)")]
        onAccepted: dlg.setFile(side, selectedFile.toString())
    }

    // The pairs: the status, how lines work, and the board.
    Txt { visible: dlg.hasResult; Layout.fillWidth: true; text: dlg.status }
    Txt {
        visible: dlg.hasResult
        Layout.fillWidth: true
        role: "help"
        text: qsTr("A line joins a sentence and its translation, from the point on the one to the point on the other; a ring instead of a point means nothing is joined there. To draw a line, choose a sentence and then its translation, or pull from one point to the other; the same takes a line away, as does choosing the line. Sentences joined by lines become one unit. A dotted line was paired for you; one you draw or confirm is solid, and stays when the rest is paired again around it.")
    }
    Txt {
        visible: dlg.hasResult
        Layout.fillWidth: true
        role: "help"
        text: qsTr("Where the translation has its sections in another order than the document, drag them by the handle beside them into the document's order, or move them with Alt+Up and Alt+Down. Shift+click on handles marks several to move together. What was paired for you is paired again in the new order.")
    }
    RowLayout {
        visible: dlg.hasResult
        Layout.fillWidth: true
        spacing: Theme.s2
        LangChip { code: dlg.sourceLang }
        Item { Layout.fillWidth: true }
        Button { small: true; kind: "quiet"; text: qsTr("Undo"); enabled: dlg.past.length > 0 && !dlg.busy; onClicked: dlg.undo(); ToolTip { visible: parent.hovered; text: "Ctrl+Z" } }
        Button { small: true; kind: "quiet"; text: qsTr("Redo"); enabled: dlg.future.length > 0 && !dlg.busy; onClicked: dlg.redo(); ToolTip { visible: parent.hovered; text: "Ctrl+Y" } }
        Button {
            small: true; kind: "quiet"
            text: qsTr("Confirm")
            enabled: { dlg.rev; return dlg.picked >= 0 && dlg.links.some(l => l[dlg.pickedSide] === dlg.picked && !dlg.has(dlg.mine, l)) }
            onClicked: dlg.confirmPicked()
        }
        Button { small: true; kind: "quiet"; text: qsTr("Confirm all"); enabled: dlg.mine.length < dlg.links.length; onClicked: dlg.confirmAll() }
        Button {
            small: true; kind: "quiet"
            text: dlg.busy ? qsTr("Pairing…") : qsTr("Pair the rest again")
            enabled: { dlg.rev; const s = dlg.shown; let anyAlone = false
                       for (let k = 0; k < 2 && !anyAlone; k++) for (let i = 0; i < dlg.segs[k].length && !anyAlone; i++) anyAlone = dlg.alone(k, i)
                       return !dlg.busy && s.checked > 0 && !(s.checked === s.units.length && !anyAlone) }
            onClicked: dlg.pairAgain()
        }
        Button { small: true; kind: "quiet"; text: qsTr("Next to look at"); enabled: { dlg.rev; return dlg.shown.lone > 0 || dlg.shown.look.length > 0 } onClicked: dlg.next() }
        Item { Layout.fillWidth: true }
        LangChip { code: dlg.targetLang }
    }
    Rectangle {
        visible: dlg.hasResult
        Layout.fillWidth: true
        implicitHeight: Math.max(120, Math.min(board.implicitHeight + 2, 520))
        radius: Theme.rControl
        color: Theme.surface2
        border.color: Theme.border
        clip: true
        Accessible.name: qsTr("The two texts and the lines between them")
        Flickable {
            id: view
            anchors.fill: parent
            anchors.margins: 1
            contentHeight: board.implicitHeight
            boundsBehavior: Flickable.StopAtBounds
            C.ScrollBar.vertical: ScrollBar {}
            AlignBoard { id: board; dialog: dlg; width: view.width; view: view }
        }
    }
    CheckBox {
        visible: dlg.hasResult
        text: qsTr("Replace other translations of these sentences")
        checked: dlg.overwrite
        onToggled: dlg.overwrite = checked
    }
    Note {
        visible: dlg.leaving > 0
        Layout.fillWidth: true
        kind: "warn"
        text: dlg.leaveQuestion
        Accessible.name: qsTr("Leave the alignment")
        RowLayout {
            spacing: Theme.s2
            Button { small: true; text: qsTr("Stay"); onClicked: dlg.stay() }
            Button { small: true; kind: "danger"; text: dlg.leaving === 1 ? qsTr("Close") : qsTr("Other documents"); onClicked: dlg.leave() }
        }
    }

    buttons: [
        Button { visible: dlg.hasResult; small: true; kind: "quiet"; text: qsTr("Other documents"); enabled: !dlg.leaving; onClicked: dlg.askOther() },
        Button { small: true; kind: "quiet"; text: qsTr("Cancel"); enabled: !dlg.leaving; onClicked: dlg.cancel() },
        Button {
            small: true; kind: "primary"
            text: dlg.hasResult ? Fmt.fill(qsTr("Add {units} to {memory}"), { units: Fmt.plural(dlg.shown.units.length, qsTr("{n} unit"), qsTr("{n} units")), memory: dlg.memory })
                                : dlg.busy ? qsTr("Pairing…") : qsTr("Pair the sentences")
            enabled: !dlg.leaving && (dlg.hasResult ? dlg.shown.units.length > 0 && !dlg.busy
                                                    : !dlg.busy && dlg.paths[0] !== "" && dlg.paths[1] !== "" && dlg.destinationProblem === "")
            onClicked: dlg.hasResult ? dlg.add() : dlg.pair()
        }
    ]
}
