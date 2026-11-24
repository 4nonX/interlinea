// The whole app against a real team server (INTERLINEA_TEAM_SERVER, with
// the administrator INTERLINEA_TEAM_USER / INTERLINEA_TEAM_PASSWORD) and
// its own core: sign in, keep a memory on the server, make a project on
// each from a document, translate on the server, and get the translated
// document back. Skipped without a team server.
#include "accesspanel.h"
#include "apiclient.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "mainwindow.h"
#include "memoriespage.h"
#include "newprojectdialog.h"
#include "projectpage.h"
#include "projectsmodel.h"
#include "projectspage.h"
#include "settingseditor.h"
#include "settingspage.h"
#include "testcore.h"
#include "theme.h"
#include "webui.h"

#include <QApplication>
#include <QComboBox>
#include <QCheckBox>
#include <QFile>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStackedWidget>
#include <QStandardPaths>
#include <QTableWidget>
#include <QTemporaryDir>
#include <QTest>
#include <QTreeWidget>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

template <typename T>
static T *named(QWidget *root, const QString &name)
{
    for (T *w : root->findChildren<T *>())
        if (w->accessibleName() == name)
            return w;
    return nullptr;
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>())
        if (b->text().remove(QLatin1Char('&')) == text && !b->isHidden())
            return b;
    return nullptr;
}

static QLabel *label(QWidget *root, const QString &text)
{
    for (QLabel *l : root->findChildren<QLabel *>())
        if (l->text() == text && l->isVisibleTo(root))
            return l;
    return nullptr;
}

