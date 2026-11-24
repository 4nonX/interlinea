#pragma once

#include <QLayout>
#include <QList>

// Lays widgets out in a row that wraps onto the next line when it runs out
// of room, as the web's flex-wrap rows (.tool-row, .row).
class FlowLayout : public QLayout {
public:
    explicit FlowLayout(QWidget *parent = nullptr, int spacing = 8);
    ~FlowLayout() override;

    void addItem(QLayoutItem *item) override;
    int count() const override { return int(m_items.size()); }
    QLayoutItem *itemAt(int index) const override { return m_items.value(index); }
    QLayoutItem *takeAt(int index) override;
    Qt::Orientations expandingDirections() const override { return {}; }
    bool hasHeightForWidth() const override { return true; }
    int heightForWidth(int width) const override { return doLayout(QRect(0, 0, width, 0), true); }
    QSize minimumSize() const override;
    QSize sizeHint() const override { return minimumSize(); }
    void setGeometry(const QRect &rect) override;

private:
    int doLayout(const QRect &rect, bool measureOnly) const;

    QList<QLayoutItem *> m_items;
    int m_spacing;
};
