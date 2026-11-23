#include "testcore.h"

#include <QCoreApplication>
#include <QDir>
#include <QSignalSpy>
#include <QTest>

QString testCoreProgram()
{
    const QString forced = qEnvironmentVariable("INTERLINEA_CORE");
    if (!forced.isEmpty())
        return forced;
    return CoreLauncher::defaultProgram();
}

TestCore::TestCore()
    : launcher(testCoreProgram(), dir.path() + QStringLiteral("/data"), QStringLiteral("Tess Tester"))
{
}

bool TestCore::start(QString *why)
{
    QSignalSpy started(&launcher, &CoreLauncher::started);
    QSignalSpy failed(&launcher, &CoreLauncher::failed);
    launcher.start();
    for (int i = 0; i < 300 && started.isEmpty() && failed.isEmpty(); ++i)
        QTest::qWait(100);
    if (started.isEmpty()) {
        *why = failed.isEmpty() ? QStringLiteral("no answer") : failed.first().first().toString();
        return false;
    }
    return true;
}
