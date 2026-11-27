import QtQuick
import QtQuick.Layouts
import Interlinea

// Reviewing a merge request of a project termbase (the web's
// ServerMergeReview): how far the reviewers are, who reviews which part
// on a team server, and each entry the project submitted with what it
// changed, its conflicts with the master and the decision on it.
Page {
    id: page
    title: read ? (request.title || "") : qsTr("Review")
    headerActions: [
        Button { kind: "quiet"; small: true; text: qsTr("Back to the termbase"); onClicked: page.back(page.cid, page.request.project || "") }
    ]

    signal notice(string text)
    signal back(string connectionId, string termbase)

    property string cid: ""
    property int requestId: 0
    property int view: 0
    property int limit: 50
    property bool mine: false
    property bool read: false
    property string problem: ""
    property var request: ({})
    property var progress: ({})
    property var target: ({})
    property var items: []
    property int total: 0
    property var reviewers: []
    property var people: []
    // What is being written for each entry, kept while the page is read again.
    property var comments: ({})
    property string assignWho: ""
    property string assignPart: ""
    property int reading: 0

    readonly property var conn: Backend.connection(cid)
    readonly property bool team: !!conn && !conn.local
    readonly property bool isOpen: request.state === "open"
    readonly property string targetName: request.target || ""
    readonly property var states: progress.states || {}
    readonly property int waiting: states.waiting || 0
    readonly property int count: progress.total || 0
    readonly property string base: "/api/merge-requests/" + requestId

    function open(connectionId, id) {
        cid = connectionId
        requestId = id
        view = 0
        limit = 50
        mine = false
        read = false
        request = ({}); progress = ({}); target = ({}); items = []; total = 0
        reviewers = []; people = []; comments = ({})
        assignWho = ""; assignPart = ""; problem = ""
        reload()
    }
    function setView(v) { if (view === v) return; view = v; limit = 50; reload() }
    function setMine(m) { if (mine === m) return; mine = m; limit = 50; reload() }
    // On this computer everything is one's own.
    function may(permission) {
        const role = target.role || ""
        return role === "" ? !team : Roles.allows(role, permission)
    }
    function day(iso) { return iso ? Backend.date(iso) : "" }
    function stateText(s) {
        return ({
            "waiting": qsTr("Waiting"), "approved": qsTr("Approved"), "approved-with-edits": qsTr("Approved with edits"),
            "changes-requested": qsTr("Changes requested"), "rejected": qsTr("Rejected"), "postponed": qsTr("Postponed"), "reverted": qsTr("Reverted")
        })[s] || s
    }
    function nameOf(username) {
        const p = people.find(x => x.username === username)
        return p && p.name ? p.name : username
    }

    // What is read is taken in together, once all of it is there.
    function reload() {
        if (!cid || !requestId) return
        const r = ++reading
        let left = team ? 6 : 4
        let failed = ""
        const got = {}
        const done = res => {
            if (res && !res.ok && !failed) failed = res.error.message
            if (--left > 0 || r !== reading) return
            // Read again with nothing new, as after someone's change elsewhere
            // in the termbase: the page stays, with what is being written.
            for (const k in got) if (JSON.stringify(page[k]) !== JSON.stringify(got[k])) page[k] = got[k]
            problem = failed
            read = failed === ""
        }
        Api.get(cid, base, res => {
            if (res.ok) got.request = res.data
            done(res)
            // The master says what one may do with the request.
            if (!res.ok || r !== reading) { done(null); return }
            Api.get(cid, "/api/termbases/" + Api.seg(res.data.target), t => {
                if (t.ok) got.target = t.data
                // Without a look at the master the request is still shown.
                done(null)
            })
        })
        if (team) {
            Api.get(cid, base + "/reviewers", res => { if (res.ok) got.reviewers = res.data; done(res) })
            Api.get(cid, "/api/people", res => { if (res.ok) got.people = res.data; done(null) })
        }
        Api.get(cid, base + "/progress", res => { if (res.ok) got.progress = res.data; done(res) })
        const filter = view === 0 ? { states: ["waiting"] }
                     : view === 1 ? { states: ["approved", "approved-with-edits", "changes-requested", "rejected", "postponed", "reverted"] } : {}
        const query = { filter: filter, limit: limit, offset: 0 }
        if (mine && conn) query.reviewer = conn.login
        Api.post(cid, base + "/items", query, res => {
            if (res.ok) { got.items = res.data.items || []; got.total = res.data.total || 0 }
            done(res)
        })
    }
    function decide(filter, state, edited, what) {
        const decision = { state: state }
        if (edited) decision.edited = edited
        // What was written for the one entry goes with its decision.
        if ((filter.ids || []).length === 1) {
            const id = filter.ids[0]
            const c = (comments[id] || "").trim()
            if (c) decision.comment = c
            const next = Object.assign({}, comments)
            delete next[id]
            comments = next
        }
        const tname = targetName
        Api.post(cid, base + "/decide", { filter: filter, decision: decision }, res => {
            if (!res.ok) {
                notice(res.status === 403 ? Fmt.fill(qsTr("{problem} Ask a manager of {termbase} for the approve role."), { problem: res.error.message, termbase: tname }) : res.error.message)
                return
            }
            const left = (res.data.needResolution || []).length
            let text = (what || stateText(state)) + ": " + TermData.entries(res.data.decided || 0) + "."
            if (left) text += " " + Fmt.plural(left, qsTr("1 entry needs a decision one by one: the master changed the same places."),
                                               qsTr("{n} entries need a decision one by one: the master changed the same places."))
            notice(text)
            reload()
        })
    }
    function partText(scope) {
        const s = scope || {}, parts = []
        if ((s.langs || []).length)
            parts.push(Fmt.fill(qsTr("entries with terms in {languages}"), { languages: s.langs.map(l => Backend.languageName(l)).join(", ") }))
        for (const k of s.kinds || [])
            parts.push(k === "new" ? qsTr("new entries") : k === "changed" ? qsTr("changed entries") : qsTr("entries deleted in the project"))
        if (s.conflicts) parts.push(qsTr("entries with conflicts"))
        if ((s.authors || []).length) parts.push(Fmt.fill(qsTr("entries by {authors}"), { authors: s.authors.join(", ") }))
        return parts.length ? parts.join("; ") : qsTr("all of it")
    }

    // What others decide is shown as it happens.
    Timer { id: soon; interval: 200; onTriggered: page.reload() }
    Connections {
        target: Backend
        function onEvent(c, type) { if (c === page.cid && type === "termbase" && page.requestId) soon.restart() }
    }

    Txt {
        visible: !page.read
        Layout.fillWidth: true
        role: page.problem ? "error" : "muted"
        text: page.problem || qsTr("Loading the request…")
    }

    Txt {
        visible: page.read
        Layout.fillWidth: true
        role: "help"
        text: {
            const said = { project: page.request.project || "", termbase: page.targetName, author: page.request.createdBy || "", date: page.day(page.request.createdAt), by: page.day(page.request.deadline) }
            let about = said.by ? Fmt.fill(qsTr("{project} into {termbase}. Submitted by {author} on {date}, to be decided by {by}."), said)
                                : Fmt.fill(qsTr("{project} into {termbase}. Submitted by {author} on {date}."), said)
            if (page.read && !page.isOpen) about += " " + Fmt.fill(qsTr("Closed on {date}."), { date: page.day(page.request.closedAt || page.request.createdAt) })
            return about
        }
    }

    // Progress.
    Panel {
        visible: page.read
        title: qsTr("Progress")
        Txt {
            Layout.fillWidth: true
            text: {
                const parts = []
                for (const s of ["waiting", "approved", "approved-with-edits", "changes-requested", "rejected", "postponed", "reverted"])
                    if (page.states[s]) parts.push(Fmt.num(page.states[s]) + " " + page.stateText(s).toLowerCase())
                let t = TermData.entries(page.count) + ": " + parts.join(", ") + "."
                const c = page.progress.conflicts || 0
                if (c) t += " " + Fmt.plural(c, qsTr("1 of them conflicts with the master."), qsTr("{n} of them conflict with the master."))
                return t
            }
        }
        Rectangle {
            Layout.fillWidth: true
            implicitHeight: 6
            radius: 3
            color: Theme.dark ? Theme.border : "#e3e7ed"
            Accessible.role: Accessible.ProgressBar
            Accessible.name: qsTr("Decided")
            Rectangle {
                height: parent.height
                radius: 3
                color: Theme.ok
                width: parent.width * (page.count ? (page.count - page.waiting) / page.count : 0)
            }
        }
        Txt {
            visible: page.isOpen && !page.may("approve")
            Layout.fillWidth: true
            role: "help"
            text: Fmt.fill(qsTr("Those who may approve changes to {termbase} decide. You see how far they are."), { termbase: page.targetName })
        }
        Flow {
            visible: page.isOpen && page.may("approve")
            Layout.fillWidth: true
            spacing: Theme.s2
            Button {
                kind: "primary"
                iconName: "check"
                text: qsTr("Approve all waiting without conflicts")
                enabled: page.waiting > 0
                onClicked: page.decide({ states: ["waiting"], conflicts: false }, "approved", null, qsTr("Approved all waiting entries without conflicts"))
            }
            ConfirmButton {
                small: false
                danger: false
                text: qsTr("Close the request")
                action: qsTr("Close")
                question: page.waiting === 0 ? qsTr("Close the request?")
                        : page.waiting === 1 ? qsTr("1 entry is still waiting. Close anyway?")
                        : Fmt.fill(qsTr("{n} entries are still waiting. Close anyway?"), { n: page.waiting })
                onConfirmed: Api.post(page.cid, page.base + "/close", undefined, res => {
                    page.notice(res.ok ? qsTr("Closed the request") : res.error.message)
                    page.reload()
                })
            }
        }
    }

    // Who reviews which part of the request, on a team server.
    Panel {
        visible: page.read && page.team
        title: qsTr("Reviewers")
        Txt {
            visible: page.reviewers.length === 0
            Layout.fillWidth: true
            role: "help"
            text: Fmt.fill(qsTr("Nobody is given a part: everyone who may approve changes to {termbase} sees all of it."), { termbase: page.targetName })
        }
        Repeater {
            model: page.reviewers
            RowLayout {
                id: rrow
                required property var modelData
                readonly property string who: modelData.reviewer
                readonly property var counts: (page.progress.byReviewer || {})[who] || {}
                readonly property int all: Object.keys(counts).reduce((n, k) => n + counts[k], 0)
                readonly property int undecided: (counts.waiting || 0) + (counts.postponed || 0)
                readonly property string what: page.partText(modelData.scope)
                    + (all > 0 ? ": " + (undecided === 0 ? Fmt.fill(qsTr("all {n} decided"), { n: all }) : Fmt.fill(qsTr("{left} of {n} to decide"), { left: undecided, n: all })) : "")
                Layout.fillWidth: true
                spacing: Theme.s3
                Accessible.name: page.nameOf(who) + ", " + what
                Txt { role: "strong"; text: page.nameOf(rrow.who); wrapMode: Text.NoWrap }
                Txt { Layout.fillWidth: true; role: "sub"; text: rrow.what }
                Button {
                    visible: page.isOpen && page.may("manage")
                    kind: "quiet"
                    small: true
                    text: qsTr("Remove")
                    Accessible.name: Fmt.fill(qsTr("Take the part from {name}"), { name: page.nameOf(rrow.who) })
                    onClicked: {
                        const name = page.nameOf(rrow.who)
                        Api.call(page.cid, "DELETE", page.base + "/reviewers", undefined, res => {
                            page.notice(res.ok ? Fmt.fill(qsTr("{name} no longer has a part of the request"), { name: name }) : res.error.message)
                            page.reload()
                        }, { reviewer: rrow.who })
                    }
                }
            }
        }
        RowLayout {
            visible: page.isOpen && page.may("manage")
            spacing: Theme.s3
            Accessible.name: qsTr("Give someone a part")
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 240
                label: qsTr("Who")
                ComboBox {
                    id: whoBox
                    readonly property var values: [""].concat(page.people.map(p => p.username))
                    model: [qsTr("Choose someone")].concat(page.people.map(p => p.name ? p.name + " (" + p.username + ")" : p.username))
                    currentIndex: Math.max(0, values.indexOf(page.assignWho))
                    onActivated: i => page.assignWho = values[i]
                }
            }
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 240
                label: qsTr("Part")
                ComboBox {
                    id: partBox
                    readonly property var langs: ((page.target.definition || {}).languages) || []
                    readonly property var values: [""].concat(langs.map(l => "lang:" + l), ["kind:new", "kind:changed", "conflicts"])
                    model: [qsTr("All of it")].concat(langs.map(l => Fmt.fill(qsTr("Terms in {language}"), { language: Backend.languageName(l) })),
                                                      [qsTr("New entries"), qsTr("Changed entries"), qsTr("Entries with conflicts")])
                    currentIndex: Math.max(0, values.indexOf(page.assignPart))
                    onActivated: i => page.assignPart = values[i]
                }
            }
            Button {
                Layout.alignment: Qt.AlignBottom
                text: qsTr("Give the part")
                onClicked: {
                    const reviewer = page.assignWho
                    if (!reviewer) { page.notice(qsTr("Choose who reviews.")); return }
                    const p = page.assignPart, scope = {}
                    if (p.startsWith("lang:")) scope.langs = [p.slice(5)]
                    else if (p.startsWith("kind:")) scope.kinds = [p.slice(5)]
                    else if (p === "conflicts") scope.conflicts = true
                    const name = page.nameOf(reviewer)
                    Api.put(page.cid, page.base + "/reviewers", { reviewer: reviewer, scope: scope }, res => {
                        if (!res.ok) { page.notice(res.error.message); return }
                        page.assignWho = ""
                        page.assignPart = ""
                        page.notice(Fmt.fill(qsTr("{name} reviews a part of the request"), { name: name }))
                        page.reload()
                    })
                }
            }
            Item { Layout.fillWidth: true }
        }
        Txt {
            visible: page.isOpen && page.may("manage")
            Layout.fillWidth: true
            role: "help"
            text: Fmt.fill(qsTr("A reviewer must be able to approve changes to {termbase}. Giving someone a part again replaces the one they had."), { termbase: page.targetName })
        }
    }

    ColumnLayout {
        visible: page.read
        Layout.fillWidth: true
        spacing: Theme.s3
        Tabs {
            Layout.fillWidth: true
            inset: 0
            small: true
            name: qsTr("Entries to show")
            current: page.view
            model: [Fmt.fill(qsTr("Waiting ({n})"), { n: page.waiting }), qsTr("Decided"), qsTr("All")]
            onChanged: i => page.setView(i)
        }
        // Someone given a part of the request can look at that part alone.
        CheckBox {
            visible: page.mine || (!!page.conn && page.reviewers.some(a => a.reviewer === page.conn.login))
            text: qsTr("Only my part")
            checked: page.mine
            onToggled: page.setMine(checked)
        }
        Txt {
            visible: page.items.length === 0
            role: "muted"
            text: page.view === 0 ? qsTr("Nothing is waiting.") : qsTr("No entries here.")
        }
        Repeater {
            model: page.items
            ReviewItem {
                required property var modelData
                review: page
                item: modelData
            }
        }
        RowLayout {
            visible: page.total > page.items.length
            spacing: Theme.s3
            Txt { role: "sub"; text: Fmt.fill(qsTr("Showing the first {shown} of {total}."), { shown: page.items.length, total: page.total }) }
            Button { small: true; text: qsTr("Show more"); onClicked: { page.limit += 50; page.reload() } }
        }
    }
}
