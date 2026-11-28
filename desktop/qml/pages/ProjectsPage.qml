import QtQuick
import QtQml.Models
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// Projects (web/src/screens/Projects.tsx): the drop zone to start a
// project from files, the projects on the team servers signed in to, and
// those on this computer.
Page {
    id: page
    title: qsTr("Projects")

    // Open a project in the editor, its settings, the review of its
    // termbase's changes, or a new project with files.
    signal openEditor(string connectionId, var projectId)
    signal openProject(string connectionId, var projectId)
    signal openMerge(string connectionId, var projectId, string termbase)
    signal newProject(var files)

    // Per connection: { projects: [...] or null while loading, error,
    // work: [assignments], words: {id: n}, changes: {id: n} }
    property var places: ({})

    function reload(cid) {
        const c = Backend.connection(cid)
        if (!c || !c.ready)
            return
        Api.get(cid, "/api/projects", r => {
            const place = Object.assign({}, places[cid] || {})
            place.error = r.ok ? "" : r.error.message
            if (r.ok)
                place.projects = r.data
            places = Object.assign({}, places, { [cid]: place })
            if (!r.ok)
                return
            if (c.local) {
                for (const p of r.data)
                    loadLocalDetails(cid, p)
            } else {
                Api.get(cid, "/api/me/assignments", a => {
                    if (!a.ok) return
                    const pl = Object.assign({}, places[cid])
                    pl.work = a.data
                    places = Object.assign({}, places, { [cid]: pl })
                })
            }
        })
    }

    // The words of a project on this computer (each file once, in its
    // first language), and how many changes its termbase has to review.
    function loadLocalDetails(cid, p) {
        Api.get(cid, "/api/projects/" + p.id + "/progress", r => {
            if (!r.ok) return
            const seen = {}
            let total = 0
            for (const fp of r.data) {
                if (seen[fp.file]) continue
                seen[fp.file] = true
                total += fp.words || 0
            }
            setDetail(cid, "words", p.id, total)
        })
        const t = (p.settings || {}).terminology || {}
        if (t.mode === "layered" && t.termbase) {
            Api.get(cid, "/api/termbases/" + Api.seg(t.termbase) + "/layered", r => {
                if (!r.ok) return
                setDetail(cid, "changes", p.id, r.data.filter(e => e.termbase === t.termbase).length)
            })
        }
    }
    function setDetail(cid, key, id, value) {
        const pl = Object.assign({}, places[cid])
        pl[key] = Object.assign({}, pl[key] || {}, { [id]: value })
        places = Object.assign({}, places, { [cid]: pl })
    }

    function reloadAll() {
        for (const c of Backend.connections)
            reload(c.id)
    }
    Component.onCompleted: reloadAll()

    // Changes come in bursts (a pre-translation): one reading follows.
    Timer {
        id: soon
        interval: 400
        property var pending: ({})
        onTriggered: {
            for (const cid in pending) page.reload(cid)
            pending = {}
        }
    }
    Connections {
        target: Backend
        function onEvent(cid, type) {
            if (type === "project" || type === "account") {
                soon.pending[cid] = true
                soon.restart()
            }
        }
        function onReset(cid) { page.reload(cid) }
        function onConnectionsChanged() { page.reloadAll() }
    }
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() { page.reload(modelData.id) }
        }
    }

    function filesLine(s, withClient) {
        const files = s.files || []
        const tr = files.filter(f => f.role === "translatable")
        const refs = files.length - tr.length
        let first = tr.length === 1 ? Backend.fileName(tr[0].name) : Fmt.plural(tr.length, qsTr("{n} file"), qsTr("{n} files"))
        if (refs)
            first = first + ", " + Fmt.plural(refs, qsTr("{n} reference file"), qsTr("{n} reference files"))
        return withClient && s.client ? s.client + ", " + first : first
    }

    function workScope(a) {
        const parts = [a.file || qsTr("all files"), a.targetLang || qsTr("all languages")]
        if (a.fromSegment)
            parts.push(a.fromSegment === a.toSegment ? Fmt.fill(qsTr("segment {id}"), { id: a.fromSegment })
                                                     : Fmt.fill(qsTr("segments {from} to {to}"), { from: a.fromSegment, to: a.toSegment || "" }))
        if (a.due)
            parts.push(Fmt.fill(qsTr("due {due}"), { due: Fmt.date(a.due) }))
        return parts.join(", ")
    }

    // The files and folders go to New project as they are: a folder's
    // files keep its name in theirs.
    function start(urls) {
        if (urls.length)
            page.newProject(Array.from(urls))
    }

    DropZone {
        Accessible.name: qsTr("Start a project from files")
        onDropped: urls => page.start(urls)
        Icon { name: "upload"; color: Theme.accent; size: 22; Layout.alignment: Qt.AlignVCenter }
        ColumnLayout {
            Layout.fillWidth: true
            Layout.minimumWidth: 240
            spacing: 2
            Txt { Layout.fillWidth: true; text: qsTr("Start a project from files"); role: "h3"; Accessible.role: Accessible.Heading }
            Txt {
                Layout.fillWidth: true
                text: qsTr("Drop files, a folder, or a project package here. Interlinea detects the source language and suggests memories and termbases for the client.")
                role: "muted"
            }
        }
        // The buttons: beside the text where they fit, under it and onto
        // more lines where the window is narrow.
        Flow {
            id: startButtons
            spacing: Theme.s2
            Layout.alignment: Qt.AlignVCenter
            // As wide as all on one line, and narrowed by the row, down to
            // nothing, where it has less: then the buttons wrap.
            Layout.minimumWidth: 0
            Layout.maximumWidth: parent ? parent.width : Number.POSITIVE_INFINITY
            Layout.preferredWidth: {
                let w = 0
                for (let i = 0; i < children.length; i++) w += children[i].implicitWidth
                return w + Math.max(0, children.length - 1) * spacing
            }
            Button { text: qsTr("Choose files"); kind: "primary"; onClicked: filesDialog.open() }
            Button { text: qsTr("Choose a folder"); iconName: "folder-open"; onClicked: folderDialog.open() }
            Button { text: qsTr("New project without files"); kind: "quiet"; onClicked: page.newProject([]) }
        }
    }
    FileDialog {
        id: filesDialog
        title: qsTr("Choose files")
        fileMode: FileDialog.OpenFiles
        onAccepted: page.start(selectedFiles)
    }
    FolderDialog {
        id: folderDialog
        title: qsTr("Choose a folder")
        onAccepted: page.start([selectedFolder])
    }

    // The team servers first, as on the web, then this computer.
    Repeater {
        model: Backend.connections.filter(c => !c.local).concat(Backend.connections.filter(c => c.local))
        Section {
            id: place
            required property var modelData
            readonly property string cid: modelData.id
            readonly property var data_: page.places[cid] || {}
            readonly property var projects: data_.projects || null
            readonly property bool local: modelData.local
            visible: local || modelData.ready || (data_.error || "") !== ""
            title: local ? qsTr("On this computer")
                         : Backend.connections.length > 2 ? Fmt.fill(qsTr("On the team server {host}"), { host: modelData.displayName })
                         : qsTr("On the team server")

            Txt {
                Layout.fillWidth: true
                visible: (place.data_.error || "") !== ""
                text: place.data_.error || ""
                role: "error"
                Accessible.role: Accessible.AlertMessage
            }
            Txt {
                Layout.fillWidth: true
                visible: place.projects === null && !place.local && (place.data_.error || "") === ""
                text: Fmt.fill(qsTr("Loading the projects on {host}…"), { host: place.modelData.displayName })
                role: "muted"
            }
            Txt {
                Layout.fillWidth: true
                visible: place.projects !== null && place.projects.length === 0
                text: place.local ? qsTr("No projects yet. Start one from files above.")
                                  : Fmt.fill(qsTr("No projects on {host} for you yet. Projects you make there, or work given to you, appear here."), { host: place.modelData.displayName })
                role: "muted"
            }

            Table {
                visible: place.projects !== null && place.projects.length > 0
                name: place.title
                columns: place.local
                    ? [{ title: qsTr("Project"), fill: 1.6 }, { title: qsTr("Languages"), width: 150 }, { title: qsTr("Terminology"), fill: 1.4 },
                       { title: qsTr("Words"), width: 72, align: "right" }, { title: qsTr("Done"), width: 68, align: "right" },
                       { title: qsTr("Due"), width: 104 }, { title: "", width: 272 }]
                    : [{ title: qsTr("Project"), fill: 1.6 }, { title: qsTr("Languages"), width: 150 }, { title: qsTr("Your work"), fill: 1.6 },
                       { title: qsTr("Done"), width: 68, align: "right" }, { title: qsTr("Due"), width: 104 }, { title: "", width: 132 }]

                Repeater {
                    model: place.projects || []
                    TableRow {
                        id: row
                        required property var modelData
                        required property int index
                        readonly property var s: modelData.settings || {}
                        readonly property var t: s.terminology || {}
                        readonly property var work: ((place.data_ || {}).work || []).filter(a => a.projectId === modelData.id)
                        readonly property int changes: ((place.data_ || {}).changes || {})[modelData.id] || 0
                        last: index === (place.projects || []).length - 1
                        Accessible.name: s.name || ""

                        CellTitle {
                            title: row.s.name || ""
                            sub: page.filesLine(row.s, !place.local)
                            link: true
                            onActivated: page.openEditor(place.cid, row.modelData.id)
                        }
                        LangPair { source: row.s.sourceLang || ""; targets: row.s.targetLangs || [] }
                        // Terminology here; the person's work on a server.
                        Item {
                            implicitHeight: !!place.local ? termCell.implicitHeight : workCell.implicitHeight
                            Column {
                                id: termCell
                                width: parent.width
                                visible: !!place.local
                                spacing: 2
                                Txt {
                                    width: parent.width
                                    role: "strong"
                                    text: row.t.mode === "direct" ? qsTr("Writes to master") : row.t.mode === "own" ? qsTr("Own termbase") : Words.get("projectTermbase")
                                }
                                Flow {
                                    width: parent.width
                                    spacing: 4
                                    Txt {
                                        role: "sub"
                                        wrapMode: Text.NoWrap
                                        text: (row.t.masters || []).length
                                              ? (row.t.mode === "direct" ? "" : Words.get("basedOn") + " ") + row.t.masters.join(", ")
                                              : qsTr("No master")
                                    }
                                    Badge { text: qsTr("Local"); iconName: "hard-drive" }
                                }
                            }
                            Column {
                                id: workCell
                                width: parent.width
                                visible: !place.local
                                spacing: 2
                                readonly property var tasks: (row.work || []).map(a => a.task === "translate" ? qsTr("Translating") : qsTr("Reviewing"))
                                                                              .filter((x, i, xs) => xs.indexOf(x) === i)
                                readonly property string role_: (row.modelData.access || {}).role || ""
                                Txt { width: parent.width; visible: (row.work || []).length > 0; role: "strong"; text: Fmt.listOf(workCell.tasks) }
                                Txt {
                                    width: parent.width
                                    visible: (row.work || []).length > 0
                                    role: "sub"
                                    text: (row.work || []).map(a => page.workScope(a)).join("; ") + ((row.work || []).length && row.work.every(a => a.done) ? " · " + qsTr("Done") : "")
                                }
                                Txt {
                                    width: parent.width
                                    visible: (row.work || []).length === 0 && workCell.role_ !== ""
                                    role: Roles.allows(workCell.role_, "manage") ? "" : "muted"
                                    text: Roles.allows(workCell.role_, "manage") ? qsTr("Managing") : qsTr("No work given to you")
                                }
                            }
                        }
                        Txt {
                            visible: !!place.local
                            horizontalAlignment: Text.AlignRight
                            text: ((place.data_ || {}).words || {})[row.modelData.id] === undefined ? "…" : Fmt.num((place.data_ || {}).words[row.modelData.id])
                        }
                        Txt {
                            horizontalAlignment: Text.AlignRight
                            text: Fmt.percent(row.modelData.confirmed || 0, row.modelData.segments || 0) + "%"
                        }
                        Txt {
                            text: row.s.due ? Fmt.date(row.s.due) : qsTr("None")
                            role: row.s.due ? "" : "muted"
                        }
                        Flow {
                            spacing: Theme.s2
                            layoutDirection: Qt.RightToLeft
                            Button {
                                small: true
                                kind: "quiet"
                                iconName: "sliders"
                                text: qsTr("Settings")
                                Accessible.name: Fmt.fill(qsTr("Settings of {name}"), { name: row.s.name })
                                onClicked: page.openProject(place.cid, row.modelData.id)
                            }
                            Button {
                                visible: !!place.local && row.changes > 0
                                small: true
                                iconName: "git-merge"
                                text: Words.get("reviewMerge") + " (" + row.changes + ")"
                                onClicked: page.openMerge(place.cid, row.modelData.id, row.t.termbase)
                            }
                        }
                    }
                }
            }
        }
    }
}
