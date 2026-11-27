#include "connectionmanager.h"

#include "apiclient.h"
#include "eventfeed.h"
#include "providersignin.h"
#include "secretstore.h"

#include <QCoreApplication>
#include <QDesktopServices>
#include <QJsonObject>
#include <QSettings>
#include <QSysInfo>
#include <QUuid>


static QString secretKey(const Connection *c)
{
    return QStringLiteral("server/") + c->id();
}

ConnectionManager::ConnectionManager(CoreLauncher *core, QObject *parent)
    : QObject(parent), m_core(core), m_local(new Connection(Connection::Kind::Local, QStringLiteral("local"), QUrl(), this)),
      m_open([](const QUrl &u) { return QDesktopServices::openUrl(u); })
{
    adopt(m_local);
    connect(core, &CoreLauncher::started, this, [this](const CoreLauncher::Handshake &h) {
        m_local->setUrl(h.url);
        m_local->setServerInfo(h.version, QString());
        m_local->activate(h.token, h.user);
    });
    connect(core, &CoreLauncher::restarting, this, [this](const QString &why) {
        m_local->setState(Connection::State::Connecting, why + QLatin1Char(' ') + tr("Starting it again."));
    });
    connect(core, &CoreLauncher::failed, this, [this](const QString &why) {
        m_local->setState(Connection::State::Failed, why);
    });
}

void ConnectionManager::adopt(Connection *c)
{
    connect(c, &Connection::stateChanged, this, [this, c] { emit changed(c); });
    connect(c, &Connection::userChanged, this, [this, c] { emit changed(c); });
}

QList<Connection *> ConnectionManager::all() const
{
    QList<Connection *> out{m_local};
    out += m_teams;
    return out;
}

Connection *ConnectionManager::find(const QString &id) const
{
    for (Connection *c : all()) {
        if (c->id() == id)
            return c;
    }
    return nullptr;
}

QUrl ConnectionManager::serverUrl(const QString &typed)
{
    QString s = typed.trimmed();
    while (s.endsWith(QLatin1Char('/')))
        s.chop(1);
    if (s.isEmpty())
        return {};
    if (!s.startsWith(QLatin1String("http://"), Qt::CaseInsensitive) && !s.startsWith(QLatin1String("https://"), Qt::CaseInsensitive))
        s.prepend(QLatin1String("https://"));
    QUrl u(s, QUrl::StrictMode);
    if (!u.isValid() || u.host().isEmpty())
        return {};
    return u;
}

QString ConnectionManager::deviceName()
{
    return tr("Interlinea on %1").arg(QSysInfo::machineHostName());
}

void ConnectionManager::start()
{
    QSettings settings;
    const int n = settings.beginReadArray(QStringLiteral("servers"));
    for (int i = 0; i < n; ++i) {
        settings.setArrayIndex(i);
        const QString id = settings.value(QStringLiteral("id")).toString();
        const QUrl url(settings.value(QStringLiteral("url")).toString());
        if (id.isEmpty() || !url.isValid() || find(id))
            continue;
        auto *c = new Connection(Connection::Kind::Team, id, url, this);
        c->setUser(QJsonObject::fromVariantMap(settings.value(QStringLiteral("user")).toMap()));
        m_teams.append(c);
        adopt(c);
    }
    settings.endArray();
    emit loaded();
    for (Connection *c : std::as_const(m_teams))
        restore(c);
    m_core->start();
}

// Signs in again with the kept token, and keeps the connection's sign-in
// fresh: a server that can't be reached leaves it Offline, and it tries
// again when asked to.
void ConnectionManager::restore(Connection *c)
{
    QString err;
    const auto token = SecretStore::read(secretKey(c), &err);
    if (!token) {
        c->setState(Connection::State::SignedOut, err.isEmpty() ? tr("Sign in to work on this server.") : err);
        return;
    }
    c->api()->setToken(*token);
    c->setState(Connection::State::Connecting);
    onResult(c->api()->get(QStringLiteral("/api/server")), c, [c](const ApiResult &r) {
        if (r.ok())
            c->setServerInfo(r.object().value(QLatin1String("version")).toString(), r.object().value(QLatin1String("provider")).toString());
    });
    onResult(c->api()->get(QStringLiteral("/api/me")), c, [this, c, t = *token](const ApiResult &r) {
        if (r.ok()) {
            c->activate(t, r.object().value(QLatin1String("user")).toObject());
            save();
        } else if (r.error.offline()) {
            c->setState(Connection::State::Offline, r.error.message);
        } else {
            c->setState(Connection::State::SignedOut, r.error.message);
        }
    });
}

void ConnectionManager::probe(const QUrl &url, std::function<void(const ServerInfo &, const ApiError &)> done)
{
    auto *api = new ApiClient(url, QString(), this);
    onResult(api->get(QStringLiteral("/api/server")), this, [api, url, done](const ApiResult &r) {
        api->deleteLater();
        ServerInfo info;
        info.url = url;
        if (!r.ok()) {
            done(info, r.error);
            return;
        }
        const QJsonObject o = r.object();
        if (o.value(QLatin1String("name")).toString() != QLatin1String("Interlinea")) {
            done(info, ApiError{r.status, QStringLiteral("notInterlinea"), tr("%1 is not an Interlinea server.").arg(url.toString()), {}});
            return;
        }
        info.version = o.value(QLatin1String("version")).toString();
        info.provider = o.value(QLatin1String("provider")).toString();
        info.setup = o.value(QLatin1String("setup")).toBool();
        done(info, {});
    });
}

