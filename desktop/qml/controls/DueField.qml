import QtQuick
import QtQuick.Layouts
import Interlinea

// When something is due: "Set a date" until one is set, then the date and
// time (yyyy-mm-dd hh:mm, local time) with a button to take it away.
// iso is the time in UTC, empty for none; edited(iso) when it changes.
RowLayout {
    id: due
    property string iso: ""
    property string name: qsTr("Due")
    signal edited(string iso)
    spacing: Theme.s2

    function pad(n) { return (n < 10 ? "0" : "") + n }
    function shown(value) {
        const d = new Date(value)
        if (!value || isNaN(d.getTime())) return ""
        return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + " " + pad(d.getHours()) + ":" + pad(d.getMinutes())
    }
    function parse(text) {
        const m = /^\s*(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{2}))?\s*$/.exec(text)
        if (!m) return ""
        const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), m[4] ? Number(m[4]) : 17, m[5] ? Number(m[5]) : 0)
        if (isNaN(d.getTime()) || d.getMonth() !== Number(m[2]) - 1) return ""
        return d.toISOString().replace(/\.\d{3}Z$/, "Z")
    }

    Button {
        visible: due.iso === ""
        small: true
        iconName: "clock"
        text: qsTr("Set a date")
        Accessible.name: Fmt.fill(qsTr("{name}: set a date"), { name: due.name })
        onClicked: {
            // A week from now, at the end of the working day.
            const d = new Date()
            d.setDate(d.getDate() + 7)
            d.setHours(17, 0, 0, 0)
            due.edited(d.toISOString().replace(/\.\d{3}Z$/, "Z"))
            field.forceActiveFocus()
        }
    }
    TextField {
        id: field
        visible: due.iso !== ""
        Layout.fillWidth: true
        Layout.maximumWidth: 220
        text: due.shown(due.iso)
        placeholderText: "yyyy-mm-dd hh:mm"
        invalid: text.trim() !== "" && due.parse(text) === ""
        Accessible.name: due.name
        onEditingFinished: {
            const v = due.parse(text)
            if (v && v !== due.iso) due.edited(v)
            else text = Qt.binding(() => due.shown(due.iso))
        }
    }
    IconButton {
        visible: due.iso !== ""
        iconName: "x"
        tip: Fmt.fill(qsTr("{name}: no date"), { name: due.name })
        onClicked: due.edited("")
    }
    Item { Layout.fillWidth: true }
}
