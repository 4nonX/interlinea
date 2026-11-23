#pragma once

#include <QJsonObject>
#include <QObject>
#include <QUrl>

class ApiClient;
class EventFeed;

// Where work is kept: this computer (the app's core) or a team server,
// and who is signed in there.
class Connection : public QObject {
    Q_OBJECT
public:
    enum class Kind { Local, Team };
    Q_ENUM(Kind)
    enum class State {
        // Starting the core, or checking a sign-in.
        Connecting,
        Ready,
        // The sign-in ended or there is none; sign in again.
        SignedOut,
        // The server can't be reached; the sign-in is kept.
        Offline,
        // The core could not be started.
        Failed,
    };
    Q_ENUM(State)

    Connection(Kind kind, const QString &id, const QUrl &url, QObject *parent = nullptr);

    Kind kind() const { return m_kind; }
    bool isLocal() const { return m_kind == Kind::Local; }
    QString id() const { return m_id; }
    QUrl url() const { return m_url; }
    // The core listens on another port after each start.
    void setUrl(const QUrl &url);
    // "This computer", or the server's address without the scheme.
    QString displayName() const;
    ApiClient *api() const { return m_api; }
    EventFeed *feed() const { return m_feed; }

    QJsonObject user() const { return m_user; }
    QString userName() const;
    bool isAdmin() const;
    QString serverVersion() const { return m_version; }
    QString provider() const { return m_provider; }

    State state() const { return m_state; }
    QString message() const { return m_message; }
    bool isReady() const { return m_state == State::Ready; }

    // Signed in: requests go with token, the live feed runs.
    void activate(const QString &token, const QJsonObject &user);
    void setUser(const QJsonObject &user);
    void setServerInfo(const QString &version, const QString &provider);
    void setState(State state, const QString &message = {});

signals:
    void stateChanged(Connection::State state);
    void userChanged();

private:
    Kind m_kind;
    QString m_id;
    QUrl m_url;
    ApiClient *m_api;
    EventFeed *m_feed;
    QJsonObject m_user;
    QString m_version;
    QString m_provider;
    State m_state = State::Connecting;
    QString m_message;
};
