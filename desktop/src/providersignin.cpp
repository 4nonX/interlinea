#include "providersignin.h"

#include <QCoreApplication>
#include <QTcpServer>
#include <QTcpSocket>
#include <QUrlQuery>


ProviderSignIn::ProviderSignIn(QObject *parent)
    : QObject(parent)
{
    m_timeout.setSingleShot(true);
    m_timeout.setInterval(10 * 60 * 1000);
    connect(&m_timeout, &QTimer::timeout, this, [this] {
        if (!m_done) {
            m_done = true;
            emit failed(tr("The sign-in was not finished within 10 minutes."));
        }
        close();
    });
}

ProviderSignIn::~ProviderSignIn()
{
    close();
}

bool ProviderSignIn::listen(QString *error)
{
    close();
    m_done = false;
    m_server = new QTcpServer(this);
    if (!m_server->listen(QHostAddress::LocalHost, 0)) {
        if (error)
            *error = m_server->errorString();
        delete m_server;
        m_server = nullptr;
        return false;
    }
    connect(m_server, &QTcpServer::newConnection, this, &ProviderSignIn::onConnection);
    m_timeout.start();
    return true;
}

QUrl ProviderSignIn::returnTo() const
{
    if (!m_server)
        return {};
    return QUrl(QStringLiteral("http://127.0.0.1:%1/").arg(m_server->serverPort()));
}

void ProviderSignIn::close()
{
    m_timeout.stop();
    if (m_server) {
        m_server->close();
        m_server->deleteLater();
        m_server = nullptr;
    }
}

void ProviderSignIn::onConnection()
{
    while (QTcpSocket *s = m_server ? m_server->nextPendingConnection() : nullptr) {
        connect(s, &QTcpSocket::readyRead, this, [this, s] { answer(s); });
        connect(s, &QTcpSocket::disconnected, s, &QObject::deleteLater);
    }
}

static void respond(QTcpSocket *s, const QByteArray &status, const QByteArray &type, const QByteArray &body)
{
    QByteArray out = "HTTP/1.1 " + status + "\r\nContent-Type: " + type + "\r\nContent-Length: " + QByteArray::number(body.size()) +
        "\r\nCache-Control: no-store\r\nConnection: close\r\n"
        "Content-Security-Policy: default-src 'none'; script-src 'unsafe-inline'; connect-src 'self'; style-src 'unsafe-inline'\r\n\r\n" +
        body;
    s->write(out);
    s->disconnectFromHost();
}

void ProviderSignIn::answer(QTcpSocket *s)
{
    // Requests are small: wait for the whole head.
    if (!s->canReadLine() || !s->peek(8192).contains("\r\n\r\n"))
        return;
    const QByteArray line = s->readLine().trimmed();
    s->readAll();
    const QList<QByteArray> parts = line.split(' ');
    if (parts.size() < 2 || parts[0] != "GET") {
        respond(s, "405 Method Not Allowed", "text/plain; charset=utf-8", "");
        return;
    }
    const QUrl url(QString::fromUtf8(parts[1]));
    if (url.path() == QLatin1String("/")) {
        // The result is in the fragment; the page sends it here.
        const QByteArray page =
            "<!doctype html><meta charset=utf-8><title>Interlinea</title>"
            "<body style=\"font:16px system-ui,sans-serif;margin:3em\"><p id=m>" +
            tr("Signing in…").toUtf8() +
            "</p><script>fetch('/result?'+location.hash.slice(1)).then(function(){document.getElementById('m').textContent=" +
            QByteArray("\"") + tr("You can close this window and go back to Interlinea.").toUtf8() + "\"})</script>";
        respond(s, "200 OK", "text/html; charset=utf-8", page);
        return;
    }
    if (url.path() != QLatin1String("/result")) {
        respond(s, "404 Not Found", "text/plain; charset=utf-8", "");
        return;
    }
    respond(s, "204 No Content", "text/plain; charset=utf-8", "");
    if (m_done)
        return;
    const QUrlQuery q(url);
    // The server writes the value as a query value ("+" for spaces).
    const auto value = [&q](const char *key) {
        QString v = q.queryItemValue(QLatin1String(key), QUrl::FullyEncoded);
        v.replace(QLatin1Char('+'), QLatin1Char(' '));
        return QUrl::fromPercentEncoding(v.toUtf8());
    };
    m_done = true;
    if (q.hasQueryItem(QStringLiteral("interlinea-code")))
        emit code(value("interlinea-code"));
    else if (q.hasQueryItem(QStringLiteral("interlinea-link")))
        emit linkCode(value("interlinea-link"));
    else if (q.hasQueryItem(QStringLiteral("interlinea-error")))
        emit failed(value("interlinea-error"));
    else
        emit failed(tr("The sign-in came back without a result."));
    // Answers are sent before the server goes.
    QTimer::singleShot(1000, this, &ProviderSignIn::close);
}
