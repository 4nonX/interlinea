import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// The web's .main > .page: scrolling, 24 px from the top and 32 px from
// the sides, at most 1120 px wide (760 narrow, none wide), its parts 24 px
// apart, with the page's title, description and actions at the top.
B.ScrollView {
    id: page
    property string title: ""
    property string description: ""
    property int maxWidth: 1120
    property list<Item> headerActions
    default property alias content: column.data
    property alias gap: column.spacing
    clip: true
    contentWidth: availableWidth
    B.ScrollBar.horizontal.policy: B.ScrollBar.AlwaysOff
    B.ScrollBar.vertical: ScrollBar {
        parent: page
        x: page.width - width
        height: page.height
        active: page.contentHeight > page.height
    }
    Accessible.role: Accessible.Pane
    Accessible.name: title

    Item {
        width: page.availableWidth
        implicitHeight: column.implicitHeight + Theme.s5 + Theme.s6
        ColumnLayout {
            id: column
            y: Theme.s5
            width: Math.min(page.availableWidth - 2 * Theme.s6, page.maxWidth > 0 ? page.maxWidth : 100000)
            x: (page.availableWidth - width) / 2
            spacing: Theme.s5
            PageHeader {
                visible: page.title !== ""
                title: page.title
                description: page.description
                actions: page.headerActions
            }
        }
    }
}
