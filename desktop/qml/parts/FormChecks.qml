import QtQuick
import QtQuick.Layouts
import Interlinea

// A project's checks: how context matches count, the quality checks the
// editor runs, the limits for subtitles, the spelling dictionary of each
// target language, the project's own words, the grammar service, and the
// project's own rules.
ColumnLayout {
    id: checks
    property var form
    readonly property var qa: form.s.qa || {}
    readonly property var disabled: qa.disabled || []
    readonly property var enabledKinds: qa.enabled || []
    function isOn(k) { return k.default ? disabled.indexOf(k.kind) < 0 : enabledKinds.indexOf(k.kind) >= 0 }
    readonly property bool spellingOn: { const k = form.kinds.find(x => x.kind === "spelling"); return !!k && isOn(k) }
    readonly property int includedLanguages: {
        const set = {}
        for (const d of form.dicts) if (d.included) set[(d.lang || "").split("-")[0]] = true
        return Object.keys(set).length
    }
    spacing: Theme.s4
    enabled: !form.readOnly
    Layout.fillWidth: true

    // Context matches.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: qsTr("Context matches") }
        Repeater {
            model: [{ key: "previous", title: qsTr("The segment before matches"), desc: qsTr("The usual rule for a context match.") },
                    { key: "both", title: qsTr("The segments before and after match"), desc: qsTr("Stricter: both neighbours must match.") }]
            Choice {
                required property var modelData
                Layout.fillWidth: true
                text: modelData.title
                description: modelData.desc
                checked: ((checks.form.s.analysis || {}).context || "previous") === modelData.key
                onToggled: if (checked) { const k = modelData.key; checks.form.edit("analysis", a => { a.context = k }) }
            }
        }
    }

    // The checks, in two columns.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: qsTr("Quality checks while translating") }
        Txt { visible: checks.form.kinds.length === 0; role: "muted"; text: qsTr("Loading the checks…") }
        GridLayout {
            Layout.fillWidth: true
            columns: 2
            columnSpacing: Theme.s5
            rowSpacing: 10
            Repeater {
                model: checks.form.kinds
                ColumnLayout {
                    required property var modelData
                    Layout.fillWidth: true
                    Layout.preferredWidth: 1
                    Layout.alignment: Qt.AlignTop
                    spacing: 2
                    readonly property string desc: (modelData.description || "") + (modelData.kind === "length" ? " " + qsTr("Limits come from the file (XLIFF maxwidth); documents have none.")
                                                                                    : modelData.kind === "spelling" ? " " + qsTr("With the dictionaries below.")
                                                                                    : modelData.kind === "rule" ? " " + qsTr("The rules below.") : "")
                    CheckBox {
                        Layout.fillWidth: true
                        text: modelData.name
                        checked: checks.isOn(modelData)
                        Accessible.description: parent.desc
                        onToggled: {
                            const kind = modelData.kind, byDefault = !!modelData.default, yes = checked
                            checks.form.editQa(x => {
                                const key = byDefault ? "disabled" : "enabled"
                                x[key] = (x[key] || []).filter(k => k !== kind).concat(byDefault !== yes ? [kind] : [])
                            })
                        }
                    }
                    Txt { Layout.fillWidth: true; Layout.leftMargin: 24; role: "help"; text: parent.desc }
                }
            }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.preferredWidth: 1
                Layout.alignment: Qt.AlignTop
                spacing: 2
                CheckBox {
                    Layout.fillWidth: true
                    text: qsTr("Unedited fuzzy matches")
                    checked: !!checks.qa.uneditedFuzzy
                    onToggled: { const yes = checked; checks.form.editQa(x => { x.uneditedFuzzy = yes }) }
                }
                Txt { Layout.fillWidth: true; Layout.leftMargin: 24; role: "help"; text: qsTr("A fuzzy match is confirmed without a change.") }
            }
        }
    }

    // Subtitles.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: qsTr("Subtitles") }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: qsTr("Limits for subtitle and caption files, checked with their times: the longest line, the most lines, and how fast the subtitle must be read. Closed captions (SCC) also hold at most 32 characters a line.")
        }
        Repeater {
            model: [{ key: "lineLength", label: qsTr("Characters in a line"), min: 1, max: 200, step: 1, fallback: 42, decimals: 0 },
                    { key: "lines", label: qsTr("Lines in a subtitle"), min: 1, max: 10, step: 1, fallback: 2, decimals: 0 },
                    { key: "cps", label: qsTr("Characters per second"), min: 1, max: 100, step: 0.5, fallback: 17, decimals: 1 }]
            ColumnLayout {
                required property var modelData
                spacing: Theme.s1
                RowLayout {
                    spacing: Theme.s2
                    Txt { text: modelData.label; Layout.preferredWidth: 200 }
                    NumberField {
                        from: modelData.min; to: modelData.max; step: modelData.step; decimals: modelData.decimals
                        value: (checks.qa.subtitles || {})[modelData.key] > 0 ? checks.qa.subtitles[modelData.key] : modelData.fallback
                        Accessible.name: modelData.label
                        onEdited: v => { const key = modelData.key; checks.form.editQa(x => { const l = x.subtitles || {}; l[key] = key === "cps" ? v : Math.round(v); x.subtitles = l }) }
                    }
                }
                Txt { visible: text !== ""; role: "error"; text: checks.form.problemsAt("qa.subtitles." + modelData.key) }
            }
        }
    }

    // Spelling.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: qsTr("Spelling") }
        Txt { visible: !checks.spellingOn; role: "muted"; text: qsTr("The spelling check is off.") }
        Repeater {
            model: checks.form.targets
            ColumnLayout {
                id: dictRow
                required property string modelData
                readonly property var fit: checks.form.dictionariesFor(modelData)
                readonly property var values: [""].concat(fit.map(d => d.name))
                spacing: Theme.s1
                RowLayout {
                    spacing: Theme.s2
                    Txt { text: Backend.languageName(modelData); Layout.preferredWidth: 200 }
                    ComboBox {
                        implicitWidth: 320
                        enabled: checks.spellingOn
                        model: [dictRow.fit.length ? qsTr("No spelling check") : qsTr("No dictionary for this language")]
                               .concat(dictRow.fit.map(d => d.name + " (" + Backend.languageName(d.lang) + ")"))
                        currentIndex: Math.max(0, dictRow.values.indexOf((checks.qa.dictionaries || {})[modelData] || ""))
                        Accessible.name: Fmt.fill(qsTr("Spelling dictionary for {language}"), { language: Backend.languageName(modelData) })
                        onActivated: i => {
                            const t = modelData, name = dictRow.values[i]
                            checks.form.editQa(x => { const d = x.dictionaries || {}; if (name) d[t] = name; else delete d[t]; x.dictionaries = d })
                        }
                    }
                }
                Txt { visible: text !== ""; role: "error"; text: checks.form.problemsAt("qa.dictionaries." + modelData) }
            }
        }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: checks.includedLanguages === 1 ? qsTr("A dictionary for one language comes with Interlinea. Others are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files.")
                : checks.includedLanguages > 1 ? Fmt.fill(qsTr("Dictionaries for {n} languages come with Interlinea. Others are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files."), { n: checks.includedLanguages })
                : qsTr("Dictionaries are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files.")
        }
    }

    // Words of the project's own.
    ColumnLayout {
        id: words
        Layout.fillWidth: true
        spacing: Theme.s2
        readonly property var have: checks.qa.words || []
        function add() {
            const x = word.text.trim()
            if (!x || have.indexOf(x) >= 0) return
            checks.form.editQa(q => { q.words = (q.words || []).concat([x]) })
            word.text = ""
        }
        Txt { role: "label"; text: qsTr("Words always spelled right in this project") }
        Flow {
            visible: words.have.length > 0
            Layout.fillWidth: true
            spacing: 6
            Accessible.name: qsTr("Words always spelled right in this project")
            Repeater {
                model: words.have
                Rectangle {
                    required property string modelData
                    implicitWidth: wordRow.implicitWidth + 8
                    implicitHeight: 22
                    radius: 4
                    color: Theme.dark ? "#2a3038" : "#eceff3"
                    Row {
                        id: wordRow
                        x: 6
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: 2
                        Txt { text: modelData; color: Theme.text2; font.pixelSize: Theme.tiny; font.weight: 600; anchors.verticalCenter: parent.verticalCenter }
                        IconButton {
                            implicitWidth: 18; implicitHeight: 18; iconSize: 12
                            iconName: "x"
                            tip: Fmt.fill(qsTr("Remove {word}"), { word: modelData })
                            onClicked: { const w = modelData; checks.form.editQa(q => { q.words = (q.words || []).filter(x => x !== w) }) }
                        }
                    }
                }
            }
        }
        RowLayout {
            spacing: Theme.s2
            TextField { id: word; implicitWidth: 300; placeholderText: qsTr("A name or term"); Accessible.name: qsTr("Add a word"); onAccepted: words.add() }
            Button { small: true; iconName: "plus"; text: qsTr("Add"); Accessible.name: qsTr("Add the word"); enabled: word.text.trim() !== ""; onClicked: words.add() }
        }
        Txt { visible: text !== ""; role: "error"; text: checks.form.problemsAt("qa.words") }
    }

    // The grammar service that reads the translations, if any.
    ColumnLayout {
        id: grammarBox
        Layout.fillWidth: true
        spacing: Theme.s2
        readonly property string set: checks.qa.grammar || ""
        readonly property bool gone: set !== "" && !checks.form.grammar.some(n => n.toLowerCase() === set.toLowerCase())
        readonly property var values: [""].concat(gone ? [set] : [], checks.form.grammar)
        Txt { role: "label"; text: qsTr("Grammar") }
        RowLayout {
            spacing: Theme.s2
            Txt { text: qsTr("Grammar service"); Layout.preferredWidth: 200 }
            ComboBox {
                implicitWidth: 320
                model: [checks.form.grammar.length === 0 && !grammarBox.gone ? qsTr("No grammar service yet") : qsTr("No grammar check")]
                       .concat(grammarBox.gone ? [Fmt.fill(qsTr("{name} (no longer there)"), { name: grammarBox.set })] : [], checks.form.grammar)
                currentIndex: Math.max(0, grammarBox.values.findIndex((v, i) => i > 0 && v.toLowerCase() === grammarBox.set.toLowerCase()))
                Accessible.name: qsTr("Grammar service")
                onActivated: i => { const name = grammarBox.values[i]; checks.form.editQa(x => { if (name) x.grammar = name; else delete x.grammar }) }
            }
        }
        Txt { visible: text !== ""; role: "error"; text: checks.form.problemsAt("qa.grammar") }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: checks.form.host ? qsTr("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under Settings, by an administrator of the server.")
                                   : qsTr("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under Settings.")
        }
    }

    // Rules of the project's own.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        readonly property var rules: checks.qa.rules || []
        Txt { role: "label"; text: qsTr("Rules of your own") }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: qsTr("Regular expressions the translation must or must not match, only where the source matches one if you give it, or must match as often as the source matches.")
        }
        Repeater {
            // By count, so typing in a rule keeps it where it is.
            model: parent.rules.length
            Rectangle {
                id: rule
                required property int index
                readonly property var r: (checks.qa.rules || [])[index] || {}
                readonly property string n: Fmt.fill(qsTr("rule {n}"), { n: index + 1 })
                function setRule(key, value, optional) {
                    const i = index
                    checks.form.editQa(q => {
                        const a = q.rules || []
                        const o = a[i] || {}
                        if (optional && !value) delete o[key]
                        else o[key] = value
                        a[i] = o
                        q.rules = a
                    })
                }
                Layout.fillWidth: true
                implicitHeight: ruleBody.implicitHeight + 2 * Theme.s3
                radius: Theme.rControl
                color: Theme.surface
                border.color: Theme.border
                Accessible.name: r.name || Fmt.fill(qsTr("Rule {n}"), { n: index + 1 })
                ColumnLayout {
                    id: ruleBody
                    x: Theme.s3; y: Theme.s3
                    width: parent.width - 2 * Theme.s3
                    spacing: Theme.s3
                    GridLayout {
                        Layout.fillWidth: true
                        columns: 2
                        columnSpacing: Theme.s4
                        rowSpacing: Theme.s3
                        Field {
                            Layout.preferredWidth: 1
                            label: qsTr("Name")
                            TextField { text: rule.r.name || ""; Accessible.name: Fmt.fill(qsTr("Name of {rule}"), { rule: rule.n }); onTextEdited: rule.setRule("name", text, false) }
                        }
                        Field {
                            Layout.preferredWidth: 1
                            label: qsTr("The translation")
                            ComboBox {
                                readonly property var values: ["must-not", "must", "count"]
                                model: [qsTr("must not match"), qsTr("must match"), qsTr("must match as often as the source")]
                                currentIndex: Math.max(0, values.indexOf(rule.r.mode || "must-not"))
                                Accessible.name: Fmt.fill(qsTr("What the translation must do, {rule}"), { rule: rule.n })
                                onActivated: i => rule.setRule("mode", values[i], false)
                            }
                        }
                        Field {
                            Layout.preferredWidth: 1
                            label: qsTr("Translation pattern")
                            TextField { text: rule.r.target || ""; font.family: "monospace"; Accessible.name: Fmt.fill(qsTr("Translation pattern of {rule}"), { rule: rule.n }); onTextEdited: rule.setRule("target", text, false) }
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            Layout.preferredWidth: 1
                            spacing: Theme.s2
                            Row {
                                spacing: 6
                                Txt { text: qsTr("Source pattern"); role: "label" }
                                Txt { text: rule.r.mode === "count" ? qsTr("needed") : qsTr("optional"); color: Theme.text3 }
                            }
                            TextField { Layout.fillWidth: true; text: rule.r.source || ""; font.family: "monospace"; Accessible.name: Fmt.fill(qsTr("Source pattern of {rule}"), { rule: rule.n }); onTextEdited: rule.setRule("source", text, true) }
                        }
                        Field {
                            Layout.preferredWidth: 1
                            label: qsTr("Shown as")
                            ComboBox {
                                readonly property var values: ["warning", "error"]
                                model: [qsTr("Warning"), qsTr("Error")]
                                currentIndex: Math.max(0, values.indexOf(rule.r.severity || "warning"))
                                Accessible.name: Fmt.fill(qsTr("How {rule} is shown"), { rule: rule.n })
                                onActivated: i => rule.setRule("severity", values[i], false)
                            }
                        }
                        Field {
                            Layout.preferredWidth: 1
                            label: qsTr("Message")
                            optional: true
                            TextField { text: rule.r.message || ""; Accessible.name: Fmt.fill(qsTr("Message of {rule}"), { rule: rule.n }); onTextEdited: rule.setRule("message", text, true) }
                        }
                    }
                    Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: checks.form.problemsAt("qa.rules[" + rule.index + "]") }
                    Button {
                        kind: "quiet"
                        small: true
                        iconName: "x"
                        text: qsTr("Remove rule")
                        Accessible.name: Fmt.fill(qsTr("Remove {rule}"), { rule: rule.n })
                        onClicked: { const i = rule.index; checks.form.editQa(q => { q.rules = (q.rules || []).filter((_, k) => k !== i) }) }
                    }
                }
            }
        }
        Button {
            small: true
            iconName: "plus"
            text: qsTr("Add a rule")
            onClicked: checks.form.editQa(q => { q.rules = (q.rules || []).concat([{ name: "", target: "", mode: "must-not", severity: "warning" }]) })
        }
    }
}
