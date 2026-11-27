import QtQuick
import QtQuick.Layouts
import Interlinea

// The account chosen on the Accounts page: its details, whether it is an
// administrator or disabled, its password, where it is signed in, its
// links to the identity provider, its roles, and deleting it.
Panel {
    id: detail
    property var accounts           // the AccountsPage
    readonly property var u: accounts.user || {}
    readonly property string username: u.username || ""
    readonly property string base: "/api/users/" + (u.id || 0)
    readonly property int version: u.version || 0
    readonly property var conn: accounts.conn
    readonly property bool me: !!conn && (conn.user || {}).id === u.id
    readonly property string provider: conn ? conn.provider : ""
    title: u.name || username
    spacing: Theme.s3

    function attempt(method, path, body, done, query, ver) { accounts.attempt(method, path, body, done, query, ver) }

    Txt {
        role: "sub"
        text: Fmt.fill(detail.me ? qsTr("{username}, made on {date}. This is you.") : qsTr("{username}, made on {date}."),
                       { username: detail.username, date: detail.accounts.day(detail.u.createdAt) })
    }
    Txt { visible: detail.accounts.problem !== ""; Layout.fillWidth: true; role: "error"; text: detail.accounts.problem }

    // Details (.server-form, at most 28 rem wide)
    ColumnLayout {
        id: details
        Layout.fillWidth: true
        Layout.maximumWidth: 448
        spacing: Theme.s3
        Accessible.name: qsTr("Details")
        readonly property bool changed: detailName.text !== (detail.u.name || "") || detailEmail.text !== (detail.u.email || "")
        function save() {
            if (changed) detail.attempt("PATCH", detail.base, { name: detailName.text.trim(), email: detailEmail.text.trim() }, qsTr("Saved the details"), undefined, detail.version)
        }
        Field { label: qsTr("Name"); TextField { id: detailName; text: detail.u.name || ""; onAccepted: details.save() } }
        Field { label: qsTr("Email"); TextField { id: detailEmail; text: detail.u.email || ""; onAccepted: details.save() } }
        Button { text: qsTr("Save details"); enabled: details.changed; onClicked: details.save() }
    }

    // .account-switches
    CheckBox {
        text: qsTr("Administrator: may do everything on the server")
        checked: !!detail.u.admin
        onToggled: detail.attempt("PATCH", detail.base, { admin: checked },
                                  Fmt.fill(checked ? qsTr("{name} is an administrator") : qsTr("{name} is no longer an administrator"), { name: detail.username }), undefined, detail.version)
    }
    CheckBox {
        text: qsTr("Disabled: can't sign in; signed out everywhere")
        checked: !!detail.u.disabled
        onToggled: detail.attempt("PATCH", detail.base, { disabled: checked },
                                  Fmt.fill(checked ? qsTr("Disabled {name} and signed them out everywhere") : qsTr("Enabled {name}"), { name: detail.username }), undefined, detail.version)
    }

    // Password (.server-connect)
    RowLayout {
        id: pw
        Layout.fillWidth: true
        spacing: Theme.s3
        function send() {
            if (!newPassword.text) return
            detail.attempt("PUT", detail.base + "/password", { password: newPassword.text },
                           Fmt.fill(qsTr("Set a new password for {name}; their sign-ins ended"), { name: detail.username }))
            newPassword.text = ""
        }
        Field {
            label: detail.u.hasPassword ? qsTr("New password") : qsTr("Password (the account has none)")
            TextField { id: newPassword; echoMode: TextInput.Password; onAccepted: pw.send() }
        }
        Button { Layout.alignment: Qt.AlignBottom; text: qsTr("Set the password"); enabled: newPassword.text !== ""; onClicked: pw.send() }
    }
    ConfirmButton {
        visible: !!detail.u.hasPassword && detail.accounts.identities.length > 0
        text: qsTr("Remove the password")
        action: qsTr("Remove")
        question: Fmt.fill(qsTr("{name} then signs in only through the identity provider."), { name: detail.username })
        onConfirmed: detail.attempt("PUT", detail.base + "/password", { password: "" }, Fmt.fill(qsTr("{name} signs in through the provider only"), { name: detail.username }))
    }

    // Sign-ins
    Txt { role: "label"; text: qsTr("Sign-ins") }
    Txt { visible: detail.accounts.signIns.length === 0; role: "muted"; text: qsTr("Not signed in anywhere.") }
    Repeater {
        model: detail.accounts.signIns
        Txt {
            required property var modelData
            Layout.fillWidth: true
            role: "sub"
            text: Fmt.fill(qsTr("{name}: since {since}, last used {used}, ends {ends}"), { name: modelData.name || "", since: detail.accounts.day(modelData.createdAt),
                                                                                         used: detail.accounts.day(modelData.lastUsed), ends: detail.accounts.day(modelData.expiresAt) })
        }
    }
    Button {
        visible: detail.accounts.signIns.length > 0
        small: true
        text: qsTr("Sign out everywhere")
        onClicked: detail.attempt("DELETE", detail.base + "/tokens", undefined, Fmt.fill(qsTr("Signed {name} out everywhere"), { name: detail.username }))
    }

    // Sign-in through the identity provider.
    ColumnLayout {
        visible: detail.provider !== "" || detail.accounts.identities.length > 0
        Layout.fillWidth: true
        spacing: Theme.s3
        Txt { role: "label"; text: Fmt.fill(qsTr("Sign-in through {provider}"), { provider: detail.provider || qsTr("the identity provider") }) }
        Txt {
            visible: detail.accounts.identities.length === 0
            Layout.fillWidth: true
            role: "muted"
            text: Fmt.fill(qsTr("Not linked. {name} signs in with a password, or links it themselves after signing in."), { name: detail.username })
        }
        Repeater {
            model: detail.accounts.identities
            RowLayout {
                required property var modelData
                spacing: Theme.s3
                Layout.fillWidth: true
                Txt { Layout.fillWidth: true; role: "sub"; text: Fmt.fill(qsTr("{host}, subject {subject}"), { host: (modelData.issuer || "").replace(/^[a-z]+:\/\//, "").replace(/[\/:].*$/, ""), subject: modelData.subject || "" }) }
                Button {
                    kind: "quiet"
                    small: true
                    text: qsTr("Unlink")
                    onClicked: detail.attempt("DELETE", detail.base + "/identities", undefined, qsTr("Unlinked the sign-in"), { issuer: modelData.issuer, subject: modelData.subject })
                }
            }
        }
        RowLayout {
            id: linkRow
            visible: detail.provider !== ""
            Layout.fillWidth: true
            spacing: Theme.s3
            function send() {
                const s = subject.text.trim()
                if (!s) return
                detail.attempt("POST", detail.base + "/identities", { issuer: "", subject: s }, Fmt.fill(qsTr("Linked {name} to the sign-in"), { name: detail.username }))
                subject.text = ""
            }
            Field {
                label: Fmt.fill(qsTr("Subject at {provider} (its ID for the person)"), { provider: detail.provider })
                TextField { id: subject; onAccepted: linkRow.send() }
            }
            Button { Layout.alignment: Qt.AlignBottom; iconName: "link"; text: qsTr("Link the sign-in"); enabled: subject.text.trim() !== ""; onClicked: linkRow.send() }
        }
    }

    // Roles
    Txt { role: "label"; text: qsTr("Roles") }
    Txt {
        visible: !!detail.u.admin
        Layout.fillWidth: true
        role: "sub"
        text: Fmt.fill(qsTr("As an administrator, {name} may do everything; roles add nothing."), { name: detail.username })
    }
    Txt {
        visible: detail.accounts.grants.length === 0
        Layout.fillWidth: true
        role: "muted"
        text: Fmt.fill(qsTr("No roles. {name} sees only the projects with work given to them."), { name: detail.username })
    }
    Table {
        visible: detail.accounts.grants.length > 0
        name: qsTr("Roles")
        columns: [{ title: qsTr("On"), fill: 1 }, { title: qsTr("Role"), width: 200 }, { title: "", width: 100 }]
        Repeater {
            model: detail.accounts.grants
            TableRow {
                id: grow
                required property var modelData
                required property int index
                readonly property string on: detail.accounts.resourceName(modelData)
                last: index === detail.accounts.grants.length - 1
                CellTitle { title: grow.on; sub: detail.accounts.kindText(grow.modelData.kind) }
                ComboBox {
                    readonly property var choices: Roles.choices(detail.accounts.roles, grow.modelData.role)
                    implicitHeight: 28
                    model: choices.map(c => c.label)
                    currentIndex: Math.max(0, choices.findIndex(c => c.id === grow.modelData.role))
                    Accessible.name: Fmt.fill(qsTr("Role on {resource}"), { resource: grow.on })
                    onActivated: i => {
                        if (choices[i].id === grow.modelData.role) return
                        detail.attempt("PUT", "/api/grants", Object.assign({}, grow.modelData, { role: choices[i].id }), qsTr("Changed the role"))
                    }
                }
                Button {
                    kind: "quiet"
                    small: true
                    text: qsTr("Remove")
                    Accessible.name: Fmt.fill(qsTr("Remove the role on {resource}"), { resource: grow.on })
                    onClicked: detail.attempt("DELETE", "/api/grants", undefined, qsTr("Took the role away"),
                                              { userId: detail.u.id, kind: grow.modelData.kind, resource: grow.modelData.resource })
                }
            }
        }
    }
    // .role-add: in a row, or two to a row where the account is narrow.
    GridLayout {
        id: give
        Layout.fillWidth: true
        readonly property bool wide: detail.width >= 760
        columns: wide ? 5 : 2
        columnSpacing: Theme.s3
        rowSpacing: Theme.s3
        Accessible.name: qsTr("Give a role")
        readonly property var kinds: ["project", "memory", "termbase", "all"]
        readonly property string kind: kinds[kindBox.currentIndex]
        readonly property var resources: {
            const out = [{ id: "*", label: qsTr("All of them, also new ones") }]
            if (kind === "project")
                for (const p of detail.accounts.projects) out.push({ id: String(p.id), label: p.settings.name })
            else if (kind === "memory" || kind === "termbase")
                for (const m of detail.accounts.catalog[kind === "memory" ? "memories" : "termbases"] || []) out.push({ id: m.name, label: m.name })
            return out
        }
        readonly property var roleChoices: Roles.choices(detail.accounts.roles, "")
        Field {
            Layout.fillWidth: !give.wide
            Layout.preferredWidth: 160
            label: qsTr("On")
            ComboBox { id: kindBox; model: [qsTr("Projects"), qsTr("Memories"), qsTr("Termbases"), qsTr("Everything")] }
        }
        Field {
            visible: give.kind !== "all"
            Layout.fillWidth: !give.wide
            Layout.preferredWidth: 240
            label: qsTr("Which")
            ComboBox { id: whichBox; model: give.resources.map(r => r.label) }
        }
        Field {
            Layout.fillWidth: !give.wide
            Layout.preferredWidth: 160
            label: qsTr("Role")
            ComboBox { id: roleBox; model: give.roleChoices.map(c => c.label); currentIndex: Math.max(0, give.roleChoices.findIndex(c => c.id === "edit")) }
        }
        Button {
            Layout.alignment: Qt.AlignBottom
            text: qsTr("Give the role")
            onClicked: detail.attempt("PUT", "/api/grants", {
                userId: detail.u.id, kind: give.kind,
                resource: give.kind === "all" ? "*" : (give.resources[whichBox.currentIndex] || give.resources[0]).id,
                role: give.roleChoices[roleBox.currentIndex].id
            }, qsTr("Gave the role"))
        }
        Item { visible: give.wide; Layout.fillWidth: true }
    }
    Txt {
        Layout.fillWidth: true
        role: "sub"
        text: qsTr("Manage on all projects makes someone a project manager: they make projects and give out work. Manage on all memories or termbases lets them make those.")
    }

    // .account-delete
    Rectangle { visible: !detail.me; Layout.fillWidth: true; height: 1; color: Theme.border }
    ConfirmButton {
        visible: !detail.me
        small: false
        iconName: "trash"
        text: qsTr("Delete the account")
        action: qsTr("Delete")
        question: Fmt.fill(qsTr("Delete {name}? Their work stays signed with their name; their roles and sign-ins go."), { name: detail.username })
        onConfirmed: {
            const name = detail.username, acc = detail.accounts
            Api.del(acc.cid, detail.base, res => {
                if (!res.ok) { acc.problem = res.error.message; return }
                acc.notice(Fmt.fill(qsTr("Deleted the account {name}"), { name: name }))
                acc.selected = 0
                acc.reload()
            })
        }
    }
}
