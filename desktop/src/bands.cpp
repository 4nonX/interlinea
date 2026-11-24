#include "bands.h"

#include "theme.h"

#include <QHBoxLayout>
#include <QHeaderView>
#include <QJsonArray>
#include <QLabel>
#include <QHash>
#include <QLocale>
#include <QTableWidget>

namespace Bands {

QString ofScore(int score, bool context)
{
    if (context && score == 100)
        return QStringLiteral("cm");
    if (score >= 100)
        return QStringLiteral("100");
    if (score >= 95)
        return QStringLiteral("95");
    if (score >= 85)
        return QStringLiteral("85");
    if (score >= 75)
        return QStringLiteral("75");
    if (score >= 50)
        return QStringLiteral("50");
    return QStringLiteral("new");
}

QString scoreLabel(int score, bool context)
{
    return context && score == 100 ? QStringLiteral("CM") : QStringLiteral("%1%").arg(score);
}

QString scoreDescription(int score, bool context)
{
    if (context && score == 100)
        return QObject::tr("Context match (101%): the text and the segment before it match");
    if (score == 100)
        return QObject::tr("Exact match (100%)");
    return QObject::tr("Fuzzy match, %1%").arg(score);
}

Colors colors(const QString &band)
{
    const bool dark = Theme::dark();
    if (band == QLatin1String("cm") || band == QLatin1String("100") || band == QLatin1String("context"))
        return {Theme::color("okBg"), Theme::color("ok"), Qt::transparent};
    if (band == QLatin1String("95"))
        return {dark ? QColor(0x2e, 0x35, 0x17) : QColor(0xed, 0xf3, 0xd6), dark ? QColor(0xc7, 0xd8, 0x7a) : QColor(0x4b, 0x5e, 0x0c), Qt::transparent};
    if (band == QLatin1String("85"))
        return {Theme::color("warnBg"), Theme::color("warn"), Qt::transparent};
    if (band == QLatin1String("75"))
        return {dark ? QColor(0x3d, 0x28, 0x17) : QColor(0xfd, 0xe8, 0xd6), dark ? QColor(0xf2, 0xa8, 0x6b) : QColor(0x8a, 0x3f, 0x00), Qt::transparent};
    if (band == QLatin1String("rep") || band == QLatin1String("repetition") || band == QLatin1String("cross-file"))
        return {Theme::color("serverBg"), Theme::color("server"), Qt::transparent};
    if (band == QLatin1String("locked"))
        return {Qt::transparent, Theme::color("text3"), Theme::color("borderStrong")};
    return {dark ? QColor(0x2a, 0x30, 0x38) : QColor(0xee, 0xf0, 0xf3), Theme::color("text2"), Qt::transparent};
}

QWidget *badge(const QString &text, const QString &band, QWidget *parent)
{
    auto *l = new QLabel(text, parent);
    const Colors c = colors(band);
    l->setAlignment(Qt::AlignCenter);
    l->setMinimumWidth(36);
    l->setFixedHeight(20);
    l->setStyleSheet(QStringLiteral("QLabel { background: %1; color: %2; border: 1px solid %3; border-radius: 4px;"
                                    " font-size: 12px; font-weight: 600; padding: 0 5px; }")
                         .arg(c.background.name(QColor::HexArgb), c.text.name(), c.border.name(QColor::HexArgb)));
    return l;
}

static QString shortName(const QString &band)
{
    static const QHash<QString, QString> names{
        {QStringLiteral("locked"), QStringLiteral("Lock")}, {QStringLiteral("context"), QStringLiteral("CM")},
        {QStringLiteral("repetition"), QStringLiteral("Rep")}, {QStringLiteral("cross-file"), QStringLiteral("XRep")},
        {QStringLiteral("100"), QStringLiteral("100%")},      {QStringLiteral("95"), QStringLiteral("95+")},
        {QStringLiteral("85"), QStringLiteral("85+")},        {QStringLiteral("75"), QStringLiteral("75+")},
        {QStringLiteral("50"), QStringLiteral("50+")},        {QStringLiteral("new"), QStringLiteral("New")}};
    return names.value(band, band);
}

static QString hint(const QString &band)
{
    if (band == QLatin1String("locked"))
        return QObject::tr("Locked in the file; not translated");
    if (band == QLatin1String("context"))
        return QObject::tr("100% match whose preceding segment also matches (101%)");
    if (band == QLatin1String("repetition"))
        return QObject::tr("Same text and tags earlier in the same file");
    if (band == QLatin1String("cross-file"))
        return QObject::tr("Same text and tags in an earlier file of the project");
    if (band == QLatin1String("100"))
        return QObject::tr("Same text and tags as a memory unit");
    if (band == QLatin1String("50"))
        return QObject::tr("Usually charged like new text");
    if (band == QLatin1String("new"))
        return QObject::tr("No match of 50% or more");
    return {};
}

QTableWidget *analysisTable(const QJsonObject &report, QWidget *parent)
{
    const QJsonArray rows = report.value(QLatin1String("rows")).toArray();
    const QJsonObject total = report.value(QLatin1String("total")).toObject();
    const int totalWords = total.value(QLatin1String("words")).toInt();
    auto *t = new QTableWidget(rows.size() + 1, 4, parent);
    t->setAccessibleName(QObject::tr("Match analysis"));
    t->setHorizontalHeaderLabels({QObject::tr("Match"), QObject::tr("Segments"), QObject::tr("Words"), QObject::tr("Share")});
    t->verticalHeader()->hide();
    t->setEditTriggers(QAbstractItemView::NoEditTriggers);
    t->setSelectionMode(QAbstractItemView::NoSelection);
    t->setFocusPolicy(Qt::TabFocus);
    t->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
    for (int c = 1; c < 4; ++c) {
        t->horizontalHeader()->setSectionResizeMode(c, QHeaderView::ResizeToContents);
        t->horizontalHeaderItem(c)->setTextAlignment(Qt::AlignRight | Qt::AlignVCenter);
    }
    const QLocale loc;
    auto num = [t, &loc](int row, int col, const QString &text) {
        auto *item = new QTableWidgetItem(text);
        item->setTextAlignment(Qt::AlignRight | Qt::AlignTop);
        t->setItem(row, col, item);
    };
    for (int i = 0; i < rows.size(); ++i) {
        const QJsonObject r = rows.at(i).toObject();
        const QString band = r.value(QLatin1String("band")).toString();
        const int words = r.value(QLatin1String("words")).toInt();
        auto *cell = new QWidget(t);
        auto *l = new QHBoxLayout(cell);
        l->setContentsMargins(12, 8, 12, 8);
        l->setSpacing(12);
        l->addWidget(badge(shortName(band), band, cell), 0, Qt::AlignTop);
        QString text = r.value(QLatin1String("name")).toString().toHtmlEscaped();
        if (const QString h = hint(band); !h.isEmpty())
            text += QStringLiteral("<br><span style=\"color:%1; font-size:12px\">%2</span>").arg(Theme::color("text3").name(), h.toHtmlEscaped());
        auto *label = new QLabel(text, cell);
        label->setWordWrap(true);
        l->addWidget(label, 1);
        if (words == 0)
            cell->setStyleSheet(QStringLiteral("QLabel { color: %1; }").arg(Theme::color("text3").name()));
        t->setCellWidget(i, 0, cell);
        auto *item = new QTableWidgetItem(QStringLiteral("%1 %2").arg(shortName(band), r.value(QLatin1String("name")).toString()));
        item->setForeground(Qt::transparent);
        t->setItem(i, 0, item);
        num(i, 1, loc.toString(r.value(QLatin1String("segments")).toInt()));
        num(i, 2, loc.toString(words));
        num(i, 3, totalWords ? QStringLiteral("%1%").arg(qRound(100.0 * words / totalWords)) : QStringLiteral("0%"));
    }
    const int last = rows.size();
    auto *totalItem = new QTableWidgetItem(QObject::tr("Total"));
    QFont bold = totalItem->font();
    bold.setWeight(QFont::DemiBold);
    totalItem->setFont(bold);
    t->setItem(last, 0, totalItem);
    num(last, 1, loc.toString(total.value(QLatin1String("segments")).toInt()));
    num(last, 2, loc.toString(totalWords));
    num(last, 3, QStringLiteral("100%"));
    t->resizeRowsToContents();
    return t;
}

} // namespace Bands
