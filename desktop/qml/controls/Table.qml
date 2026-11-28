import QtQuick
import QtQuick.Layouts
import Interlinea

// The web's .table-wrap .table: a white box with a grey header row and a
// line under each row. columns: [{ title, width } or { title, fill }],
// with align: "right" for numbers. Rows are TableRow items, given as
// children or by a Repeater; their cells line up with the columns.
Rectangle {
    id: table
    property var columns: []
    default property alias rows: body.data
    property string name: ""
    // The widths the columns get: fixed ones as asked, the rest shared by
    // their fill weights. Where the table is too narrow for that (a small
    // window, a large font), the wide fixed columns, those of buttons,
    // give way, down to what one button needs, so that the others keep
    // some room and nothing runs out of the table: their buttons go onto
    // more lines.
    readonly property int keepFilled: 100
    readonly property int wideColumn: 200
    readonly property int narrowestWide: 160
    readonly property var widths: {
        const inner = width - 2
        let fixed = 0, wide = 0, weights = 0, fills = 0
        for (const c of columns) {
            if (c.width) {
                fixed += c.width
                if (c.width >= wideColumn) wide += c.width
            } else {
                weights += (c.fill || 1)
                fills++
            }
        }
        let squeeze = 1
        if (wide > 0 && inner - fixed < fills * keepFilled)
            squeeze = Math.max(0, inner - fills * keepFilled - (fixed - wide)) / wide
        const fixedWidth = c => c.width >= wideColumn && squeeze < 1 ? Math.max(narrowestWide, c.width * squeeze) : c.width
        let taken = 0
        for (const c of columns)
            if (c.width) taken += fixedWidth(c)
        const left = Math.max(0, inner - taken)
        return columns.map(c => c.width ? fixedWidth(c) : left * (c.fill || 1) / weights)
    }
    function colX(i) {
        let s = 1
        for (let k = 0; k < i; k++) s += widths[k]
        return s
    }
    Layout.fillWidth: true
    implicitHeight: head.height + body.implicitHeight + 2
    radius: Theme.rPanel
    color: Theme.surface
    border.color: Theme.border
    clip: true
    Accessible.role: Accessible.Table
    Accessible.name: name

    Rectangle {
        id: head
        x: 1; y: 1
        width: table.width - 2
        height: 40
        radius: Theme.rPanel - 1
        color: Theme.surface2
        // Square at the bottom, where the rows follow.
        Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: parent.radius; color: parent.color }
        Repeater {
            model: table.columns
            Text {
                required property var modelData
                required property int index
                x: table.colX(index) - 1 + Theme.s3
                width: table.widths[index] - 2 * Theme.s3
                height: head.height
                verticalAlignment: Text.AlignVCenter
                horizontalAlignment: modelData.align === "right" ? Text.AlignRight : Text.AlignLeft
                text: modelData.title || ""
                color: Theme.text2
                font.family: Theme.font
                font.pixelSize: Theme.small
                font.weight: 600
                elide: Text.ElideRight
                Accessible.role: Accessible.ColumnHeader
            }
        }
        Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
    }
    Column {
        id: body
        x: 1
        y: head.height + 1
        width: table.width - 2
    }
}
