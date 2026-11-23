#pragma once

#include <QWidget>

class ConnectionManager;
class Connection;
class QLabel;
class QLineEdit;
class QPushButton;
class QTreeWidget;
class QTreeWidgetItem;

// Settings: where work is kept (this computer and team servers), the
// person's name, and what the app is.
class SettingsPage : public QWidget {
    Q_OBJECT
public:
    explicit SettingsPage(ConnectionManager *manager, QWidget *parent = nullptr);

    QTreeWidget *connectionList() const { return m_list; }

private:
    void refresh();
    void updateDetails();
    Connection *selected() const;
    void addServer();
    void signInAgain();
    void signOut();
    void removeServer();
    void saveName();

    ConnectionManager *m_manager;
    QTreeWidget *m_list;
    QLabel *m_details;
    QPushButton *m_add;
    QPushButton *m_signIn;
    QPushButton *m_signOut;
    QPushButton *m_remove;
    QPushButton *m_openData;
    QPushButton *m_restart;
    QLineEdit *m_name;
    QPushButton *m_saveName;
    QLabel *m_nameStatus;
    QLabel *m_secrets;
};
