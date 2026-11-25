#include "flowlayout.h"

#include <QWidget>

FlowLayout::FlowLayout(QWidget *parent, int spacing)
    : QLayout(parent)
    , m_spacing(spacing)
{
    setContentsMargins(0, 0, 0, 0);
}

FlowLayout::~FlowLayout()
{
    while (QLayoutItem *item = takeAt(0))
        delete item;
}

void FlowLayout::addItem(QLayoutItem *item)
{
    m_items.append(item);
}

QLayoutItem *FlowLayout::takeAt(int index)
{
    if (index < 0 || index >= m_items.size())
        return nullptr;
    return m_items.takeAt(index);
}

QSize FlowLayout::minimumSize() const
{
    // As narrow as the widest item: everything else wraps.
    QSize size;
    for (const QLayoutItem *item : m_items)
        size = size.expandedTo(item->minimumSize());
    const QMargins m = contentsMargins();
    return size + QSize(m.left() + m.right(), m.top() + m.bottom());
}

void FlowLayout::setGeometry(const QRect &rect)
{
    QLayout::setGeometry(rect);
    doLayout(rect, false);
}

int FlowLayout::doLayout(const QRect &rect, bool measureOnly) const
{
    const QMargins m = contentsMargins();
    const QRect area = rect.adjusted(m.left(), m.top(), -m.right(), -m.bottom());
    int x = area.x();
    int y = area.y();
    int lineHeight = 0;
    for (QLayoutItem *item : m_items) {
        if (item->widget() && item->widget()->isHidden())
            continue;
        const QSize hint = item->sizeHint().expandedTo(item->minimumSize());
        if (x > area.x() && x + hint.width() > area.right() + 1) {
            x = area.x();
            y += lineHeight + m_spacing;
            lineHeight = 0;
        }
        // A part wider than the line gets the line, and the height that
        // width asks for (a row of buttons that wraps in turn).
        const int w = qMin(hint.width(), area.width());
        int h = hint.height();
        if (w < hint.width() && item->hasHeightForWidth())
            h = qMax(h, item->heightForWidth(w));
        if (!measureOnly)
            item->setGeometry(QRect(QPoint(x, y), QSize(w, h)));
        x += w + m_spacing;
        lineHeight = qMax(lineHeight, h);
    }
    return y + lineHeight - rect.y() + m.bottom();
}
