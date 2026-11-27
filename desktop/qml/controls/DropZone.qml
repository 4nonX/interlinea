import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .dropzone: a dashed box that takes dropped files and folders,
// blue while something is dragged over it. Its parts are its children.
Item {
    id: zone
    default property alias content: row.data
    signal dropped(var urls)
    Layout.fillWidth: true
    implicitHeight: row.implicitHeight + 2 * Theme.s5
    implicitWidth: row.implicitWidth + 2 * Theme.s5
    Accessible.role: Accessible.Grouping

    Canvas {
        id: frame
        anchors.fill: parent
        property color line: drop.containsDrag ? Theme.accent : Theme.borderStrong
        property color fill: drop.containsDrag ? Theme.accentWeak : Theme.surface
        onLineChanged: requestPaint()
        onFillChanged: requestPaint()
        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d")
            ctx.reset()
            const r = Theme.rPanel, w = width - 1.5, h = height - 1.5
            ctx.beginPath()
            ctx.roundedRect(0.75, 0.75, w, h, r, r)
            ctx.fillStyle = fill
            ctx.fill()
            ctx.setLineDash([3, 2])
            ctx.lineWidth = 1.5
            ctx.strokeStyle = line
            ctx.stroke()
        }
    }
    RowLayout {
        id: row
        x: Theme.s5
        y: Theme.s5
        width: zone.width - 2 * Theme.s5
        spacing: Theme.s4
    }
    DropArea {
        id: drop
        anchors.fill: parent
        keys: ["text/uri-list"]
        onDropped: d => {
            if (d.hasUrls) {
                zone.dropped(d.urls)
                d.acceptProposedAction()
            }
        }
    }
}
