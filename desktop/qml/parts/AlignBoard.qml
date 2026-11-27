import QtQuick
import QtQuick.Layouts
import Interlinea

// The two texts of an alignment side by side, with the lines between them:
// each segment in a box chosen with the mouse or the keyboard, the point
// beside it where its lines begin, and the handle on the outer side to move
// it, with the segments marked with it, to another place in the order its
// text is read in.
Item {
    id: board
    property var dialog
    property var view
    readonly property var d: dialog
    // Where a dragged run would go: a line across that side, -1 while none.
    property int dropSide: -1
    property real dropY: 0
    property int dropBefore: -2
    property var dragItems: []
    property int dragSide: -1
    property int hoverLine: -1
    property var hoverPort: null      // [side, index]
    property var pulling: null        // { side, index, x, y }
    implicitHeight: Math.max(cols.implicitHeight + 2 * Theme.s3, 60)

    // The box of segment index of a side, on the board.
    function item(side, index) {
        const k = d.order[side].indexOf(index)
        return k < 0 ? null : (side === 0 ? sourceRep : targetRep).itemAt(k)
    }
    function box(side, index) {
        const it = item(side, index)
        if (!it) return null
        const p = it.seg.mapToItem(board, 0, 0)
        return Qt.rect(p.x, p.y, it.seg.width, it.seg.height)
    }
    function grip(side, index) { const it = item(side, index); return it ? it.handle : null }
    function port(side, index) {
        const b = box(side, index)
        if (!b) return null
        return side === 0 ? Qt.point(b.x + b.width + d.portOut, b.y + b.height / 2) : Qt.point(b.x - d.portOut, b.y + b.height / 2)
    }
    function portAt(x, y) {
        for (let s = 0; s < 2; s++)
            for (let i = 0; i < d.segs[s].length; i++) {
                const c = port(s, i)
                if (c && Math.hypot(c.x - x, c.y - y) <= d.portRadius + 3) return [s, i]
            }
        return null
    }
    function segmentAt(x, y) {
        const p = portAt(x, y)
        if (p) return p
        for (let s = 0; s < 2; s++)
            for (let i = 0; i < d.segs[s].length; i++) {
                const b = box(s, i)
                if (b && x >= b.x && x <= b.x + b.width && y >= b.y && y <= b.y + b.height) return [s, i]
            }
        return null
    }
    // Where a line starts and ends: it is drawn as a curve that leaves and
    // arrives level.
    function ends(l) {
        const a = box(0, l[0]), b = box(1, l[1])
        if (!a || !b) return null
        return { x1: a.x + a.width, y1: a.y + a.height / 2, x2: b.x, y2: b.y + b.height / 2 }
    }
    function curvePoint(e, t) {
        const mid = (e.x1 + e.x2) / 2, u = 1 - t
        return { x: u * u * u * e.x1 + 3 * u * u * t * mid + 3 * u * t * t * mid + t * t * t * e.x2,
                 y: u * u * u * e.y1 + 3 * u * u * t * e.y1 + 3 * u * t * t * e.y2 + t * t * t * e.y2 }
    }
    // Of the lines near the place, the one whose curve is nearest.
    function lineAt(x, y) {
        let best = -1, bestDistance = d.near + 0.001
        for (let i = 0; i < d.links.length; i++) {
            const e = ends(d.links[i])
            if (!e || x < e.x1 - d.near || x > e.x2 + d.near) continue
            for (let k = 0; k <= 40; k++) {
                const p = curvePoint(e, k / 40)
                const dist = Math.hypot(p.x - x, p.y - y)
                if (dist < bestDistance) { bestDistance = dist; best = i }
            }
        }
        return best
    }
    function repaint() { lines.requestPaint() }

    // A dragged run: before the first segment, not in it, whose middle is below the pointer.
    function dragTo(side, y) {
        if (y - view.contentY < 36) view.contentY = Math.max(0, view.contentY - 14)
        else if (y - view.contentY > view.height - 36) view.contentY = Math.min(view.contentHeight - view.height, view.contentY + 14)
        dropBefore = -1
        let lastBottom = 0
        for (const i of d.order[side]) {
            if (dragItems.indexOf(i) >= 0) continue
            const b = box(side, i)
            if (y < b.y + b.height / 2) { dropBefore = i; dropY = b.y - Theme.s2 / 2; break }
            lastBottom = b.y + b.height
        }
        if (dropBefore === -1) dropY = lastBottom + Theme.s2 / 2
        dropSide = side
    }
    function dropAt(side, y) {
        dragTo(side, y)
        const items = dragItems, before = dropBefore
        dragSide = -1; dragItems = []; dropBefore = -2; dropSide = -1
        d.move(side, items, before)
    }

    Connections {
        target: board.dialog
        function onRevChanged() { lines.requestPaint() }
    }
    onWidthChanged: lines.requestPaint()
    onHeightChanged: lines.requestPaint()

    // The lines and the points, under the boxes' text but over the gap.
    Canvas {
        id: lines
        anchors.fill: parent
        z: 1
        onPaint: {
            const ctx = getContext("2d")
            ctx.reset()
            const dd = board.d
            for (let i = 0; i < dd.links.length; i++) {
                const l = dd.links[i], e = board.ends(l)
                if (!e) continue
                const own = dd.has(dd.mine, l), look = dd.has(dd.shown.lookLines, l)
                ctx.beginPath()
                ctx.lineCap = "round"
                if (i === board.hoverLine) { ctx.strokeStyle = Theme.danger; ctx.lineWidth = 2.5; ctx.setLineDash([]) }
                else if (own) { ctx.strokeStyle = Theme.text2; ctx.lineWidth = 1.5; ctx.setLineDash([]) }
                else { ctx.strokeStyle = look ? Theme.warn : Theme.text2; ctx.lineWidth = look ? 2.5 : 2; ctx.setLineDash([0.05, 5 / ctx.lineWidth]) }
                const mid = (e.x1 + e.x2) / 2
                ctx.moveTo(e.x1, e.y1)
                ctx.bezierCurveTo(mid, e.y1, mid, e.y2, e.x2, e.y2)
                ctx.stroke()
            }
            ctx.setLineDash([])
            // A line being pulled from a point.
            if (board.pulling) {
                const c = board.port(board.pulling.side, board.pulling.index)
                if (c) {
                    ctx.beginPath()
                    ctx.strokeStyle = Theme.accent
                    ctx.lineWidth = 2
                    ctx.moveTo(c.x, c.y)
                    ctx.lineTo(board.pulling.x, board.pulling.y)
                    ctx.stroke()
                }
            }
            for (let s = 0; s < 2; s++)
                for (let i = 0; i < dd.segs[s].length; i++) {
                    const c = board.port(s, i)
                    if (!c) continue
                    const it = board.item(s, i)
                    const lit = (dd.pickedSide === s && dd.picked === i) || (board.hoverPort && board.hoverPort[0] === s && board.hoverPort[1] === i) || (it && it.hovered)
                    const alone = dd.alone(s, i)
                    const ink = lit ? Theme.accent : alone ? Theme.warn : Theme.text2
                    ctx.beginPath()
                    ctx.lineWidth = 2
                    ctx.strokeStyle = ink
                    ctx.fillStyle = alone && !lit ? Theme.surface : ink
                    ctx.arc(c.x, c.y, dd.portRadius - 1, 0, 2 * Math.PI)
                    ctx.fill()
                    ctx.stroke()
                }
            if (board.dropSide >= 0) {
                let left = board.width, right = 0
                for (let i = 0; i < dd.segs[board.dropSide].length; i++) {
                    const it = board.item(board.dropSide, i)
                    if (!it) continue
                    const p = it.mapToItem(board, 0, 0)
                    left = Math.min(left, p.x); right = Math.max(right, p.x + it.width)
                }
                ctx.fillStyle = Theme.accent
                ctx.fillRect(left, board.dropY - 1.5, right - left, 3)
            }
        }
    }

    // Lines and points take the mouse in the gap between the texts.
    MouseArea {
        // Over the gap only, with the points beside the boxes.
        readonly property real gapLeft: cols.x + (cols.width - board.d.gutter) / 2
        x: gapLeft - 12
        width: board.d.gutter + 24
        height: parent.height
        z: 2
        hoverEnabled: true
        acceptedButtons: Qt.LeftButton
        propagateComposedEvents: true
        property var pressedPort: null
        cursorShape: board.hoverPort ? Qt.CrossCursor : board.hoverLine >= 0 ? Qt.PointingHandCursor : Qt.ArrowCursor
        function at(mouse) { return Qt.point(mouse.x + x, mouse.y) }
        onPositionChanged: mouse => {
            const m = at(mouse)
            if (pressedPort) {
                board.pulling = { side: pressedPort[0], index: pressedPort[1], x: m.x, y: m.y }
                lines.requestPaint()
                return
            }
            const p = board.portAt(m.x, m.y)
            const i = p ? -1 : board.lineAt(m.x, m.y)
            if (JSON.stringify(p) !== JSON.stringify(board.hoverPort) || i !== board.hoverLine) {
                board.hoverPort = p
                board.hoverLine = i
                lines.requestPaint()
            }
        }
        onExited: { board.hoverPort = null; board.hoverLine = -1; lines.requestPaint() }
        onPressed: mouse => {
            // A point comes before the line that ends in it.
            const m = at(mouse)
            const p = board.portAt(m.x, m.y)
            if (p) { pressedPort = p; mouse.accepted = true; return }
            const i = board.lineAt(m.x, m.y)
            if (i >= 0) { mouse.accepted = true; const l = board.d.links[i]; board.hoverLine = -1; board.d.takeLine(l[0], l[1]); return }
            mouse.accepted = false
        }
        onReleased: mouse => {
            const from = pressedPort
            pressedPort = null
            board.pulling = null
            lines.requestPaint()
            if (!from) return
            const m = at(mouse)
            const to = board.segmentAt(m.x, m.y)
            if (to && to[0] === from[0] && to[1] === from[1]) board.d.choose(from[0], from[1])
            else if (to && to[0] !== from[0]) board.d.pull(from[0], from[1], to[1])
        }
        ToolTip {
            visible: !!board.hoverPort || board.hoverLine >= 0
            text: board.hoverPort ? qsTr("Pull from here to the segment that belongs to it") : qsTr("Take this line away")
            delay: 500
        }
    }

    RowLayout {
        id: cols
        x: Theme.s3; y: Theme.s3
        width: board.width - 2 * Theme.s3
        spacing: 0
        Repeater {
            model: 2
            delegate: ColumnLayout {
                id: col
                required property int index
                readonly property int side: index
                // Each text half of what the gap between them leaves.
                Layout.preferredWidth: (cols.width - board.d.gutter) / 2
                Layout.maximumWidth: (cols.width - board.d.gutter) / 2
                Layout.alignment: Qt.AlignTop
                Layout.rightMargin: side === 0 ? board.d.gutter : 0
                spacing: Theme.s2
                Repeater {
                    id: rep
                    model: board.d.order[col.side]
                    Component.onCompleted: { if (col.side === 0) board.sourceRepRef = rep; else board.targetRepRef = rep }
                    delegate: RowLayout {
                        id: entry
                        required property int modelData
                        required property int index
                        readonly property int i: modelData
                        readonly property int k: index
                        readonly property alias seg: segBox
                        readonly property alias handle: gripItem
                        readonly property bool hovered: segHover.hovered
                        readonly property bool picked: { board.d.rev; return board.d.pickedSide === col.side && board.d.picked === i }
                        readonly property bool alone: { board.d.rev; return board.d.alone(col.side, i) }
                        readonly property bool marked: { board.d.rev; return board.d.runSide === col.side && board.d.run.indexOf(i) >= 0 }
                        Layout.fillWidth: true
                        spacing: 4
                        layoutDirection: col.side === 0 ? Qt.LeftToRight : Qt.RightToLeft
                        onHoveredChanged: lines.requestPaint()
                        onYChanged: lines.requestPaint()
                        onHeightChanged: lines.requestPaint()

                        // The handle: drag to move; Shift+click marks several; Alt+Up and Alt+Down move.
                        Rectangle {
                            id: gripItem
                            Layout.preferredWidth: 20
                            Layout.fillHeight: true
                            radius: 4
                            activeFocusOnTab: true
                            readonly property bool lit: entry.marked || activeFocus || gripHover.hovered
                            color: entry.marked ? Theme.accentWeak : lit ? Theme.surface : "transparent"
                            border.color: activeFocus ? Theme.accent : "transparent"
                            border.width: activeFocus ? 2 : 0
                            Accessible.role: Accessible.Button
                            Accessible.name: col.side === 0 ? Fmt.fill(qsTr("Move sentence {n}"), { n: entry.i + 1 }) : Fmt.fill(qsTr("Move translation {n}"), { n: entry.i + 1 })
                            Accessible.description: [entry.marked ? qsTr("Marked to move") : "", entry.i !== entry.k ? Fmt.fill(qsTr("Place {k} of {n}"), { k: entry.k + 1, n: board.d.order[col.side].length }) : ""].filter(x => x).join(". ")
                            Grid {
                                anchors.centerIn: parent
                                columns: 2
                                spacing: 2.4
                                Repeater {
                                    model: 6
                                    Rectangle { width: 2.6; height: 2.6; radius: 1.3; color: entry.marked ? Theme.accentText : gripItem.lit ? Theme.text : Theme.text3 }
                                }
                            }
                            HoverHandler { id: gripHover; cursorShape: gripDrag.active ? Qt.ClosedHandCursor : Qt.OpenHandCursor }
                            ToolTip { visible: gripHover.hovered && !gripDrag.active; text: qsTr("Drag to move. Shift+click marks several. Alt+Up and Alt+Down move."); delay: 600 }
                            TapHandler {
                                onTapped: (point, button) => { gripItem.forceActiveFocus(); board.d.mark(col.side, entry.i, !!(point.modifiers & Qt.ShiftModifier)) }
                            }
                            DragHandler {
                                id: gripDrag
                                target: null
                                yAxis.enabled: true
                                xAxis.enabled: false
                                dragThreshold: 4
                                onActiveChanged: {
                                    const y = gripItem.mapToItem(board, 0, centroid.position.y).y
                                    if (active) {
                                        gripItem.forceActiveFocus()
                                        board.dragSide = col.side
                                        board.dragItems = board.d.movingFrom(col.side, entry.i)
                                        board.dragTo(col.side, y)
                                    } else if (board.dragSide === col.side) {
                                        board.dropAt(col.side, y)
                                    }
                                    lines.requestPaint()
                                }
                                onCentroidChanged: if (active) { board.dragTo(col.side, gripItem.mapToItem(board, 0, centroid.position.y).y); lines.requestPaint() }
                            }
                            Keys.onPressed: event => {
                                const by = event.key === Qt.Key_Up ? -1 : event.key === Qt.Key_Down ? 1 : 0
                                if (by && (event.modifiers & Qt.AltModifier)) { event.accepted = true; board.d.nudge(col.side, entry.i, by); return }
                                if (by && (event.modifiers & Qt.ShiftModifier)) { event.accepted = true; board.d.extendRun(col.side, entry.k, by); return }
                                if (event.key === Qt.Key_Space || event.key === Qt.Key_Return || event.key === Qt.Key_Enter) {
                                    event.accepted = true
                                    board.d.mark(col.side, entry.i, !!(event.modifiers & Qt.ShiftModifier))
                                }
                            }
                        }
                        // The segment: its number and its text.
                        Rectangle {
                            id: segBox
                            Layout.fillWidth: true
                            implicitHeight: segRow.implicitHeight + 2 * Theme.s2
                            radius: Theme.rControl
                            activeFocusOnTab: true
                            color: entry.picked ? Theme.accentWeak : segHover.hovered ? Theme.rowHover : Theme.surface
                            border.color: entry.picked || activeFocus ? Theme.accent : entry.alone ? Theme.warn : Theme.border
                            border.width: entry.picked || activeFocus ? 2 : 1
                            Accessible.role: Accessible.Button
                            Accessible.name: Fmt.fill(qsTr("{what} {n}{alone}: {text}"), { what: col.side === 0 ? qsTr("Sentence") : qsTr("Translation"), n: entry.i + 1,
                                                                                        alone: entry.alone ? qsTr(", no pair") : "", text: Fmt.plain(board.d.segs[col.side][entry.i]) })
                            Accessible.description: entry.picked ? qsTr("Chosen") : ""
                            RowLayout {
                                id: segRow
                                x: Theme.s3; y: Theme.s2
                                width: parent.width - 2 * Theme.s3
                                spacing: Theme.s2
                                Txt { Layout.alignment: Qt.AlignTop; Layout.minimumWidth: 24; role: "sub"; text: entry.i + 1 }
                                Txt { Layout.fillWidth: true; text: Fmt.plain(board.d.segs[col.side][entry.i]) }
                            }
                            HoverHandler { id: segHover; cursorShape: Qt.PointingHandCursor }
                            // A click chooses it; let go over a segment of the other text, a line is pulled to it.
                            DragHandler {
                                id: segDrag
                                target: null
                                dragThreshold: 6
                                onActiveChanged: {
                                    const p = segBox.mapToItem(board, centroid.position.x, centroid.position.y)
                                    if (active) {
                                        board.pulling = { side: col.side, index: entry.i, x: p.x, y: p.y }
                                    } else {
                                        board.pulling = null
                                        const to = board.segmentAt(p.x, p.y)
                                        if (to && to[0] !== col.side) board.d.pull(col.side, entry.i, to[1])
                                    }
                                    lines.requestPaint()
                                }
                                onCentroidChanged: if (active) {
                                    const p = segBox.mapToItem(board, centroid.position.x, centroid.position.y)
                                    board.pulling = { side: col.side, index: entry.i, x: p.x, y: p.y }
                                    lines.requestPaint()
                                }
                            }
                            TapHandler { onTapped: { segBox.forceActiveFocus(); board.d.choose(col.side, entry.i) } }
                            Keys.onPressed: event => {
                                if (event.key === Qt.Key_Space || event.key === Qt.Key_Return || event.key === Qt.Key_Enter) { event.accepted = true; board.d.choose(col.side, entry.i) }
                            }
                        }
                    }
                }
                Item { Layout.fillHeight: true }
            }
        }
    }
    property var sourceRepRef: null
    property var targetRepRef: null
    readonly property var sourceRep: sourceRepRef
    readonly property var targetRep: targetRepRef
}
