import QtQuick
import QtQuick.Layouts
import Interlinea

// A project's target languages: those the files suggest and those chosen
// as check boxes, any other from the list. A language picked and not yet
// added is added when the project moves on (pending, addPending()).
ColumnLayout {
    id: targets
    property var form
    readonly property var quick: {
        const out = []
        for (const t of (form.suggested || []).concat(form.targets))
            if (t && t !== form.source && out.indexOf(t) < 0) out.push(t)
        return out
    }
    readonly property string pending: {
        if (form.readOnly) return ""
        const t = adding.language
        return !t || t === form.source || form.targets.indexOf(t) >= 0 ? "" : t
    }
    function addPending() {
        const t = pending
        if (!t) return
        form.addTarget(t)
        adding.pick("")
        adding.editText = ""
    }
    spacing: Theme.s2
    enabled: !form.readOnly
    Layout.fillWidth: true

    Repeater {
        model: targets.quick
        CheckBox {
            required property string modelData
            readonly property bool fixedTarget: targets.form.fixed.indexOf(modelData) >= 0
            text: fixedTarget ? Fmt.fill(qsTr("{language} (a bilingual file is for it)"), { language: Backend.languageName(modelData) }) : Backend.languageName(modelData)
            Accessible.name: Backend.languageName(modelData)
            checked: targets.form.targets.indexOf(modelData) >= 0
            enabled: !fixedTarget
            onToggled: {
                const t = modelData
                const list = targets.form.targets.filter(x => x !== t)
                targets.form.setTargets(checked ? list.concat([t]) : list)
            }
        }
    }
    RowLayout {
        spacing: Theme.s2
        LanguageField {
            id: adding
            implicitWidth: 300
            placeholder: targets.form.targets.length ? qsTr("Add another language…") : qsTr("Choose a language…")
            // The source and the languages already offered are not offered again.
            model: Backend.commonLanguages().filter(l => l.code !== targets.form.source && targets.quick.indexOf(l.code) < 0)
            Accessible.name: qsTr("Add another target language")
            onAccepted: targets.addPending()
        }
        Button {
            small: true
            iconName: "plus"
            text: qsTr("Add")
            Accessible.name: qsTr("Add the target language")
            enabled: targets.pending !== ""
            onClicked: targets.addPending()
        }
    }
    Txt {
        visible: targets.pending !== ""
        Layout.fillWidth: true
        role: "help"
        text: targets.form.isNew ? Fmt.fill(qsTr("{language} is added when you continue, or press Add now."), { language: Backend.languageName(targets.pending) })
                                 : Fmt.fill(qsTr("Press Add to add {language} to the project."), { language: Backend.languageName(targets.pending) })
    }
    Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: targets.form.problemsAt("targetLangs") }
}
