import QtQuick
import QtQuick.Layouts
import Interlinea

// The match analysis of the editor's language: what was counted against
// what, and each band's words and segments.
Dialog {
    id: dlg
    property var editor
    property var report: null
    property string text: ""
    wide: true
    title: qsTr("Analysis")

    function start() {
        report = null
        text = qsTr("Analysing…")
        open()
        const lang = editor.targetLang, settings = editor.settings
        Api.post(editor.cid, editor.base + "/analysis", { lang: lang }, r => {
            if (!r.ok) { text = r.error.message; return }
            const rep = r.data.find(x => x.targetLang === lang) || {}
            const memories = rep.memories || []
            const files = (rep.files || []).length
            const projectMemory = (settings.name || "") + " (" + lang + ")"
            const said = { files: Fmt.plural(files, qsTr("{n} file"), qsTr("{n} files")), language: Backend.languageName(lang), memories: Fmt.listOf(memories) }
            let t = memories.length
                ? Fmt.fill(qsTr("{files} in {language} against {memories}, counted in this order: context matches first, then repetitions within a file and across files, then the best match. Below 50% counts as new."), said)
                : Fmt.fill(qsTr("{files} in {language}, against no memory: only repetitions count. Below 50% counts as new."), said)
            if (settings.projectMemory && memories.indexOf(projectMemory) < 0)
                t += " " + qsTr("The project memory is left out: it holds this project's own translations.")
            text = t
            report = rep
        })
    }

    Txt { Layout.fillWidth: true; role: "muted"; text: dlg.text }
    AnalysisTable { visible: !!dlg.report; Layout.fillWidth: true; report: dlg.report || {} }

    buttons: [ Button { small: true; kind: "primary"; text: qsTr("Close"); onClicked: dlg.close() } ]
}
