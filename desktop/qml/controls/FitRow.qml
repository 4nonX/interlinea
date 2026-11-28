import QtQuick
import QtQuick.Layouts
import Interlinea

// A row of fields and buttons that stays a row while it fits, and puts
// its parts one under another when the window is too narrow for them,
// instead of running out of its panel. A part that only fills the space
// between the others (an Item with Layout.fillWidth and nothing in it)
// counts for nothing; give it visible: row.fits so it leaves no gap when
// the parts stand one under another.
GridLayout {
    id: row
    Layout.fillWidth: true
    // The whole width, whatever its parts would make of it: stacked, they
    // would cap it at the widest one, and it would never see they fit.
    Layout.maximumWidth: Number.POSITIVE_INFINITY
    // And the width it is given, however narrow: side by side, its parts
    // would hold it as wide as they are, and it would never see they do
    // not fit.
    Layout.minimumWidth: 0
    columnSpacing: Theme.s3
    rowSpacing: Theme.s2

    // A part counts as wide as it asks to be (Layout.preferredWidth) or as
    // its content; one that stretches (Layout.fillWidth) as the least it
    // takes (Layout.minimumWidth), a text as narrow as it may be wrapped,
    // and a filler with no least width as nothing.
    function wanted(c) {
        if (c.Layout.fillWidth)
            return Math.max(0, c.Layout.minimumWidth)
        const preferred = c.Layout.preferredWidth
        return preferred > 0 ? preferred : c.implicitWidth
    }
    // The parts side by side: how many, and how wide together.
    readonly property var line: {
        let w = 0, n = 0, all = 0
        for (let i = 0; i < children.length; i++) {
            const c = children[i]
            const own = wanted(c)
            // A filler is a column of its own while the parts fit, and
            // shown only then: its showing is not asked here.
            if (own <= 0) { all++; continue }
            if (!c.visible) continue
            all++
            w += own
            n++
        }
        return { width: w + Math.max(0, n - 1) * columnSpacing, parts: all }
    }
    readonly property bool fits: width >= line.width
    columns: fits ? Math.max(1, line.parts) : 1
}
