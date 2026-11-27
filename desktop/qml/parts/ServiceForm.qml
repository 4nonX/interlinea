import QtQuick
import QtQuick.Layouts
import Interlinea

// The form for a new service at a place. A translation service is one of
// those built in, set up with the key of one's own account (and a model or
// a region where it needs one), or a service of one's own at an address;
// a grammar service is one of one's own.
Rectangle {
    id: form
    property string cid
    property string kind: "translate"
    property var kinds: []
    signal saved(string name)
    signal cancelled()
    signal notice(string text)
    readonly property bool check: kind === "check"
    property string service: kinds.length ? kinds[0].service : "own"
    readonly property var info: check ? null : (kinds.find(k => k.service === service) || null)
    readonly property bool own: check || !info || service === "own"
    property var models: []
    readonly property bool needsModel: !own && !!info && !!info.model
    implicitHeight: body.implicitHeight + 2 * Theme.s3
    radius: Theme.rControl
    color: Theme.surface2
    border.color: Theme.border
    Accessible.role: Accessible.Grouping
    Accessible.name: check ? qsTr("A grammar service") : qsTr("A translation service")
    onServiceChanged: {
        name.text = own ? "" : (info ? info.name : "")
        models = []
    }

    GridLayout {
        id: body
        x: Theme.s3; y: Theme.s3
        width: parent.width - 2 * Theme.s3
        columns: 2
        columnSpacing: Theme.s4
        rowSpacing: Theme.s3
        Field {
            Layout.columnSpan: 2
            visible: !form.check
            label: qsTr("Service")
            ComboBox {
                model: form.kinds.map(k => k.name)
                currentIndex: Math.max(0, form.kinds.findIndex(k => k.service === form.service))
                onActivated: i => form.service = form.kinds[i].service
            }
        }
        Text {
            Layout.columnSpan: 2
            Layout.fillWidth: true
            visible: !form.check && !!form.info && !!(form.info.note || form.info.signup)
            textFormat: Text.StyledText
            wrapMode: Text.Wrap
            color: Theme.text2
            linkColor: Theme.accentText
            font.family: Theme.font
            font.pixelSize: Theme.small
            text: form.info ? (form.info.note || "").replace(/&/g, "&amp;").replace(/</g, "&lt;")
                              + (form.info.signup ? " <a href=\"" + form.info.signup + "\">" + qsTr("Get a key") + "</a>" : "") : ""
            onLinkActivated: link => Qt.openUrlExternally(link)
        }
        Field { label: qsTr("Name"); TextField { id: name } }
        Field {
            visible: form.own || (!!form.info && !!form.info.url)
            label: form.own ? qsTr("Address requests are sent to") : qsTr("Address of a service that speaks the same API")
            optional: !form.own
            TextField {
                id: url
                placeholderText: form.check ? "https://…/v2/check" : form.own ? "https://" : "https://api.openai.com/v1"
            }
        }
        Field {
            visible: form.own
            label: qsTr("Header that signs requests")
            optional: true
            TextField { id: headerName; placeholderText: "Authorization" }
        }
        Field {
            visible: form.own
            label: qsTr("Its value")
            optional: true
            TextField { id: headerValue; echoMode: TextInput.Password; placeholderText: qsTr("Bearer …") }
        }
        Field { visible: !form.own; label: qsTr("Key"); TextField { id: key; echoMode: TextInput.Password } }
        Field {
            visible: !form.own && !!form.info && !!form.info.region
            label: qsTr("Region of the key")
            optional: true
            help: qsTr("Not needed for a global key.")
            TextField { id: region; placeholderText: "westeurope" }
        }
        Field {
            Layout.columnSpan: 2
            visible: !form.own && !!form.info && !!form.info.model
            label: qsTr("Model")
            RowLayout {
                spacing: Theme.s2
                ComboBox {
                    id: model
                    Layout.fillWidth: true
                    editable: true
                    model: form.models
                }
                Button {
                    small: true
                    text: qsTr("List the models")
                    enabled: key.text.trim() !== ""
                    onClicked: {
                        enabled = false
                        const body = { service: form.service, key: key.text.trim() }
                        if (url.visible) body.url = url.text.trim()
                        Api.post(form.cid, "/api/providers/models", body, r => {
                            enabled = Qt.binding(() => key.text.trim() !== "")
                            if (!r.ok) { form.notice(r.error.message); return }
                            const was = model.editText
                            form.models = r.data.models || []
                            if (was) model.editText = was
                            form.notice(form.models.length ? Fmt.fill(qsTr("The key may use {n} models."), { n: form.models.length }) : qsTr("The key may use no model."))
                        })
                    }
                }
            }
        }
        Row {
            Layout.columnSpan: 2
            spacing: Theme.s2
            Button {
                kind: "primary"
                small: true
                text: qsTr("Add the service")
                // What a service needs before it can be added; the core
                // says what is wrong with the rest when it is saved.
                enabled: name.text.trim() !== "" && (form.own ? url.text.trim() !== ""
                         : key.text.trim() !== "" && (!form.needsModel || model.editText.trim() !== ""))
                onClicked: {
                    const n = name.text.trim()
                    const body = { kind: form.kind, service: form.own ? "own" : form.service, url: url.visible ? url.text.trim() : "",
                                   headerName: form.own ? headerName.text.trim() : "", headerValue: form.own ? headerValue.text : key.text,
                                   region: region.visible ? region.text.trim() : "", model: form.needsModel ? model.editText.trim() : "" }
                    Api.put(form.cid, "/api/providers/" + Api.seg(n), body, r => {
                        if (!r.ok) { form.notice(r.error.message); return }
                        form.saved(n)
                    })
                }
            }
            Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: form.cancelled() }
        }
    }
}
