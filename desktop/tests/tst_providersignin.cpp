#include "providersignin.h"

#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QSignalSpy>
#include <QTest>

static QByteArray fetch(QNetworkAccessManager &net, const QUrl &url, int *status = nullptr)
{
    QNetworkReply *r = net.get(QNetworkRequest(url));
    QSignalSpy done(r, &QNetworkReply::finished);
    done.wait(5000);
    if (status)
        *status = r->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    const QByteArray body = r->readAll();
    r->deleteLater();
    return body;
}

class TestProviderSignIn : public QObject {
    Q_OBJECT
private slots:
    void codeComesBack()
    {
        ProviderSignIn p;
        QString err;
        QVERIFY2(p.listen(&err), qPrintable(err));
        const QUrl back = p.returnTo();
        QCOMPARE(back.host(), QStringLiteral("127.0.0.1"));
        QSignalSpy code(&p, &ProviderSignIn::code);
        QNetworkAccessManager net;
        int status = 0;
        // The browser opens the page; its script sends the fragment.
        const QByteArray page = fetch(net, back, &status);
        QCOMPARE(status, 200);
        QVERIFY(page.contains("location.hash"));
        fetch(net, back.resolved(QUrl(QStringLiteral("/result?interlinea-code=c0de_Abc-123"))), &status);
        QCOMPARE(status, 204);
        QTRY_COMPARE(code.size(), 1);
        QCOMPARE(code.first().first().toString(), QStringLiteral("c0de_Abc-123"));
    }

    void errorsAreSaid()
    {
        ProviderSignIn p;
        QVERIFY(p.listen());
        QSignalSpy failed(&p, &ProviderSignIn::failed);
        QNetworkAccessManager net;
        fetch(net, p.returnTo().resolved(QUrl(QStringLiteral("/result?interlinea-error=The+identity+provider+refused%3A+access_denied"))));
        QTRY_COMPARE(failed.size(), 1);
        QCOMPARE(failed.first().first().toString(), QStringLiteral("The identity provider refused: access_denied"));
    }

    void otherPathsRefused()
    {
        ProviderSignIn p;
        QVERIFY(p.listen());
        QNetworkAccessManager net;
        int status = 0;
        fetch(net, p.returnTo().resolved(QUrl(QStringLiteral("/other"))), &status);
        QCOMPARE(status, 404);
    }
};

QTEST_GUILESS_MAIN(TestProviderSignIn)
#include "tst_providersignin.moc"
