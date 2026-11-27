import QtQuick
import QtQuick.Templates as T
import Interlinea

// The translation of the open segment (the web's .target-editor): its
// text with the tags as labels that move and delete as one character,
// words offered to complete the one being typed, the words the spelling
// check found underlined with their suggestions on the context menu, and
// copy and paste that keep tags.
FocusScope {
    id: target
    property string language: ""
    property var completions: []
    // [{ word, suggestions }]
    property var misspellings: []
    property alias placeholder: placeholderText.text
    property alias textEdit: edit
    readonly property alias content: doc.content
    readonly property string plainText: doc.plainText
    readonly property string selectedText: doc.textBetween(edit.selectionStart, edit.selectionEnd)
    property bool setting: false
    signal edited()
    signal addWord(string word)
    implicitHeight: edit.implicitHeight
    activeFocusOnTab: true

    function setContent(c) {
        setting = true
        doc.content = c || []
        setting = false
        edit.cursorPosition = edit.length
    }
    function insertPlain(text) {
        edit.cursorPosition = doc.replace(edit.selectionStart, edit.selectionEnd, text)
        edit.forceActiveFocus()
    }
    function insertTag(tag) {
        const at = edit.selectionStart
        if (edit.selectionEnd > at) doc.replace(at, edit.selectionEnd, "")
        doc.insertTag(at, tag)
        edit.cursorPosition = at + 1
    }

    // Completions: what is offered for the word the caret is at the end of.
    property var offered: []
    property int offeredIndex: 0
    property string prefix: ""
    function closeOffer() { offered = []; prefix = "" }
    function suggest() {
        const at = edit.cursorPosition
        if (edit.readOnly || !completions.length || edit.selectionStart !== edit.selectionEnd) { closeOffer(); return }
        const before = doc.textBetween(Math.max(0, at - 80), at)
        const m = Backend.matchAll("[\\p{L}\\p{N}][\\p{L}\\p{N}\\p{M}'\u2019-]*$", before)
        const after = doc.textBetween(at, at + 1)
        if (!m.length || Backend.matchAll("^[\\p{L}\\p{N}\\p{M}]", after).length) { closeOffer(); return }
        const p = m[0].toLowerCase()
        if ([...p].length < 2) { closeOffer(); return }
        const seen = {}, out = []
        for (const c of completions) {
            const k = c.toLowerCase()
            if (k === p || !k.startsWith(p) || seen[k]) continue
            seen[k] = true
            out.push(c)
        }
        out.sort((a, b) => a.length !== b.length ? a.length - b.length : a.localeCompare(b))
        if (!out.length) { closeOffer(); return }
        prefix = m[0]
        offered = out.slice(0, 8)
        offeredIndex = 0
        // Over the page, under the caret, or over it near the window's foot.
        const r = edit.cursorRectangle
        const below = edit.mapToItem(offer.parent, r.x, r.y + r.height + 4)
        const above = edit.mapToItem(offer.parent, r.x, r.y - 4)
        const h = offered.length * 26 + 8
        offer.x = Math.max(4, Math.min(below.x, offer.parent.width - offer.width - 4))
        offer.y = below.y + h > offer.parent.height - 4 ? above.y - h : below.y
    }
    // The rest of the chosen word goes in after what was typed, which stays as typed.
    function accept(word) {
        const rest = [...word].slice([...prefix].length).join("")
        closeOffer()
        insertPlain(rest)
    }

    TextEdit {
        id: edit
        width: target.width
        focus: true
        color: Theme.text
        selectionColor: Theme.selection
        selectedTextColor: Theme.text
        font.family: Theme.font
        font.pixelSize: 15
        wrapMode: TextEdit.Wrap
        textFormat: TextEdit.RichText
        selectByMouse: true
        persistentSelection: true
        horizontalAlignment: Backend.rtl(target.language) ? TextEdit.AlignRight : TextEdit.AlignLeft
        Accessible.role: Accessible.EditableText
        Accessible.name: target.Accessible.name
        Accessible.description: target.Accessible.description

        Keys.onPressed: event => {
            if (target.offered.length && !(event.modifiers & (Qt.ControlModifier | Qt.AltModifier | Qt.MetaModifier))) {
                const n = target.offered.length
                if (event.key === Qt.Key_Down) { target.offeredIndex = (target.offeredIndex + 1) % n; event.accepted = true; return }
                if (event.key === Qt.Key_Up) { target.offeredIndex = (target.offeredIndex + n - 1) % n; event.accepted = true; return }
                if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && !(event.modifiers & Qt.ShiftModifier) || event.key === Qt.Key_Tab) {
                    target.accept(target.offered[target.offeredIndex]); event.accepted = true; return
                }
                if (event.key === Qt.Key_Escape) { target.closeOffer(); event.accepted = true; return }
                if ([Qt.Key_Left, Qt.Key_Right, Qt.Key_Home, Qt.Key_End].indexOf(event.key) >= 0) target.closeOffer()
            }
            // Copy, cut and paste keep the tags.
            if (event.matches(StandardKey.Copy)) { if (edit.selectionStart !== edit.selectionEnd) doc.copy(edit.selectionStart, edit.selectionEnd); event.accepted = true; return }
            if (event.matches(StandardKey.Cut)) {
                if (!edit.readOnly && edit.selectionStart !== edit.selectionEnd) { doc.copy(edit.selectionStart, edit.selectionEnd); edit.cursorPosition = doc.replace(edit.selectionStart, edit.selectionEnd, "") }
                event.accepted = true; return
            }
            if (event.matches(StandardKey.Paste)) { if (!edit.readOnly) edit.cursorPosition = doc.paste(edit.selectionStart, edit.selectionEnd); event.accepted = true; return }
            // A line break is Shift+Enter; Enter alone is the page's.
            if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && !(event.modifiers & Qt.ShiftModifier)) event.accepted = false
        }
        onCursorPositionChanged: if (!activeFocus) target.closeOffer()
        onActiveFocusChanged: if (!activeFocus) target.closeOffer()

        Text {
            id: placeholderText
            visible: edit.length === 0 && !edit.preeditText
            anchors.left: parent.left
            anchors.right: parent.right
            horizontalAlignment: edit.horizontalAlignment
            color: Theme.text3
            font: edit.font
            text: qsTr("Type the translation")
        }
        TapHandler {
            acceptedButtons: Qt.RightButton
            onTapped: (point) => {
                const pos = edit.positionAt(point.position.x, point.position.y)
                const w = doc.wordAt(pos)
                const hit = target.misspellings.find(m => m.word === w.word)
                menu.word = hit ? w : null
                menu.suggestions = hit ? (hit.suggestions || []).slice(0, 6) : []
                menu.popup()
            }
        }
    }
    SegmentDocument {
        id: doc
        document: edit.textDocument
        misspelled: target.misspellings.map(m => m.word)
        onContentChanged: {
            if (target.setting) return
            target.edited()
            if (edit.activeFocus) target.suggest()
            else target.closeOffer()
        }
    }

    // The words offered, under the caret, never taking the keyboard.
    PopupSurface {
        id: offer
        parent: T.Overlay.overlay
        visible: target.offered.length > 0 && target.visible
        z: 100
        width: Math.min(360, Math.max(160, offerList.implicitWidth + 16))
        height: offerList.implicitHeight + 8
        Accessible.role: Accessible.List
        Accessible.name: qsTr("Completions")
        Column {
            id: offerList
            x: 4; y: 4
            width: offer.width - 8
            Repeater {
                model: target.offered
                Rectangle {
                    required property string modelData
                    required property int index
                    width: offerList.width
                    implicitWidth: word.implicitWidth + 16
                    height: 26
                    radius: 4
                    color: index === target.offeredIndex ? Theme.accentWeak : "transparent"
                    Accessible.role: Accessible.ListItem
                    Accessible.name: modelData
                    Text { id: word; x: 8; anchors.verticalCenter: parent.verticalCenter; text: modelData; color: Theme.text; font.family: Theme.font; font.pixelSize: Theme.body }
                    TapHandler { onTapped: { target.accept(modelData); edit.forceActiveFocus() } }
                }
            }
        }
    }

    Menu {
        id: menu
        property var word: null
        property var suggestions: []
        Repeater {
            model: menu.word ? menu.suggestions : []
            MenuItem {
                required property string modelData
                text: modelData
                onTriggered: { edit.cursorPosition = doc.replace(menu.word.start, menu.word.end, modelData) }
            }
        }
        MenuItem { visible: !!menu.word && menu.suggestions.length === 0; height: visible ? implicitHeight : 0; enabled: false; text: qsTr("No suggestions") }
        MenuItem {
            visible: !!menu.word
            height: visible ? implicitHeight : 0
            text: menu.word ? Fmt.fill(qsTr("Add “{word}” to the project's words"), { word: menu.word.word }) : ""
            onTriggered: target.addWord(menu.word.word)
        }
        MenuSeparator { visible: !!menu.word; height: visible ? implicitHeight : 0 }
        MenuItem { text: qsTr("Cut"); enabled: !edit.readOnly && edit.selectionStart !== edit.selectionEnd; onTriggered: { doc.copy(edit.selectionStart, edit.selectionEnd); edit.cursorPosition = doc.replace(edit.selectionStart, edit.selectionEnd, "") } }
        MenuItem { text: qsTr("Copy"); enabled: edit.selectionStart !== edit.selectionEnd; onTriggered: doc.copy(edit.selectionStart, edit.selectionEnd) }
        MenuItem { text: qsTr("Paste"); enabled: !edit.readOnly; onTriggered: edit.cursorPosition = doc.paste(edit.selectionStart, edit.selectionEnd) }
        MenuItem { text: qsTr("Select all"); onTriggered: edit.selectAll() }
    }
}
