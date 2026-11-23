#include "apiclient.h"

#include <QBuffer>
#include <QCoreApplication>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QRegularExpression>

QString ApiResult::fileName() const
{
    // Content-Disposition: attachment; filename="..." or filename*=utf-8''...
    const QString cd = QString::fromUtf8(header("content-disposition"));
    static const QRegularExpression ext(QStringLiteral("filename\\*=(?:utf-8|UTF-8)''([^;]+)"));
    static const QRegularExpression plain(QStringLiteral("filename=\"?([^\";]+)\"?"));
    if (auto m = ext.match(cd); m.hasMatch())
        return QUrl::fromPercentEncoding(m.captured(1).toUtf8());
    if (auto m = plain.match(cd); m.hasMatch())
        return m.captured(1);
    return {};
}

ApiReply::ApiReply(QNetworkReply *reply, QObject *parent)
    : QObject(parent), m_reply(reply)
{
    connect(reply, &QNetworkReply::finished, this, &ApiReply::finish);
    connect(reply, &QNetworkReply::downloadProgress, this, &ApiReply::progress);
    connect(reply, &QNetworkReply::uploadProgress, this, &ApiReply::progress);
}

void ApiReply::abort()
{
    if (m_reply)
        m_reply->abort();
}

void ApiReply::finish()
{
    QNetworkReply *r = m_reply;
    if (!r || m_finished)
        return;
    m_finished = true;
    m_result.status = r->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    m_result.body = r->readAll();
    for (const auto &pair : r->rawHeaderPairs())
        m_result.headers.insert(pair.first.toLower(), pair.second);
    if (m_result.status == 0) {
        m_result.error = r->error() == QNetworkReply::OperationCanceledError
            ? ApiError{0, QStringLiteral("cancelled"), QCoreApplication::translate("ApiError", "The request was cancelled."), {}}
            : ApiError::offlineError(r->errorString());
    } else if (m_result.status >= 400) {
        m_result.error = ApiError::fromBody(m_result.status, m_result.body);
    } else if (m_result.header("content-type").startsWith("application/json") && !m_result.body.isEmpty()) {
        QJsonParseError pe;
        m_result.json = QJsonDocument::fromJson(m_result.body, &pe);
        if (pe.error != QJsonParseError::NoError)
            m_result.error = {m_result.status, QStringLiteral("badAnswer"),
                              QCoreApplication::translate("ApiError", "The server's answer could not be read: %1").arg(pe.errorString()), {}};
    }
    r->deleteLater();
    emit finished(m_result);
    deleteLater();
}

void onResult(ApiReply *reply, QObject *context, std::function<void(const ApiResult &)> fn)
{
    QObject::connect(reply, &ApiReply::finished, context, [fn = std::move(fn)](const ApiResult &r) { fn(r); });
}

ApiClient::ApiClient(const QUrl &base, const QString &token, QObject *parent)
    : QObject(parent), m_net(new QNetworkAccessManager(this)), m_base(base), m_token(token)
{
    // Redirects are the API's own business (provider sign-in); requests
    // don't follow them.
    m_net->setRedirectPolicy(QNetworkRequest::ManualRedirectPolicy);
}

QString ApiClient::seg(const QString &s)
{
    return QString::fromLatin1(QUrl::toPercentEncoding(s));
}

QNetworkRequest ApiClient::request(const QString &path, const QUrlQuery &query) const
{
    QUrl url = m_base;
    QString p = url.path();
    if (p.endsWith(QLatin1Char('/')))
        p.chop(1);
    url.setPath(p + path, QUrl::TolerantMode);
    if (!query.isEmpty())
        url.setQuery(query);
    QNetworkRequest req(url);
    if (!m_token.isEmpty())
        req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());
    req.setRawHeader("Accept", "application/json");
    req.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    return req;
}

ApiReply *ApiClient::send(const QByteArray &method, const QString &path, const QUrlQuery &query, const QByteArray &body,
                          const QString &contentType, std::optional<int> version)
{
    QNetworkRequest req = request(path, query);
    if (!contentType.isEmpty())
        req.setHeader(QNetworkRequest::ContentTypeHeader, contentType);
    if (version)
        req.setRawHeader("If-Match", QByteArray::number(*version));
    // Large files take their time; ordinary requests don't.
    req.setTransferTimeout(body.size() > (4 << 20) ? 15 * 60 * 1000 : 2 * 60 * 1000);
    QNetworkReply *r = nullptr;
    if (method == "GET")
        r = m_net->get(req);
    else if (method == "DELETE" && body.isEmpty())
        r = m_net->deleteResource(req);
    else
        r = m_net->sendCustomRequest(req, method, body);
    auto *reply = new ApiReply(r, this);
    connect(reply, &ApiReply::finished, this, [this](const ApiResult &res) {
        if (res.status == 401 && res.error.code == QLatin1String("signIn"))
            emit signedOut();
    });
    return reply;
}

static QByteArray jsonBytes(const QJsonValue &v)
{
    if (v.isObject())
        return QJsonDocument(v.toObject()).toJson(QJsonDocument::Compact);
    if (v.isArray())
        return QJsonDocument(v.toArray()).toJson(QJsonDocument::Compact);
    return {};
}

ApiReply *ApiClient::get(const QString &path, const QUrlQuery &query)
{
    return send("GET", path, query, {}, {}, std::nullopt);
}

ApiReply *ApiClient::post(const QString &path, const QJsonValue &body, const QUrlQuery &query)
{
    const QByteArray b = jsonBytes(body);
    return send("POST", path, query, b, b.isEmpty() ? QString() : QStringLiteral("application/json"), std::nullopt);
}

ApiReply *ApiClient::put(const QString &path, const QJsonValue &body, std::optional<int> version, const QUrlQuery &query)
{
    return send("PUT", path, query, jsonBytes(body), QStringLiteral("application/json"), version);
}

ApiReply *ApiClient::patch(const QString &path, const QJsonValue &body, std::optional<int> version)
{
    return send("PATCH", path, {}, jsonBytes(body), QStringLiteral("application/json"), version);
}

ApiReply *ApiClient::del(const QString &path, const QUrlQuery &query, std::optional<int> version)
{
    return send("DELETE", path, query, {}, {}, version);
}

ApiReply *ApiClient::upload(const QByteArray &method, const QString &path, const QByteArray &data,
                            const QString &contentType, const QUrlQuery &query)
{
    return send(method, path, query, data, contentType, std::nullopt);
}
