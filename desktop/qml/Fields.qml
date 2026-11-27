pragma Singleton
import QtQuick
import Interlinea

// The fields of a termbase as the web app makes and edits them
// (TermbaseFields.tsx): the kinds of value, where a field belongs, and the
// ready-made structures a termbase starts from.
QtObject {
    function kinds() {
        return [{ id: "text", name: qsTr("Text") }, { id: "picklist", name: qsTr("A list of values") }, { id: "number", name: qsTr("Number") },
                { id: "date", name: qsTr("Date") }, { id: "boolean", name: qsTr("Yes or no") }]
    }
    function kindName(id) { const k = kinds().find(x => x.id === id); return k ? k.name : id }
    function levels() {
        return [{ id: "entry", name: qsTr("Entry"), hint: qsTr("the whole entry, the same in every language") },
                { id: "language", name: qsTr("Language"), hint: qsTr("each language, such as a definition per language") },
                { id: "term", name: qsTr("Term"), hint: qsTr("each term, such as grammar or usage") }]
    }
    function levelName(id) { const l = levels().find(x => x.id === id); return l ? l.name : id }
    // What keeps a field from being saved, or nothing.
    function problem(f, others) {
        const name = (f.name || "").trim()
        if (!name) return qsTr("Give the field a name.")
        if ((others || []).some(o => o.trim().toLowerCase() === name.toLowerCase())) return qsTr("Another field has this name.")
        if (f.kind === "picklist" && !(f.values || []).length) return qsTr("Give the list its values, one a line.")
        return ""
    }
    function toJson(f) {
        const o = { name: f.name.trim(), level: f.level || "entry", kind: f.kind || "text" }
        if (o.kind === "picklist") o.values = f.values || []
        return o
    }
    // The values of a list as typed: no empty lines, none twice.
    function tidy(text) {
        const out = []
        for (const v of (text || "").split("\n").map(x => x.trim()))
            if (v && out.indexOf(v) < 0) out.push(v)
        return out
    }
    function structures() {
        const text = (name, level) => ({ name: name, level: level, kind: "text", values: [] })
        const list = (name, level, values) => ({ name: name, level: level, kind: "picklist", values: values })
        return [
            { id: "terms", name: qsTr("Terms only"), text: qsTr("A glossary: the terms of each language, each with its status. Fields can be added later."), fields: [] },
            { id: "definitions", name: qsTr("Terms with definitions and notes"), text: qsTr("A definition and its source for each language, and a note on the entry."),
              fields: [text(qsTr("Definition"), "language"), text(qsTr("Source"), "language"), text(qsTr("Note"), "entry")] },
            { id: "grammar", name: qsTr("Terminology with grammar"),
              text: qsTr("Subject field, notes and the date it was last checked; definitions per language; part of speech, gender, number, context and usage per term."),
              fields: [text(qsTr("Subject field"), "entry"), text(qsTr("Note"), "entry"), { name: qsTr("Last checked"), level: "entry", kind: "date", values: [] },
                       text(qsTr("Definition"), "language"), text(qsTr("Source of the definition"), "language"),
                       list(qsTr("Part of speech"), "term", [qsTr("noun"), qsTr("verb"), qsTr("adjective"), qsTr("adverb"), qsTr("proper noun"), qsTr("other")]),
                       list(qsTr("Gender"), "term", [qsTr("masculine"), qsTr("feminine"), qsTr("neuter"), qsTr("common")]),
                       list(qsTr("Number"), "term", [qsTr("singular"), qsTr("plural")]), text(qsTr("Context"), "term"), text(qsTr("Usage note"), "term")] }
        ]
    }

    // The formats a termbase is exported to for other tools.
    function exportFormats() {
        return [{ id: "tbx", label: qsTr("TBX 3"), ext: ".tbx", note: qsTr("The ISO standard (30042:2019), read by current terminology tools.") },
                { id: "tbx2", label: qsTr("TBX 2"), ext: ".tbx", note: qsTr("The older <martif> form, for tools that do not read TBX 3 yet.") },
                { id: "multiterm", label: qsTr("Termbase XML"), ext: ".xml", note: qsTr("The XML exchange format of .sdltb termbases.") },
                { id: "xlsx", label: qsTr("XLSX workbook"), ext: ".xlsx", note: qsTr("One row per entry, one column per language and field. Synonyms share a cell, separated by “; ”.") },
                { id: "csv", label: qsTr("CSV"), ext: ".csv", note: qsTr("Comma-separated, UTF-8. Opens in spreadsheet programs.") },
                { id: "tsv", label: qsTr("Tab-separated text"), ext: ".tsv", note: qsTr("UTF-8, one row per entry.") }]
    }
    // Glossaries to read and print.
    function readFormats() {
        return [{ id: "pdf", note: qsTr("To print or send: fonts included, the same everywhere.") },
                { id: "docx", note: qsTr("To edit further in a word processor.") },
                { id: "html", note: qsTr("One web page with a letter index and a search.") },
                { id: "txt", note: qsTr("Plain text.") }]
    }
    function glossary(f) { return ["pdf", "docx", "html", "txt"].indexOf(f) >= 0 }
    function formatLabel(id) {
        const f = exportFormats().find(x => x.id === id)
        if (f) return f.label
        if (id === "sdltb") return qsTr(".sdltb termbase")
        if (id === "xdt") return qsTr(".xdt termbase definition")
        return id
    }
    // Options as a fresh export of a format starts, as on the web.
    function defaultOptions(format) {
        const o = { format: format, allFields: true }
        if (glossary(format)) { o.layout = "dictionary"; o.letters = true; o.labels = true; o.status = true }
        return o
    }
}
