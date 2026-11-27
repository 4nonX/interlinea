pragma Singleton
import QtQuick
import Interlinea

// Reading and changing termbase entries as the core sends them: an entry
// has fields and languages, a language has fields and terms, a term has a
// text, a status and fields. A field value is {field, value}. Every
// function returns a new object and leaves the one given as it was.
QtObject {
    function copy(o) { return JSON.parse(JSON.stringify(o || {})) }
    function languageIndex(langs, lang) {
        const want = (lang || "").toLowerCase()
        return (langs || []).findIndex(l => (l.lang || "").toLowerCase() === want)
    }
    function language(entry, lang) {
        const langs = (entry || {}).languages || []
        const i = languageIndex(langs, lang)
        return i < 0 ? ({}) : langs[i]
    }
    function terms(entry, lang) { return language(entry, lang).terms || [] }
    function languages(entry) { return ((entry || {}).languages || []).map(l => l.lang) }
    function sectionFor(entry, lang) {
        const s = copy(language(entry, lang))
        if (!s.lang) s.lang = lang
        return s
    }
    // A language left without terms and fields is taken out.
    function withLanguage(entry, lang, section) {
        const e = copy(entry)
        const langs = e.languages || []
        const i = languageIndex(langs, lang)
        const empty = !(section.terms || []).length && !(section.fields || []).length
        if (empty) { if (i >= 0) langs.splice(i, 1) }
        else if (i >= 0) langs[i] = section
        else langs.push(section)
        e.languages = langs
        return e
    }
    function withTerms(entry, lang, list) {
        const s = sectionFor(entry, lang)
        s.terms = list
        return withLanguage(entry, lang, s)
    }
    function withLanguageField(entry, lang, field, value) {
        const s = sectionFor(entry, lang)
        s.fields = withValue(s.fields || [], field, value)
        return withLanguage(entry, lang, s)
    }
    function withTermField(entry, lang, term, field, value) {
        const s = sectionFor(entry, lang)
        const ts = s.terms || []
        if (term < 0 || term >= ts.length) return entry
        ts[term].fields = withValue(ts[term].fields || [], field, value)
        s.terms = ts
        return withLanguage(entry, lang, s)
    }
    function withEntryField(entry, field, value) {
        const e = copy(entry)
        e.fields = withValue(e.fields || [], field, value)
        return e
    }
    function value(fields, field) {
        const f = (fields || []).find(x => x.field === field)
        return f ? (f.value || "") : ""
    }
    // The fields with one set; an empty value takes it out. A field is
    // kept once; the first place it had stays.
    function withValue(fields, field, val) {
        const out = []
        let set = false
        for (const f of fields || []) {
            if (f.field !== field) { out.push(f); continue }
            if (!val || set) continue
            out.push({ field: field, value: val })
            set = true
        }
        if (val && !set) out.push({ field: field, value: val })
        return out
    }
    // The term an entry is named by: the first in lang, else the first of
    // any language, as { text, lang }.
    function head(entry, lang) {
        const own = terms(entry, lang)
        if (own.length) return { text: own[0].text || "", lang: lang }
        for (const l of (entry || {}).languages || [])
            if ((l.terms || []).length) return { text: l.terms[0].text || "", lang: l.lang }
        return { text: "", lang: lang }
    }
    function statusLabel(status) {
        return status === "approved" ? qsTr("Approved") : status === "forbidden" ? qsTr("Forbidden") : qsTr("Proposed")
    }
    // Whether the text is one of the terms, space aside (case counts: a
    // spelling that differs in case is a term of its own).
    function hasTerm(list, text, except) {
        const want = (text || "").replace(/\s+/g, " ").trim()
        return (list || []).some((t, i) => i !== except && (t.text || "").replace(/\s+/g, " ").trim() === want)
    }
    function searchText(entry) {
        const parts = ((entry || {}).fields || []).map(f => f.value || "")
        for (const l of (entry || {}).languages || [])
            for (const t of l.terms || []) parts.push(t.text || "")
        return parts.join("\n").toLowerCase()
    }
    function hasStatus(entry, status) {
        for (const l of (entry || {}).languages || [])
            for (const t of l.terms || [])
                if ((t.status || "proposed") === status) return true
        return false
    }
    function entries(n) { return Fmt.plural(n, qsTr("{n} entry"), qsTr("{n} entries")) }
    function termCount(n) { return Fmt.plural(n, qsTr("{n} term"), qsTr("{n} terms")) }
    // The entry without what the core adds to it (version, who and when).
    function forSaving(stored) {
        return { id: stored.id, fields: copy(stored.fields || []), languages: copy(stored.languages || []) }
    }
}
