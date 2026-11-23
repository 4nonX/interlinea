#pragma once

#include <QJsonObject>
#include <QList>
#include <QString>

// A problem with one field of a refused request.
struct Problem {
    QString field;
    QString message;
};

// Why the server (or the network) refused a request. Code is the server's
// stable code ("conflict", "invalid", "signIn", ...), or "offline" when the
// server could not be reached and "badAnswer" when its answer was not the
// API's.
struct ApiError {
    int status = 0;
    QString code;
    QString message;
    QList<Problem> problems;

    bool isError() const { return !code.isEmpty(); }
    bool offline() const { return code == QLatin1String("offline"); }

    // The message of the problem with a field, if any.
    QString problemWith(const QString &field) const;

    // Reads the server's error body.
    static ApiError fromBody(int status, const QByteArray &body);
    static ApiError offlineError(const QString &detail);
};
