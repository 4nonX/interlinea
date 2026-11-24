#include "options.h"

#include <QSettings>

namespace Options {

bool autoInsert()
{
    return QSettings().value(QStringLiteral("editor/autoInsert"), true).toBool();
}

void setAutoInsert(bool on)
{
    QSettings().setValue(QStringLiteral("editor/autoInsert"), on);
}

bool autoPropagate()
{
    return QSettings().value(QStringLiteral("editor/autoPropagate"), true).toBool();
}

void setAutoPropagate(bool on)
{
    QSettings().setValue(QStringLiteral("editor/autoPropagate"), on);
}

QString contextRule()
{
    const QString rule = QSettings().value(QStringLiteral("projects/contextRule")).toString();
    return rule == QLatin1String("both") ? rule : QStringLiteral("previous");
}

void setContextRule(const QString &rule)
{
    QSettings().setValue(QStringLiteral("projects/contextRule"), rule == QLatin1String("both") ? rule : QStringLiteral("previous"));
}

} // namespace Options
