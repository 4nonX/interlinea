#include "backend.h"
#include "iconprovider.h"
#include "shothook.h"

#include "connectionmanager.h"
#include "corelauncher.h"
#include "interfacelanguage.h"
#include "singleinstance.h"

#include <QApplication>
#include <QDir>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QSettings>
#include <QTimer>

// The person's name for their account on this computer: the one they gave,
// else their login name.
static QString personName()
{
    const QString saved = QSettings().value(QStringLiteral("you/name")).toString();
    if (!saved.isEmpty())
        return saved;
    QString login = qEnvironmentVariable("USERNAME");
    if (login.isEmpty())
        login = qEnvironmentVariable("USER");
    return login;
}

int main(int argc, char *argv[])
{
    QApplication app(argc, argv);
    QApplication::setOrganizationName(QStringLiteral("Interlinea"));
    QApplication::setOrganizationDomain(QStringLiteral("interlinea.dev"));
    QApplication::setApplicationName(QStringLiteral("Interlinea"));
    QApplication::setApplicationVersion(QStringLiteral(INTERLINEA_VERSION));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/interlinea.png")));
    QGuiApplication::setDesktopFileName(QStringLiteral("dev.interlinea.Interlinea"));
    // The controls are the app's own (qml/controls), drawn the same on
    // every system; Basic is what they build on.
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    InterfaceLanguage::install(app);

    SingleInstance instance(QDir::homePath());
    if (!instance.claim(app.arguments().mid(1)))
        return 0;

    CoreLauncher core(CoreLauncher::defaultProgram(), CoreLauncher::defaultDataDir(), personName());
    ConnectionManager connections(&core);
    Backend backend(&connections);
    Backend::setInstance(&backend);

    QQmlApplicationEngine engine;
    engine.addImageProvider(QStringLiteral("icon"), new IconProvider);
    QObject::connect(&app, &QApplication::aboutToQuit, &core, &CoreLauncher::stop);
    engine.loadFromModule("Interlinea", "Main");
    if (engine.rootObjects().isEmpty())
        return 1;
    connections.start();
    // INTERLINEA_SHOT: a picture of the window, for checking the layout
    // (shothook.h).
    if (!qEnvironmentVariable("INTERLINEA_SHOT").isEmpty())
        installShotHook(qobject_cast<QQuickWindow *>(engine.rootObjects().first()), &backend);
    return app.exec();
}
