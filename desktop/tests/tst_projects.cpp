#include "apiclient.h"
#include "connectionmanager.h"
#include "languages.h"
#include "newprojectdialog.h"
#include "projectpage.h"
#include "projectsmodel.h"
#include "projectspage.h"
#include "settingseditor.h"
#include "testcore.h"
#include "theme.h"

#include <QApplication>
#include <QComboBox>
#include <QFile>
#include <QLineEdit>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStackedWidget>
#include <QStandardPaths>
#include <QTabWidget>
#include <QTableWidget>
#include <QTemporaryDir>
#include <QTest>
#include <QTreeWidget>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

template <typename T>
static T *named(QWidget *root, const QString &accessibleName)
{
    for (T *w : root->findChildren<T *>()) {
        if (w->accessibleName() == accessibleName)
            return w;
    }
    return nullptr;
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>()) {
        if (b->text().remove(QLatin1Char('&')) == text)
            return b;
    }
    return nullptr;
}

class TestProjects : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_projects"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void languages()
    {
        QCOMPARE(Languages::normalize(QStringLiteral("de_de")), QStringLiteral("de-DE"));
        QCOMPARE(Languages::normalize(QStringLiteral("zh-hant-tw")), QStringLiteral("zh-Hant-TW"));
        QVERIFY(Languages::normalize(QStringLiteral("not a tag")).isEmpty());
        QVERIFY(Languages::samePrimary(QStringLiteral("de-AT"), QStringLiteral("de-DE")));
        QVERIFY(!Languages::samePrimary(QStringLiteral("de-DE"), QStringLiteral("en-GB")));
        QVERIFY(Languages::name(QStringLiteral("de-DE")).startsWith(QStringLiteral("German")));
    }

    void makeAndWorkOnAProject()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        // A memory to choose in the setup.
        QVERIFY(wait(api->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Weber DE-EN"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());

        Theme::apply(*qApp);
        ProjectsPage page(&m);
        page.show();

        // The document is read and its language guessed.
        const QString doc = m_tmp.filePath(QStringLiteral("Hinweise.txt"));
        QFile f(doc);
        QVERIFY(f.open(QIODevice::WriteOnly));
        f.write("Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters. "
                "Die Kaution beträgt drei Monatsmieten und wird bei Beginn des Mietverhältnisses fällig.\n");
        f.close();
        NewProjectDialog dlg(&m, &page);
        dlg.show();
        dlg.addFiles({doc});
        auto *files = named<QTreeWidget>(&dlg, QStringLiteral("Documents"));
        QVERIFY(files);
        QTRY_COMPARE_WITH_TIMEOUT(files->topLevelItem(0)->text(1), QStringLiteral("To translate"), 10000);
        QCOMPARE(files->topLevelItem(0)->text(3), QStringLiteral("de-DE"));
        // The first step's source language, not the setup's.
        auto *source = named<LanguageCombo>(dlg.findChild<QStackedWidget *>()->widget(0), QStringLiteral("Source language"));
        QCOMPARE(source->language(), QStringLiteral("de-DE"));

        button(&dlg, QStringLiteral("Next"))->click();
        auto *editor = dlg.findChild<SettingsEditor *>();
        QTRY_VERIFY_WITH_TIMEOUT(editor->isVisible(), 10000);
        QCOMPARE(named<QLineEdit>(editor, QStringLiteral("Project name"))->text(), QStringLiteral("Hinweise"));
        named<LanguageCombo>(editor, QStringLiteral("Target language to add"))->setLanguage(QStringLiteral("en-GB"));
        button(editor, QStringLiteral("Add"))->click();
        // The memory of the language pair is offered and added.
        auto *addMemory = named<QComboBox>(editor, QStringLiteral("Memory to add"));
        QCOMPARE(addMemory->count(), 1);
        button(editor, QStringLiteral("Add memory"))->click();
        named<QLineEdit>(editor, QStringLiteral("Client"))->setText(QStringLiteral("Kanzlei Weber"));
        QSignalSpy accepted(&dlg, &QDialog::accepted);
        button(&dlg, QStringLiteral("Make the project"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(accepted.size(), 1, 20000);
        const qint64 id = dlg.projectId();
        QVERIFY(id > 0);

        ApiResult r = wait(api->get(QStringLiteral("/api/projects/%1").arg(id)));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        const QJsonObject s = r.object().value("settings").toObject();
        QCOMPARE(s.value("client").toString(), QStringLiteral("Kanzlei Weber"));
        const QJsonArray mems = s.value("memories").toObject().value("en-GB").toArray();
        QCOMPARE(mems.size(), 2);
        QCOMPARE(mems.at(0).toObject().value("memory").toString(), QStringLiteral("Hinweise (en-GB)"));
        QCOMPARE(mems.at(1).toObject().value("memory").toString(), QStringLiteral("Weber DE-EN"));
        QCOMPARE(s.value("terminology").toObject().value("termbase").toString(), QStringLiteral("Hinweise (project)"));

        // The list shows it; the project page shows its file and progress.
        QTRY_COMPARE_WITH_TIMEOUT(page.model()->rowCount(), 1, 10000);
        page.open(dlg.connectionId(), id);
        ProjectPage *pp = page.openProject();
        QVERIFY(pp);
        auto *progress = named<QTreeWidget>(pp, QStringLiteral("Files and their progress"));
        QTRY_VERIFY_WITH_TIMEOUT(progress->topLevelItemCount() == 1 && progress->topLevelItem(0)->childCount() == 1, 10000);
        QCOMPARE(progress->topLevelItem(0)->child(0)->text(2), QStringLiteral("0 %"));
        QCOMPARE(progress->topLevelItem(0)->child(0)->text(4), QStringLiteral("0 of 2"));

        // A translation saved elsewhere shows up through the live feed.
        r = wait(api->get(QStringLiteral("/api/projects/%1/segments").arg(id), QUrlQuery{{"file", "Hinweise.txt"}, {"lang", "en-GB"}}));
        const QJsonObject seg = r.array().at(0).toObject();
        r = wait(api->put(QStringLiteral("/api/projects/%1/segments").arg(id),
                          QJsonObject{{"target", QJsonArray{QJsonObject{{"text", "The tenant pays the rent."}}}}, {"status", "confirmed"}},
                          seg.value("version").toInt(), QUrlQuery{{"file", "Hinweise.txt"}, {"lang", "en-GB"}, {"segment", seg.value("id").toString()}}));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QTRY_COMPARE_WITH_TIMEOUT(progress->topLevelItem(0)->child(0)->text(4), QStringLiteral("1 of 2"), 10000);

        // Settings are changed and saved.
        auto *ped = pp->findChild<SettingsEditor *>();
        QTRY_COMPARE_WITH_TIMEOUT(named<QLineEdit>(ped, QStringLiteral("Client"))->text(), QStringLiteral("Kanzlei Weber"), 10000);
        named<QLineEdit>(ped, QStringLiteral("Client"))->setText(QStringLiteral("Weber & Partner"));
        button(pp, QStringLiteral("Save settings"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/projects/%1").arg(id))).object().value("settings").toObject().value("client").toString(),
                                  QStringLiteral("Weber & Partner"), 10000);

        // The analysis counts its words.
        button(pp, QStringLiteral("Analyse"))->click();
        auto *table = named<QTableWidget>(pp, QStringLiteral("Analysis"));
        QTRY_VERIFY_WITH_TIMEOUT(table->rowCount() > 1, 10000);
        // INTERLINEA_SCREENSHOTS keeps pictures of the pages, to look at.
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            page.resize(1200, 760);
            auto *tabs = pp->findChild<QTabWidget *>();
            for (int i = 0; i < tabs->count(); ++i) {
                tabs->setCurrentIndex(i);
                QTest::qWait(100);
                page.grab().save(QStringLiteral("%1/project-%2.png").arg(dir).arg(i));
            }
            dlg.resize(900, 640);
            dlg.grab().save(dir + QStringLiteral("/new-project.png"));
        }
        QCOMPARE(table->item(table->rowCount() - 1, 0)->text(), QStringLiteral("Total"));
        QVERIFY(table->item(table->rowCount() - 1, 2)->text().toInt() >= 20);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestProjects)
#include "tst_projects.moc"
