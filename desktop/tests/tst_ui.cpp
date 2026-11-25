#include "apiclient.h"
#include "connectionmanager.h"
#include "options.h"
#include "webui.h"
#include "words.h"
#include "mainwindow.h"
#include "navigationrail.h"
#include "settingspage.h"
#include "testcore.h"

#include <QAbstractButton>
#include <QApplication>
#include <QCheckBox>
#include <QJsonObject>
#include <QRadioButton>
#include <QScrollArea>
#include <QSignalSpy>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QSettings>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QJsonDocument>
#include <QJsonArray>
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

static QLabel *labelWith(QWidget *root, const QString &part)
{
    for (QLabel *l : root->findChildren<QLabel *>())
        if (l->text().contains(part))
            return l;
    return nullptr;
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
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        // The page is the web's: its panels, in its order.
        QStringList panels;
        for (QFrame *f : page->findChildren<QFrame *>(QStringLiteral("panel")))
            panels << f->accessibleName();
        QCOMPARE(panels, (QStringList{QStringLiteral("Your name"), QStringLiteral("Terminology wording"), QStringLiteral("Editor"), QStringLiteral("Project templates"),
                                      QStringLiteral("Spelling dictionaries"), QStringLiteral("Translation services"), QStringLiteral("Grammar services"), QStringLiteral("Access for programs"),
                                      QStringLiteral("Team server"), QStringLiteral("Saved work")}));
        // A translation service is added on this computer, listed by its
        // name and where it is, and its key is not shown.
        {
            const QString here = m.local()->id();
            const auto wait = [](ApiReply *reply) {
                QSignalSpy spy(reply, &ApiReply::finished);
                return spy.wait(20000) ? spy.first().first().value<ApiResult>() : ApiResult{};
            };
            QTRY_VERIFY_WITH_TIMEOUT(page->findChild<QPushButton *>(QStringLiteral("addProvider:%1").arg(here)), 10000);
            page->findChild<QPushButton *>(QStringLiteral("addProvider:%1").arg(here))->click();
            auto *save = page->findChild<QPushButton *>(QStringLiteral("saveProvider"));
            QVERIFY(save && !save->isEnabled());
            page->findChild<QLineEdit *>(QStringLiteral("providerName"))->setText(QStringLiteral("Echo"));
            page->findChild<QLineEdit *>(QStringLiteral("providerUrl"))->setText(QStringLiteral("https://mt.example.org/translate"));
            page->findChild<QLineEdit *>(QStringLiteral("providerHeader"))->setText(QStringLiteral("Authorization"));
            page->findChild<QLineEdit *>(QStringLiteral("providerHeaderValue"))->setText(QStringLiteral("Bearer k1"));
            QVERIFY(save->isEnabled());
            save->click();
            QTRY_COMPARE_WITH_TIMEOUT(page->lastNotice(), QStringLiteral("Added the service Echo"), 10000);
            const QJsonArray listed = wait(m.local()->api()->get(QStringLiteral("/api/providers"))).array();
            QCOMPARE(listed.size(), 1);
            QCOMPARE(listed.first().toObject().value("host").toString(), QStringLiteral("mt.example.org"));
            QVERIFY(listed.first().toObject().value("signed").toBool() && !QJsonDocument(listed).toJson().contains("k1"));
            bool shown = false;
            QTRY_VERIFY_WITH_TIMEOUT([&] {
                for (QLabel *l : page->findChildren<QLabel *>())
                    shown = shown || (l->text() == QStringLiteral("mt.example.org") && !l->isHidden());
                return shown;
            }(), 10000);
            // An address that is none is refused by the core, in its words.
            page->saveProvider(here, QStringLiteral("Bad"), QStringLiteral("ftp://nowhere"), QString(), QString());
            QTRY_VERIFY_WITH_TIMEOUT(page->lastNotice().contains(QStringLiteral("https://")), 10000);

            // A grammar service has its own list and form, and is kept as one.
            QTRY_VERIFY_WITH_TIMEOUT(page->findChild<QPushButton *>(QStringLiteral("addChecker:%1").arg(here)), 10000);
            QCOMPARE(page->findChild<QPushButton *>(QStringLiteral("addChecker:%1").arg(here))->text(), QStringLiteral("Add a grammar service"));
            page->findChild<QPushButton *>(QStringLiteral("addChecker:%1").arg(here))->click();
            auto *saveChecker = page->findChild<QPushButton *>(QStringLiteral("saveChecker"));
            QVERIFY(saveChecker && !saveChecker->isEnabled() && !page->findChild<QPushButton *>(QStringLiteral("saveProvider")));
            page->findChild<QLineEdit *>(QStringLiteral("checkerName"))->setText(QStringLiteral("Checker"));
            page->findChild<QLineEdit *>(QStringLiteral("checkerUrl"))->setText(QStringLiteral("https://grammar.example.org/v2/check"));
            saveChecker->click();
            QTRY_COMPARE_WITH_TIMEOUT(page->lastNotice(), QStringLiteral("Added the service Checker"), 10000);
            QString kinds;
            for (const QJsonValue &v : wait(m.local()->api()->get(QStringLiteral("/api/providers"))).array())
                kinds += v.toObject().value("name").toString() + QLatin1Char('=') + v.toObject().value("kind").toString() + QLatin1Char(' ');
            QCOMPARE(kinds, QStringLiteral("Checker=check Echo=translate "));
            QTRY_VERIFY_WITH_TIMEOUT([&] {
                for (QLabel *l : page->findChildren<QLabel *>()) {
                    if (l->text() == QStringLiteral("grammar.example.org") && !l->isHidden())
                        return true;
                }
                return false;
            }(), 10000);

            // An access token for a program: made, shown once, and listed.
            const int before = int(wait(m.local()->api()->get(QStringLiteral("/api/me/tokens"))).array().size());
            auto *mcp = page->findChild<QLabel *>(QStringLiteral("mcpAddress:%1").arg(here));
            QVERIFY(mcp && mcp->text().startsWith(QStringLiteral("Assistants connect to http")) && mcp->text().endsWith(QStringLiteral("/mcp")));
            auto *tokenName = page->findChild<QLineEdit *>(QStringLiteral("tokenName:%1").arg(here));
            auto *make = page->findChild<QPushButton *>(QStringLiteral("makeToken:%1").arg(here));
            QVERIFY(tokenName && make && !make->isEnabled());
            tokenName->setText(QStringLiteral("My assistant"));
            QVERIFY(make->isEnabled());
            make->click();
            QTRY_VERIFY_WITH_TIMEOUT(!page->madeToken().isEmpty(), 10000);
            QTRY_VERIFY_WITH_TIMEOUT(page->findChild<QLineEdit *>(QStringLiteral("tokenValue")), 10000);
            QCOMPARE(page->findChild<QLineEdit *>(QStringLiteral("tokenValue"))->text(), page->madeToken());
            const QJsonArray tokens = wait(m.local()->api()->get(QStringLiteral("/api/me/tokens"))).array();
            QCOMPARE(int(tokens.size()), before + 1);
            bool named = false;
            for (const QJsonValue &v : tokens)
                named = named || v.toObject().value("name").toString() == QStringLiteral("My assistant");
            QVERIFY(named);
        }
        // The name field holds the person's name and saves it to the core.
        auto *name = page->findChild<QLineEdit *>(QStringLiteral("userName"));
        QVERIFY(name);
        QTRY_COMPARE(name->text(), QStringLiteral("Tess Tester"));
        name->setText(QStringLiteral("Tess T."));
        QTest::keyClick(name, Qt::Key_Return);
        QTRY_COMPARE_WITH_TIMEOUT(m.local()->userName(), QStringLiteral("Tess T."), 10000);
        QCOMPARE(QSettings().value(QStringLiteral("you/name")).toString(), QStringLiteral("Tess T."));
        // The wording and the editor's choices are kept as they are set.
        for (QRadioButton *b : page->findChildren<QRadioButton *>()) {
            if (b->text() == QStringLiteral("Git terms") || b->text() == QStringLiteral("The segments before and after also match"))
                b->setChecked(true);
        }
        QVERIFY(Words::git());
        QCOMPARE(Options::contextRule(), QStringLiteral("both"));
        for (QCheckBox *b : page->findChildren<QCheckBox *>()) {
            if (b->text() == QStringLiteral("Fill repetitions when you confirm"))
                b->setChecked(false);
        }
        QVERIFY(!Options::autoPropagate());
        QVERIFY(Options::autoInsert());
        Words::setGit(false);
        Options::setContextRule(QStringLiteral("previous"));
        Options::setAutoPropagate(true);
        // Nothing is there yet to delete, and a server's address is asked for.
        QTRY_VERIFY(labelWith(page, QStringLiteral("No template yet.")));
        QTRY_VERIFY(labelWith(page, QStringLiteral("No dictionary yet.")));
        QVERIFY(page->findChild<QLineEdit *>(QStringLiteral("serverAddress")));

        // A dictionary sent as its two files is listed, and deleted again.
        ApiReply *reply = m.local()->api()->post(QStringLiteral("/api/dictionaries"),
                                                 QJsonObject{{"name", "Hausliste"}, {"lang", "de-DE"}, {"aff", QString::fromLatin1(QByteArray("SET UTF-8\n").toBase64())},
                                                             {"dic", QString::fromLatin1(QByteArray("2\nMieter\nKaution\n").toBase64())}});
        QSignalSpy sent(reply, &ApiReply::finished);
        QVERIFY(sent.wait(10000));
        page->hide();
        page->show();
        QTRY_VERIFY_WITH_TIMEOUT(labelWith(page, QStringLiteral("Hausliste")), 10000);
        QVERIFY(labelWith(page, QStringLiteral("2 words")));
        Web::ConfirmButton *del = nullptr;
        for (Web::ConfirmButton *c : page->findChildren<Web::ConfirmButton *>()) {
            if (c->button()->accessibleName() == QStringLiteral("Delete the dictionary Hausliste"))
                del = c;
        }
        QVERIFY(del);
        del->button()->click();
        del->yes()->click();
        QTRY_VERIFY_WITH_TIMEOUT(page->lastNotice() == QStringLiteral("Deleted the dictionary Hausliste"), 10000);
        QTRY_VERIFY(labelWith(page, QStringLiteral("No dictionary yet.")));
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            w.resize(1200, 900);
            QTest::qWait(200);
            w.grab().save(dir + QStringLiteral("/settings.png"));
            page->findChild<QScrollArea *>(QStringLiteral("page"))->widget()->grab().save(dir + QStringLiteral("/settings-full.png"));
        }
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
