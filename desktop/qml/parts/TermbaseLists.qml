import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// The termbases of each place, the opened one marked; under them, making a
// master or a project termbase and importing one (the web's .tb-list).
Rectangle {
    id: lists
    property var page
    radius: Theme.rPanel
    color: Theme.surface
    border.color: Theme.border
    clip: true

    readonly property var groups: {
        const out = []
        const local = Backend.local
        const of = (c, kind) => (page.lists[c.id] || []).filter(t => !kind || t.kind === kind)
        if (local.ready) {
            out.push({ title: qsTr("Masters"), c: local, list: of(local, "master"), empty: "" })
            out.push({ title: Words.get("projectTermbases"), c: local, list: of(local, "project"), empty: "" })
        }
        const teams = Backend.connections.filter(c => !c.local && c.ready)
        for (const c of teams)
            out.push({ title: teams.length > 1 ? Fmt.fill(qsTr("On {host}"), { host: c.displayName }) : qsTr("On the team server"), c: c, list: of(c, ""),
                       empty: Fmt.fill(qsTr("No termbases on {host} for you yet."), { host: c.displayName }) })
        return out
    }
    readonly property bool anyMaster: Object.keys(page.lists).some(k => (page.lists[k] || []).some(t => t.kind === "master"))

    B.ScrollView {
        id: scroll
        anchors.fill: parent
        anchors.margins: 1
        contentWidth: availableWidth
        B.ScrollBar.vertical: ScrollBar { parent: scroll; x: scroll.width - width; height: scroll.height }
        ColumnLayout {
            width: scroll.availableWidth - 2 * Theme.s2
            x: Theme.s2
            spacing: Theme.s3
            Item { implicitHeight: Theme.s2 }
            Repeater {
                model: lists.groups
                ColumnLayout {
                    id: group
                    required property var modelData
                    Layout.fillWidth: true
                    spacing: 2
                    Accessible.name: modelData.title
                    Txt { Layout.leftMargin: Theme.s2; text: group.modelData.title; color: Theme.text2; font.pixelSize: Theme.small; font.weight: 600 }
                    Txt { Layout.fillWidth: true; Layout.leftMargin: Theme.s3; visible: group.modelData.list.length === 0 && group.modelData.empty !== ""; role: "sub"; text: group.modelData.empty }
                    Repeater {
                        model: group.modelData.list
                        TbLink {
                            required property var modelData
                            Layout.fillWidth: true
                            connection: group.modelData.c
                            termbase: modelData
                            current: page.cid === group.modelData.c.id && page.name === modelData.name
                            count: current && page.opened ? page.entries.length : (modelData.entries || 0)
                            onClicked: page.open(group.modelData.c.id, modelData.name)
                        }
                    }
                }
            }
            ColumnLayout {
                spacing: 0
                Button { kind: "quiet"; small: true; iconName: "plus"; text: qsTr("New master"); onClicked: newTermbase.start(false) }
                Button { visible: lists.anyMaster; kind: "quiet"; small: true; iconName: "git-branch"; text: qsTr("New project termbase"); onClicked: newTermbase.start(true) }
                Button { kind: "quiet"; small: true; iconName: "upload"; text: qsTr("Import termbase"); onClicked: importDialog.start("", "") }
            }
            Item { implicitHeight: Theme.s2 }
        }
    }
    NewTermbaseDialog {
        id: newTermbase
        objectName: "newTermbase"
        lists: page.lists
        onCreated: (cid, name, message) => { page.notice(message); page.open(cid, name); page.reload() }
    }
    ImportTermbaseDialog {
        id: importDialog
        objectName: "importTermbase"
        lists: page.lists
        onImported: (cid, name, message) => { page.notice(message); page.open(cid, name); page.reload() }
    }

    component TbLink: B.AbstractButton {
        id: link
        property var connection
        property var termbase
        property bool current: false
        property int count: 0
        implicitHeight: col.implicitHeight + 14
        hoverEnabled: true
        focusPolicy: Qt.StrongFocus
        Accessible.name: Fmt.fill(qsTr("{name}, {place}, {entries}"), { name: termbase.name, place: connection.local ? qsTr("on this computer") : Fmt.fill(qsTr("on {host}"), { host: connection.displayName }), entries: TermData.entries(count) })
        background: Rectangle {
            radius: Theme.rControl
            color: link.current ? Theme.accentWeak : link.hovered ? Theme.surface2 : "transparent"
            FocusRing { target: link }
        }
        contentItem: ColumnLayout {
            id: col
            spacing: 3
            // A project's termbase is named after it; the list says which
            // it is by its group.
            Txt { Layout.fillWidth: true; Layout.leftMargin: Theme.s2; Layout.topMargin: 7; text: link.termbase.name.replace(/ \(project\)$/, ""); font.weight: 600 }
            RowLayout {
                Layout.leftMargin: Theme.s2
                Layout.rightMargin: Theme.s2
                Layout.bottomMargin: 7
                Badge { text: link.connection.local ? qsTr("Local") : qsTr("Server"); kind: link.connection.local ? "" : "server"; iconName: link.connection.local ? "hard-drive" : "cloud" }
                Item { Layout.fillWidth: true }
                Txt { text: Fmt.num(link.count); role: "sub"; wrapMode: Text.NoWrap }
            }
        }
    }
}
