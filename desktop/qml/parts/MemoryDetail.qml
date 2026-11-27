import QtQuick
import QtQuick.Layouts
import Interlinea

// An opened memory: its units searched, in the sources or the
// translations, and added, changed and deleted by those who may; on a
// server also its files out and in, and deleting it.
Panel {
    id: detail
    property string cid
    property bool local: true
    property var memory: null
    signal notice(string text)
    signal reloadAll()
    signal closed()
    signal askImport()
    signal askExport(string format)
    signal align()
    readonly property string name: memory ? memory.name : ""
    readonly property bool edit: local || (!!memory && Roles.allows(memory.role, "edit"))
    readonly property bool manage: !local && !!memory && Roles.allows(memory.role, "manage")
    property var found: null
    title: name
    onNameChanged: { found = null; query.text = "" }

    function search() {
        if (!query.text.trim()) return
        Api.post(cid, "/api/memories/" + Api.seg(name) + "/concordance", { query: query.text, inTarget: inTarget.checked }, r => {
            if (!r.ok) { detail.notice(r.error.message); return }
            found = r.data.map(x => x.unit)
        })
    }
    function changed(u) {
        return [u.changedBy || "", u.changedAt ? Fmt.date(u.changedAt) : ""].filter(x => x).join(", ")
    }

    RowLayout {
        Layout.fillWidth: true
        spacing: Theme.s3
        Accessible.name: Fmt.fill(qsTr("Search {name}"), { name: detail.name })
        Field {
            Layout.fillWidth: false
            Layout.preferredWidth: 280
            label: qsTr("Search the units")
            TextField { id: query; onAccepted: detail.search() }
        }
        CheckBox { id: inTarget; Layout.alignment: Qt.AlignBottom; Layout.bottomMargin: 6; text: qsTr("In the translations") }
        Button { Layout.alignment: Qt.AlignBottom; iconName: "search"; text: qsTr("Search"); enabled: query.text.trim() !== ""; onClicked: detail.search() }
        Item { Layout.fillWidth: true }
        Button { visible: detail.edit; Layout.alignment: Qt.AlignBottom; iconName: "plus"; text: qsTr("Add a unit"); onClicked: addUnit.open() }
        Button { visible: detail.edit; Layout.alignment: Qt.AlignBottom; iconName: "columns"; text: qsTr("Align documents"); onClicked: detail.align() }
    }
    Txt { visible: detail.found !== null && detail.found.length === 0; role: "muted"; text: qsTr("Nothing found.") }
    Table {
        visible: detail.found !== null && detail.found.length > 0
        name: qsTr("Units found")
        columns: [{ title: detail.memory ? detail.memory.sourceLang : "", fill: 1 }, { title: detail.memory ? detail.memory.targetLang : "", fill: 1 },
                  { title: qsTr("Changed"), width: 180 }].concat(detail.edit ? [{ title: "", width: 150 }] : [])
        Repeater {
            model: detail.found || []
            TableRow {
                id: urow
                required property var modelData
                required property int index
                readonly property string said: Fmt.plain(modelData.source || [])
                last: index === detail.found.length - 1
                SegmentEdit { framed: false; readOnly: true; content: urow.modelData.source; language: detail.memory.sourceLang; minimumLines: 1 }
                SegmentEdit { framed: false; readOnly: true; content: urow.modelData.target; language: detail.memory.targetLang; minimumLines: 1 }
                Txt { role: "sub"; text: detail.changed(urow.modelData) }
                Row {
                    visible: detail.edit
                    spacing: Theme.s1
                    Button {
                        small: true; kind: "quiet"; iconName: "pencil"
                        text: qsTr("Change")
                        Accessible.name: Fmt.fill(qsTr("Change the translation of “{text}”"), { text: urow.said })
                        onClicked: editUnit.start(urow.modelData)
                    }
                    ConfirmButton {
                        text: ""
                        iconName: "trash"
                        action: qsTr("Delete")
                        accessibleName: Fmt.fill(qsTr("Delete the unit “{text}”"), { text: urow.said })
                        question: qsTr("Delete this unit?")
                        onConfirmed: Api.del(detail.cid, "/api/memories/" + Api.seg(detail.name) + "/units/" + urow.modelData.id, r => {
                            detail.notice(r.ok ? qsTr("Deleted the unit") : r.error.message)
                            detail.search()
                            detail.reloadAll()
                        })
                    }
                }
            }
        }
    }
    Txt {
        visible: detail.found !== null && detail.found.length >= 50
        role: "sub"
        text: qsTr("The first 50 units found. Search for more of the sentence to narrow it down.")
    }
    // Memory files out and in, and deleting, for those who may.
    Row {
        visible: !detail.local
        spacing: Theme.s2
        Button { iconName: "download"; text: qsTr("Export TMX"); onClicked: detail.askExport("tmx") }
        Button {
            iconName: "download"
            text: qsTr("Export .txt")
            Accessible.name: Fmt.fill(qsTr("Export {name} as a .txt memory"), { name: detail.name })
            onClicked: detail.askExport("wordfast")
        }
        Button {
            visible: detail.edit
            iconName: "upload"
            text: qsTr("Import")
            Accessible.name: Fmt.fill(qsTr("Import a memory file into {name}"), { name: detail.name })
            onClicked: detail.askImport()
        }
        ConfirmButton {
            visible: detail.manage
            small: false
            danger: false
            text: qsTr("Delete the memory")
            action: qsTr("Delete")
            question: detail.memory ? Fmt.fill(qsTr("Delete {name} and its {units} on the server for everyone?"),
                                               { name: detail.name, units: Fmt.plural(detail.memory.units || 0, qsTr("{n} unit"), qsTr("{n} units")) }) : ""
            onConfirmed: Api.del(detail.cid, "/api/memories/" + Api.seg(detail.name), r => {
                if (!r.ok) { detail.notice(r.error.message); return }
                detail.notice(Fmt.fill(qsTr("Deleted {name} on the server"), { name: detail.name }))
                detail.closed()
                detail.reloadAll()
            })
        }
    }

    // A new unit: the sentence and its translation, as plain text.
    Dialog {
        id: addUnit
        wide: true
        title: Fmt.fill(qsTr("Add a unit to {name}"), { name: detail.name })
        onOpened: { unitSource.text = ""; unitTarget.text = ""; overwrite.checked = false; error = ""; unitSource.area.forceActiveFocus() }
        Field { label: detail.memory ? Backend.languageName(detail.memory.sourceLang) : ""; TextArea { id: unitSource; minimumLines: 2 } }
        Field { label: detail.memory ? Backend.languageName(detail.memory.targetLang) : ""; TextArea { id: unitTarget; minimumLines: 2 } }
        CheckBox { id: overwrite; text: qsTr("Replace other translations of this sentence") }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Without it, a sentence that is there already keeps its translations and gets this one as well.") }
        buttons: [
            Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: addUnit.close() },
            Button {
                kind: "primary"; small: true
                text: qsTr("Add the unit")
                enabled: unitSource.text.trim() !== "" && unitTarget.text.trim() !== ""
                onClicked: Api.call(detail.cid, "POST", "/api/memories/" + Api.seg(detail.name) + "/units",
                                    { source: [{ text: unitSource.text.trim() }], target: [{ text: unitTarget.text.trim() }] }, r => {
                    if (!r.ok) { addUnit.error = r.error.message; return }
                    detail.notice(Fmt.fill(qsTr("Added the unit to {name}"), { name: detail.name }))
                    addUnit.close()
                    detail.search()
                    detail.reloadAll()
                }, overwrite.checked ? { policy: "overwrite" } : {})
            }
        ]
    }

    // A unit found, to change its translation; the formatting tags stay.
    Dialog {
        id: editUnit
        wide: true
        title: qsTr("Change the unit")
        property var unit: null
        function start(u) { unit = u; error = ""; open(); editTarget.content = u.target; editSource.content = u.source }
        Field { label: detail.memory ? Backend.languageName(detail.memory.sourceLang) : ""; SegmentEdit { id: editSource; readOnly: true; language: detail.memory ? detail.memory.sourceLang : "" } }
        Field { label: detail.memory ? Backend.languageName(detail.memory.targetLang) : ""; SegmentEdit { id: editTarget; language: detail.memory ? detail.memory.targetLang : "" } }
        Txt {
            Layout.fillWidth: true
            visible: !!editUnit.unit && detail.changed(editUnit.unit) !== ""
            role: "help"
            text: !editUnit.unit ? "" : Fmt.fill(qsTr("Last changed by {who}."), { who: detail.changed(editUnit.unit) })
                  + ((editUnit.unit.usageCount || 0) > 0 ? " " + (editUnit.unit.usageCount === 1 ? qsTr("Used once.") : Fmt.fill(qsTr("Used {n} times."), { n: editUnit.unit.usageCount })) : "")
        }
        buttons: [
            Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: editUnit.close() },
            Button {
                kind: "primary"; small: true
                text: qsTr("Save")
                onClicked: {
                    if (!editTarget.plainText.trim()) { editUnit.error = qsTr("A unit needs a translation. Delete the unit instead."); return }
                    const u = editUnit.unit
                    const body = { source: u.source, target: editTarget.content }
                    if (u.attributes) body.attributes = u.attributes
                    if (u.notes) body.notes = u.notes
                    Api.put(detail.cid, "/api/memories/" + Api.seg(detail.name) + "/units/" + u.id, body, r => {
                        if (r.status === 409) { editUnit.error = qsTr("Someone else changed this unit in the meantime. Search again to see it as it is now."); return }
                        if (!r.ok) { editUnit.error = r.error.message; return }
                        detail.notice(qsTr("Saved the unit"))
                        editUnit.close()
                        detail.search()
                    }, u.version)
                }
            }
        ]
    }
}
