pragma Singleton
import QtQuick
import Interlinea

// Texts as the web app writes them (web/src/format.ts, i18n.ts): a
// translated text's {placeholders} filled, numbers, dates, counts and
// lists in the interface language. Texts are translated where they are
// written, with qsTr, so the catalog check finds them.
QtObject {
    // Fills {name} in a translated text.
    function fill(s, vars) {
        if (!vars) return s
        return s.replace(/\{([A-Za-z0-9_]+)\}/g, (m, k) => k in vars ? String(vars[k]) : m)
    }
    function num(n) { return Backend.number(n, 0) }
    function dec(n, digits) { return Backend.number(n, digits) }
    function date(iso) { return iso ? Backend.date(iso) : "" }
    function time(iso) { return iso ? Backend.time(iso) : "" }
    // A count with its noun: one and many are the translated "{n} file"
    // and "{n} files".
    function plural(n, one, many) { return fill(n === 1 ? one : many, { n: num(n) }) }
    function listOf(items) { return Backend.listOf(items) }
    // A word put inside a sentence: lower case, but not in German.
    function inSentence(s) { return Backend.language === "de" ? s : s.toLowerCase() }
    function percent(part, whole) { return whole ? Math.round(100 * part / whole) : 0 }
    // The text of a segment's content, its tags left out.
    function plain(content) { return (content || []).filter(r => r.text !== undefined).map(r => r.text).join("") }
}
