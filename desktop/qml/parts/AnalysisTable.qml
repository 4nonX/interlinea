import QtQuick
import QtQuick.Layouts
import Interlinea

// A match analysis (the web's analysis table): each band with its words,
// segments and share of the words, and the total.
Table {
    id: analysis
    property var report: ({})
    readonly property var bandRows: report.rows || []
    readonly property var total: report.total || {}
    name: qsTr("Match analysis")
    columns: [{ title: qsTr("Match"), fill: 1 }, { title: qsTr("Segments"), width: 96, align: "right" }, { title: qsTr("Words"), width: 96, align: "right" }, { title: qsTr("Share"), width: 80, align: "right" }]
    Repeater {
        model: analysis.bandRows
        TableRow {
            id: arow
            required property var modelData
            readonly property int words: modelData.words || 0
            Accessible.name: Bands.shortName(modelData.band) + " " + modelData.name
            RowLayout {
                spacing: Theme.s3
                                BandBadge { band: arow.modelData.band; Layout.alignment: Qt.AlignTop }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 2
                    Txt { Layout.fillWidth: true; text: arow.modelData.name; color: arow.words === 0 ? Theme.text3 : Theme.text }
                    Txt { Layout.fillWidth: true; visible: text !== ""; text: Bands.hint(arow.modelData.band); color: Theme.text3; font.pixelSize: Theme.tiny }
                }
            }
            Txt { horizontalAlignment: Text.AlignRight; text: Fmt.num(arow.modelData.segments || 0) }
            Txt { horizontalAlignment: Text.AlignRight; text: Fmt.num(arow.words) }
            Txt { horizontalAlignment: Text.AlignRight; text: (analysis.total.words ? Math.round(100 * arow.words / analysis.total.words) : 0) + "%" }
        }
    }
    TableRow {
        last: true
        Txt { role: "strong"; text: qsTr("Total") }
        Txt { horizontalAlignment: Text.AlignRight; role: "strong"; text: Fmt.num(analysis.total.segments || 0) }
        Txt { horizontalAlignment: Text.AlignRight; role: "strong"; text: Fmt.num(analysis.total.words || 0) }
        Txt { horizontalAlignment: Text.AlignRight; role: "strong"; text: "100%" }
    }
}
