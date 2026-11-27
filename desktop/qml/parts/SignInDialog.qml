import QtQuick
import QtQuick.Layouts
import Interlinea

// Signing in to a team server: its address, then the sign-in it offers
// (a password, or its identity provider in the browser), or, where it
// has no accounts yet, its first administrator.
Dialog {
    id: dlg
    title: qsTr("Sign in to a team server")
    signal signedIn(string connectionId)
    property string url: ""
    property var info: null
    property bool busy: false
    property string status: ""

    function start(address) {
        url = address
        info = null
        error = ""
        status = ""
        password.text = ""
        open()
        if (url) lookUp()
    }
    function lookUp() {
        const typed = address.text.trim() || url
        if (!typed) {
            error = qsTr("Type the server's address, such as interlinea.example.com.")
            return
        }
        url = Backend.serverUrl(typed)
        error = ""
        busy = true
        status = Fmt.fill(qsTr("Looking up {host}…"), { host: url.replace(/^https?:\/\//, "").replace(/\/.*$/, "") })
        Backend.probe(url)
    }
    function signIn() {
        if (!username.text.trim() || !password.text) {
            error = qsTr("Type your username and password.")
            return
        }
        error = ""
        busy = true
        status = qsTr("Signing in…")
        Backend.signInWithPassword(info.url, username.text.trim(), password.text)
    }
    onClosed: if (busy && info && info.provider) Backend.cancelProviderSignIn()

    Connections {
        target: Backend
        function onProbed(u, i) {
            if (u !== dlg.url) return
            dlg.busy = false
            dlg.status = ""
            if (i.error) { dlg.error = i.error.message; return }
            dlg.info = i
        }
        function onSignedIn(u, cid, e) {
            if (!dlg.info || u !== dlg.info.url) return
            dlg.busy = false
            dlg.status = ""
            if (e.message) { dlg.error = e.message; return }
            dlg.signedIn(cid)
            dlg.close()
        }
    }

    // The address.
    ColumnLayout {
        Layout.fillWidth: true
        visible: !dlg.info
        spacing: Theme.s3
        Txt { Layout.fillWidth: true; role: "muted"; text: qsTr("A team server keeps projects, memories and termbases for a team. Ask its administrator for its address.") }
        Field {
            label: qsTr("Server address")
            TextField { id: address; text: dlg.url; placeholderText: "https://interlinea.example.com"; onAccepted: dlg.lookUp() }
        }
    }
    // Signing in.
    ColumnLayout {
        Layout.fillWidth: true
        visible: !!dlg.info && !dlg.info.setup
        spacing: Theme.s3
        Txt {
            Layout.fillWidth: true
            text: dlg.info ? Fmt.fill(qsTr("Sign in to {host} (Interlinea {version})."), { host: dlg.url.replace(/^https?:\/\//, "").replace(/\/.*$/, ""), version: dlg.info.version }) : ""
        }
        Button {
            visible: !!dlg.info && !!dlg.info.provider
            text: dlg.info ? Fmt.fill(qsTr("Sign in with {provider}"), { provider: dlg.info.provider }) : ""
            enabled: !dlg.busy
            onClicked: {
                dlg.error = ""
                dlg.busy = true
                dlg.status = qsTr("Finish signing in in your browser.")
                Backend.signInWithProvider(dlg.info.url)
            }
        }
        Field { label: qsTr("Username"); TextField { id: username } }
        Field { label: qsTr("Password"); TextField { id: password; echoMode: TextInput.Password; onAccepted: dlg.signIn() } }
    }
    // The first administrator of a new server.
    ColumnLayout {
        Layout.fillWidth: true
        visible: !!dlg.info && !!dlg.info.setup
        spacing: Theme.s3
        Txt { Layout.fillWidth: true; role: "muted"; text: qsTr("This server has no accounts yet. Make its first administrator with the setup token from the server's log.") }
        Field { label: qsTr("Setup token"); TextField { id: setupToken; echoMode: TextInput.Password } }
        Field { label: qsTr("Username"); TextField { id: adminUser } }
        Field { label: qsTr("Name"); TextField { id: adminName } }
        Field { label: qsTr("Email"); TextField { id: adminEmail } }
        Field { label: qsTr("Password"); help: qsTr("12 characters or more."); TextField { id: adminPassword; echoMode: TextInput.Password } }
    }
    Txt { Layout.fillWidth: true; visible: dlg.status !== ""; role: "muted"; text: dlg.status; Accessible.role: Accessible.StatusBar }

    buttons: [
        Button { visible: !!dlg.info; kind: "quiet"; text: qsTr("Other server"); onClicked: { dlg.info = null; dlg.error = "" } },
        Button { kind: "quiet"; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button {
            kind: "primary"
            enabled: !dlg.busy
            text: !dlg.info ? qsTr("Continue") : dlg.info.setup ? qsTr("Make the administrator") : qsTr("Sign in")
            onClicked: {
                if (!dlg.info) dlg.lookUp()
                else if (!dlg.info.setup) dlg.signIn()
                else {
                    dlg.error = ""
                    dlg.busy = true
                    dlg.status = qsTr("Making the administrator…")
                    Backend.setUpServer(dlg.info.url, setupToken.text.trim(), adminUser.text.trim(), adminName.text.trim(), adminEmail.text.trim(), adminPassword.text)
                }
            }
        }
    ]
}
