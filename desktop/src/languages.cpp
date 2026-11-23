#include "languages.h"

#include <QLineEdit>
#include <QLocale>
#include <QRegularExpression>

namespace Languages {

QStringList common()
{
    return {QStringLiteral("de-DE"), QStringLiteral("de-AT"), QStringLiteral("de-CH"), QStringLiteral("en-GB"), QStringLiteral("en-US"),
            QStringLiteral("fr-FR"), QStringLiteral("fr-CH"), QStringLiteral("es-ES"), QStringLiteral("es-MX"), QStringLiteral("it-IT"),
            QStringLiteral("nl-NL"), QStringLiteral("pl-PL"), QStringLiteral("pt-PT"), QStringLiteral("pt-BR"), QStringLiteral("cs-CZ"),
            QStringLiteral("da-DK"), QStringLiteral("sv-SE"), QStringLiteral("nb-NO"), QStringLiteral("fi-FI"), QStringLiteral("el-GR"),
            QStringLiteral("hu-HU"), QStringLiteral("ro-RO"), QStringLiteral("bg-BG"), QStringLiteral("hr-HR"), QStringLiteral("sk-SK"),
            QStringLiteral("sl-SI"), QStringLiteral("uk-UA"), QStringLiteral("ru-RU"), QStringLiteral("tr-TR"), QStringLiteral("ar-SA"),
            QStringLiteral("he-IL"), QStringLiteral("zh-CN"), QStringLiteral("zh-TW"), QStringLiteral("ja-JP"), QStringLiteral("ko-KR")};
}

QString normalize(const QString &typed)
{
    QString t = typed.trimmed();
    t.replace(QLatin1Char('_'), QLatin1Char('-'));
    static const QRegularExpression re(QStringLiteral("^([A-Za-z]{2,3})(?:-([A-Za-z]{4}))?(?:-([A-Za-z]{2}|[0-9]{3}))?$"));
    const auto m = re.match(t);
    if (!m.hasMatch())
        return {};
    QString out = m.captured(1).toLower();
    if (!m.captured(2).isEmpty())
        out += QLatin1Char('-') + m.captured(2).left(1).toUpper() + m.captured(2).mid(1).toLower();
    if (!m.captured(3).isEmpty())
        out += QLatin1Char('-') + m.captured(3).toUpper();
    return out;
}

QString name(const QString &tag)
{
    const QLocale l(QString(tag).replace(QLatin1Char('-'), QLatin1Char('_')));
    if (l.language() == QLocale::C)
        return tag;
    QString n = QLocale::languageToString(l.language());
    if (tag.contains(QLatin1Char('-')) && l.territory() != QLocale::AnyTerritory)
        n += QStringLiteral(" (%1)").arg(QLocale::territoryToString(l.territory()));
    return n;
}

QString label(const QString &tag)
{
    const QString n = name(tag);
    return n == tag ? tag : QStringLiteral("%1, %2").arg(n, tag);
}

QString when(const QDateTime &t)
{
    const QDateTime l = t.toLocalTime();
    QString time = QLocale().timeFormat(QLocale::ShortFormat);
    time.remove(QStringLiteral(":ss"));
    return QStringLiteral("%1, %2").arg(QLocale().toString(l.date(), QLocale::ShortFormat), l.time().toString(time));
}

bool samePrimary(const QString &a, const QString &b)
{
    return a.section(QLatin1Char('-'), 0, 0).compare(b.section(QLatin1Char('-'), 0, 0), Qt::CaseInsensitive) == 0;
}

} // namespace Languages

LanguageCombo::LanguageCombo(QWidget *parent)
    : QComboBox(parent)
{
    setEditable(true);
    setInsertPolicy(QComboBox::NoInsert);
    for (const QString &t : Languages::common())
        addItem(Languages::label(t), t);
    setCurrentIndex(-1);
    lineEdit()->setPlaceholderText(tr("Language, or a tag such as de-DE"));
}

QString LanguageCombo::language() const
{
    const int i = findText(currentText());
    if (i >= 0)
        return itemData(i).toString();
    // A typed tag, possibly with its name before it.
    QString t = currentText();
    if (t.contains(QLatin1Char(',')))
        t = t.section(QLatin1Char(','), -1);
    return Languages::normalize(t);
}

void LanguageCombo::setLanguage(const QString &tag)
{
    const QString n = Languages::normalize(tag);
    int i = findData(n);
    if (i < 0 && !n.isEmpty()) {
        addItem(Languages::label(n), n);
        i = count() - 1;
    }
    setCurrentIndex(i);
}
