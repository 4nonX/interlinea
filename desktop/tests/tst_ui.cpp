#include "testcore.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQuickWindow>
#include <QSettings>
#include <QStandardPaths>
#include <QStyleHints>
#include <QGuiApplication>
#include <QTest>

// The window as a whole: its pages at the sizes windows have, the
// settings worked with as a person does, and a core that fails to start.
class TestUi : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_ui"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void everyPageAtEverySize()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        // Something on each page: a memory, a termbase, a project.
        QVERIFY(waitFor(app.api()->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Weber DE-EN"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());
        QVERIFY(waitFor(app.api()->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Kanzlei Weber"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", QStringLiteral("Mietvertrag Hofmann mit einem langen Namen, der nicht auf eine Zeile passt"));
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB", "fr-FR"});
        QVERIFY(waitFor(app.api()->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings}, {"files", QJsonArray{QJsonObject{{"name", "a.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(QByteArray("Der Mieter zahlt.").toBase64())}}}}})).ok());
        app.call(QStringLiteral("projectsPage"), "reload");
        for (const QString page : {"projects", "memories", "terminology", "settings", "new"}) {
            if (page == QLatin1String("new"))
                app.call(app.window, "newProject", {QVariantList{}});
            else
                app.show(page);
            for (const QSize size : {QSize(900, 600), QSize(1280, 800), QSize(1920, 1080)}) {
                app.window->resize(size);
                const QString broken = layoutProblems(app.window);
                QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1 at %2x%3:\n%4").arg(page).arg(size.width()).arg(size.height()).arg(broken)));
            }
            app.shot(page);
        }
    }

    void settings()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        app.show(QStringLiteral("settings"));
        const QString here = app.connections.local()->id();

        // The licence, in full.
        QVERIFY(app.click(QStringLiteral("readLicence")));
        QTRY_VERIFY(app.get(QStringLiteral("licence"), "opened").toBool());
        const QString said = app.backend.licence();
        QVERIFY(said.startsWith(QStringLiteral("Interlinea\nCopyright (C) 2026 Dan Dreßen")));
        QVERIFY(said.contains(QStringLiteral("GNU AFFERO GENERAL PUBLIC LICENSE")) && said.contains(QStringLiteral("END OF TERMS AND CONDITIONS")));
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.call(QStringLiteral("licence"), "close");

        // Your name, typed: kept by the core.
        QVERIFY(app.type(QStringLiteral("userName"), QStringLiteral("Tess Translator")));
        QTest::keyClick(app.window, Qt::Key_Tab);
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Your name is Tess Translator"), 10000);
        const QString me = app.connections.local()->user().value("id").toVariant().toString();
        QCOMPARE(waitFor(app.api()->get(QStringLiteral("/api/users/") + me)).object().value("name").toString(), QStringLiteral("Tess Translator"));

        // A translation service: the form follows the service chosen; its key
        // goes to the core and never comes back.
        QTRY_VERIFY_WITH_TIMEOUT(app.clickable(QStringLiteral("addProvider:") + here), 10000);
        QVERIFY(app.click(QStringLiteral("addProvider:") + here));
        QObject *form = app.find(QStringLiteral("providerForm"));
        QVERIFY(form);
        QTRY_VERIFY_WITH_TIMEOUT(form->property("kinds").toList().size() >= 5, 10000);
        QCOMPARE(form->property("service").toString(), QStringLiteral("deepl"));
        QCOMPARE(app.find(form, QStringLiteral("name"))->property("text").toString(), QStringLiteral("DeepL"));
        QObject *save = app.find(form, QStringLiteral("save"));
        QVERIFY(!save->property("enabled").toBool());
        QVERIFY(app.type(app.find(form, QStringLiteral("key")), QStringLiteral("dk:fx")));
        QVERIFY(app.click(save));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added the service DeepL"), 10000);
        QVERIFY(!QJsonDocument(waitFor(app.api()->get(QStringLiteral("/api/providers"))).array()).toJson().contains("dk:fx"));

        // One of its own, signed with a header.
        QTRY_VERIFY_WITH_TIMEOUT(app.clickable(QStringLiteral("addProvider:") + here), 10000);
        QVERIFY(app.click(QStringLiteral("addProvider:") + here));
        form->setProperty("service", QStringLiteral("own"));
        QVERIFY(app.type(app.find(form, QStringLiteral("name")), QStringLiteral("Echo")));
        QVERIFY(app.type(app.find(form, QStringLiteral("url")), QStringLiteral("https://mt.example.org/translate")));
        QVERIFY(app.type(app.find(form, QStringLiteral("headerName")), QStringLiteral("Authorization")));
        QVERIFY(app.type(app.find(form, QStringLiteral("headerValue")), QStringLiteral("Bearer k1")));
        QVERIFY(app.click(app.find(form, QStringLiteral("save"))));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added the service Echo"), 10000);
        const QJsonArray listed = waitFor(app.api()->get(QStringLiteral("/api/providers"))).array();
        QCOMPARE(listed.size(), 2);
        QVERIFY(!QJsonDocument(listed).toJson().contains("k1"));

        // A grammar service.
        QTRY_VERIFY_WITH_TIMEOUT(app.clickable(QStringLiteral("addChecker:") + here), 10000);
        QVERIFY(app.click(QStringLiteral("addChecker:") + here));
        QObject *checker = app.find(QStringLiteral("checkerForm"));
        QVERIFY(app.type(app.find(checker, QStringLiteral("name")), QStringLiteral("Checker")));
        QVERIFY(app.type(app.find(checker, QStringLiteral("url")), QStringLiteral("https://grammar.example.org/v2/check")));
        QVERIFY(app.click(app.find(checker, QStringLiteral("save"))));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added the service Checker"), 10000);
        QString kinds;
        for (const QJsonValue &v : waitFor(app.api()->get(QStringLiteral("/api/providers"))).array())
            kinds += v.toObject().value("name").toString() + QLatin1Char('=') + v.toObject().value("kind").toString() + QLatin1Char(' ');
        QCOMPARE(kinds, QStringLiteral("Checker=check DeepL=translate Echo=translate "));

        // An access token for a program.
        const int before = int(waitFor(app.api()->get(QStringLiteral("/api/me/tokens"))).array().size());
        QVERIFY(!app.click(QStringLiteral("makeToken:") + here));
        QVERIFY(app.type(QStringLiteral("tokenName:") + here, QStringLiteral("My assistant")));
        QVERIFY(app.click(QStringLiteral("makeToken:") + here));
        QTRY_COMPARE_WITH_TIMEOUT(int(waitFor(app.api()->get(QStringLiteral("/api/me/tokens"))).array().size()), before + 1, 10000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
    }

    // Light, dark and the system's setting, chosen in Settings: shown at
    // once, kept, and the choice of the system brings back what it is set to.
    void appearanceChosen()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        app.show(QStringLiteral("settings"));

        QVERIFY(app.click(QStringLiteral("appearance:dark")));
        QTRY_VERIFY(app.backend.dark());
        QTRY_COMPARE(app.window->color(), QColor(QStringLiteral("#14171c")));
        QCOMPARE(QSettings().value(QStringLiteral("interface/appearance")).toString(), QStringLiteral("dark"));
        QVERIFY(app.get(QStringLiteral("appearance:dark"), "checked").toBool());
        QVERIFY(!app.get(QStringLiteral("appearance:system"), "checked").toBool());
        QCOMPARE(app.get(QStringLiteral("appearance:system"), "description").toString(), QStringLiteral("Light or dark, as the system is set."));
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));

        QVERIFY(app.click(QStringLiteral("appearance:light")));
        QTRY_VERIFY(!app.backend.dark());
        QTRY_COMPARE(app.window->color(), QColor(QStringLiteral("#f3f5f8")));
        QCOMPARE(QSettings().value(QStringLiteral("interface/appearance")).toString(), QStringLiteral("light"));

        QVERIFY(app.click(QStringLiteral("appearance:system")));
        QTRY_VERIFY(app.get(QStringLiteral("appearance:system"), "checked").toBool());
        QVERIFY(!QSettings().contains(QStringLiteral("interface/appearance")));
        const bool dark = app.backend.dark();
        QCOMPARE(app.window->color(), QColor(dark ? QStringLiteral("#14171c") : QStringLiteral("#f3f5f8")));
        QCOMPARE(app.get(QStringLiteral("appearance:system"), "description").toString(),
                 dark ? QStringLiteral("Dark now, as the system is set.") : QStringLiteral("Light now, as the system is set."));
    }

    void coreFailureShown()
    {
        QTemporaryDir dir;
        CoreLauncher launcher(dir.filePath(QStringLiteral("no-core")), dir.filePath(QStringLiteral("data")), QStringLiteral("x"));
        QmlApp app(&launcher);
        QString why;
        QVERIFY(!app.start(&why));
        QVERIFY(app.window);
        // Every page says so, over it, with a way to try again.
        QTRY_VERIFY(app.get(QStringLiteral("coreProblem"), "visible").toBool());
        QVERIFY(app.get(QStringLiteral("coreProblem"), "text").toString().startsWith(QStringLiteral("Work on this computer can't be opened: ")));
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestUi)
#include "tst_ui.moc"
