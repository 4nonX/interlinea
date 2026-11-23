#include "apiclient.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "secretstore.h"
#include "testcore.h"

#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QTest>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

// The core serves the team server's API, so the tests sign in to it as if
// it were a team server too.
class TestConnections : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_connections"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_secrets.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void addresses()
    {
        QCOMPARE(ConnectionManager::serverUrl(QStringLiteral(" interlinea.example.com/ ")), QUrl(QStringLiteral("https://interlinea.example.com")));
        QCOMPARE(ConnectionManager::serverUrl(QStringLiteral("http://127.0.0.1:8080")), QUrl(QStringLiteral("http://127.0.0.1:8080")));
        QVERIFY(!ConnectionManager::serverUrl(QStringLiteral("  ")).isValid());
        QVERIFY(ConnectionManager::deviceName().startsWith(QStringLiteral("Interlinea on ")));
    }

    void localAndTeam()
    {
        TestCore core;
        QUrl coreUrl;
        {
            ConnectionManager m(&core.launcher);
            m.start();
            QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
            QCOMPARE(m.local()->userName(), QStringLiteral("Tess Tester"));
            QVERIFY(m.local()->isAdmin());
            coreUrl = m.local()->url();

            // An account to sign in with.
            ApiResult r = wait(m.local()->api()->post(QStringLiteral("/api/users"),
                                                      QJsonObject{{"username", "ben"}, {"name", "Ben Berger"}, {"password", "a long enough password"}}));
            QVERIFY2(r.ok(), qPrintable(r.error.message));

            ServerInfo info;
            ApiError probeError;
            bool probed = false;
            m.probe(coreUrl, [&](const ServerInfo &i, const ApiError &e) { info = i; probeError = e; probed = true; });
            QTRY_VERIFY(probed);
            QVERIFY2(!probeError.isError(), qPrintable(probeError.message));
            QVERIFY(!info.setup);

            // A wrong password is said so, and nothing is added.
            Connection *got = nullptr;
            ApiError err;
            bool done = false;
            m.signInWithPassword(coreUrl, QStringLiteral("ben"), QStringLiteral("wrong password here"), [&](Connection *c, const ApiError &e) { got = c; err = e; done = true; });
            QTRY_VERIFY(done);
            QVERIFY(!got);
            QCOMPARE(err.code, QStringLiteral("credentials"));
            QVERIFY(m.teams().isEmpty());

            QSignalSpy added(&m, &ConnectionManager::added);
            done = false;
            m.signInWithPassword(coreUrl, QStringLiteral("ben"), QStringLiteral("a long enough password"), [&](Connection *c, const ApiError &e) { got = c; err = e; done = true; });
            QTRY_VERIFY(done);
            QVERIFY2(got, qPrintable(err.message));
            QCOMPARE(added.size(), 1);
            QVERIFY(got->isReady());
            QCOMPARE(got->userName(), QStringLiteral("Ben Berger"));
            QVERIFY(SecretStore::read(QStringLiteral("server/") + got->id()).has_value());
            QTRY_VERIFY_WITH_TIMEOUT(got->feed()->isLive(), 10000);
        }
        core.launcher.stop();

        // The next start signs in again with the kept sign-in. The core has
        // a new port, so the remembered server is the old address; point it
        // at the new one.
        QString why;
        QVERIFY2(core.start(&why), qPrintable(why));
        {
            QSettings s;
            s.beginReadArray(QStringLiteral("servers"));
            s.setArrayIndex(0);
            const QString id = s.value(QStringLiteral("id")).toString();
            s.endArray();
            s.beginWriteArray(QStringLiteral("servers"), 1);
            s.setArrayIndex(0);
            s.setValue(QStringLiteral("id"), id);
            s.setValue(QStringLiteral("url"), core.launcher.handshake().url.toString());
            s.endArray();
        }
        // Tokens survive the core's restart for other accounts; only the
        // core's own sign-in is renewed.
        ConnectionManager m(&core.launcher);
        m.start();
        QCOMPARE(m.teams().size(), 1);
        Connection *ben = m.teams().first();
        QTRY_VERIFY_WITH_TIMEOUT(ben->isReady(), 20000);
        QCOMPARE(ben->userName(), QStringLiteral("Ben Berger"));

        m.signOut(ben);
        QCOMPARE(ben->state(), Connection::State::SignedOut);
        QVERIFY(!SecretStore::read(QStringLiteral("server/") + ben->id()).has_value());

        QSignalSpy removed(&m, &ConnectionManager::removed);
        m.remove(ben);
        QCOMPARE(removed.size(), 1);
        QVERIFY(m.teams().isEmpty());
        QSettings s;
        QCOMPARE(s.beginReadArray(QStringLiteral("servers")), 0);
        s.endArray();
    }

    void coreMissing()
    {
        QTemporaryDir dir;
        CoreLauncher launcher(dir.filePath(QStringLiteral("no-core")), dir.filePath(QStringLiteral("data")), QStringLiteral("x"));
        ConnectionManager m(&launcher);
        m.start();
        QCOMPARE(m.local()->state(), Connection::State::Failed);
        QVERIFY(m.local()->message().contains(QStringLiteral("missing")));
    }

private:
    QTemporaryDir m_secrets;
};

QTEST_GUILESS_MAIN(TestConnections)
#include "tst_connections.moc"
