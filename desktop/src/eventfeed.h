#pragma once

#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QTimer>

class ApiClient;
class QNetworkReply;

// The live feed of a connection (GET /api/events, server-sent events). It
// reconnects by itself, with the id of the last event seen, so nothing is
// missed; a "reset" asks the views to read everything again.
class EventFeed : public QObject {
    Q_OBJECT
public:
    explicit EventFeed(ApiClient *api, QObject *parent = nullptr);
    ~EventFeed() override;

    void start();
    void stop();
    bool isLive() const { return m_live; }
    QString lastId() const { return m_last; }

signals:
    // type is "project", "memory", "termbase", "account" or "presence".
    void event(const QString &type, const QJsonObject &data);
    // Too much changed while the feed was away: read everything again.
    void reset();
    void liveChanged(bool live);
    // The sign-in ended; the feed stops.
    void signedOut();

private:
    void connectNow();
    void onReadyRead();
    void onFinished();
    void dispatch(const QString &type, const QString &id, const QByteArray &data);
    void setLive(bool live);

    ApiClient *m_api;
    QPointer<QNetworkReply> m_reply;
    QTimer m_retry;
    // Restarted by everything the server sends; it fires when the
    // keep-alives stop coming.
    QTimer m_watchdog;
    QByteArray m_buffer;
    QString m_last;
    int m_wait = 1000;
    bool m_running = false;
    bool m_live = false;
};
