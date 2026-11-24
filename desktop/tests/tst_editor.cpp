#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "editorpage.h"
#include "projectpage.h"
#include "projectspage.h"
#include "segmentcontent.h"
#include "segmentmodel.h"
#include "targeteditor.h"
#include "testcore.h"
#include "theme.h"

#include <QApplication>
#include <QCheckBox>
#include <QComboBox>
#include <QDialog>
#include <QLineEdit>
#include <QListWidget>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStackedWidget>
#include <QStandardPaths>
#include <QTableView>
#include <QTableWidget>
#include <QTest>
#include <QTimer>
#include <QTreeWidget>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>()) {
        if (b->text().remove(QLatin1Char('&')) == text)
            return b;
    }
    return nullptr;
}

static QJsonArray text(const QString &s)
{
    return QJsonArray{QJsonObject{{QStringLiteral("text"), s}}};
}

class TestEditor : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_editor"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void contentWithTags()
    {
        const QJsonObject open{{"kind", "open"}, {"id", "1"}, {"type", "bold"}, {"native", "<b>"}};
        const QJsonObject close{{"kind", "close"}, {"id", "1"}, {"type", "bold"}, {"native", "</b>"}};
        const QJsonArray content{QJsonObject{{"text", "Bitte "}}, QJsonObject{{"tag", open}},
                                 QJsonObject{{"text", "jetzt"}}, QJsonObject{{"tag", close}},
                                 QJsonObject{{"text", " zahlen."}}};
        QCOMPARE(SegmentContent::plainText(content), QStringLiteral("Bitte jetzt zahlen."));
        QCOMPARE(SegmentContent::tagLabel(open), QStringLiteral("1›"));
        QCOMPARE(SegmentContent::tagLabel(close), QStringLiteral("‹1"));
        // Through a document and back, the tags stay whole.
        TargetEditor e;
        e.setContent(content);
        QCOMPARE(e.content(), content);
        QCOMPARE(e.toPlainText(), QStringLiteral("Bitte ￼jetzt￼ zahlen."));
        // Two tags side by side stay two.
        const QJsonArray twin{QJsonObject{{"tag", open}}, QJsonObject{{"tag", open}}};
        e.setContent(twin);
        QCOMPARE(e.content(), twin);
        // A match's words that the segment lacks are marked.
        const QString diff = SegmentContent::diffHtml(QStringLiteral("Die Miete ist fällig"),
                                                      QStringLiteral("Die Kaution ist fällig"));
        QCOMPARE(diff, QStringLiteral("Die <span class=\"mark\">Kaution</span> ist fällig"));
    }

    void translateAFile()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();

        // A memory with one unit, and a project using it.
        QVERIFY(wait(api->post(QStringLiteral("/api/memories"),
                               QJsonObject{{"name", "M"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());
        QVERIFY(wait(api->post(QStringLiteral("/api/memories/M/units"),
                               QJsonObject{{"source", text("Der Vertrag beginnt heute.")},
                                           {"target", text("The contract starts today.")}})).ok());
        QVERIFY(wait(api->post(QStringLiteral("/api/memories/M/units"),
                               QJsonObject{{"source", text("Die Kaution war fällig.")},
                                           {"target", text("The deposit was due.")}})).ok());
        QJsonObject settings = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
        QVERIFY(!settings.isEmpty());
        settings.insert("name", "Mietvertrag");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("memories", QJsonObject{{"en-GB", QJsonArray{QJsonObject{{"memory", "M"}, {"lookup", true}, {"concordance", true}, {"update", true}}}}});
        QJsonObject terminology = settings.value("terminology").toObject();
        terminology.insert("termbase", "Mietvertrag terms");
        settings.insert("terminology", terminology);
        const QByteArray txt = QStringLiteral("Der Vertrag beginnt heute. Die Miete ist fällig. Die Kaution ist fällig. Die Miete ist fällig.\n").toUtf8();
        const QByteArray xlf =
            "<?xml version=\"1.0\"?><xliff version=\"1.2\" xmlns=\"urn:oasis:names:tc:xliff:document:1.2\">"
            "<file source-language=\"de-DE\" target-language=\"en-GB\" datatype=\"plaintext\" original=\"b.txt\"><body>"
            "<trans-unit id=\"1\"><source>Bitte <g id=\"1\">jetzt</g> zahlen.</source></trans-unit>"
            "</body></file></xliff>";
        const ApiResult made = wait(api->post(
            QStringLiteral("/api/projects"),
            QJsonObject{{"settings", settings},
                        {"files", QJsonArray{QJsonObject{{"name", "Mietvertrag.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(txt.toBase64())}},
                                             QJsonObject{{"name", "Hinweis.xlf"}, {"role", "translatable"}, {"data", QString::fromLatin1(xlf.toBase64())}}}},
                        {"termbase", QJsonObject{{"name", "Mietvertrag terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        const QString base = QStringLiteral("/api/projects/%1").arg(id);
        QVERIFY(wait(api->post(base + "/terms",
                               QJsonObject{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Vertrag"}}}}},
                                                                    QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", "contract"}}}}}}}})).ok());

        Theme::apply(*qApp);
        // The project page opens the editor on a file and language.
        ProjectsPage projects(&m);
        projects.resize(1280, 820);
        projects.show();
        projects.open(m.local()->id(), id);
        ProjectPage *page = projects.openProject();
        QVERIFY(page);
        auto *files = page->findChild<QTreeWidget *>();
        QTreeWidgetItem *lang = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT(files->topLevelItemCount() == 2, 10000);
        for (int i = 0; i < files->topLevelItemCount(); ++i) {
            QTreeWidgetItem *f = files->topLevelItem(i);
            if (f->data(0, Qt::UserRole).toString() == "Mietvertrag.txt" && f->childCount() > 0)
                lang = f->child(0);
        }
        QVERIFY(lang);
        files->setCurrentItem(lang);
        button(page, "Translate")->click();
        EditorPage *ed = projects.openEditor();
        QVERIFY(ed);
        QCOMPARE(ed->file(), QStringLiteral("Mietvertrag.txt"));
        QTRY_COMPARE_WITH_TIMEOUT(ed->model()->rowCount(), 4, 10000);
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 0, 10000);

        // The match column: what the memory holds for each segment, before
        // anything is inserted. The third segment has a match with one other
        // word, the fourth repeats the second, and the second
        // has nothing.
        auto offer = [ed](int row) { return ed->model()->index(row, SegmentModel::State).data(SegmentModel::AvailableRole).toJsonObject(); };
        QTRY_VERIFY_WITH_TIMEOUT(!offer(2).isEmpty(), 10000);
        QVERIFY2(offer(2).value("score").toInt() >= 70 && offer(2).value("score").toInt() < 100, qPrintable(QString::number(offer(2).value("score").toInt())));
        QCOMPARE(offer(2).value("memory").toString(), QStringLiteral("M"));
        QCOMPARE(offer(3).value("band").toString(), QStringLiteral("repetition"));
        QVERIFY(offer(1).isEmpty());
        QVERIFY(ed->model()->index(2, SegmentModel::State).data(Qt::ToolTipRole).toString().contains(QStringLiteral("in M, not inserted")));
        QVERIFY(ed->model()->index(3, SegmentModel::State).data(Qt::DisplayRole).toString().contains(QStringLiteral("Repeats an earlier segment")));
        // The first has its exact match inserted already: no offer beside it.
        QTRY_VERIFY_WITH_TIMEOUT(offer(0).isEmpty(), 10000);

        // The memory's match and the term are offered for the first segment.
        auto hasKind = [ed](const QString &kind) {
            for (const QJsonValue &v : ed->results()) {
                if (v.toObject().value("kind").toString() == kind)
                    return true;
            }
            return false;
        };
        QTRY_VERIFY_WITH_TIMEOUT(hasKind("match") && hasKind("term"), 10000);
        QCOMPARE(ed->results().at(0).toObject().value("score").toInt(), 100);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            projects.grab().save(dir + QStringLiteral("/editor-desktop.png"));
        }
        // The best match is already in the empty segment, as in Trados;
        // inserting it by hand gives the same.
        QTRY_COMPARE_WITH_TIMEOUT(SegmentContent::plainText(ed->target()->content()), QStringLiteral("The contract starts today."), 5000);
        ed->insertResult(1);
        QCOMPARE(SegmentContent::plainText(ed->target()->content()), QStringLiteral("The contract starts today."));
        QSignalSpy saved(ed, &EditorPage::saved);
        ed->confirm();
        QVERIFY(saved.wait(10000));
        // On to the next segment, and the first is confirmed on the server,
        // with where its translation came from.
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 1, 10000);
        QJsonArray segs = wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array();
        QCOMPARE(segs.at(0).toObject().value("status").toString(), QStringLiteral("confirmed"));
        QCOMPARE(segs.at(0).toObject().value("origin").toObject().value("kind").toString(), QStringLiteral("tm"));

        // Checks run as you type; a confirmed segment keeps its warnings,
        // and its repetition is filled.
        QSignalSpy checked(ed, &EditorPage::checked);
        projects.activateWindow();
        QVERIFY(QTest::qWaitForWindowActive(&projects));
        QTRY_VERIFY_WITH_TIMEOUT(ed->target(), 5000);
        ed->target()->setFocus();
        ed->target()->setContent({});
        QTest::keyClicks(ed->target(), "The rent  is due");
        QVERIFY(checked.wait(5000));
        QTRY_VERIFY_WITH_TIMEOUT([ed] {
            for (const QJsonValue &v : ed->issues()) {
                if (v.toObject().value("kind").toString() == "double-space")
                    return true;
            }
            return false;
        }(), 5000);
        // The grid shows the translation before it is saved.
        QCOMPARE(ed->model()->index(1, SegmentModel::Target).data().toString(), QStringLiteral("The rent  is due"));
        saved.clear();
        QTest::keyClick(ed->target(), Qt::Key_Return, Qt::ControlModifier);
        QTRY_VERIFY_WITH_TIMEOUT(ed->message().contains("Also filled 1 repetition (segment 4)"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 2, 10000);
        QTRY_VERIFY_WITH_TIMEOUT(wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array().at(3).toObject().value("status").toString() == "confirmed", 10000);
        segs = wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array();
        QCOMPARE(segs.at(1).toObject().value("status").toString(), QStringLiteral("confirmed"));
        QVERIFY(!segs.at(1).toObject().value("warnings").toArray().isEmpty());
        QCOMPARE(segs.at(3).toObject().value("origin").toObject().value("kind").toString(), QStringLiteral("repetition"));

        // Memory search in the panel finds the memory's unit.
        ed->openLookup("Vertrag");
        QTRY_VERIFY_WITH_TIMEOUT([ed] {
            for (QWidget *w : ed->findChildren<QWidget *>()) {
                if (w->isVisible() && w->accessibleName().contains("The contract starts today."))
                    return true;
            }
            return false;
        }(), 10000);

        // The filter: All, To do, Warnings. The active segment always shows.
        button(ed, "To do")->click();
        QCOMPARE(ed->shownCount(), 1);
        button(ed, "Warnings (1)")->click();
        QCOMPARE(ed->shownCount(), 2);
        button(ed, "All")->click();
        QCOMPARE(ed->shownCount(), 4);

        // Someone else saves the segment being worked on: both are shown,
        // and theirs can be taken.
        ed->goToSegment("s1");
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 0, 5000);
        segs = wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array();
        ed->target()->setContent(text("My version"));
        emit ed->target()->edited();
        const int version = segs.at(0).toObject().value("version").toInt();
        QVERIFY(wait(api->put(base + "/segments", QJsonObject{{"target", text("Their version")}, {"status", "draft"}}, version,
                              {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}, {"segment", "s1"}})).ok());
        QPushButton *takeTheirs = button(ed, "Take theirs");
        QTRY_VERIFY_WITH_TIMEOUT(takeTheirs->isVisible(), 10000);
        QVERIFY(ed->message().contains("Their version"));
        takeTheirs->click();
        QCOMPARE(SegmentContent::plainText(ed->target()->content()), QStringLiteral("Their version"));

        // Adding a term with the form in the panel.
        ed->openCapture();
        auto *capSource = ed->findChild<QLineEdit *>("captureSource");
        auto *capTarget = ed->findChild<QLineEdit *>("captureTarget");
        QVERIFY(capSource && capTarget && capSource->isVisible());
        capSource->setText("Miete");
        capTarget->setText("rent");
        button(ed, "Add term")->click();
        QTRY_VERIFY_WITH_TIMEOUT(ed->message().contains("Added “Miete”"), 10000);
        const QJsonArray hits = wait(api->post(base + "/recognize", QJsonObject{{"lang", "en-GB"}, {"text", "Die Miete"}})).array();
        QCOMPARE(hits.size(), 1);

        // Others see where this person is.
        const QJsonArray here = wait(api->get(base + "/presence", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array();
        QCOMPARE(here.size(), 1);
        QCOMPARE(here.at(0).toObject().value("segment").toString(), QStringLiteral("s1"));

        // The Memories and Analysis dialogs.
        int memoryRows = -1;
        QTimer::singleShot(300, [&memoryRows] {
            auto *dlg = qobject_cast<QDialog *>(QApplication::activeModalWidget());
            if (!dlg)
                return;
            for (QTableWidget *t : dlg->findChildren<QTableWidget *>()) {
                if (t->accessibleName() == QLatin1String("Memories of this project"))
                    memoryRows = t->rowCount();
            }
            dlg->accept();
        });
        button(ed, "Memories")->click();
        QCOMPARE(memoryRows, 1);
        int bands = -1;
        QTimer::singleShot(200, [&bands] {
            auto *dlg = qobject_cast<QDialog *>(QApplication::activeModalWidget());
            if (!dlg)
                return;
            QTRY_VERIFY_WITH_TIMEOUT(dlg->findChild<QTableWidget *>(), 10000);
            bands = dlg->findChild<QTableWidget *>()->rowCount();
            dlg->accept();
        });
        button(ed, "Analysis")->click();
        QVERIFY(bands > 1);

        // Tags: the source's tags go in with F9 and are saved as tags.
        emit ed->back();
        QTRY_VERIFY_WITH_TIMEOUT(!projects.openEditor(), 5000);
        projects.edit("Hinweis.xlf", "en-GB");
        ed = projects.openEditor();
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 0, 10000);
        // Keys reach the shortcuts of the active window only.
        projects.activateWindow();
        QVERIFY(QTest::qWaitForWindowActive(&projects));
        ed->target()->setFocus();
        QTest::keyClicks(ed->target(), "Please ");
        QTest::keyClick(ed->target(), Qt::Key_F9);
        QTest::keyClicks(ed->target(), "now");
        QTest::keyClick(ed->target(), Qt::Key_F9);
        QTest::keyClicks(ed->target(), " pay.");
        const QJsonArray withTags = ed->target()->content();
        QCOMPARE(SegmentContent::tags(withTags).size(), 2);
        saved.clear();
        QSignalSpy saved2(ed, &EditorPage::saved);
        ed->confirm();
        QVERIFY(saved2.wait(10000));
        segs = wait(api->get(base + "/segments", {{"file", "Hinweis.xlf"}, {"lang", "en-GB"}})).array();
        const QJsonArray stored = segs.at(0).toObject().value("target").toArray();
        QCOMPARE(SegmentContent::tags(stored).size(), 2);
        QCOMPARE(SegmentContent::plainText(stored), QStringLiteral("Please now pay."));
        QCOMPARE(segs.at(0).toObject().value("status").toString(), QStringLiteral("confirmed"));

    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEditor)
#include "tst_editor.moc"
