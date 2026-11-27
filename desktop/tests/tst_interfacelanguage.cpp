#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "editorpage.h"
#include "projectpage.h"
#include "interfacelanguage.h"
#include "mainwindow.h"
#include "accountspage.h"
#include "memoriespage.h"
#include "projectspage.h"
#include "settingspage.h"
#include "terminologypage.h"
#include "testcore.h"
#include "theme.h"

#include <QApplication>
#include <QComboBox>
#include <QJsonArray>
#include <QJsonObject>
#include <QPushButton>
#include <QFrame>
#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTest>
#include <QTextBrowser>
#include <QPointer>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

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
    }

    void catalog()
    {
        CatalogTranslator de;
        QVERIFY(de.loadCatalog(QStringLiteral(":/i18n/de.json")));
        QCOMPARE(de.translate("SettingsPage", "Settings"), QStringLiteral("Einstellungen"));
        QCOMPARE(de.translate(nullptr, "Delete the role %1?"), QStringLiteral("Rolle %1 löschen?"));
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
            Theme::apply(*qApp);
            TestCore core;
            ConnectionManager m(&core.launcher);
            MainWindow w(&m);
            w.addPage(QStringLiteral("projects"), QObject::tr("Projects"), QIcon(), new ProjectsPage(&m, &w));
            w.addPage(QStringLiteral("memories"), QObject::tr("Memories"), QIcon(), new MemoriesPage(&m, &w));
            w.addPage(QStringLiteral("terminology"), QObject::tr("Terminology"), QIcon(), new TerminologySection(&m, &w));
            w.addPage(QStringLiteral("accounts"), QObject::tr("Accounts"), QIcon(), new AccountsPage(&m, &w));
            auto *settings = new SettingsPage(&m, &w);
            w.addPage(QStringLiteral("settings"), QObject::tr("Settings"), QIcon(), settings);
            w.show();
            m.start();
            QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
            // The page speaks the language, the choice among the languages.
            QStringList panels;
            for (QFrame *f : settings->findChildren<QFrame *>(QStringLiteral("panel")))
                panels << f->accessibleName();
            QCOMPARE(panels.value(0), QString::fromUtf8(lang == QLatin1String("de") ? "Sprache der Oberfläche" : "인터페이스 언어"));
            auto *pick = settings->findChild<QComboBox *>(QStringLiteral("interfaceLanguage"));
            QVERIFY(pick);
            QCOMPARE(pick->count(), 4);
            QCOMPARE(pick->itemText(2), QStringLiteral("Deutsch"));
            for (const QString page : {"projects", "memories", "terminology", "accounts", "settings"}) {
                w.showPage(page);
                for (const QSize size : {QSize(1024, 680), QSize(1366, 768)}) {
                    w.resize(size);
                    const QString broken = layoutProblems(&w);
                    QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1, %2 at %3x%4:\n%5").arg(QLatin1String(lang), page).arg(size.width()).arg(size.height()).arg(broken)));
                }
                if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
                    w.resize(1366, 768);
                    QTest::qWait(300);
                    w.grab().save(QStringLiteral("%1/%2-%3.png").arg(dir, QLatin1String(lang), page));
                }
            }
            // The editor, on a document: its bar holds its longer buttons at
            // the sizes a window has, from a small one to a full screen.
            {
                ApiClient *api = m.local()->api();
                QJsonObject settings = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
                settings.insert("name", QStringLiteral("Test"));
                settings.insert("sourceLang", "de-DE");
                settings.insert("targetLangs", QJsonArray{"en-US"});
                // A heading, segments of one sign, a long one with tags.
                const QByteArray html = QStringLiteral(
                    "<html><body><h1>HA-Setup Pro/Contra</h1><p>+</p><p>-</p>"
                    "<p>Besseres Verständnis des Systems für Dritte (IT-Dienstleister, Nachfolger, etc.), da System nach "
                    "„Industriestandard“ (Setups dieser Art gibt es in vielen anderen Unternehmen).</p>"
                    "<p>Komplettes neu aufsetzen EINES Servers notwendig.</p>"
                    "<p><b>Weniger</b> Management-Aufwand, beide Server müssen auf derselben Softwareversion gehalten werden, "
                    "<i>dafür aber Alles in einer Management-Oberfläche und System nach gut dokumentierten Standards.</i></p>"
                    "</body></html>").toUtf8();
                const ApiResult made = wait(api->post(QStringLiteral("/api/projects"),
                    QJsonObject{{"settings", settings}, {"files", QJsonArray{QJsonObject{{"name", "HA-Setup Pro-Contra.html"}, {"role", "translatable"}, {"data", QString::fromLatin1(html.toBase64())}}}}}));
                QVERIFY2(made.ok(), qPrintable(made.error.message));
                ProjectsPage projects(&m);
                projects.resize(1280, 820);
                projects.show();
                projects.open(m.local()->id(), made.object().value("id").toInteger());
                ProjectPage *page = projects.openProject();
                QVERIFY(page);
                QPushButton *open = nullptr;
                QTRY_VERIFY_WITH_TIMEOUT((open = page->findChild<QPushButton *>(QStringLiteral("progress"))) != nullptr, 10000);
                open->click();
                EditorPage *ed = projects.openEditor();
                QVERIFY(ed);
                QTRY_VERIFY_WITH_TIMEOUT(ed->model()->rowCount() > 0 && ed->activeRow() == 0, 10000);
                for (const QSize size : {QSize(1024, 700), QSize(1280, 820), QSize(1700, 1000), QSize(1920, 1032)}) {
                    projects.resize(size);
                    const QString broken = layoutProblems(&projects);
                    if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty())
                        projects.grab().save(QStringLiteral("%1/%2-editor-%3.png").arg(dir, QLatin1String(lang)).arg(size.width()));
                    QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1, editor at %2x%3:\n%4").arg(QLatin1String(lang)).arg(size.width()).arg(size.height()).arg(broken)));
                }
                // After work: the first translated and confirmed, the next
                // two left as drafts, the fourth confirmed, and back to the
                // first. Each row shows its whole text, the open one too.
                auto put = [ed](const QString &text) { ed->target()->setContent(QJsonArray{QJsonObject{{"text", text}}}); };
                put(QStringLiteral("HA-Setup Pro and Contra"));
                ed->confirm();
                QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 1, 10000);
                put(QStringLiteral("+"));
                ed->move(1);
                QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 2, 10000);
                put(QStringLiteral("-"));
                ed->move(1);
                QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 3, 10000);
                put(QStringLiteral("Better understanding of the system for third parties."));
                ed->confirm();
                QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 4, 10000);
                for (int back = 4; back > 0; back--) {
                    ed->move(-1);
                    QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), back - 1, 10000);
                }
                // A click into the source, to select a term in it: the text
                // stays whole, no frame comes around it.
                QPointer<QTextBrowser> source = ed->findChild<QTextBrowser *>(QStringLiteral("activeSource"));
                QVERIFY(source);
                QTest::mouseClick(source->viewport(), Qt::LeftButton, {}, QPoint(10, 5));
                QTest::qWait(300);
                // The click may open the segment's editors again.
                source = ed->findChild<QTextBrowser *>(QStringLiteral("activeSource"));
                QVERIFY(source);
                source->setFocus(Qt::MouseFocusReason);
                QTRY_VERIFY(source->hasFocus());
                QTest::qWait(100);
                QVERIFY2(source->viewport()->height() >= int(source->document()->size().height()),
                         qPrintable(QStringLiteral("%1: the source shows %2 of its %3 px").arg(QLatin1String(lang)).arg(source->viewport()->height()).arg(source->document()->size().height())));
                QCOMPARE(source->viewport()->geometry(), source->rect());
                for (const QSize size : {QSize(1280, 820), QSize(1920, 1032)}) {
                    projects.resize(size);
                    QTest::qWait(300);
                    const QString broken = layoutProblems(&projects);
                    if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty())
                        projects.grab().save(QStringLiteral("%1/%2-editor-worked-%3.png").arg(dir, QLatin1String(lang)).arg(size.width()));
                    QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1, editor after work at %2x%3:\n%4").arg(QLatin1String(lang)).arg(size.width()).arg(size.height()).arg(broken)));
                }
            }
            QCoreApplication::removeTranslator(&tr);
        }
    }
};

QTEST_MAIN(TestInterfaceLanguage)
#include "tst_interfacelanguage.moc"
