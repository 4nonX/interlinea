#pragma once

#include <QListWidget>

// The navigation down the window's side: one entry per page. Arrow keys
// move through it and Enter or a click opens the page.
class NavigationRail : public QListWidget {
    Q_OBJECT
public:
    explicit NavigationRail(QWidget *parent = nullptr);

    void addPage(const QString &id, const QString &label, const QIcon &icon, const QString &shortcutHint = {});
    void setCurrentPage(const QString &id);
    QString currentPage() const;

signals:
    void pageChosen(const QString &id);
};
