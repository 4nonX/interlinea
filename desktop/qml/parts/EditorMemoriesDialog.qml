import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// The memories of the project's language in the editor, as the web's
// "Translation memories": how each is used, its units; the project
// memory takes imports, and each goes out as TMX or as a .txt memory.
Dialog {
    id: dlg
    property var editor
    property var units: ({})
    wide: true
    title: qsTr("Translation memories")
    readonly property var settings: editor.settings
    readonly property var mems: (settings.memories || {})[editor.targetLang] || []
    readonly property string projectMemory: settings.projectMemory ? (settings.name || "") + " (" + editor.targetLang + ")" : ""

    onOpened: { units = ({}); for (const m of mems) count(m.memory) }
    function count(name) {
        Api.get(editor.cid, "/api/memories/" + Api.seg(name), r => {
            const u = Object.assign({}, units)
            u[name] = r.ok ? Fmt.num(r.data.units || 0) : "–"
            units = u
        })
    }
    function uses(m) {
        const out = []
        if (m.lookup) out.push(m.penalty ? Fmt.fill(qsTr("lookup (penalty {n})"), { n: m.penalty }) : qsTr("lookup"))
        if (m.concordance) out.push(qsTr("concordance"))
        if (m.update) out.push(qsTr("updated on confirm"))
        return out.join(", ")
    }
    function importNotice(st, memoryName, file) {
        const u = n => Fmt.plural(n, qsTr("{n} unit"), qsTr("{n} units"))
        if (!st.stored && st.note) return Fmt.fill(qsTr("Nothing was imported from {file}. {note}"), { file: file, note: st.note })
        if (st.skipped > 0)
            return Fmt.fill(qsTr("Imported {units} from {file} into {memory}; {skipped} without both languages skipped"), { units: u(st.stored || 0), file: file, memory: memoryName, skipped: Fmt.num(st.skipped) })
        return Fmt.fill(qsTr("Imported {units} from {file} into {memory}"), { units: u(st.stored || 0), file: file, memory: memoryName })
    }

    Txt {
        Layout.fillWidth: true
        role: "muted"
        textFormat: Text.StyledText
        text: Fmt.fill(qsTr("Memories for {language}, highest priority first. Matches are scored in the core, with each memory's penalty taken off. {link}"),
                       { language: Backend.languageName(dlg.editor.targetLang), link: "<a href=\"project\">" + qsTr("Change them in the project settings") + "</a>." })
        linkColor: Theme.accentText
        onLinkActivated: { dlg.close(); dlg.editor.leave(); dlg.editor.openProject(dlg.editor.cid, dlg.editor.projectId) }
        HoverHandler { cursorShape: parent.hoveredLink ? Qt.PointingHandCursor : Qt.ArrowCursor }
    }
    Txt { visible: !dlg.mems.length; Layout.fillWidth: true; role: "muted"; text: qsTr("This language of the project uses no memory, so the editor shows no matches.") }
    Table {
        visible: dlg.mems.length > 0
        Layout.fillWidth: true
        name: qsTr("Memories of this project")
        columns: [{ title: qsTr("Memory"), fill: 1 }, { title: qsTr("Used for"), width: 200 }, { title: qsTr("Units"), width: 80, align: "right" }, { title: "", width: 300, align: "right" }]
        Repeater {
            model: dlg.mems
            TableRow {
                id: mrow
                required property var modelData
                readonly property string name: modelData.memory
                readonly property bool isProject: name === dlg.projectMemory
                CellTitle { title: mrow.name; sub: mrow.isProject ? qsTr("project memory") : "" }
                Txt { text: dlg.uses(mrow.modelData); wrapMode: Text.Wrap }
                Txt { text: dlg.units[mrow.name] || "…"; horizontalAlignment: Text.AlignRight }
                // At the right of their column, and onto a second line
                // rather than over the units where the column is too narrow.
                Flow {
                    spacing: Theme.s1
                    layoutDirection: Qt.RightToLeft
                    Button { small: true; kind: "quiet"; iconName: "download"; text: qsTr("Export .txt"); Accessible.name: Fmt.fill(qsTr("Export {name} as a .txt memory"), { name: mrow.name }); onClicked: dlg.askExport(mrow.name, "wordfast") }
                    Button { small: true; kind: "quiet"; iconName: "download"; text: qsTr("Export TMX"); onClicked: dlg.askExport(mrow.name, "tmx") }
                    Button {
                        visible: mrow.isProject
                        small: true; iconName: "upload"; text: qsTr("Import")
                        Accessible.name: Fmt.fill(qsTr("Import a memory file into {name}"), { name: mrow.name })
                        ToolTip { visible: parent.hovered; text: qsTr("A memory file: .tmx, .sdltm or .txt.") }
                        onClicked: { importDialog.memoryName = mrow.name; importDialog.open() }
                    }
                }
            }
        }
    }

    buttons: [ Button { small: true; kind: "primary"; text: qsTr("Close"); onClicked: dlg.close() } ]

    function askExport(name, format) {
        exportDialog.memoryName = name
        exportDialog.format = format
        exportDialog.selectedFile = "file:" + Backend.fileSafe(name) + (format === "wordfast" ? ".txt" : ".tmx")
        exportDialog.open()
    }
    FileDialog {
        id: importDialog
        property string memoryName
        title: Fmt.fill(qsTr("Import a memory file into {name}"), { name: memoryName })
        nameFilters: [qsTr("Memory files (*.tmx *.sdltm *.txt)"), ".tmx (*.tmx)", ".sdltm (*.sdltm)", ".txt (*.txt)", qsTr("All files (*)")]
        onAccepted: {
            const path = Backend.localPath(selectedFile), file = Backend.fileName(path), name = memoryName
            Backend.upload(dlg.editor.cid, "POST", "/api/memories/" + Api.seg(name) + "/import", path).done.connect(r => {
                if (!r.ok) { dlg.editor.say(Fmt.fill(qsTr("Could not import {file}: {error}"), { file: file, error: r.error.message })); return }
                dlg.editor.say(dlg.importNotice(r.data, name, file))
                dlg.count(name)
            })
        }
    }
    FileDialog {
        id: exportDialog
        property string memoryName
        property string format: "tmx"
        fileMode: FileDialog.SaveFile
        title: format === "wordfast" ? Fmt.fill(qsTr("Export {name} as a .txt memory"), { name: memoryName }) : qsTr("Export TMX")
        nameFilters: format === "wordfast" ? [qsTr(".txt memories (*.txt)")] : [qsTr("TMX files (*.tmx)")]
        defaultSuffix: format === "wordfast" ? "txt" : "tmx"
        onAccepted: {
            const path = Backend.localPath(selectedFile)
            Backend.download(dlg.editor.cid, dlg.editor.base + "/memories/" + Api.seg(memoryName) + "/export", path, format === "wordfast" ? { format: "wordfast" } : {}).done.connect(r => {
                dlg.editor.say(!r.ok ? r.error.message : !r.saved ? Fmt.fill(qsTr("Could not write {file}."), { file: path }) : Fmt.fill(qsTr("Saved {file}"), { file: Backend.fileName(path) }))
            })
        }
    }
}
