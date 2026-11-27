import QtQuick
import QtQuick.Layouts
import Interlinea

// A project's details: its name, the client, when it is due, and a
// description.
GridLayout {
    id: details
    property var form
    columns: 2
    columnSpacing: Theme.s4
    rowSpacing: Theme.s4
    enabled: !form.readOnly
    Layout.fillWidth: true

    Field {
        Layout.alignment: Qt.AlignTop
        Layout.preferredWidth: 1
        label: qsTr("Project name")
        error: details.form.problemsAt("name")
        TextField { text: details.form.s.name || ""; invalid: details.form.problemsAt("name") !== ""; Accessible.name: qsTr("Project name"); onTextEdited: details.form.put("name", text) }
    }
    Field {
        Layout.alignment: Qt.AlignTop
        Layout.preferredWidth: 1
        label: qsTr("Client")
        optional: true
        help: details.form.isNew ? qsTr("Used to suggest the client's memories and termbases.") : ""
        error: details.form.problemsAt("client")
        TextField { text: details.form.s.client || ""; Accessible.name: qsTr("Client"); onTextEdited: details.form.put("client", text) }
    }
    Field {
        Layout.alignment: Qt.AlignTop
        Layout.preferredWidth: 1
        label: qsTr("Due")
        optional: true
        error: details.form.problemsAt("due")
        DueField { iso: details.form.s.due || ""; onEdited: v => details.form.put("due", v) }
    }
    Field {
        Layout.alignment: Qt.AlignTop
        Layout.preferredWidth: 1
        label: qsTr("Description")
        optional: true
        TextField { text: details.form.s.description || ""; Accessible.name: qsTr("Description"); onTextEdited: details.form.put("description", text) }
    }
}
