import QtQuick
import Interlinea

// A row's first cell: its title in bold (a link when it opens something)
// and its grey line under it.
Column {
    id: cell
    property string title
    property string sub: ""
    property bool link: false
    signal activated()
    spacing: 2
    Text {
        width: cell.width
        text: cell.title
        color: cell.link && area.containsMouse ? Theme.accentText : Theme.text
        font.family: Theme.font
        font.pixelSize: Theme.body
        font.weight: 600
        font.underline: cell.link && area.containsMouse
        wrapMode: Text.Wrap
        Accessible.role: cell.link ? Accessible.Link : Accessible.StaticText
        Accessible.name: cell.title
        Accessible.onPressAction: cell.activated()
        activeFocusOnTab: cell.link
        Keys.onReturnPressed: cell.activated()
        Keys.onSpacePressed: cell.activated()
        FocusRing { visible: parent.activeFocus; radius_: 3; inset: -2 }
        MouseArea {
            id: area
            anchors.fill: parent
            enabled: cell.link
            hoverEnabled: true
            cursorShape: cell.link ? Qt.PointingHandCursor : Qt.ArrowCursor
            onClicked: cell.activated()
        }
    }
    Txt {
        width: cell.width
        visible: cell.sub !== ""
        text: cell.sub
        role: "sub"
    }
}
