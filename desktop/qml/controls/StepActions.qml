import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .panel-actions: a line, and the step's buttons at the right.
ColumnLayout {
    default property alias buttons: row.data
    Layout.fillWidth: true
    spacing: Theme.s4
    Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
    RowLayout {
        Layout.fillWidth: true
        Item { Layout.fillWidth: true }
        Row { id: row; spacing: Theme.s2 }
    }
}
