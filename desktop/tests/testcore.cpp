#include "testcore.h"

#include "layoutcheck.h"

#include <cstdio>
#include <cstdlib>

#include <QApplication>
#include <QCoreApplication>
#include <QFile>
#include <QHash>
#include <QLabel>
#include <QSet>
#include <QTimer>
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

// Every window the tests show is looked over for broken layout (text cut
// off, parts over each other, buttons narrower than their labels): what
// stays wrong is listed when the test ends, and the test fails. With
// INTERLINEA_OVERFLOW set to a file, the findings also go there.
static QStringList *layoutFound()
{
    static QStringList found;
    return &found;
}

static void reportLayout()
{
    const QStringList &found = *layoutFound();
    if (found.isEmpty())
        return;
    fprintf(stderr, "FAIL!  : broken layout in %lld places:\n", static_cast<long long>(found.size()));
    for (const QString &line : found)
        fprintf(stderr, "   %s\n", qPrintable(line));
    fflush(stdout);
    fflush(stderr);
    std::_Exit(1);
}

static void startLayoutCheck()
{
    // What the test says goes out at once: into a pipe, as under ctest,
    // it would otherwise wait in a buffer a crash or a quick exit loses.
    setvbuf(stdout, nullptr, _IONBF, 0);
    const QString out = qEnvironmentVariable("INTERLINEA_OVERFLOW");
    LayoutCheck::watch([out](const QString &line) {
        layoutFound()->append(line);
        if (!out.isEmpty()) {
            QFile f(out);
            if (f.open(QIODevice::Append | QIODevice::Text))
                f.write(line.toUtf8() + "\n");
        }
    });
    qAddPostRoutine(reportLayout);
}
Q_COREAPP_STARTUP_FUNCTION(startLayoutCheck)

QString layoutProblems(QWidget *window)
{
    QTest::qWait(300);
    return LayoutCheck::problems(window).join(QLatin1Char('\n'));
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
