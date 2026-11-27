import QtQuick
import QtQuick.Layouts
import Interlinea

// The opened termbase's head (.pane-head): its name and tools, where it is,
// its count and languages; under a project termbase what it is based on,
// its merge requests and submitting it for review.
ColumnLayout {
    id: head
    property var page
    spacing: Theme.s2
    readonly property var conn: Backend.connection(page.cid)

    ColumnLayout {
        Layout.fillWidth: true
        Layout.margins: Theme.s4
        Layout.bottomMargin: Theme.s3
        spacing: Theme.s2
        Txt {
            Layout.fillWidth: true
            visible: !page.opened
            role: page.problem ? "error" : "help"
            text: page.problem ? page.problem
                 : !Backend.connections.some(c => c.ready) ? qsTr("Starting…")
                 : !page.name ? qsTr("No termbase yet. Make a master on the left, or import a termbase file.")
                 : Fmt.fill(qsTr("Opening {name}…"), { name: page.name })
        }
        // The name sits above its tools, and the tools wrap within the pane.
        ColumnLayout {
            Layout.fillWidth: true
            visible: page.opened
            spacing: Theme.s1
            Txt { Layout.fillWidth: true; text: page.name; role: "h2"; wrapMode: Text.NoWrap; elide: Text.ElideRight }
            Flow {
                Layout.fillWidth: true
                spacing: 2
                Accessible.name: Fmt.fill(qsTr("{name} tools"), { name: page.name })
                Button { visible: page.may("edit"); kind: "quiet"; small: true; iconName: "upload"; text: qsTr("Import"); onClicked: importInto.start(page.cid, page.name) }
                Button { kind: "quiet"; small: true; iconName: "download"; text: qsTr("Export"); enabled: page.entries.length > 0; onClicked: exportDialog.open() }
                Button {
                    visible: page.isTeam
                    kind: "quiet"; small: true; iconName: "refresh"
                    text: qsTr("Reload")
                    onClicked: { page.reload(); page.notice(Fmt.fill(qsTr("Loaded {name} again from the server"), { name: page.name })) }
                }
                Button { visible: page.may("manage"); kind: "quiet"; small: true; iconName: "sliders"; text: qsTr("Settings"); onClicked: settingsDialog.start() }
            }
        }
        Flow {
            Layout.fillWidth: true
            visible: page.opened
            spacing: Theme.s2
            Badge { text: page.isTeam ? qsTr("Server") : qsTr("Local"); kind: page.isTeam ? "server" : ""; iconName: page.isTeam ? "cloud" : "hard-drive" }
            Txt { text: TermData.entries(page.entries.length); role: "sub"; wrapMode: Text.NoWrap }
            Repeater { model: page.languages; LangChip { required property string modelData; code: modelData } }
            Badge {
                visible: page.isTeam && !page.may("manage")
                text: Roles.text(page.detail.role || "")
                ToolTip { visible: parent.visible && hover.hovered; text: qsTr("What you may do in this termbase") }
                HoverHandler { id: hover }
            }
        }
        // What a project termbase is based on.
        RowLayout {
            Layout.fillWidth: true
            visible: page.opened && !page.isTeam && page.isProject && page.upstreams.length > 0
            spacing: 6
            Icon { Layout.alignment: Qt.AlignTop; name: "git-branch"; color: Theme.text2; size: 14 }
            Txt {
                Layout.fillWidth: true
                role: "muted"
                font.pixelSize: Theme.small
                text: Fmt.fill(qsTr("{basedOn} {masters}. Shows only this project's new and changed entries."), { basedOn: Words.get("basedOn"), masters: page.upstreams.join(", ") })
            }
        }
        // The merge requests.
        Repeater {
            model: page.opened ? page.requests : []
            ColumnLayout {
                required property var modelData
                spacing: 0
                Button {
                    kind: "quiet"; small: true; leftPadding: 0
                    iconName: "git-pull-request"
                    text: modelData.title
                    onClicked: page.openReview(page.cid, page.name, modelData.id)
                }
                Txt {
                    Layout.fillWidth: true
                    role: "sub"
                    text: Fmt.fill(qsTr("{project} into {target}, {state}"), { project: modelData.project, target: modelData.target,
                          state: modelData.state === "open" ? Fmt.fill(qsTr("open since {date}"), { date: Fmt.date(modelData.createdAt) })
                                                           : Fmt.fill(qsTr("closed {date}"), { date: Fmt.date(modelData.closedAt || modelData.createdAt) }) })
                }
            }
        }
        // Submitting the project termbase for review.
        ColumnLayout {
            id: submit
            Layout.fillWidth: true
            visible: page.opened && page.isProject && page.upstreams.length > 0
            spacing: Theme.s2
            Accessible.name: qsTr("Submit for review")
            readonly property string master: page.upstreams.length > 1 ? (page.upstreams[into.currentIndex] || "") : (page.upstreams[0] || "")
            function send() {
                const t = reviewTitle.text.trim() || Fmt.fill(qsTr("{project} into {master}"), { project: page.name, master: master })
                Api.post(page.cid, page.path("/merge-requests"), { master: master, title: t }, r => {
                    if (!r.ok) { page.notice(r.error.message); return }
                    page.notice(Fmt.fill(qsTr("Submitted {name} to {target} for review"), { name: page.name, target: r.data.target }))
                    reviewTitle.text = ""
                    page.reload()
                    page.openReview(page.cid, page.name, r.data.id)
                })
            }
            Field { visible: page.upstreams.length > 1; label: qsTr("Into"); ComboBox { id: into; model: page.upstreams } }
            Field {
                label: qsTr("Title of the request")
                TextField { id: reviewTitle; placeholderText: Fmt.fill(qsTr("{project} into {master}"), { project: page.name, master: submit.master }); onAccepted: submit.send() }
            }
            Button {
                small: true
                iconName: "git-pull-request"
                text: !page.isTeam ? Words.get("reviewMerge") : page.upstreams.length > 1 ? qsTr("Submit to the master for review")
                      : Fmt.fill(qsTr("Submit to {master} for review"), { master: page.upstreams[0] || "" })
                onClicked: submit.send()
            }
        }
    }
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }

    ImportTermbaseDialog {
        id: importInto
        lists: page.lists
        onImported: (cid, name, message) => { page.notice(message); page.reload() }
    }
    ExportTermbaseDialog { id: exportDialog; objectName: "exportTermbase"; page: head.page }
    TermbaseSettingsDialog { id: settingsDialog; objectName: "termbaseSettings"; page: head.page }
}
