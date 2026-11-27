import QtQuick
import Interlinea

// An entry's terms in every language, each language on a line of its own
// and every term of it alike: a concept has no source language, and a
// synonym is as much a term as the first. Forbidden terms stay, struck
// through. big for the head of the entry shown.
Column {
    id: et
    property var entry: null
    property var languages: []
    property bool big: false
    spacing: big ? Theme.s1 : 2
    readonly property var shown: {
        const out = (languages || []).slice()
        for (const l of TermData.languages(entry)) if (TermData.terms(entry, l).length && out.indexOf(l) < 0) out.push(l)
        return out
    }
    Accessible.role: Accessible.StaticText
    Accessible.name: shown.map(l => Backend.languageName(l) + ": " + (TermData.terms(entry, l).map(t => t.text).join(", ") || qsTr("missing"))).join("; ")
    Repeater {
        model: et.shown
        Row {
            id: line
            required property string modelData
            readonly property var ts: TermData.terms(et.entry, modelData)
            width: et.width
            spacing: 6
            LangChip { code: line.modelData; anchors.verticalCenter: parent.verticalCenter }
            Text {
                width: line.width - x
                anchors.verticalCenter: parent.verticalCenter
                textFormat: Text.StyledText
                wrapMode: Text.Wrap
                color: line.ts.length ? Theme.text : Theme.text3
                font.family: Theme.font
                font.pixelSize: et.big ? 16 : Theme.body
                font.weight: et.big ? 650 : 500
                font.italic: !line.ts.length
                horizontalAlignment: Backend.rtl(line.modelData) ? Text.AlignRight : Text.AlignLeft
                text: !line.ts.length ? qsTr("missing")
                      : line.ts.map(t => {
                          const s = (t.text || "").replace(/&/g, "&amp;").replace(/</g, "&lt;")
                          return t.status === "forbidden" ? "<s><font color=\"" + Theme.danger + "\">" + s + "</font></s>" : s
                        }).join(", ")
            }
        }
    }
}
