import QtQuick
import QtQuick.Layouts
import Interlinea

// The work given out in a project on a team server: who does what in
// which part, done or not, taken back; and giving out more.
ColumnLayout {
    id: work
    property var project            // the ProjectPage
    readonly property var s: project.s
    readonly property bool manage: project.manages
    readonly property var me: project.conn ? (project.conn.user || {}).id : 0
    readonly property var targets: s.targetLangs || []
    readonly property var files: (s.files || []).filter(f => f.role === "translatable").map(f => f.name)
    // A file made for one language is given out in that language.
    function fileLangs(name) {
        const f = (s.files || []).find(x => x.name === name && x.targetLang)
        return f ? [f.targetLang] : targets
    }
    Layout.fillWidth: true
    spacing: Theme.s3

    Txt { visible: work.project.assignments.length === 0; role: "muted"; text: qsTr("No work given out yet.") }
    Table {
        visible: work.project.assignments.length > 0
        name: qsTr("Work given out")
        columns: [{ title: qsTr("Who"), width: 150 }, { title: qsTr("Task"), width: 96 }, { title: qsTr("Part"), fill: 1 }, { title: qsTr("Done"), width: 64 }, { title: "", width: 120 }]
        Repeater {
            model: work.project.assignments
            TableRow {
                id: arow
                required property var modelData
                required property int index
                readonly property var a: modelData
                readonly property string who: a.userName || ""
                last: index === work.project.assignments.length - 1
                CellTitle { title: arow.who }
                Txt { text: arow.a.task === "review" ? qsTr("Review") : qsTr("Translate") }
                Txt {
                    text: {
                        const part = [arow.a.file || qsTr("All files")]
                        if (arow.a.targetLang) part.push(Backend.languageName(arow.a.targetLang))
                        if (arow.a.fromSegment) part.push(Fmt.fill(qsTr("segments {from} to {to}"), { from: arow.a.fromSegment, to: arow.a.toSegment || arow.a.fromSegment }))
                        if (arow.a.due) part.push(Fmt.fill(qsTr("due {date}"), { date: Fmt.time(arow.a.due) }))
                        return part.join(", ")
                    }
                }
                CheckBox {
                    checked: !!arow.a.done
                    enabled: work.manage || arow.a.userId === work.me
                    Accessible.name: Fmt.fill(qsTr("{name}'s work is done"), { name: arow.who })
                    onToggled: {
                        const on = checked
                        Api.put(work.project.cid, work.project.base + "/assignments/" + arow.a.id, Object.assign({}, arow.a, { done: on }), r => {
                            work.project.say(r.ok ? (on ? qsTr("Reported done") : qsTr("Reported not done")) : r.error.message)
                            work.project.reload()
                        }, arow.a.version || 0)
                    }
                }
                Item {
                    implicitHeight: 28
                    ConfirmButton {
                        visible: work.manage
                        text: qsTr("Take back")
                        action: qsTr("Take back")
                        accessibleName: Fmt.fill(qsTr("Take the work back from {name}"), { name: arow.who })
                        question: Fmt.fill(qsTr("Take this work back from {name}?"), { name: arow.who })
                        onConfirmed: Api.del(work.project.cid, work.project.base + "/assignments/" + arow.a.id, r => { if (!r.ok) work.project.say(r.error.message); work.project.reload() })
                    }
                }
            }
        }
    }

    // Giving out work.
    ColumnLayout {
        id: give
        visible: work.manage
        Layout.fillWidth: true
        spacing: Theme.s3
        Accessible.name: qsTr("Give out work")
        property var segments: []
        property string error: ""
        property string due: ""
        readonly property string file: fileBox.currentIndex > 0 ? work.files[fileBox.currentIndex - 1] : ""
        readonly property var langs: work.fileLangs(file)
        readonly property string lang: langs.length > 1 ? (langBox.currentIndex > 0 ? langs[langBox.currentIndex - 1] : "") : ""
        readonly property string segmentLang: lang || (langs.length === 1 ? langs[0] : "")
        function loadSegments() {
            segments = []
            fromBox.currentIndex = 0
            if (!file || !segmentLang) return
            const f = file, l = segmentLang
            Api.get(work.project.cid, work.project.base + "/segments", r => {
                if (f === give.file && l === give.segmentLang) give.segments = r.ok ? r.data.map(x => x.id) : []
            }, { file: f, lang: l })
        }
        onFileChanged: loadSegments()
        onSegmentLangChanged: loadSegments()

        Txt { role: "label"; text: qsTr("Give out work") }
        Field {
            label: qsTr("Who")
            ComboBox {
                id: whoBox
                model: [qsTr("Choose someone")].concat(work.project.people.map(p => p.name + " (" + p.username + ")"))
            }
        }
        Field {
            label: qsTr("Task")
            ComboBox { id: taskBox; model: [qsTr("Translate"), qsTr("Review")] }
        }
        Field {
            label: qsTr("File")
            ComboBox { id: fileBox; model: [qsTr("All files")].concat(work.files) }
        }
        Field {
            label: qsTr("Language")
            ComboBox {
                id: langBox
                model: give.langs.length === 1 ? [Backend.languageName(give.langs[0])] : [qsTr("All languages")].concat(give.langs.map(t => Backend.languageName(t)))
            }
        }
        RowLayout {
            visible: give.segments.length > 0
            spacing: Theme.s4
            Field {
                Layout.fillWidth: false
                Layout.preferredWidth: 180
                label: qsTr("From segment")
                ComboBox { id: fromBox; model: [qsTr("The whole file")].concat(give.segments.map((_, i) => String(i + 1))) }
            }
            Field {
                visible: fromBox.currentIndex > 0
                Layout.fillWidth: false
                Layout.preferredWidth: 180
                label: qsTr("To segment")
                ComboBox {
                    id: toBox
                    readonly property int first: Math.max(0, fromBox.currentIndex - 1)
                    model: give.segments.slice(first).map((_, i) => String(first + i + 1))
                    onModelChanged: currentIndex = Math.max(0, model.length - 1)
                }
            }
        }
        Field {
            label: qsTr("Due")
            optional: true
            DueField { iso: give.due; onEdited: v => give.due = v }
        }
        Txt { visible: give.error !== ""; Layout.fillWidth: true; role: "error"; text: give.error }
        Button {
            kind: "primary"
            iconName: "user-plus"
            text: qsTr("Give out")
            enabled: whoBox.currentIndex > 0
            onClicked: {
                const a = { userId: work.project.people[whoBox.currentIndex - 1].id, task: taskBox.currentIndex === 1 ? "review" : "translate" }
                if (give.file) a.file = give.file
                if (give.lang) a.targetLang = give.lang
                if (fromBox.currentIndex > 0) {
                    a.fromSegment = give.segments[fromBox.currentIndex - 1]
                    a.toSegment = give.segments[toBox.first + toBox.currentIndex] || a.fromSegment
                }
                if (give.due) a.due = give.due
                Api.post(work.project.cid, work.project.base + "/assignments", a, r => {
                    if (!r.ok) { give.error = r.error.message; return }
                    give.error = ""
                    work.project.say(qsTr("Work given out"))
                    work.project.reload()
                })
            }
        }
        Txt {
            Layout.fillWidth: true
            role: "sub"
            text: Fmt.fill(qsTr("{work} in this project. Each segment has one translator and one reviewer."), { work: Fmt.plural(work.project.assignments.length, qsTr("{n} piece of work"), qsTr("{n} pieces of work")) })
        }
    }
}
