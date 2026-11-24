#pragma once

#include <QFrame>
#include <QJsonArray>
#include <QPointer>

class Connection;
class QComboBox;
class QLabel;
class QVBoxLayout;

// Who may use a project, memory or termbase on a team server, for its
// managers and the administrators: roles given, changed and taken away
// (the web's AccessPanel).
class AccessPanel : public QFrame {
    Q_OBJECT
public:
    // Embedded, it is a section of a dialog: no card around it, and its
    // title as the dialog's other sections have theirs.
    AccessPanel(Connection *connection, const QString &kind, const QString &resource, const QString &title, QWidget *parent = nullptr, bool embedded = false);

    static QStringList roles();
    static QString roleText(const QString &role);

    void reload();

private:
    void show(const QJsonArray &grants);
    void attempt(class ApiReply *reply);

    QPointer<Connection> m_conn;
    QString m_kind;
    QString m_resource;
    QLabel *m_problem;
    QVBoxLayout *m_list;
    QComboBox *m_who;
    QComboBox *m_role;
    QJsonArray m_grants;
    QJsonArray m_people;
    QJsonArray m_roles;
};
