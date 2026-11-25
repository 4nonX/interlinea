#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QString>
#include <QStringList>

// Reading and changing termbase entries as the core sends them: an entry
// has fields and languages, a language has fields and terms, a term has a
// text, a status and fields. A field value is {field, value}.
namespace TermData {

// The language section of an entry, empty if it has none.
QJsonObject language(const QJsonObject &entry, const QString &lang);
QJsonArray terms(const QJsonObject &entry, const QString &lang);
// The languages an entry has terms or fields in, in its order.
QStringList languages(const QJsonObject &entry);
// The entry with the language's terms replaced. A language left without
// terms and fields is taken out.
QJsonObject withTerms(QJsonObject entry, const QString &lang, const QJsonArray &terms);
QJsonObject withLanguageField(QJsonObject entry, const QString &lang, const QString &field, const QString &value);
QJsonObject withTermField(QJsonObject entry, const QString &lang, int term, const QString &field, const QString &value);
QJsonObject withEntryField(QJsonObject entry, const QString &field, const QString &value);

QString value(const QJsonArray &fields, const QString &field);
// The fields with one set; an empty value takes it out.
QJsonArray withValue(QJsonArray fields, const QString &field, const QString &value);

// The term an entry is named by: the first in lang, else the first of any
// language. *in is the language it was found in.
QString head(const QJsonObject &entry, const QString &lang, QString *in = nullptr);
// "approved" as "Approved".
QString statusLabel(const QString &status);
// Whether the text is one of the terms, space aside (case counts: a
// spelling that differs in case is a term of its own).
bool hasTerm(const QJsonArray &terms, const QString &text, int except = -1);
// All terms of all languages, and every entry field, in one lower-case
// text to search.
QString searchText(const QJsonObject &entry);
bool hasStatus(const QJsonObject &entry, const QString &status);
// "1 entry", "3 entries".
QString entries(qint64 n);
QString termCount(qint64 n);
// The entry without what the core adds to it (version, who and when).
QJsonObject forSaving(const QJsonObject &stored);

}
