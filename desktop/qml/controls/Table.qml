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
    // their fill weights.
    readonly property var widths: {
        const inner = width - 2
        let fixed = 0, weights = 0
        for (const c of columns) {
            if (c.width) fixed += c.width
            else weights += (c.fill || 1)
        }
        const left = Math.max(0, inner - fixed)
        return columns.map(c => c.width ? c.width : left * (c.fill || 1) / weights)
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
