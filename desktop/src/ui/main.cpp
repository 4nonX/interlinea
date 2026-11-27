#include "backend.h"
#include "iconprovider.h"
#include "shothook.h"

#include "connectionmanager.h"
#include "corelauncher.h"
#include "interfacelanguage.h"
#include "singleinstance.h"

#include <QApplication>
#include <QFileOpenEvent>
#include <QDir>
#include <QFileInfo>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QSettings>
#include <QTimer>
#include <QtQml/qqmlextensionplugin.h>

Q_IMPORT_QML_PLUGIN(InterlineaPlugin)

// Files the system asks to open (macOS hands them over as events).
class FileOpener : public QObject {
public:
    explicit FileOpener(Backend *backend)
        : m_backend(backend)
    {
    }

protected:
    bool eventFilter(QObject *watched, QEvent *e) override
    {
        if (e->type() == QEvent::FileOpen) {
            m_backend->openFiles({static_cast<QFileOpenEvent *>(e)->file()});
            return true;
        }
        return QObject::eventFilter(watched, e);
    }

private:
    Backend *m_backend;
};

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
#ifdef Q_OS_MACOS
    // On macOS the settings are named after the domain, which was
    // interlinea.org before 0.5: what was saved there comes along once.
    {
        QSettings now;
        now.setFallbacksEnabled(false);
        if (now.allKeys().isEmpty()) {
            QSettings before(QSettings::NativeFormat, QSettings::UserScope, QStringLiteral("interlinea.org"), QStringLiteral("Interlinea"));
            before.setFallbacksEnabled(false);
            for (const QString &k : before.allKeys())
                now.setValue(k, before.value(k));
            now.sync();
        }
    }
#endif
    QApplication::setApplicationVersion(QStringLiteral(INTERLINEA_VERSION));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/interlinea.png")));
    QGuiApplication::setDesktopFileName(QStringLiteral("dev.interlinea.Interlinea"));
    // The controls are the app's own (qml/controls), drawn the same on
    // every system; Basic is what they build on.
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    InterfaceLanguage::install(app);
    Backend::applyAppearance();

    // Files named when started, as full paths: the Interlinea already
    // running may have been started elsewhere.
    QStringList files;
    for (const QString &a : app.arguments().mid(1))
        files << QFileInfo(a).absoluteFilePath();
    SingleInstance instance(QDir::homePath());
    if (!instance.claim(files))
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
    // Documents opened with Interlinea start a project, here or in the
    // Interlinea already running.
    FileOpener opener(&backend);
    app.installEventFilter(&opener);
    QObject::connect(&instance, &SingleInstance::activated, &backend, &Backend::openFiles);
    if (!files.isEmpty())
        QTimer::singleShot(0, &backend, [&backend, files] { backend.openFiles(files); });
    // INTERLINEA_SHOT: a picture of the window, for checking the layout
    // (shothook.h).
    if (!qEnvironmentVariable("INTERLINEA_SHOT").isEmpty())
        installShotHook(qobject_cast<QQuickWindow *>(engine.rootObjects().first()), &backend);
    return app.exec();
}
