#include "apiclient.h"
#include "eventfeed.h"
#include "testcore.h"

#include <QSignalSpy>
#include <QTest>

class TestEventFeed : public QObject {
    Q_OBJECT
private slots:
    void changesArrive()
    {
        TestCore core;
        QString why;
        QVERIFY2(core.start(&why), qPrintable(why));
        ApiClient api(core.launcher.handshake().url, core.launcher.handshake().token);
        EventFeed feed(&api);
        QSignalSpy live(&feed, &EventFeed::liveChanged);
        QSignalSpy events(&feed, &EventFeed::event);
        feed.start();
        QVERIFY(live.wait(10000));
        QVERIFY(feed.isLive());
        api.post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Hofmann"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}});
        QTRY_VERIFY_WITH_TIMEOUT(!events.isEmpty(), 10000);
        QCOMPARE(events.first().at(0).toString(), QStringLiteral("memory"));
        QCOMPARE(events.first().at(1).toJsonObject().value("memory").toString(), QStringLiteral("Hofmann"));
        QVERIFY(!feed.lastId().isEmpty());

        // Gone and back: what happened meanwhile still arrives.
        feed.stop();
        QVERIFY(!feed.isLive());
        events.clear();
        api.post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Second"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}});
        QTest::qWait(500);
        feed.start();
        QTRY_VERIFY_WITH_TIMEOUT(!events.isEmpty(), 10000);
        QCOMPARE(events.first().at(1).toJsonObject().value("memory").toString(), QStringLiteral("Second"));
    }

    void signedOut()
    {
        TestCore core;
        QString why;
        QVERIFY2(core.start(&why), qPrintable(why));
        ApiClient api(core.launcher.handshake().url, QStringLiteral("il_wrong"));
        EventFeed feed(&api);
        QSignalSpy out(&feed, &EventFeed::signedOut);
        feed.start();
        QVERIFY(out.wait(10000));
        QVERIFY(!feed.isLive());
    }
};

QTEST_GUILESS_MAIN(TestEventFeed)
#include "tst_eventfeed.moc"
