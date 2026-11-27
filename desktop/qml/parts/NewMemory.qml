import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// Making a memory: on this computer with its name, client and languages,
// filled from a memory file if one is chosen; on a server with its name and
// languages. Beside it, aligning documents into a memory.
ColumnLayout {
    id: nm
    property string cid
    property bool local: true
    property string host
    property var exists: function(name) { return false }
    property bool opened: false
    property string file: ""
    property string error: ""
    signal notice(string text)
    signal made(string name)
    signal alignHere()
    Layout.fillWidth: true
    spacing: Theme.s3

    function problem() {
        const n = name.text.trim()
        if (!n) return qsTr("Give the memory a name.")
        if (exists(n)) return qsTr("A memory with this name exists already.")
        if (!source.language || !target.language) return qsTr("Choose a source and a target language.")
        if (source.language === target.language) return qsTr("Source and target language must differ.")
        return ""
    }
    function reset() {
        opened = false
        file = ""
        error = ""
        name.text = ""
        client.text = ""
    }
    // The file's own name and languages, where it gives them, fill what is
    // still as it was.
    function choose(url) {
        file = Backend.localPath(url)
        const path = file
        Backend.upload(cid, "POST", "/api/memoryfiles/inspect", path).done.connect(r => {
            if (path !== file || !r.ok) return
            if (!name.text.trim() && r.data.name) name.text = r.data.name
            if (r.data.sourceLang) source.pick(r.data.sourceLang)
            if (r.data.targetLang) target.pick(r.data.targetLang)
        })
    }
    function submit() {
        if (problem()) return
        const made = name.text.trim()
        error = ""
        const body = { name: made, sourceLang: source.language, targetLang: target.language }
        if (local) body.client = client.text.trim()
        Api.post(cid, "/api/memories", body, r => {
            if (!r.ok) {
                error = !local && r.status === 403 ? qsTr("You may not make memories on the server. Ask an administrator for the manage role on all memories.") : r.error.message
                return
            }
            const madeName = r.data.name || made
            if (!local || !file) {
                nm.notice(local ? Fmt.fill(qsTr("Created the memory {name}"), { name: madeName }) : Fmt.fill(qsTr("Made the memory {name} on {host}"), { name: madeName, host: host }))
                reset()
                nm.made(madeName)
                return
            }
            const at = "/api/memories/" + Api.seg(madeName)
            const path = file
            Backend.upload(cid, "POST", at + "/import", path).done.connect(i => {
                if (i.ok) {
                    nm.notice(i.data.stored > 0 ? Fmt.fill(qsTr("Created the memory {name} with {units}"), { name: madeName, units: Fmt.plural(i.data.stored, qsTr("{n} unit"), qsTr("{n} units")) })
                                                : Fmt.fill(qsTr("Created the memory {name}"), { name: madeName }))
                    reset()
                    nm.made(madeName)
                    return
                }
                // No memory is left behind for a file that can't be read.
                const why = i.error.message
                Api.del(cid, at, () => {
                    error = Fmt.fill(qsTr("Could not read {file}: {error}"), { file: Backend.fileName(path), error: why })
                    nm.made("")
                })
            })
        })
    }

    Row {
        visible: !nm.opened
        spacing: Theme.s2
        Button { small: nm.local; iconName: nm.local ? "plus" : ""; text: nm.local ? qsTr("New memory") : qsTr("New memory on the server"); onClicked: { nm.opened = true; name.forceActiveFocus() } }
        Button { small: nm.local; iconName: nm.local ? "columns" : ""; text: qsTr("Align documents"); onClicked: nm.alignHere() }
    }
    Panel {
        visible: nm.opened
        title: nm.local ? qsTr("New memory") : qsTr("New memory on the server")
        GridLayout {
            Layout.fillWidth: true
            columns: 2
            columnSpacing: Theme.s4
            rowSpacing: Theme.s4
            Field { label: qsTr("Name"); TextField { id: name; onAccepted: nm.submit() } }
            Field { visible: nm.local; label: qsTr("Client"); optional: true; TextField { id: client; onAccepted: nm.submit() } }
            Field { label: nm.local ? qsTr("Source language") : qsTr("From"); LanguageField { id: source; language: "de-DE" } }
            Field { label: nm.local ? qsTr("Target language") : qsTr("To"); LanguageField { id: target; language: "en-GB" } }
            Field {
                visible: nm.local
                label: qsTr("Fill it from a memory file")
                optional: true
                help: qsTr("A memory file: .tmx, .sdltm or .txt.")
                Row {
                    spacing: Theme.s2
                    Button { small: true; text: qsTr("Choose file"); Accessible.name: qsTr("Fill it from a memory file"); onClicked: fileDialog.open() }
                    Txt { anchors.verticalCenter: parent.verticalCenter; role: "muted"; wrapMode: Text.NoWrap; text: nm.file ? Backend.fileName(nm.file) : qsTr("No file chosen") }
                }
            }
        }
        Txt {
            Layout.fillWidth: true
            readonly property string why: nm.problem()
            visible: (why !== "" && name.text !== "") || nm.error !== ""
            role: "error"
            text: nm.error || why
        }
        Row {
            spacing: Theme.s2
            Button { kind: "primary"; small: true; text: nm.local ? qsTr("Create memory") : qsTr("Make the memory"); enabled: nm.problem() === ""; onClicked: nm.submit() }
            Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: nm.reset() }
        }
        Txt {
            Layout.fillWidth: true
            visible: nm.local
            role: "help"
            text: Fmt.fill(qsTr("{source} to {target}. Projects with these languages can use it."), { source: Backend.languageName(source.language), target: Backend.languageName(target.language) })
        }
    }
    FileDialog {
        id: fileDialog
        title: qsTr("Fill it from a memory file")
        nameFilters: [qsTr("Memory files (*.tmx *.sdltm *.txt)"), qsTr("All files (*)")]
        onAccepted: nm.choose(selectedFile)
    }
}
