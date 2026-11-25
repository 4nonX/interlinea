#include "testcore.h"

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

// With INTERLINEA_OVERFLOW set to a file, every window the tests show is
// looked over a few times a second for text that does not fit where it
// stands: a label lower or narrower than its text, or two texts of one
// row lying over each other. What is found is written to the file, once
// each. A way to find such places in every screen the tests walk through,
// at any scaling and in any theme.
static void watchOverflow()
{
    static bool on = false;
    const QString out = qEnvironmentVariable("INTERLINEA_OVERFLOW");
    if (on || out.isEmpty())
        return;
    on = true;
    auto *seen = new QSet<QString>;
    auto *timer = new QTimer(qApp);
    timer->setInterval(150);
    QObject::connect(timer, &QTimer::timeout, qApp, [out, seen] {
        const auto where = [](const QWidget *w) {
            QStringList path;
            for (const QWidget *p = w->parentWidget(); p && path.size() < 6; p = p->parentWidget()) {
                const QString n = !p->objectName().isEmpty() ? p->objectName() : QString::fromLatin1(p->metaObject()->className());
                if (!n.startsWith(QLatin1String("qt_")) && n != QLatin1String("QWidget"))
                    path.prepend(n);
            }
            return path.join(QLatin1Char('/'));
        };
        QStringList found;
        for (QWidget *top : QApplication::topLevelWidgets()) {
            if (!top->isVisible())
                continue;
            for (QLabel *l : top->findChildren<QLabel *>()) {
                if (!l->isVisible() || l->text().isEmpty() || l->width() < 2 || l->pixmap(Qt::ReturnByValue).width() > 0)
                    continue;
                const int need = l->wordWrap() ? l->heightForWidth(l->width()) : l->sizeHint().height();
                const QString text = l->text().left(60).replace(QLatin1Char('\n'), QLatin1Char(' '));
                if (need > l->height() + 2)
                    found << QStringLiteral("too low by %1px: \"%2\" in %3").arg(need - l->height()).arg(text, where(l));
                else if (!l->wordWrap() && l->sizeHint().width() > l->width() + 2 && l->textFormat() != Qt::RichText)
                    found << QStringLiteral("too narrow by %1px: \"%2\" in %3").arg(l->sizeHint().width() - l->width()).arg(text, where(l));
                // Another text of the same parent over this one.
                if (QWidget *parent = l->parentWidget()) {
                    for (QLabel *o : parent->findChildren<QLabel *>(Qt::FindDirectChildrenOnly)) {
                        if (o != l && o < l && o->isVisible() && !o->text().isEmpty() && o->geometry().intersects(l->geometry()) &&
                            o->geometry().intersected(l->geometry()).height() > 2 && o->geometry().intersected(l->geometry()).width() > 2)
                            found << QStringLiteral("over each other: \"%1\" and \"%2\" in %3").arg(text, o->text().left(40), where(l));
                    }
                }
            }
        }
        // A screen being laid out is not looked at: only what stays as
        // it is from one look to the next counts.
        static QHash<QString, int> held;
        for (auto it = held.begin(); it != held.end();) {
            if (found.contains(it.key()))
                ++it;
            else
                it = held.erase(it);
        }
        QFile f(out);
        for (const QString &line : std::as_const(found)) {
            if (++held[line] < 2 || seen->contains(line))
                continue;
            seen->insert(line);
            if (f.isOpen() || f.open(QIODevice::Append | QIODevice::Text))
                f.write(line.toUtf8() + "\n");
        }
    });
    timer->start();
}

TestCore::TestCore()
    : launcher(testCoreProgram(), dir.path() + QStringLiteral("/data"), QStringLiteral("Tess Tester"))
{
    watchOverflow();
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
