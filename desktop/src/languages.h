#pragma once

#include <QComboBox>
#include <QDateTime>
#include <QString>
#include <QStringList>

// Language tags (BCP 47) as people read them.
namespace Languages {
// The languages offered by name, the same as in the browser client; any
// other is typed as a tag.
QStringList common();
// Whether the language is written from right to left.
bool rtl(const QString &tag);
// "German (Germany)" for de-DE; the tag itself when unknown.
QString name(const QString &tag);
// "German (Germany), de-DE".
QString label(const QString &tag);
// The tag in canonical form ("de_de" becomes "de-DE"), empty if it isn't one.
QString normalize(const QString &typed);
// A date and time as people read them, without seconds.
QString when(const QDateTime &t);
// Whether two tags are the same language ("de" and "de-AT" are).
bool samePrimary(const QString &a, const QString &b);
}

// A choice of language: the common ones, and any other typed as a tag.
class LanguageCombo : public QComboBox {
    Q_OBJECT
public:
    explicit LanguageCombo(QWidget *parent = nullptr);
    QString language() const;
    void setLanguage(const QString &tag);
};
