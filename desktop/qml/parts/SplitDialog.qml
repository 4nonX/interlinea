import QtQuick
import QtQuick.Layouts
import Interlinea

// Splitting the open segment: the source with a caret nothing can be
// typed at; where the caret is, the second part begins.
Dialog {
    id: dlg
    property var editor
    property var first: []
    property var second: []
    property bool working: false
    readonly property bool ok: Backend.matchAll("[\\p{L}\\p{N}]", Fmt.plain(first)).length > 0 && Backend.matchAll("[\\p{L}\\p{N}]", Fmt.plain(second)).length > 0
    title: qsTr("Split the segment")

    function start() {
        const s = editor.activeSegment
        if (!s) return
        error = ""
        working = false
        doc.content = s.source || []
        open()
        edit.cursorPosition = 0
        divide()
        edit.forceActiveFocus()
    }
    function trimmed(c, end) {
        if (!c.length) return c
        const out = c.slice()
        const i = end ? out.length - 1 : 0
        if (out[i].text === undefined) return out
        out[i] = { text: end ? out[i].text.replace(/\s+$/u, "") : out[i].text.replace(/^\s+/u, "") }
        return out
    }
    // Puts the caret at a place of the source, as a click there does.
    function placeAt(position) { edit.cursorPosition = Math.max(0, Math.min(Number(position), edit.length)); edit.forceActiveFocus() }
    function split() {
        if (!ok || working) return
        working = true
        error = ""
        editor.resegment("split", first, why => { error = why; working = false })
    }
    function divide() {
        first = doc.contentBetween(0, edit.cursorPosition)
        second = doc.contentBetween(edit.cursorPosition, edit.length)
    }

    Txt {
        id: how
        Layout.fillWidth: true
        role: "help"
        text: qsTr("Click where the second part begins, or go there with the arrow keys. The translation stays with the first part.")
              + (((dlg.editor && dlg.editor.activeSegment || {}).joined || []).length ? " " + qsTr("Split where two joined segments met, they stand apart again.") : "")
    }
    Rectangle {
        Layout.fillWidth: true
        implicitHeight: edit.implicitHeight + 2 * Theme.s2
        radius: Theme.rControl
        color: Theme.surface
        border.color: edit.activeFocus ? Theme.accent : Theme.borderStrong
        border.width: edit.activeFocus ? 2 : 1
        TextEdit {
            id: edit
            x: Theme.s3; y: Theme.s2
            width: parent.width - 2 * Theme.s3
            readOnly: true
            cursorVisible: true
            selectByMouse: true
            selectByKeyboard: true
            objectName: "splitSource"
            persistentSelection: false
            wrapMode: TextEdit.Wrap
            textFormat: TextEdit.RichText
            color: Theme.text
            font.family: Theme.font
            font.pixelSize: Theme.body
            Accessible.name: qsTr("Source of the segment")
            Accessible.description: how.text
            onCursorPositionChanged: dlg.divide()
        }
        SegmentDocument { id: doc; document: edit.textDocument }
    }
    GridLayout {
        Layout.fillWidth: true
        columns: 2
        columnSpacing: Theme.s3
        rowSpacing: 6
        Txt { role: "label"; text: qsTr("First part"); Layout.alignment: Qt.AlignTop }
        Text {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            textFormat: Text.RichText
            font.family: Theme.font
            font.pixelSize: Theme.body
            color: Theme.text
            text: dlg.ok ? Backend.segmentHtml(dlg.trimmed(dlg.first, true)) : "<span style=\"color:" + Theme.text3 + "\">" + qsTr("Place the cursor inside the text.") + "</span>"
        }
        Txt { role: "label"; text: qsTr("Second part"); Layout.alignment: Qt.AlignTop }
        Text {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            textFormat: Text.RichText
            font.family: Theme.font
            font.pixelSize: Theme.body
            color: Theme.text
            text: dlg.ok ? Backend.segmentHtml(dlg.trimmed(dlg.second, false)) : "<span style=\"color:" + Theme.text3 + "\">" + qsTr("Both parts need text.") + "</span>"
        }
    }

    buttons: [
        Button { small: true; kind: "quiet"; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button {
            small: true; kind: "primary"
            text: qsTr("Split")
            enabled: dlg.ok && !dlg.working
            onClicked: dlg.split()
        }
    ]
}
