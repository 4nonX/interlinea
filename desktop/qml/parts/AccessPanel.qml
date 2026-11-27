import QtQuick
import QtQuick.Layouts
import Interlinea

// Who may use a project, memory or termbase on a server, and with which
// role: the roles given, and a row to give one.
Panel {
    id: access
    property string cid
    property string kind
    property string resource
    property var roles: []
    property var people: []
    property var grants: null
    property string problem: ""

    function reload() {
        Api.get(cid, "/api/roles", r => { if (r.ok) roles = r.data })
        Api.get(cid, "/api/people", p => {
            if (p.ok) people = p.data
            Api.call(cid, "GET", "/api/grants", undefined, r => {
                if (!r.ok) { problem = r.error.message; return }
                grants = r.data
            }, { kind: kind, resource: resource })
        })
    }
    function attempt(method, body, query) {
        problem = ""
        Api.call(cid, method, "/api/grants", body, r => {
            if (!r.ok) { problem = r.error.message; return }
            reload()
        }, query)
    }
    Component.onCompleted: reload()
    onResourceChanged: reload()

    Txt { Layout.fillWidth: true; visible: access.problem !== ""; role: "error"; text: access.problem }
    Txt {
        Layout.fillWidth: true
        visible: access.grants !== null && access.grants.length === 0
        role: "muted"
        text: access.kind === "project" ? qsTr("Nobody has a role on it yet; people with work given to them see it all the same.") : qsTr("Nobody has a role on it yet.")
    }
    Table {
        visible: access.grants !== null && access.grants.length > 0
        name: qsTr("Roles")
        columns: [{ title: qsTr("Who"), fill: 1 }, { title: qsTr("Role"), width: 220 }, { title: "", width: 120 }]
        Repeater {
            model: access.grants || []
            TableRow {
                id: grantRow
                required property var modelData
                required property int index
                readonly property string who: modelData.name || modelData.username
                last: index === access.grants.length - 1
                CellTitle { title: grantRow.who }
                ComboBox {
                    readonly property var choices: Roles.choices(access.roles, grantRow.modelData.role)
                    model: choices.map(c => c.label)
                    currentIndex: Math.max(0, choices.findIndex(c => c.id === grantRow.modelData.role))
                    Accessible.name: Fmt.fill(qsTr("Role of {who}"), { who: grantRow.who })
                    onActivated: i => access.attempt("PUT", { userId: grantRow.modelData.userId, kind: access.kind, resource: access.resource, role: choices[i].id })
                }
                Button {
                    small: true
                    kind: "quiet"
                    text: qsTr("Remove")
                    Accessible.name: Fmt.fill(qsTr("Remove the role of {who}"), { who: grantRow.who })
                    onClicked: access.attempt("DELETE", undefined, { userId: grantRow.modelData.userId, kind: access.kind, resource: access.resource })
                }
            }
        }
    }
    RowLayout {
        id: addRow
        spacing: Theme.s3
        readonly property var candidates: access.people.filter(p => !(access.grants || []).some(g => g.userId === p.id))
        Field {
            Layout.fillWidth: false
            Layout.preferredWidth: 260
            label: qsTr("Who")
            ComboBox {
                id: who
                model: [qsTr("Choose someone")].concat(addRow.candidates.map(p => p.name + " (" + p.username + ")"))
            }
        }
        Field {
            Layout.fillWidth: false
            Layout.preferredWidth: 200
            label: qsTr("Role")
            ComboBox {
                id: role
                readonly property var choices: Roles.choices(access.roles, "")
                model: choices.map(c => c.label)
                currentIndex: Math.max(0, choices.findIndex(c => c.id === (access.kind === "project" ? "edit" : "read")))
            }
        }
        Button {
            id: give
            Layout.alignment: Qt.AlignBottom
            text: qsTr("Give the role")
            enabled: who.currentIndex > 0
            onClicked: access.attempt("PUT", { userId: addRow.candidates[who.currentIndex - 1].id, kind: access.kind, resource: access.resource,
                                               role: role.choices[role.currentIndex].id })
        }
    }
}
