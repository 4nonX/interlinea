import QtQuick
import QtQuick.Layouts
import Interlinea

// A row of the web's .settings-list: what it is on the left, what to do
// with it on the right, 36 px high, a line under it.
Item {
    id: row
    default property alias what: left.data
    property alias actions: right.data
    Layout.fillWidth: true
    implicitHeight: Math.max(36, Math.max(left.implicitHeight, right.implicitHeight) + 8)
    RowLayout {
        id: left
        anchors.left: parent.left
        anchors.right: right.left
        anchors.rightMargin: Theme.s3
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.s2
    }
    Row {
        id: right
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.s2
    }
    Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
}
