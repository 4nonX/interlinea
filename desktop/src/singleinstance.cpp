#include "singleinstance.h"

#include <QCryptographicHash>
#include <QDataStream>
#include <QLocalServer>
#include <QLocalSocket>

SingleInstance::SingleInstance(const QString &key, QObject *parent)
    : QObject(parent)
{
    // Socket names have length limits on some systems: a hash fits.
    m_key = QStringLiteral("interlinea-") +
        QString::fromLatin1(QCryptographicHash::hash(key.toUtf8(), QCryptographicHash::Sha256).toHex().left(24));
}

SingleInstance::~SingleInstance()
{
    if (m_server)
        m_server->close();
}

bool SingleInstance::claim(const QStringList &args)
{
    {
        QLocalSocket s;
        s.connectToServer(m_key);
        if (s.waitForConnected(500)) {
            QByteArray msg;
            QDataStream out(&msg, QIODevice::WriteOnly);
            out << args;
            s.write(msg);
            s.waitForBytesWritten(1000);
            s.disconnectFromServer();
            return false;
        }
    }
    m_server = new QLocalServer(this);
    m_server->setSocketOptions(QLocalServer::UserAccessOption);
    if (!m_server->listen(m_key)) {
        // A server left behind by a crash.
        QLocalServer::removeServer(m_key);
        m_server->listen(m_key);
    }
    connect(m_server, &QLocalServer::newConnection, this, [this] {
        while (QLocalSocket *s = m_server->nextPendingConnection()) {
            connect(s, &QLocalSocket::disconnected, s, &QObject::deleteLater);
            connect(s, &QLocalSocket::readyRead, this, [this, s] {
                QDataStream in(s->readAll());
                QStringList args;
                in >> args;
                emit activated(args);
            });
        }
    });
    return true;
}
