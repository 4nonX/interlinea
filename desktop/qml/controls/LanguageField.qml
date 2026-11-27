import QtQuick
import QtQuick.Templates as T
import Interlinea

// A choice of language: the common ones by name, and any other typed as a
// tag (BCP 47). language is the tag chosen, empty while none fits.
ComboBox {
    id: box
    property string language: ""
    property string placeholder: qsTr("Choose the language")
    editable: true
    model: Backend.commonLanguages()
    textRole: "label"
    valueRole: "code"
    function pick(tag) {
        language = tag
        const i = indexOfValue(tag)
        currentIndex = i
        if (i < 0) editText = tag
    }
    // What shows is the language chosen, or nothing: a combo box would
    // take the first of a new list by itself.
    function settle() {
        if (language) { if (currentValue !== language) pick(language) }
        else if (currentIndex !== -1 || editText !== "") { currentIndex = -1; editText = "" }
    }
    Component.onCompleted: settle()
    onCountChanged: Qt.callLater(settle)
    onModelChanged: Qt.callLater(settle)
    onActivated: i => language = model[i].code
    onAccepted: language = Backend.normalizeLanguage(editText)
    // A name from the list, or a tag typed.
    function typed(text) {
        const t = text.trim()
        const hit = model.find(l => l.label === t)
        language = hit ? hit.code : Backend.normalizeLanguage(t)
    }
    contentItem: T.TextField {
        leftPadding: 0
        text: box.editText
        onTextEdited: box.typed(text)
        color: Theme.text
        font: box.font
        verticalAlignment: Text.AlignVCenter
        selectByMouse: true
        placeholderText: box.placeholder
        placeholderTextColor: Theme.text3
        Text {
            anchors.fill: parent
            verticalAlignment: Text.AlignVCenter
            text: parent.placeholderText
            color: Theme.text3
            font: parent.font
            visible: !parent.length
        }
    }
}
