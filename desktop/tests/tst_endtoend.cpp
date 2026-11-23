// The whole app against a real team server (INTERLINEA_TEAM_SERVER, with
// the administrator INTERLINEA_TEAM_USER / INTERLINEA_TEAM_PASSWORD) and
// its own core: sign in, make a project on each from a document, translate
// on the server, and get the translated document back. Skipped without a
// team server.
#include "apiclient.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "mainwindow.h"
#include "newprojectdialog.h"
#include "projectpage.h"
#include "projectsmodel.h"
#include "projectspage.h"
#include "settingseditor.h"
#include "settingspage.h"
#include "testcore.h"
#include "theme.h"

#include <QApplication>
#include <QComboBox>
#include <QFile>
#include <QLineEdit>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStackedWidget>
#include <QStandardPaths>
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
        if (b->text().remove(QLatin1Char('&')) == text)
            return b;
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
