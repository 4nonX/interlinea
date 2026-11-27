import QtQuick
import QtQuick.Layouts
import QtQuick.Dialogs
import QtQml.Models
import Interlinea

// Settings (web/src/screens/Settings.tsx and the desktop's own parts): the
// interface language, your name, the terminology wording, the editor, the
// templates, dictionaries, translation and grammar services and access
// tokens of each place, the team servers, where work is saved, and about.
Page {
    id: page
    title: qsTr("Settings")
    maxWidth: 760

    signal notice(string text)

    readonly property var ready: Backend.connections.filter(c => c.ready)
    readonly property bool many: ready.length > 1
    readonly property var local: Backend.local

    // What each place holds, by connection id.
    property var templates: ({})
    property var dictionaries: ({})
    property var services: ({})
    property var tokens: ({})
    property var serviceKinds: []
    property var includedOpen: ({})
    property var dictionaryProblems: []
    // Dictionaries chosen as loose .aff and .dic files whose language is
    // still to say: { cid, name, aff, dic, lang }.
    property var loose: []
    property string serviceFormAt: ""
    property string serviceFormKind: ""
    property var madeToken: null

    function placeName(c) {
        return c.local ? qsTr("On this computer") : Fmt.fill(qsTr("On {host}"), { host: c.displayName })
    }
    function put(map, cid, value) {
        return Object.assign({}, map, { [cid]: value })
    }

    function reload() {
        for (const c of ready) {
            const cid = c.id
            Api.get(cid, "/api/templates", r => { if (r.ok) templates = put(templates, cid, r.data) })
            Api.get(cid, "/api/dictionaries", r => { if (r.ok) dictionaries = put(dictionaries, cid, r.data) })
            Api.get(cid, "/api/providers", r => { if (r.ok) services = put(services, cid, r.data) })
            Api.get(cid, "/api/me/tokens", r => { if (r.ok) tokens = put(tokens, cid, r.data) })
            if (!serviceKinds.length)
                Api.get(cid, "/api/providers/services", r => { if (r.ok && !serviceKinds.length) serviceKinds = r.data })
        }
    }
    Component.onCompleted: reload()
    onVisibleChanged: if (visible) reload()
    Instantiator {
        model: Backend.connections
        delegate: Connections {
            required property var modelData
            target: modelData
            function onStateChanged() { page.reload() }
            function onUserChanged() { if (modelData.local && !nameField.activeFocus) nameField.text = modelData.user.name || "" }
        }
    }

    function saveName() {
        const name = nameField.text.trim()
        const user = local.user
        if (!local.ready || !name || name === user.name)
            return
        Api.patch(local.id, "/api/users/" + user.id, { name: name }, r => {
            nameError.text = r.ok ? "" : r.error.message
            if (!r.ok) return
            Backend.setUser(local.id, r.data)
            page.notice(Fmt.fill(qsTr("Your name is {name}"), { name: r.data.name }))
        }, user.version)
    }

    function deleteThen(cid, path, query, said) {
        Api.call(cid, "DELETE", path, undefined, r => {
            page.notice(r.ok ? said : r.error.message)
            page.reload()
        }, query)
    }

    // Dictionaries: LibreOffice extensions and ZIP files hold theirs; a
    // loose one is its .aff and its .dic file, chosen together.
    property string dictionariesFor: ""
    function addDictionaries(urls) {
        const cid = dictionariesFor
        dictionaryProblems = []
        const pairs = {}
        for (const u of urls) {
            const path = Backend.localPath(u)
            const name = Backend.fileName(path)
            const ext = name.split(".").pop().toLowerCase()
            if (ext === "aff" || ext === "dic") {
                const base = name.slice(0, -(ext.length + 1))
                pairs[base] = pairs[base] || { cid: cid, name: base, aff: "", dic: "", lang: "" }
                pairs[base][ext] = path
                continue
            }
            const call = Backend.upload(cid, "POST", "/api/dictionaries", path)
            call.done.connect(r => {
                if (r.ok)
                    page.notice(Fmt.fill(qsTr("Added {names}"), { names: r.data.map(d => Fmt.fill(qsTr("{name} ({words})"), { name: d.name, words: Fmt.plural(d.words, qsTr("{n} word"), qsTr("{n} words")) })).join(", ") }))
                else
                    dictionaryProblems = dictionaryProblems.concat([name + ": " + r.error.message])
                page.reload()
            })
        }
        for (const k in pairs) {
            const l = pairs[k]
            if (!l.aff || !l.dic) {
                dictionaryProblems = dictionaryProblems.concat([Fmt.fill(qsTr("{name}: a dictionary is two files, its .aff and its .dic. Choose both."), { name: l.name })])
                continue
            }
            loose = loose.concat([l])
            sendLoose(l)
        }
    }
    function sendLoose(l) {
        const body = { name: l.name, aff: Backend.base64File(l.aff), dic: Backend.base64File(l.dic) }
        if (l.lang) body.lang = l.lang
        Api.post(l.cid, "/api/dictionaries", body, r => {
            const drop = () => { loose = loose.filter(x => !(x.cid === l.cid && x.name === l.name)) }
            if (r.ok) {
                drop()
                const d = r.data[0]
                page.notice(Fmt.fill(qsTr("Added {name} ({words})"), { name: d.name, words: Fmt.plural(d.words, qsTr("{n} word"), qsTr("{n} words")) }))
            } else if (!(r.error.problems || []).some(p => p.field === "lang") || l.lang) {
                drop()
                dictionaryProblems = dictionaryProblems.concat([l.name + ": " + r.error.message])
            }
            // Otherwise the files do not say the language: it is asked for.
            page.reload()
        })
    }
    FileDialog {
        id: dictionaryDialog
        title: qsTr("Spelling dictionaries")
        fileMode: FileDialog.OpenFiles
        nameFilters: [qsTr("Dictionaries (*.oxt *.zip *.aff *.dic)"), qsTr("All files (*)")]
        onAccepted: page.addDictionaries(selectedFiles)
    }

    function serviceWhere(d) {
        const service = d.service || ""
        if (!service || service === "own")
            return d.host || ""
        const known = serviceKinds.find(k => k.service === service)
        const name = known ? known.name : service
        return d.model ? name + ", " + d.model : name
    }

    // Interface language.
    Panel {
        title: qsTr("Interface language")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("The language of menus, buttons and messages. Documents, projects and termbases keep their own languages.") }
        Field {
            label: qsTr("Show Interlinea in")
            help: qsTr("Takes effect when Interlinea is started again.")
            ComboBox {
                id: languagePick
                objectName: "interfaceLanguage"
                readonly property var codes: [""].concat(Backend.interfaceLanguages())
                model: codes.map(c => c === "" ? Fmt.fill(qsTr("The system's language ({name})"), { name: Backend.interfaceLanguageName(Backend.systemLanguage()) })
                                               : Backend.interfaceLanguageName(c))
                currentIndex: Math.max(0, codes.indexOf(Backend.chosenInterfaceLanguage()))
                onActivated: i => Backend.chooseInterfaceLanguage(codes[i])
            }
        }
    }

    // Your name.
    Panel {
        title: qsTr("Your name")
        Field {
            label: qsTr("Name")
            help: qsTr("Entries and translations you change on this computer are signed with it, in their history and in exported files. On a team server your account's name is used.")
            error: nameError.text
            TextField {
                id: nameField
                placeholderText: qsTr("Me")
                enabled: page.local.ready
                text: page.local.user.name || ""
                Accessible.name: qsTr("Your name")
                onEditingFinished: page.saveName()
            }
        }
        Text { id: nameError; visible: false }
    }

    // Terminology wording.
    Panel {
        title: qsTr("Terminology wording")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("Project termbases work like branches of a master. Choose how the interface names this. The behaviour is the same either way.") }
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            Accessible.name: qsTr("Wording")
            Choice {
                Layout.fillWidth: true
                text: qsTr("Plain words")
                description: qsTr("Project termbase, review and merge, update from master, history.")
                checked: !Backend.gitWords
                onClicked: Backend.gitWords = false
            }
            Choice {
                Layout.fillWidth: true
                text: qsTr("Git terms")
                description: qsTr("Branch, diff and merge, pull, log, ours and theirs.")
                checked: Backend.gitWords
                onClicked: Backend.gitWords = true
            }
        }
    }

    // Editor.
    Panel {
        title: qsTr("Editor")
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            ChoiceCheck {
                Layout.fillWidth: true
                text: qsTr("Insert the best memory match when a segment opens")
                description: qsTr("Only into empty segments. The match percentage stays next to the segment until you confirm.")
                checked: Backend.option("autoInsert")
                onToggled: Backend.setOption("autoInsert", checked)
            }
            ChoiceCheck {
                Layout.fillWidth: true
                text: qsTr("Complete words while you type")
                description: qsTr("After two letters, the terms found for the segment and the words of its memory matches are offered. Tab or Enter takes one, Escape closes the list.")
                checked: Backend.option("autoComplete")
                onToggled: Backend.setOption("autoComplete", checked)
            }
            ChoiceCheck {
                Layout.fillWidth: true
                text: qsTr("Fill repetitions when you confirm")
                description: qsTr("Other segments with the same text and tags get the same translation and are confirmed. Segments you already confirmed are left alone.")
                checked: Backend.option("autoPropagate")
                onToggled: Backend.setOption("autoPropagate", checked)
            }
        }
        ColumnLayout {
            Layout.fillWidth: true
            spacing: Theme.s2
            Accessible.name: qsTr("Context matches (CM) in new projects")
            Txt { Layout.fillWidth: true; role: "label"; text: qsTr("Context matches (CM) in new projects") }
            Txt { Layout.fillWidth: true; role: "help"; text: qsTr("Each project counts context matches its own way; change it in the project's settings.") }
            Choice {
                Layout.fillWidth: true
                text: qsTr("The segment before also matches")
                description: qsTr("The usual rule.")
                checked: Backend.option("contextRule") !== "both"
                onClicked: Backend.setOption("contextRule", "previous")
            }
            Choice {
                Layout.fillWidth: true
                text: qsTr("The segments before and after also match")
                description: qsTr("Stricter: such a match is often shown as 101%.")
                checked: Backend.option("contextRule") === "both"
                onClicked: Backend.setOption("contextRule", "both")
            }
        }
    }

    // Project templates.
    Panel {
        title: qsTr("Project templates")
        ColumnLayout {
            Layout.fillWidth: true
            spacing: 0
            Repeater {
                model: page.ready
                ColumnLayout {
                    id: tplPlace
                    required property var modelData
                    readonly property var list: page.templates[modelData.id] || []
                    Layout.fillWidth: true
                    visible: list.length > 0
                    spacing: 0
                    Txt { visible: page.many; role: "label"; text: page.placeName(tplPlace.modelData); Layout.topMargin: Theme.s2 }
                    Repeater {
                        model: tplPlace.list
                        ListRow {
                            required property var modelData
                            Txt { role: "strong"; text: modelData.name; wrapMode: Text.NoWrap }
                            LangPair { source: modelData.sourceLang; targets: modelData.targetLangs || [] }
                            Txt {
                                Layout.fillWidth: true
                                role: "muted"
                                text: [modelData.client || "", Fmt.fill(qsTr("saved {date}"), { date: Fmt.date(modelData.changedAt || modelData.createdAt) })].filter(x => x).join(", ")
                            }
                            actions: ConfirmButton {
                                text: qsTr("Delete")
                                accessibleName: Fmt.fill(qsTr("Delete the template {name}"), { name: modelData.name })
                                question: Fmt.fill(qsTr("Delete the template “{name}”? Projects made from it stay as they are."), { name: modelData.name })
                                onConfirmed: page.deleteThen(tplPlace.modelData.id, "/api/templates", { name: modelData.name },
                                                             Fmt.fill(qsTr("Deleted the template “{name}”"), { name: modelData.name }))
                            }
                        }
                    }
                }
            }
            Txt {
                Layout.fillWidth: true
                visible: page.ready.every(c => !(page.templates[c.id] || []).length)
                role: "help"
                text: qsTr("No template yet. Save a project's settings as a template at the end of New project, or from a project's settings.")
            }
        }
    }

    // Spelling dictionaries.
    Panel {
        title: qsTr("Spelling dictionaries")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("Hunspell dictionaries, as LibreOffice and Firefox use them, checked by the core. Each project chooses one per target language in its checks; a new project takes the one that fits each language.") }
        Repeater {
            model: page.ready
            ColumnLayout {
                id: dictPlace
                required property var modelData
                readonly property string cid: modelData.id
                readonly property var all: page.dictionaries[cid] || []
                readonly property var included: all.filter(d => d.included)
                readonly property var added: all.filter(d => !d.included)
                readonly property bool open: !!page.includedOpen[cid]
                Layout.fillWidth: true
                spacing: Theme.s2
                Txt { visible: page.many; role: "label"; text: page.placeName(dictPlace.modelData) }
                // Those that come with Interlinea, folded away until asked for.
                Button {
                    visible: dictPlace.included.length > 0
                    kind: "quiet"
                    small: true
                    leftPadding: 0
                    iconName: dictPlace.open ? "chevron-down" : "chevron-right"
                    text: Fmt.fill(dictPlace.included.length === 1 ? qsTr("{length} dictionary comes with Interlinea") : qsTr("{length} dictionaries come with Interlinea"), { length: dictPlace.included.length })
                    Accessible.name: dictPlace.open ? qsTr("Hide the dictionaries that come with Interlinea") : qsTr("Show the dictionaries that come with Interlinea")
                    onClicked: page.includedOpen = page.put(page.includedOpen, dictPlace.cid, !dictPlace.open)
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: dictPlace.open
                    spacing: 0
                    Repeater {
                        model: dictPlace.open ? dictPlace.included : []
                        ListRow {
                            required property var modelData
                            Txt { role: "strong"; text: modelData.name; wrapMode: Text.NoWrap }
                            LangChip { code: modelData.lang }
                            Txt { Layout.fillWidth: true; role: "muted"; text: Fmt.plural(modelData.words, qsTr("{n} word"), qsTr("{n} words")) }
                        }
                    }
                    Txt { Layout.fillWidth: true; Layout.topMargin: Theme.s2; role: "help"; text: qsTr("From the LibreOffice dictionaries, each with its authors' licence. Add one of the same name to use your own instead.") }
                }
                Txt { visible: dictPlace.included.length > 0; role: "label"; text: qsTr("Added dictionaries") }
                Txt {
                    visible: dictPlace.added.length === 0
                    role: "muted"
                    text: dictPlace.included.length ? qsTr("None added.") : qsTr("No dictionary yet.")
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    Repeater {
                        model: dictPlace.added
                        ListRow {
                            required property var modelData
                            Txt { role: "strong"; text: modelData.name; wrapMode: Text.NoWrap }
                            LangChip { code: modelData.lang }
                            Txt { Layout.fillWidth: true; role: "muted"; text: Fmt.plural(modelData.words, qsTr("{n} word"), qsTr("{n} words")) }
                            actions: ConfirmButton {
                                text: qsTr("Delete")
                                accessibleName: Fmt.fill(qsTr("Delete the dictionary {name}"), { name: modelData.name })
                                question: Fmt.fill(qsTr("Delete the dictionary {name}?"), { name: modelData.name })
                                onConfirmed: page.deleteThen(dictPlace.cid, "/api/dictionaries/" + Api.seg(modelData.name), {},
                                                             Fmt.fill(qsTr("Deleted the dictionary {name}"), { name: modelData.name }))
                            }
                        }
                    }
                }
                // Dictionaries chosen as loose files whose language is still to say.
                Txt { visible: page.loose.some(l => l.cid === dictPlace.cid); role: "label"; text: qsTr("Found in the files") }
                Repeater {
                    model: page.loose.filter(l => l.cid === dictPlace.cid)
                    ListRow {
                        id: looseRow
                        required property var modelData
                        Txt { Layout.fillWidth: true; role: "strong"; text: modelData.name }
                        actions: [
                            LanguageField {
                                id: looseLang
                                width: 240
                                Accessible.name: Fmt.fill(qsTr("Language of {name}"), { name: looseRow.modelData.name })
                            },
                            Button {
                                small: true
                                kind: "primary"
                                text: qsTr("Add dictionary")
                                enabled: looseLang.language !== ""
                                Accessible.name: Fmt.fill(qsTr("Add the dictionary {name}"), { name: looseRow.modelData.name })
                                onClicked: page.sendLoose(Object.assign({}, looseRow.modelData, { lang: looseLang.language }))
                            },
                            Button {
                                small: true
                                kind: "quiet"
                                text: qsTr("Cancel")
                                Accessible.name: Fmt.fill(qsTr("Do not add {name}"), { name: looseRow.modelData.name })
                                onClicked: page.loose = page.loose.filter(l => !(l.cid === looseRow.modelData.cid && l.name === looseRow.modelData.name))
                            }
                        ]
                    }
                }
                Button {
                    small: true
                    iconName: "upload"
                    text: qsTr("Add dictionaries")
                    Accessible.name: page.many ? Fmt.fill(qsTr("Add dictionaries on {host}"), { host: dictPlace.modelData.displayName }) : qsTr("Add dictionaries")
                    onClicked: {
                        page.dictionariesFor = dictPlace.cid
                        dictionaryDialog.open()
                    }
                }
            }
        }
        Txt { visible: page.ready.length === 0; role: "muted"; text: qsTr("No dictionary yet.") }
        Repeater {
            model: page.dictionaryProblems
            Txt { required property string modelData; Layout.fillWidth: true; role: "error"; text: modelData }
        }
    }

    // Translation and grammar services.
    Repeater {
        model: [
            { kind: "translate", title: qsTr("Translation services"),
              help: qsTr("Machine translation or a language model of your choice, asked for a suggestion in the editor when you ask for one. Any service works that answers at an address as the integration guide describes. A segment you ask a suggestion for is sent to the service; nothing is sent on its own.") },
            { kind: "check", title: qsTr("Grammar services"),
              help: qsTr("A grammar checker of your choice, which reads the translation you are working on in a project that names it under Quality checks. Any service works that answers a check request at its address as the integration guide describes. In such a project each translation is sent to the service while you work on it; in other projects nothing is sent. Spelling stays with the project's dictionary.") }
        ]
        Panel {
            id: svcPanel
            required property var modelData
            readonly property bool check: modelData.kind === "check"
            title: modelData.title
            Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: svcPanel.modelData.help }
            Repeater {
                model: page.ready
                ColumnLayout {
                    id: svcPlace
                    required property var modelData
                    readonly property string cid: modelData.id
                    readonly property bool admin: modelData.local || modelData.admin
                    readonly property var list: (page.services[cid] || []).filter(d => (d.kind === "check") === svcPanel.check)
                    Layout.fillWidth: true
                    spacing: Theme.s2
                    Txt { visible: page.many; role: "label"; text: page.placeName(svcPlace.modelData) }
                    Txt { visible: svcPlace.list.length === 0; role: "muted"; text: qsTr("No service yet.") }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 0
                        Repeater {
                            model: svcPlace.list
                            ListRow {
                                required property var modelData
                                Txt { role: "strong"; text: modelData.name; wrapMode: Text.NoWrap }
                                Txt { Layout.fillWidth: true; role: "muted"; text: page.serviceWhere(modelData) }
                                actions: ConfirmButton {
                                    visible: svcPlace.admin
                                    text: qsTr("Delete")
                                    accessibleName: Fmt.fill(qsTr("Delete the service {name}"), { name: modelData.name })
                                    question: Fmt.fill(qsTr("Delete the service {name}?"), { name: modelData.name })
                                    onConfirmed: page.deleteThen(svcPlace.cid, "/api/providers/" + Api.seg(modelData.name), {},
                                                                 Fmt.fill(qsTr("Deleted the service {name}"), { name: modelData.name }))
                                }
                            }
                        }
                    }
                    Txt {
                        Layout.fillWidth: true
                        visible: !svcPlace.admin
                        role: "help"
                        text: qsTr("An administrator adds the services of the server. They are asked by the server, for its projects, and their keys stay there.")
                    }
                    Button {
                        visible: svcPlace.admin && !(page.serviceFormAt === svcPlace.cid && page.serviceFormKind === svcPanel.modelData.kind)
                        small: true
                        iconName: "plus"
                        text: svcPanel.check ? qsTr("Add a grammar service") : qsTr("Add a service")
                        onClicked: {
                            page.serviceFormAt = svcPlace.cid
                            page.serviceFormKind = svcPanel.modelData.kind
                        }
                    }
                    ServiceForm {
                        visible: svcPlace.admin && page.serviceFormAt === svcPlace.cid && page.serviceFormKind === svcPanel.modelData.kind
                        Layout.fillWidth: true
                        cid: svcPlace.cid
                        kind: svcPanel.modelData.kind
                        kinds: page.serviceKinds
                        onCancelled: page.serviceFormAt = ""
                        onSaved: name => {
                            page.serviceFormAt = ""
                            page.notice(Fmt.fill(qsTr("Added the service {name}"), { name: name }))
                            page.reload()
                        }
                        onNotice: text => page.notice(text)
                    }
                }
            }
        }
    }

    // Access for programs.
    Panel {
        title: qsTr("Access for programs")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("An AI assistant or a script works as you, with your rights, when it signs in with a token made here. Assistants that speak the Model Context Protocol connect to the address shown with each place.") }
        Repeater {
            model: page.ready
            ColumnLayout {
                id: tokPlace
                required property var modelData
                readonly property string cid: modelData.id
                Layout.fillWidth: true
                spacing: Theme.s2
                Txt { role: "label"; text: page.placeName(tokPlace.modelData) }
                TextEdit {
                    Layout.fillWidth: true
                    readOnly: true
                    selectByMouse: true
                    wrapMode: Text.Wrap
                    color: Theme.text2
                    font.family: Theme.font
                    font.pixelSize: Theme.small
                    text: Fmt.fill(qsTr("Assistants connect to {address}"), { address: Backend.baseUrl(tokPlace.cid) + "/mcp" })
                    Accessible.name: text
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    Repeater {
                        model: page.tokens[tokPlace.cid] || []
                        ListRow {
                            id: tokRow
                            required property var modelData
                            readonly property string name: modelData.name || qsTr("Sign-in")
                            Txt { role: "strong"; text: tokRow.name; wrapMode: Text.NoWrap }
                            Txt {
                                Layout.fillWidth: true
                                role: "muted"
                                text: Fmt.fill(modelData.current ? qsTr("this sign-in, until {date}") : qsTr("until {date}"), { date: Fmt.date(modelData.expiresAt) })
                            }
                            actions: ConfirmButton {
                                visible: !modelData.current
                                text: qsTr("End")
                                accessibleName: Fmt.fill(qsTr("End {name}"), { name: modelData.name || qsTr("Sign-in") })
                                question: Fmt.fill(qsTr("End {name}? What signs in with it is refused from then on."), { name: modelData.name || qsTr("Sign-in") })
                                onConfirmed: page.deleteThen(tokPlace.cid, "/api/me/tokens/" + modelData.id, {},
                                                             Fmt.fill(qsTr("Ended {name}"), { name: modelData.name || qsTr("Sign-in") }))
                            }
                        }
                    }
                }
                Note {
                    Layout.fillWidth: true
                    visible: page.madeToken !== null && page.madeToken.cid === tokPlace.cid
                    kind: "ok"
                    text: page.madeToken ? Fmt.fill(qsTr("The token for {name}. It is shown this once: copy it now."), { name: page.madeToken.name }) : ""
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Theme.s2
                        TextField {
                            Layout.fillWidth: true
                            readOnly: true
                            text: page.madeToken ? page.madeToken.token : ""
                            Accessible.name: qsTr("The token")
                        }
                        Button {
                            small: true
                            text: qsTr("Copy")
                            onClicked: {
                                Backend.copy(page.madeToken.token)
                                page.notice(qsTr("Copied the token"))
                            }
                        }
                        Button { small: true; kind: "quiet"; text: qsTr("Done"); onClicked: page.madeToken = null }
                    }
                }
                RowLayout {
                    spacing: Theme.s3
                    Accessible.name: qsTr("A new access token")
                    Field {
                        Layout.fillWidth: false
                        Layout.preferredWidth: 260
                        label: qsTr("Program the token is for")
                        TextField { id: tokenName }
                    }
                    Field {
                        Layout.fillWidth: false
                        Layout.preferredWidth: 140
                        label: qsTr("Lasts")
                        ComboBox { id: tokenDays; model: [qsTr("30 days"), qsTr("90 days"), qsTr("A year")]; currentIndex: 1 }
                    }
                    Button {
                        Layout.alignment: Qt.AlignBottom
                        text: qsTr("Make a token")
                        enabled: tokenName.text.trim() !== ""
                        onClicked: Api.post(tokPlace.cid, "/api/me/tokens", { name: tokenName.text.trim(), days: [30, 90, 366][tokenDays.currentIndex] }, r => {
                            if (!r.ok) { page.notice(r.error.message); return }
                            page.madeToken = { cid: tokPlace.cid, token: r.data.token, name: r.data.name }
                            tokenName.text = ""
                            page.reload()
                        })
                    }
                }
            }
        }
    }

    // Team servers.
    Panel {
        title: qsTr("Team server")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("A team server keeps a team's termbases in one place: everyone translates with the same terms, and new and changed entries reach the masters through review. Projects and memories can be shared there as well, with work given to translators and reviewers.") }
        Repeater {
            model: Backend.connections.filter(c => !c.local)
            Rectangle {
                id: server
                required property var modelData
                Layout.fillWidth: true
                implicitHeight: sv.implicitHeight + 2 * Theme.s3
                radius: Theme.rControl
                color: Theme.surface
                border.color: Theme.border
                Accessible.role: Accessible.Grouping
                Accessible.name: modelData.displayName
                ColumnLayout {
                    id: sv
                    x: Theme.s3; y: Theme.s3
                    width: parent.width - 2 * Theme.s3
                    spacing: 6
                    Row {
                        spacing: Theme.s2
                        Txt { role: "strong"; text: server.modelData.displayName; wrapMode: Text.NoWrap }
                        Badge {
                            text: [qsTr("Connecting"), qsTr("Ready"), qsTr("Signed out"), qsTr("Can't be reached"), qsTr("Not working")][server.modelData.state]
                            kind: server.modelData.ready ? "ok" : server.modelData.state === 0 ? "" : "warn"
                        }
                    }
                    TextEdit {
                        Layout.fillWidth: true
                        readOnly: true
                        selectByMouse: true
                        wrapMode: Text.Wrap
                        color: Theme.text2
                        font.family: Theme.font
                        font.pixelSize: Theme.body
                        text: [Backend.baseUrl(server.modelData.id),
                               server.modelData.ready ? Fmt.fill(server.modelData.admin ? qsTr("Signed in as {name} ({login}), an administrator there.") : qsTr("Signed in as {name} ({login})."),
                                                                 { name: server.modelData.userName, login: server.modelData.login }) : "",
                               server.modelData.serverVersion ? Fmt.fill(qsTr("Interlinea {version} on the server."), { version: server.modelData.serverVersion }) : "",
                               server.modelData.message].filter(x => x).join("\n")
                        Accessible.name: text
                    }
                    Row {
                        spacing: Theme.s2
                        Button {
                            visible: !server.modelData.ready
                            small: true
                            text: qsTr("Sign in")
                            Accessible.name: Fmt.fill(qsTr("Sign in to {host}"), { host: server.modelData.displayName })
                            onClicked: signIn.start(Backend.baseUrl(server.modelData.id))
                        }
                        Button {
                            visible: server.modelData.state !== 2
                            small: true
                            text: qsTr("Sign out")
                            Accessible.name: Fmt.fill(qsTr("Sign out of {host}"), { host: server.modelData.displayName })
                            onClicked: Backend.signOut(server.modelData.id)
                        }
                        ConfirmButton {
                            text: qsTr("Remove")
                            accessibleName: Fmt.fill(qsTr("Remove {host}"), { host: server.modelData.displayName })
                            question: Fmt.fill(qsTr("Sign out of {host} and forget it? Your work on the server stays there."), { host: server.modelData.displayName })
                            onConfirmed: Backend.removeServer(server.modelData.id)
                        }
                    }
                }
            }
        }
        Field {
            label: Backend.connections.length > 1 ? qsTr("Another server's address") : qsTr("Server address")
            RowLayout {
                spacing: Theme.s2
                TextField {
                    id: serverAddress
                    Layout.fillWidth: true
                    placeholderText: "https://interlinea.example.com"
                    Accessible.name: qsTr("Server address")
                    onAccepted: signIn.start(text.trim() ? Backend.serverUrl(text.trim()) : "")
                }
                Button { text: qsTr("Connect"); onClicked: signIn.start(serverAddress.text.trim() ? Backend.serverUrl(serverAddress.text.trim()) : "") }
            }
        }
        Txt { Layout.fillWidth: true; visible: Backend.connections.length > 1; role: "help"; text: Backend.keychainNote() }
    }
    SignInDialog { id: signIn; onSignedIn: serverAddress.text = "" }

    // Saved work.
    Panel {
        title: qsTr("Saved work")
        Note {
            Layout.fillWidth: true
            visible: page.local.state === 4
            kind: "warn"
            text: page.local.message || qsTr("Work on this computer can't be opened.")
        }
        Txt { Layout.fillWidth: true; visible: page.local.state === 0; role: "muted"; text: qsTr("Starting the work on this computer…") }
        TextEdit {
            Layout.fillWidth: true
            readOnly: true
            selectByMouse: true
            wrapMode: Text.Wrap
            color: Theme.text2
            font.family: Theme.font
            font.pixelSize: Theme.small
            text: Fmt.fill(qsTr("Projects, translations, termbases, memories, templates and dictionaries are saved on this computer as you work, in {folder}. They stay on this computer."), { folder: Backend.dataDir() })
                  + (page.local.serverVersion ? " " + Fmt.fill(qsTr("Core {version}."), { version: page.local.serverVersion }) : "")
            Accessible.name: text
        }
        Row {
            spacing: Theme.s2
            Button { small: true; iconName: "folder-open"; text: qsTr("Open the folder"); onClicked: Backend.openFolder(Backend.dataDir()) }
            Button { visible: page.local.state === 4; small: true; iconName: "refresh"; text: qsTr("Start the core again"); onClicked: Backend.restartCore() }
        }
    }

    // About.
    Panel {
        title: qsTr("About Interlinea")
        Txt { Layout.fillWidth: true; Layout.topMargin: -Theme.s3; role: "muted"; text: qsTr("Interlinea is free software under the GNU Affero General Public License, version 3: you may use it, pass it on and change it under the terms of that licence. It comes without warranty.") }
        Txt { Layout.fillWidth: true; role: "muted"; text: qsTr("Copyright © 2026 Dan Dreßen.") }
        Row {
            spacing: Theme.s2
            Button { small: true; text: qsTr("Read the licence"); onClicked: licenceDialog.open() }
            Button { small: true; text: qsTr("Source code"); onClicked: Qt.openUrlExternally("https://github.com/4nonX/interlinea") }
        }
    }
    Txt {
        Layout.fillWidth: true
        role: "muted"
        text: Fmt.fill(qsTr("Interlinea {version}. Open source: a translation workspace with its editor, terminology, memories and projects."), { version: Backend.version })
    }

    Dialog {
        id: licenceDialog
        objectName: "licence"
        title: qsTr("GNU Affero General Public License, version 3")
        wide: true
        TextArea {
            Layout.fillWidth: true
            implicitHeight: 420
            text: licenceDialog.visible ? Backend.licence() : ""
            area.readOnly: true
            Accessible.name: qsTr("The licence")
        }
        buttons: Button { kind: "primary"; text: qsTr("Close"); onClicked: licenceDialog.close() }
    }
}
