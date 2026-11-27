#pragma once

#include "apiclient.h"
#include "apierror.h"
#include "connection.h"
#include "corelauncher.h"

#include <QList>
#include <QObject>
#include <QPointer>

#include <functional>

class ProviderSignIn;

// What a team server says about itself before anyone signs in.
struct ServerInfo {
    QUrl url;
    QString version;
    // The identity provider people sign in with, if any.
    QString provider;
    // The server has no accounts yet: the first administrator can be made.
    bool setup = false;
};

// The connections: this computer, always there, and the team servers the
// person signed in to. Servers are remembered in the settings, their
// sign-ins in the keychain.
class ConnectionManager : public QObject {
    Q_OBJECT
public:
    using Done = std::function<void(Connection *connection, const ApiError &error)>;

    ConnectionManager(CoreLauncher *core, QObject *parent = nullptr);

    // Starts the core and signs in again to the remembered servers.
    void start();

    CoreLauncher *core() const { return m_core; }
    Connection *local() const { return m_local; }
    QList<Connection *> teams() const { return m_teams; }
    QList<Connection *> all() const;
    Connection *find(const QString &id) const;

    // A server address as typed: https:// is added, the end trimmed.
    static QUrl serverUrl(const QString &typed);
    // The name this computer's sign-ins are listed under.
    static QString deviceName();

    // Asks a server what it is.
    void probe(const QUrl &url, std::function<void(const ServerInfo &info, const ApiError &error)> done);
    void signInWithPassword(const QUrl &url, const QString &username, const QString &password, Done done);
    // Makes the first administrator of a new server.
    void setUpServer(const QUrl &url, const QString &setupToken, const QString &username, const QString &name,
                     const QString &email, const QString &password, Done done);
    // Opens the system browser to sign in through the server's provider.
    void signInWithProvider(const QUrl &url, Done done);
    // Cancels a provider sign-in in progress.
    void cancelProviderSignIn();
    void signOut(Connection *c);
    // Signs out and forgets the server.
    void remove(Connection *c);

    // How the browser is opened (tests replace it).
    void setUrlOpener(std::function<bool(const QUrl &)> open) { m_open = std::move(open); }

signals:
    void added(Connection *c);
    void removed(const QString &id);
    // The servers kept from the last time were read (start()).
    void loaded();
    // A connection's state or user changed.
    void changed(Connection *c);

private:
    void adopt(Connection *c);
    Connection *teamFor(const QUrl &url);
    void signedIn(const QUrl &url, const ApiResult &r, Done done);
    void restore(Connection *c);
    void save();

    CoreLauncher *m_core;
    Connection *m_local;
    QList<Connection *> m_teams;
    QPointer<ProviderSignIn> m_provider;
    std::function<bool(const QUrl &)> m_open;
};
