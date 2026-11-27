import QtQuick
import QtQuick.Layouts
import QtQuick.Templates as T
import QtQuick.Controls.Basic as B
import Interlinea

// The web's .dialog: a window over the page with its title (17 px), 24 px
// around its parts, which are 12 px apart; 640 wide, or 920 when wide.
// Its buttons sit at the bottom right; the parts scroll when they are
// taller than the window allows. Escape closes it.
T.Dialog {
    id: dialog
    property bool wide: false
    property string error: ""
    default property alias content: body.data
    property alias buttons: buttonRow.data
    parent: T.Overlay.overlay
    modal: true
    focus: true
    anchors.centerIn: parent
    width: Math.min(wide ? 920 : 640, parent ? parent.width - 48 : 640)
    // The templates leave the implicit size to the style: the parts with
    // the padding, the title and the buttons.
    implicitHeight: implicitContentHeight + topPadding + bottomPadding
                    + (implicitHeaderHeight > 0 ? implicitHeaderHeight + spacing : 0)
                    + (implicitFooterHeight > 0 ? implicitFooterHeight + spacing : 0)
    height: Math.min(implicitHeight, parent ? parent.height - 48 : implicitHeight)
    padding: Theme.s5
    closePolicy: T.Popup.CloseOnEscape
    font.family: Theme.font
    font.pixelSize: Theme.body

    T.Overlay.modal: Rectangle { color: Theme.dark ? Qt.rgba(0, 0, 0, 0.55) : Qt.rgba(20 / 255, 30 / 255, 45 / 255, 0.35) }

    header: Txt {
        text: dialog.title
        topPadding: Theme.s5
        leftPadding: Theme.s5
        rightPadding: Theme.s5
        font.pixelSize: 17
        font.weight: 600
        Accessible.role: Accessible.Heading
    }

    contentItem: B.ScrollView {
        id: scroll
        clip: true
        contentWidth: availableWidth
        implicitHeight: body.implicitHeight
        B.ScrollBar.vertical: ScrollBar { parent: scroll; x: scroll.width - width + 12; height: scroll.height }
        ColumnLayout {
            id: body
            width: scroll.availableWidth
            spacing: Theme.s3
        }
    }

    footer: ColumnLayout {
        spacing: Theme.s2
        Txt {
            Layout.fillWidth: true
            Layout.leftMargin: Theme.s5
            Layout.rightMargin: Theme.s5
            visible: dialog.error !== ""
            text: dialog.error
            role: "error"
            Accessible.role: Accessible.AlertMessage
        }
        RowLayout {
            Layout.fillWidth: true
            Layout.leftMargin: Theme.s5
            Layout.rightMargin: Theme.s5
            Layout.bottomMargin: Theme.s5
            Item { Layout.fillWidth: true }
            Row {
                id: buttonRow
                spacing: Theme.s2
            }
        }
    }

    background: Item {
        Rectangle {
            anchors.fill: parent
            anchors.margins: -6
            anchors.topMargin: -2
            radius: 18
            color: Theme.shadow
            opacity: 0.3
        }
        Rectangle {
            anchors.fill: parent
            radius: 12
            color: Theme.surface
            border.color: Theme.border
        }
    }
}
