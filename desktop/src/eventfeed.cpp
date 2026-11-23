#include "eventfeed.h"

#include "apiclient.h"

#include <QJsonDocument>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QNetworkRequest>

EventFeed::EventFeed(ApiClient *api, QObject *parent)
    : QObject(parent), m_api(api)
{
    m_retry.setSingleShot(true);
    connect(&m_retry, &QTimer::timeout, this, &EventFeed::connectNow);
    m_watchdog.setSingleShot(true);
    m_watchdog.setInterval(70 * 1000);
    connect(&m_watchdog, &QTimer::timeout, this, [this] {
        if (m_reply)
            m_reply->abort(); // finished() reconnects
    });
}

EventFeed::~EventFeed()
{
    stop();
}

void EventFeed::start()
{
    if (m_running)
        return;
    m_running = true;
    m_wait = 1000;
    connectNow();
}

void EventFeed::stop()
{
    m_running = false;
    m_retry.stop();
    m_watchdog.stop();
    if (m_reply) {
        QNetworkReply *r = m_reply;
        m_reply = nullptr;
        r->disconnect(this);
        r->abort();
        r->deleteLater();
    }
    setLive(false);
}

void EventFeed::connectNow()
{
    if (!m_running)
        return;
    QNetworkRequest req = m_api->request(QStringLiteral("/api/events"));
    req.setRawHeader("Accept", "text/event-stream");
    if (!m_last.isEmpty())
        req.setRawHeader("Last-Event-ID", m_last.toUtf8());
    // The stream stays open; the server sends a keep-alive every 25 s, so
    // the watchdog takes a minute without anything as a lost connection.
    req.setTransferTimeout(0);
    m_watchdog.start();
    m_buffer.clear();
    m_reply = m_api->network()->get(req);
    connect(m_reply, &QNetworkReply::readyRead, this, &EventFeed::onReadyRead);
    connect(m_reply, &QNetworkReply::finished, this, &EventFeed::onFinished);
    connect(m_reply, &QNetworkReply::metaDataChanged, this, [this] {
        const int status = m_reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        if (status == 200) {
            setLive(true);
            m_wait = 1000;
        }
    });
}

void EventFeed::onReadyRead()
{
    m_watchdog.start();
    m_buffer += m_reply->readAll();
    for (;;) {
        // Events end with a blank line; servers may send \r\n.
        qsizetype cut = m_buffer.indexOf("\n\n");
        qsizetype skip = 2;
        const qsizetype crlf = m_buffer.indexOf("\r\n\r\n");
        if (crlf >= 0 && (cut < 0 || crlf < cut)) {
            cut = crlf;
            skip = 4;
        }
        if (cut < 0)
            break;
        const QByteArray block = m_buffer.left(cut);
        m_buffer.remove(0, cut + skip);
        QString type, id;
        QByteArray data;
        for (QByteArray line : block.split('\n')) {
            if (line.endsWith('\r'))
                line.chop(1);
            if (line.startsWith(':'))
                continue; // keep-alive
            const qsizetype colon = line.indexOf(':');
            const QByteArray field = colon < 0 ? line : line.left(colon);
            QByteArray value = colon < 0 ? QByteArray() : line.mid(colon + 1);
            if (value.startsWith(' '))
                value.remove(0, 1);
            if (field == "event")
                type = QString::fromUtf8(value);
            else if (field == "id")
                id = QString::fromUtf8(value);
            else if (field == "data")
                data += data.isEmpty() ? value : "\n" + value;
        }
        if (!type.isEmpty())
            dispatch(type, id, data);
        if (!m_running)
            return;
    }
}

void EventFeed::dispatch(const QString &type, const QString &id, const QByteArray &data)
{
    if (!id.isEmpty())
        m_last = id;
    const QJsonObject o = QJsonDocument::fromJson(data).object();
    if (type == QLatin1String("hello"))
        return;
    if (type == QLatin1String("signedOut")) {
        stop();
        emit signedOut();
        return;
    }
    if (type == QLatin1String("reset")) {
        emit reset();
        return;
    }
    emit event(type, o);
}

void EventFeed::onFinished()
{
    QNetworkReply *r = m_reply;
    if (!r)
        return;
    m_reply = nullptr;
    m_watchdog.stop();
    const int status = r->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    r->deleteLater();
    setLive(false);
    if (!m_running)
        return;
    if (status == 401) {
        m_running = false;
        emit signedOut();
        return;
    }
    // Reconnect, waiting longer each time up to half a minute.
    m_retry.start(m_wait);
    m_wait = qMin(m_wait * 2, 30000);
}

void EventFeed::setLive(bool live)
{
    if (m_live == live)
        return;
    m_live = live;
    emit liveChanged(live);
}
