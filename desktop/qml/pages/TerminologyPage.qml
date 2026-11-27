import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import QtQml.Models
import Interlinea

// Terminology (web/src/screens/Terminology.tsx, LocalTermbase.tsx,
// ServerTermbase.tsx): the termbases on the left (masters, project
// termbases, those on each team server), the entries of the one opened in
// the middle with search, duplicates, deleted entries and the master's
// entries, and the chosen entry on the right. Everything is read from and
// saved to the core of the place the termbase lives in.
Item {
    id: tb
    signal notice(string text)
    // A merge request of a termbase is to be reviewed.
    signal openReview(string connectionId, string termbase, int mergeRequest)

    // The termbases of each place, by connection id.
    property var lists: ({})
    // The opened termbase.
    property string cid: ""
    property string name: ""
    property var detail: ({})
    property var entries: []
    property var masterEntries: []
    property var bases: ({})
    property var requests: []
    property bool opened: false
    property bool loading: false
    property string problem: ""
    property string selected: ""
    property string wantEntry: ""
    property int view: 0
    property string query: ""
    property string status: ""
    property var duplicates: null
    property string dupLang: ""
    property var dismissed: []
    property var deleted: null
    property var removals: []
    property string masterSelected: ""
    property int reading: 0
    // Changes waiting to be saved, one after the other.
    property var queue: []
    property bool saving: false

    readonly property bool isTeam: { const c = Backend.connection(cid); return !!c && !c.local }
    readonly property bool isProject: detail.kind === "project"
    readonly property var languages: ((detail.definition || {}).languages) || []
    readonly property var fields: ((detail.definition || {}).fields) || []
    readonly property var upstreams: detail.upstreams || []
    readonly property var shown: entries.filter(e => (!status || TermData.hasStatus(e, status)) && (!query.trim() || TermData.searchText(e).indexOf(query.trim().toLowerCase()) >= 0))
    readonly property var selectedEntry: entries.find(e => e.id === selected) || null
    readonly property var selectedMaster: masterEntries.find(e => e.termbase + "\n" + e.id === masterSelected) || null

    function may(permission) {
        const role = detail.role || ""
        return role === "" ? !isTeam : Roles.allows(role, permission)
    }
    function path(rest) { return "/api/termbases/" + Api.seg(name) + (rest || "") }

    // Reading.
    function reload() {
        const ready = Backend.connections.filter(c => c.ready)
        const r = ++reading
        let left = ready.length
        if (!left) { lists = {}; opened = false; detail = {}; entries = []; return }
        const got = {}
        for (const c of ready) {
            const id = c.id
            Api.get(id, "/api/termbases", res => {
                if (r !== reading) return
                if (res.ok) got[id] = res.data
                if (--left > 0) return
                lists = got
                pickDefault()
                loadTermbase()
            })
        }
    }
    // The termbase shown is still there, or the first master of this
    // computer is shown, else the first termbase of any place.
    function pickDefault() {
        const has = (c, n) => (lists[c] || []).some(t => t.name === n)
        if (name && has(cid, name)) return
        cid = ""; name = ""
        const order = [Backend.local].concat(Backend.connections.filter(c => !c.local))
        for (let pass = 0; pass < 2 && !name; pass++)
            for (const c of order) {
                const t = (lists[c.id] || []).find(t => pass > 0 || t.kind === "master")
                if (t) { cid = c.id; name = t.name; break }
            }
    }
    function open(connectionId, termbase, entryId) {
        const same = cid === connectionId && name === termbase
        cid = connectionId
        name = termbase
        wantEntry = entryId || ""
        if (!same) { selected = ""; view = 0; dismissed = []; dupLang = ""; newEntry.opened = false }
        ++reading
        loadTermbase()
    }
    function loadTermbase() {
        const r = reading
        problem = ""
        if (!cid || !name) { opened = false; detail = {}; entries = []; return }
        loading = true
        Api.get(cid, path(), res => {
            if (r !== reading) return
            if (!res.ok) { loading = false; opened = false; detail = {}; entries = []; problem = res.error.message; return }
            const d = res.data
            // A layered termbase without languages of its own has its masters'.
            if (!((d.definition || {}).languages || []).length) {
                const langs = []
                for (const u of d.upstreams || [])
                    for (const t of lists[cid] || [])
                        if (t.name === u) for (const l of t.languages || []) if (langs.indexOf(l) < 0) langs.push(l)
                d.definition = Object.assign({}, d.definition || {}, { languages: langs })
            }
            loadEntries(d, [], 0, r)
        })
    }
    function loadEntries(d, got, offset, r) {
        Api.call(cid, "GET", path("/entries"), undefined, res => {
            if (r !== reading) return
            const page = res.ok ? (res.data.entries || []) : []
            got = got.concat(page)
            if (res.ok && page.length && offset + page.length < (res.data.total || 0)) { loadEntries(d, got, offset + page.length, r); return }
            if (!res.ok) problem = res.error.message
            // What a project termbase's entries are based on, and the merge
            // requests, come with it.
            let left = 1
            const masters = [], b = {}
            let reqs = []
            const done = () => {
                if (--left > 0 || r !== reading) return
                detail = d
                entries = got
                masterEntries = masters
                bases = b
                requests = reqs
                loading = false
                opened = true
                if (wantEntry && got.some(e => e.id === wantEntry)) { selected = wantEntry; wantEntry = "" }
                if (!selectedEntry) selected = shown.length ? shown[0].id : ""
                if (view === 1) loadDuplicates()
                if (view === 2) loadDeleted()
                if (view === 3) loadRemovals()
            }
            if (d.kind === "project") {
                left++
                Api.get(cid, path("/layered"), lr => {
                    for (const o of lr.ok ? lr.data : []) {
                        if (o.termbase !== name) masters.push(o)
                        else if (o.base && o.base.termbase) b[o.id] = o.base.termbase
                    }
                    done()
                })
            }
            left++
            Api.get(cid, path("/merge-requests"), mr => { reqs = mr.ok ? mr.data : []; done() })
            done()
        }, { limit: 1000, offset: offset })
    }
    Component.onCompleted: reload()
    Timer { id: soon; interval: 150; onTriggered: tb.reload() }
    Connections {
        target: Backend
        function onEvent(c, type) { if (type === "termbase" || type === "account") soon.restart() }
        function onReset(c) { soon.restart() }
        function onConnectionsChanged() { soon.restart() }
        function onGitWordsChanged() { tb.lists = Object.assign({}, tb.lists) }
    }
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() { soon.restart() }
        }
    }

    // Changing: each change is made to the entry as the core last returned
    // it, and saved from that version, one after the other.
    function change(entryId, summary, apply, rebuild) {
        queue = queue.concat([{ entry: entryId, summary: summary, apply: apply }])
        if (!saving) sendNext()
    }
    function sendNext() {
        if (!queue.length) { saving = false; return }
        const ch = queue[0]
        queue = queue.slice(1)
        const before = entries.find(e => e.id === ch.entry)
        if (!before) { sendNext(); return }
        saving = true
        const after = ch.apply(TermData.forSaving(before))
        const r = reading
        Api.put(cid, path("/entries/" + Api.seg(ch.entry)), { entry: after, summary: ch.summary }, res => {
            if (r !== reading) { queue = []; saving = false; return }
            if (!res.ok) {
                queue = []
                saving = false
                tb.notice(Fmt.fill(qsTr("{what} was not saved: {why}"), { what: qsTr("The entry"), why: res.error.message }))
                soon.restart()
                return
            }
            replaceEntry(res.data)
            sendNext()
        }, before.version)
    }
    function replaceEntry(stored) {
        const list = entries.slice()
        const i = list.findIndex(e => e.id === stored.id)
        if (i >= 0) list[i] = stored
        else list.push(stored)
        entries = list
    }
    function deleteEntry(e, head) {
        Api.del(cid, path("/entries/" + Api.seg(e.id)), res => {
            if (!res.ok) { tb.notice(Fmt.fill(qsTr("{what} was not saved: {why}"), { what: qsTr("Deleting the entry"), why: res.error.message })); soon.restart(); return }
            entries = entries.filter(x => x.id !== e.id)
            selected = shown.length ? shown[0].id : ""
            tb.notice(head ? Fmt.fill(qsTr("Deleted the entry “{term}”"), { term: head }) : qsTr("Deleted the entry"))
        })
    }
    function revert(entryId, version, at, then) {
        Api.post(cid, path("/entries/" + Api.seg(entryId) + "/revert"), { version: version }, res => {
            if (!res.ok) { tb.notice(Fmt.fill(qsTr("The version was not restored: {error}"), { error: res.error.message })); return }
            replaceEntry(res.data)
            tb.notice(Fmt.fill(qsTr("Restored the version of {when}"), { when: at }))
            then()
        })
    }
    function putDefinition(def, done) {
        const r = reading
        Api.put(cid, path("/definition"), def, res => {
            if (!res.ok) { tb.notice(res.error.message); return }
            if (done) tb.notice(done)
            if (r !== reading) return
            detail = Object.assign({}, detail, { definition: res.data.definition })
        })
    }
    function addLanguage(lang) {
        if (languages.indexOf(lang) >= 0) return
        const def = TermData.copy(detail.definition || {})
        def.languages = (def.languages || []).concat([lang])
        putDefinition(def, "")
    }
    function addField(field) {
        const def = TermData.copy(detail.definition || {})
        def.fields = (def.fields || []).concat([field])
        putDefinition(def, Fmt.fill(qsTr("Added field “{field}” to {termbase}"), { field: field.name, termbase: name }))
    }
    function removeField(field) {
        Api.del(cid, path("/fields/" + Api.seg(field)), res => {
            if (!res.ok) { tb.notice(res.error.message); return }
            tb.notice(Fmt.fill(qsTr("Removed the field “{name}”"), { name: field }))
            reload()
        })
    }
    function override(e) {
        Api.post(cid, path("/overrides"), { master: e.termbase, entry: e.id }, res => {
            if (!res.ok) { tb.notice(res.error.message); return }
            tb.notice(qsTr("Took the entry into the project"))
            wantEntry = res.data.id
            view = 0
            reload()
        })
    }
    function leaveOut(e) {
        Api.post(cid, path("/removals"), { master: e.termbase, entry: e.id }, res => {
            if (!res.ok) { tb.notice(res.error.message); return }
            tb.notice(qsTr("Left the entry out of the project"))
            masterSelected = ""
            reload()
        })
    }
    function addEntry(text, lang, st) {
        const t = text.trim()
        if (!t) return
        const e = { languages: [{ lang: lang, terms: [{ text: t, status: st }] }] }
        Api.post(cid, path("/entries"), { entry: e, summary: qsTr("Created") }, res => {
            if (!res.ok) { tb.notice(Fmt.fill(qsTr("The new entry was not saved: {error}"), { error: res.error.message })); return }
            entries = entries.concat([res.data])
            selected = res.data.id
            if (languages.indexOf(lang) < 0) {
                const def = TermData.copy(detail.definition || {})
                def.languages = (def.languages || []).concat([lang])
                detail = Object.assign({}, detail, { definition: def })
            }
            tb.notice(Fmt.fill(qsTr("Added “{term}”"), { term: t }))
            newEntry.opened = false
        })
    }
    function loadDuplicates() {
        duplicates = null
        const r = reading, forLang = dupLang
        Api.call(cid, "GET", path("/duplicates"), undefined, res => {
            if (r !== reading || forLang !== dupLang) return
            if (!res.ok) { duplicates = { error: res.error.message }; return }
            const groups = []
            for (const g of res.data) {
                const group = g.map(id => entries.find(e => e.id === id)).filter(e => !!e)
                if (group.length < 2) continue
                const key = group.map(e => e.id).join("|")
                if (dismissed.indexOf(key) >= 0) continue
                // The oldest entry is kept, so links to it stay valid.
                group.sort((a, b) => (a.createdAt || "") < (b.createdAt || "") ? -1 : (a.createdAt || "") > (b.createdAt || "") ? 1 : 0)
                groups.push({ key: key, entries: group })
            }
            duplicates = groups
        }, dupLang ? { lang: dupLang } : {})
    }
    function mergeGroup(group) {
        const keep = group.entries[0].id, others = group.entries.slice(1).map(e => e.id)
        Api.post(cid, path("/merge"), { keep: keep, others: others }, res => {
            if (!res.ok) { tb.notice(res.error.message); return }
            entries = entries.filter(e => others.indexOf(e.id) < 0)
            replaceEntry(res.data)
            selected = keep
            tb.notice(Fmt.fill(qsTr("Merged {n} entries into one"), { n: group.entries.length }))
            loadDuplicates()
        })
    }
    function loadDeleted() {
        deleted = null
        const r = reading
        Api.get(cid, path("/deleted"), res => {
            if (r !== reading) return
            if (!res.ok) { deleted = { error: res.error.message }; return }
            // Of an entry deleted more than once, the last deletion counts.
            const seen = {}, gone = []
            for (const d of res.data) { if (seen[d.entryId]) continue; seen[d.entryId] = true; gone.push(d) }
            deleted = gone
        })
    }
    function bringBack(id) {
        Api.post(cid, path("/deleted/" + Api.seg(id) + "/restore"), {}, res => {
            if (!res.ok) { tb.notice(Fmt.fill(qsTr("The entry was not brought back: {error}"), { error: res.error.message })); return }
            entries = entries.concat([res.data])
            selected = res.data.id
            const head = TermData.head(res.data, languages[0] || "").text
            tb.notice(head ? Fmt.fill(qsTr("Brought back “{term}”"), { term: head }) : qsTr("Brought the entry back"))
            loadDeleted()
        })
    }
    function loadRemovals() {
        const r = reading
        Api.get(cid, path("/removals"), res => { if (r === reading) removals = res.ok ? res.data : [] })
    }
    function setView(v) {
        view = v
        if (v !== 1) dismissed = []
        if (v === 1) loadDuplicates()
        else if (v === 2) loadDeleted()
        else if (v === 3) { loadRemovals(); if (!selectedMaster && masterEntries.length) masterSelected = masterEntries[0].termbase + "\n" + masterEntries[0].id }
    }

    // The three panes, as the web's .term-layout.
    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: Theme.s6
        anchors.rightMargin: Theme.s6
        anchors.topMargin: Theme.s5
        anchors.bottomMargin: Theme.s5
        spacing: Theme.s3

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: Theme.s5
            Txt { text: qsTr("Terminology"); role: "h1"; Accessible.role: Accessible.Heading }
            // Three panes side by side (the web's .workspace); on a
            // narrower window the termbases go over the entries, and the
            // entry has the rest of the width.
            GridLayout {
                id: workspace
                Layout.fillWidth: true
                Layout.fillHeight: true
                readonly property bool wide: width >= 1000
                columns: wide ? 3 : 2
                columnSpacing: Theme.s3
                rowSpacing: Theme.s3

                TermbaseLists {
                    Layout.row: 0
                    Layout.column: 0
                    Layout.preferredWidth: workspace.wide ? 220 : 300
                    Layout.fillHeight: workspace.wide
                    Layout.preferredHeight: workspace.wide ? -1 : 220
                    page: tb
                }

                // The entries of the opened termbase.
                Rectangle {
                    Layout.row: workspace.wide ? 0 : 1
                    Layout.column: workspace.wide ? 1 : 0
                    Layout.preferredWidth: workspace.wide ? 360 : 300
                    Layout.fillHeight: true
                    radius: Theme.rPanel
                    color: Theme.surface
                    border.color: Theme.border
                    clip: true
                    ColumnLayout {
                        anchors.fill: parent
                        anchors.margins: 1
                        spacing: 0
                        TermbaseHead {
                            Layout.fillWidth: true
                            page: tb
                        }
                        Tabs {
                            visible: tb.opened
                            Layout.fillWidth: true
                            name: qsTr("Entry lists")
                            small: true
                            current: tb.view
                            model: [qsTr("Entries"),
                                    { text: qsTr("Duplicates"), icon: "copy", visible: tb.may("edit"), enabled: tb.entries.length >= 2 },
                                    qsTr("Deleted"),
                                    { text: Fmt.fill(qsTr("Master ({n})"), { n: tb.masterEntries.length }), visible: tb.isProject,
                                      accessibleName: Fmt.fill(qsTr("The master's entries ({n})"), { n: tb.masterEntries.length }) }]
                            onChanged: i => tb.setView(i)
                        }
                        EntryList {
                            id: entryList
                            visible: tb.opened && tb.view === 0
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            page: tb
                            NewEntryForm { id: newEntry; page: tb }
                        }
                        TermViews {
                            visible: tb.opened && tb.view > 0
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            page: tb
                        }
                        Item { visible: !tb.opened; Layout.fillHeight: true }
                    }
                }

                EntryDetail {
                    Layout.row: 0
                    Layout.column: workspace.wide ? 2 : 1
                    Layout.rowSpan: workspace.wide ? 1 : 2
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    tb: tb
                    entry: tb.view === 3 ? tb.selectedMaster : tb.selectedEntry
                    masterView: tb.view === 3
                    visible: tb.opened
                }
                Item {
                    visible: !tb.opened
                    Layout.row: 0
                    Layout.column: workspace.wide ? 2 : 1
                    Layout.rowSpan: workspace.wide ? 1 : 2
                    Layout.fillWidth: true
                }
            }
        }
    }
}