Connection *ConnectionManager::teamFor(const QUrl &url)
{
    for (Connection *c : std::as_const(m_teams)) {
        if (c->url().matches(url, QUrl::StripTrailingSlash | QUrl::NormalizePathSegments))
            return c;
    }
    auto *c = new Connection(Connection::Kind::Team, QUuid::createUuid().toString(QUuid::WithoutBraces), url, this);
    m_teams.append(c);
    adopt(c);
    emit added(c);
    return c;
}

void ConnectionManager::signedIn(const QUrl &url, const ApiResult &r, Done done)
{
    if (!r.ok()) {
        done(nullptr, r.error);
        return;
    }
    const QString token = r.object().value(QLatin1String("token")).toString();
    const QJsonObject user = r.object().value(QLatin1String("user")).toObject();
    if (token.isEmpty()) {
        done(nullptr, ApiError{r.status, QStringLiteral("badAnswer"), tr("The server's answer has no sign-in."), {}});
        return;
    }
    Connection *c = teamFor(url);
    QString err;
    if (!SecretStore::write(secretKey(c), token, &err)) {
        // Signed in for now; the next start asks again.
        c->activate(token, user);
        save();
        done(c, ApiError{0, QStringLiteral("notKept"), tr("Signed in, but the sign-in could not be kept: %1").arg(err), {}});
        return;
    }
    c->activate(token, user);
    save();
    onResult(c->api()->get(QStringLiteral("/api/server")), c, [c](const ApiResult &res) {
        if (res.ok())
            c->setServerInfo(res.object().value(QLatin1String("version")).toString(), res.object().value(QLatin1String("provider")).toString());
    });
    done(c, {});
}

void ConnectionManager::signInWithPassword(const QUrl &url, const QString &username, const QString &password, Done done)
{
    auto *api = new ApiClient(url, QString(), this);
    QJsonObject body{{QStringLiteral("username"), username}, {QStringLiteral("password"), password}, {QStringLiteral("device"), deviceName()}};
    onResult(api->post(QStringLiteral("/api/auth/password"), body), this, [this, api, url, done](const ApiResult &r) {
        api->deleteLater();
        signedIn(url, r, done);
    });
}

void ConnectionManager::setUpServer(const QUrl &url, const QString &setupToken, const QString &username, const QString &name,
                                    const QString &email, const QString &password, Done done)
{
    auto *api = new ApiClient(url, QString(), this);
    QJsonObject body{{QStringLiteral("setupToken"), setupToken}, {QStringLiteral("username"), username}, {QStringLiteral("name"), name},
                     {QStringLiteral("email"), email},         {QStringLiteral("password"), password}, {QStringLiteral("device"), deviceName()}};
    onResult(api->post(QStringLiteral("/api/setup"), body), this, [this, api, url, done](const ApiResult &r) {
        api->deleteLater();
        signedIn(url, r, done);
    });
}

void ConnectionManager::signInWithProvider(const QUrl &url, Done done)
{
    cancelProviderSignIn();
    auto *receiver = new ProviderSignIn(this);
    m_provider = receiver;
    QString err;
    if (!receiver->listen(&err)) {
        receiver->deleteLater();
        done(nullptr, ApiError{0, QStringLiteral("local"), tr("The sign-in can't come back to the app: %1").arg(err), {}});
        return;
    }
    auto *api = new ApiClient(url, QString(), receiver);
    connect(receiver, &ProviderSignIn::code, this, [this, api, url, receiver, done](const QString &code) {
        onResult(api->post(QStringLiteral("/api/auth/exchange"), QJsonObject{{QStringLiteral("code"), code}}), this,
                 [this, url, receiver, done](const ApiResult &r) {
                     receiver->deleteLater();
                     signedIn(url, r, done);
                 });
    });
    connect(receiver, &ProviderSignIn::failed, this, [receiver, done](const QString &message) {
        receiver->deleteLater();
        done(nullptr, ApiError{0, QStringLiteral("provider"), message, {}});
    });
    const QJsonObject body{{QStringLiteral("return"), receiver->returnTo().toString()}, {QStringLiteral("device"), deviceName()}};
    onResult(api->post(QStringLiteral("/api/auth/oidc/start"), body), receiver, [this, receiver, done](const ApiResult &r) {
        if (!r.ok()) {
            receiver->deleteLater();
            done(nullptr, r.error);
            return;
        }
        const QUrl to(r.object().value(QLatin1String("url")).toString());
        if (!m_open(to)) {
            receiver->deleteLater();
            done(nullptr, ApiError{0, QStringLiteral("browser"), tr("The browser could not be opened. Open this address: %1").arg(to.toString()), {}});
        }
    });
}

void ConnectionManager::cancelProviderSignIn()
{
    if (m_provider) {
        m_provider->close();
        m_provider->deleteLater();
    }
}

void ConnectionManager::signOut(Connection *c)
{
    if (!c || c->isLocal())
        return;
    if (c->isReady())
        c->api()->post(QStringLiteral("/api/auth/signout"));
    c->api()->setToken(QString());
    SecretStore::remove(secretKey(c));
    c->setState(Connection::State::SignedOut, tr("Signed out."));
}

void ConnectionManager::remove(Connection *c)
{
    if (!c || c->isLocal() || !m_teams.contains(c))
        return;
    signOut(c);
    m_teams.removeAll(c);
    const QString id = c->id();
    save();
    emit removed(id);
    c->deleteLater();
}

void ConnectionManager::save()
{
    QSettings settings;
    settings.beginWriteArray(QStringLiteral("servers"), int(m_teams.size()));
    for (int i = 0; i < m_teams.size(); ++i) {
        settings.setArrayIndex(i);
        settings.setValue(QStringLiteral("id"), m_teams[i]->id());
        settings.setValue(QStringLiteral("url"), m_teams[i]->url().toString());
        settings.setValue(QStringLiteral("user"), m_teams[i]->user().toVariantMap());
    }
    settings.endArray();
}
