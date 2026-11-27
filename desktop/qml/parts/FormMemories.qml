import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import Interlinea

// A project's memories for each target language: which, in what order,
// used for what and with what penalty. New memories are made here, empty
// or filled from a TMX file, and made with the project.
ColumnLayout {
    id: memories
    property var form
    readonly property bool own: !!form.s.projectMemory
    spacing: Theme.s4
    enabled: !form.readOnly
    Layout.fillWidth: true

    ColumnLayout {
        visible: memories.form.isNew
        Layout.fillWidth: true
        spacing: Theme.s2
        CheckBox {
            text: qsTr("Give the project its own memory for each language")
            checked: memories.own
            onToggled: memories.form.put("projectMemory", checked)
        }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: memories.own ? qsTr("Confirmed translations go into the project memory first. When the project is done, you add them to a main memory from the project's settings. The project memory is searched before the memories below.")
                               : qsTr("Confirmed translations go only into the memories below that you update.")
        }
    }

    Repeater {
        model: memories.form.targets
        ColumnLayout {
            id: set
            required property string modelData
            readonly property string lang: modelData
            readonly property var list: memories.form.memoriesFor(lang)
            readonly property var fitting: {
                const all = (memories.form.catalog.memories || []).concat(memories.form.created.map(n => ({ name: n.name, sourceLang: n.sourceLang, targetLang: n.targetLang })))
                return all.filter(m => !list.some(x => x.memory === m.name) && memories.form.samePrimary(m.sourceLang, memories.form.source) && memories.form.samePrimary(m.targetLang, lang)).map(m => m.name)
            }
            Layout.fillWidth: true
            spacing: Theme.s2
            Accessible.name: Fmt.fill(qsTr("Memories for {language}"), { language: Backend.languageName(lang) })

            Txt { role: "label"; text: Fmt.fill(qsTr("{source} to {target}"), { source: Backend.languageName(memories.form.source), target: Backend.languageName(set.lang) }) }
            Txt {
                visible: set.list.length === 0
                Layout.fillWidth: true
                role: "muted"
                text: memories.own && memories.form.isNew ? qsTr("No memory chosen. Matches come only from the project memory.")
                    : memories.form.isNew ? qsTr("No memory chosen. The editor will show no matches.") : qsTr("No memory chosen. The editor shows no matches.")
            }
            Table {
                visible: set.list.length > 0
                name: Fmt.fill(qsTr("Memories for {language}, highest priority first"), { language: Backend.languageName(set.lang) })
                columns: [{ title: qsTr("Memory"), fill: 1 }, { title: qsTr("Lookup"), width: 84 }, { title: qsTr("Concordance"), width: 104 },
                          { title: qsTr("Update"), width: 80 }, { title: qsTr("Penalty"), width: 112 }, { title: "", width: 104 }]
                Repeater {
                    // By count, so a change keeps the rows where they are.
                    model: set.list.length
                    TableRow {
                        id: mrow
                        required property int index
                        readonly property var m: set.list[index] || {}
                        readonly property string name: m.memory || ""
                        readonly property var info: memories.form.memoryInfo(name)
                        last: index === set.list.length - 1
                        function change(key, value) {
                            const a = memories.form.copy(set.list)
                            a[index][key] = value
                            memories.form.setMemories(set.lang, a)
                        }
                        function move(to) {
                            const a = memories.form.copy(set.list)
                            if (to < 0 || to >= a.length) return
                            const x = a[index]; a[index] = a[to]; a[to] = x
                            memories.form.setMemories(set.lang, a)
                        }
                        CellTitle {
                            title: mrow.name
                            sub: !mrow.info ? qsTr("This memory no longer exists.")
                                 : mrow.info.isNew ? (mrow.info.tmx ? Fmt.fill(qsTr("{source} → {target}, new, filled from {file}"), { source: mrow.info.sourceLang, target: mrow.info.targetLang, file: Backend.fileName(mrow.info.tmx) })
                                                                    : Fmt.fill(qsTr("{source} → {target}, new and empty"), { source: mrow.info.sourceLang, target: mrow.info.targetLang }))
                                 : mrow.info.sourceLang + " → " + mrow.info.targetLang + " " + Fmt.plural(mrow.info.units || 0, qsTr("{n} unit"), qsTr("{n} units"))
                        }
                        CheckBox { checked: !!mrow.m.lookup; Accessible.name: Fmt.fill(qsTr("Look up matches in {memory}"), { memory: mrow.name }); onToggled: mrow.change("lookup", checked) }
                        CheckBox { checked: !!mrow.m.concordance; Accessible.name: Fmt.fill(qsTr("Search with concordance in {memory}"), { memory: mrow.name }); onToggled: mrow.change("concordance", checked) }
                        CheckBox { checked: !!mrow.m.update; Accessible.name: Fmt.fill(qsTr("Store confirmed translations in {memory}"), { memory: mrow.name }); onToggled: mrow.change("update", checked) }
                        NumberField {
                            implicitWidth: 80
                            from: 0; to: 100
                            value: mrow.m.penalty || 0
                            Accessible.name: Fmt.fill(qsTr("Penalty for {memory}, in points"), { memory: mrow.name })
                            onEdited: v => mrow.change("penalty", v)
                        }
                        Row {
                            spacing: 2
                            IconButton { iconName: "arrow-up"; enabled: mrow.index > 0; tip: Fmt.fill(qsTr("Move {memory} up"), { memory: mrow.name }); onClicked: mrow.move(mrow.index - 1) }
                            IconButton { iconName: "arrow-down"; enabled: mrow.index < set.list.length - 1; tip: Fmt.fill(qsTr("Move {memory} down"), { memory: mrow.name }); onClicked: mrow.move(mrow.index + 1) }
                            IconButton { iconName: "x"; tip: Fmt.fill(qsTr("Remove {memory}"), { memory: mrow.name }); onClicked: memories.form.removeMemory(set.lang, mrow.index) }
                        }
                    }
                }
            }

            // Adding one: a memory of the language pair, or a new one.
            RowLayout {
                spacing: Theme.s2
                ComboBox {
                    id: pick
                    implicitWidth: 300
                    model: [set.fitting.length ? qsTr("Add a memory…") : qsTr("No other memory fits")].concat(set.fitting)
                    Accessible.name: Fmt.fill(qsTr("Add a memory for {language}"), { language: Backend.languageName(set.lang) })
                }
                Button {
                    small: true
                    iconName: "plus"
                    text: qsTr("Add")
                    Accessible.name: Fmt.fill(qsTr("Add the memory for {language}"), { language: Backend.languageName(set.lang) })
                    enabled: pick.currentIndex > 0
                    onClicked: { memories.form.addMemory(set.lang, set.fitting[pick.currentIndex - 1]); pick.currentIndex = 0 }
                }
                Button {
                    visible: memories.form.creatingFor !== set.lang
                    kind: "quiet"
                    small: true
                    text: qsTr("New memory")
                    Accessible.name: Fmt.fill(qsTr("New memory for {language}"), { language: Backend.languageName(set.lang) })
                    onClicked: memories.form.creatingFor = set.lang
                }
            }

            // A new memory, made with the project.
            Rectangle {
                visible: memories.form.creatingFor === set.lang
                Layout.fillWidth: true
                implicitHeight: fresh.implicitHeight + 2 * Theme.s3
                radius: Theme.rControl
                color: Theme.surface
                border.color: Theme.border
                Accessible.name: Fmt.fill(qsTr("New memory for {language}"), { language: Backend.languageName(set.lang) })
                ColumnLayout {
                    id: fresh
                    x: Theme.s3; y: Theme.s3
                    width: parent.width - 2 * Theme.s3
                    spacing: Theme.s3
                    property string tmx: ""
                    readonly property string problem: !newName.text.trim() ? qsTr("Give the memory a name.") : memories.form.nameTaken(newName.text) ? qsTr("A memory with this name exists already.") : ""
                    function suggestName() {
                        const s = memories.form.s
                        const base = (s.client || "").trim() || (s.name || "").trim() || qsTr("Memory")
                        return base + " (" + memories.form.source.toUpperCase() + "-" + set.lang.toUpperCase() + ")"
                    }
                    onVisibleChanged: if (visible) { newName.text = suggestName(); tmx = ""; newName.forceActiveFocus() }
                    Field {
                        label: qsTr("Name of the new memory")
                        error: fresh.problem
                        TextField { id: newName; invalid: fresh.problem !== "" }
                    }
                    Field {
                        label: qsTr("Fill it from a TMX file")
                        optional: true
                        RowLayout {
                            spacing: Theme.s2
                            Button { small: true; text: qsTr("Choose a file…"); Accessible.name: qsTr("Fill it from a TMX file"); onClicked: tmxDialog.open() }
                            Txt { role: "muted"; text: fresh.tmx ? Backend.fileName(fresh.tmx) : qsTr("No file chosen") }
                        }
                    }
                    RowLayout {
                        spacing: Theme.s2
                        Button {
                            kind: "primary"
                            small: true
                            text: qsTr("Create memory")
                            enabled: fresh.problem === ""
                            onClicked: memories.form.makeMemory(newName.text.trim(), set.lang, fresh.tmx)
                        }
                        Button { kind: "quiet"; small: true; text: qsTr("Cancel"); Accessible.name: qsTr("Cancel the new memory"); onClicked: memories.form.creatingFor = "" }
                    }
                    FileDialog {
                        id: tmxDialog
                        title: qsTr("A TMX file")
                        nameFilters: [qsTr("TMX files (*.tmx)"), qsTr("All files (*)")]
                        onAccepted: fresh.tmx = Backend.localPath(selectedFile)
                    }
                }
            }
            Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: memories.form.problemsAt("memories." + set.lang) }
        }
    }
    Txt {
        Layout.fillWidth: true
        role: "help"
        text: qsTr("A penalty takes points off a memory's matches, for memories you trust less: with 2 points, an exact match shows as 98% and is not inserted as an exact match.")
    }
}