class TestEndToEnd : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_endtoend"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
        Theme::apply(*qApp);
    }

    void wholeApp()
    {
        const QString server = qEnvironmentVariable("INTERLINEA_TEAM_SERVER");
        if (server.isEmpty())
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        TestCore core;
        ConnectionManager m(&core.launcher);
        MainWindow w(&m);
        auto *projects = new ProjectsPage(&m, &w);
        w.addPage(QStringLiteral("projects"), QStringLiteral("Projects"), QIcon(), projects);
        auto *memories = new MemoriesPage(&m, &w);
        w.addPage(QStringLiteral("memories"), QStringLiteral("Memories"), QIcon(), memories);
        w.addPage(QStringLiteral("settings"), QStringLiteral("Settings"), QIcon(), new SettingsPage(&m, &w));
        w.show();
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);

        // Sign in to the team server.
        Connection *team = nullptr;
        ApiError err;
        bool done = false;
        m.signInWithPassword(ConnectionManager::serverUrl(server), qEnvironmentVariable("INTERLINEA_TEAM_USER"),
                             qEnvironmentVariable("INTERLINEA_TEAM_PASSWORD"), [&](Connection *c, const ApiError &e) { team = c; err = e; done = true; });
        QTRY_VERIFY_WITH_TIMEOUT(done, 20000);
        QVERIFY2(team, qPrintable(err.message));
        QTRY_VERIFY_WITH_TIMEOUT(team->feed()->isLive(), 10000);

        // A memory kept on the team server: made, filled from TMX, searched,
        // shared, written out and deleted, all from the Memories page.
        w.showPage(QStringLiteral("memories"));
        const QString host = team->displayName();
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("On the team server")), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(button(memories, QStringLiteral("New memory on the server")), 10000);
        button(memories, QStringLiteral("New memory on the server"))->click();
        QTRY_VERIFY(button(memories, QStringLiteral("Make the memory")));
        QVERIFY(!button(memories, QStringLiteral("Make the memory"))->isEnabled());
        named<QLineEdit>(memories, QStringLiteral("Name"))->setText(QStringLiteral("Kanzlei Weber DE-EN"));
        button(memories, QStringLiteral("Make the memory"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Made the memory Kanzlei Weber DE-EN on %1").arg(host), 10000);
        // It is listed and opened, with who may use it for its manager.
        QTRY_COMPARE_WITH_TIMEOUT(memories->openMemory(team->id()), QStringLiteral("Kanzlei Weber DE-EN"), 10000);
        QTableWidget *listed = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((listed = named<QTableWidget>(memories, QStringLiteral("Memories on %1").arg(host))) && listed->rowCount() == 1, 10000);
        QCOMPARE(listed->item(0, 3)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("manage"));
        QTRY_VERIFY_WITH_TIMEOUT(memories->findChild<AccessPanel *>(), 10000);
        QVERIFY(button(memories, QStringLiteral("Close")));

        const QString tmxFile = m_tmp.filePath(QStringLiteral("Weber.tmx"));
        {
            QFile t(tmxFile);
            QVERIFY(t.open(QIODevice::WriteOnly));
            t.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tmx version=\"1.4\"><header creationtool=\"test\" creationtoolversion=\"1\" "
                    "datatype=\"plaintext\" segtype=\"sentence\" adminlang=\"en\" srclang=\"de-DE\" o-tmf=\"test\"/><body>"
                    "<tu><tuv xml:lang=\"de-DE\"><seg>Der Vermieter haftet nicht.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The landlord is not liable.</seg></tuv></tu>"
                    "<tu><tuv xml:lang=\"de-DE\"><seg>Die Nebenkosten sind fällig.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The service charges are due.</seg></tuv></tu>"
                    "</body></tmx>\n");
        }
        memories->importFile(team->id(), QStringLiteral("Kanzlei Weber DE-EN"), tmxFile);
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Read 2 units from Weber.tmx; 2 stored"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((listed = named<QTableWidget>(memories, QStringLiteral("Memories on %1").arg(host))) &&
                                     listed->item(0, 2)->data(Qt::AccessibleTextRole).toString() == QStringLiteral("2"),
                                 10000);
        // The open memory was not put away by the list being read again.
        QCOMPARE(memories->openMemory(team->id()), QStringLiteral("Kanzlei Weber DE-EN"));

        // Its units are searched, in the sources and in the translations.
        auto *query = named<QLineEdit>(memories, QStringLiteral("Search the units"));
        QVERIFY(query);
        QVERIFY(!button(memories, QStringLiteral("Search"))->isEnabled());
        query->setText(QStringLiteral("Nebenkosten"));
        button(memories, QStringLiteral("Search"))->click();
        QTableWidget *found = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((found = named<QTableWidget>(memories, QStringLiteral("Units found"))) && found->rowCount() == 1, 10000);
        QCOMPARE(found->item(0, 1)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("The service charges are due."));
        // When it changed, without a stray comma where the TMX names nobody.
        const QString changed = found->item(0, 2)->data(Qt::AccessibleTextRole).toString();
        QVERIFY2(!changed.isEmpty() && !changed.startsWith(QLatin1Char(',')), qPrintable(changed));
        query->setText(QStringLiteral("landlord"));
        button(memories, QStringLiteral("Search"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("Nothing found.")), 10000);
        for (QCheckBox *c : memories->findChildren<QCheckBox *>())
            if (c->text() == QStringLiteral("In the translations"))
                c->setChecked(true);
        button(memories, QStringLiteral("Search"))->click();
        QTRY_VERIFY_WITH_TIMEOUT((found = named<QTableWidget>(memories, QStringLiteral("Units found"))) && found->rowCount() == 1 &&
                                     found->item(0, 0)->data(Qt::AccessibleTextRole).toString() == QStringLiteral("Der Vermieter haftet nicht."),
                                 10000);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            w.resize(1440, 900);
            QTest::qWait(400);
            w.grab().save(dir + QStringLiteral("/memories-server-desktop.png"));
        }

        const QString written = m_tmp.filePath(QStringLiteral("Weber-out.tmx"));
        memories->exportFile(team->id(), QStringLiteral("Kanzlei Weber DE-EN"), written);
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Saved Weber-out.tmx"), 10000);
        {
            QFile t(written);
            QVERIFY(t.open(QIODevice::ReadOnly));
            QVERIFY(t.readAll().contains("The landlord is not liable."));
        }

        // Deleting it for everyone asks first.
        button(memories, QStringLiteral("Delete the memory"))->click();
        QTRY_VERIFY(label(memories, QStringLiteral("Delete Kanzlei Weber DE-EN and its 2 units on the server for everyone?")));
        for (Web::ConfirmButton *c : memories->findChildren<Web::ConfirmButton *>())
            if (c->asking())
                c->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Deleted Kanzlei Weber DE-EN on the server"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("No memories on %1 for you yet.").arg(host)), 10000);
        QVERIFY(memories->openMemory(team->id()).isEmpty());
        QVERIFY(!memories->findChild<AccessPanel *>() || memories->findChild<AccessPanel *>()->isHidden());
        w.showPage(QStringLiteral("projects"));

        const QString doc = m_tmp.filePath(QStringLiteral("Mietvertrag.txt"));
        QFile f(doc);
        QVERIFY(f.open(QIODevice::WriteOnly));
        f.write("Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats. Die Kaution beträgt drei Monatsmieten.\n");
        f.close();

        // A project on each: this computer and the team server.
        for (Connection *where : {m.local(), team}) {
            NewProjectDialog dlg(&m, &w);
            dlg.show();
            auto *first = dlg.findChild<QStackedWidget *>()->widget(0);
            auto *combo = named<QComboBox>(first, QStringLiteral("Kept on"));
            QTRY_VERIFY(combo->findData(where->id()) >= 0);
            combo->setCurrentIndex(combo->findData(where->id()));
            dlg.addFiles({doc});
            auto *files = named<QTreeWidget>(first, QStringLiteral("Documents"));
            QTRY_COMPARE_WITH_TIMEOUT(files->topLevelItem(0)->text(1), QStringLiteral("To translate"), 10000);
            button(&dlg, QStringLiteral("Next"))->click();
            auto *editor = dlg.findChild<SettingsEditor *>();
            QTRY_VERIFY_WITH_TIMEOUT(editor->isVisible(), 10000);
            named<LanguageCombo>(editor, QStringLiteral("Target language to add"))->setLanguage(QStringLiteral("en-GB"));
            button(editor, QStringLiteral("Add"))->click();
            QSignalSpy accepted(&dlg, &QDialog::accepted);
            button(&dlg, QStringLiteral("Make the project"))->click();
            QTRY_COMPARE_WITH_TIMEOUT(accepted.size(), 1, 20000);
            QCOMPARE(dlg.connectionId(), where->id());

            // Translate both segments where the project is kept.
            const QString base = QStringLiteral("/api/projects/%1").arg(dlg.projectId());
            const QUrlQuery fq{{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}};
            ApiResult r = wait(where->api()->get(base + "/segments", fq));
            QVERIFY2(r.ok(), qPrintable(r.error.message));
            QCOMPARE(r.array().size(), 2);
            const QStringList en{QStringLiteral("The tenant pays the rent by the third working day of each month."),
                                 QStringLiteral("The deposit is three months' rent.")};
            for (int i = 0; i < 2; ++i) {
                const QJsonObject s = r.array().at(i).toObject();
                QUrlQuery q = fq;
                q.addQueryItem("segment", s.value("id").toString());
                const ApiResult saved = wait(where->api()->put(base + "/segments", QJsonObject{{"target", QJsonArray{QJsonObject{{"text", en[i]}}}}, {"status", "confirmed"}},
                                                               s.value("version").toInt(), q));
                QVERIFY2(saved.ok(), qPrintable(saved.error.message));
            }
            // The project page shows it done, and the document comes back translated.
            projects->open(where->id(), dlg.projectId());
            auto *progress = named<QTreeWidget>(projects->openProject(), QStringLiteral("Files and their progress"));
            QTRY_VERIFY_WITH_TIMEOUT(progress->topLevelItemCount() == 1 && progress->topLevelItem(0)->childCount() == 1 &&
                                         progress->topLevelItem(0)->child(0)->text(4) == QStringLiteral("2 of 2"),
                                     15000);
            r = wait(where->api()->get(base + "/files/translated", QUrlQuery{{"name", "Mietvertrag.txt"}, {"lang", "en-GB"}}));
            QVERIFY2(r.ok(), qPrintable(r.error.message));
            QCOMPARE(QString::fromUtf8(r.body).trimmed(), en.join(QLatin1Char(' ')));
            // Confirmed translations went into the project's own memory.
            r = wait(where->api()->post(base + "/lookup", QJsonObject{{"lang", "en-GB"}, {"source", QJsonArray{QJsonObject{{"text", "Die Kaution beträgt drei Monatsmieten."}}}}}));
            QVERIFY2(r.ok() && !r.array().isEmpty(), qPrintable(r.error.message));
            QCOMPARE(r.array().at(0).toObject().value("score").toInt(), 100);
            projects->showList();
        }
        // Both projects are listed, each kept where it was made.
        QTRY_COMPARE_WITH_TIMEOUT(projects->model()->rowCount(), 2, 10000);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEndToEnd)
#include "tst_endtoend.moc"
