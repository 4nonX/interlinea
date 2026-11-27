import QtQuick
import QtQuick.Layouts
import Interlinea

// Find and replace in the translations of the file; the tags stay where
// they are.
Dialog {
    id: dlg
    property var editor
    title: qsTr("Find and replace")
    readonly property var count: { editor.revision; return editor.replaceCount(findField.text, withField.text, matchCase.checked, wholeWords.checked) }

    function start() {
        if (!editor.segments.length) return
        findField.text = editor.find.trim()
        withField.text = ""
        open()
        findField.forceActiveFocus()
    }

    Field {
        Layout.fillWidth: true
        label: qsTr("Find in the translations")
        TextField { id: findField; width: parent.width; onAccepted: go.clicked() }
    }
    Field {
        Layout.fillWidth: true
        label: qsTr("Replace with")
        TextField { id: withField; width: parent.width; onAccepted: go.clicked() }
    }
    CheckBox { id: matchCase; text: qsTr("Match case") }
    CheckBox { id: wholeWords; text: qsTr("Whole words only") }
    Txt {
        Layout.fillWidth: true
        text: {
            const c = dlg.count
            let said = !findField.text ? qsTr("Nothing to find yet.")
                     : c.places === 0 ? qsTr("Not found in the translations of this file.")
                     : Fmt.fill(qsTr("{places} in {segments}."), { places: Fmt.plural(c.places, qsTr("{n} place"), qsTr("{n} places")), segments: Fmt.plural(c.segments, qsTr("{n} segment"), qsTr("{n} segments")) })
            if (c.locked > 0)
                said += " " + Fmt.plural(c.locked, qsTr("1 locked segment has it too and stays as it is."), qsTr("{n} locked segments have it too and stay as they are."))
            return said
        }
    }
    Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Formatting tags stay where they are. A confirmed segment that changes is a draft again, to be confirmed anew.") }

    buttons: [
        Button { small: true; kind: "quiet"; text: qsTr("Cancel"); onClicked: dlg.close() },
        Button {
            id: go
            small: true; kind: "primary"
            text: qsTr("Replace all")
            enabled: dlg.count.places > 0
            onClicked: if (enabled) { dlg.editor.replaceAll(findField.text, withField.text, matchCase.checked, wholeWords.checked); dlg.close() }
        }
    ]
}
