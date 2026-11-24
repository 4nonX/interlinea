#include "roles.h"

#include <QComboBox>
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
    return name;
}

void fill(QComboBox *box, const QJsonArray &defined, const QString &current)
{
    box->clear();
    if (defined.isEmpty()) {
        for (const QString &p : permissions())
            box->addItem(text(p), p);
    } else {
        for (const QJsonValue &v : defined) {
            const QString name = v.toObject().value(QLatin1String("name")).toString();
            box->addItem(text(name), name);
        }
    }
    int at = box->findData(current);
    // A role that is no longer defined is still shown as what it is.
    if (at < 0 && !current.isEmpty()) {
        box->addItem(text(current), current);
        at = box->count() - 1;
    }
    box->setCurrentIndex(qMax(0, at));
}

} // namespace Roles
