import QtQuick
import QtQuick.Layouts
import Interlinea

// One segment of the editor (the web's .seg): its number and who else has
// it open, the source, the translation, and its state: the status, a
// lock, warnings, review issues, comments and where the translation came
// from. The open segment shows its source to select from and its
// translation to type in.
Rectangle {
    id: row
    property var editor             // the EditorPage
    property int index: -1          // in the editor's segments
    readonly property var s: editor.segmentAt(index, editor.revision)
    readonly property string sid: s.id || ""
    readonly property bool active: sid !== "" && sid === editor.activeId
    readonly property bool locked: !!s.locked
    readonly property var target: editor.targetOf(s, editor.revision)
    readonly property string status: s.status || "new"
    readonly property var presence: editor.presence[sid] || []
    readonly property var origin: s.origin || {}
    readonly property int warnings: (s.warnings || []).length
    readonly property int issues: (s.issues || []).length
    readonly property int comments: (s.comments || []).length
    width: ListView.view ? ListView.view.width : 0
    implicitHeight: Math.max(cells.implicitHeight, stateColumn.implicitHeight + 30, 48)
    color: active ? Theme.activeRow : Theme.surface
    Accessible.role: Accessible.Row
    Accessible.name: Fmt.fill(qsTr("Segment {n}"), { n: index + 1 })

    function statusName(st) { return st === "confirmed" ? qsTr("Confirmed") : st === "draft" ? qsTr("Draft") : qsTr("Not translated") }

    Rectangle { visible: row.active; width: 3; height: parent.height; color: Theme.accent }
    Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.rowLine }

    RowLayout {
        id: cells
        anchors.left: parent.left
        anchors.right: parent.right
        spacing: 0

        // The number, and who else is here.
        Item {
            Layout.preferredWidth: 44
            Layout.fillHeight: true
            implicitHeight: numberColumn.implicitHeight + 15
            Column {
                id: numberColumn
                anchors.right: parent.right
                anchors.rightMargin: Theme.s3
                y: 15
                spacing: 4
                Text {
                    anchors.right: parent.right
                    text: row.index + 1
                    color: row.active ? Theme.accentText : Theme.text3
                    font.family: Theme.font
                    font.pixelSize: Theme.tiny
                    font.weight: row.active ? 600 : 400
                }
                Repeater {
                    model: row.presence
                    Rectangle {
                        required property string modelData
                        width: 20; height: 20; radius: 10
                        color: Theme.serverBg
                        Accessible.name: Fmt.fill(qsTr("Open by {name}"), { name: modelData })
                        Text {
                            anchors.centerIn: parent
                            text: modelData.split(" ").filter(x => x).slice(0, 2).map(x => x[0].toUpperCase()).join("")
                            color: Theme.server
                            font.family: Theme.font
                            font.pixelSize: 9
                            font.weight: 600
                        }
                    }
                }
            }
        }

        // The source: to read, or, when open, to select a term from.
        Item {
            Layout.fillWidth: true
            Layout.preferredWidth: 1
            Layout.fillHeight: true
            implicitHeight: (row.active ? sourceEdit.implicitHeight : sourceText.implicitHeight) + 2 * Theme.s3
            Text {
                id: sourceText
                visible: !row.active
                x: Theme.s3; y: Theme.s3
                width: parent.width - 2 * Theme.s3
                textFormat: Text.RichText
                wrapMode: Text.Wrap
                color: Theme.text
                font.family: Theme.font
                font.pixelSize: 15
                horizontalAlignment: Backend.rtl(row.editor.sourceLang) ? Text.AlignRight : Text.AlignLeft
                text: row.active ? "" : Backend.segmentHtml(row.s.source || [], [])
                Accessible.name: Fmt.fill(qsTr("Source of segment {n}"), { n: row.index + 1 })
            }
            TextEdit {
                id: sourceEdit
                visible: row.active
                x: Theme.s3; y: Theme.s3
                width: parent.width - 2 * Theme.s3
                readOnly: true
                selectByMouse: true
                persistentSelection: true
                textFormat: TextEdit.RichText
                wrapMode: TextEdit.Wrap
                color: Theme.text
                selectionColor: Theme.selection
                selectedTextColor: Theme.text
                font.family: Theme.font
                font.pixelSize: 15
                horizontalAlignment: Backend.rtl(row.editor.sourceLang) ? TextEdit.AlignRight : TextEdit.AlignLeft
                text: row.active ? Backend.segmentHtml(row.s.source || [], row.editor.marks) : ""
                activeFocusOnTab: false
                Accessible.name: Fmt.fill(qsTr("Source of segment {n}"), { n: row.index + 1 })
                onSelectedTextChanged: if (row.active) row.editor.sourceSelection = selectedText.replace(/￼/g, "").trim()
            }
            Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Theme.rowLine }
            TapHandler { enabled: !row.active; onTapped: row.editor.goToSegment(row.sid) }
        }

        // The translation: to read, or, when open, to type in.
        Item {
            id: targetCell
            Layout.fillWidth: true
            Layout.preferredWidth: 1
            Layout.fillHeight: true
            implicitHeight: (row.active && !row.locked ? editBox.implicitHeight : targetText.implicitHeight + 2 * Theme.s3 + (lockedNote.visible ? lockedNote.implicitHeight : 0))
            Text {
                id: targetText
                visible: !(row.active && !row.locked)
                x: Theme.s3; y: Theme.s3
                width: parent.width - 2 * Theme.s3
                textFormat: Text.RichText
                wrapMode: Text.Wrap
                color: Theme.text
                font.family: Theme.font
                font.pixelSize: 15
                horizontalAlignment: Backend.rtl(row.editor.targetLang) ? Text.AlignRight : Text.AlignLeft
                text: row.target.length ? Backend.segmentHtml(row.target, []) : "<style>" + Backend.segmentCss() + "</style><span class=\"missing\">" + qsTr("Not translated") + "</span>"
                Accessible.name: row.target.length ? Fmt.plain(row.target) : qsTr("Not translated")
            }
            Txt {
                id: lockedNote
                visible: row.locked
                anchors.top: targetText.bottom
                x: Theme.s3
                role: "sub"
                font.pixelSize: Theme.tiny
                text: qsTr("Locked in the file")
            }
            TapHandler { enabled: !row.active; onTapped: row.editor.goToSegment(row.sid) }
            // .target-editor
            Rectangle {
                id: editBox
                visible: row.active && !row.locked
                anchors.fill: parent
                implicitHeight: editorLoader.implicitHeight + 2 * Theme.s3 + 2 + counters.implicitHeight
                color: Theme.surface
                border.width: 2
                border.color: Theme.accent
                Loader {
                    id: editorLoader
                    active: row.active && !row.locked
                    x: Theme.s3; y: Theme.s3
                    width: parent.width - 2 * Theme.s3
                    sourceComponent: TargetEdit {
                        objectName: "targetEdit"
                        width: editorLoader.width
                        language: row.editor.targetLang
                        completions: row.editor.completions
                        misspellings: row.editor.misspellings
                        Accessible.name: Fmt.fill(qsTr("Translation of segment {n}, {language}"), { n: row.index + 1, language: Backend.languageName(row.editor.targetLang) })
                        Accessible.description: qsTr("Control Enter confirms and moves on. Control and a number inserts a numbered result. Control E adds a term. Control K searches the memory. F9 inserts the next tag.")
                        Component.onCompleted: row.editor.attach(this)
                        Component.onDestruction: row.editor.detach(this)
                        onEdited: row.editor.typed()
                        onAddWord: word => row.editor.addWord(word)
                    }
                }
                Column {
                    id: counters
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: Theme.s3
                    anchors.rightMargin: Theme.s3
                    anchors.bottomMargin: Theme.s1
                    Txt {
                        visible: (row.s.maxLength || 0) > 0
                        width: parent.width
                        font.pixelSize: Theme.tiny
                        color: row.editor.length > (row.s.maxLength || 0) ? Theme.danger : Theme.text3
                        text: row.editor.length > row.s.maxLength
                              ? Fmt.fill(qsTr("{length} of {max} characters, {over} too many"), { length: row.editor.length, max: row.s.maxLength, over: row.editor.length - row.s.maxLength })
                              : Fmt.fill(qsTr("{length} of {max} characters"), { length: row.editor.length, max: row.s.maxLength || 0 })
                    }
                    Txt {
                        // A part of a split subtitle has the time of the whole, not its own.
                        visible: !!row.s.timing && !row.s.split
                        width: parent.width
                        textFormat: Text.StyledText
                        font.pixelSize: Theme.tiny
                        color: Theme.text3
                        text: visible ? row.editor.subtitleCounter() : ""
                    }
                }
            }
        }

        // .seg-status: the status icon, then lock, warnings, issues,
        // comments and where the translation came from.
        Item {
            Layout.preferredWidth: 56
            Layout.fillHeight: true
            Accessible.role: Accessible.StaticText
            Accessible.name: {
                const parts = [row.statusName(row.status)]
                if (row.locked) parts.push(qsTr("Locked"))
                if (row.warnings) parts.push(Fmt.plural(row.warnings, qsTr("{n} warning"), qsTr("{n} warnings")))
                if (row.issues) parts.push(Fmt.plural(row.issues, qsTr("1 issue marked in review"), qsTr("{n} issues marked in review")))
                if (row.comments) parts.push(Fmt.plural(row.comments, qsTr("{n} comment"), qsTr("{n} comments")))
                if (row.editor.pending[row.sid] !== undefined) parts.push(qsTr("Not saved"))
                if (row.presence.length) parts.push(Fmt.fill(qsTr("Open by {names}"), { names: row.presence.join(", ") }))
                return parts.join(", ")
            }
            Column {
                id: stateColumn
                anchors.horizontalCenter: parent.horizontalCenter
                y: 15
                spacing: 2
                Icon {
                    anchors.horizontalCenter: parent.horizontalCenter
                    name: row.status === "confirmed" ? "check" : row.status === "draft" ? "pencil-line" : "circle-dashed"
                    color: row.status === "confirmed" ? Theme.ok : row.status === "draft" ? Theme.warn : Theme.text3
                    size: 16
                }
                Icon { visible: row.locked; anchors.horizontalCenter: parent.horizontalCenter; name: "lock"; color: Theme.text3; size: 13 }
                Repeater {
                    model: [{ n: row.warnings, icon: "alert-triangle", color: Theme.warn }, { n: row.issues, icon: "flag", color: Theme.warn }, { n: row.comments, icon: "comment", color: Theme.text2 }]
                    Row {
                        required property var modelData
                        visible: modelData.n > 0
                        anchors.horizontalCenter: parent.horizontalCenter
                        spacing: 2
                        Icon { name: modelData.icon; color: modelData.color; size: 13; anchors.verticalCenter: parent.verticalCenter }
                        Text { text: modelData.n; color: modelData.color; font.family: Theme.font; font.pixelSize: 11; font.weight: 600 }
                    }
                }
                BandBadge {
                    visible: row.origin.kind === "tm"
                    anchors.horizontalCenter: parent.horizontalCenter
                    band: Bands.ofScore(row.origin.score || 0, !!row.origin.context)
                    text: Bands.scoreLabel(row.origin.score || 0, !!row.origin.context)
                }
                Icon { visible: row.origin.kind === "repetition"; anchors.horizontalCenter: parent.horizontalCenter; name: "repeat"; color: Theme.text3; size: 13 }
            }
            ToolTip {
                visible: stateHover.hovered && text !== ""
                text: (row.s.warnings || []).map(w => w.text).concat((row.s.comments || []).map(c => c.by + ": " + c.text), row.s.changedBy ? [Fmt.fill(qsTr("Last saved by {name}"), { name: row.s.changedBy })] : []).join("\n")
            }
            HoverHandler { id: stateHover }
            TapHandler { enabled: !row.active; onTapped: row.editor.goToSegment(row.sid) }
        }
    }
}
