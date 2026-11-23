#include "apiclient.h"
#include "testcore.h"

#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTest>

// Waits for a reply's result.
static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

class TestApiClient : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QString why;
        QVERIFY2(m_core.start(&why), qPrintable(why));
        m_api = new ApiClient(m_core.launcher.handshake().url, m_core.launcher.handshake().token, this);
    }

    void signedIn()
    {
        const ApiResult r = wait(m_api->get(QStringLiteral("/api/me")));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QCOMPARE(r.object().value("user").toObject().value("username").toString(), QStringLiteral("me"));
        QCOMPARE(r.object().value("user").toObject().value("name").toString(), QStringLiteral("Tess Tester"));
    }

    void refusedWithoutSignIn()
    {
        ApiClient anon(m_api->base(), QStringLiteral("il_not-a-token"));
        QSignalSpy out(&anon, &ApiClient::signedOut);
        const ApiResult r = wait(anon.get(QStringLiteral("/api/projects")));
        QCOMPARE(r.status, 401);
        QCOMPARE(r.error.code, QStringLiteral("signIn"));
        QVERIFY(!r.error.message.isEmpty());
        QCOMPARE(out.size(), 1);
    }

    void writesWithVersions()
    {
        ApiResult r = wait(m_api->post(QStringLiteral("/api/memories"),
                                       QJsonObject{{"name", "Weber DE-EN"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}}));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QCOMPARE(r.status, 201);
        const QString base = QStringLiteral("/api/memories/") + ApiClient::seg(QStringLiteral("Weber DE-EN"));
        r = wait(m_api->post(base + "/units", QJsonObject{{"source", QJsonArray{QJsonObject{{"text", "Der Mieter zahlt."}}}},
                                                          {"target", QJsonArray{QJsonObject{{"text", "The tenant pays."}}}}}));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        const qint64 id = r.object().value("id").toInteger();
        const int version = r.object().value("version").toInt();
        const QJsonObject change{{"source", QJsonArray{QJsonObject{{"text", "Der Mieter zahlt."}}}},
                                 {"target", QJsonArray{QJsonObject{{"text", "The tenant pays the rent."}}}}};
        r = wait(m_api->put(base + QStringLiteral("/units/%1").arg(id), change, version));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        // A write from the old version is refused.
        r = wait(m_api->put(base + QStringLiteral("/units/%1").arg(id), change, version));
        QCOMPARE(r.status, 409);
        QCOMPARE(r.error.code, QStringLiteral("conflict"));
    }

    void filesUpAndDown()
    {
        const QByteArray tmx = "<?xml version=\"1.0\"?><tmx version=\"1.4\"><header srclang=\"de-DE\" datatype=\"plaintext\" segtype=\"sentence\" "
                               "adminlang=\"en\" o-tmf=\"x\" creationtool=\"t\" creationtoolversion=\"1\"/><body>"
                               "<tu><tuv xml:lang=\"de-DE\"><seg>Die Kaution beträgt drei Monatsmieten.</seg></tuv>"
                               "<tuv xml:lang=\"en-GB\"><seg>The deposit is three months' rent.</seg></tuv></tu></body></tmx>";
        const QString base = QStringLiteral("/api/memories/") + ApiClient::seg(QStringLiteral("Weber DE-EN"));
        ApiResult r = wait(m_api->upload("POST", base + "/import", tmx, QStringLiteral("application/x-tmx+xml")));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QCOMPARE(r.object().value("stored").toInt(), 1);
        r = wait(m_api->get(base + "/export"));
        QVERIFY(r.ok());
        QCOMPARE(r.fileName(), QStringLiteral("Weber DE-EN.tmx"));
        QVERIFY(r.body.contains("three months"));
        // A document is inspected without being kept.
        r = wait(m_api->upload("POST", "/api/files/inspect", "Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters.",
                               QStringLiteral("text/plain"), QUrlQuery{{"name", "Hinweise.txt"}}));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QCOMPARE(r.object().value("format").toString(), QStringLiteral("txt"));
        QCOMPARE(r.object().value("detection").toObject().value("lang").toString(), QStringLiteral("de-DE"));
    }

    void problemsByField()
    {
        const ApiResult r = wait(m_api->post(QStringLiteral("/api/memories"), QJsonObject{{"name", ""}}));
        QCOMPARE(r.status, 422);
        QCOMPARE(r.error.code, QStringLiteral("invalid"));
        QVERIFY(!r.error.problemWith(QStringLiteral("name")).isEmpty());
    }

    void offline()
    {
        // A server that hangs up without answering.
        QTcpServer rude;
        QVERIFY(rude.listen(QHostAddress::LocalHost, 0));
        connect(&rude, &QTcpServer::newConnection, &rude, [&rude] {
            while (QTcpSocket *s = rude.nextPendingConnection()) {
                s->close();
                s->deleteLater();
            }
        });
        ApiClient gone(QUrl(QStringLiteral("http://127.0.0.1:%1").arg(rude.serverPort())), QString());
        const ApiResult r = wait(gone.get(QStringLiteral("/api/server")));
        QVERIFY2(r.error.offline(), qPrintable(QStringLiteral("%1 (%2): %3").arg(r.error.code).arg(r.status).arg(r.error.message)));
        QVERIFY(!r.error.message.isEmpty());
    }

    void escapesSegments()
    {
        QCOMPARE(ApiClient::seg(QStringLiteral("Kanzlei/Weber DE")), QStringLiteral("Kanzlei%2FWeber%20DE"));
    }

private:
    TestCore m_core;
    ApiClient *m_api = nullptr;
};

QTEST_GUILESS_MAIN(TestApiClient)
#include "tst_apiclient.moc"
