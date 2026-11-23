#include "secretstore.h"

#include <QFile>
#include <QTemporaryDir>
#include <QTest>

class TestSecretStore : public QObject {
    Q_OBJECT
private slots:
    void fileStore()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath(QStringLiteral("secrets/sign-ins.json"));
        qputenv("INTERLINEA_SECRETS_FILE", path.toUtf8());
        QVERIFY(!SecretStore::read(QStringLiteral("server/a")));
        QString err;
        QVERIFY2(SecretStore::write(QStringLiteral("server/a"), QStringLiteral("il_secret-a"), &err), qPrintable(err));
        QVERIFY(SecretStore::write(QStringLiteral("server/b"), QStringLiteral("il_secret-b")));
        QCOMPARE(SecretStore::read(QStringLiteral("server/a")).value_or(QString()), QStringLiteral("il_secret-a"));
        QVERIFY(!SecretStore::secure());
        QVERIFY(SecretStore::where().contains(QStringLiteral("sign-ins.json")));
#ifndef Q_OS_WIN
        // Only its owner may read it.
        const auto perms = QFile::permissions(path);
        QVERIFY(perms.testFlag(QFileDevice::ReadOwner));
        QVERIFY(!perms.testFlag(QFileDevice::ReadGroup));
        QVERIFY(!perms.testFlag(QFileDevice::ReadOther));
#endif
        QVERIFY(SecretStore::remove(QStringLiteral("server/a")));
        QVERIFY(!SecretStore::read(QStringLiteral("server/a")));
        QCOMPARE(SecretStore::read(QStringLiteral("server/b")).value_or(QString()), QStringLiteral("il_secret-b"));
        // Replacing keeps one value.
        QVERIFY(SecretStore::write(QStringLiteral("server/b"), QStringLiteral("il_new")));
        QCOMPARE(SecretStore::read(QStringLiteral("server/b")).value_or(QString()), QStringLiteral("il_new"));
        qunsetenv("INTERLINEA_SECRETS_FILE");
    }
};

QTEST_GUILESS_MAIN(TestSecretStore)
#include "tst_secretstore.moc"
