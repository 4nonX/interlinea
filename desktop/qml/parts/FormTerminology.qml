import QtQuick
import QtQuick.Layouts
import Interlinea

// A project's terminology: which masters it uses and where new terms go.
// On an existing project the termbase setup itself stays; the masters and
// updating can change.
ColumnLayout {
    id: term
    property var form
    readonly property var t: form.s.terminology || {}
    readonly property string mode: t.mode || "own"
    readonly property var used: t.masters || []
    readonly property var addLangs: t.addLanguages || []
    readonly property string writeTo: t.writeTo || ""
    readonly property var langs: [form.source].concat(form.targets).filter(l => !!l)
    // The masters: those on the connection, and any the project uses that
    // are kept elsewhere.
    readonly property var masters: {
        const own = t.termbase || ""
        const out = (form.catalog.termbases || []).filter(tb => tb.kind !== "project" && tb.name !== own)
        for (const name of used) if (!out.some(m => m.name === name)) out.push({ name: name, missing: true })
        return out
    }
    function setTerm(change) { form.edit("terminology", change) }
    spacing: Theme.s4
    enabled: !form.readOnly
    Layout.fillWidth: true

    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: term.mode === "direct" ? qsTr("Master termbases") : qsTr("Master termbases to use") }
        Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Masters you keep across projects, for a client, a domain or a set of languages.") }
        Txt { visible: term.masters.length === 0; Layout.fillWidth: true; role: "muted"; text: qsTr("There is no master termbase yet. The project gets a termbase of its own.") }
        Repeater {
            model: term.masters
            ColumnLayout {
                id: mrow
                required property var modelData
                readonly property string name: modelData.name
                readonly property bool chosen: term.used.indexOf(name) >= 0
                readonly property var missingLangs: chosen && !modelData.missing ? term.langs.filter(l => (modelData.languages || []).indexOf(l) < 0) : []
                readonly property bool adds: term.addLangs.indexOf(name) >= 0
                Layout.fillWidth: true
                spacing: Theme.s1
                ChoiceCheck {
                    Layout.fillWidth: true
                    text: mrow.name
                    checked: mrow.chosen
                    onToggled: {
                        const on = checked, name = mrow.name
                        term.setTerm(x => {
                            const next = (x.masters || []).filter(m => m !== name).concat(on ? [name] : [])
                            // Choosing a master for a project with only its own termbase
                            // layers the project over it; taking the last one away leaves
                            // only its own.
                            const was = x.mode || "own"
                            x.mode = !next.length ? "own" : was === "own" ? "layered" : was
                            x.masters = next
                            x.addLanguages = (x.addLanguages || []).filter(l => next.indexOf(l) >= 0)
                            if (next.indexOf(x.writeTo || "") < 0) x.writeTo = next[0] || ""
                        })
                    }
                    Txt { visible: !!mrow.modelData.missing; role: "muted"; text: qsTr("Not kept where this project is.") }
                    Flow {
                        visible: !mrow.modelData.missing
                        width: parent.width
                        spacing: Theme.s2
                        Badge { text: term.form.host ? term.form.host : qsTr("Local"); kind: term.form.host ? "server" : ""; iconName: term.form.host ? "cloud" : "hard-drive" }
                        Txt { role: "muted"; text: TermData.entries(mrow.modelData.entries || 0); wrapMode: Text.NoWrap }
                        Repeater { model: mrow.modelData.languages || []; LangChip { required property string modelData; code: modelData } }
                    }
                }
                ColumnLayout {
                    visible: mrow.missingLangs.length > 0
                    Layout.fillWidth: true
                    Layout.leftMargin: 26
                    Layout.bottomMargin: Theme.s1
                    spacing: Theme.s1
                    Note {
                        text: Fmt.fill(qsTr("{master} has no {languages}."), { master: mrow.name, languages: Fmt.listOf(mrow.missingLangs.map(l => Backend.languageName(l))) })
                              + (mrow.adds ? "" : " " + (term.mode === "direct" && term.writeTo === mrow.name ? qsTr("New terms go into this master, so it needs the languages.")
                                                                                                                : Fmt.fill(qsTr("Otherwise {master} is used for the languages it has, and terms in the others stay in the project termbase."), { master: mrow.name })))
                    }
                    CheckBox {
                        text: mrow.missingLangs.length === 1 ? Fmt.fill(qsTr("Add it to {master}"), { master: mrow.name }) : Fmt.fill(qsTr("Add them to {master}"), { master: mrow.name })
                        Accessible.name: Fmt.fill(qsTr("Add the missing languages to {master}"), { master: mrow.name })
                        checked: mrow.adds
                        onToggled: {
                            const on = checked, name = mrow.name
                            term.setTerm(x => { x.addLanguages = (x.addLanguages || []).filter(l => l !== name).concat(on ? [name] : []) })
                        }
                    }
                }
            }
        }
        Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: [term.form.problemsAt("terminology.masters"), term.form.problemsAt("terminology.addLanguages")].filter(x => x).join("\n") }
    }

    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { role: "label"; text: qsTr("Where new terms go") }
        Txt {
            visible: !term.form.isNew
            Layout.fillWidth: true
            textFormat: Text.StyledText
            text: {
                const said = term.mode === "direct" ? qsTr("Straight into a master.")
                           : term.mode === "own" ? qsTr("Into the project's own termbase, without masters.")
                           : Fmt.fill(qsTr("Into the {projectTermbase}, then merged into the masters."), { projectTermbase: Words.get("projectTermbase").toLowerCase() })
                const esc = x => x.replace(/&/g, "&amp;").replace(/</g, "&lt;")
                return esc(said) + " <font color=\"" + Theme.text3 + "\">" + esc(qsTr("This stays as the project was set up, so the terms already collected stay where they are.")) + "</font>"
            }
        }
        Repeater {
            model: term.form.isNew ? [
                { key: "layered", title: Fmt.fill(qsTr("{projectTermbase}, then merge into the masters"), { projectTermbase: Words.get("projectTermbase") }),
                  desc: qsTr("New and changed terms stay in the project. When the project is done, you review them and merge into the masters. The masters stay unchanged until then."), tag: qsTr("Recommended") },
                { key: "direct", title: qsTr("Directly into a master"), desc: qsTr("New terms are saved to the master right away. Simple when you work alone on this client."), tag: "" },
                { key: "own", title: qsTr("Own project termbase only"), desc: qsTr("No master is used. You can turn it into a master or merge it into one later."), tag: "" }
            ] : []
            Choice {
                required property var modelData
                Layout.fillWidth: true
                text: modelData.title
                description: modelData.desc
                tag: modelData.tag
                checked: term.mode === modelData.key
                enabled: modelData.key === "own" || term.used.length > 0
                onToggled: {
                    if (!checked) return
                    const key = modelData.key
                    term.setTerm(x => {
                        x.mode = key
                        const ms = x.masters || []
                        if (key === "own") x.masters = []
                        if (key === "direct") { if (!x.writeTo) x.writeTo = ms[0] || "" }
                        else delete x.writeTo
                    })
                }
            }
        }
        Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: term.form.problemsAt("terminology.mode") }
    }

    Field {
        visible: term.mode === "direct" && term.used.length > 1
        label: qsTr("Master for new terms")
        ComboBox {
            model: term.used
            currentIndex: Math.max(0, term.used.indexOf(term.writeTo))
            onActivated: i => { const name = term.used[i]; term.setTerm(x => { x.writeTo = name }) }
        }
    }
    Txt { visible: text !== ""; Layout.fillWidth: true; role: "error"; text: term.form.problemsAt("terminology.writeTo") }

    ColumnLayout {
        visible: term.mode === "layered"
        Layout.fillWidth: true
        spacing: Theme.s2
        CheckBox {
            text: Fmt.fill(qsTr("{update} each time the project opens"), { update: Words.get("updateFromMaster") })
            checked: !!term.t.autoUpdate
            onToggled: { const on = checked; term.setTerm(x => { x.autoUpdate = on }) }
        }
        Txt {
            Layout.fillWidth: true
            role: "help"
            text: term.t.autoUpdate ? qsTr("Changes made to the masters since the project last opened are taken in when the editor opens.")
                                    : Fmt.fill(qsTr("Changes made to the masters are listed under {reviewMerge} until you take them in yourself."), { reviewMerge: Words.get("reviewMerge") })
        }
    }
}
