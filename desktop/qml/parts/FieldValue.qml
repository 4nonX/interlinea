import QtQuick
import QtQuick.Layouts
import Interlinea

// A field's name and the control its kind asks for, as the web's
// FieldValue: a list, yes or no, a date, a number or text. A value that
// does not fit its kind is shown as it is and can be corrected as text.
// saved(value) comes when the value changed and was left.
RowLayout {
    id: row
    property var field: ({})
    property string value: ""
    property bool small: false
    // Given to those who manage the termbase: the field can be removed
    // from it here, from every entry.
    property string termbase: ""
    signal saved(string value)
    signal removeField()
    readonly property string kind: field.kind || "text"
    readonly property bool dateLike: kind === "date" && (value === "" || /^\d{4}-\d{2}-\d{2}/.test(value))
    Layout.fillWidth: true
    spacing: Theme.s3

    function store(v) {
        if (v !== row.value) row.saved(v)
    }

    Txt {
        Layout.preferredWidth: row.small ? 96 : 120
        Layout.alignment: Qt.AlignTop
        Layout.topMargin: row.small ? 4 : 7
        text: row.field.name || ""
        role: "muted"
        font.pixelSize: Theme.small
    }
    // A list, or yes or no.
    ComboBox {
        Layout.fillWidth: true
        visible: row.kind === "picklist" || row.kind === "boolean"
        implicitHeight: row.small ? 28 : 32
        Accessible.name: row.field.name || ""
        readonly property var options: {
            const o = [{ v: "", t: qsTr("Not set") }]
            if (row.kind === "boolean") o.push({ v: "true", t: qsTr("Yes") }, { v: "false", t: qsTr("No") })
            else for (const x of row.field.values || []) o.push({ v: x, t: x })
            if (row.value && !o.some(x => x.v === row.value)) o.push({ v: row.value, t: row.value })
            return o
        }
        model: options.map(o => o.t)
        currentIndex: Math.max(0, options.findIndex(o => o.v === row.value))
        onActivated: i => row.store(options[i].v)
    }
    // A date, yyyy-mm-dd; empty for none.
    TextField {
        Layout.fillWidth: true
        visible: row.dateLike
        implicitHeight: row.small ? 28 : 32
        text: row.value.slice(0, 10)
        placeholderText: qsTr("Not set")
        inputMask: ""
        invalid: text !== "" && !/^\d{4}-\d{2}-\d{2}$/.test(text)
        Accessible.name: row.field.name || ""
        onEditingFinished: if (!invalid) row.store(text.trim())
    }
    // A number.
    ColumnLayout {
        Layout.fillWidth: true
        visible: row.kind === "number"
        spacing: Theme.s1
        TextField {
            id: number
            Layout.fillWidth: true
            implicitHeight: row.small ? 28 : 32
            text: row.value
            placeholderText: qsTr("Not set")
            inputMethodHints: Qt.ImhFormattedNumbersOnly
            Accessible.name: row.field.name || ""
            property bool wrong: false
            invalid: wrong
            onTextEdited: wrong = false
            onEditingFinished: {
                const v = text.trim()
                if (v && !/^[-+]?(\d+([.,]\d*)?|[.,]\d+)$/.test(v)) { wrong = true; return }
                row.store(v)
            }
        }
        Txt { visible: number.wrong; role: "error"; text: qsTr("Type a number, such as 12 or 0.5.") }
    }
    // Text: several lines for an entry's field, one for a term's.
    TextArea {
        Layout.fillWidth: true
        visible: !row.small && !(row.kind === "picklist" || row.kind === "boolean" || row.dateLike || row.kind === "number")
        minimumLines: 2
        text: row.value
        placeholderText: qsTr("Not set")
        Accessible.name: row.field.name || ""
        area.onActiveFocusChanged: if (!area.activeFocus && text !== row.value) row.saved(text.trim())
    }
    TextField {
        Layout.fillWidth: true
        visible: row.small && !(row.kind === "picklist" || row.kind === "boolean" || row.dateLike || row.kind === "number")
        implicitHeight: 28
        text: row.value
        placeholderText: qsTr("Not set")
        Accessible.name: row.field.name || ""
        onEditingFinished: if (text !== row.value) row.saved(text.trim())
    }
    ConfirmButton {
        visible: row.termbase !== ""
        Layout.alignment: Qt.AlignTop
        text: ""
        iconName: "trash"
        action: qsTr("Remove")
        accessibleName: Fmt.fill(qsTr("Remove the field “{name}” from {termbase}"), { name: row.field.name || "", termbase: row.termbase })
        question: Fmt.fill(qsTr("Remove “{name}” from {termbase}? Every entry loses its value."), { name: row.field.name || "", termbase: row.termbase })
        onConfirmed: row.removeField()
    }
}
