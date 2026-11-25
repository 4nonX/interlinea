#include "apiclient.h"
#include "connectionmanager.h"
#include "languages.h"
#include "editorpage.h"
#include "newprojectpage.h"
#include "projectform.h"
#include "projectpage.h"
#include "projectsmodel.h"
#include "projectspage.h"
#include "testcore.h"
#include "theme.h"

#include <QLabel>
#include <QApplication>
#include <QCheckBox>
#include <QComboBox>
#include <QRadioButton>
#include <QScrollArea>
#include <QSpinBox>
#include "webui.h"
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
        // The listed languages are the core's, named as in the browser client.
        QVERIFY(Languages::common().size() >= 90);
        QCOMPARE(Languages::name(QStringLiteral("th-TH")), QStringLiteral("Thai (Thailand)"));
        QCOMPARE(Languages::label(QStringLiteral("de-DE")), QStringLiteral("German (Germany)"));
        QCOMPARE(Languages::name(QStringLiteral("sr-Latn-RS")), QStringLiteral("Serbian (Latin, Serbia)"));
        // One outside the list is told apart by its tag.
        QVERIFY(Languages::label(QStringLiteral("de-LI")).endsWith(QStringLiteral(", de-LI")));
        QVERIFY(Languages::rtl(QStringLiteral("ar-SA")));
        QVERIFY(Languages::rtl(QStringLiteral("he-IL")));
        QVERIFY(Languages::rtl(QStringLiteral("ur")));
        QVERIFY(Languages::rtl(QStringLiteral("az-Arab-IR")));
        QVERIFY(!Languages::rtl(QStringLiteral("de-DE")));
        QVERIFY(!Languages::rtl(QStringLiteral("ur-Latn")));
        {
            LanguageCombo combo;
            combo.setLanguage(QStringLiteral("fa-IR"));
            QCOMPARE(combo.currentText(), QStringLiteral("Persian (Iran)"));
            QCOMPARE(combo.language(), QStringLiteral("fa-IR"));
            combo.setEditText(QStringLiteral("de-LI"));
            QCOMPARE(combo.language(), QStringLiteral("de-LI"));
        }
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
        page.resize(1200, 900);
        page.show();
        const QString shots = qEnvironmentVariable("INTERLINEA_SCREENSHOTS");
        const auto shot = [&](const QString &name) {
            if (shots.isEmpty())
                return;
            QTest::qWait(150);
            page.grab().save(QStringLiteral("%1/%2.png").arg(shots, name));
        };

        // The document is read and its language guessed.
        const QString doc = m_tmp.filePath(QStringLiteral("Hinweise.txt"));
        QFile f(doc);
        QVERIFY(f.open(QIODevice::WriteOnly));
        f.write("Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters. "
                "Die Kaution beträgt drei Monatsmieten und wird bei Beginn des Mietverhältnisses fällig.\n");
        f.close();
        page.newProject({doc});
        NewProjectPage *np = page.newProjectPage();
        QVERIFY(np);
        QCOMPARE(np->step(), 0);
        // It starts at Start: a blank project, with nothing else to start from yet.
        QTRY_VERIFY_WITH_TIMEOUT(button(np, QStringLiteral("Continue to files and languages")) && button(np, QStringLiteral("Continue to files and languages"))->isEnabled(), 15000);
        shot(QStringLiteral("new-project-start"));
        button(np, QStringLiteral("Continue to files and languages"))->click();
        QCOMPARE(np->step(), 1);
        QTRY_VERIFY_WITH_TIMEOUT(named<QComboBox>(np, QStringLiteral("Use of Hinweise.txt")) && named<QComboBox>(np, QStringLiteral("Use of Hinweise.txt"))->isEnabled(), 10000);
        QCOMPARE(named<QComboBox>(np, QStringLiteral("Use of Hinweise.txt"))->currentText(), QStringLiteral("Translate"));
        QTRY_VERIFY(named<LanguageCombo>(np, QStringLiteral("Source language")));
        QCOMPARE(named<LanguageCombo>(np, QStringLiteral("Source language"))->language(), QStringLiteral("de-DE"));
        auto *detection = np->findChild<QFrame *>(QStringLiteral("sourceDetection"));
        QVERIFY(detection);
        QVERIFY2(detection->accessibleName().startsWith(QStringLiteral("Probably German (Germany) (70%)")), qPrintable(detection->accessibleName()));
        // The project is named after its file; without a target language it goes no further.
        QCOMPARE(named<QLineEdit>(np, QStringLiteral("Project name"))->text(), QStringLiteral("Hinweise"));
        QVERIFY(!button(np, QStringLiteral("Continue to memories"))->isEnabled());
        named<LanguageCombo>(np, QStringLiteral("Add another target language"))->setLanguage(QStringLiteral("en-GB"));
        QVERIFY(named<QPushButton>(np, QStringLiteral("Add the target language"))->isEnabled());
        named<QPushButton>(np, QStringLiteral("Add the target language"))->click();
        QTRY_VERIFY(named<QCheckBox>(np, QStringLiteral("English (United Kingdom)")));
        QVERIFY(named<QCheckBox>(np, QStringLiteral("English (United Kingdom)"))->isChecked());
        QTest::keyClicks(named<QLineEdit>(np, QStringLiteral("Client")), QStringLiteral("Kanzlei Weber"));
        QVERIFY(button(np, QStringLiteral("Continue to memories"))->isEnabled());
        shot(QStringLiteral("new-project-files"));
        button(np, QStringLiteral("Continue to memories"))->click();
        QCOMPARE(np->step(), 2);

        // The memory of the language pair is offered and added: looked up
        // in, not updated, since the project memory collects.
        const QString en = QStringLiteral("English (United Kingdom)");
        auto *pick = named<QComboBox>(np, QStringLiteral("Add a memory for %1").arg(en));
        QVERIFY(pick);
        QCOMPARE(pick->count(), 2);
        QVERIFY(!named<QPushButton>(np, QStringLiteral("Add the memory for %1").arg(en))->isEnabled());
        pick->setCurrentIndex(1);
        named<QPushButton>(np, QStringLiteral("Add the memory for %1").arg(en))->click();
        QJsonArray chosen = np->form()->settings().value("memories").toObject().value("en-GB").toArray();
        QCOMPARE(chosen.size(), 1);
        QCOMPARE(chosen.at(0).toObject().value("memory").toString(), QStringLiteral("Weber DE-EN"));
        QVERIFY(chosen.at(0).toObject().value("lookup").toBool());
        QVERIFY(!chosen.at(0).toObject().value("update").toBool());
        QVERIFY(named<QSpinBox>(np, QStringLiteral("Penalty for Weber DE-EN, in points")));
        named<QSpinBox>(np, QStringLiteral("Penalty for Weber DE-EN, in points"))->setValue(2);
        // A new memory is made with the project; a name that is taken is refused.
        named<QPushButton>(np, QStringLiteral("New memory for %1").arg(en))->click();
        auto *newName = named<QLineEdit>(np, QStringLiteral("Name of the new memory"));
        QVERIFY(newName);
        QCOMPARE(newName->text(), QStringLiteral("Kanzlei Weber (DE-DE-EN-GB)"));
        newName->setText(QStringLiteral("weber de-en"));
        QVERIFY(!button(np, QStringLiteral("Create memory"))->isEnabled());
        newName->setText(QStringLiteral("Weber neu"));
        button(np, QStringLiteral("Create memory"))->click();
        chosen = np->form()->settings().value("memories").toObject().value("en-GB").toArray();
        QCOMPARE(chosen.size(), 2);
        QCOMPARE(chosen.at(0).toObject().value("penalty").toInt(), 2);
        // The new one is moved first.
        named<QPushButton>(np, QStringLiteral("Move Weber neu up"))->click();
        QCOMPARE(np->form()->settings().value("memories").toObject().value("en-GB").toArray().at(0).toObject().value("memory").toString(), QStringLiteral("Weber neu"));
        shot(QStringLiteral("new-project-memories"));
        button(np, QStringLiteral("Continue to terminology"))->click();
        QCOMPARE(np->step(), 3);
        // Without a master the project keeps its own termbase.
        QCOMPARE(np->form()->settings().value("terminology").toObject().value("mode").toString(), QStringLiteral("own"));
        shot(QStringLiteral("new-project-terminology"));
        button(np, QStringLiteral("Continue to review"))->click();
        QCOMPARE(np->step(), 4);

        // The review analyses the files before anything is made.
        QTRY_VERIFY_WITH_TIMEOUT(named<QTableWidget>(np, QStringLiteral("Match analysis, %1").arg(en)), 15000);
        auto *analysis = named<QTableWidget>(np, QStringLiteral("Match analysis, %1").arg(en));
        QCOMPARE(analysis->item(analysis->rowCount() - 1, 0)->text(), QStringLiteral("Total"));
        QTRY_VERIFY_WITH_TIMEOUT(button(np, QStringLiteral("Create project"))->isEnabled(), 10000);
        // The settings are saved as a template with it; a template needs a name.
        for (QCheckBox *c : np->findChildren<QCheckBox *>()) {
            if (c->text() == QStringLiteral("Save these settings as a template"))
                c->setChecked(true);
        }
        QTRY_VERIFY(named<QLineEdit>(np, QStringLiteral("Template name")));
        QCOMPARE(named<QLineEdit>(np, QStringLiteral("Template name"))->text(), QStringLiteral("Kanzlei Weber"));
        shot(QStringLiteral("new-project-review"));
        QSignalSpy made(np, &NewProjectPage::created);
        button(np, QStringLiteral("Create project"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(made.size(), 1, 20000);
        const qint64 id = made.first().at(1).toLongLong();
        const QString where = made.first().at(0).toString();
        QVERIFY(id > 0);

        ApiResult r = wait(api->get(QStringLiteral("/api/projects/%1").arg(id)));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        const QJsonObject s = r.object().value("settings").toObject();
        QCOMPARE(s.value("client").toString(), QStringLiteral("Kanzlei Weber"));
        const QJsonArray mems = s.value("memories").toObject().value("en-GB").toArray();
        QCOMPARE(mems.size(), 3);
        QCOMPARE(mems.at(0).toObject().value("memory").toString(), QStringLiteral("Hinweise (en-GB)"));
        QCOMPARE(mems.at(1).toObject().value("memory").toString(), QStringLiteral("Weber neu"));
        QCOMPARE(mems.at(2).toObject().value("memory").toString(), QStringLiteral("Weber DE-EN"));
        QCOMPARE(s.value("terminology").toObject().value("termbase").toString(), QStringLiteral("Hinweise (project)"));
        QVERIFY(wait(api->get(QStringLiteral("/api/memories/Weber%20neu"))).ok());
        r = wait(api->get(QStringLiteral("/api/templates")));
        QCOMPARE(r.array().size(), 1);
        QCOMPARE(r.array().at(0).toObject().value("name").toString(), QStringLiteral("Kanzlei Weber"));
        // The template keeps the memories chosen, not the project's own.
        QCOMPARE(r.array().at(0).toObject().value("memories").toObject().value("en-GB").toArray().size(), 2);

        // The new project opens in the editor; the list shows it.
        QTRY_VERIFY_WITH_TIMEOUT(page.openEditor(), 10000);
        QVERIFY(!page.newProjectPage());
        QTRY_COMPARE_WITH_TIMEOUT(page.model()->rowCount(), 1, 10000);

        // A second project can start from the template or from the first.
        page.newProject();
        np = page.newProjectPage();
        QRadioButton *fromTemplate = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            for (QRadioButton *b : np->findChildren<QRadioButton *>()) {
                if (b->text() == QStringLiteral("Template: Kanzlei Weber"))
                    fromTemplate = b;
            }
            return fromTemplate != nullptr;
        }(), 10000);
        bool like = false;
        for (QRadioButton *b : np->findChildren<QRadioButton *>())
            like = like || b->text() == QStringLiteral("Like an earlier project");
        QVERIFY(like);
        fromTemplate->setChecked(true);
        QTRY_COMPARE(np->form()->settings().value("sourceLang").toString(), QStringLiteral("de-DE"));
        QCOMPARE(np->form()->settings().value("memories").toObject().value("en-GB").toArray().size(), 2);
        QCOMPARE(np->form()->settings().value("client").toString(), QStringLiteral("Kanzlei Weber"));
        shot(QStringLiteral("new-project-from-template"));
        QSignalSpy cancelled(np, &NewProjectPage::cancelled);
        named<QPushButton>(np, QStringLiteral("Cancel the new project"))->click();
        QCOMPARE(cancelled.size(), 1);
        QVERIFY(!page.newProjectPage());

        // The project page shows its file and how far it is.
        page.open(where, id);
        ProjectPage *pp = page.openProject();
        QVERIFY(pp);
        const auto progress = [pp]() -> QPushButton * { return pp->findChild<QPushButton *>(QStringLiteral("progress")); };
        QTRY_VERIFY_WITH_TIMEOUT(progress(), 10000);
        QCOMPARE(progress()->text(), QStringLiteral("0%"));
        QCOMPARE(progress()->property("segments").toInt(), 2);

        // A translation saved elsewhere shows up through the live feed.
        r = wait(api->get(QStringLiteral("/api/projects/%1/segments").arg(id), QUrlQuery{{"file", "Hinweise.txt"}, {"lang", "en-GB"}}));
        const QJsonObject seg = r.array().at(0).toObject();
        r = wait(api->put(QStringLiteral("/api/projects/%1/segments").arg(id),
                          QJsonObject{{"target", QJsonArray{QJsonObject{{"text", "The tenant pays the rent."}}}}, {"status", "confirmed"}},
                          seg.value("version").toInt(), QUrlQuery{{"file", "Hinweise.txt"}, {"lang", "en-GB"}, {"segment", seg.value("id").toString()}}));
        QVERIFY2(r.ok(), qPrintable(r.error.message));
        QTRY_VERIFY_WITH_TIMEOUT(progress() && progress()->text() == QStringLiteral("50%"), 10000);

        // The settings are in panels down the page, and saved together.
        QTRY_VERIFY_WITH_TIMEOUT(named<QLineEdit>(pp, QStringLiteral("Client")) && named<QLineEdit>(pp, QStringLiteral("Client"))->text() == QStringLiteral("Kanzlei Weber"), 10000);
        QVERIFY(!button(pp, QStringLiteral("Save changes"))->isEnabled());
        QVERIFY(!pp->unsaved());
        auto *client = named<QLineEdit>(pp, QStringLiteral("Client"));
        client->setFocus();
        client->selectAll();
        QTest::keyClicks(client, QStringLiteral("Weber & Partner"));
        QVERIFY(pp->unsaved());
        QVERIFY(button(pp, QStringLiteral("Save changes"))->isEnabled());
        // Unsaved, pre-translation and the template wait.
        QVERIFY(!button(pp, QStringLiteral("Pre-translate now"))->isEnabled());
        // Discarding puts the saved settings back.
        button(pp, QStringLiteral("Discard changes"))->click();
        QCOMPARE(named<QLineEdit>(pp, QStringLiteral("Client"))->text(), QStringLiteral("Kanzlei Weber"));
        QVERIFY(!pp->unsaved());
        client = named<QLineEdit>(pp, QStringLiteral("Client"));
        client->setFocus();
        client->selectAll();
        QTest::keyClicks(client, QStringLiteral("Weber & Partner"));
        // A second target language is added, and a rule of the project's own.
        named<LanguageCombo>(pp, QStringLiteral("Add another target language"))->setLanguage(QStringLiteral("fr-FR"));
        named<QPushButton>(pp, QStringLiteral("Add the target language"))->click();
        button(pp, QStringLiteral("Add a rule"))->click();
        QTRY_VERIFY(named<QLineEdit>(pp, QStringLiteral("Name of rule 1")));
        QTest::keyClicks(named<QLineEdit>(pp, QStringLiteral("Name of rule 1")), QStringLiteral("No double spaces"));
        QTest::keyClicks(named<QLineEdit>(pp, QStringLiteral("Translation pattern of rule 1")), QStringLiteral("  "));
        // A segmentation rule of the project's own: one that is no regular expression is refused.
        button(pp, QStringLiteral("Add a segmentation rule"))->click();
        QTRY_VERIFY(named<QLineEdit>(pp, QStringLiteral("Text before the place, segmentation rule 1")));
        QTest::keyClicks(named<QLineEdit>(pp, QStringLiteral("Text before the place, segmentation rule 1")), QStringLiteral("("));
        button(pp, QStringLiteral("Save changes"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(pp->findChild<QWidget *>()->window() && [pp] {
            for (QLabel *l : pp->findChildren<QLabel *>())
                if (l->isVisible() && l->text().contains(QStringLiteral("not a regular expression that can be used")))
                    return true;
            return false;
        }(), 10000);
        QVERIFY(pp->unsaved());
        auto *before = named<QLineEdit>(pp, QStringLiteral("Text before the place, segmentation rule 1"));
        before->setFocus();
        before->selectAll();
        QTest::keyClicks(before, QStringLiteral(";"));
        QTest::keyClicks(named<QLineEdit>(pp, QStringLiteral("Text after the place, segmentation rule 1")), QStringLiteral("\\s"));
        auto *what = pp->findChild<QComboBox *>(QStringLiteral("segBreak0"));
        QVERIFY(what);
        what->setCurrentIndex(1);
        button(pp, QStringLiteral("Save changes"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!pp->unsaved(), 10000);
        QVERIFY2(pp->lastNotice().contains(QStringLiteral("French (France) added to every file")), qPrintable(pp->lastNotice()));
        {
            const QJsonArray rules = wait(api->get(QStringLiteral("/api/projects/%1").arg(id))).object().value("settings").toObject().value("segmentation").toArray();
            QCOMPARE(rules.size(), 1);
            QCOMPARE(rules.at(0).toObject().value("before").toString(), QStringLiteral(";"));
            QCOMPARE(rules.at(0).toObject().value("after").toString(), QStringLiteral("\\s"));
            QVERIFY(rules.at(0).toObject().value("break").toBool());
        }
        r = wait(api->get(QStringLiteral("/api/projects/%1").arg(id)));
        const QJsonObject saved = r.object().value("settings").toObject();
        QCOMPARE(saved.value("client").toString(), QStringLiteral("Weber & Partner"));
        QCOMPARE(saved.value("targetLangs").toArray().size(), 2);
        QCOMPARE(saved.value("qa").toObject().value("rules").toArray().at(0).toObject().value("name").toString(), QStringLiteral("No double spaces"));
        QCOMPARE(saved.value("terminology").toObject().value("termbase").toString(), QStringLiteral("Hinweise (project)"));
        QCOMPARE(saved.value("files").toArray().size(), 1);

        // The analysis counts its words, language by language.
        button(pp, QStringLiteral("Analyse the files"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(pp->findChild<QTableWidget *>(QStringLiteral("analysis")), 10000);
        auto *table = pp->findChild<QTableWidget *>(QStringLiteral("analysis"));
        QCOMPARE(table->item(table->rowCount() - 1, 0)->text(), QStringLiteral("Total"));
        QVERIFY(table->item(table->rowCount() - 1, 2)->text().toInt() >= 20);
        QVERIFY(button(pp, QStringLiteral("Save as CSV")));

        // The settings are saved as a template again: the name is taken, so it is replaced.
        auto *templateName = pp->findChild<QLineEdit *>(QStringLiteral("templateName"));
        QVERIFY(templateName);
        templateName->setText(QStringLiteral("kanzlei weber"));
        QCOMPARE(pp->findChild<QPushButton *>(QStringLiteral("saveTemplate"))->text(), QStringLiteral("Replace template"));
        templateName->setText(QStringLiteral("Weber, zwei Sprachen"));
        QCOMPARE(pp->findChild<QPushButton *>(QStringLiteral("saveTemplate"))->text(), QStringLiteral("Save as template"));
        pp->findChild<QPushButton *>(QStringLiteral("saveTemplate"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/templates"))).array().size(), 2, 10000);
        shot(QStringLiteral("project-page"));
        if (!shots.isEmpty()) {
            // The whole page, to look at.
            auto *scroll = pp->findChild<QScrollArea *>(QStringLiteral("page"));
            QWidget *inner = scroll->widget();
            inner->grab().save(QStringLiteral("%1/project-page-full.png").arg(shots));
        }

        // Deleting asks first, then the list is back.
        auto *del = pp->findChild<Web::ConfirmButton *>(QStringLiteral("deleteProject"));
        QVERIFY(del);
        del->button()->click();
        QVERIFY(del->asking());
        QSignalSpy gone(pp, &ProjectPage::gone);
        del->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(gone.size(), 1, 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/projects/%1").arg(id))).status, 404);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestProjects)
#include "tst_projects.moc"
