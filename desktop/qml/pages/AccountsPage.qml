import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// Accounts (web/src/screens/Accounts.tsx): the people on a team server
// for its administrators: making accounts, their details, password,
// sign-ins, links to the identity provider and roles, and the roles the
// server defines.
Page {
    id: page
    objectName: "accountsPage"
    title: qsTr("Accounts")

    signal notice(string text)
    signal openSettings()

    property string cid: ""
    property var users: []
    property bool read: false
    property string listProblem: ""
    property var catalog: ({})
    property var projects: []
    property var roles: []
    property int selected: 0
    property var grants: []
    property var signIns: []
    property var identities: []
    property string problem: ""
    property string roleProblem: ""
    property var newProblems: ({})
    property int reading: 0
    property int detailReading: 0

    readonly property var servers: Backend.connections.filter(c => !c.local && c.ready)
    readonly property var conn: { const c = Backend.connection(cid); return c && !c.local && c.ready ? c : null }
    readonly property var user: users.find(u => u.id === selected) || null

    function day(iso) {
        // The zero time the server sends for no date is "never".
        return iso && !iso.startsWith("0001-") ? Backend.date(iso) : qsTr("never")
    }
    function kindText(kind) {
        return kind === "project" ? qsTr("Project") : kind === "memory" ? qsTr("Memory") : kind === "termbase" ? qsTr("Termbase") : qsTr("Everything")
    }
    function resourceName(g) {
        if (g.kind === "all") return qsTr("Everything")
        if (g.resource === "*") return g.kind === "memory" ? qsTr("All memories") : g.kind === "project" ? qsTr("All projects") : qsTr("All termbases")
        if (g.kind === "project") {
            const p = projects.find(p => String(p.id) === g.resource)
            return p ? p.settings.name : Fmt.fill(qsTr("Project {id}"), { id: g.resource })
        }
        return g.resource
    }
    function problemsOf(r) { return (r.error.problems || []).map(p => p.message).join(" ") }

    function setConnection(id) {
        if (cid === id) return
        cid = id
        selected = 0
        users = []
        read = false
        newProblems = ({})
        reload()
    }
    function reload() {
        // The server shown is one signed in to; the first, unless one is chosen.
        if (!conn) cid = servers.length ? servers[0].id : ""
        if (!conn || !conn.admin) { users = []; read = false; selected = 0; return }
        const r = ++reading
        Api.get(cid, "/api/users", res => {
            if (r !== reading) return
            listProblem = res.ok ? "" : res.error.message
            if (res.ok) { users = res.data; read = true }
            if (!users.some(u => u.id === selected)) selected = 0
        })
        Api.get(cid, "/api/catalog", res => { if (r === reading && res.ok) catalog = res.data })
        Api.get(cid, "/api/projects", res => { if (r === reading && res.ok) projects = res.data })
        Api.get(cid, "/api/roles", res => { if (r === reading && res.ok) roles = res.data })
        if (selected) loadDetail()
    }
    function select(id) {
        id = Number(id)
        selected = id
        grants = []; signIns = []; identities = []
        problem = ""
        loadDetail()
    }
    function loadDetail() {
        if (!conn || !selected) return
        const id = selected, r = ++detailReading, base = "/api/users/" + id
        // Answers of an earlier reading are passed over: they may come late.
        const part = key => res => {
            if (id !== selected || r !== detailReading) return
            if (res.ok) page[key] = res.data
            else problem = res.error.message
        }
        Api.get(cid, base + "/tokens", part("signIns"))
        Api.get(cid, base + "/identities", part("identities"))
        Api.get(cid, base + "/grants", part("grants"))
    }
    // A change to an account: what went wrong is said in the account.
    function attempt(method, path, body, done, query, version) {
        problem = ""
        roleProblem = ""
        Api.call(cid, method, path, body, res => {
            if (!res.ok) {
                problem = res.error.code === "lastAdmin" ? qsTr("The server needs at least one active administrator: make someone else one first.")
                        : problemsOf(res) || res.error.message
                return
            }
            notice(done)
            reload()
        }, query, version)
    }
    function attemptRole(method, path, body, done) {
        roleProblem = ""
        Api.call(cid, method, path, body, res => {
            if (!res.ok) { roleProblem = problemsOf(res) || res.error.message; return }
            notice(done)
            reload()
        })
    }

    Timer { id: soon; interval: 200; onTriggered: page.reload() }
    Connections {
        target: Backend
        function onEvent(c, type) { if (c === page.cid && ["account", "project", "memory", "termbase"].indexOf(type) >= 0) soon.restart() }
        function onReset(c) { soon.restart() }
        function onConnectionsChanged() { soon.restart() }
    }
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() { soon.restart() }
        }
    }
    Component.onCompleted: reload()

    // Not signed in to a team server.
    ColumnLayout {
        visible: !page.conn
        Layout.fillWidth: true
        Layout.maximumWidth: 760
        spacing: Theme.s3
        Txt { Layout.fillWidth: true; text: qsTr("Accounts are kept on a team server. Sign in to one in Settings.") }
        Button { small: true; text: qsTr("Open Settings"); onClicked: page.openSettings() }
    }
    // With several servers, the one shown is chosen first.
    Segmented {
        visible: !!page.conn && page.servers.length > 1
        name: qsTr("Team server")
        model: page.servers.map(s => s.displayName)
        current: Math.max(0, page.servers.findIndex(s => s.id === page.cid))
        onChanged: i => page.setConnection(page.servers[i].id)
    }
    Txt {
        visible: !!page.conn && !page.conn.admin
        Layout.fillWidth: true
        Layout.maximumWidth: 760
        text: qsTr("Only the server's administrators manage accounts. Managers decide who may use their projects on each project's page.")
    }

    ColumnLayout {
        visible: !!page.conn && page.conn.admin
        Layout.fillWidth: true
        spacing: Theme.s4
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: Fmt.fill(qsTr("The people on {server}. A role is a set of permissions: five come with the server, each including the ones before it, and you define others below."),
                           { server: page.conn ? page.conn.displayName : "" })
        }
        Txt { visible: page.listProblem !== ""; Layout.fillWidth: true; role: "error"; text: page.listProblem }

        // .accounts: the list in 16 to 22 rem, the account beside it.
        RowLayout {
            Layout.fillWidth: true
            spacing: Theme.s4
            Panel {
                Layout.alignment: Qt.AlignTop
                Layout.preferredWidth: 320
                Layout.minimumWidth: 256
                Layout.maximumWidth: 352
                title: page.read ? Fmt.plural(page.users.length, qsTr("{n} account"), qsTr("{n} accounts")) : qsTr("Accounts")
                spacing: Theme.s3
                Txt { visible: !page.read; role: "muted"; text: qsTr("Loading…") }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    Repeater {
                        model: page.users
                        B.AbstractButton {
                            id: urow
                            required property var modelData
                            readonly property string heading: modelData.name || modelData.username
                            readonly property string sub: modelData.username + (modelData.admin ? qsTr(", administrator") : "")
                                                          + (modelData.disabled ? qsTr(", disabled") : "") + (!modelData.hasPassword ? qsTr(", provider sign-in only") : "")
                            Layout.fillWidth: true
                            implicitHeight: ucol.implicitHeight + 2 * Theme.s2
                            hoverEnabled: true
                            focusPolicy: Qt.StrongFocus
                            Accessible.name: heading + ", " + sub
                            onClicked: page.select(modelData.id)
                            background: Rectangle {
                                radius: Theme.rControl
                                color: urow.modelData.id === page.selected ? Theme.accentWeak : urow.hovered ? Theme.surface2 : "transparent"
                                FocusRing { target: urow }
                            }
                            contentItem: ColumnLayout {
                                id: ucol
                                spacing: 2
                                Txt { Layout.fillWidth: true; Layout.leftMargin: Theme.s3; Layout.rightMargin: Theme.s3; Layout.topMargin: Theme.s2; role: "strong"; text: urow.heading }
                                Txt { Layout.fillWidth: true; Layout.leftMargin: Theme.s3; Layout.rightMargin: Theme.s3; Layout.bottomMargin: Theme.s2; role: "sub"; text: urow.sub }
                            }
                        }
                    }
                }
                Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }

                // .account-new
                ColumnLayout {
                    id: newAccount
                    Layout.fillWidth: true
                    spacing: Theme.s3
                    Accessible.name: qsTr("New account")
                    function submit() {
                        if (!page.conn || !newUsername.text.trim()) return
                        page.newProblems = ({})
                        const body = { username: newUsername.text, name: newName.text, email: newEmail.text, password: newPassword.text, admin: newAdmin.checked }
                        Api.post(page.cid, "/api/users", body, res => {
                            if (!res.ok) {
                                const p = {}
                                if ((res.error.problems || []).length) for (const x of res.error.problems) p[x.field] = x.message
                                else if (res.error.code === "exists") p.username = res.error.message
                                else p.form = res.error.message
                                page.newProblems = p
                                return
                            }
                            page.notice(Fmt.fill(qsTr("Made the account {name}"), { name: res.data.username }))
                            newUsername.text = ""; newName.text = ""; newEmail.text = ""; newPassword.text = ""; newAdmin.checked = false
                            page.select(res.data.id)
                            page.reload()
                        })
                    }
                    Txt { role: "label"; text: qsTr("New account") }
                    Field { label: qsTr("Username"); error: page.newProblems.username || ""; TextField { id: newUsername; invalid: !!page.newProblems.username; onAccepted: newAccount.submit() } }
                    Field { label: qsTr("Name"); TextField { id: newName; onAccepted: newAccount.submit() } }
                    Field { label: qsTr("Email"); error: page.newProblems.email || ""; TextField { id: newEmail; invalid: !!page.newProblems.email; onAccepted: newAccount.submit() } }
                    Field {
                        label: qsTr("Password")
                        help: qsTr("12 characters or more. Leave it empty for someone who signs in only through the identity provider.")
                        error: page.newProblems.password || ""
                        TextField { id: newPassword; echoMode: TextInput.Password; invalid: !!page.newProblems.password; onAccepted: newAccount.submit() }
                    }
                    CheckBox { id: newAdmin; text: qsTr("Administrator") }
                    Txt { visible: !!page.newProblems.form; Layout.fillWidth: true; role: "error"; text: page.newProblems.form || "" }
                    Button { kind: "primary"; iconName: "user-plus"; text: qsTr("Make the account"); enabled: newUsername.text.trim() !== ""; onClicked: newAccount.submit() }
                }
            }

            Panel {
                Layout.alignment: Qt.AlignTop
                Layout.fillWidth: true
                visible: !page.user
                Txt { role: "muted"; text: qsTr("Choose an account to see and change it.") }
            }
            Loader {
                Layout.alignment: Qt.AlignTop
                Layout.fillWidth: true
                active: !!page.user
                visible: active
                sourceComponent: detailPanel
            }
        }

        // The roles the server defines.
        Panel {
            Layout.fillWidth: true
            title: qsTr("Roles")
            spacing: Theme.s3
            Txt {
                Layout.fillWidth: true
                role: "help"
                text: qsTr("What each role allows. The first five come with the server. Define your own for other combinations, such as a reviewer who suggests and approves but does not edit. A change applies at once to everyone who has the role.")
            }
            Txt { visible: page.roleProblem !== ""; Layout.fillWidth: true; role: "error"; text: page.roleProblem }
            Table {
                id: roleTable
                readonly property var perms: Roles.permissions()
                name: qsTr("Roles")
                columns: [{ title: qsTr("Role"), fill: 1 }].concat(perms.map(p => ({ title: Roles.text(p), width: 156 })), [{ title: qsTr("Given"), width: 72, align: "right" }, { title: "", width: 110 }])
                Repeater {
                    model: page.roles
                    TableRow {
                        id: rr
                        required property var modelData
                        required property int index
                        readonly property string roleName: modelData.name
                        readonly property bool builtIn: !!modelData.builtIn
                        readonly property var has: modelData.permissions || []
                        readonly property int given: modelData.grants || 0
                        readonly property string route: "/api/roles/" + Api.seg(roleName)
                        last: index === page.roles.length - 1
                        Item {
                            implicitHeight: rr.builtIn ? builtInCell.implicitHeight : 32
                            CellTitle { id: builtInCell; visible: rr.builtIn; width: parent.width; title: Roles.text(rr.roleName); sub: qsTr("Comes with the server") }
                            TextField {
                                visible: !rr.builtIn
                                width: Math.min(parent.width, 256)
                                text: rr.roleName
                                Accessible.name: Fmt.fill(qsTr("Name of the role {role}"), { role: rr.roleName })
                                onEditingFinished: {
                                    const v = text.trim()
                                    if (!v || v === rr.roleName) { text = rr.roleName; return }
                                    page.attemptRole("PUT", rr.route, { name: v, permissions: rr.has }, Fmt.fill(qsTr("Renamed the role to {role}"), { role: v }))
                                }
                            }
                        }
                        Repeater {
                            model: roleTable.perms
                            Item {
                                required property string modelData
                                required property int index
                                readonly property int cell: index + 1
                                implicitHeight: 20
                                CheckBox {
                                    checked: rr.has.indexOf(modelData) >= 0
                                    enabled: !rr.builtIn && modelData !== "read"
                                    Accessible.name: Roles.text(rr.roleName) + ": " + Roles.text(modelData)
                                    onToggled: {
                                        const p = modelData, on = checked
                                        const now = roleTable.perms.filter(x => x === p ? on : rr.has.indexOf(x) >= 0)
                                        page.attemptRole("PUT", rr.route, { permissions: now },
                                                         Fmt.fill(on ? qsTr("{role} now allows: {permission}") : qsTr("{role} no longer allows: {permission}"),
                                                                  { role: rr.roleName, permission: Roles.text(p).toLowerCase() }))
                                    }
                                }
                            }
                        }
                        Txt { readonly property int cell: roleTable.perms.length + 1; horizontalAlignment: Text.AlignRight; text: Fmt.num(rr.given) }
                        Item {
                            readonly property int cell: roleTable.perms.length + 2
                            implicitHeight: 28
                            // A role people have stays until they have another.
                            ConfirmButton {
                                visible: !rr.builtIn
                                available: rr.given === 0
                                text: qsTr("Delete")
                                accessibleName: Fmt.fill(qsTr("Delete the role {role}"), { role: rr.roleName })
                                question: Fmt.fill(qsTr("Delete the role {role}?"), { role: rr.roleName })
                                onConfirmed: page.attemptRole("DELETE", rr.route, undefined, Fmt.fill(qsTr("Deleted the role {role}"), { role: rr.roleName }))
                            }
                        }
                    }
                }
            }
            // .role-add: a new role.
            RowLayout {
                id: newRole
                spacing: Theme.s4
                Accessible.name: qsTr("New role")
                property var allows: ["read"]
                function submit() {
                    const n = roleName.text.trim()
                    if (!n) return
                    page.roleProblem = ""
                    Api.put(page.cid, "/api/roles/" + Api.seg(n), { permissions: allows }, res => {
                        if (!res.ok) { page.roleProblem = page.problemsOf(res) || res.error.message; return }
                        page.notice(Fmt.fill(qsTr("Made the role {role}"), { role: n }))
                        roleName.text = ""
                        newRole.allows = ["read"]
                        page.reload()
                    })
                }
                Field {
                    Layout.fillWidth: false
                    Layout.preferredWidth: 240
                    label: qsTr("New role")
                    TextField {
                        id: roleName
                        placeholderText: qsTr("For example Reviewer")
                        // What was wrong with the last try is over once it is changed.
                        onTextEdited: page.roleProblem = ""
                        onAccepted: newRole.submit()
                    }
                }
                Field {
                    Layout.fillWidth: false
                    label: qsTr("It allows")
                    Row {
                        spacing: Theme.s4
                        topPadding: 6
                        Repeater {
                            model: roleTable.perms
                            CheckBox {
                                required property string modelData
                                text: Roles.text(modelData)
                                checked: newRole.allows.indexOf(modelData) >= 0
                                enabled: modelData !== "read"
                                onToggled: newRole.allows = roleTable.perms.filter(p => p === modelData ? checked : newRole.allows.indexOf(p) >= 0)
                            }
                        }
                    }
                }
                Button { Layout.alignment: Qt.AlignBottom; text: qsTr("Make the role"); enabled: roleName.text.trim() !== ""; onClicked: newRole.submit() }
                Item { Layout.fillWidth: true }
            }
        }
    }

    Component {
        id: detailPanel
        AccountDetail { accounts: page }
    }
}
