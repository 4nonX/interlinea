#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "editorpage.h"
#include "webui.h"
#include "projectpage.h"
#include "projectspage.h"
#include "segmentcontent.h"
#include "segmentmodel.h"
#include "targeteditor.h"
#include "testcore.h"
#include "theme.h"

#include <QScrollArea>
#include <QTemporaryDir>
#include <QLabel>
#include <QPointer>
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
#include <QPlainTextEdit>
#include <QTcpServer>
#include <QTcpSocket>
#include <QJsonDocument>
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

static QLabel *labelWith(QWidget *root, const QString &part)
{
    for (QLabel *l : root->findChildren<QLabel *>()) {
        if (l->text().contains(part) && l->isVisible())
            return l;
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

    // Typing two letters offers the words that start with them; Tab or
    // Enter takes one, Escape closes the list, and what was typed stays.
    void completions()
    {
        QCOMPARE(TargetEditor::completionsFor(QStringLiteral("b"), {QStringLiteral("Basic")}), QStringList());
        QCOMPARE(TargetEditor::completionsFor(QStringLiteral("ba"), {QStringLiteral("Basic rent"), QStringLiteral("basic"), QStringLiteral("Basic"), QStringLiteral("Bank"), QStringLiteral("ba"), QStringLiteral("rent")}),
                 (QStringList{QStringLiteral("Bank"), QStringLiteral("basic"), QStringLiteral("Basic rent")}));
        QCOMPARE(TargetEditor::completionsFor(QStringLiteral("ÜB"), {QStringLiteral("Übergabe")}), QStringList{QStringLiteral("Übergabe")});

        QWidget page;
        page.resize(600, 300);
        auto *e = new TargetEditor(&page);
        e->setGeometry(20, 20, 400, 80);
        auto *other = new QLineEdit(&page);
        other->setGeometry(20, 120, 200, 30);
        page.show();
        page.activateWindow();
        e->setFocus();
        QTRY_VERIFY(e->hasFocus());
        e->setCompletions({QStringLiteral("Basic rent"), QStringLiteral("Basic"), QStringLiteral("Deposit")});
        QTest::keyClicks(e, QStringLiteral("b"));
        QVERIFY(e->offered().isEmpty());
        QTest::keyClicks(e, QStringLiteral("a"));
        QCOMPARE(e->offered(), (QStringList{QStringLiteral("Basic"), QStringLiteral("Basic rent")}));
        QCOMPARE(e->offeredIndex(), 0);
        auto *list = page.findChild<QListWidget *>(QStringLiteral("completions"));
        QVERIFY(list && list->isVisible() && page.rect().contains(list->geometry()) && list->y() > e->y());
        if (qEnvironmentVariableIsSet("INTERLINEA_SHOTS"))
            page.grab().save(qEnvironmentVariable("INTERLINEA_SHOTS") + QStringLiteral("/completions.png"));
        // Escape closes the list and nothing else.
        QTest::keyClick(e, Qt::Key_Escape);
        QVERIFY(e->offered().isEmpty());
        QCOMPARE(e->toPlainText(), QStringLiteral("ba"));
        // Typing on offers again; the arrows choose and go round.
        QTest::keyClicks(e, QStringLiteral("s"));
        QCOMPARE(e->offered().size(), 2);
        QTest::keyClick(e, Qt::Key_Down);
        QCOMPARE(e->offeredIndex(), 1);
        QTest::keyClick(e, Qt::Key_Down);
        QCOMPARE(e->offeredIndex(), 0);
        QTest::keyClick(e, Qt::Key_Up);
        QCOMPARE(e->offeredIndex(), 1);
        // Tab takes the chosen one and stays in the translation.
        QSignalSpy edited(e, &TargetEditor::edited);
        QTest::keyClick(e, Qt::Key_Tab);
        QCOMPARE(e->toPlainText(), QStringLiteral("basic rent"));
        QVERIFY(e->offered().isEmpty() && e->hasFocus() && edited.size() == 1);
        // Enter takes one too, and adds no line.
        QTest::keyClicks(e, QStringLiteral(" De"));
        QCOMPARE(e->offered(), QStringList{QStringLiteral("Deposit")});
        QTest::keyClick(e, Qt::Key_Return);
        QCOMPARE(e->toPlainText(), QStringLiteral("basic rent Deposit"));
        // In the middle of a word nothing is offered.
        e->setContent(QJsonArray{QJsonObject{{"text", "sic"}}});
        e->moveCursor(QTextCursor::Start);
        QTest::keyClicks(e, QStringLiteral("ba"));
        QVERIFY(e->offered().isEmpty());
        // Without a list, Tab moves on as before.
        QTest::keyClick(e, Qt::Key_Tab);
        QVERIFY(other->hasFocus());
        // Text put in from elsewhere offers nothing.
        e->setCompletions({QStringLiteral("Basic")});
        e->setContent(QJsonArray{});
        e->insertPlain(QStringLiteral("ba"));
        QVERIFY(e->offered().isEmpty());
    }

    // Every document format opens in the editor and is saved as it came:
    // a presentation, a workbook, a web page and an InDesign document as
    // translated documents, a memoQ package's file as a return package.
    void documentFormats()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        auto fixture = [](const QString &name) {
            QFile f(QStringLiteral(INTERLINEA_FIXTURES "/documents/") + name);
            return f.open(QIODevice::ReadOnly) ? f.readAll() : QByteArray();
        };
        QJsonObject settings = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Formate");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("projectMemory", false);
        QJsonObject terminology = settings.value("terminology").toObject();
        terminology.insert("mode", "own");
        terminology.insert("termbase", "Formate terms");
        settings.insert("terminology", terminology);
        QJsonArray files;
        const QStringList names{"Folien.pptx", "Tabelle.xlsx", "Seite.html", "Layout.idml", "Vertrag.pdf", "film.ttml", "Untertitel.stl", "film.ass", "Untertitel.scc"};
        for (const QString &n : names) {
            const QByteArray data = fixture(n);
            QVERIFY2(!data.isEmpty(), qPrintable(n));
            // What the server says of the file before a project is made of it.
            const ApiResult in = wait(api->upload("POST", QStringLiteral("/api/files/inspect"), data, QStringLiteral("application/octet-stream"), QUrlQuery{{"name", n}}));
            QVERIFY2(in.ok() && in.object().value("translatable").toBool() && in.object().value("segments").toInt() > 0, qPrintable(n + ": " + in.error.message));
            // A PDF says what becomes of it.
            if (n.endsWith(".pdf"))
                QVERIFY(in.object().value("warnings").toArray().at(0).toString().contains(QStringLiteral("saved as that DOCX document")));
            files << QJsonObject{{"name", n}, {"role", "translatable"}, {"data", QString::fromLatin1(data.toBase64())}};
        }
        const ApiResult made = wait(api->post(QStringLiteral("/api/projects"),
                                              QJsonObject{{"settings", settings}, {"files", files}, {"termbase", QJsonObject{{"name", "Formate terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        Theme::apply(*qApp);
        const QHash<QString, int> segments{{"Folien.pptx", 18}, {"Tabelle.xlsx", 4}, {"Seite.html", 5}, {"Layout.idml", 3}, {"Vertrag.pdf", 21}, {"film.ttml", 2}, {"Untertitel.stl", 3}, {"film.ass", 2}, {"Untertitel.scc", 2}};
        for (const QString &n : names) {
            EditorPage ed(m.local(), id, n, QStringLiteral("en-GB"));
            ed.resize(1280, 820);
            ed.show();
            QTRY_COMPARE_WITH_TIMEOUT(ed.model()->rowCount(), segments.value(n), 10000);
            QPushButton *save = nullptr;
            QTRY_VERIFY_WITH_TIMEOUT((save = button(&ed, "Save translated document")) && save->isVisible(), 10000);
            QVERIFY2(button(&ed, "Preview") && button(&ed, "Preview")->isVisible(), qPrintable(n));
            if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty() && n == "Seite.html") {
                QTest::qWait(600);
                ed.grab().save(dir + QStringLiteral("/editor-html.png"));
            }
            // A subtitle is counted against the project's limits as it is
            // typed: its longest line, its lines and its reading speed.
            if (n == "film.ass") {
                const QString first = ed.model()->segments().first().value("id").toString();
                ed.goToSegment(first);
                QTRY_COMPARE_WITH_TIMEOUT(ed.activeId(), first, 5000);
                ed.target()->setContent(QJsonArray{QJsonObject{{"text", "Good evening and a very warm welcome to everybody watching tonight."}}});
                emit ed.target()->edited();
                auto *counter = ed.findChild<QLabel *>(QStringLiteral("subtitleCounter"));
                QVERIFY(counter);
                QVERIFY2(counter->text().contains(QStringLiteral("Longest line 67 of 42")) && counter->text().contains(QStringLiteral("characters per second in 2.00 s")), qPrintable(counter->text()));
                QVERIFY(counter->text().contains(QStringLiteral("font-weight:600")));
                if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
                    QTest::qWait(400);
                    ed.grab().save(dir + QStringLiteral("/editor-subtitles.png"));
                }
                ed.target()->setContent(QJsonArray{});
                emit ed.target()->edited();
            }
            // The preview is the document: its paragraphs as the file has
            // them, formatting where the segments have tags, and what is
            // not translated yet in the source text, grey and underlined.
            if (n == "Seite.html") {
                ed.showPreview(true);
                QTRY_VERIFY_WITH_TIMEOUT(!ed.layout().isEmpty(), 10000);
                QString html = ed.previewHtml();
                QVERIFY2(html.contains(QStringLiteral("<b>")) && !html.contains(QChar(0x203A)) && !html.contains(QChar(0x2039)), qPrintable(html));
                QVERIFY2(html.contains(QStringLiteral("font-size:14px; font-weight:700;")) && html.contains(QStringLiteral("Title and description of the page")), qPrintable(html));
                QVERIFY(html.contains(QStringLiteral("color:#6b7280;")) && html.contains(QStringLiteral("Not translated yet: the source text is used")));
                // A translation takes the place of its source text, in black.
                const QString first = ed.model()->segments().first().value("id").toString();
                ed.goToSegment(first);
                QTRY_COMPARE_WITH_TIMEOUT(ed.activeId(), first, 5000);
                QSignalSpy saved(&ed, &EditorPage::saved);
                ed.target()->setContent(QJsonArray{QJsonObject{{"text", "Deadlines"}}});
                emit ed.target()->edited();
                ed.confirm();
                QVERIFY(saved.wait(10000));
                html = ed.previewHtml();
                QVERIFY2(html.contains(QStringLiteral("color:#111111;\">Deadlines</a>")), qPrintable(html.left(1500)));
                if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
                    QTest::qWait(400);
                    ed.grab().save(dir + QStringLiteral("/editor-preview.png"));
                }
            }
        }
        // The translated files come back in their formats.
        const QString base = QStringLiteral("/api/projects/%1").arg(id);
        const QJsonArray segs = wait(api->get(base + "/segments", {{"file", "Seite.html"}, {"lang", "en-GB"}})).array();
        QCOMPARE(segs.at(1).toObject().value("source").toArray().at(0).toObject().value("text").toString(), QStringLiteral("Fristen im Mietrecht"));
        const ApiResult page = wait(api->get(base + "/files/translated", {{"name", "Seite.html"}, {"lang", "en-GB"}}));
        QVERIFY(page.ok());
        QCOMPARE(page.fileName(), QStringLiteral("Seite (en-GB).html"));
        QVERIFY(page.body.contains("<html lang=\"en-GB\">"));
        // The translation of a PDF is the Word document its text was read into.
        const ApiResult word = wait(api->get(base + "/files/translated", {{"name", "Vertrag.pdf"}, {"lang", "en-GB"}}));
        QVERIFY(word.ok());
        QCOMPARE(word.fileName(), QStringLiteral("Vertrag (en-GB).docx"));
        QVERIFY(word.body.startsWith("PK"));
        // Subtitles as broadcasters exchange them: the binary EBU STL file
        // comes back as one, its blocks as they were without translations.
        const ApiResult stl = wait(api->get(base + "/files/translated", {{"name", "Untertitel.stl"}, {"lang", "en-GB"}}));
        QVERIFY(stl.ok());
        QCOMPARE(stl.fileName(), QStringLiteral("Untertitel (en-GB).stl"));
        QCOMPARE(stl.body.size(), 1024 + 2 * 128);
        QCOMPARE(stl.body.mid(14, 2), QByteArray("09"));

        // A memoQ handoff package: its document is a bilingual file of
        // the project, saved back as the package's return.
        QJsonObject s2 = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
        s2.insert("name", "Hofmann memoQ");
        s2.insert("sourceLang", "de-de");
        s2.insert("targetLangs", QJsonArray{"en-gb"});
        s2.insert("projectMemory", false);
        QJsonObject t2 = s2.value("terminology").toObject();
        t2.insert("mode", "own");
        t2.insert("termbase", "Hofmann memoQ terms");
        s2.insert("terminology", t2);
        const QByteArray handoff = fixture("Hofmann.mqout");
        const ApiResult in = wait(api->upload("POST", QStringLiteral("/api/files/inspect"), handoff, QStringLiteral("application/octet-stream"), QUrlQuery{{"name", "Hofmann.mqout"}}));
        QVERIFY(in.ok());
        QCOMPARE(in.object().value("format").toString(), QStringLiteral("package"));
        QCOMPARE(in.object().value("package").toObject().value("name").toString(), QStringLiteral("Mietvertrag Hofmann"));
        const ApiResult made2 = wait(api->post(QStringLiteral("/api/projects"),
                                               QJsonObject{{"settings", s2},
                                                           {"files", QJsonArray{QJsonObject{{"name", "Hofmann.mqout"}, {"role", "package"}, {"data", QString::fromLatin1(handoff.toBase64())}}}},
                                                           {"termbase", QJsonObject{{"name", "Hofmann memoQ terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made2.ok(), qPrintable(made2.error.message));
        const qint64 id2 = made2.object().value("id").toInteger();
        const QString member = made2.object().value("settings").toObject().value("files").toArray().at(1).toObject().value("name").toString();
        QCOMPARE(member, QStringLiteral("Documents/Mietvertrag.docx.mqxliff"));
        EditorPage ed(m.local(), id2, member, QStringLiteral("en-gb"));
        ed.resize(1280, 820);
        ed.show();
        QTRY_COMPARE_WITH_TIMEOUT(ed.model()->rowCount(), 3, 10000);
        QPushButton *save = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((save = button(&ed, "Save return package")) && save->isVisible(), 10000);
        const ApiResult back = wait(api->get(QStringLiteral("/api/projects/%1/return-package").arg(id2)));
        QVERIFY(back.ok());
        QCOMPARE(back.fileName(), QStringLiteral("Hofmann.mqback"));
    }

    // Segments of a document are split and joined by the translator;
    // the file is written with what was translated where they stood.
    void splitAndJoin()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        QFile f(QStringLiteral(INTERLINEA_FIXTURES "/documents/LIESMICH.md"));
        QVERIFY(f.open(QIODevice::ReadOnly));
        const QByteArray data = f.readAll();
        QFile tf(QStringLiteral(INTERLINEA_FIXTURES "/Mietvertrag.docx.sdlxliff"));
        QVERIFY(tf.open(QIODevice::ReadOnly));
        const QByteArray trados = tf.readAll();
        QJsonObject settings = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Umschnitt");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("projectMemory", false);
        QJsonObject terminology = settings.value("terminology").toObject();
        terminology.insert("mode", "own");
        terminology.insert("termbase", "Umschnitt terms");
        settings.insert("terminology", terminology);
        const ApiResult made = wait(api->post(QStringLiteral("/api/projects"),
                                              QJsonObject{{"settings", settings},
                                                          {"files", QJsonArray{QJsonObject{{"name", "LIESMICH.md"}, {"role", "translatable"}, {"data", QString::fromLatin1(data.toBase64())}},
                                                                               QJsonObject{{"name", "Hinweise.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(QByteArray("Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.\n").toBase64())}},
                                                                               QJsonObject{{"name", "Mietvertrag.docx.sdlxliff"}, {"role", "translatable"}, {"data", QString::fromLatin1(trados.toBase64())}}}},
                                                          {"termbase", QJsonObject{{"name", "Umschnitt terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        const QString base = QStringLiteral("/api/projects/%1").arg(id);
        Theme::apply(*qApp);
        EditorPage ed(m.local(), id, QStringLiteral("LIESMICH.md"), QStringLiteral("en-GB"));
        ed.resize(1280, 820);
        ed.show();
        QTRY_COMPARE_WITH_TIMEOUT(ed.model()->rowCount(), 3, 10000);
        QTRY_COMPARE_WITH_TIMEOUT(ed.activeRow(), 0, 10000);
        auto *split = ed.findChild<QPushButton *>("splitSegment");
        auto *join = ed.findChild<QPushButton *>("joinSegment");
        auto *note = ed.findChild<QLabel *>("cutNote");
        QVERIFY(split && join && note);
        auto source = [&ed](int row) { return SegmentContent::plainText(ed.model()->segment(row).value("source").toArray()); };
        auto idOf = [&ed](int row) { return ed.model()->segment(row).value("id").toString(); };
        QSignalSpy cut(&ed, &EditorPage::resegmented);
        // A heading is not joined with the paragraph after it.
        QVERIFY(split->isVisible() && split->isEnabled() && !join->isEnabled() && !note->isVisible());

        // Two sentences of a paragraph are joined; what was typed is joined with them.
        ed.goToSegment(idOf(1));
        QTRY_VERIFY(ed.target() && ed.target()->hasFocus());
        QVERIFY(join->isEnabled());
        QTest::keyClicks(ed.target(), QStringLiteral("The tenant pays."));
        join->click();
        QTRY_COMPARE_WITH_TIMEOUT(cut.size(), 1, 10000);
        QCOMPARE(cut.at(0).at(0).toString(), QStringLiteral("join"));
        QVERIFY(cut.at(0).at(1).toBool());
        QCOMPARE(ed.model()->rowCount(), 2);
        QCOMPARE(ed.activeRow(), 1);
        QCOMPARE(source(1), QStringLiteral("Der Mieter zahlt die Miete. Der Vermieter haftet nicht."));
        QTRY_VERIFY(ed.target());
        QCOMPARE(ed.target()->toPlainText(), QStringLiteral("The tenant pays."));
        // A joined segment is split like any other; nothing follows it in its paragraph to join.
        QVERIFY(split->isEnabled() && !join->isEnabled());
        QCOMPARE(note->text(), QStringLiteral("Joined from 2 segments of the file. Split it where they met to have them apart again."));
        ed.target()->selectAll();
        QTest::keyClicks(ed.target(), QStringLiteral("The landlord is not liable, and the tenant pays."));
        QSignalSpy saved(&ed, &EditorPage::saved);
        ed.goToSegment(idOf(0));
        QTRY_VERIFY_WITH_TIMEOUT(saved.size() >= 1, 10000);
        ApiResult out = wait(api->get(base + "/files/translated", {{"name", "LIESMICH.md"}, {"lang", "en-GB"}}));
        QVERIFY(out.ok());
        QCOMPARE(QString::fromUtf8(out.body), QStringLiteral("# Mietvertrag\n\nThe landlord is not liable, and the tenant pays.\n\n```\nnicht übersetzen\n```\n"));

        // Splits the open segment at a place in its source, through the dialog.
        const auto splitAt = [&](int place) {
            Web::Dialog *d = ed.showSplitDialog();
            QVERIFY(d);
            auto *box = d->findChild<TargetEditor *>("splitSource");
            auto *go = d->findChild<QPushButton *>("save");
            QVERIFY(box && go);
            QTextCursor c = box->textCursor();
            c.setPosition(place);
            box->setTextCursor(c);
            QVERIFY(go->isEnabled());
            const int before = cut.size();
            go->click();
            QTRY_COMPARE_WITH_TIMEOUT(cut.size(), before + 1, 10000);
            QVERIFY(cut.last().at(1).toBool());
            ed.activateWindow();
        };
        // Inside its second sentence: the parts stand for both sentences, and are written one after the other.
        ed.goToSegment(idOf(1));
        QTRY_COMPARE(ed.activeRow(), 1);
        const QString joinedText = source(1);
        splitAt(int(joinedText.indexOf(QStringLiteral("haftet"))) + 2); // two tags stand before it, one place each
        QCOMPARE(ed.model()->rowCount(), 3);
        QVERIFY2(source(1).endsWith(QStringLiteral("zahlt die Miete. Der Vermieter")) && source(2) == QStringLiteral("haftet nicht."), qPrintable(source(1) + '|' + source(2)));
        QCOMPARE(note->text(), QStringLiteral("A part of 2 segments of the file that were joined."));
        QVERIFY(join->isEnabled());
        out = wait(api->get(base + "/files/translated", {{"name", "LIESMICH.md"}, {"lang", "en-GB"}}));
        QVERIFY2(out.body.contains("\n\nThe landlord is not liable, and the tenant pays. haftet nicht.\n\n"), out.body.constData());
        // Joined again and split where the sentences met, they are the file's two again.
        join->click();
        QTRY_COMPARE_WITH_TIMEOUT(cut.size(), 3, 10000);
        QCOMPARE(ed.model()->rowCount(), 2);
        splitAt(int(source(1).indexOf(QStringLiteral("Der Vermieter"))) + 2);
        QCOMPARE(ed.model()->rowCount(), 3);
        QCOMPARE(source(2), QStringLiteral("Der Vermieter haftet nicht."));
        QVERIFY(ed.model()->segment(1).value("joined").toArray().isEmpty() && !ed.model()->segment(2).contains("split"));
        QTRY_VERIFY(!note->isVisible());
        const int sofar = cut.size();

        // Split: the caret in the source is the place.
        ed.goToSegment(idOf(2));
        QTRY_COMPARE(ed.activeRow(), 2);
        const QString whole = idOf(2);
        Web::Dialog *d = ed.showSplitDialog();
        QVERIFY(d);
        auto *box = d->findChild<TargetEditor *>("splitSource");
        auto *go = d->findChild<QPushButton *>("save");
        auto *first = d->findChild<QLabel *>("splitFirst");
        auto *second = d->findChild<QLabel *>("splitSecond");
        QVERIFY(box && go && first && second);
        QTRY_VERIFY(box->hasFocus());
        QVERIFY(!go->isEnabled());
        QCOMPARE(box->toPlainText(), QStringLiteral("Der Vermieter haftet nicht."));
        // Typing changes nothing; the arrow keys move the caret.
        QTest::keyClicks(box, QStringLiteral("x"));
        QCOMPARE(box->toPlainText(), QStringLiteral("Der Vermieter haftet nicht."));
        for (int i = 0; i < 14; ++i)
            QTest::keyClick(box, Qt::Key_Right);
        QCOMPARE(box->textCursor().position(), 14);
        QVERIFY(go->isEnabled());
        QVERIFY2(first->text().contains(QStringLiteral("Der Vermieter")) && !first->text().contains(QStringLiteral("haftet")), qPrintable(first->text()));
        QVERIFY2(second->text().contains(QStringLiteral("haftet nicht.")), qPrintable(second->text()));
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            ed.window()->grab().save(dir + QStringLiteral("/editor-split.png"));
            d->grab().save(dir + QStringLiteral("/split-dialog.png"));
        }
        QPointer<Web::Dialog> open(d);
        go->click();
        QTRY_COMPARE_WITH_TIMEOUT(cut.size(), sofar + 1, 10000);
        QVERIFY(cut.at(sofar).at(1).toBool());
        QTRY_VERIFY(!open || !open->isVisible());
        QCOMPARE(ed.model()->rowCount(), 4);
        QCOMPARE(idOf(2), whole + QStringLiteral("#1"));
        QCOMPARE(source(2), QStringLiteral("Der Vermieter"));
        QCOMPARE(source(3), QStringLiteral("haftet nicht."));
        QCOMPARE(ed.activeRow(), 2);
        QCOMPARE(note->text(), QStringLiteral("A part of a segment of the file that was split."));
        QVERIFY(join->isEnabled());
        ed.goToSegment(idOf(3));
        QTRY_COMPARE(ed.activeRow(), 3);
        QTRY_VERIFY(ed.target());
        QTest::keyClicks(ed.target(), QStringLiteral("is not liable."));
        saved.clear();
        ed.goToSegment(idOf(2));
        QTRY_VERIFY_WITH_TIMEOUT(saved.size() >= 1, 10000);
        out = wait(api->get(base + "/files/translated", {{"name", "LIESMICH.md"}, {"lang", "en-GB"}}));
        QVERIFY2(out.body.contains("and the tenant pays. Der Vermieter is not liable.\n"), out.body.constData());
        // Alt+Shift+J joins the parts into the segment it was.
        // The dialog is gone; without a window system the page has to be told it is in front again.
        ed.activateWindow();
        QTRY_VERIFY(ed.isActiveWindow());
        QTest::keySequence(&ed, QKeySequence(Qt::ALT | Qt::SHIFT | Qt::Key_J));
        QTRY_COMPARE_WITH_TIMEOUT(cut.size(), sofar + 2, 10000);
        QCOMPARE(ed.model()->rowCount(), 3);
        QCOMPARE(idOf(2), whole);
        QCOMPARE(source(2), QStringLiteral("Der Vermieter haftet nicht."));
        QTRY_VERIFY(ed.target());
        QCOMPARE(ed.target()->toPlainText(), QStringLiteral("is not liable."));

        // What someone else cuts differently shows without opening the file again.
        const QJsonObject mine = ed.model()->segment(2);
        const ApiResult theirs = wait(api->post(base + "/segments/split", QJsonObject{{"before", QJsonArray{QJsonObject{{"text", "Der Vermieter"}}}}},
                                                {{"file", "LIESMICH.md"}, {"lang", "en-GB"}, {"segment", whole}, {"version", QString::number(mine.value("version").toInt())}}));
        QVERIFY2(theirs.ok(), qPrintable(theirs.error.message));
        QTRY_COMPARE_WITH_TIMEOUT(ed.model()->rowCount(), 4, 10000);
        // What the segments do not allow is said, and nothing changes.
        ed.goToSegment(idOf(0));
        ed.resegment(QStringLiteral("join"));
        QTRY_COMPARE_WITH_TIMEOUT(cut.size(), sofar + 3, 10000);
        QVERIFY(!cut.at(sofar + 2).at(1).toBool());
        QVERIFY2(ed.message().contains(QStringLiteral("different paragraphs")), qPrintable(ed.message()));
        QCOMPARE(ed.model()->rowCount(), 4);

        // A text file is cut like any document, and saved from the editor.
        {
            EditorPage text(m.local(), id, QStringLiteral("Hinweise.txt"), QStringLiteral("en-GB"));
            text.resize(1280, 820);
            text.show();
            QTRY_COMPARE_WITH_TIMEOUT(text.model()->rowCount(), 2, 10000);
            QTRY_COMPARE_WITH_TIMEOUT(text.activeRow(), 0, 10000);
            auto *tSplit = text.findChild<QPushButton *>("splitSegment");
            auto *tJoin = text.findChild<QPushButton *>("joinSegment");
            QVERIFY(tSplit->isVisible() && tSplit->isEnabled() && tJoin->isEnabled());
            QSignalSpy tCut(&text, &EditorPage::resegmented);
            tJoin->click();
            QTRY_COMPARE_WITH_TIMEOUT(tCut.size(), 1, 10000);
            QCOMPARE(text.model()->rowCount(), 1);
            QTRY_VERIFY(text.target());
            QTest::keyClicks(text.target(), QStringLiteral("The heating is on; the key is with the caretaker."));
            QSignalSpy tSaved(&text, &EditorPage::saved);
            QTest::keySequence(text.target(), QKeySequence(Qt::CTRL | Qt::Key_Return));
            QTRY_VERIFY_WITH_TIMEOUT(tSaved.size() >= 1, 10000);
            const ApiResult got = wait(api->get(base + "/files/translated", {{"name", "Hinweise.txt"}, {"lang", "en-GB"}}));
            QCOMPARE(QString::fromUtf8(got.body), QStringLiteral("The heating is on; the key is with the caretaker.\n"));
            bool save = false;
            for (QPushButton *b : text.findChildren<QPushButton *>())
                save = save || (b->isVisible() && b->text() == QStringLiteral("Save translated document"));
            QVERIFY(save);
        }
        // A Trados file keeps its segments, and the editor says why.
        {
            EditorPage sdl(m.local(), id, QStringLiteral("Mietvertrag.docx.sdlxliff"), QStringLiteral("en-GB"));
            sdl.resize(1280, 820);
            sdl.show();
            QTRY_VERIFY_WITH_TIMEOUT(sdl.model()->rowCount() > 1, 10000);
            QTRY_VERIFY_WITH_TIMEOUT(sdl.activeRow() >= 0, 10000);
            auto *sSplit = sdl.findChild<QPushButton *>("splitSegment");
            auto *sJoin = sdl.findChild<QPushButton *>("joinSegment");
            auto *sNote = sdl.findChild<QLabel *>("cutNote");
            QVERIFY(sSplit->isVisible() && !sSplit->isEnabled() && !sJoin->isEnabled());
            QVERIFY2(sNote->isVisible() && sNote->text().contains(QStringLiteral("keeps its own record of every segment")), qPrintable(sNote->text()));
            QVERIFY(sdl.model()->segment(0).value("file").toObject().value("fixed").toBool());
            sdl.resegment(QStringLiteral("join"));
            QSignalSpy refused(&sdl, &EditorPage::resegmented);
            QTRY_COMPARE_WITH_TIMEOUT(refused.size(), 1, 10000);
            QVERIFY(!refused.at(0).at(1).toBool());
            QVERIFY2(sdl.message().contains(QStringLiteral("keeps its own record")), qPrintable(sdl.message()));
        }
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
        // Each file says how far it is in each language; that opens the editor.
        QPushButton *open = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            open = nullptr;
            for (QPushButton *b : page->findChildren<QPushButton *>(QStringLiteral("progress")))
                if (b->property("file").toString() == "Mietvertrag.txt")
                    open = b;
            return open != nullptr;
        }(), 10000);
        open->click();
        EditorPage *ed = projects.openEditor();
        QVERIFY(ed);
        QCOMPARE(ed->file(), QStringLiteral("Mietvertrag.txt"));
        QTRY_COMPARE_WITH_TIMEOUT(ed->model()->rowCount(), 4, 10000);
        QTRY_COMPARE_WITH_TIMEOUT(ed->activeRow(), 0, 10000);
        // The editor holds together in a small window and a large one: its
        // bar wraps, nothing is cut off or lies over something else.
        for (const QSize size : {QSize(1024, 700), QSize(1920, 1080), QSize(1280, 820)}) {
            projects.resize(size);
            const QString broken = layoutProblems(&projects);
            QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("%1x%2:\n%3").arg(size.width()).arg(size.height()).arg(broken)));
        }

        // The match column shows a match once it is in the segment, not
        // what the memory would offer: the third segment has a fuzzy match
        // waiting and says nothing of it.
        QVERIFY(!ed->model()->index(2, SegmentModel::State).data(Qt::ToolTipRole).toString().contains(QStringLiteral("%")));
        QVERIFY(!ed->model()->index(2, SegmentModel::State).data(Qt::DisplayRole).toString().contains(QStringLiteral("%")));

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

        // Text the segments shown must hold, in the source or the translation,
        // whatever its case. The active segment always shows.
        auto *find = ed->findChild<QLineEdit *>(QStringLiteral("findInSegments"));
        auto *foundLabel = ed->findChild<QLabel *>(QStringLiteral("foundSegments"));
        QVERIFY(find && foundLabel && foundLabel->isHidden());
        find->setText(QStringLiteral("the CONTRACT starts"));
        QCOMPARE(foundLabel->text(), QStringLiteral("1 segment"));
        QVERIFY(!foundLabel->isHidden() && ed->shownCount() >= 1 && ed->shownCount() <= 2);
        find->setText(QStringLiteral("no such words anywhere"));
        QCOMPARE(foundLabel->text(), QStringLiteral("Nothing found"));
        QCOMPARE(ed->shownCount(), 1);
        find->clear();
        QVERIFY(foundLabel->isHidden());
        QCOMPARE(ed->shownCount(), 4);

        // Find and replace in the translations: what would change is counted
        // first, and the segments that change are drafts again.
        QVERIFY(ed->findChild<QPushButton *>(QStringLiteral("replaceButton")));
        Web::Dialog *rep = ed->showReplaceDialog();
        QVERIFY(rep);
        QCOMPARE(rep->title(), QStringLiteral("Find and replace"));
        auto *repFind = rep->findChild<QLineEdit *>(QStringLiteral("replaceFind"));
        auto *repWith = rep->findChild<QLineEdit *>(QStringLiteral("replaceWith"));
        auto *repStatus = rep->findChild<QLabel *>(QStringLiteral("replaceStatus"));
        auto *repGo = rep->findChild<QPushButton *>(QStringLiteral("save"));
        QCOMPARE(repStatus->text(), QStringLiteral("Nothing to find yet."));
        QVERIFY(!repGo->isEnabled());
        repFind->setText(QStringLiteral("CONTRACT"));
        QCOMPARE(repStatus->text(), QStringLiteral("1 place in 1 segment."));
        rep->findChild<QCheckBox *>(QStringLiteral("matchCase"))->setChecked(true);
        QCOMPARE(repStatus->text(), QStringLiteral("Not found in the translations of this file."));
        QVERIFY(!repGo->isEnabled());
        rep->findChild<QCheckBox *>(QStringLiteral("matchCase"))->setChecked(false);
        repFind->setText(QStringLiteral("contrac"));
        QCOMPARE(repStatus->text(), QStringLiteral("1 place in 1 segment."));
        rep->findChild<QCheckBox *>(QStringLiteral("wholeWords"))->setChecked(true);
        QCOMPARE(repStatus->text(), QStringLiteral("Not found in the translations of this file."));
        repFind->setText(QStringLiteral("contract"));
        repWith->setText(QStringLiteral("agreement"));
        QVERIFY(repGo->isEnabled());
        QSignalSpy replacedSpy(ed, &EditorPage::replaced);
        repGo->click();
        QTRY_COMPARE_WITH_TIMEOUT(replacedSpy.size(), 1, 10000);
        QCOMPARE(replacedSpy.first().at(0).toInt(), 1);
        QCOMPARE(replacedSpy.first().at(1).toInt(), 1);
        {
            const QJsonArray after = wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array();
            int with = 0;
            for (const QJsonValue &v : after) {
                const QJsonObject o = v.toObject();
                if (SegmentContent::plainText(o.value("target").toArray()) == QStringLiteral("The agreement starts today.")) {
                    ++with;
                    QCOMPARE(o.value("status").toString(), QStringLiteral("draft"));
                }
            }
            QCOMPARE(with, 1);
        }
        // Replaced back, the text is as it was.
        ed->replaceAll(QStringLiteral("agreement"), QStringLiteral("contract"), true, true);
        QTRY_COMPARE_WITH_TIMEOUT(replacedSpy.size(), 2, 10000);

        // A translation service of one's own: asked for a suggestion for the
        // open segment, and its answer inserted.
        {
            QTcpServer service;
            QVERIFY(service.listen(QHostAddress::LocalHost));
            QByteArray seen;
            connect(&service, &QTcpServer::newConnection, &service, [&service, &seen] {
                QTcpSocket *sock = service.nextPendingConnection();
                auto *buf = new QByteArray;
                connect(sock, &QTcpSocket::readyRead, sock, [sock, buf, &seen] {
                    buf->append(sock->readAll());
                    const int head = buf->indexOf("\r\n\r\n");
                    if (head < 0)
                        return;
                    const int at = buf->toLower().indexOf("content-length:");
                    const int length = at < 0 ? 0 : buf->mid(at + 15, buf->indexOf("\r\n", at) - at - 15).trimmed().toInt();
                    if (buf->size() < head + 4 + length)
                        return;
                    seen = *buf;
                    const QJsonObject q = QJsonDocument::fromJson(buf->mid(head + 4)).object();
                    const QString id = q.value("segments").toArray().first().toObject().value("id").toString();
                    const QByteArray body = QJsonDocument(QJsonObject{{"translations", QJsonArray{QJsonObject{{"id", id}, {"text", "A suggestion in " + q.value("targetLang").toString()}}}}}).toJson(QJsonDocument::Compact);
                    sock->write("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + QByteArray::number(body.size()) + "\r\n\r\n" + body);
                    sock->disconnectFromHost();
                    delete buf;
                });
            });
            QVERIFY(ed->services().isEmpty() && !ed->findChild<QPushButton *>(QStringLiteral("suggestWith:Echo")));
            const QString url = QStringLiteral("http://127.0.0.1:%1/translate").arg(service.serverPort());
            QVERIFY(wait(api->put(QStringLiteral("/api/providers/Echo"), QJsonObject{{"url", url}, {"headerName", "X-Key"}, {"headerValue", "k1"}})).ok());
            ed->reloadServices();
            QTRY_COMPARE_WITH_TIMEOUT(ed->services(), QStringList{QStringLiteral("Echo")}, 10000);
            auto *askButton = ed->findChild<QPushButton *>(QStringLiteral("suggestWith:Echo"));
            QVERIFY(askButton);
            QCOMPARE(askButton->text(), QStringLiteral("Suggest with Echo"));
            QVERIFY(seen.isEmpty());
            QSignalSpy suggestedSpy(ed, &EditorPage::suggested);
            askButton->click();
            QTRY_COMPARE_WITH_TIMEOUT(suggestedSpy.size(), 1, 15000);
            QVERIFY(suggestedSpy.first().first().toBool());
            QVERIFY(seen.toLower().contains("x-key: k1") && seen.contains("\"targetLang\":\"en-GB\""));
            QCOMPARE(ed->findChild<QLabel *>(QStringLiteral("suggestionWords"))->text(), QStringLiteral("A suggestion in en-GB"));
            ed->findChild<QPushButton *>(QStringLiteral("insertSuggestion"))->click();
            QCOMPARE(SegmentContent::plainText(ed->target()->content()), QStringLiteral("A suggestion in en-GB"));
            // A service that is gone says so, in words.
            service.close();
            ed->askService(QStringLiteral("Echo"));
            QTRY_COMPARE_WITH_TIMEOUT(suggestedSpy.size(), 2, 15000);
            QVERIFY(!suggestedSpy.at(1).first().toBool());
            QVERIFY(ed->findChild<QLabel *>(QStringLiteral("serviceProblem"))->text().startsWith(QStringLiteral("Echo did not translate")));
            QVERIFY(wait(api->del(QStringLiteral("/api/providers/Echo"))).ok());
        }

        // Comments on the open segment: written, counted, and deleted; the
        // segment's translation and version stay as they are.
        {
            auto *words = ed->findChild<QPlainTextEdit *>(QStringLiteral("commentText"));
            auto *addButton = ed->findChild<QPushButton *>(QStringLiteral("addComment"));
            QVERIFY(words && addButton && !addButton->isEnabled());
            QVERIFY(ed->findChild<QLabel *>(QStringLiteral("noComments")));
            const QString openId = ed->activeId();
            QVERIFY(!openId.isEmpty());
            const auto stored = [&] {
                for (const QJsonValue &v : wait(api->get(base + "/segments", {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}})).array()) {
                    if (v.toObject().value("id").toString() == openId)
                        return v.toObject();
                }
                return QJsonObject();
            };
            const int versionBefore = stored().value("version").toInt();
            words->setPlainText(QStringLiteral("Is this the cold rent or the total?"));
            QVERIFY(addButton->isEnabled());
            QSignalSpy commentedSpy(ed, &EditorPage::commented);
            addButton->click();
            QTRY_COMPARE_WITH_TIMEOUT(commentedSpy.size(), 1, 10000);
            QCOMPARE(ed->comments().size(), 1);
            QCOMPARE(ed->comments().first().toObject().value("text").toString(), QStringLiteral("Is this the cold rent or the total?"));
            QVERIFY(words->toPlainText().isEmpty() && !ed->findChild<QLabel *>(QStringLiteral("noComments")));
            QCOMPARE(ed->findChild<QLabel *>(QStringLiteral("commentWords"))->text(), QStringLiteral("Is this the cold rent or the total?"));
            QCOMPARE(stored().value("comments").toArray().size(), 1);
            QCOMPARE(stored().value("version").toInt(), versionBefore);
            // One written elsewhere arrives without touching the translation.
            QVERIFY(wait(api->post(base + "/comments", QJsonObject{{"text", "The total, see clause 3."}}, {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}, {"segment", openId}})).ok());
            QTRY_COMPARE_WITH_TIMEOUT(ed->comments().size(), 2, 10000);
            const QString first = ed->comments().first().toObject().value("id").toString();
            auto *del = ed->findChild<QPushButton *>(QStringLiteral("deleteComment:%1").arg(first));
            QVERIFY(del);
            del->click();
            QTRY_COMPARE_WITH_TIMEOUT(ed->comments().size(), 1, 10000);
            QCOMPARE(ed->comments().first().toObject().value("text").toString(), QStringLiteral("The total, see clause 3."));
            ed->deleteComment(ed->comments().first().toObject().value("id").toString());
            QTRY_VERIFY_WITH_TIMEOUT(ed->comments().isEmpty() && ed->findChild<QLabel *>(QStringLiteral("noComments")), 10000);
        }

        // Scored review: issues are marked on the open segment, the file
        // gets a score from them, and the review is saved as a table.
        {
            auto *category = ed->findChild<QComboBox *>(QStringLiteral("issueCategory"));
            auto *severity = ed->findChild<QComboBox *>(QStringLiteral("issueSeverity"));
            auto *note = ed->findChild<QLineEdit *>(QStringLiteral("issueNote"));
            auto *mark = ed->findChild<QPushButton *>(QStringLiteral("addIssue"));
            auto *score = ed->findChild<QLabel *>(QStringLiteral("qualityScore"));
            auto *said = ed->findChild<QLabel *>(QStringLiteral("qualityText"));
            QVERIFY(category && severity && note && mark && score && said);
            QCOMPARE(category->count(), 7);
            QCOMPARE(severity->itemText(2), QStringLiteral("Critical (25 points)"));
            QVERIFY(ed->findChild<QLabel *>(QStringLiteral("noIssues")));
            QTRY_VERIFY_WITH_TIMEOUT(!ed->quality().isEmpty(), 10000);
            QCOMPARE(ed->quality().value("score").toDouble(), 100.0);
            const int words = ed->quality().value("words").toInt();
            QVERIFY(words > 0);
            QVERIFY2(score->text().contains(QStringLiteral(">100.0<")) && said->text().startsWith(QStringLiteral("No issues marked in %1 words.").arg(words)), qPrintable(score->text() + said->text()));
            const QString openId = ed->activeId();
            const int versionBefore = ed->model()->segment(ed->activeRow()).value("version").toInt();
            category->setCurrentIndex(category->findData(QStringLiteral("terminology")));
            severity->setCurrentIndex(severity->findData(QStringLiteral("major")));
            note->setText(QStringLiteral("Kaltmiete is basic rent in this client's glossary."));
            QSignalSpy reviewedSpy(ed, &EditorPage::reviewed);
            QSignalSpy scoredSpy(ed, &EditorPage::scored);
            mark->click();
            QTRY_COMPARE_WITH_TIMEOUT(reviewedSpy.size(), 1, 10000);
            QCOMPARE(ed->reviewIssues().size(), 1);
            QVERIFY(note->text().isEmpty() && !ed->findChild<QLabel *>(QStringLiteral("noIssues")));
            QCOMPARE(ed->findChild<QLabel *>(QStringLiteral("issueWords"))->text(), QStringLiteral("<b>Terminology</b>, major (5 points): Kaltmiete is basic rent in this client's glossary."));
            QCOMPARE(ed->model()->segment(ed->activeRow()).value("version").toInt(), versionBefore);
            // One marked elsewhere arrives, and the score counts both.
            QVERIFY(wait(api->post(base + "/issues", QJsonObject{{"category", "style"}, {"severity", "minor"}}, {{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}, {"segment", openId}})).ok());
            QTRY_COMPARE_WITH_TIMEOUT(ed->reviewIssues().size(), 2, 10000);
            QTRY_VERIFY2_WITH_TIMEOUT(ed->quality().value("penalty").toInt() == 6, QJsonDocument(ed->quality()).toJson(QJsonDocument::Compact).constData(), 10000);
            const double want = qMax(0.0, qRound((100.0 - 600.0 / words) * 10) / 10.0);
            QCOMPARE(ed->quality().value("score").toDouble(), want);
            QVERIFY2(said->text().startsWith(QStringLiteral("2 issues (1 minor, 1 major), 6 penalty points in %1 words.").arg(words)), qPrintable(said->text()));
            QVERIFY(score->text().contains(QStringLiteral(">%1<").arg(QString::number(want, 'f', 1))));
            if (const QString shots = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !shots.isEmpty()) {
                // The panel with the review in view.
                if (auto *area = said->window()->findChild<QScrollArea *>(); area)
                    area->ensureWidgetVisible(said);
                QTest::qWait(300);
                ed->window()->grab().save(shots + QStringLiteral("/editor-review.png"));
            }
            // The review as a table.
            QTemporaryDir dir;
            const QString to = dir.filePath(QStringLiteral("review.csv"));
            QSignalSpy savedSpy(ed, &EditorPage::reviewSaved);
            ed->saveReviewTo(to);
            QTRY_COMPARE_WITH_TIMEOUT(savedSpy.size(), 1, 10000);
            QVERIFY(savedSpy.at(0).at(0).toBool());
            QFile table(to);
            QVERIFY(table.open(QIODevice::ReadOnly));
            const QString csv = QString::fromUtf8(table.readAll());
            QVERIFY2(csv.count(QLatin1Char('\n')) == 3 && csv.contains(QStringLiteral(",terminology,major,5,Kaltmiete is basic rent in this client's glossary.,")) && csv.contains(QStringLiteral(",style,minor,1,")), qPrintable(csv));
            // Deleted, the file scores 100 again.
            const QString first = ed->reviewIssues().first().toObject().value("id").toString();
            auto *del = ed->findChild<QPushButton *>(QStringLiteral("deleteIssue:%1").arg(first));
            QVERIFY(del);
            del->click();
            QTRY_COMPARE_WITH_TIMEOUT(ed->reviewIssues().size(), 1, 10000);
            ed->deleteIssue(ed->reviewIssues().first().toObject().value("id").toString());
            QTRY_VERIFY_WITH_TIMEOUT(ed->reviewIssues().isEmpty() && ed->findChild<QLabel *>(QStringLiteral("noIssues")), 10000);
            QTRY_VERIFY_WITH_TIMEOUT(ed->quality().value("score").toDouble() == 100.0, 10000);
        }

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

        // The Memories and Analysis dialogs, open over the editor while
        // the layout check looks at them.
        button(ed, "Memories")->click();
        Web::Dialog *dlg = nullptr;
        QTRY_VERIFY((dlg = ed->window()->findChild<Web::Dialog *>(QStringLiteral("memoriesDialog"))) && dlg->isVisible());
        QCOMPARE(dlg->findChild<QTableWidget *>(QStringLiteral("projectMemories"))->rowCount(), 1);
        QTRY_VERIFY(!labelWith(dlg, QStringLiteral("…")));
        QTest::qWait(600);
        dlg->accept();
        QTRY_VERIFY(!ed->window()->findChild<Web::Dialog *>(QStringLiteral("memoriesDialog")));
        QSignalSpy analysed(ed, &EditorPage::analysisShown);
        button(ed, "Analysis")->click();
        QTRY_VERIFY_WITH_TIMEOUT(!analysed.isEmpty(), 10000);
        dlg = ed->window()->findChild<Web::Dialog *>(QStringLiteral("analysisDialog"));
        QVERIFY(dlg && dlg->findChild<QTableWidget *>()->rowCount() > 1);
        QTest::qWait(600);
        dlg->accept();
        QTRY_VERIFY(!ed->window()->findChild<Web::Dialog *>(QStringLiteral("analysisDialog")));
        projects.activateWindow();
        QVERIFY(QTest::qWaitForWindowActive(&projects));

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

        // A grammar service of one's own: named in the project, it reads the
        // translation that is worked on, and what it finds is listed with
        // the checks, with its replacements.
        {
            QTcpServer checker;
            QVERIFY(checker.listen(QHostAddress::LocalHost));
            QStringList asked;
            connect(&checker, &QTcpServer::newConnection, &checker, [&checker, &asked] {
                QTcpSocket *sock = checker.nextPendingConnection();
                auto *buf = new QByteArray;
                connect(sock, &QTcpSocket::readyRead, sock, [sock, buf, &asked] {
                    buf->append(sock->readAll());
                    const int head = buf->indexOf("\r\n\r\n");
                    if (head < 0)
                        return;
                    const int at = buf->toLower().indexOf("content-length:");
                    const int length = at < 0 ? 0 : buf->mid(at + 15, buf->indexOf("\r\n", at) - at - 15).trimmed().toInt();
                    if (buf->size() < head + 4 + length)
                        return;
                    const QUrlQuery q(QString::fromUtf8(buf->mid(head + 4)).replace(QLatin1Char('+'), QLatin1String("%20")));
                    const QString lang = q.queryItemValue("language", QUrl::FullyDecoded);
                    const QString said = q.queryItemValue("text", QUrl::FullyDecoded);
                    asked << lang + "|" + q.queryItemValue("disabledCategories") + "|" + said;
                    QByteArray status = "200 OK";
                    QByteArray body;
                    if (lang != "en") {
                        // It knows English, without regions.
                        status = "400 Bad Request";
                        body = "Error: not a language code known";
                    } else {
                        QJsonArray matches;
                        if (const int i = int(said.indexOf("a apple")); i >= 0)
                            matches.append(QJsonObject{{"message", "Use “an” before a vowel sound."}, {"offset", i}, {"length", 1},
                                                       {"replacements", QJsonArray{QJsonObject{{"value", "an"}}}},
                                                       {"rule", QJsonObject{{"id", "A_AN"}, {"category", QJsonObject{{"id", "MISC"}, {"name", "Misc"}}}}}});
                        if (const int i = int(said.indexOf("teh end")); i >= 0)
                            matches.append(QJsonObject{{"message", "These words are in the wrong form."}, {"offset", i}, {"length", 7},
                                                       {"replacements", QJsonArray{QJsonObject{{"value", "the end"}}}},
                                                       {"rule", QJsonObject{{"id", "FORM"}, {"category", QJsonObject{{"id", "GRAMMAR"}, {"name", "Grammar"}}}}}});
                        if (const int i = int(said.indexOf("apple")); i >= 0)
                            matches.append(QJsonObject{{"message", "Possible spelling mistake."}, {"offset", i}, {"length", 5},
                                                       {"rule", QJsonObject{{"id", "SPELL"}, {"issueType", "misspelling"}, {"category", QJsonObject{{"id", "TYPOS"}}}}}});
                        body = QJsonDocument(QJsonObject{{"matches", matches}}).toJson(QJsonDocument::Compact);
                    }
                    sock->write("HTTP/1.1 " + status + "\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + QByteArray::number(body.size()) + "\r\n\r\n" + body);
                    sock->disconnectFromHost();
                    delete buf;
                });
            });
            const QString url = QStringLiteral("http://127.0.0.1:%1/v2/check").arg(checker.serverPort());
            QVERIFY(wait(api->put(QStringLiteral("/api/providers/Checker"), QJsonObject{{"kind", "check"}, {"url", url}})).ok());
            // It is no translation service: the editor does not offer it for suggestions.
            ed->reloadServices();
            QTest::qWait(500);
            QVERIFY(!ed->services().contains(QStringLiteral("Checker")));

            const QString first = segs.at(0).toObject().value("id").toString();
            ed->goToSegment(first);
            QTRY_COMPARE_WITH_TIMEOUT(ed->activeId(), first, 5000);
            QSignalSpy read(ed, &EditorPage::grammarChecked);
            const auto type = [ed](const QJsonArray &content) {
                ed->target()->setContent(content);
                emit ed->target()->edited();
            };
            // A project that names no service sends nothing.
            type(text("I ate a apple."));
            QTRY_VERIFY_WITH_TIMEOUT(read.size() > 0, 10000);
            QVERIFY(asked.isEmpty() && ed->grammarIssues().isEmpty());

            QJsonObject project = wait(api->get(base)).object();
            QJsonObject settings = project.value("settings").toObject();
            QJsonObject qa = settings.value("qa").toObject();
            qa.insert("grammar", "Checker");
            settings.insert("qa", qa);
            QVERIFY(wait(api->put(base + "/settings", settings, project.value("version").toInt())).ok());
            QTRY_COMPARE_WITH_TIMEOUT(ed->grammarService(), QStringLiteral("Checker"), 10000);

            type(text("I ate a apple!"));
            QTRY_COMPARE_WITH_TIMEOUT(ed->grammarIssues().size(), 1, 10000);
            // Asked for the target language, then for the language alone; spelling is not asked for.
            QCOMPARE(asked, (QStringList{QStringLiteral("en-GB|TYPOS|I ate a apple!"), QStringLiteral("en|TYPOS|I ate a apple!")}));
            const QJsonObject found = ed->grammarIssues().first().toObject();
            QCOMPARE(found.value("message").toString(), QStringLiteral("Use “an” before a vowel sound. (“a”)"));
            QCOMPARE(found.value("offset").toInt(), 6);
            auto *put = ed->findChild<QPushButton *>(QStringLiteral("replaceFound"));
            QVERIFY(put);
            QCOMPARE(put->text(), QStringLiteral("Replace with “an”"));
            QCOMPARE(put->accessibleName(), QStringLiteral("Replace a with an"));
            if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
                QTest::qWait(300);
                projects.grab().save(dir + QStringLiteral("/grammar-desktop.png"));
            }
            int reads = int(read.size());
            put->click();
            QCOMPARE(SegmentContent::plainText(ed->target()->content()), QStringLiteral("I ate an apple!"));
            // Read again, the finding is gone.
            QTRY_VERIFY_WITH_TIMEOUT(read.size() > reads, 10000);
            QVERIFY(ed->grammarIssues().isEmpty() && !ed->findChild<QPushButton *>(QStringLiteral("replaceFound")));

            // A finding with a tag in it is listed, and left to the translator to change.
            QJsonArray tagged{QJsonObject{{"text", "teh "}}};
            for (const QJsonValue &run : withTags) {
                if (run.toObject().contains("tag")) {
                    tagged.append(run);
                    if (tagged.size() == 2)
                        tagged.append(QJsonObject{{"text", "end"}});
                }
            }
            type(tagged);
            QTRY_COMPARE_WITH_TIMEOUT(ed->grammarIssues().size(), 1, 10000);
            QVERIFY(!ed->findChild<QPushButton *>(QStringLiteral("replaceFound")));
            QVERIFY(!ed->replaceFound(0, 7, QStringLiteral("the end")));
            QCOMPARE(SegmentContent::tags(ed->target()->content()).size(), 2);

            // Confirmed before the service has read it: it is read first,
            // and what it finds is kept with the segment.
            QJsonArray both{QJsonObject{{"text", "We ate a apple"}}};
            for (const QJsonValue &run : withTags) {
                if (run.toObject().contains("tag"))
                    both.append(run);
            }
            QSignalSpy saved3(ed, &EditorPage::saved);
            type(both);
            ed->confirm();
            QVERIFY(saved3.wait(10000));
            QVERIFY(asked.contains(QStringLiteral("en|TYPOS|We ate a apple")));
            bool kept = false;
            for (const QJsonValue &v : wait(api->get(base + "/segments", {{"file", "Hinweis.xlf"}, {"lang", "en-GB"}})).array()) {
                if (v.toObject().value("id").toString() != first)
                    continue;
                QCOMPARE(v.toObject().value("status").toString(), QStringLiteral("confirmed"));
                for (const QJsonValue &w : v.toObject().value("warnings").toArray())
                    kept = kept || (w.toObject().value("kind").toString() == "grammar" && w.toObject().value("text").toString().startsWith(QStringLiteral("Use “an”")));
            }
            QVERIFY(kept);

            // A service that is gone says so, and the other checks go on.
            checker.close();
            ed->goToSegment(first);
            QTRY_COMPARE_WITH_TIMEOUT(ed->activeId(), first, 5000);
            type(text("They ate a apple."));
            QTRY_VERIFY_WITH_TIMEOUT(ed->grammarError().startsWith(QStringLiteral("Checker did not check")), 15000);
            QTRY_VERIFY_WITH_TIMEOUT(ed->findChild<QLabel *>(QStringLiteral("grammarError")), 5000);
        }
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEditor)
#include "tst_editor.moc"
