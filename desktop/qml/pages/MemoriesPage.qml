import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import QtQml.Models
import Interlinea

// Memories (web/src/screens/Memories.tsx, ServerMemories.tsx): the
// memories on each team server and on this computer, one opened to search
// and change its units, new memories, memory files in and out, and on a
// server who may use them.
Page {
    id: page
    title: qsTr("Memories")
    description: qsTr("Translation memories, in the core. Projects choose which to use for each target language.")

    signal notice(string text)
    signal align(string connectionId, var into, var taken, string memory)

    // Per connection: { memories, projects, templates, error, read }.
    property var places: ({})
    // The memory opened at each place, and what its search found.
    property var opened: ({})

    function put(map, k, v) { return Object.assign({}, map, { [k]: v }) }
    function setPlace(cid, key, value) {
        const p = Object.assign({}, places[cid] || {})
        p[key] = value
        places = put(places, cid, p)
    }
    function same(a, b) { return (a || "").toLowerCase() === (b || "").toLowerCase() }
    function uses(memories, name) {
        for (const lang in (memories || {}))
            for (const m of memories[lang] || [])
                if (same(m.memory, name)) return true
        return false
    }
    // A project's own memory is named after it and its language, and is
    // the project's to fill.
    function ownerProject(cid, name) {
        for (const p of (places[cid] || {}).projects || []) {
            const s = p.settings || {}
            if (!s.projectMemory) continue
            for (const t of s.targetLangs || [])
                if (same(s.name + " (" + t + ")", name) && uses({ [t]: (s.memories || {})[t] }, name))
                    return s.name
        }
        return ""
    }
    function usedBy(cid, name) {
        const out = []
        for (const p of (places[cid] || {}).projects || [])
            if (uses((p.settings || {}).memories, name)) out.push(p.settings.name)
        for (const t of (places[cid] || {}).templates || [])
            if (uses(t.memories, name)) out.push(Fmt.fill(qsTr("template {name}"), { name: t.name }))
        return out
    }
    function units(n) { return Fmt.plural(n, qsTr("{n} unit"), qsTr("{n} units")) }
    function memory(cid, name) { return ((places[cid] || {}).memories || []).find(m => same(m.name, name)) || null }

    function load(cid) {
        const c = Backend.connection(cid)
        if (!c || !c.ready) return
        Api.get(cid, "/api/memories", r => {
            setPlace(cid, "error", r.ok ? "" : r.error.message)
            if (r.ok) setPlace(cid, "memories", r.data)
            setPlace(cid, "read", true)
            if (opened[cid] && !memory(cid, opened[cid])) open(cid, "")
        })
        if (!c.local) return
        Api.get(cid, "/api/projects", r => { if (r.ok) setPlace(cid, "projects", r.data) })
        Api.get(cid, "/api/templates", r => { if (r.ok) setPlace(cid, "templates", r.data) })
    }
    function reload() { for (const c of Backend.connections) load(c.id) }
    Component.onCompleted: reload()
    Timer { id: soon; interval: 150; onTriggered: page.reload() }
    Connections {
        target: Backend
        function onEvent(cid, type) { if (type === "memory" || type === "account" || type === "project") soon.restart() }
        function onReset(cid) { soon.restart() }
        function onConnectionsChanged() { page.reload() }
    }
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() { page.load(modelData.id) }
        }
    }

    function open(cid, name) {
        opened = put(opened, cid, name)
    }

    // Memory files in and out.
    property var fileJob: null
    function importFile(cid, name, url) {
        const path = Backend.localPath(url)
        const file = Backend.fileName(path)
        const local = Backend.connection(cid).local
        Backend.upload(cid, "POST", "/api/memories/" + Api.seg(name) + "/import", path).done.connect(r => {
            if (!r.ok) {
                page.notice(local ? Fmt.fill(qsTr("Could not import {file}: {error}"), { file: file, error: r.error.message }) : r.error.message)
                return
            }
            page.notice(importNotice(r.data, name, file))
            page.reload()
        })
    }
    function importNotice(st, memoryName, file) {
        if (!st.stored && st.note)
            return Fmt.fill(qsTr("Nothing was imported from {file}. {note}"), { file: file, note: st.note })
        let text = Fmt.fill(qsTr("Imported {units} from {file} into {memory}"), { units: units(st.stored || 0), file: file, memory: memoryName })
        if (st.skipped > 0)
            text = Fmt.fill(qsTr("Imported {units} from {file} into {memory}; {skipped} without both languages skipped"),
                            { units: units(st.stored || 0), file: file, memory: memoryName, skipped: Fmt.num(st.skipped) })
        return text
    }
    function exportFile(cid, name, url, format) {
        const path = Backend.localPath(url)
        Backend.download(cid, "/api/memories/" + Api.seg(name) + "/export", path, format === "wordfast" ? { format: "wordfast" } : {}).done.connect(r => {
            page.notice(!r.ok ? r.error.message : !r.saved ? Fmt.fill(qsTr("{file} could not be saved: {error}"), { file: Backend.fileName(path), error: r.saveError })
                              : Fmt.fill(qsTr("Saved {file}"), { file: Backend.fileName(path) }))
        })
    }
    FileDialog {
        id: importDialog
        property string cid
        property string memoryName
        title: Fmt.fill(qsTr("Import a memory file into {name}"), { name: memoryName })
        nameFilters: [qsTr("Memory files (*.tmx *.sdltm *.txt)"), ".tmx (*.tmx)", ".sdltm (*.sdltm)", ".txt (*.txt)", qsTr("All files (*)")]
        onAccepted: page.importFile(cid, memoryName, selectedFile)
    }
    FileDialog {
        id: exportDialog
        property string cid
        property string memoryName
        property string format: "tmx"
        fileMode: FileDialog.SaveFile
        title: format === "wordfast" ? Fmt.fill(qsTr("Export {name} as a .txt memory"), { name: memoryName }) : Fmt.fill(qsTr("Export {name} as TMX"), { name: memoryName })
        nameFilters: format === "wordfast" ? [qsTr(".txt memories (*.txt)"), qsTr("All files (*)")] : [qsTr("TMX files (*.tmx)"), qsTr("All files (*)")]
        defaultSuffix: format === "wordfast" ? "txt" : "tmx"
        onAccepted: page.exportFile(cid, memoryName, selectedFile, format)
    }
    function askImport(cid, name) { importDialog.cid = cid; importDialog.memoryName = name; importDialog.open() }
    function askExport(cid, name, format) {
        exportDialog.cid = cid
        exportDialog.memoryName = name
        exportDialog.format = format
        exportDialog.selectedFile = "file:" + Backend.fileSafe(name) + (format === "wordfast" ? ".txt" : ".tmx")
        exportDialog.open()
    }
    function deleteMemory(cid, name, said) {
        Api.del(cid, "/api/memories/" + Api.seg(name), r => {
            page.notice(r.ok ? said : r.error.message)
            if (r.ok && same(opened[cid], name)) page.open(cid, "")
            page.reload()
        })
    }

    // The team servers first, then this computer.
    Repeater {
        model: Backend.connections.filter(c => !c.local && c.ready).concat(Backend.connections.filter(c => c.local && c.ready))
        ColumnLayout {
            id: place
            required property var modelData
            readonly property string cid: modelData.id
            readonly property bool local: modelData.local
            readonly property var data_: page.places[cid] || {}
            readonly property var memories: data_.memories || []
            readonly property var main: local ? memories.filter(m => !page.ownerProject(cid, m.name)) : memories
            readonly property var own: local ? memories.filter(m => !!page.ownerProject(cid, m.name)) : []
            readonly property string openName: page.opened[cid] || ""
            readonly property var openMemory: openName ? page.memory(cid, openName) : null
            Layout.fillWidth: true
            spacing: Theme.s5

            Section {
                title: place.local ? qsTr("Main memories on this computer")
                                   : Backend.connections.filter(c => !c.local).length > 1 ? Fmt.fill(qsTr("On the team server {host}"), { host: place.modelData.displayName })
                                   : qsTr("On the team server")
                Txt { Layout.fillWidth: true; visible: (place.data_.error || "") !== ""; role: "error"; text: place.data_.error || "" }
                Txt {
                    Layout.fillWidth: true
                    visible: !place.data_.read
                    role: "muted"
                    text: place.local ? qsTr("Loading the memories…") : Fmt.fill(qsTr("Loading the memories on {host}…"), { host: place.modelData.displayName })
                }
                Txt {
                    Layout.fillWidth: true
                    visible: !!place.data_.read && place.main.length === 0
                    role: "muted"
                    text: place.local ? qsTr("No memory yet.") : Fmt.fill(qsTr("No memories on {host} for you yet."), { host: place.modelData.displayName })
                }
                MemoryTable {
                    visible: place.main.length > 0
                    place: place
                    list: place.main
                    mainList: true
                }
                Txt {
                    Layout.fillWidth: true
                    visible: place.local && place.main.length > 0
                    role: "help"
                    text: qsTr("A memory a project or template uses can't be deleted; remove it there first.")
                }
            }
            Section {
                visible: place.local && place.own.length > 0
                title: qsTr("Project memories")
                Txt {
                    Layout.fillWidth: true
                    role: "help"
                    text: qsTr("Each collects one project's confirmed translations. Add them to a main memory from the project's settings; they go when the project is deleted.")
                }
                MemoryTable { place: place; list: place.own; mainList: false }
            }

            // The opened memory: its units searched and changed.
            MemoryDetail {
                visible: !!place.openMemory
                cid: place.cid
                local: place.local
                memory: place.openMemory
                onNotice: text => page.notice(text)
                onReloadAll: page.reload()
                onClosed: page.open(place.cid, "")
                onAskImport: page.askImport(place.cid, place.openName)
                onAskExport: format => page.askExport(place.cid, place.openName, format)
                onAlign: page.align(place.cid, [place.openMemory], [], place.openName)
            }
            AccessPanel {
                visible: !place.local && !!place.openMemory && Roles.allows(place.openMemory.role, "manage")
                title: Fmt.fill(qsTr("Who may use {name}"), { name: place.openName })
                cid: place.cid
                kind: "memory"
                resource: place.openName
            }

            // New memories.
            NewMemory {
                cid: place.cid
                local: place.local
                host: place.modelData.displayName
                exists: name => !!page.memory(place.cid, name)
                onNotice: text => page.notice(text)
                onMade: name => {
                    page.reload()
                    if (!place.local) page.open(place.cid, name)
                }
                onAlignHere: page.align(place.cid,
                                        place.memories.filter(m => place.local ? !page.ownerProject(place.cid, m.name) : Roles.allows(m.role, "edit")),
                                        place.memories.map(m => m.name), "")
            }

            component MemoryTable: Table {
                id: mt
                property var list: []
                property bool mainList: true
                // The place it is at: an inline component sees none of the ids around it.
                property var place: null
                name: mt.place.local ? (mainList ? qsTr("Main memories on this computer") : qsTr("Project memories"))
                                  : Fmt.fill(qsTr("Memories on {host}"), { host: mt.place.modelData.displayName })
                columns: mt.place.local ? [{ title: qsTr("Memory"), fill: 1.2 }, { title: qsTr("Languages"), width: 150 }, { title: qsTr("Units"), width: 72, align: "right" },
                                        { title: qsTr("Used by"), fill: 1 }, { title: "", width: mainList ? 400 : 300 }]
                                     : [{ title: qsTr("Memory"), fill: 1.4 }, { title: qsTr("Languages"), width: 150 }, { title: qsTr("Units"), width: 72, align: "right" },
                                        { title: qsTr("You may"), fill: 1 }, { title: "", width: 100 }]
                Repeater {
                    model: mt.list
                    TableRow {
                        id: mrow
                        required property var modelData
                        required property int index
                        readonly property bool isOpen: page.same(modelData.name, mt.place.openName)
                        readonly property var users: mt.place.local ? page.usedBy(mt.place.cid, modelData.name) : []
                        last: index === mt.list.length - 1
                        CellTitle {
                            title: mrow.modelData.name
                            sub: mt.place.local ? (mrow.modelData.client ? mrow.modelData.client + ", " : "") + Fmt.fill(qsTr("created {date}"), { date: Fmt.date(mrow.modelData.createdAt) })
                                             : Fmt.fill(qsTr("Since {date}"), { date: Fmt.date(mrow.modelData.createdAt) })
                        }
                        LangPair { source: mrow.modelData.sourceLang; targets: [mrow.modelData.targetLang] }
                        Txt { horizontalAlignment: Text.AlignRight; text: Fmt.num(mrow.modelData.units || 0) }
                        Txt {
                            text: mt.place.local ? (mrow.users.length ? mrow.users.join(", ") : qsTr("No project")) : Roles.text(mrow.modelData.role || "")
                            role: mt.place.local && !mrow.users.length ? "muted" : ""
                        }
                        Flow {
                            spacing: Theme.s1
                            Button {
                                small: true; kind: "quiet"
                                text: mrow.isOpen ? qsTr("Close") : qsTr("Open")
                                Accessible.name: Fmt.fill(mrow.isOpen ? qsTr("Close {name}") : qsTr("Open {name}"), { name: mrow.modelData.name })
                                onClicked: page.open(mt.place.cid, mrow.isOpen ? "" : mrow.modelData.name)
                            }
                            Button {
                                visible: !!mt.place.local && mt.mainList
                                small: true; iconName: "upload"
                                text: qsTr("Import")
                                Accessible.name: Fmt.fill(qsTr("Import a memory file into {name}"), { name: mrow.modelData.name })
                                ToolTip { visible: parent.hovered; text: qsTr("A memory file: .tmx, .sdltm or .txt.") }
                                onClicked: page.askImport(mt.place.cid, mrow.modelData.name)
                            }
                            Button {
                                visible: !!mt.place.local
                                small: true; kind: "quiet"; iconName: "download"
                                text: qsTr("Export TMX")
                                enabled: (mrow.modelData.units || 0) > 0
                                Accessible.name: Fmt.fill(qsTr("Export {name} as TMX"), { name: mrow.modelData.name })
                                onClicked: page.askExport(mt.place.cid, mrow.modelData.name, "tmx")
                            }
                            Button {
                                visible: !!mt.place.local
                                small: true; kind: "quiet"; iconName: "download"
                                text: qsTr("Export .txt")
                                enabled: (mrow.modelData.units || 0) > 0
                                Accessible.name: Fmt.fill(qsTr("Export {name} as a .txt memory"), { name: mrow.modelData.name })
                                onClicked: page.askExport(mt.place.cid, mrow.modelData.name, "wordfast")
                            }
                            ConfirmButton {
                                visible: !!mt.place.local && mt.mainList
                                text: qsTr("Delete")
                                iconName: "trash"
                                available: mrow.users.length === 0
                                accessibleName: Fmt.fill(qsTr("Delete {name}"), { name: mrow.modelData.name })
                                question: Fmt.fill(qsTr("Delete {name} and its {units}?"), { name: mrow.modelData.name, units: page.units(mrow.modelData.units || 0) })
                                onConfirmed: page.deleteMemory(mt.place.cid, mrow.modelData.name, Fmt.fill(qsTr("Deleted the memory {name}"), { name: mrow.modelData.name }))
                            }
                        }
                    }
                }
            }
        }
    }
}
