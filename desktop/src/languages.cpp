#include "languages.h"
#include <QEvent>
#include <QLineEdit>

#include "languagelist.h"

#include <QLineEdit>
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
        return QString::fromUtf8(known->name);
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

} // namespace Languages

LanguageCombo::LanguageCombo(QWidget *parent)
    : QComboBox(parent)
{
    setEditable(true);
    setInsertPolicy(QComboBox::NoInsert);
    for (const QString &t : Languages::common())
        addItem(Languages::label(t), t);
    setCurrentIndex(-1);
    lineEdit()->setPlaceholderText(tr("Choose a language"));
    setMaxVisibleItems(14);
    // A list to choose from first of all: a click on the field opens it,
    // as a click on its arrow does. A language that is not listed can
    // still be typed as its tag (gsw-CH).
    lineEdit()->installEventFilter(this);
    setToolTip(tr("Choose a language from the list. A language that is not listed can be typed as its tag, such as gsw-CH."));
}

bool LanguageCombo::eventFilter(QObject *o, QEvent *e)
{
    if (o == lineEdit() && e->type() == QEvent::MouseButtonRelease && isEnabled() && !lineEdit()->hasSelectedText() && lineEdit()->text().isEmpty()) {
        showPopup();
        return true;
    }
    return QComboBox::eventFilter(o, e);
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
