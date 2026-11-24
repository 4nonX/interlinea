#include "connection.h"

#include "apiclient.h"
#include "eventfeed.h"

#include <QCoreApplication>

Connection::Connection(Kind kind, const QString &id, const QUrl &url, QObject *parent)
    : QObject(parent), m_kind(kind), m_id(id), m_url(url),
      m_api(new ApiClient(url, QString(), this)), m_feed(new EventFeed(m_api, this))
{
    connect(m_api, &ApiClient::signedOut, this, [this] {
        m_feed->stop();
        setState(State::SignedOut, QCoreApplication::translate("Connection", "The sign-in has ended. Sign in again."));
    });
    connect(m_feed, &EventFeed::signedOut, this, [this] {
        setState(State::SignedOut, QCoreApplication::translate("Connection", "The sign-in has ended. Sign in again."));
    });
}

void Connection::setUrl(const QUrl &url)
{
    m_url = url;
    m_api->setBase(url);
}

QString Connection::displayName() const
{
    if (isLocal())
        return QCoreApplication::translate("Connection", "This computer");
    QString s = m_url.host();
    if (m_url.port() > 0)
        s += QLatin1Char(':') + QString::number(m_url.port());
    if (m_url.path().length() > 1)
        s += m_url.path();
    return s;
}

QString Connection::userName() const
{
    const QString name = m_user.value(QLatin1String("name")).toString();
    return name.isEmpty() ? m_user.value(QLatin1String("username")).toString() : name;
}

QString Connection::login() const
{
    return m_user.value(QLatin1String("username")).toString();
}

bool Connection::isAdmin() const
{
    return m_user.value(QLatin1String("admin")).toBool();
}

void Connection::activate(const QString &token, const QJsonObject &user)
{
    m_feed->stop();
    m_api->setToken(token);
    setUser(user);
    setState(State::Ready);
    m_feed->start();
}

void Connection::setUser(const QJsonObject &user)
{
    m_user = user;
    emit userChanged();
}

void Connection::setServerInfo(const QString &version, const QString &provider)
{
    m_version = version;
    m_provider = provider;
}

void Connection::setState(State state, const QString &message)
{
    if (state != State::Ready)
        m_feed->stop();
    if (m_state == state && m_message == message)
        return;
    m_state = state;
    m_message = message;
    emit stateChanged(state);
}
