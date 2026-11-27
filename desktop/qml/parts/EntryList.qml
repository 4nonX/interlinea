import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// The entries of the opened termbase: search and status, the new entry,
// and the list, each entry with its head term, its other languages and its
// domain; what to do when there are none.
ColumnLayout {
    id: list
    property var page
    default property alias form: formHolder.data
    spacing: 0

    // .filters
    RowLayout {
        Layout.fillWidth: true
        Layout.margins: Theme.s2
        spacing: Theme.s2
        TextField {
            id: search
            Layout.fillWidth: true
            placeholderText: qsTr("Search terms and fields")
            leftPadding: 30
            text: page.query
            onTextChanged: page.query = text
            Icon { x: 9; anchors.verticalCenter: parent.verticalCenter; name: "search"; color: Theme.text3; size: 15 }
        }
        ComboBox {
            implicitWidth: 120
            readonly property var states: ["", "approved", "proposed", "forbidden"]
            model: [qsTr("All statuses"), TermData.statusLabel("approved"), TermData.statusLabel("proposed"), TermData.statusLabel("forbidden")]
            currentIndex: Math.max(0, states.indexOf(page.status))
            Accessible.name: qsTr("Status")
            onActivated: i => page.status = states[i]
        }
    }
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
    ColumnLayout { id: formHolder; Layout.fillWidth: true; spacing: 0 }

    ListView {
        id: view
        Layout.fillWidth: true
        Layout.fillHeight: true
        visible: page.shown.length > 0
        clip: true
        model: page.shown
        currentIndex: page.shown.findIndex(e => e.id === page.selected)
        keyNavigationEnabled: true
        activeFocusOnTab: true
        Accessible.role: Accessible.List
        Accessible.name: Fmt.fill(qsTr("Entries in {name}"), { name: page.name })
        B.ScrollBar.vertical: ScrollBar {}
        onCurrentIndexChanged: if (currentIndex >= 0 && page.shown[currentIndex] && page.shown[currentIndex].id !== page.selected) page.selected = page.shown[currentIndex].id
        delegate: B.ItemDelegate {
            id: row
            required property var modelData
            required property int index
            readonly property var headTerm: TermData.head(modelData, page.languages[0] || "")
            readonly property string domain: { const f = (modelData.fields || []).find(x => (x.field || "").toLowerCase() === "domain"); return f ? f.value : "" }
            width: view.width
            highlighted: page.selected === modelData.id
            hoverEnabled: true
            padding: 0
            Accessible.name: headTerm.text
            onClicked: { page.selected = modelData.id; view.forceActiveFocus() }
            background: Rectangle {
                color: row.highlighted ? Theme.accentWeak : row.hovered ? Theme.surface2 : "transparent"
                Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
            }
            contentItem: ColumnLayout {
                spacing: 3
                EntryTerms {
                    Layout.fillWidth: true
                    Layout.leftMargin: Theme.s4
                    Layout.rightMargin: Theme.s4
                    Layout.topMargin: Theme.s3
                    entry: row.modelData
                    languages: page.languages
                }
                Txt { Layout.leftMargin: Theme.s4; Layout.bottomMargin: Theme.s3; visible: row.domain !== ""; text: row.domain; role: "sub" }
                Item { visible: row.domain === ""; implicitHeight: Theme.s3 - 3 }
            }
        }
    }

    // .empty
    ColumnLayout {
        Layout.fillWidth: true
        Layout.fillHeight: true
        Layout.margins: Theme.s4
        visible: page.opened && page.shown.length === 0
        spacing: Theme.s2
        Txt {
            Layout.fillWidth: true
            role: "h3"
            text: page.entries.length === 0 ? qsTr("No entries yet")
                 : !page.query.trim() ? qsTr("Nothing has this status") : Fmt.fill(qsTr("Nothing matches “{query}”"), { query: page.query })
        }
        Txt {
            Layout.fillWidth: true
            visible: page.entries.length === 0
            role: "muted"
            text: !page.may("edit") ? (page.isProject && page.masterEntries.length ? qsTr("The project has no entries of its own. The master's are under Master.") : qsTr("Those who may edit this termbase add its entries."))
                 : page.isProject && page.masterEntries.length ? qsTr("The project has no entries of its own yet: it uses the master's, which are under Master. Add an entry above, or change one of the master's for this project.")
                 : qsTr("Add an entry above, import a TBX, .sdltb or XLSX file, or add terms while you translate.")
        }
        Button {
            visible: page.entries.length === 0 && page.may("edit")
            small: true
            iconName: "upload"
            text: qsTr("Import a file")
            onClicked: importHere.start(page.cid, page.name)
        }
        Button {
            visible: page.entries.length > 0
            small: true
            text: qsTr("Clear search")
            onClicked: { page.query = ""; page.status = "" }
        }
        Item { Layout.fillHeight: true }
    }
    ImportTermbaseDialog {
        id: importHere
        lists: page.lists
        onImported: (cid, name, message) => { page.notice(message); page.reload() }
    }
}
