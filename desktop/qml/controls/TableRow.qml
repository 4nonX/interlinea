import QtQuick
import Interlinea

// A row of a Table: each child is a cell, in the column of its place,
// 10 by 12 px in, the row as tall as its tallest cell; a line under it.
Item {
    id: row
    property Item table: {
        for (let p = parent; p; p = p.parent)
            if (p.columns !== undefined && p.widths !== undefined) return p
        return null
    }
    property bool last: false
    default property alias cells: holder.data
    width: parent ? parent.width : 0
    implicitHeight: tallest + 20 + (last ? 0 : 1)
    // The cells: the row's children, a Repeater's in its place, not the
    // Repeater itself.
    function cellItems() {
        const out = []
        for (let i = 0; i < holder.children.length; i++)
            if (!(holder.children[i] instanceof Repeater)) out.push(holder.children[i])
        return out
    }
    property real tallest: {
        let h = 20
        for (let i = 0; i < holder.children.length; i++)
            if (!(holder.children[i] instanceof Repeater)) h = Math.max(h, holder.children[i].implicitHeight)
        return h
    }
    Accessible.role: Accessible.Row

    Rectangle {
        anchors.fill: parent
        color: hover.hovered ? Theme.rowHover : "transparent"
    }
    HoverHandler { id: hover }
    Item {
        id: holder
        anchors.fill: parent
        onChildrenChanged: row.layoutCells()
    }
    Rectangle {
        visible: !row.last
        anchors.bottom: parent.bottom
        width: parent.width
        height: 1
        color: Theme.border
    }
    function layoutCells() {
        if (!table) return
        const cells = cellItems()
        for (let i = 0; i < cells.length; i++) {
            const c = cells[i]
            // A cell made by a Repeater says its column (cell: n).
            const k = c.cell !== undefined ? c.cell : i
            c.x = Qt.binding(() => table.colX(k) - 1 + Theme.s3)
            c.y = 10
            c.width = Qt.binding(() => Math.max(0, table.widths[k] - 2 * Theme.s3))
        }
    }
    onTableChanged: layoutCells()
}
