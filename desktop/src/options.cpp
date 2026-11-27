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

bool autoComplete()
{
    return QSettings().value(QStringLiteral("editor/autoComplete"), true).toBool();
}

void setAutoComplete(bool on)
{
    QSettings().setValue(QStringLiteral("editor/autoComplete"), on);
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

QString appearance()
{
    const QString a = QSettings().value(QStringLiteral("interface/appearance")).toString();
    return a == QLatin1String("light") || a == QLatin1String("dark") ? a : QString();
}

void setAppearance(const QString &appearance)
{
    if (appearance == QLatin1String("light") || appearance == QLatin1String("dark"))
        QSettings().setValue(QStringLiteral("interface/appearance"), appearance);
    else
        QSettings().remove(QStringLiteral("interface/appearance"));
}

} // namespace Options
