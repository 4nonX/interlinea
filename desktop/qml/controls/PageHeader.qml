import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .page-header: the title (22 px), its description, and the
// page's actions on the right.
RowLayout {
    id: header
    property string title
    property string description
    property list<Item> actions
    Layout.fillWidth: true
    spacing: Theme.s4
    ColumnLayout {
        Layout.fillWidth: true
        Layout.alignment: Qt.AlignTop
        spacing: Theme.s1
        Txt {
            Layout.fillWidth: true
            text: header.title
            role: "h1"
            Accessible.role: Accessible.Heading
        }
        Txt {
            Layout.fillWidth: true
            visible: header.description !== ""
            text: header.description
            role: "muted"
        }
    }
    Row {
        Layout.alignment: Qt.AlignTop
        spacing: Theme.s2
        data: header.actions
    }
}
