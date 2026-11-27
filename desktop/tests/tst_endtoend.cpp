#include "testcore.h"

#include "connection.h"
#include "eventfeed.h"

#include <QFile>
#include <QJsonArray>
#include <QJsonObject>
#include <QQuickWindow>
#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTest>
#include <QUrl>
#include <QUrlQuery>

// The whole app against a real team server (INTERLINEA_TEAM_SERVER, with
// the administrator INTERLINEA_TEAM_USER / INTERLINEA_TEAM_PASSWORD) and
// its own core: sign in, keep a memory on the server, make a project there
// and translate it in the editor, get the translated document back; the
// accounts of the server; and its termbases. Skipped without a team server.
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
    }

    void wholeApp()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        Connection *team = signIn(app);
        if (!team)
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        ApiClient *api = team->api();
        for (const QString &old : {QStringLiteral("E2E Weber DE-EN")})
            waitFor(api->del(QStringLiteral("/api/memories/") + ApiClient::seg(old)));
        const QString host = team->displayName();

        // A memory kept on the team server, made from the Memories page.
        app.show(QStringLiteral("memories"));
        app.call(QStringLiteral("memoriesPage"), "reload");
        QObject *form = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((form = app.find(QStringLiteral("newMemory:") + team->id())), 10000);
        QVERIFY(app.click(app.find(form, QStringLiteral("newMemory"))));
        QVERIFY(app.type(app.find(form, QStringLiteral("memoryName")), QStringLiteral("E2E Weber DE-EN")));
        QTRY_VERIFY(app.find(form, QStringLiteral("createMemory"))->property("enabled").toBool());
        QVERIFY(app.click(app.find(form, QStringLiteral("createMemory"))));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Made the memory E2E Weber DE-EN on %1").arg(host), 10000);
        // Filled from a TMX file.
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
        app.call(QStringLiteral("memoriesPage"), "importFile", {team->id(), QStringLiteral("E2E Weber DE-EN"), QUrl::fromLocalFile(tmxFile).toString()});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Imported 2 units from Weber.tmx into E2E Weber DE-EN"), 10000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));

        // A project on the server using it, translated in the editor.
        QJsonObject settings = waitFor(api->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", QStringLiteral("E2E Hofmann %1").arg(QDateTime::currentMSecsSinceEpoch()));
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("memories", QJsonObject{{"en-GB", QJsonArray{QJsonObject{{"memory", "E2E Weber DE-EN"}, {"lookup", true}, {"update", true}}}}});
        const QByteArray txt = QStringLiteral("Der Vermieter haftet nicht. Die Nebenkosten sind fällig.\n").toUtf8();
        const ApiResult made = waitFor(api->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings},
            {"files", QJsonArray{QJsonObject{{"name", "Hofmann.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(txt.toBase64())}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        app.call(app.window, "openEditor", {team->id(), id, QStringLiteral("Hofmann.txt"), QStringLiteral("en-GB")});
        QObject *ed = app.find(QStringLiteral("editorPage"));
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("segments").toList().size(), 2, 15000);
        for (int i = 0; i < 2; ++i) {
            QTRY_VERIFY_WITH_TIMEOUT(app.find(QStringLiteral("targetEdit")) && !app.find(QStringLiteral("targetEdit"))->property("plainText").toString().isEmpty(), 15000);
            app.call(ed, "confirm");
            QTest::qWait(500);
        }
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("progress").toMap().value("done").toInt(), ed->property("progress").toMap().value("total").toInt(), 15000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        // The translated document comes back from the server.
        const ApiResult back = waitFor(api->get(QStringLiteral("/api/projects/%1/files/translated").arg(id), QUrlQuery{{"name", "Hofmann.txt"}, {"lang", "en-GB"}}));
        QVERIFY(back.ok());
        QCOMPARE(QString::fromUtf8(back.body).trimmed(), QStringLiteral("The landlord is not liable. The service charges are due."));
        waitFor(api->del(QStringLiteral("/api/projects/%1").arg(id)));
        waitFor(api->del(QStringLiteral("/api/memories/E2E%20Weber%20DE-EN")));
    }

    void accounts()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        // Without a team server the page says where accounts are kept.
        Connection *team = signIn(app);
        if (!team)
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        ApiClient *api = team->api();
        for (const QJsonValue &v : waitFor(api->get(QStringLiteral("/api/users"))).array())
            if (v.toObject().value("username").toString() == QStringLiteral("ben.review"))
                waitFor(api->del(QStringLiteral("/api/users/%1").arg(v.toObject().value("id").toInteger())));
        QTRY_VERIFY_WITH_TIMEOUT(app.window->property("admin").toBool(), 10000);
        app.show(QStringLiteral("accounts"));
        QObject *page = app.find(QStringLiteral("accountsPage"));
        app.call(page, "setConnection", {team->id()});
        QTRY_VERIFY_WITH_TIMEOUT(!page->property("users").toList().isEmpty(), 15000);
        // A new account: a password too short is said at its field.
        QVERIFY(app.type(QStringLiteral("newUsername"), QStringLiteral("ben.review")));
        QVERIFY(app.type(QStringLiteral("newName"), QStringLiteral("Ben Review")));
        QVERIFY(app.type(QStringLiteral("newPassword"), QStringLiteral("short")));
        QVERIFY(app.click(QStringLiteral("makeAccount")));
        QTRY_VERIFY_WITH_TIMEOUT(page->property("newProblems").toMap().contains("password"), 10000);
        QVERIFY(app.type(QStringLiteral("newPassword"), QStringLiteral("a long enough password")));
        QVERIFY(app.click(QStringLiteral("makeAccount")));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Made the account ben.review"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page->property("user").toMap().value("username").toString(), QStringLiteral("ben.review"), 10000);
        for (const QSize size : {QSize(1024, 700), QSize(1366, 860)}) {
            app.window->resize(size);
            const QString broken = layoutProblems(app.window);
            app.shot(QStringLiteral("accounts-%1").arg(size.width()));
            QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("at %1x%2:\n%3").arg(size.width()).arg(size.height()).arg(broken)));
        }
        // Deleted again.
        const qint64 ben = page->property("user").toMap().value("id").toLongLong();
        app.call(page, "attempt", {QStringLiteral("DELETE"), QStringLiteral("/api/users/%1").arg(ben), QVariant(), QStringLiteral("Deleted ben.review"), QVariant(), QVariant()});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Deleted ben.review"), 10000);
    }

    void terminologyOnTheServer()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        Connection *team = signIn(app);
        if (!team)
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        ApiClient *api = team->api();
        waitFor(api->del(QStringLiteral("/api/termbases/E2E%20Recht")));
        QVERIFY(waitFor(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "E2E Recht"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        app.show(QStringLiteral("terminology"));
        QObject *tb = app.find(QStringLiteral("terminology"));
        app.call(tb, "open", {team->id(), QStringLiteral("E2E Recht"), QString()});
        QTRY_VERIFY_WITH_TIMEOUT(tb->property("opened").toBool() && tb->property("isTeam").toBool() && !tb->property("loading").toBool(), 15000);
        app.call(tb, "addEntry", {QStringLiteral("Mietsache"), QStringLiteral("de-DE"), QStringLiteral("approved")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added “Mietsache”"), 10000);
        QCOMPARE(waitFor(api->get(QStringLiteral("/api/termbases/E2E%20Recht"))).object().value("entries").toInt(), 1);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        waitFor(api->del(QStringLiteral("/api/termbases/E2E%20Recht")));
    }

private:
    // Signs in to the team server as its administrator, as the sign-in
    // dialog does; null without a team server.
    Connection *signIn(QmlApp &app)
    {
        const QString server = qEnvironmentVariable("INTERLINEA_TEAM_SERVER");
        if (server.isEmpty())
            return nullptr;
        QSignalSpy signedIn(&app.backend, &Backend::signedIn);
        app.backend.signInWithPassword(app.backend.serverUrl(server), qEnvironmentVariable("INTERLINEA_TEAM_USER"), qEnvironmentVariable("INTERLINEA_TEAM_PASSWORD"));
        if (!signedIn.wait(20000))
            return nullptr;
        const QString id = signedIn.first().at(1).toString();
        if (id.isEmpty()) {
            qWarning("sign-in failed: %s", qPrintable(signedIn.first().at(2).toMap().value("message").toString()));
            return nullptr;
        }
        auto *team = qobject_cast<Connection *>(app.backend.connection(id));
        for (int i = 0; i < 100 && team && !team->feed()->isLive(); ++i)
            QTest::qWait(100);
        return team;
    }

    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEndToEnd)
#include "tst_endtoend.moc"
