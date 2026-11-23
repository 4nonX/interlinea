#include "apierror.h"

#include <QCoreApplication>
#include <QJsonArray>
#include <QJsonDocument>

QString ApiError::problemWith(const QString &field) const
{
    for (const Problem &p : problems) {
        if (p.field == field)
            return p.message;
    }
    return {};
}

ApiError ApiError::fromBody(int status, const QByteArray &body)
{
    ApiError e;
    e.status = status;
    const QJsonDocument doc = QJsonDocument::fromJson(body);
    const QJsonObject o = doc.object();
    e.code = o.value(QLatin1String("code")).toString();
    e.message = o.value(QLatin1String("message")).toString();
    const QJsonArray ps = o.value(QLatin1String("problems")).toArray();
    for (const QJsonValue &v : ps) {
        const QJsonObject p = v.toObject();
        e.problems.append({p.value(QLatin1String("field")).toString(), p.value(QLatin1String("message")).toString()});
    }
    if (e.code.isEmpty()) {
        // Not the server's answer: a proxy's page, say.
        e.code = QStringLiteral("badAnswer");
        e.message = QCoreApplication::translate("ApiError", "The server answered %1.").arg(status);
    }
    return e;
}

ApiError ApiError::offlineError(const QString &detail)
{
    ApiError e;
    e.code = QStringLiteral("offline");
    e.message = detail.isEmpty()
        ? QCoreApplication::translate("ApiError", "The server can't be reached.")
        : QCoreApplication::translate("ApiError", "The server can't be reached (%1).").arg(detail);
    return e;
}
