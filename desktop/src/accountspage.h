#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QPointer>
#include <QSet>
#include <QTimer>
#include <QWidget>

#include <functional>

#include "webui.h"

class ApiReply;
class Connection;
class ConnectionManager;
class QVBoxLayout;
struct ApiResult;

// The accounts of a team server and what each may do, laid out as the web
// app's Accounts page: administrators make accounts, disable them, set
// passwords, link provider sign-ins and give roles on projects, memories
// and termbases.
class AccountsPage : public QWidget {
    Q_OBJECT
public:
    explicit AccountsPage(ConnectionManager *manager, QWidget *parent = nullptr);

    void reload();
    // The team server shown, of those signed in to.
    QString connectionId() const { return m_conn; }
    void setConnection(const QString &id);
    QJsonArray users() const { return m_users; }
    qint64 selected() const { return m_selected; }
    void select(qint64 userId);
    // What is shown of the chosen account.
    QJsonArray grants() const { return m_grants; }
    QJsonArray signIns() const { return m_signIns; }
    QJsonArray identities() const { return m_identities; }
    // The roles of the server: those that come with it and those defined.
    QJsonArray roles() const { return m_roles; }
    QString problem() const { return m_problem; }
    bool busy() const { return m_pending > 0; }
    void notify(const QString &text);
    QString message() const;

signals:
    // The accounts, and the chosen account's parts, are read and shown.
    void loaded();
    // Leads to the Settings page, where servers are signed in to.
    void openSettings();

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    Connection *conn() const;
    void listen();
    void render();
    void showList(QVBoxLayout *into, QWidget *host);
    void showNewAccount(QVBoxLayout *into, QWidget *host);
    void showDetail(QVBoxLayout *into, QWidget *host);
    void showIdentities(QVBoxLayout *into, QWidget *host, const QJsonObject &user);
    void showRoles(QVBoxLayout *into, QWidget *host, const QJsonObject &user);
    void showRoleDefinitions(QVBoxLayout *into, QWidget *host);
    // Sends a change of the roles themselves, says done, and reads again.
    void attemptRole(ApiReply *reply, const QString &done);
    void loadDetail();
    QJsonObject user(qint64 id) const;
    // Sends a change of the chosen account, says done, and reads again.
    void attempt(ApiReply *reply, const QString &done);
    void request(ApiReply *reply, std::function<void(const ApiResult &)> then);
    QString resourceName(const QJsonObject &grant) const;

    ConnectionManager *m_manager;
    QSet<QString> m_listening;
    QTimer m_reload;
    QString m_conn;
    QJsonArray m_users;
    bool m_read = false;
    QJsonArray m_projects;
    QJsonObject m_catalog;
    QJsonArray m_roles;
    QString m_roleProblem;
    QString m_newRoleName;
    QStringList m_newRolePermissions{QStringLiteral("read")};
    qint64 m_selected = 0;
    QJsonArray m_grants;
    QJsonArray m_signIns;
    QJsonArray m_identities;
    QString m_listProblem;
    QString m_problem;
    QJsonObject m_newAccount;
    QJsonObject m_newProblems;
    int m_pending = 0;
    int m_reading = 0;
    int m_detailReading = 0;
    QVBoxLayout *m_body;
    Web::Toast *m_toast;
};
