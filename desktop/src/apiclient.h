#pragma once

#include "apierror.h"

#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QUrl>
#include <QUrlQuery>

#include <functional>
#include <optional>

class QNetworkAccessManager;
class QNetworkReply;
class QNetworkRequest;

// The answer to a request.
struct ApiResult {
    int status = 0;
    QByteArray body;
    QJsonDocument json;
    ApiError error;
    // Response headers, by lower-case name.
    QHash<QByteArray, QByteArray> headers;

    bool ok() const { return !error.isError(); }
    QJsonObject object() const { return json.object(); }
    QJsonArray array() const { return json.array(); }
    QByteArray header(const QByteArray &name) const { return headers.value(name.toLower()); }
    // The file name a download is offered under.
    QString fileName() const;
};

// A request in flight. It deletes itself after finished().
class ApiReply : public QObject {
    Q_OBJECT
public:
    explicit ApiReply(QNetworkReply *reply, QObject *parent = nullptr);
    void abort();
    bool isFinished() const { return m_finished; }
    const ApiResult &result() const { return m_result; }

signals:
    void finished(const ApiResult &result);
    void progress(qint64 done, qint64 total);

private:
    void finish();

    QPointer<QNetworkReply> m_reply;
    ApiResult m_result;
    bool m_finished = false;
};

// Calls fn with the reply's result, unless context is gone by then.
void onResult(ApiReply *reply, QObject *context, std::function<void(const ApiResult &)> fn);

// A client of the Interlinea API (the team server, or the desktop core,
// which serves the same API on this computer).
class ApiClient : public QObject {
    Q_OBJECT
public:
    ApiClient(const QUrl &base, const QString &token, QObject *parent = nullptr);

    QUrl base() const { return m_base; }
    void setBase(const QUrl &base);
    QString token() const { return m_token; }
    void setToken(const QString &token) { m_token = token; }
    QNetworkAccessManager *network() const { return m_net; }

    ApiReply *get(const QString &path, const QUrlQuery &query = {});
    ApiReply *post(const QString &path, const QJsonValue &body = {}, const QUrlQuery &query = {});
    ApiReply *put(const QString &path, const QJsonValue &body, std::optional<int> version = {}, const QUrlQuery &query = {});
    ApiReply *patch(const QString &path, const QJsonValue &body, std::optional<int> version = {});
    ApiReply *del(const QString &path, const QUrlQuery &query = {}, std::optional<int> version = {});
    // Sends bytes (a file) as the body.
    ApiReply *upload(const QByteArray &method, const QString &path, const QByteArray &data,
                     const QString &contentType = QStringLiteral("application/octet-stream"),
                     const QUrlQuery &query = {});
    // A request for path with the sign-in, for callers that read the
    // answer themselves (the live feed).
    QNetworkRequest request(const QString &path, const QUrlQuery &query = {}) const;

    // Path segments are escaped: names may contain "/" and spaces.
    static QString seg(const QString &s);

signals:
    // A request was refused because the sign-in ended.
    void signedOut();

private:
    ApiReply *send(const QByteArray &method, const QString &path, const QUrlQuery &query, const QByteArray &body,
                   const QString &contentType, std::optional<int> version);

    QNetworkAccessManager *m_net;
    QUrl m_base;
    QString m_token;
};
