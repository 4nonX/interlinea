import QtQuick
import QtQuick.Layouts
import Interlinea

// The project's own rules for where sentences end.
ColumnLayout {
    id: seg
    property var form
    readonly property var rules: form.s.segmentation || []
    function setRules(change) {
        const a = form.copy(rules)
        change(a)
        form.put("segmentation", a.length ? a : undefined)
    }
    spacing: Theme.s2
    enabled: !form.readOnly
    Layout.fillWidth: true

    Txt { role: "label"; text: qsTr("Segmentation rules of your own") }
    Txt {
        Layout.fillWidth: true
        role: "help"
        text: Fmt.fill(qsTr("Sentences end as is usual for the source language. A rule changes that at every place where the text before it ends with the first pattern and the text after it begins with the second (regular expressions; one may stay empty). Your rules come first. {which} Files that bring their own segments keep them."),
                       { which: seg.form.isNew ? qsTr("They cut the documents of the project.") : qsTr("They cut the documents added from now on; the files in the project stay as they are.") })
    }
    Repeater {
        // By count, so typing in a rule keeps it where it is.
        model: seg.rules.length
        Rectangle {
            id: rule
            required property int index
            readonly property var r: seg.rules[index] || {}
            readonly property string n: Fmt.fill(qsTr("segmentation rule {n}"), { n: index + 1 })
            function setField(field, value) {
                const i = index
                seg.setRules(a => { if (value === "" ) delete a[i][field]; else a[i][field] = value })
            }
            Layout.fillWidth: true
            implicitHeight: body.implicitHeight + 2 * Theme.s3
            radius: Theme.rControl
            color: Theme.surface
            border.color: Theme.border
            Accessible.name: Fmt.fill(qsTr("Segmentation rule {n}"), { n: index + 1 })
            ColumnLayout {
                id: body
                x: Theme.s3; y: Theme.s3
                width: parent.width - 2 * Theme.s3
                spacing: Theme.s3
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Theme.s4
                    Field {
                        Layout.preferredWidth: 1
                        label: qsTr("At this place")
                        ComboBox {
                            model: [qsTr("a sentence does not end"), qsTr("a sentence ends")]
                            currentIndex: rule.r.break ? 1 : 0
                            Accessible.name: Fmt.fill(qsTr("What happens at the place, {rule}"), { rule: rule.n })
                            onActivated: i => { const k = rule.index; seg.setRules(a => { a[k].break = i === 1 }) }
                        }
                    }
                    Field {
                        Layout.preferredWidth: 1
                        label: qsTr("Text before the place")
                        TextField { text: rule.r.before || ""; font.family: "monospace"; Accessible.name: Fmt.fill(qsTr("Text before the place, {rule}"), { rule: rule.n }); onTextEdited: rule.setField("before", text) }
                    }
                    Field {
                        Layout.preferredWidth: 1
                        label: qsTr("Text after the place")
                        TextField { text: rule.r.after || ""; font.family: "monospace"; Accessible.name: Fmt.fill(qsTr("Text after the place, {rule}"), { rule: rule.n }); onTextEdited: rule.setField("after", text) }
                    }
                }
                Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: seg.form.problemsAt("segmentation[" + rule.index + "]") }
                Button {
                    kind: "quiet"
                    small: true
                    iconName: "x"
                    text: qsTr("Remove rule")
                    Accessible.name: Fmt.fill(qsTr("Remove {rule}"), { rule: rule.n })
                    onClicked: { const i = rule.index; seg.setRules(a => a.splice(i, 1)) }
                }
            }
        }
    }
    Button { small: true; iconName: "plus"; text: qsTr("Add a segmentation rule"); onClicked: seg.setRules(a => a.push({ break: false })) }
    Txt {
        Layout.fillWidth: true
        role: "help"
        text: qsTr("An abbreviation that should not end a sentence: “does not end”, before \\bca\\., after \\s. A new segment after every semicolon: “ends”, before ;, after \\s.")
    }
}
