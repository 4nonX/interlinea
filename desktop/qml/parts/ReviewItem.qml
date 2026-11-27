import QtQuick
import QtQuick.Layouts
import Interlinea

// One entry of a merge request (the web's .mr-item): what kind of change,
// by whom, its terms in each language, what the project changed, where the
// master changed the same places, and the decision on it.
Panel {
    id: card
    property var review          // the ReviewPage
    property var item: ({})
    readonly property int itemId: item.id || 0
    readonly property string decided: item.state || ""
    readonly property string change: item.kind || ""
    readonly property var entry: item.entry || {}
    readonly property var conflicts: item.conflicts || []
    readonly property var merged: item.merged || null
    readonly property bool hasMerged: !!merged && Object.keys(merged).length > 0
    readonly property bool waits: decided === "waiting" || decided === "postponed"
    readonly property bool deciding: review.isOpen && waits && review.may("approve")
    readonly property string kindText: change === "new" ? qsTr("New entry") : change === "changed" ? qsTr("Changed entry") : qsTr("Deleted in the project")
    readonly property string nothing: qsTr("nothing")
    spacing: Theme.s2
    Accessible.name: [kindText].concat(TermData.languages(entry).map(l => Backend.languageName(l) + ": " + TermData.terms(entry, l).map(t => t.text).join("; "))).join(". ")

    // Each way out of a conflict names what it puts into the master: the
    // value itself for one conflict, how many places for several.
    function result(side) {
        if (conflicts.length !== 1) return Fmt.fill(qsTr("{n} places"), { n: conflicts.length })
        const v = conflicts[0][side] || ""
        return "“" + (v || nothing) + "”"
    }

    // .mr-head
    RowLayout {
        Layout.fillWidth: true
        spacing: Theme.s3
        Txt { role: "strong"; text: card.kindText; wrapMode: Text.NoWrap }
        Txt {
            Layout.fillWidth: true
            role: "sub"
            text: {
                let meta = Fmt.fill(qsTr("by {author}, {date}"), { author: card.item.author || "", date: card.review.day(card.item.changedAt) })
                const who = card.item.reviewers || []
                if (who.length && card.waits) meta += Fmt.fill(qsTr("; for {reviewers} to review"), { reviewers: who.join(", ") })
                return meta
            }
        }
        Badge {
            text: card.review.stateText(card.decided)
            kind: card.decided === "waiting" ? "warn" : card.decided.startsWith("approved") ? "ok" : "danger"
        }
    }

    // .mr-terms
    Repeater {
        model: TermData.languages(card.entry)
        RowLayout {
            required property string modelData
            Layout.fillWidth: true
            spacing: Theme.s2
            LangChip { code: modelData; Layout.alignment: Qt.AlignTop }
            Txt {
                Layout.fillWidth: true
                textFormat: Text.PlainText
                text: TermData.terms(card.entry, modelData).map(t => t.text).join("; ")
                horizontalAlignment: Backend.rtl(modelData) ? Text.AlignRight : Text.AlignLeft
            }
        }
    }

    // .mr-changes: what the project changed.
    Repeater {
        model: card.change === "changed" ? (card.item.changes || []) : []
        Txt {
            required property var modelData
            readonly property string before: modelData.old || card.nothing
            readonly property string after: modelData.new || card.nothing
            Layout.fillWidth: true
            Layout.leftMargin: Theme.s4
            textFormat: Text.StyledText
            function esc(t) { return t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;") }
            text: "<font color=\"" + Theme.text3 + "\">" + esc(modelData.path || "") + ":</font> <s><font color=\"" + Theme.text3 + "\">" + esc(before)
                  + "</font></s> → <b><font color=\"" + Theme.ok + "\">" + esc(after) + "</font></b>"
            Accessible.name: Fmt.fill(qsTr("{field}: {before} becomes {after}"), { field: modelData.path || "", before: before, after: after })
        }
    }

    // .notice: where the master changed the same places.
    Rectangle {
        visible: card.conflicts.length > 0
        Layout.fillWidth: true
        implicitHeight: conflictBody.implicitHeight + 2 * Theme.s3
        radius: Theme.rControl
        color: Theme.accentWeak
        Accessible.role: Accessible.StaticText
        Accessible.name: qsTr("The master changed the same places")
        RowLayout {
            id: conflictBody
            x: Theme.s4; y: Theme.s3
            width: parent.width - 2 * Theme.s4
            spacing: Theme.s3
            Icon { Layout.alignment: Qt.AlignTop; Layout.topMargin: 2; name: "alert-triangle"; color: Theme.accentText; size: 16 }
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Theme.s1
                Txt { role: "strong"; text: qsTr("The master changed the same places") }
                Repeater {
                    model: card.conflicts
                    Txt {
                        required property var modelData
                        Layout.fillWidth: true
                        textFormat: Text.PlainText
                        text: Fmt.fill(qsTr("{field}: the project has “{ours}”, {termbase} has “{theirs}”"),
                                       { field: modelData.path || "", ours: modelData.ours || card.nothing, termbase: card.review.targetName, theirs: modelData.theirs || card.nothing })
                              + (modelData.base ? Fmt.fill(qsTr(" (was “{base}”)"), { base: modelData.base }) : "")
                    }
                }
                Txt {
                    visible: card.deciding && card.hasMerged
                    Layout.fillWidth: true
                    role: "sub"
                    text: Fmt.fill(qsTr("Choose which of the two goes into {termbase}. What only one side changed is merged either way."), { termbase: card.review.targetName })
                }
            }
        }
    }

    Txt { visible: !!card.item.forbidden; Layout.fillWidth: true; color: Theme.warn; font.pixelSize: Theme.small; text: qsTr("It has a forbidden term.") }
    Txt {
        readonly property int n: (card.item.duplicates || []).length
        visible: n > 0
        Layout.fillWidth: true
        color: Theme.warn
        font.pixelSize: Theme.small
        text: n === 1 ? qsTr("1 master entry shares a term with it.") : Fmt.fill(qsTr("{n} master entries share a term with it."), { n: n })
    }
    Txt {
        visible: !!card.item.comment
        Layout.fillWidth: true
        role: "sub"
        text: card.item.reviewer ? Fmt.fill(qsTr("“{comment}”, {reviewer}"), { comment: card.item.comment || "", reviewer: card.item.reviewer })
                                 : Fmt.fill(qsTr("“{comment}”"), { comment: card.item.comment || "" })
    }

    // What becomes of an entry that did not go into the master.
    Txt {
        visible: card.decided === "reverted" || card.decided === "rejected" || card.decided === "changes-requested"
        Layout.fillWidth: true
        role: "sub"
        text: card.decided === "reverted" ? Fmt.fill(qsTr("Taken out of {termbase} again. It is submitted again once the project has changed it."), { termbase: card.review.targetName })
                                        : Fmt.fill(qsTr("Not in {termbase}. It is submitted again once the project has changed it."), { termbase: card.review.targetName })
    }
    // What was merged can be taken out of the master again.
    ConfirmButton {
        visible: card.review.isOpen && card.review.may("approve") && (card.decided === "approved" || card.decided === "approved-with-edits") && card.item.rule !== "identical"
        text: qsTr("Undo the merge")
        action: qsTr("Undo")
        question: Fmt.fill(qsTr("Take this out of {termbase} again? The entry there goes back to what it was before."), { termbase: card.review.targetName })
        onConfirmed: {
            const tname = card.review.targetName, rv = card.review
            Api.post(rv.cid, rv.base + "/items/" + card.itemId + "/revert", undefined, res => {
                if (res.ok) rv.notice(Fmt.fill(qsTr("Took the merge back out of {termbase}"), { termbase: tname }))
                else if (res.status === 409) rv.notice(Fmt.fill(qsTr("The entry in {termbase} was changed after the merge. Change it there by hand."), { termbase: tname }))
                else rv.notice(res.error.message)
                rv.reload()
            })
        }
    }

    // What the author is told with the decision.
    TextField {
        visible: card.deciding
        Layout.fillWidth: true
        Layout.maximumWidth: 520
        text: card.review.comments[card.itemId] || ""
        placeholderText: qsTr("A note for the author, with your decision")
        Accessible.name: qsTr("A note for the author")
        onTextChanged: if (text !== (card.review.comments[card.itemId] || "")) card.review.comments = Object.assign({}, card.review.comments, { [card.itemId]: text })
    }
    // .server-actions
    Flow {
        visible: card.deciding
        Layout.fillWidth: true
        spacing: Theme.s2
        readonly property var only: ({ ids: [card.itemId] })
        readonly property bool choose: card.conflicts.length > 0 && card.hasMerged
        Button {
            visible: parent.choose
            small: true
            iconName: "check"
            text: Words.get("useProject") + ": " + card.result("ours")
            onClicked: {
                const ours = card.item.mergedOurs && Object.keys(card.item.mergedOurs).length ? card.item.mergedOurs : card.entry
                card.review.decide(parent.only, "approved-with-edits", ours, Fmt.fill(qsTr("Used the project version ({value})"), { value: card.result("ours") }))
            }
        }
        Button {
            visible: parent.choose
            small: true
            iconName: "check"
            text: Words.get("keepMaster") + ": " + card.result("theirs")
            onClicked: card.review.decide(parent.only, "approved-with-edits", card.merged, Fmt.fill(qsTr("Kept the master version ({value})"), { value: card.result("theirs") }))
        }
        Button {
            visible: !parent.choose
            kind: "primary"
            small: true
            iconName: "check"
            text: qsTr("Approve")
            onClicked: card.review.decide(parent.only, "approved", null, "")
        }
        // What was postponed is decided now, not put off again.
        Button {
            visible: card.decided !== "postponed"
            small: true
            iconName: "clock"
            text: qsTr("Postpone")
            onClicked: card.review.decide(parent.only, "postponed", null, "")
        }
        Button { small: true; iconName: "x"; text: qsTr("Reject"); onClicked: card.review.decide(parent.only, "rejected", null, "") }
    }
}
