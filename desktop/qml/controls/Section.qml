import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .section: a 16 px title and its parts, 12 px apart.
ColumnLayout {
    property string title
    Layout.fillWidth: true
    spacing: Theme.s3
    Txt {
        Layout.fillWidth: true
        visible: parent.title !== ""
        text: parent.title
        role: "h3"
        Accessible.role: Accessible.Heading
    }
}
