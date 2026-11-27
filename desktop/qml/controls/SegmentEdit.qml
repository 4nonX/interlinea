import QtQuick
import QtQuick.Controls.Basic as B
import Interlinea

// A segment's text with its tags as labels, to type in or only to read:
// tags move and delete as one character and can't be broken by typing.
B.ScrollView {
    id: view
    property alias content: doc.content
    property alias readOnly: edit.readOnly
    property alias edit: edit
    property string language: ""
    property bool framed: true
    property int minimumLines: 2
    readonly property string plainText: doc.plainText
    signal edited()
    implicitWidth: 320
    implicitHeight: Math.max(minimumLines * 21 + 2 * Theme.s2, Math.min(edit.implicitHeight, 260))
    clip: true
    Accessible.role: Accessible.EditableText
    Accessible.name: plainText

    TextEdit {
        id: edit
        width: view.availableWidth
        padding: view.framed ? Theme.s2 : 0
        leftPadding: view.framed ? Theme.s2 + 1 : 0
        color: Theme.text
        selectionColor: Theme.selection
        selectedTextColor: Theme.text
        font.family: Theme.font
        font.pixelSize: Theme.body
        wrapMode: TextEdit.Wrap
        textFormat: TextEdit.RichText
        selectByMouse: true
        persistentSelection: true
        horizontalAlignment: Backend.rtl(view.language) ? TextEdit.AlignRight : TextEdit.AlignLeft
        onTextChanged: if (activeFocus && !readOnly) view.edited()
    }
    SegmentDocument {
        id: doc
        document: edit.textDocument
    }

    background: Rectangle {
        visible: view.framed
        radius: Theme.rControl
        color: edit.readOnly ? Theme.surface2 : Theme.surface
        border.width: edit.activeFocus && !edit.readOnly ? 2 : 1
        border.color: edit.activeFocus && !edit.readOnly ? Theme.accent : Theme.borderStrong
    }
}
