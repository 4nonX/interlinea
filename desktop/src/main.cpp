#include "connectionmanager.h"
#include "corelauncher.h"
#include "mainwindow.h"
#include "settingspage.h"
#include "singleinstance.h"

#include <QApplication>
#include <QDir>
#include <QIcon>
#include <QLibraryInfo>
#include <QLocale>
#include <QSettings>
#include <QStyle>
#include <QTranslator>

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
    QApplication::setOrganizationDomain(QStringLiteral("interlinea.org"));
    QApplication::setApplicationName(QStringLiteral("Interlinea"));
    QApplication::setApplicationVersion(QStringLiteral(INTERLINEA_VERSION));

    QTranslator qt, own;
    if (qt.load(QLocale(), QStringLiteral("qtbase"), QStringLiteral("_"), QLibraryInfo::path(QLibraryInfo::TranslationsPath)))
        app.installTranslator(&qt);
    if (own.load(QLocale(), QStringLiteral("interlinea"), QStringLiteral("_"), QStringLiteral(":/translations")))
        app.installTranslator(&own);

    SingleInstance instance(QDir::homePath());
    if (!instance.claim(app.arguments().mid(1)))
        return 0;

    CoreLauncher core(CoreLauncher::defaultProgram(), CoreLauncher::defaultDataDir(), personName());
    ConnectionManager connections(&core);

    MainWindow window(&connections);
    window.addPage(QStringLiteral("settings"), QObject::tr("Settings"), app.style()->standardIcon(QStyle::SP_FileDialogDetailedView),
                   new SettingsPage(&connections, &window), QKeySequence(QStringLiteral("Ctrl+,")));
    QObject::connect(&instance, &SingleInstance::activated, &window, [&window](const QStringList &) {
        window.showNormal();
        window.raise();
        window.activateWindow();
    });
    QObject::connect(&app, &QApplication::aboutToQuit, &core, &CoreLauncher::stop);

    connections.start();
    window.show();
    return app.exec();
}
