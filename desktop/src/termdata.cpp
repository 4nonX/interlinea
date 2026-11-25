#include "termdata.h"

#include <QCoreApplication>
#include <QLocale>

namespace TermData {

namespace {
int languageIndex(const QJsonArray &langs, const QString &lang)
{
    for (int i = 0; i < langs.size(); i++) {
        if (langs[i].toObject().value(QLatin1String("lang")).toString().compare(lang, Qt::CaseInsensitive) == 0)
            return i;
    }
    return -1;
}

QJsonObject withLanguage(QJsonObject entry, const QString &lang, const QJsonObject &section)
{
    QJsonArray langs = entry.value(QLatin1String("languages")).toArray();
    const int i = languageIndex(langs, lang);
    const bool empty = section.value(QLatin1String("terms")).toArray().isEmpty() && section.value(QLatin1String("fields")).toArray().isEmpty();
    if (empty) {
        if (i >= 0)
            langs.removeAt(i);
    } else if (i >= 0) {
        langs[i] = section;
    } else {
        langs.append(section);
    }
    entry.insert(QLatin1String("languages"), langs);
    return entry;
}

QJsonObject sectionFor(const QJsonObject &entry, const QString &lang)
{
    QJsonObject s = language(entry, lang);
    if (s.isEmpty())
        s.insert(QLatin1String("lang"), lang);
    return s;
}
}

QJsonObject language(const QJsonObject &entry, const QString &lang)
{
    const QJsonArray langs = entry.value(QLatin1String("languages")).toArray();
    const int i = languageIndex(langs, lang);
    return i < 0 ? QJsonObject() : langs[i].toObject();
}

QJsonArray terms(const QJsonObject &entry, const QString &lang)
{
    return language(entry, lang).value(QLatin1String("terms")).toArray();
}

QStringList languages(const QJsonObject &entry)
{
    QStringList out;
    for (const QJsonValue &l : entry.value(QLatin1String("languages")).toArray())
        out << l.toObject().value(QLatin1String("lang")).toString();
    return out;
}

QJsonObject withTerms(QJsonObject entry, const QString &lang, const QJsonArray &terms)
{
    QJsonObject s = sectionFor(entry, lang);
    s.insert(QLatin1String("terms"), terms);
    return withLanguage(entry, lang, s);
}

QJsonObject withLanguageField(QJsonObject entry, const QString &lang, const QString &field, const QString &value)
{
    QJsonObject s = sectionFor(entry, lang);
    s.insert(QLatin1String("fields"), withValue(s.value(QLatin1String("fields")).toArray(), field, value));
    return withLanguage(entry, lang, s);
}

QJsonObject withTermField(QJsonObject entry, const QString &lang, int term, const QString &field, const QString &value)
{
    QJsonObject s = sectionFor(entry, lang);
    QJsonArray ts = s.value(QLatin1String("terms")).toArray();
    if (term < 0 || term >= ts.size())
        return entry;
    QJsonObject t = ts[term].toObject();
    t.insert(QLatin1String("fields"), withValue(t.value(QLatin1String("fields")).toArray(), field, value));
    ts[term] = t;
    s.insert(QLatin1String("terms"), ts);
    return withLanguage(entry, lang, s);
}

QJsonObject withEntryField(QJsonObject entry, const QString &field, const QString &value)
{
    entry.insert(QLatin1String("fields"), withValue(entry.value(QLatin1String("fields")).toArray(), field, value));
    return entry;
}

QString value(const QJsonArray &fields, const QString &field)
{
    for (const QJsonValue &v : fields) {
        const QJsonObject o = v.toObject();
        if (o.value(QLatin1String("field")).toString() == field)
            return o.value(QLatin1String("value")).toString();
    }
    return {};
}

QJsonArray withValue(QJsonArray fields, const QString &field, const QString &value)
{
    bool found = false;
    for (int i = fields.size() - 1; i >= 0; i--) {
        if (fields[i].toObject().value(QLatin1String("field")).toString() != field)
            continue;
        // A field is kept once; the first place it had stays.
        if (value.isEmpty() || found) {
            fields.removeAt(i);
            continue;
        }
        found = true;
    }
    if (value.isEmpty())
        return fields;
    const QJsonObject set{{QLatin1String("field"), field}, {QLatin1String("value"), value}};
    for (int i = 0; i < fields.size(); i++) {
        if (fields[i].toObject().value(QLatin1String("field")).toString() == field) {
            fields[i] = set;
            return fields;
        }
    }
    fields.append(set);
    return fields;
}

QString head(const QJsonObject &entry, const QString &lang, QString *in)
{
    const QJsonArray own = terms(entry, lang);
    if (!own.isEmpty()) {
        if (in)
            *in = lang;
        return own.first().toObject().value(QLatin1String("text")).toString();
    }
    for (const QJsonValue &l : entry.value(QLatin1String("languages")).toArray()) {
        const QJsonArray ts = l.toObject().value(QLatin1String("terms")).toArray();
        if (ts.isEmpty())
            continue;
        if (in)
            *in = l.toObject().value(QLatin1String("lang")).toString();
        return ts.first().toObject().value(QLatin1String("text")).toString();
    }
    return {};
}

QString statusLabel(const QString &status)
{
    if (status == QLatin1String("approved"))
        return QCoreApplication::translate("TermData", "Approved");
    if (status == QLatin1String("forbidden"))
        return QCoreApplication::translate("TermData", "Forbidden");
    return QCoreApplication::translate("TermData", "Proposed");
}

bool hasTerm(const QJsonArray &terms, const QString &text, int except)
{
    // Space aside, but not case: spellings that differ in case are terms of
    // their own, as in the core (a forbidden "Mistjet" beside "MistJet").
    const QString want = text.simplified();
    for (int i = 0; i < terms.size(); i++) {
        if (i != except && terms[i].toObject().value(QLatin1String("text")).toString().simplified() == want)
            return true;
    }
    return false;
}

QString searchText(const QJsonObject &entry)
{
    QStringList parts;
    for (const QJsonValue &f : entry.value(QLatin1String("fields")).toArray())
        parts << f.toObject().value(QLatin1String("value")).toString();
    for (const QJsonValue &l : entry.value(QLatin1String("languages")).toArray()) {
        for (const QJsonValue &t : l.toObject().value(QLatin1String("terms")).toArray())
            parts << t.toObject().value(QLatin1String("text")).toString();
    }
    return parts.join(QLatin1Char('\n')).toLower();
}

bool hasStatus(const QJsonObject &entry, const QString &status)
{
    for (const QJsonValue &l : entry.value(QLatin1String("languages")).toArray()) {
        for (const QJsonValue &t : l.toObject().value(QLatin1String("terms")).toArray()) {
            QString s = t.toObject().value(QLatin1String("status")).toString();
            if (s.isEmpty())
                s = QStringLiteral("proposed");
            if (s == status)
                return true;
        }
    }
    return false;
}

QString entries(qint64 n)
{
    return n == 1 ? QCoreApplication::translate("TermData", "1 entry") : QCoreApplication::translate("TermData", "%1 entries").arg(QLocale().toString(n));
}

QString termCount(qint64 n)
{
    return n == 1 ? QCoreApplication::translate("TermData", "1 term") : QCoreApplication::translate("TermData", "%1 terms").arg(QLocale().toString(n));
}

QJsonObject forSaving(const QJsonObject &stored)
{
    QJsonObject e;
    e.insert(QLatin1String("id"), stored.value(QLatin1String("id")));
    e.insert(QLatin1String("fields"), stored.value(QLatin1String("fields")).toArray());
    e.insert(QLatin1String("languages"), stored.value(QLatin1String("languages")).toArray());
    return e;
}

} // namespace TermData
