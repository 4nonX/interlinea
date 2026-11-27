import QtQuick
import QtQuick.Layouts
import Interlinea

// A new entry: its first term, the term's language and status (the web's
// .new-entry); closed, a bar with the button that opens it.
ColumnLayout {
    id: form
    property var page
    property bool opened: false
    Layout.fillWidth: true
    visible: !page.opened || page.may("edit")
    spacing: 0
    onOpenedChanged: if (opened) { firstTerm.text = page.query.trim(); firstTerm.forceActiveFocus() }
    readonly property var langs: page.languages.length ? page.languages : Backend.commonLanguages().map(l => l.code)

    Button {
        visible: !form.opened
        Layout.margins: Theme.s1
        Layout.leftMargin: Theme.s3
        kind: "quiet"
        small: true
        iconName: "plus"
        text: qsTr("New entry")
        onClicked: form.opened = true
    }
    ColumnLayout {
        visible: form.opened
        Layout.fillWidth: true
        Layout.margins: Theme.s3
        spacing: Theme.s2
        Accessible.name: qsTr("New entry")
        Field { label: qsTr("First term"); TextField { id: firstTerm; onAccepted: add.clicked() } }
        RowLayout {
            spacing: Theme.s2
            ComboBox { id: lang; Layout.fillWidth: true; model: form.langs.map(l => Backend.languageName(l)); Accessible.name: qsTr("Language") }
            ComboBox { id: status; implicitWidth: 120; model: ["proposed", "approved", "forbidden"].map(s => TermData.statusLabel(s)); Accessible.name: qsTr("Status") }
        }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Add the other languages and fields in the entry.") }
        RowLayout {
            Item { Layout.fillWidth: true }
            Button { kind: "quiet"; small: true; text: qsTr("Cancel"); onClicked: form.opened = false }
            Button {
                id: add
                kind: "primary"; small: true
                text: qsTr("Add entry")
                enabled: firstTerm.text.trim() !== ""
                onClicked: if (enabled) page.addEntry(firstTerm.text, form.langs[lang.currentIndex], ["proposed", "approved", "forbidden"][status.currentIndex])
            }
        }
    }
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
}
