import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .panel: a white box with a 10 px radius, 24 px in, its title
// (18 px) over its parts, which are 24 px apart. Children go into it.
Rectangle {
    id: panel
    property string title: ""
    property string description: ""
    default property alias content: body.data
    property alias spacing: body.spacing
    Layout.fillWidth: true
    implicitWidth: body.implicitWidth + 2 * Theme.s5
    implicitHeight: body.implicitHeight + 2 * Theme.s5
    radius: Theme.rPanel
    color: Theme.surface
    border.color: Theme.border
    Accessible.role: Accessible.Grouping
    Accessible.name: title

    ColumnLayout {
        id: body
        x: Theme.s5
        y: Theme.s5
        width: panel.width - 2 * Theme.s5
        spacing: Theme.s5
        Txt {
            Layout.fillWidth: true
            visible: panel.title !== ""
            text: panel.title
            role: "h2"
            Accessible.role: Accessible.Heading
        }
        Txt {
            Layout.fillWidth: true
            Layout.topMargin: -Theme.s3
            visible: panel.description !== ""
            text: panel.description
            role: "muted"
        }
    }
}
