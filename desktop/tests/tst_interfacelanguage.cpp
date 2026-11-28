#include "interfacelanguage.h"
#include "testcore.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QQuickWindow>
#include <QSettings>
#include <QStandardPaths>
#include <QTest>

// The interface in German and Korean: the catalogs are built in, strings
// are looked up by their English text, the setting is kept, and every page
// keeps its layout with the longer German and the Korean text.
class TestInterfaceLanguage : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_interfacelanguage"));
        QSettings().clear();
        qRegisterMetaType<ApiResult>("ApiResult");
    }

    void catalog()
    {
        CatalogTranslator de;
        QVERIFY(de.loadCatalog(QStringLiteral(":/i18n/de.json")));
        QCOMPARE(de.translate("SettingsPage", "Settings"), QStringLiteral("Einstellungen"));
        QCOMPARE(de.translate(nullptr, "Delete the role {role}?"), QStringLiteral("Rolle {role} löschen?"));
        // A string the catalog lacks stays English: Qt takes a null string so.
        QVERIFY(de.translate("X", "No such string in the catalog").isNull());
        CatalogTranslator ko;
        QVERIFY(ko.loadCatalog(QStringLiteral(":/i18n/ko.json")));
        QCOMPARE(ko.translate("SettingsPage", "Settings"), QStringLiteral("설정"));
    }

    void choice()
    {
        QCOMPARE(InterfaceLanguage::chosen(), QString());
        InterfaceLanguage::choose(QStringLiteral("ko"));
        QCOMPARE(InterfaceLanguage::chosen(), QStringLiteral("ko"));
        InterfaceLanguage::choose(QStringLiteral("xx"));
        QCOMPARE(InterfaceLanguage::chosen(), QString());
        QCOMPARE(InterfaceLanguage::nativeName(QStringLiteral("de")), QStringLiteral("Deutsch"));
        QVERIFY(InterfaceLanguage::codes().contains(InterfaceLanguage::systemLanguage()));
    }

    void everyPageInEachLanguage()
    {
        for (const char *lang : {"de", "ko"}) {
            CatalogTranslator tr;
            QVERIFY(tr.loadCatalog(QStringLiteral(":/i18n/%1.json").arg(QLatin1String(lang))));
            QCoreApplication::installTranslator(&tr);
            TestCore core;
            QmlApp app(&core.launcher);
            QString why;
            QVERIFY2(app.start(&why), qPrintable(why));
            // The page speaks the language, the choice among the languages.
            QCOMPARE(app.get(QStringLiteral("interfaceLanguage"), "model").toList().value(2).toString(), QStringLiteral("Deutsch"));
            QCOMPARE(app.get(QStringLiteral("interfaceLanguage"), "count").toInt(), 4);
            QCOMPARE(QCoreApplication::translate("Main", "Settings"), QString::fromUtf8(lang == QLatin1String("de") ? "Einstellungen" : "설정"));
            for (const QString page : {"projects", "memories", "terminology", "settings"}) {
                app.show(page);
                for (const QSize size : {QSize(800, 520), QSize(1024, 680), QSize(1366, 768)}) {
                    app.window->resize(size);
                    const QString broken = layoutProblems(app.window);
                    QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1, %2 at %3x%4:\n%5").arg(QLatin1String(lang), page).arg(size.width()).arg(size.height()).arg(broken)));
                }
                app.shot(QStringLiteral("%1-%2").arg(QLatin1String(lang), page));
            }
            // The editor, on a document: its bar holds its longer buttons at
            // the sizes a window has, from a small one to a full screen.
            QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
            settings.insert("name", QStringLiteral("Test"));
            settings.insert("sourceLang", "de-DE");
            settings.insert("targetLangs", QJsonArray{"en-US"});
            const QByteArray html = QStringLiteral(
                "<html><body><h1>HA-Setup Pro/Contra</h1><p>+</p><p>-</p>"
                "<p>Besseres Verständnis des Systems für Dritte (IT-Dienstleister, Nachfolger, etc.), da System nach "
                "„Industriestandard“ (Setups dieser Art gibt es in vielen anderen Unternehmen).</p>"
                "<p><b>Weniger</b> Management-Aufwand, <i>dafür aber Alles in einer Management-Oberfläche.</i></p>"
                "</body></html>").toUtf8();
            const ApiResult made = waitFor(app.api()->post(QStringLiteral("/api/projects"),
                QJsonObject{{"settings", settings}, {"files", QJsonArray{QJsonObject{{"name", "HA-Setup Pro-Contra.html"}, {"role", "translatable"}, {"data", QString::fromLatin1(html.toBase64())}}}}}));
            QVERIFY2(made.ok(), qPrintable(made.error.message));
            app.call(app.window, "openEditor", {app.connections.local()->id(), made.object().value("id").toInteger(), QString(), QString()});
            QTRY_VERIFY_WITH_TIMEOUT(app.get(QStringLiteral("editorPage"), "activeId").toString() != QString(), 10000);
            for (const QSize size : {QSize(1024, 700), QSize(1280, 820), QSize(1920, 1032)}) {
                app.window->resize(size);
                const QString broken = layoutProblems(app.window);
                app.shot(QStringLiteral("%1-editor-%2").arg(QLatin1String(lang)).arg(size.width()));
                QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1, editor at %2x%3:\n%4").arg(QLatin1String(lang)).arg(size.width()).arg(size.height()).arg(broken)));
            }
            QCoreApplication::removeTranslator(&tr);
        }
    }
};

QTEST_MAIN(TestInterfaceLanguage)
#include "tst_interfacelanguage.moc"
