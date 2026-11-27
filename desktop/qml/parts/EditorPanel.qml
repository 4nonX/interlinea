import QtQuick
import QtQuick.Layouts
import QtQuick.Controls as C
import Interlinea

// The panel beside the segments (the web's .results): a conflict to
// settle, the translation results and suggestion services, the checks,
// the memory search, the scored review, the comments, adding a term, the
// segment's tools and the keyboard shortcuts.
Rectangle {
    id: panel
    property var editor
    readonly property var seg: { editor.revision; return editor.activeSegment || {} }
    readonly property bool all: !!editor.conn && (editor.conn.local || Roles.allows((editor.project.access || {}).role || "", "manage"))
    readonly property string me: editor.conn ? editor.conn.login : ""
    color: Theme.bg
    Accessible.name: qsTr("Translation results")

    function showCapture() {
        captureSourceField.text = editor.captureSource
        captureTargetField.text = editor.captureTarget
        captureNote.text = ""
        Qt.callLater(() => { (editor.captureSource ? captureTargetField : captureSourceField).forceActiveFocus(); flick.reveal(captureBox) })
    }
    function showLookup() {
        lookupField.text = editor.lookupQuery
        Qt.callLater(() => { lookupField.forceActiveFocus(); lookupField.selectAll(); flick.reveal(lookupBox) })
    }
    function commentSent() { commentText.text = "" }
    function issueSent() { issueNote.text = "" }
    function closeCapture() { editor.captureOpen = false; if (editor.te) editor.te.textEdit.forceActiveFocus() }
    function closeLookup() { editor.lookupOpen = false; if (editor.te) editor.te.textEdit.forceActiveFocus() }
    function byLine(o) { return Fmt.fill(qsTr("{who}, {date}"), { who: o.by || "", date: Fmt.date(o.at || "") }) }
    function highlight(s, q) {
        const at = s.toLowerCase().indexOf(q.toLowerCase())
        const esc = t => t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
        if (at < 0 || !q) return esc(s)
        return esc(s.slice(0, at)) + "<span style=\"background:" + Theme.mark + "\">" + esc(s.slice(at, at + q.length)) + "</span>" + esc(s.slice(at + q.length))
    }

    Rectangle { width: 1; height: parent.height; color: Theme.border }

    component Head: RowLayout {
        property string title
        property int count: 0
        Layout.fillWidth: true
        spacing: Theme.s2
        Txt { text: parent.title; role: "label"; font.pixelSize: Theme.small; color: Theme.text2; Accessible.role: Accessible.Heading }
        Badge { visible: parent.count > 0; text: String(parent.count) }
        Item { Layout.fillWidth: true }
    }
    component Box: Rectangle {
        default property alias content: inner.data
        property alias spacing: inner.spacing
        property color tone: Theme.surface
        Layout.fillWidth: true
        implicitHeight: inner.implicitHeight + 2 * Theme.s2
        radius: Theme.rControl
        color: tone
        border.color: Theme.border
        ColumnLayout {
            id: inner
            x: Theme.s3; y: Theme.s2
            width: parent.width - 2 * Theme.s3
            spacing: Theme.s1
        }
    }
    component Small: Button {
        small: true
        implicitHeight: 24
        leftPadding: 8
        rightPadding: 8
        font.pixelSize: Theme.tiny
    }

    Flickable {
        id: flick
        anchors.fill: parent
        anchors.leftMargin: 1
        contentHeight: body.implicitHeight + 2 * Theme.s4
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        C.ScrollBar.vertical: ScrollBar {}
        function reveal(item) {
            const y = item.mapToItem(body, 0, 0).y
            if (y < contentY || y + item.height > contentY + height) contentY = Math.max(0, Math.min(y - Theme.s4, contentHeight - height))
        }

        ColumnLayout {
            id: body
            x: Theme.s4; y: Theme.s4
            width: flick.width - 2 * Theme.s4
            spacing: Theme.s4

            // Someone else saved the segment being worked on.
            Box {
                visible: panel.editor.conflictId !== "" && panel.editor.conflictId in panel.editor.conflicts
                tone: Theme.warnBg
                spacing: Theme.s2
                Txt {
                    Layout.fillWidth: true
                    readonly property var theirs: panel.editor.conflicts[panel.editor.conflictId] || {}
                    text: Fmt.fill(qsTr("{who} saved segment {n} while you worked on it. Their translation: “{text}”."),
                                   { who: theirs.changedBy || qsTr("Someone"), n: (panel.editor.rowOf[panel.editor.conflictId] || 0) + 1, text: Fmt.plain(theirs.target) })
                }
                RowLayout {
                    spacing: Theme.s2
                    Button { small: true; text: qsTr("Keep mine"); onClicked: panel.editor.keepMine() }
                    Button { small: true; text: qsTr("Take theirs"); onClicked: panel.editor.takeTheirs() }
                }
            }

            // Translation results: memory matches and terms, Ctrl+1 to 9.
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Theme.s2
                Head { title: qsTr("Translation results"); count: panel.editor.results.length }
                Txt { visible: !panel.editor.results.length; Layout.fillWidth: true; role: "muted"; text: qsTr("No memory matches or terms for this segment.") }
                Repeater {
                    model: panel.editor.results
                    Rectangle {
                        id: card
                        required property var modelData
                        required property int index
                        readonly property bool isMatch: modelData.kind === "match"
                        readonly property bool chosen: index === Math.min(panel.editor.selected, panel.editor.results.length - 1)
                        Layout.fillWidth: true
                        implicitHeight: cardRow.implicitHeight + 2 * Theme.s2
                        radius: Theme.rControl
                        color: hover.hovered ? Theme.rowHover : Theme.surface
                        border.color: chosen ? Theme.accent : Theme.border
                        border.width: chosen ? 2 : 1
                        Accessible.role: Accessible.Button
                        Accessible.name: isMatch
                            ? Fmt.fill(qsTr("{n}: {score}, {target}. {meta}"), { n: index + 1, score: Bands.scoreDescription(modelData.score, !!modelData.context), target: Fmt.plain(modelData.unit.target), meta: meta.text })
                            : Fmt.fill(qsTr("{n}: term {term}: {targets}"), { n: index + 1, term: (modelData.term || {}).text || "", targets: (modelData.targets || []).map(t => t.text).join(", ") })
                        HoverHandler { id: hover; cursorShape: Qt.PointingHandCursor }
                        TapHandler { onTapped: panel.editor.insertResult(card.index + 1) }
                        ToolTip { visible: hover.hovered && card.index < 9; text: "Ctrl+" + (card.index + 1); delay: 600 }
                        RowLayout {
                            id: cardRow
                            x: Theme.s3; y: Theme.s2
                            width: parent.width - 2 * Theme.s3
                            spacing: Theme.s2
                            Txt { Layout.alignment: Qt.AlignTop; Layout.minimumWidth: 18; text: card.index + 1; color: Theme.text3; font.pixelSize: Theme.tiny }
                            BandBadge {
                                visible: card.isMatch
                                Layout.alignment: Qt.AlignTop
                                Layout.minimumWidth: 44
                                band: card.isMatch ? Bands.ofScore(card.modelData.score, !!card.modelData.context) : "new"
                                text: card.isMatch ? Bands.scoreLabel(card.modelData.score, !!card.modelData.context) : ""
                                HoverHandler { id: bandHover }
                                ToolTip { visible: bandHover.hovered; text: card.isMatch ? Bands.scoreDescription(card.modelData.score, !!card.modelData.context) : "" }
                            }
                            Rectangle {
                                visible: !card.isMatch
                                Layout.alignment: Qt.AlignTop
                                implicitWidth: 44; implicitHeight: 20
                                radius: 4
                                color: Theme.accentWeak
                                Icon { anchors.centerIn: parent; name: "tag"; size: 13; color: Theme.accentText }
                                HoverHandler { id: termHover }
                                ToolTip { visible: termHover.hovered; text: qsTr("Term") }
                            }
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 2
                                Text {
                                    Layout.fillWidth: true
                                    wrapMode: Text.Wrap
                                    textFormat: card.isMatch ? Text.RichText : Text.PlainText
                                    color: Theme.text2
                                    font.family: Theme.font
                                    font.pixelSize: Theme.small
                                    text: card.isMatch ? "<style>" + Backend.segmentCss() + "</style>" + Backend.diffHtml(Fmt.plain((panel.editor.activeSegment || {}).source), Fmt.plain(card.modelData.unit.source))
                                                       : ((card.modelData.term || {}).text || "")
                                }
                                Text {
                                    Layout.fillWidth: true
                                    wrapMode: Text.Wrap
                                    textFormat: Text.RichText
                                    color: Theme.text
                                    font.family: Theme.font
                                    font.pixelSize: Theme.body
                                    font.weight: 550
                                    text: card.isMatch ? Backend.segmentHtml(card.modelData.unit.target)
                                        : (card.modelData.targets || []).map(t => {
                                              const forbidden = t.status === "forbidden"
                                              let item = t.text.replace(/&/g, "&amp;").replace(/</g, "&lt;")
                                              if (forbidden) item = "<span style=\"color:" + Theme.danger + "\">" + item + "</span>"
                                              if (t.status && t.status !== "approved")
                                                  item += " <span style=\"background:" + (forbidden ? Theme.dangerBg : Theme.warnBg) + "; color:" + (forbidden ? Theme.danger : Theme.warn) + "; font-size:11px; font-weight:600\">&nbsp;" + (forbidden ? qsTr("Forbidden") : qsTr("Proposed")) + "&nbsp;</span>"
                                              return item
                                          }).join(", ")
                                }
                                Txt {
                                    id: meta
                                    Layout.fillWidth: true
                                    color: Theme.text3
                                    font.pixelSize: Theme.tiny
                                    text: card.isMatch ? [card.modelData.memory, card.modelData.unit.changedBy || "", Fmt.date(card.modelData.unit.changedAt || "")].filter(x => x).join(", ")
                                                       : (card.modelData.source || "")
                                }
                            }
                        }
                    }
                }
                // Under the results: the translation services, and what one answered.
                Repeater {
                    model: panel.editor.activeId && panel.editor.editable() ? panel.editor.services : []
                    Button {
                        required property var modelData
                        small: true
                        iconName: "languages"
                        text: panel.editor.asking === modelData ? Fmt.fill(qsTr("Asking {name}…"), { name: modelData }) : Fmt.fill(qsTr("Suggest with {name}"), { name: modelData })
                        enabled: panel.editor.asking === ""
                        onClicked: panel.editor.askService(modelData)
                    }
                }
                Txt { visible: panel.editor.serviceProblem !== ""; Layout.fillWidth: true; role: "error"; text: panel.editor.serviceProblem }
                Box {
                    visible: !!panel.editor.suggestion
                    Txt { Layout.fillWidth: true; text: panel.editor.suggestion ? Fmt.plain(panel.editor.suggestion.target) : "" }
                    Txt {
                        Layout.fillWidth: true
                        role: "help"
                        text: !panel.editor.suggestion ? ""
                              : Fmt.fill(panel.editor.suggestion.tagsLost ? qsTr("From {name}, without the source's tags: put them in by hand.") : qsTr("From {name}"), { name: panel.editor.suggestion.name })
                    }
                    Button { small: true; text: qsTr("Insert"); onClicked: panel.editor.insertSuggestion() }
                }
            }

            // Checks.
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Theme.s2
                readonly property var shown: panel.editor.shownIssues
                Head { title: qsTr("Checks"); count: parent.shown.length }
                Txt {
                    visible: !!panel.editor.grammarService && panel.editor.grammar.error !== "" && panel.editor.grammar.id === panel.editor.activeId
                    Layout.fillWidth: true
                    role: "muted"
                    text: Fmt.fill(qsTr("Grammar is not checked: {error}"), { error: panel.editor.grammar.error })
                }
                Repeater {
                    model: parent.shown
                    Rectangle {
                        id: issue
                        required property var modelData
                        readonly property bool forbidden: modelData.kind === "term-forbidden"
                        readonly property int offset: modelData.offset || 0
                        Layout.fillWidth: true
                        implicitHeight: issueRow.implicitHeight + 2 * Theme.s2
                        radius: Theme.rControl
                        color: forbidden ? Theme.dangerBg : Theme.warnBg
                        RowLayout {
                            id: issueRow
                            x: Theme.s3; y: Theme.s2
                            width: parent.width - 2 * Theme.s3
                            spacing: Theme.s2
                            Icon { Layout.alignment: Qt.AlignTop; Layout.topMargin: 2; name: "alert-triangle"; size: 14; color: issue.forbidden ? Theme.danger : Theme.warn }
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: Theme.s1
                                Txt { Layout.fillWidth: true; text: issue.modelData.message }
                                Repeater {
                                    model: issue.modelData.kind === "grammar" && panel.editor.te && panel.editor.runOfStretch(panel.editor.te.content, issue.offset, issue.modelData.length || 0)
                                           ? (issue.modelData.replacements || []).slice(0, 3) : []
                                    Small {
                                        required property var modelData
                                        text: Fmt.fill(qsTr("Replace with “{value}”"), { value: modelData })
                                        Accessible.name: Fmt.fill(qsTr("Replace {found} with {value}"), { found: issue.modelData.found, value: modelData })
                                        onClicked: panel.editor.replaceFound(issue.offset, issue.modelData.length || 0, modelData)
                                    }
                                }
                            }
                            Small {
                                visible: issue.modelData.kind === "spelling" && !!issue.modelData.found
                                Layout.alignment: Qt.AlignTop
                                text: qsTr("Add to the project's words")
                                Accessible.name: Fmt.fill(qsTr("Add {word} to the project's words"), { word: issue.modelData.found || "" })
                                onClicked: panel.editor.addWord(issue.modelData.found)
                            }
                            Small {
                                Layout.alignment: Qt.AlignTop
                                text: qsTr("Ignore")
                                Accessible.name: Fmt.fill(qsTr("Ignore: {text}"), { text: issue.modelData.message })
                                onClicked: panel.editor.ignoreIssue(issue.modelData.message)
                            }
                        }
                    }
                }
                Txt {
                    visible: !parent.shown.length
                    Layout.fillWidth: true
                    role: "muted"
                    text: { panel.editor.revision; return panel.editor.te && panel.editor.te.plainText.trim() ? qsTr("No problems found.") : qsTr("Checks run as you type and again when you confirm.") }
                }
                Txt {
                    visible: panel.editor.ignored.length > 0
                    Layout.fillWidth: true
                    role: "help"
                    text: Fmt.fill(qsTr("{warnings} ignored for this segment."), { warnings: Fmt.plural(panel.editor.ignored.length, qsTr("{n} warning"), qsTr("{n} warnings")) })
                }
            }

            // Memory search, open while in use.
            ColumnLayout {
                id: lookupBox
                visible: panel.editor.lookupOpen
                Layout.fillWidth: true
                spacing: Theme.s2
                Head { title: qsTr("Memory search") }
                RowLayout {
                    spacing: Theme.s2
                    Icon { name: "search"; size: 14; color: Theme.text3 }
                    TextField {
                        id: lookupField
                        Layout.fillWidth: true
                        placeholderText: qsTr("Words in either language")
                        Accessible.name: qsTr("Search the memory")
                        onTextEdited: { panel.editor.lookupQuery = text; searchTimer.restart() }
                        Keys.onEscapePressed: panel.closeLookup()
                    }
                    IconButton { iconName: "x"; tip: qsTr("Close memory search"); onClicked: panel.closeLookup() }
                    Timer { id: searchTimer; interval: 250; onTriggered: panel.editor.searchMemory() }
                }
                Txt {
                    visible: panel.editor.lookupHits !== null && panel.editor.lookupHits.length === 0
                    Layout.fillWidth: true
                    role: "muted"
                    text: Fmt.fill(qsTr("Nothing in the memory contains “{query}”."), { query: panel.editor.lookupQuery.trim() })
                }
                Repeater {
                    model: panel.editor.lookupHits || []
                    ColumnLayout {
                        required property var modelData
                        readonly property string q: panel.editor.lookupQuery.trim()
                        readonly property string src: Fmt.plain(modelData.unit.source)
                        readonly property string tgt: Fmt.plain(modelData.unit.target)
                        Layout.fillWidth: true
                        Layout.bottomMargin: Theme.s1
                        spacing: 2
                        Accessible.name: Fmt.fill(qsTr("{source}, translated as {target}"), { source: src, target: tgt })
                        Txt { Layout.fillWidth: true; textFormat: Text.RichText; text: panel.highlight(parent.src, parent.q) }
                        Txt { Layout.fillWidth: true; textFormat: Text.RichText; font.weight: 550; text: panel.highlight(parent.tgt, parent.q) }
                        Txt { Layout.fillWidth: true; color: Theme.text3; font.pixelSize: Theme.tiny; text: [parent.modelData.memory, parent.modelData.unit.changedBy || ""].filter(x => x).join(", ") }
                    }
                }
            }

            // Scored review: the issues marked on the open segment, a form
            // for a new one, and the file's score.
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Theme.s2
                readonly property var list: panel.seg.issues || []
                Head { title: qsTr("Review"); count: parent.list.length }
                Txt { visible: !parent.list.length; Layout.fillWidth: true; role: "muted"; text: qsTr("No issues marked on this segment.") }
                Repeater {
                    model: parent.list
                    Box {
                        id: rev
                        required property var modelData
                        readonly property var sev: panel.editor.issueSeverities.find(x => x[0] === modelData.severity) || [modelData.severity, modelData.severity, 0]
                        readonly property string category: (panel.editor.issueCategories.find(x => x[0] === modelData.category) || [modelData.category, modelData.category])[1]
                        Txt {
                            Layout.fillWidth: true
                            textFormat: Text.StyledText
                            text: "<b>" + rev.category + "</b>, " + rev.sev[1].toLowerCase() + " (" + Fmt.plural(rev.sev[2], qsTr("{n} point"), qsTr("{n} points")) + ")"
                                  + (rev.modelData.note ? ": " + rev.modelData.note.replace(/&/g, "&amp;").replace(/</g, "&lt;") : "")
                        }
                        RowLayout {
                            spacing: Theme.s2
                            Txt { Layout.fillWidth: true; role: "help"; text: panel.byLine(rev.modelData) }
                            Button {
                                visible: panel.all || rev.modelData.by === panel.me
                                small: true; kind: "quiet"
                                text: qsTr("Delete")
                                Accessible.name: Fmt.fill(qsTr("Delete the issue {category}, {severity}"), { category: rev.category, severity: rev.sev[1].toLowerCase() })
                                onClicked: panel.editor.deleteIssue(rev.modelData.id)
                            }
                        }
                    }
                }
                RowLayout {
                    spacing: Theme.s2
                    Field {
                        Layout.fillWidth: true
                        Layout.preferredWidth: 1
                        label: qsTr("Kind of issue")
                        ComboBox { id: issueCategory; width: parent.width; model: panel.editor.issueCategories.map(c => c[1]) }
                    }
                    Field {
                        Layout.fillWidth: true
                        Layout.preferredWidth: 1
                        label: qsTr("Severity")
                        ComboBox {
                            id: issueSeverity
                            width: parent.width
                            model: panel.editor.issueSeverities.map(s => Fmt.fill(qsTr("{name} ({points})"), { name: s[1], points: Fmt.plural(s[2], qsTr("{n} point"), qsTr("{n} points")) }))
                        }
                    }
                }
                TextField {
                    id: issueNote
                    Layout.fillWidth: true
                    placeholderText: qsTr("What is wrong (optional)")
                    Accessible.name: qsTr("What is wrong, if you want to say")
                    maximumLength: 1000
                    onAccepted: markButton.clicked()
                }
                Button {
                    id: markButton
                    Layout.fillWidth: true
                    small: true
                    iconName: "flag"
                    text: qsTr("Mark issue")
                    enabled: panel.editor.activeId !== ""
                    onClicked: panel.editor.addIssue(panel.editor.issueCategories[issueCategory.currentIndex][0], panel.editor.issueSeverities[issueSeverity.currentIndex][0], issueNote.text)
                }
                Rectangle { visible: !!panel.editor.quality; Layout.fillWidth: true; Layout.topMargin: Theme.s2; height: 1; color: Theme.border }
                Txt {
                    visible: !!panel.editor.quality
                    Layout.fillWidth: true
                    textFormat: Text.StyledText
                    text: panel.editor.quality ? Fmt.fill(qsTr("Score of this file {score} {of}"), {
                        score: "<b><font size=\"5\">" + Fmt.dec(panel.editor.quality.score || 0, 1) + "</font></b>",
                        of: "<font color=\"" + Theme.text3 + "\">" + qsTr("of 100") + "</font>" }) : ""
                }
                Txt {
                    visible: !!panel.editor.quality
                    Layout.fillWidth: true
                    role: "help"
                    text: {
                        const q = panel.editor.quality
                        if (!q) return ""
                        const words = Fmt.plural(q.words || 0, qsTr("{n} word"), qsTr("{n} words"))
                        let t
                        if (!q.issues) t = Fmt.fill(qsTr("No issues marked in {words}."), { words: words })
                        else {
                            const parts = []
                            if (q.minor) parts.push(Fmt.fill(qsTr("{n} minor"), { n: q.minor }))
                            if (q.major) parts.push(Fmt.fill(qsTr("{n} major"), { n: q.major }))
                            if (q.critical) parts.push(Fmt.fill(qsTr("{n} critical"), { n: q.critical }))
                            t = Fmt.fill(qsTr("{issues} ({parts}), {penalty} in {words}."), { issues: Fmt.plural(q.issues, qsTr("{n} issue"), qsTr("{n} issues")), parts: parts.join(", "),
                                                                                              penalty: Fmt.plural(q.penalty || 0, qsTr("{n} penalty point"), qsTr("{n} penalty points")), words: words })
                        }
                        return t + " " + qsTr("The score is 100 less the penalty points in percent of the words.")
                    }
                }
                Txt {
                    textFormat: Text.StyledText
                    color: Theme.accentText
                    text: "<u>" + qsTr("Save the project's review as CSV") + "</u>"
                    Accessible.role: Accessible.Link
                    HoverHandler { cursorShape: Qt.PointingHandCursor }
                    TapHandler { onTapped: panel.editor.saveReview() }
                }
            }

            // Comments on the open segment, and a field for a new one.
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Theme.s2
                readonly property var list: panel.seg.comments || []
                Head { title: qsTr("Comments"); count: parent.list.length }
                Txt { visible: !parent.list.length; Layout.fillWidth: true; role: "muted"; text: qsTr("No comments on this segment.") }
                Repeater {
                    model: parent.list
                    Box {
                        id: comment
                        required property var modelData
                        Txt { Layout.fillWidth: true; text: comment.modelData.text }
                        RowLayout {
                            spacing: Theme.s2
                            Txt { Layout.fillWidth: true; role: "help"; text: panel.byLine(comment.modelData) }
                            Button {
                                visible: panel.all || comment.modelData.by === panel.me
                                small: true; kind: "quiet"
                                text: qsTr("Delete")
                                Accessible.name: Fmt.fill(qsTr("Delete the comment “{text}”"), { text: comment.modelData.text })
                                onClicked: panel.editor.deleteComment(comment.modelData.id)
                            }
                        }
                    }
                }
                TextArea {
                    id: commentText
                    Layout.fillWidth: true
                    minimumLines: 2
                    placeholderText: qsTr("A comment for those working on the project")
                    Accessible.name: qsTr("A comment on this segment")
                }
                Button {
                    small: true
                    iconName: "comment"
                    text: qsTr("Add comment")
                    enabled: commentText.text.trim() !== "" && panel.editor.activeId !== ""
                    onClicked: panel.editor.addComment(commentText.text)
                }
            }

            // Add a term: the intro and its buttons, or the form.
            ColumnLayout {
                id: captureBox
                Layout.fillWidth: true
                spacing: Theme.s2
                Head { title: qsTr("Add a term") }
                Txt { visible: !panel.editor.captureOpen; Layout.fillWidth: true; role: "muted"; text: qsTr("Select the term in the source and its translation in the target, then add it. The sentence is saved with it.") }
                Flow {
                    visible: !panel.editor.captureOpen
                    Layout.fillWidth: true
                    spacing: Theme.s2
                    Button { small: true; iconName: "plus"; text: qsTr("Add term"); keys: "Ctrl+E"; onClicked: panel.editor.openCapture() }
                    Button { small: true; iconName: "search"; text: qsTr("Search memory"); keys: "Ctrl+K"; onClicked: panel.editor.openLookup("") }
                }
                ColumnLayout {
                    visible: panel.editor.captureOpen
                    Layout.fillWidth: true
                    spacing: Theme.s2
                    Keys.onEscapePressed: panel.closeCapture()
                    Field {
                        Layout.fillWidth: true
                        label: Backend.languageName(panel.editor.sourceLang)
                        TextField { id: captureSourceField; width: parent.width; placeholderText: qsTr("Select it in the source, or type"); Accessible.name: Backend.languageName(panel.editor.sourceLang); onAccepted: addTermButton.clicked() }
                    }
                    Field {
                        Layout.fillWidth: true
                        label: Backend.languageName(panel.editor.targetLang)
                        TextField { id: captureTargetField; width: parent.width; placeholderText: qsTr("Select it in your translation, or type"); Accessible.name: Backend.languageName(panel.editor.targetLang); onAccepted: addTermButton.clicked() }
                    }
                    Field {
                        Layout.fillWidth: true
                        label: qsTr("Note (optional)")
                        TextField { id: captureNote; width: parent.width; onAccepted: addTermButton.clicked() }
                    }
                    RowLayout {
                        spacing: Theme.s2
                        Button {
                            id: addTermButton
                            small: true; kind: "primary"
                            text: qsTr("Add term")
                            enabled: captureSourceField.text.trim() !== "" && captureTargetField.text.trim() !== ""
                            onClicked: if (enabled) panel.editor.addTerm(captureSourceField.text.trim(), captureTargetField.text.trim(), captureNote.text.trim())
                        }
                        Button { small: true; kind: "quiet"; text: qsTr("Cancel"); onClicked: panel.closeCapture() }
                    }
                }
            }

            // Copy source, next tag, confirm.
            Flow {
                Layout.fillWidth: true
                spacing: Theme.s2
                Button { small: true; iconName: "copy"; text: qsTr("Copy source"); keys: "Ctrl+Ins"; onClicked: panel.editor.copySource() }
                Button { small: true; iconName: "tag"; text: qsTr("Next tag"); keys: "F9"; onClicked: panel.editor.insertNextTag() }
                Button { small: true; kind: "primary"; iconName: "check"; text: qsTr("Confirm"); keys: "Ctrl+Enter"; onClicked: panel.editor.confirm() }
            }

            // Split and join: in documents and in plain XLIFF files.
            ColumnLayout {
                visible: panel.editor.cut.shown
                Layout.fillWidth: true
                spacing: Theme.s2
                Flow {
                    Layout.fillWidth: true
                    spacing: Theme.s2
                    Button { small: true; iconName: "scissors"; text: qsTr("Split"); keys: "Alt+Shift+S"; enabled: panel.editor.cut.split; onClicked: panel.editor.openSplit() }
                    Button { small: true; iconName: "combine"; text: qsTr("Join with next"); keys: "Alt+Shift+J"; enabled: panel.editor.cut.join; onClicked: panel.editor.resegment("join") }
                }
                Txt { visible: panel.editor.cut.note !== ""; Layout.fillWidth: true; role: "help"; text: panel.editor.cut.note }
            }

            // Keyboard shortcuts, folded (.shortcuts).
            Button {
                id: keysToggle
                small: true; kind: "quiet"
                iconName: "keyboard"
                text: qsTr("Keyboard shortcuts")
                checkable: true
            }
            GridLayout {
                visible: keysToggle.checked
                Layout.fillWidth: true
                columns: 2
                columnSpacing: Theme.s3
                rowSpacing: Theme.s1
                Repeater {
                    model: [["Ctrl+Enter", qsTr("Confirm, fill repetitions and go to the next open segment")],
                            ["Alt+↓ / Alt+↑", qsTr("Next / previous segment")],
                            ["Ctrl+1 to 9", qsTr("Insert result 1 to 9 (a memory match replaces the target, a term goes in at the caret)")],
                            ["Ctrl+↓ / Ctrl+↑, Ctrl+Space", qsTr("Move through the results, insert the highlighted one")],
                            ["Ctrl+Insert", qsTr("Copy source to target")],
                            ["F9", qsTr("Insert the next missing tag")],
                            ["Ctrl+E", qsTr("Add a term from the selections")],
                            ["Ctrl+H", qsTr("Find and replace in the translations")],
                            ["Alt+Shift+S, Alt+Shift+J", qsTr("Split the segment at any place, join it with the next one of its paragraph")],
                            ["Ctrl+K or F3", qsTr("Search the memory for the selection")]]
                    delegate: Item {
                        required property var modelData
                        required property int index
                        Layout.columnSpan: 2
                        Layout.fillWidth: true
                        implicitHeight: Math.max(k.implicitHeight, w.implicitHeight)
                        Txt { id: k; width: 110; text: parent.modelData[0]; font.pixelSize: Theme.tiny; font.weight: 600; wrapMode: Text.Wrap }
                        Txt { id: w; x: 110 + Theme.s3; width: parent.width - x; role: "help"; text: parent.modelData[1] }
                    }
                }
                Txt { Layout.columnSpan: 2; Layout.fillWidth: true; role: "help"; text: qsTr("These are the shortcuts usual in translation tools.") }
            }
        }
    }
}
