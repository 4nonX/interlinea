import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .detect note: an icon and its text on a tint, kind "warn",
// "ok" or "info"; what it holds more goes under the text.
Rectangle {
    id: note
    property string text
    property string kind: "info"
    default property alias more: extra.data
    Layout.fillWidth: true
    implicitHeight: body.implicitHeight + 2 * Theme.s2
    radius: Theme.rControl
    color: kind === "warn" ? Theme.warnBg : kind === "ok" ? Theme.okBg : Theme.surface2
    readonly property color fg: kind === "warn" ? Theme.warn : kind === "ok" ? Theme.ok : Theme.text2
    Accessible.role: Accessible.StaticText
    Accessible.name: text
    RowLayout {
        id: body
        x: Theme.s3; y: Theme.s2
        width: parent.width - 2 * Theme.s3
        spacing: Theme.s2
        Icon {
            Layout.alignment: Qt.AlignTop
            Layout.topMargin: 2
            name: note.kind === "warn" ? "alert-triangle" : note.kind === "ok" ? "check" : "info"
            color: note.fg
            size: 14
        }
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            Txt { Layout.fillWidth: true; text: note.text; color: note.fg; font.pixelSize: Theme.small }
            ColumnLayout { id: extra; Layout.fillWidth: true; visible: children.length > 0; spacing: Theme.s2 }
        }
    }
}
