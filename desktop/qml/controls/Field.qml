import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .field: the label in bold (with "optional" after it), the
// control, and help or the problem under it.
ColumnLayout {
    id: field
    property string label: ""
    property bool optional: false
    property string help: ""
    property string error: ""
    default property alias control: holder.data
    Layout.fillWidth: true
    spacing: Theme.s2

    Row {
        spacing: 6
        visible: field.label !== ""
        Txt { text: field.label; role: "label"; wrapMode: Text.NoWrap }
        Txt { visible: field.optional; text: qsTr("optional"); color: Theme.text3; wrapMode: Text.NoWrap }
    }
    Item {
        id: holder
        Layout.fillWidth: true
        implicitHeight: childrenRect.height
        implicitWidth: children.length ? children[0].implicitWidth : 0
        onChildrenChanged: {
            for (let i = 0; i < children.length; i++) {
                const c = children[i]
                c.width = Qt.binding(() => holder.width)
                if (field.label !== "" && !c.Accessible.name)
                    c.Accessible.name = field.label
            }
        }
    }
    Txt {
        Layout.fillWidth: true
        visible: field.error !== ""
        text: field.error
        role: "error"
        font.pixelSize: Theme.small
    }
    Txt {
        Layout.fillWidth: true
        visible: field.help !== ""
        text: field.help
        role: "help"
    }
}
