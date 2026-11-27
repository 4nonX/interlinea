#include "shothook.h"

#include "backend.h"

#include <QCoreApplication>
#include <QQuickItem>
#include <QQuickWindow>
#include <QTimer>

void installShotHook(QQuickWindow *window, Backend *backend)
{
    if (!window)
        return;
    if (const QStringList in = qEnvironmentVariable("INTERLINEA_SHOT_SIGNIN").split(QLatin1Char(',')); in.size() == 3) {
        QObject::connect(backend, &Backend::signedIn, backend, [](const QString &url, const QString &, const QVariantMap &error) {
            if (!error.value(QStringLiteral("message")).toString().isEmpty())
                qWarning("INTERLINEA_SHOT_SIGNIN: %s: %s", qPrintable(url), qPrintable(error.value(QStringLiteral("message")).toString()));
        });
        backend->signInWithPassword(in[0], in[1], in[2]);
    }
    if (const QString page = qEnvironmentVariable("INTERLINEA_SHOT_PAGE"); !page.isEmpty())
        window->setProperty("page", page);
    if (const QString size = qEnvironmentVariable("INTERLINEA_SHOT_SIZE"); !size.isEmpty())
        window->resize(size.section(QLatin1Char('x'), 0, 0).toInt(), size.section(QLatin1Char('x'), 1, 1).toInt());
    if (const QStringList names = qEnvironmentVariable("INTERLINEA_SHOT_OPEN").split(QLatin1Char('|'), Qt::SkipEmptyParts); !names.isEmpty()) {
        const QStringList calls = qEnvironmentVariable("INTERLINEA_SHOT_CALL", QStringLiteral("start")).split(QLatin1Char('|'));
        const QStringList argGroups = qEnvironmentVariable("INTERLINEA_SHOT_ARGS").split(QLatin1Char('|'));
        for (int i = 0; i < calls.size(); ++i) {
            const QByteArray call = calls[i].trimmed().toLatin1();
            const QString name = names.value(i, names.last());
            QVariantList args;
            for (const QString &a : argGroups.value(i).split(QLatin1Char(','), Qt::SkipEmptyParts))
                args << a;
            while (args.size() < 4)
                args << QVariant();
            QTimer::singleShot(2500 + 1500 * i, window, [window, name, call, args] {
                QObject *target = window->findChild<QObject *>(name);
                // Items made by views (list delegates) are only in the item tree.
                const std::function<QObject *(QQuickItem *)> find = [&](QQuickItem *in) -> QObject * {
                    for (QQuickItem *c : in->childItems()) {
                        if (c->objectName() == name)
                            return c;
                        if (QObject *o = find(c))
                            return o;
                    }
                    return nullptr;
                };
                if (!target)
                    target = find(window->contentItem());
                if (!target) {
                    qWarning("INTERLINEA_SHOT_OPEN: no object named %s", qPrintable(name));
                    return;
                }
                if (!QMetaObject::invokeMethod(target, call.constData()) && !QMetaObject::invokeMethod(target, call.constData(), Q_ARG(QVariant, args[0]))
                    && !QMetaObject::invokeMethod(target, call.constData(), Q_ARG(QVariant, args[0]), Q_ARG(QVariant, args[1]))
                    && !QMetaObject::invokeMethod(target, call.constData(), Q_ARG(QVariant, args[0]), Q_ARG(QVariant, args[1]), Q_ARG(QVariant, args[2]),
                                                  Q_ARG(QVariant, args[3])))
                    QMetaObject::invokeMethod(target, "open");
            });
        }
    }
    const int after = qEnvironmentVariableIntValue("INTERLINEA_SHOT_AFTER");
    QTimer::singleShot(after > 0 ? after : 4000, window, [window] {
        window->grabWindow().save(qEnvironmentVariable("INTERLINEA_SHOT"));
        QCoreApplication::quit();
    });
}
