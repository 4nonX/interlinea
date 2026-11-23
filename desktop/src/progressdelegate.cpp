#include "progressdelegate.h"

#include <QApplication>
#include <QPainter>

void ProgressDelegate::paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const
{
    const QVariant v = index.data(ShareRole);
    if (!v.isValid()) {
        QStyledItemDelegate::paint(painter, option, index);
        return;
    }
    QStyleOptionViewItem opt(option);
    initStyleOption(&opt, index);
    const QString text = opt.text;
    opt.text.clear();
    // Background and selection as any cell.
    QApplication::style()->drawControl(QStyle::CE_ItemViewItem, &opt, painter, opt.widget);
    const double share = qBound(0.0, v.toDouble(), 1.0);
    const QPalette &pal = opt.palette;
    const QFontMetrics fm(opt.font);
    const int textWidth = fm.horizontalAdvance(QStringLiteral("100 %")) + 8;
    const QRect r = opt.rect.adjusted(8, 0, -8, 0);
    const QRect bar(r.left(), r.center().y() - 3, qMax(20, r.width() - textWidth), 6);
    painter->save();
    painter->setRenderHint(QPainter::Antialiasing);
    painter->setPen(Qt::NoPen);
    painter->setBrush(pal.color(QPalette::Mid));
    painter->drawRoundedRect(bar, 3, 3);
    if (share > 0) {
        QRect fill = bar;
        fill.setWidth(qMax(6, int(bar.width() * share)));
        painter->setBrush(share >= 1.0 ? QColor(0x17, 0x70, 0x3f) : pal.color(QPalette::Active, QPalette::Highlight));
        painter->drawRoundedRect(fill, 3, 3);
    }
    painter->setPen(opt.state & QStyle::State_Selected ? pal.color(QPalette::Link) : pal.color(QPalette::Text));
    painter->drawText(QRect(bar.right() + 8, r.top(), textWidth, r.height()), Qt::AlignRight | Qt::AlignVCenter, text);
    painter->restore();
}

QSize ProgressDelegate::sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const
{
    QSize s = QStyledItemDelegate::sizeHint(option, index);
    if (index.data(ShareRole).isValid())
        s.setWidth(qMax(s.width(), 150));
    return s;
}
