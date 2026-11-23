#pragma once

#include <QHash>
#include <QMainWindow>

class ConnectionManager;
class Connection;
class NavigationRail;
class QLabel;
class QPushButton;
class QStackedWidget;
class QToolButton;
class QFrame;

// The window: navigation on the side, the page, a message bar for what
// stops work, and the state of each connection in the status bar.
class MainWindow : public QMainWindow {
    Q_OBJECT
public:
    explicit MainWindow(ConnectionManager *manager, QWidget *parent = nullptr);

    // Adds a page to the navigation.
    void addPage(const QString &id, const QString &label, const QIcon &icon, QWidget *page, const QKeySequence &shortcut = {});
    void showPage(const QString &id);
    QString currentPage() const;
    NavigationRail *navigation() const { return m_rail; }
    QWidget *page(const QString &id) const { return m_pages.value(id); }

protected:
    void closeEvent(QCloseEvent *event) override;

private:
    void updateStatus();
    void updateMessage();

    ConnectionManager *m_manager;
    NavigationRail *m_rail;
    QStackedWidget *m_stack;
    QHash<QString, QWidget *> m_pages;
    QFrame *m_message;
    QLabel *m_messageText;
    QPushButton *m_messageAction;
    QHash<QString, QToolButton *> m_status;
};
