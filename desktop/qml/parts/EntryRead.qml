import QtQuick
import QtQuick.Layouts
import Interlinea

// An entry as it is, to read: its fields, languages and terms.
ColumnLayout {
    id: read
    property var entry: null
    Layout.fillWidth: true
    spacing: Theme.s2
    component Values: Repeater {
        delegate: TextEdit {
            required property var modelData
            Layout.fillWidth: true
            readOnly: true
            selectByMouse: true
            wrapMode: Text.Wrap
            textFormat: Text.StyledText
            color: Theme.text
            font.family: Theme.font
            font.pixelSize: Theme.body
            text: "<span style=\"color:" + Theme.text2 + "\">" + (modelData.field || "").replace(/&/g, "&amp;").replace(/</g, "&lt;") + ":</span> "
                  + (modelData.value || "").replace(/&/g, "&amp;").replace(/</g, "&lt;")
        }
    }
    Values { model: read.entry ? (read.entry.fields || []) : [] }
    Repeater {
        model: read.entry ? (read.entry.languages || []) : []
        ColumnLayout {
            id: lang
            required property var modelData
            Layout.fillWidth: true
            spacing: Theme.s2
            Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
            Row {
                spacing: Theme.s2
                Txt { text: Backend.languageName(lang.modelData.lang); role: "strong"; wrapMode: Text.NoWrap }
                LangChip { code: lang.modelData.lang }
            }
            Values { model: lang.modelData.fields || [] }
            Repeater {
                model: lang.modelData.terms || []
                Rectangle {
                    id: term
                    required property var modelData
                    readonly property string status: modelData.status || "proposed"
                    Layout.fillWidth: true
                    implicitHeight: tv.implicitHeight + 2 * Theme.s3
                    radius: Theme.rControl
                    color: Theme.surface2
                    border.color: Theme.border
                    ColumnLayout {
                        id: tv
                        x: Theme.s3; y: Theme.s3
                        width: parent.width - 2 * Theme.s3
                        spacing: Theme.s1
                        RowLayout {
                            TextEdit {
                                Layout.fillWidth: true
                                readOnly: true
                                selectByMouse: true
                                wrapMode: Text.Wrap
                                text: term.modelData.text || ""
                                color: Theme.text
                                font.family: Theme.font
                                font.pixelSize: 16
                                font.weight: 600
                                horizontalAlignment: Backend.rtl(lang.modelData.lang) ? TextEdit.AlignRight : TextEdit.AlignLeft
                            }
                            Badge { text: TermData.statusLabel(term.status); kind: term.status === "approved" ? "ok" : term.status === "forbidden" ? "danger" : "warn" }
                        }
                        Values { model: term.modelData.fields || [] }
                    }
                }
            }
        }
    }
}
