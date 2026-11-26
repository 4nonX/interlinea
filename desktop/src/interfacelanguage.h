// The interface in other languages. Strings are written in English, with
// tr(), and looked up by their English text in the catalog of the
// interface language: i18n/de.json and i18n/ko.json, shared with the web
// app and built into the program. `go run ./i18n/gen` lists new strings
// there and checks every one has a translation.
#pragma once

#include <QHash>
#include <QLocale>
#include <QString>
#include <QStringList>
#include <QTranslator>

class QCoreApplication;

namespace InterfaceLanguage {

// The interface languages: English, German, Korean.
QStringList codes();
// A language's name in that language ("Deutsch").
QString nativeName(const QString &code);
// The language chosen in the settings; empty to follow the system's.
QString chosen();
void choose(const QString &code);
// The system's language, where the interface has it; else English.
QString systemLanguage();
// The language the interface is shown in.
QString current();
// The locale numbers and dates are written in.
QLocale locale();
// Installs the catalog of the chosen language, and Qt's own translations
// of its dialogs, into the application.
void install(QCoreApplication &app);

} // namespace InterfaceLanguage

// A translator that looks a string up by its English text in a catalog.
class CatalogTranslator : public QTranslator
{
public:
    bool loadCatalog(const QString &path);
    QString translate(const char *context, const char *sourceText, const char *disambiguation = nullptr, int n = -1) const override;
    bool isEmpty() const override { return m_strings.isEmpty(); }

private:
    QHash<QString, QString> m_strings;
};
