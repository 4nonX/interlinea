#include "connectionmanager.h"
#include "corelauncher.h"
#include "icons.h"
#include "mainwindow.h"
#include "accountspage.h"
#include "memoriespage.h"
#include "terminologypage.h"
#include "projectspage.h"
#include "settingspage.h"
#include "singleinstance.h"
#include "theme.h"

#include <QApplication>
#include <QDir>
#include <QIcon>
#include <QLibraryInfo>
#include <QLocale>
#include <QSettings>
#include <QStyle>
#include <QTimer>
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
    Theme::apply(app);
    QApplication::setOrganizationName(QStringLiteral("Interlinea"));
    QApplication::setOrganizationDomain(QStringLiteral("interlinea.org"));
    QApplication::setApplicationName(QStringLiteral("Interlinea"));
    QApplication::setApplicationVersion(QStringLiteral(INTERLINEA_VERSION));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/interlinea.png")));
    // The name of the desktop entry, so the windows are grouped under it.
    QGuiApplication::setDesktopFileName(QStringLiteral("org.interlinea.desktop"));

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
    auto *projects = new ProjectsPage(&connections, &window);
    window.addPage(QStringLiteral("projects"), QObject::tr("Projects"), Icons::get(QStringLiteral("projects")), projects,
                   QKeySequence(QStringLiteral("Ctrl+1")));
    window.addPage(QStringLiteral("memories"), QObject::tr("Memories"), Icons::get(QStringLiteral("memories")), new MemoriesPage(&connections, &window),
                   QKeySequence(QStringLiteral("Ctrl+2")));
    auto *terminology = new TerminologySection(&connections, &window);
    // "Review and merge" on a project leads to its termbase.
    QObject::connect(projects, &ProjectsPage::navigate, &window, [&window, terminology](const QString &page, const QString &connectionId, const QString &item) {
        if (page == QLatin1String("terminology"))
            terminology->open(connectionId, item);
        window.showPage(page);
    });
    window.addPage(QStringLiteral("terminology"), QObject::tr("Terminology"), Icons::get(QStringLiteral("terminology")), terminology,
                   QKeySequence(QStringLiteral("Ctrl+3")));
    auto *accounts = new AccountsPage(&connections, &window);
    window.addPage(QStringLiteral("accounts"), QObject::tr("Accounts"), Icons::get(QStringLiteral("accounts")), accounts, QKeySequence(QStringLiteral("Ctrl+4")));
    QObject::connect(accounts, &AccountsPage::openSettings, &window, [&window] { window.showPage(QStringLiteral("settings")); });
    window.addPage(QStringLiteral("settings"), QObject::tr("Settings"), Icons::get(QStringLiteral("settings")),
                   new SettingsPage(&connections, &window), QKeySequence(QStringLiteral("Ctrl+,")));
    QObject::connect(&instance, &SingleInstance::activated, &window, [&window, projects](const QStringList &files) {
        // Documents opened with Interlinea start a project.
        if (!files.isEmpty()) {
            window.showPage(QStringLiteral("projects"));
            projects->newProject(files);
        }
        window.showNormal();
        window.raise();
        window.activateWindow();
    });
    QObject::connect(&app, &QApplication::aboutToQuit, &core, &CoreLauncher::stop);

    connections.start();
    window.show();
    if (app.arguments().size() > 1) {
        QStringList files = app.arguments().mid(1);
        QTimer::singleShot(0, projects, [projects, files] { projects->newProject(files); });
    }
    return app.exec();
}
