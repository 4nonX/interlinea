#pragma once

#include <QJsonArray>
#include <QString>
#include <QStringList>

class QComboBox;

// Roles are sets of permissions. Five roles come with the core, each a
// permission and the ones before it (read, suggest, edit, approve,
// manage); administrators define others. Where the core says what someone
// may do, it names a built-in role or writes the set as its permissions
// joined by "+" ("read+suggest+approve").
namespace Roles {
// The permissions, in their order.
QStringList permissions();
// Whether what the core says someone may do includes a permission.
bool allows(const QString &role, const QString &need);
// A permission or built-in role in words ("Suggest changes"); a role of
// one's own by its name.
QString text(const QString &name);
// Fills a choice of role with the roles a place has (as GET /api/roles
// lists them; the five built-in ones if that is empty) and chooses one.
void fill(QComboBox *box, const QJsonArray &defined, const QString &current);
}
