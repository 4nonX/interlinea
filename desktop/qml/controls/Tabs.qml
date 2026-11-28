import QtQuick
import QtQuick.Templates as T
import Interlinea

// The web's .tabs: tabs over a line, the chosen one underlined in the
// accent. model: [text, …] or [{ text, icon, visible, enabled }, …].
Item {
    id: tabs
    property var model: []
    property int current: 0
    property bool small: false
    property string name: ""
    // The space before the first tab: the web's padding of .tabs, none for
    // tabs at the top of a page.
    property real inset: Theme.s3
    signal changed(int index)
    implicitHeight: (small ? 32 : 38)
    implicitWidth: row.implicitWidth
    Accessible.role: Accessible.PageTabList
    Accessible.name: name
    Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
    // Where the tabs are wider than their place (a narrow pane, a long
    // word), they scroll sideways, and the chosen one or the one with
    // the focus is brought into view.
    function bringIntoView(item) {
        if (!item || strip.width <= 0) return
        const left = item.x + row.x, right = left + item.width
        if (left < strip.contentX) strip.contentX = Math.max(0, left - tabs.inset)
        else if (right > strip.contentX + strip.width) strip.contentX = Math.min(strip.contentWidth - strip.width, right - strip.width + tabs.inset)
    }
    onCurrentChanged: Qt.callLater(() => tabs.bringIntoView(tabRepeater.itemAt(tabs.current)))
    onWidthChanged: Qt.callLater(() => tabs.bringIntoView(tabRepeater.itemAt(tabs.current)))
    Flickable {
        id: strip
        anchors.fill: parent
        contentWidth: row.x + row.width + tabs.inset
        contentHeight: height
        flickableDirection: Flickable.HorizontalFlick
        boundsBehavior: Flickable.StopAtBounds
        interactive: contentWidth > width
        clip: true
        Row {
            id: row
            x: tabs.inset
            height: parent.height
            spacing: Theme.s1
            Repeater {
                id: tabRepeater
                model: tabs.model
                T.AbstractButton {
                    id: tab
                    required property var modelData
                    required property int index
                    readonly property var item: typeof modelData === "string" ? { text: modelData } : modelData
                    visible: item.visible !== false
                    enabled: item.enabled !== false
                    height: row.height
                    implicitWidth: label.implicitWidth + 2 * Theme.s2
                    focusPolicy: Qt.StrongFocus
                    hoverEnabled: true
                    Accessible.role: Accessible.PageTab
                    Accessible.name: item.accessibleName || item.text
                    Accessible.selected: tabs.current === index
                    opacity: enabled ? 1 : 0.5
                    onActiveFocusChanged: if (activeFocus) tabs.bringIntoView(tab)
                    onClicked: {
                        if (tabs.current !== index) {
                            tabs.current = index
                            tabs.changed(index)
                        }
                    }
                    contentItem: Row {
                        id: label
                        spacing: 6
                        Icon { visible: !!tab.item.icon; name: tab.item.icon || ""; color: Theme.text2; size: 14; anchors.verticalCenter: parent.verticalCenter }
                        Text {
                            text: tab.item.text
                            color: tabs.current === tab.index || tab.hovered ? Theme.text : Theme.text2
                            font.family: Theme.font
                            font.pixelSize: tabs.small ? Theme.small : Theme.body
                            font.weight: tabs.current === tab.index ? 600 : 500
                            anchors.verticalCenter: parent.verticalCenter
                        }
                    }
                    background: Item {
                        Rectangle {
                            anchors.bottom: parent.bottom
                            width: parent.width
                            height: 2
                            color: tabs.current === tab.index ? Theme.accent : "transparent"
                        }
                        FocusRing { target: tab; inset: 2; radius_: 4 }
                    }
                }
            }
        }
    }
}
