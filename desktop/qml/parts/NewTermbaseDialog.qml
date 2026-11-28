import QtQuick
import QtQuick.Layouts
import Interlinea

// A new termbase in steps (web/src/screens/NewTermbase.tsx): its name,
// where it is kept and what kind it is; a master's languages; its fields;
// and what will be made. A project termbase is based on masters and gets
// their languages and fields.
Dialog {
    id: dlg
    wide: true
    title: qsTr("New termbase")
    property var lists: ({})
    signal created(string connectionId, string name, string message)

    property bool project: false
    property int step: 0
    property string place: ""
    property var based: []
    property var langs: []
    property var fieldDrafts: []
    property var inheritedLanguages: []
    property var inheritedFields: []
    property bool busy: false
    readonly property var steps: project ? [qsTr("Start"), qsTr("Fields"), qsTr("Review")] : [qsTr("Start"), qsTr("Languages"), qsTr("Fields"), qsTr("Review")]
    readonly property string stepName: project ? ["start", "fields", "review"][step] : ["start", "languages", "fields", "review"][step]
    readonly property var places: Backend.connections.filter(c => c.ready && (c.local || c.admin || true))
    readonly property var masters: (lists[place] || []).filter(t => t.kind === "master").map(t => t.name)

    function start(asProject) {
        project = !!asProject
        step = 0
        place = Backend.local.id
        based = []
        langs = []
        fieldDrafts = []
        inheritedLanguages = []
        inheritedFields = []
        error = ""
        name.text = ""
        open()
        name.forceActiveFocus()
    }
    function taken(n) { return (lists[place] || []).some(t => t.name.trim().toLowerCase() === n.trim().toLowerCase()) }
    function blocking() {
        if (stepName === "start") {
            if (!name.text.trim()) return qsTr("Give the termbase a name.")
            if (taken(name.text)) return qsTr("A termbase with this name is there already.")
            if (project && !based.length) return qsTr("Choose the masters it is based on.")
            return ""
        }
        if (stepName === "languages") return langs.length ? "" : qsTr("Choose its languages, at least one.")
        if (stepName === "fields") {
            const inherited = project ? inheritedFields.map(f => f.name) : []
            for (let i = 0; i < fieldDrafts.length; i++) {
                const p = Fields.problem(fieldDrafts[i], inherited.concat(fieldDrafts.filter((f, j) => j !== i).map(f => f.name)))
                if (p) return p
            }
        }
        return ""
    }
    // The fields of a termbase there is, added to those chosen.
    function addFieldsOf(tb) {
        error = ""
        Api.get(place, "/api/termbases/" + Api.seg(tb), r => {
            if (!r.ok) { error = r.error.message; return }
            fieldDrafts = Fields.withFields(fieldDrafts, (r.data.definition || {}).fields || [])
        })
    }
    // In the order the masters were chosen: the first comes first.
    function loadInherited() {
        inheritedLanguages = []
        inheritedFields = []
        if (!project || !based.length) return
        const got = {}
        let left = based.length
        busy = true
        for (const m of based) {
            Api.get(place, "/api/termbases/" + Api.seg(m), r => {
                if (!r.ok) { error = r.error.message; busy = false; return }
                got[m] = r.data.definition || {}
                if (--left > 0) return
                const ls = [], fs = []
                for (const n of based) {
                    for (const l of got[n].languages || []) if (ls.indexOf(l) < 0) ls.push(l)
                    for (const f of got[n].fields || []) if (!fs.some(x => x.name.toLowerCase() === f.name.toLowerCase())) fs.push(f)
                }
                inheritedLanguages = ls
                inheritedFields = fs
                busy = false
            })
        }
    }
    function create() {
        const n = name.text.trim()
        const definition = { name: n, fields: fieldDrafts.map(f => Fields.toJson(f)) }
        const body = {}
        if (project) body.upstreams = based
        else definition.languages = langs
        body.definition = definition
        busy = true
        error = ""
        const cid = place
        Api.post(cid, "/api/termbases", body, r => {
            busy = false
            if (!r.ok) { error = r.error.message; return }
            const made = r.data.name || n
            dlg.created(cid, made, project ? Fmt.fill(qsTr("Created project termbase “{name}”"), { name: made }) : Fmt.fill(qsTr("Created master “{name}”"), { name: made }))
            dlg.close()
        })
    }

    Stepper { steps: dlg.steps; current: dlg.step; onStepClicked: i => dlg.step = i }

    // Start.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "start"
        spacing: Theme.s3
        Field {
            label: qsTr("Name")
            TextField { id: name; placeholderText: dlg.project ? qsTr("For example a case or an order") : qsTr("For example a client or a subject") }
        }
        Txt { visible: dlg.places.length > 1; role: "label"; text: qsTr("Where it is kept") }
        Repeater {
            model: dlg.places.length > 1 ? dlg.places : []
            RadioButton {
                required property var modelData
                text: modelData.local ? qsTr("On this computer")
                      : Backend.connections.filter(c => !c.local).length > 1 ? Fmt.fill(qsTr("On {host}, for everyone working with it"), { host: modelData.displayName })
                      : qsTr("On the team server, for everyone working with it")
                checked: dlg.place === modelData.id
                onClicked: { dlg.place = modelData.id; dlg.based = [] }
            }
        }
        Txt { role: "label"; text: qsTr("What kind of termbase") }
        Choice {
            Layout.fillWidth: true
            text: qsTr("A master.")
            description: qsTr("A termbase of its own, kept across projects, for a client, a subject or a company. You choose its languages and fields.")
            checked: !dlg.project
            onClicked: dlg.project = false
        }
        Choice {
            Layout.fillWidth: true
            text: qsTr("A project termbase.")
            description: qsTr("Based on masters: it shows their entries, gets their languages and fields, and sends its new and changed entries to them for review.")
            checked: dlg.project
            enabled: dlg.masters.length > 0
            onClicked: dlg.project = true
        }
        Txt { visible: dlg.project; role: "label"; text: qsTr("Based on") }
        Txt { visible: dlg.project && dlg.masters.length === 0; role: "help"; text: qsTr("There is no master here yet. Make one first.") }
        Repeater {
            model: dlg.project ? dlg.masters : []
            CheckBox {
                required property string modelData
                text: modelData
                checked: dlg.based.indexOf(modelData) >= 0
                onToggled: dlg.based = checked ? dlg.based.concat([modelData]) : dlg.based.filter(m => m !== modelData)
            }
        }
        Txt { visible: dlg.project && dlg.masters.length > 1; Layout.fillWidth: true; role: "help"; text: qsTr("The first master chosen comes first where masters have the same entry.") }
    }

    // Languages.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "languages"
        spacing: Theme.s3
        Txt { role: "label"; text: Fmt.fill(qsTr("The languages of {name}"), { name: name.text.trim() }) }
        LanguageChecklist {
            Layout.fillWidth: true
            label: qsTr("Languages of the termbase")
            chosen: dlg.langs
            onChanged: c => dlg.langs = c
        }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("A termbase has no source and target: it has languages, and a project looks up terms in its own. More languages can be added later.") }
    }

    // Fields.
    ColumnLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "fields"
        spacing: Theme.s3
        Txt { visible: !dlg.project; role: "label"; text: qsTr("What entries hold") }
        Repeater {
            model: dlg.project ? [] : Fields.groups()
            CheckBox {
                required property var modelData
                objectName: "fieldGroup:" + modelData.id
                text: modelData.name
                description: modelData.text
                bold: true
                checked: Fields.hasGroup(dlg.fieldDrafts, modelData)
                // A click sets the box and cuts its binding; the box then
                // follows the list again, so taking out one of the group's
                // fields, or adding the fields of another termbase, shows.
                onClicked: {
                    dlg.fieldDrafts = Fields.withGroup(dlg.fieldDrafts, modelData, checked)
                    checked = Qt.binding(() => Fields.hasGroup(dlg.fieldDrafts, modelData))
                }
            }
        }
        FitRow {
            visible: !dlg.project && (dlg.lists[dlg.place] || []).length > 0
            Txt { Layout.alignment: Qt.AlignVCenter; text: qsTr("Add the fields of"); wrapMode: Text.NoWrap }
            ComboBox {
                id: fromTb
                Layout.alignment: Qt.AlignVCenter
                model: [qsTr("a termbase…")].concat((dlg.lists[dlg.place] || []).map(t => t.name))
                Accessible.name: qsTr("The termbase to take the fields from")
            }
            Button {
                Layout.alignment: Qt.AlignVCenter
                small: true
                text: qsTr("Add")
                enabled: fromTb.currentIndex > 0
                Accessible.name: qsTr("Add the fields of the termbase chosen")
                onClicked: { dlg.addFieldsOf(fromTb.model[fromTb.currentIndex]); fromTb.currentIndex = 0 }
            }
            Item { Layout.fillWidth: true; visible: parent.fits }
        }
        Txt { visible: !dlg.project; Layout.fillWidth: true; role: "help"; text: qsTr("Tick any of them, in any combination: their fields go into the list below, where you can change them, take them out, or add fields of your own. With none, an entry holds its terms. Every term has a status (approved, proposed or forbidden) without a field for it.") }
        Txt { visible: dlg.project; role: "label"; text: Fmt.fill(qsTr("From {masters}"), { masters: Fmt.listOf(dlg.based) }) }
        Txt {
            visible: dlg.project
            Layout.fillWidth: true
            role: "help"
            text: dlg.busy ? qsTr("Loading…")
                  : (dlg.inheritedLanguages.length ? Fmt.fill(qsTr("Languages: {languages}."), { languages: dlg.inheritedLanguages.map(l => Backend.languageName(l)).join(", ") }) + " " : "")
                    + (dlg.inheritedFields.length ? Fmt.fill(qsTr("Fields: {fields}."), { fields: dlg.inheritedFields.map(f => f.name).join("; ") }) : qsTr("The masters have no fields."))
        }
        Txt { visible: dlg.project; role: "label"; text: qsTr("Fields of its own") }
        Txt { visible: dlg.project; Layout.fillWidth: true; role: "help"; text: qsTr("Only if the project needs more than the masters have. A field of its own reaches a master with the first entry that uses it, once a reviewer approves the entry.") }
        FieldTable { Layout.fillWidth: true; fields: dlg.fieldDrafts; onChanged: f => dlg.fieldDrafts = f }
    }

    // Review.
    GridLayout {
        Layout.fillWidth: true
        visible: dlg.stepName === "review"
        columns: 2
        columnSpacing: Theme.s4
        rowSpacing: Theme.s2
        Txt { role: "muted"; text: qsTr("Name") }
        Txt { Layout.fillWidth: true; text: name.text.trim() }
        Txt { role: "muted"; text: qsTr("Kept"); visible: dlg.places.length > 1 }
        Txt {
            Layout.fillWidth: true
            visible: dlg.places.length > 1
            text: { const c = Backend.connection(dlg.place); return !c || c.local ? qsTr("On this computer") : Fmt.fill(qsTr("On {host}"), { host: c.displayName }) }
        }
        Txt { role: "muted"; text: qsTr("Kind") }
        Txt { Layout.fillWidth: true; text: dlg.project ? Fmt.fill(qsTr("A project termbase based on {masters}"), { masters: Fmt.listOf(dlg.based) }) : qsTr("A master") }
        Txt { role: "muted"; text: qsTr("Languages") }
        Txt {
            Layout.fillWidth: true
            readonly property var all: dlg.project ? dlg.inheritedLanguages : dlg.langs
            text: all.length ? all.map(l => Backend.languageName(l)).join(", ") : qsTr("None yet")
        }
        Txt { role: "muted"; text: qsTr("Fields"); Layout.alignment: Qt.AlignTop }
        Txt {
            Layout.fillWidth: true
            text: {
                const own = dlg.fieldDrafts.map(f => f.name + " (" + Fields.levelName(f.level) + ", " + Fields.kindName(f.kind) + (f.kind === "picklist" ? ": " + (f.values || []).join(", ") : "") + ")")
                const from = dlg.project ? dlg.inheritedFields.map(f => Fmt.fill(qsTr("{field} from the masters"), { field: f.name })) : []
                const all = from.concat(own)
                return all.length ? all.join("\n") : qsTr("None: terms and their status")
            }
        }
    }

    buttons: [
        Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button { visible: dlg.step > 0; small: true; text: qsTr("Back"); onClicked: dlg.step-- },
        Button {
            kind: "primary"; small: true
            enabled: !dlg.busy && dlg.blocking() === ""
            text: dlg.step < dlg.steps.length - 1 ? qsTr("Continue") : dlg.project ? qsTr("Create project termbase") : qsTr("Create master")
            onClicked: {
                const why = dlg.blocking()
                if (why) { dlg.error = why; return }
                dlg.error = ""
                if (dlg.step < dlg.steps.length - 1) {
                    dlg.step++
                    if (dlg.stepName === "fields" && dlg.project) dlg.loadInherited()
                } else {
                    dlg.create()
                }
            }
        }
    ]
}
