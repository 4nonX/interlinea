import QtQuick
import QtQuick.Layouts
import QtQuick.Controls.Basic as B
import Interlinea

// The other lists of the opened termbase: entries that share a term, to
// merge; the deleted entries, to bring back; and under a project termbase
// the master's entries it uses, with those left out.
B.ScrollView {
    id: views
    property var page
    clip: true
    contentWidth: availableWidth
    B.ScrollBar.vertical: ScrollBar { parent: views; x: views.width - width; height: views.height }

    function said(e) {
        return TermData.languages(e).map(l => ({ l: l, ts: TermData.terms(e, l).map(t => t.text) })).filter(x => x.ts.length)
    }

    ColumnLayout {
        width: views.availableWidth
        spacing: 0

        // Duplicates.
        ColumnLayout {
            Layout.fillWidth: true
            visible: page.view === 1
            spacing: 0
            RowLayout {
                Layout.fillWidth: true
                Layout.margins: Theme.s2
                Layout.leftMargin: Theme.s3
                spacing: Theme.s2
                Txt { text: qsTr("Shared term in"); role: "muted"; font.pixelSize: Theme.small; wrapMode: Text.NoWrap }
                ComboBox {
                    Layout.fillWidth: true
                    model: [qsTr("Any language")].concat(page.languages.map(l => Backend.languageName(l)))
                    currentIndex: Math.max(0, page.languages.indexOf(page.dupLang) + 1)
                    Accessible.name: qsTr("Shared term in")
                    onActivated: i => { page.dupLang = i ? page.languages[i - 1] : ""; page.loadDuplicates() }
                }
            }
            Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.margins: Theme.s3
                spacing: Theme.s2
                Txt { visible: !!page.duplicates && !!page.duplicates.error; role: "error"; text: page.duplicates && page.duplicates.error ? page.duplicates.error : "" }
                Repeater {
                    model: Array.isArray(page.duplicates) ? page.duplicates : []
                    Rectangle {
                        id: group
                        required property var modelData
                        Layout.fillWidth: true
                        implicitHeight: gv.implicitHeight + 2 * Theme.s3
                        radius: Theme.rControl
                        border.color: Theme.border
                        color: Theme.surface
                        ColumnLayout {
                            id: gv
                            x: Theme.s3; y: Theme.s3
                            width: parent.width - 2 * Theme.s3
                            spacing: Theme.s2
                            Repeater {
                                model: group.modelData.entries
                                B.AbstractButton {
                                    id: dup
                                    required property var modelData
                                    Layout.fillWidth: true
                                    implicitHeight: flow.implicitHeight + 12
                                    hoverEnabled: true
                                    Accessible.name: views.said(modelData).map(x => Backend.languageName(x.l) + ": " + x.ts.join(", ")).join("; ")
                                    onClicked: page.selected = modelData.id
                                    background: Rectangle { radius: 4; color: page.selected === dup.modelData.id ? Theme.accentWeak : dup.hovered ? Theme.surface2 : "transparent" }
                                    contentItem: Flow {
                                        id: flow
                                        padding: 6
                                        spacing: Theme.s2
                                        Repeater {
                                            model: views.said(dup.modelData)
                                            Row {
                                                required property var modelData
                                                spacing: 6
                                                LangChip { code: modelData.l }
                                                Txt { text: modelData.ts.join(", "); wrapMode: Text.NoWrap }
                                            }
                                        }
                                    }
                                }
                            }
                            Row {
                                spacing: Theme.s2
                                Button { small: true; text: qsTr("Merge into one entry"); onClicked: page.mergeGroup(group.modelData) }
                                Button {
                                    small: true; kind: "quiet"
                                    text: qsTr("Not duplicates")
                                    onClicked: { page.dismissed = page.dismissed.concat([group.modelData.key]); page.loadDuplicates() }
                                }
                            }
                        }
                    }
                }
                Txt { visible: Array.isArray(page.duplicates) && page.duplicates.length === 0; role: "h3"; text: qsTr("No duplicates") }
                Txt {
                    Layout.fillWidth: true
                    visible: Array.isArray(page.duplicates) && page.duplicates.length === 0
                    role: "muted"
                    text: page.dupLang ? Fmt.fill(qsTr("No two entries share a term in {language}."), { language: Backend.languageName(page.dupLang) }) : qsTr("No two entries share a term.")
                }
            }
        }

        // Deleted entries.
        ColumnLayout {
            Layout.fillWidth: true
            Layout.margins: Theme.s3
            visible: page.view === 2
            spacing: Theme.s2
            Txt { visible: !!page.deleted && !!page.deleted.error; role: "error"; text: page.deleted && page.deleted.error ? page.deleted.error : "" }
            Txt { visible: Array.isArray(page.deleted) && page.deleted.length === 0; role: "h3"; text: qsTr("Nothing deleted") }
            Txt { Layout.fillWidth: true; visible: Array.isArray(page.deleted) && page.deleted.length === 0; role: "muted"; text: qsTr("Entries you delete are kept here and can be brought back.") }
            Repeater {
                model: Array.isArray(page.deleted) ? page.deleted : []
                Rectangle {
                    id: gone
                    required property var modelData
                    readonly property string meta: {
                        const when = Fmt.time(modelData.at)
                        let m = modelData.by ? Fmt.fill(qsTr("Deleted {when} by {who}"), { when: when, who: modelData.by }) : Fmt.fill(qsTr("Deleted {when}"), { when: when })
                        // What was written down as the reason, if anything was.
                        if (modelData.summary && modelData.summary !== "Deleted") m += ". " + modelData.summary
                        return m
                    }
                    Layout.fillWidth: true
                    implicitHeight: dv.implicitHeight + 2 * Theme.s3
                    radius: Theme.rControl
                    border.color: Theme.border
                    color: Theme.surface
                    Accessible.name: views.said(modelData.entry).map(x => Backend.languageName(x.l) + ": " + x.ts.join(", ")).join("; ") + ". " + meta
                    ColumnLayout {
                        id: dv
                        x: Theme.s3; y: Theme.s3
                        width: parent.width - 2 * Theme.s3
                        spacing: Theme.s1
                        Repeater {
                            model: views.said(gone.modelData.entry)
                            RowLayout {
                                required property var modelData
                                spacing: 6
                                LangChip { code: modelData.l }
                                Txt { Layout.fillWidth: true; text: modelData.ts.join(", ") }
                            }
                        }
                        Txt { Layout.fillWidth: true; role: "sub"; text: gone.meta }
                        Button { visible: page.may("edit"); small: true; text: qsTr("Bring back"); onClicked: page.bringBack(gone.modelData.entryId) }
                    }
                }
            }
        }

        // The master's entries under a project termbase.
        ColumnLayout {
            Layout.fillWidth: true
            visible: page.view === 3
            spacing: 0
            Txt {
                Layout.fillWidth: true
                Layout.margins: Theme.s3
                role: "help"
                text: page.may("edit") ? qsTr("The master's entries this project uses as they are. Change one for this project only, or leave it out.") : qsTr("The master's entries this project uses as they are.")
            }
            Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
            Txt { Layout.fillWidth: true; Layout.margins: Theme.s3; visible: page.masterEntries.length === 0; role: "muted"; text: qsTr("The masters have no entries the project does not have itself.") }
            Repeater {
                model: page.masterEntries
                B.ItemDelegate {
                    id: me
                    required property var modelData
                    readonly property string key: modelData.termbase + "\n" + modelData.id
                    readonly property var parts: page.languages.map(l => TermData.terms(modelData, l).map(t => t.text).join(", ")).filter(x => x)
                    Layout.fillWidth: true
                    hoverEnabled: true
                    padding: 0
                    Accessible.name: Fmt.fill(qsTr("{terms}, in {master}"), { terms: parts.join("; "), master: modelData.termbase })
                    onClicked: page.masterSelected = key
                    background: Rectangle {
                        color: page.masterSelected === me.key ? Theme.accentWeak : me.hovered ? Theme.surface2 : "transparent"
                        Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Theme.border }
                    }
                    contentItem: ColumnLayout {
                        spacing: 2
                        EntryTerms { Layout.fillWidth: true; Layout.leftMargin: Theme.s4; Layout.rightMargin: Theme.s4; Layout.topMargin: Theme.s3; entry: me.modelData; languages: page.languages }
                        Txt { Layout.leftMargin: Theme.s4; Layout.bottomMargin: Theme.s3; text: Fmt.fill(qsTr("In {master}"), { master: me.modelData.termbase }); role: "sub" }
                    }
                }
            }
            // What was left out of the project, to bring back.
            ColumnLayout {
                Layout.fillWidth: true
                Layout.margins: Theme.s3
                visible: page.removals.length > 0
                spacing: Theme.s2
                Txt { text: qsTr("Left out of this project"); font.weight: 600 }
                Txt { Layout.fillWidth: true; role: "sub"; text: qsTr("Merging asks the master's reviewers to delete these there.") }
                Repeater {
                    model: page.removals
                    ColumnLayout {
                        id: rem
                        required property var modelData
                        property string head: modelData.entryId
                        spacing: 2
                        // The entry is named by its terms, as the master has it.
                        Component.onCompleted: Api.get(page.cid, "/api/termbases/" + Api.seg(modelData.termbase) + "/entries/" + Api.seg(modelData.entryId), r => {
                            if (r.ok) { const h = TermData.head(r.data, page.languages[0] || "").text; if (h) rem.head = h }
                        })
                        Txt { text: rem.head; font.weight: 600 }
                        Txt {
                            role: "sub"
                            text: rem.modelData.by ? Fmt.fill(qsTr("In {master}, left out {when} by {who}"), { master: rem.modelData.termbase, when: Fmt.time(rem.modelData.at), who: rem.modelData.by })
                                                   : Fmt.fill(qsTr("In {master}, left out {when}"), { master: rem.modelData.termbase, when: Fmt.time(rem.modelData.at) })
                        }
                        Button {
                            visible: page.may("edit")
                            small: true
                            text: qsTr("Bring back")
                            onClicked: Api.call(page.cid, "DELETE", page.path("/removals"), undefined, r => {
                                if (!r.ok) { page.notice(r.error.message); return }
                                page.notice(qsTr("Brought the entry back into the project"))
                                page.reload()
                            }, { master: rem.modelData.termbase, entry: rem.modelData.entryId })
                        }
                    }
                }
            }
        }
    }
}
