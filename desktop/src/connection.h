#pragma once

#include <QJsonObject>
#include <QObject>
#include <QUrl>
#include <QVariantMap>

class ApiClient;
class EventFeed;

// Where work is kept: this computer (the app's core) or a team server,
// and who is signed in there.
class Connection : public QObject {
    Q_OBJECT
    // For the interface.
    Q_PROPERTY(QString id READ id CONSTANT)
    Q_PROPERTY(bool local READ isLocal CONSTANT)
    Q_PROPERTY(QString displayName READ displayName NOTIFY stateChanged)
    Q_PROPERTY(QString address READ address CONSTANT)
    Q_PROPERTY(int state READ stateValue NOTIFY stateChanged)
    Q_PROPERTY(bool ready READ isReady NOTIFY stateChanged)
    Q_PROPERTY(QString message READ message NOTIFY stateChanged)
    Q_PROPERTY(QString userName READ userName NOTIFY userChanged)
    Q_PROPERTY(QString login READ login NOTIFY userChanged)
    Q_PROPERTY(bool admin READ isAdmin NOTIFY userChanged)
    Q_PROPERTY(QVariantMap user READ userMap NOTIFY userChanged)
    Q_PROPERTY(QString provider READ provider NOTIFY infoChanged)
    Q_PROPERTY(QString serverVersion READ serverVersion NOTIFY infoChanged)
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
    // The name to show, and the name the account signs in with, which is
    // what the core records as who did something.
    QString userName() const;
    QString login() const;
    bool isAdmin() const;
    QString serverVersion() const { return m_version; }
    QString provider() const { return m_provider; }

    State state() const { return m_state; }
    QString message() const { return m_message; }
    bool isReady() const { return m_state == State::Ready; }
    int stateValue() const { return int(m_state); }
    QVariantMap userMap() const { return m_user.toVariantMap(); }
    // The server's address as typed, without the scheme; empty here.
    QString address() const { return isLocal() ? QString() : m_url.host() + (m_url.port() > 0 ? QStringLiteral(":%1").arg(m_url.port()) : QString()); }

    // Signed in: requests go with token, the live feed runs.
    void activate(const QString &token, const QJsonObject &user);
    void setUser(const QJsonObject &user);
    void setServerInfo(const QString &version, const QString &provider);
    void setState(State state, const QString &message = {});

signals:
    void stateChanged(Connection::State state);
    void userChanged();
    // What the server said of itself: its version and identity provider.
    void infoChanged();

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
