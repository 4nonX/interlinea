import QtQuick
import QtQuick.Templates as T
import Interlinea

// A menu on the popup surface: its entries 30 px high, the one under the
// pointer in the accent's light tint, keys at the right.
T.Menu {
    id: control
    implicitWidth: Math.max(220, contentItem.implicitWidth + leftPadding + rightPadding)
    implicitHeight: contentItem.implicitHeight + topPadding + bottomPadding
    padding: 4
    margins: 0
    overlap: 1
    font.family: Theme.font
    font.pixelSize: Theme.body
    delegate: MenuItem {}
    contentItem: ListView {
        implicitHeight: contentHeight
        implicitWidth: {
            let w = 0
            for (let i = 0; i < count; i++) {
                const it = control.itemAt(i)
                if (it) w = Math.max(w, it.implicitWidth)
            }
            return w
        }
        model: control.contentModel
        interactive: Window.window ? contentHeight + control.topPadding + control.bottomPadding > Window.window.height : false
        clip: true
        currentIndex: control.currentIndex
    }
    background: PopupSurface { implicitWidth: 220 }
}
