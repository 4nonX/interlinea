#include "languages.h"

#include "languagelist.h"

#include <QCoreApplication>
#include <QLocale>
#include <QRegularExpression>

namespace Languages {

QStringList common()
{
    // The list of the core (langtag/languages.go), the same as in the
    // browser client, in its order.
    QStringList out;
    for (const ListedLanguage &l : listedLanguages)
        out << QString::fromUtf8(l.tag);
    return out;
}

static const ListedLanguage *listed(const QString &tag)
{
    for (const ListedLanguage &l : listedLanguages) {
        if (tag.compare(QLatin1String(l.tag), Qt::CaseInsensitive) == 0)
            return &l;
    }
    return nullptr;
}

bool rtl(const QString &tag)
{
    if (const ListedLanguage *l = listed(tag))
        return l->rtl;
    // A language outside the list: by its script, else by the language.
    const QStringList parts = normalize(tag).split(QLatin1Char('-'));
    if (parts.size() > 1 && parts.at(1).size() == 4)
        return QStringList{QStringLiteral("Arab"), QStringLiteral("Hebr"), QStringLiteral("Thaa"), QStringLiteral("Syrc"), QStringLiteral("Nkoo"),
                           QStringLiteral("Adlm")}
            .contains(parts.at(1));
    return QStringList{QStringLiteral("ar"), QStringLiteral("he"), QStringLiteral("fa"), QStringLiteral("ur"), QStringLiteral("ps"),
                       QStringLiteral("sd"), QStringLiteral("ug"), QStringLiteral("yi"), QStringLiteral("dv"), QStringLiteral("ckb")}
        .contains(parts.value(0));
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
    if (const ListedLanguage *known = listed(tag))
        return QCoreApplication::translate("Languages", known->name);
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
    // A listed language by its name alone, as the browser client's choice
    // of languages shows it; any other with its tag, which tells it apart.
    const QString n = name(tag);
    if (listed(tag) || n == tag)
        return n;
    return QStringLiteral("%1, %2").arg(n, tag);
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

QString known(const QString &code)
{
    const QString c = code.trimmed();
    if (c.isEmpty())
        return {};
    const QStringList list = common();
    for (const QString &t : list) {
        if (t.compare(c, Qt::CaseInsensitive) == 0)
            return t;
    }
    for (const QString &t : list) {
        if (samePrimary(t, c))
            return t;
    }
    return {};
}

} // namespace Languages
