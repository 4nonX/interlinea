pragma Singleton
import QtQuick
import Interlinea

// The match bands (web/src/bands.ts): which band a score is in, its short
// name, what it means, and its colours.
QtObject {
    function ofScore(score, context) {
        if (context && score === 100) return "cm"
        if (score >= 100) return "100"
        if (score >= 95) return "95"
        if (score >= 85) return "85"
        if (score >= 75) return "75"
        if (score >= 50) return "50"
        return "new"
    }
    function scoreLabel(score, context) { return context && score === 100 ? "CM" : score + "%" }
    function scoreDescription(score, context) {
        if (context && score === 100) return qsTr("Context match (101%): the text and the segment before it match")
        if (score === 100) return qsTr("Exact match (100%)")
        return Fmt.fill(qsTr("Fuzzy match, {score}%"), { score: score })
    }
    function shortName(band) {
        return ({ locked: "Lock", context: "CM", repetition: "Rep", "cross-file": "XRep", "100": "100%", "95": "95+", "85": "85+", "75": "75+", "50": "50+", "new": "New" })[band] || band
    }
    function hint(band) {
        return ({
            locked: qsTr("Locked in the file; not translated"),
            context: qsTr("100% match whose preceding segment also matches (101%)"),
            repetition: qsTr("Same text and tags earlier in the same file"),
            "cross-file": qsTr("Same text and tags in an earlier file of the project"),
            "100": qsTr("Same text and tags as a memory unit"),
            "50": qsTr("Usually charged like new text"),
            "new": qsTr("No match of 50% or more")
        })[band] || ""
    }
    // { background, text, border }
    function colors(band) {
        const dark = Theme.dark
        if (band === "cm" || band === "100" || band === "context") return { background: Theme.okBg, text: Theme.ok, border: "transparent" }
        if (band === "95") return { background: dark ? "#2e3517" : "#edf3d6", text: dark ? "#c7d87a" : "#4b5e0c", border: "transparent" }
        if (band === "85") return { background: Theme.warnBg, text: Theme.warn, border: "transparent" }
        if (band === "75") return { background: dark ? "#3d2817" : "#fde8d6", text: dark ? "#f2a86b" : "#8a3f00", border: "transparent" }
        if (band === "rep" || band === "repetition" || band === "cross-file") return { background: Theme.serverBg, text: Theme.server, border: "transparent" }
        if (band === "locked") return { background: "transparent", text: Theme.text3, border: Theme.borderStrong }
        return { background: dark ? "#2a3038" : "#eef0f3", text: Theme.text2, border: "transparent" }
    }
}
