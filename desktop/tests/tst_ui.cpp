#include "connectionmanager.h"
#include "mainwindow.h"
#include "navigationrail.h"
#include "settingspage.h"
#include "testcore.h"

#include <QAbstractButton>
#include <QApplication>
#include <QLabel>
#include <QLineEdit>
#include <QSettings>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QTest>
#include <QTreeWidget>

// Every control a person can reach says what it is: its text, an
// accessible name, or a label that names it.
static QStringList unnamed(QWidget *root)
{
    QStringList out;
    for (QWidget *w : root->findChildren<QWidget *>()) {
        if (!w->isVisibleTo(root) || w->focusPolicy() == Qt::NoFocus)
            continue;
        if (!w->accessibleName().isEmpty())
            continue;
        if (auto *b = qobject_cast<QAbstractButton *>(w); b && !b->text().isEmpty())
            continue;
        bool labelled = false;
        for (QLabel *l : root->findChildren<QLabel *>()) {
            if (l->buddy() == w)
                labelled = true;
        }
        if (!labelled)
            out << QStringLiteral("%1 %2").arg(QString::fromLatin1(w->metaObject()->className()), w->objectName());
    }
    return out;
}

class TestUi : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_ui"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_secrets.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void settings()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        MainWindow w(&m);
        auto *page = new SettingsPage(&m, &w);
        w.addPage(QStringLiteral("settings"), QStringLiteral("Settings"), QIcon(), page, QKeySequence(QStringLiteral("Ctrl+,")));
        w.show();
        m.start();
        QCOMPARE(w.currentPage(), QStringLiteral("settings"));
        QCOMPARE(w.navigation()->count(), 1);
        QTreeWidget *list = page->connectionList();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        QTRY_COMPARE(list->topLevelItem(0)->text(2), QStringLiteral("Ready"));
        QCOMPARE(list->topLevelItem(0)->text(0), QStringLiteral("This computer"));
        QCOMPARE(list->topLevelItem(0)->text(1), QStringLiteral("Tess Tester"));
        // The name field holds the person's name and saves it to the core.
        auto *name = page->findChild<QLineEdit *>();
        QVERIFY(name);
        QTRY_COMPARE(name->text(), QStringLiteral("Tess Tester"));
        name->setText(QStringLiteral("Tess T."));
        QTest::keyClick(name, Qt::Key_Return);
        QTRY_COMPARE_WITH_TIMEOUT(m.local()->userName(), QStringLiteral("Tess T."), 10000);
        QCOMPARE(QSettings().value(QStringLiteral("you/name")).toString(), QStringLiteral("Tess T."));
        const QStringList missing = unnamed(&w);
        QVERIFY2(missing.isEmpty(), qPrintable(missing.join(QStringLiteral(", "))));
    }

    void coreFailureShown()
    {
        QTemporaryDir dir;
        CoreLauncher launcher(dir.filePath(QStringLiteral("no-core")), dir.filePath(QStringLiteral("data")), QStringLiteral("x"));
        ConnectionManager m(&launcher);
        MainWindow w(&m);
        w.addPage(QStringLiteral("settings"), QStringLiteral("Settings"), QIcon(), new SettingsPage(&m, &w));
        w.show();
        m.start();
        bool shown = false;
        for (QLabel *l : w.findChildren<QLabel *>()) {
            if (l->isVisible() && l->text().startsWith(QStringLiteral("Work on this computer can't be opened")))
                shown = true;
        }
        QVERIFY(shown);
    }

private:
    QTemporaryDir m_secrets;
};

QTEST_MAIN(TestUi)
#include "tst_ui.moc"
