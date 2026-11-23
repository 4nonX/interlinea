#include "navigationrail.h"

NavigationRail::NavigationRail(QWidget *parent)
    : QListWidget(parent)
{
    setObjectName(QStringLiteral("navigation"));
    setAccessibleName(tr("Navigation"));
    setSelectionMode(QAbstractItemView::SingleSelection);
    setUniformItemSizes(true);
    setIconSize(QSize(20, 20));
    setFrameShape(QFrame::NoFrame);
    setSizePolicy(QSizePolicy::Fixed, QSizePolicy::Expanding);
    setFixedWidth(180);
    setSpacing(2);
    connect(this, &QListWidget::currentItemChanged, this, [this](QListWidgetItem *item) {
        if (item)
            emit pageChosen(item->data(Qt::UserRole).toString());
    });
}

void NavigationRail::addPage(const QString &id, const QString &label, const QIcon &icon, const QString &shortcutHint)
{
    auto *item = new QListWidgetItem(icon, label, this);
    item->setData(Qt::UserRole, id);
    item->setSizeHint(QSize(0, 34));
    if (!shortcutHint.isEmpty())
        item->setToolTip(QStringLiteral("%1 (%2)").arg(label, shortcutHint));
}

void NavigationRail::setCurrentPage(const QString &id)
{
    for (int i = 0; i < count(); ++i) {
        if (item(i)->data(Qt::UserRole).toString() == id) {
            if (currentRow() != i)
                setCurrentRow(i);
            return;
        }
    }
}

QString NavigationRail::currentPage() const
{
    return currentItem() ? currentItem()->data(Qt::UserRole).toString() : QString();
}
