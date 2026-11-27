import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's .choice: a framed check box with its title in bold and
// what it means under it; blue when chosen.
T.CheckBox {
    id: control
    property string description: ""
    // A tag beside the title ("Recommended"), and what is put inside goes
    // under the description.
    property string tag: ""
    default property alias below: extra.data
    implicitWidth: implicitContentWidth + leftPadding + rightPadding
    implicitHeight: implicitContentHeight + topPadding + bottomPadding
    padding: Theme.s3
    leftPadding: Theme.s3 + 16 + Theme.s2
    hoverEnabled: true
    font.family: Theme.font
    font.pixelSize: Theme.body
    opacity: enabled ? 1 : 0.55
    Accessible.description: description

    indicator: Rectangle {
        x: Theme.s3
        y: Theme.s3 + 2
        width: 16
        height: 16
        border.color: control.checked ? Theme.accent : control.hovered ? Theme.borderHover : Theme.borderStrong
        radius: 3
        border.width: 1
        color: control.checked ? Theme.accent : Theme.surface
        Icon { anchors.centerIn: parent; name: "check"; color: "#ffffff"; size: 12; visible: control.checked }
    }

    contentItem: Column {
        spacing: 2
        Row {
            spacing: Theme.s2
            Txt { text: control.text; role: "strong"; width: Math.min(implicitWidth, control.availableWidth - (tagBadge.visible ? tagBadge.width + Theme.s2 : 0)) }
            Badge { id: tagBadge; visible: control.tag !== ""; text: control.tag; kind: "accent"; anchors.verticalCenter: parent.verticalCenter }
        }
        Txt { width: parent.width; visible: control.description !== ""; text: control.description; role: "muted"; font.weight: control.checked ? 500 : 400 }
        Column {
            id: extra
            width: parent.width
            topPadding: children.length ? Theme.s1 : 0
            spacing: Theme.s1
        }
    }

    background: Rectangle {
        radius: Theme.rControl
        color: control.checked ? Theme.accentWeak : control.hovered ? Theme.surface2 : Theme.surface
        border.color: control.checked ? Theme.accent : Theme.border
        border.width: 1
        FocusRing { target: control }
    }
}
