#pragma once

#include <QStyledItemDelegate>

// Draws a share (0 to 1, in ProgressDelegate::ShareRole) as a slim bar with
// its percentage; screen readers get the cell's text.
class ProgressDelegate : public QStyledItemDelegate {
    Q_OBJECT
public:
    static constexpr int ShareRole = Qt::UserRole + 40;
    using QStyledItemDelegate::QStyledItemDelegate;
    void paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const override;
    QSize sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const override;
};
