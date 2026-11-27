#include "roles.h"

#include <QCoreApplication>
#include <QJsonObject>

namespace Roles {

QStringList permissions()
{
    return {QStringLiteral("read"), QStringLiteral("suggest"), QStringLiteral("edit"), QStringLiteral("approve"), QStringLiteral("manage")};
}

bool allows(const QString &role, const QString &need)
{
    const QStringList all = permissions();
    const int rank = int(all.indexOf(role));
    if (rank >= 0)
        return all.indexOf(need) >= 0 && all.indexOf(need) <= rank;
    return !need.isEmpty() && role.split(QLatin1Char('+')).contains(need);
}

QString text(const QString &name)
{
    if (name == QLatin1String("read"))
        return QCoreApplication::translate("Roles", "Read");
    if (name == QLatin1String("suggest"))
        return QCoreApplication::translate("Roles", "Suggest changes");
    if (name == QLatin1String("edit"))
        return QCoreApplication::translate("Roles", "Edit");
    if (name == QLatin1String("approve"))
        return QCoreApplication::translate("Roles", "Approve changes");
    if (name == QLatin1String("manage"))
        return QCoreApplication::translate("Roles", "Manage");
    // A set that is no built-in role: what it lets one do.
    if (name.contains(QLatin1Char('+'))) {
        QStringList parts;
        for (const QString &p : name.split(QLatin1Char('+')))
            parts << text(p);
        return parts.join(QStringLiteral(", "));
    }
    return name;
}

} // namespace Roles
