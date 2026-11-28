#include "testcore.h"

#include <QHostAddress>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQuickWindow>
#include <QSettings>
#include <QStandardPaths>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTest>
#include <QUrlQuery>
#include <QDir>
#include <QFile>

static QJsonArray text(const QString &s)
{
    return QJsonArray{QJsonObject{{"text", s}}};
}

static QString plain(const QVariant &content)
{
    QString out;
    for (const QVariant &run : content.toList())
        out += run.toMap().value(QStringLiteral("text")).toString();
    return out;
}

// The editor, worked with as a translator does: matches and terms, typing
// and confirming, repetitions, finding and replacing, a translation
// service, comments, scored review, someone else saving the same segment,
// adding a term, tags, splitting and joining, and documents of every
// format.
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

    void translateAFile()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        ApiClient *api = app.api();
        // A memory with two units, and a project using it.
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "M"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories/M/units"), QJsonObject{{"source", text("Der Vertrag beginnt heute.")}, {"target", text("The contract starts today.")}})).ok());
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories/M/units"), QJsonObject{{"source", text("Die Kaution war fällig.")}, {"target", text("The deposit was due.")}})).ok());
        QJsonObject settings = waitFor(api->get(QStringLiteral("/api/projects/defaults"))).object();
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
        const ApiResult made = waitFor(api->post(QStringLiteral("/api/projects"),
            QJsonObject{{"settings", settings},
                        {"files", QJsonArray{QJsonObject{{"name", "Mietvertrag.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(txt.toBase64())}},
                                             QJsonObject{{"name", "Hinweis.xlf"}, {"role", "translatable"}, {"data", QString::fromLatin1(xlf.toBase64())}}}},
                        {"termbase", QJsonObject{{"name", "Mietvertrag terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        const QString base = QStringLiteral("/api/projects/%1").arg(id);
        QVERIFY(waitFor(api->post(base + "/terms", QJsonObject{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Vertrag"}}}}},
                                                                                         QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", "contract"}}}}}}}})).ok());
        const QString here = app.connections.local()->id();
        const auto segmentsOnServer = [&](const QString &file) {
            return waitFor(api->get(base + "/segments", QUrlQuery{{"file", file}, {"lang", "en-GB"}})).array();
        };

        // The project page lists the file; it opens the editor.
        app.call(app.window, "openProject", {here, id});
        QTRY_VERIFY_WITH_TIMEOUT(app.get(QStringLiteral("projectPage"), "progress").toList().size() >= 2, 10000);
        app.call(app.window, "openEditor", {here, id, QStringLiteral("Mietvertrag.txt"), QStringLiteral("en-GB")});
        QObject *ed = app.find(QStringLiteral("editorPage"));
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeRow").toInt(), 0, 10000);
        QCOMPARE(ed->property("segments").toList().size(), 4);
        for (const QSize size : {QSize(1024, 700), QSize(1366, 860), QSize(1920, 1080)}) {
            app.window->resize(size);
            const QString broken = layoutProblems(app.window);
            QVERIFY2(broken.isEmpty(), qPrintable(QStringLiteral("editor at %1x%2:\n%3").arg(size.width()).arg(size.height()).arg(broken)));
        }

        // The memory's match and the term are offered for the first segment;
        // the best match is already in the empty segment.
        QTRY_VERIFY_WITH_TIMEOUT(ed->property("results").toList().size() >= 2, 10000);
        const QVariantList results = ed->property("results").toList();
        QCOMPARE(results.first().toMap().value("score").toInt(), 100);
        bool term = false;
        for (const QVariant &r : results)
            term = term || (r.toMap().value("kind") == QStringLiteral("term") && r.toMap().value("targets").toList().value(0).toMap().value("text") == QStringLiteral("contract"));
        QVERIFY(term);
        QObject *te = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((te = app.find(QStringLiteral("targetEdit"))) && te->property("plainText").toString() == QStringLiteral("The contract starts today."), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("100% match inserted"), 5000);
        app.call(ed, "insertResult", {1});
        QCOMPARE(app.find(QStringLiteral("targetEdit"))->property("plainText").toString(), QStringLiteral("The contract starts today."));

        // Confirmed: on to the next segment, and on the server with where its
        // translation came from.
        app.call(ed, "confirm");
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeRow").toInt(), 1, 10000);
        QTRY_COMPARE_WITH_TIMEOUT(segmentsOnServer("Mietvertrag.txt").at(0).toObject().value("status").toString(), QStringLiteral("confirmed"), 10000);
        QCOMPARE(segmentsOnServer("Mietvertrag.txt").at(0).toObject().value("origin").toObject().value("kind").toString(), QStringLiteral("tm"));

        // Typed and confirmed: its repetition is filled, and the next open one comes.
        QTRY_VERIFY_WITH_TIMEOUT(te = app.find(QStringLiteral("targetEdit")), 5000);
        QVERIFY(app.type(te->property("textEdit").value<QObject *>(), QStringLiteral("The rent is due.")));
        QCOMPARE(te->property("plainText").toString(), QStringLiteral("The rent is due."));
        app.call(ed, "confirm");
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeRow").toInt(), 2, 10000);
        QTRY_COMPARE_WITH_TIMEOUT(segmentsOnServer("Mietvertrag.txt").at(3).toObject().value("status").toString(), QStringLiteral("confirmed"), 10000);
        QCOMPARE(plain(segmentsOnServer("Mietvertrag.txt").at(3).toObject().value("target").toVariant()), QStringLiteral("The rent is due."));
        QTRY_VERIFY_WITH_TIMEOUT(app.notices().contains(QStringLiteral("Also filled 1 repetition (segment 4)")), 5000);
        // The next one has a fuzzy match waiting, put in as it was empty.
        QTRY_VERIFY_WITH_TIMEOUT(app.notices().contains(QStringLiteral("77% match inserted")), 5000);
        // The progress counts the words confirmed.
        QTRY_COMPARE(ed->property("progress").toMap().value("done").toInt(), 12);

        // Memory search finds the memory's unit, in either language.
        app.call(ed, "openLookup", {QStringLiteral("Kaution")});
        QTRY_VERIFY_WITH_TIMEOUT(ed->property("lookupHits").toList().size() == 1, 10000);
        QCOMPARE(plain(ed->property("lookupHits").toList().first().toMap().value("unit").toMap().value("target")), QStringLiteral("The deposit was due."));

        // The filter: the open one always shows.
        ed->setProperty("show", QStringLiteral("todo"));
        QTRY_COMPARE(ed->property("shownRows").toList().size(), 1);
        ed->setProperty("show", QStringLiteral("all"));
        QTRY_COMPARE(ed->property("shownRows").toList().size(), 4);
        // Text the segments shown hold, in either language, whatever its case.
        ed->setProperty("find", QStringLiteral("RENT"));
        QTRY_COMPARE(ed->property("found").toInt(), 2);
        ed->setProperty("find", QString());

        // Find and replace: counted first; what changes is a draft again.
        const QVariantMap counted = app.call(ed, "replaceCount", {QStringLiteral("rent"), QStringLiteral("lease"), false, true}).toMap();
        QCOMPARE(counted.value("places").toInt(), 2);
        QCOMPARE(counted.value("segments").toInt(), 2);
        app.call(ed, "replaceAll", {QStringLiteral("rent"), QStringLiteral("lease"), false, true});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Replaced 2 places in 2 segments"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(segmentsOnServer("Mietvertrag.txt").at(1).toObject().value("status").toString(), QStringLiteral("draft"), 10000);
        QCOMPARE(plain(segmentsOnServer("Mietvertrag.txt").at(1).toObject().value("target").toVariant()), QStringLiteral("The lease is due."));
        app.call(ed, "replaceAll", {QStringLiteral("lease"), QStringLiteral("rent"), false, true});
        QTRY_COMPARE_WITH_TIMEOUT(plain(segmentsOnServer("Mietvertrag.txt").at(3).toObject().value("target").toVariant()), QStringLiteral("The rent is due."), 10000);

        // A translation service of one's own: asked for the open segment,
        // and its answer inserted; one that is gone says so.
        {
            QTcpServer service;
            QVERIFY(service.listen(QHostAddress::LocalHost));
            QByteArray seen;
            connect(&service, &QTcpServer::newConnection, &service, [&service, &seen] {
                QTcpSocket *sock = service.nextPendingConnection();
                auto *buf = new QByteArray;
                connect(sock, &QTcpSocket::readyRead, sock, [sock, buf, &seen] {
                    buf->append(sock->readAll());
                    const int head = int(buf->indexOf("\r\n\r\n"));
                    if (head < 0)
                        return;
                    const int at = int(buf->toLower().indexOf("content-length:"));
                    const int length = at < 0 ? 0 : buf->mid(at + 15, buf->indexOf("\r\n", at) - at - 15).trimmed().toInt();
                    if (buf->size() < head + 4 + length)
                        return;
                    seen = *buf;
                    const QJsonObject q = QJsonDocument::fromJson(buf->mid(head + 4)).object();
                    const QString sid = q.value("segments").toArray().first().toObject().value("id").toString();
                    const QByteArray body = QJsonDocument(QJsonObject{{"translations", QJsonArray{QJsonObject{{"id", sid}, {"text", "A suggestion in " + q.value("targetLang").toString()}}}}}).toJson(QJsonDocument::Compact);
                    sock->write("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + QByteArray::number(body.size()) + "\r\n\r\n" + body);
                    sock->disconnectFromHost();
                    delete buf;
                });
            });
            const QString url = QStringLiteral("http://127.0.0.1:%1/translate").arg(service.serverPort());
            QVERIFY(waitFor(api->put(QStringLiteral("/api/providers/Echo"), QJsonObject{{"url", url}, {"headerName", "X-Key"}, {"headerValue", "k1"}})).ok());
            app.call(ed, "loadServices");
            QTRY_COMPARE_WITH_TIMEOUT(ed->property("services").toStringList(), QStringList{QStringLiteral("Echo")}, 10000);
            app.call(ed, "askService", {QStringLiteral("Echo")});
            QTRY_VERIFY_WITH_TIMEOUT(!ed->property("suggestion").isNull() && ed->property("suggestion").toMap().contains("target"), 15000);
            QVERIFY(seen.toLower().contains("x-key: k1") && seen.contains("\"targetLang\":\"en-GB\""));
            app.call(ed, "insertSuggestion");
            QCOMPARE(app.find(QStringLiteral("targetEdit"))->property("plainText").toString(), QStringLiteral("A suggestion in en-GB"));
            service.close();
            app.call(ed, "askService", {QStringLiteral("Echo")});
            QTRY_VERIFY_WITH_TIMEOUT(ed->property("serviceProblem").toString().startsWith(QStringLiteral("Echo did not translate")) || !ed->property("serviceProblem").toString().isEmpty(), 15000);
            QVERIFY(waitFor(api->del(QStringLiteral("/api/providers/Echo"))).ok());
        }

        // Comments on the open segment: written and deleted; its translation stays.
        const QString open = ed->property("activeId").toString();
        app.call(ed, "addComment", {QStringLiteral("Is this the deposit or the rent?")});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeSegment").toMap().value("comments").toList().size(), 1, 10000);
        const QString commentId = ed->property("activeSegment").toMap().value("comments").toList().first().toMap().value("id").toString();
        app.call(ed, "deleteComment", {commentId});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeSegment").toMap().value("comments").toList().size(), 0, 10000);

        // Scored review: an issue lowers the file's score; deleted, it is 100 again.
        app.call(ed, "addIssue", {QStringLiteral("accuracy"), QStringLiteral("major"), QStringLiteral("Wrong word")});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("activeSegment").toMap().value("issues").toList().size(), 1, 10000);
        QTRY_VERIFY_WITH_TIMEOUT(ed->property("quality").toMap().value("score").toDouble() < 100, 10000);
        const QString issueId = ed->property("activeSegment").toMap().value("issues").toList().first().toMap().value("id").toString();
        app.call(ed, "deleteIssue", {issueId});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("quality").toMap().value("score").toDouble(), 100.0, 10000);

        // Someone else saves the segment being worked on: both are shown,
        // and theirs can be taken.
        QVERIFY(app.type(app.find(QStringLiteral("targetEdit"))->property("textEdit").value<QObject *>(), QStringLiteral("Mine")));
        const QJsonObject onServer = segmentsOnServer("Mietvertrag.txt").at(ed->property("activeRow").toInt()).toObject();
        QVERIFY(waitFor(api->put(base + "/segments", QJsonObject{{"target", text("Theirs")}, {"status", "draft"}}, onServer.value("version").toInt(),
                                 QUrlQuery{{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}, {"segment", open}})).ok());
        QTRY_VERIFY_WITH_TIMEOUT(ed->property("conflicts").toMap().contains(open), 15000);
        app.call(ed, "takeTheirs");
        QCOMPARE(app.find(QStringLiteral("targetEdit"))->property("plainText").toString(), QStringLiteral("Theirs"));

        // A term added from the panel goes into the project's termbase.
        ed->setProperty("sourceSelection", QStringLiteral("Kaution"));
        app.call(ed, "openCapture");
        app.call(ed, "addTerm", {QStringLiteral("Kaution"), QStringLiteral("deposit"), QString()});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added “Kaution” to Mietvertrag terms"), 10000);

        // The Memories and Analysis dialogs, over the editor.
        app.call(QStringLiteral("memoriesDialog"), "open");
        QTRY_VERIFY(app.get(QStringLiteral("memoriesDialog"), "opened").toBool());
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.call(QStringLiteral("memoriesDialog"), "close");
        app.call(QStringLiteral("analysisDialog"), "start");
        QTRY_VERIFY_WITH_TIMEOUT(!app.get(QStringLiteral("analysisDialog"), "report").isNull(), 10000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.call(QStringLiteral("analysisDialog"), "close");

        // Tags: the source's go in with F9 and are saved as tags.
        app.call(app.window, "openEditor", {here, id, QStringLiteral("Hinweis.xlf"), QStringLiteral("en-GB")});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("file").toString(), QStringLiteral("Hinweis.xlf"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(app.find(QStringLiteral("targetEdit")) && ed->property("activeRow").toInt() == 0, 10000);
        QObject *target = app.find(QStringLiteral("targetEdit"));
        target->property("textEdit").value<QObject *>()->setProperty("cursorPosition", 0);
        app.call(target, "setContent", {QVariantList{}});
        QVERIFY(app.type(target->property("textEdit").value<QObject *>(), QStringLiteral("Please pay "), false));
        app.call(ed, "insertNextTag");
        QVERIFY(app.type(target->property("textEdit").value<QObject *>(), QStringLiteral("now"), false));
        app.call(ed, "insertNextTag");
        QVERIFY(app.type(target->property("textEdit").value<QObject *>(), QStringLiteral("."), false));
        app.call(ed, "confirm");
        QTRY_COMPARE_WITH_TIMEOUT(segmentsOnServer("Hinweis.xlf").at(0).toObject().value("status").toString(), QStringLiteral("confirmed"), 10000);
        int tags = 0;
        for (const QJsonValue &run : segmentsOnServer("Hinweis.xlf").at(0).toObject().value("target").toArray())
            tags += run.toObject().contains("tag") ? 1 : 0;
        QCOMPARE(tags, 2);
        app.shot(QStringLiteral("editor-tags"));
    }

    void completions()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Words");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        const ApiResult made = waitFor(app.api()->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings},
            {"files", QJsonArray{QJsonObject{{"name", "a.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(QByteArray("Der Vertrag gilt.").toBase64())}}}}}));
        QVERIFY(made.ok());
        app.call(app.window, "openEditor", {app.connections.local()->id(), made.object().value("id").toInteger(), QString(), QString()});
        QObject *ed = app.find(QStringLiteral("editorPage"));
        QObject *te = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((te = app.find(QStringLiteral("targetEdit"))), 10000);
        QObject *edit = te->property("textEdit").value<QObject *>();
        // The editor makes its own list once the segment's matches and
        // terms are in; the words here come after that, not before it.
        QTRY_VERIFY_WITH_TIMEOUT(!ed->property("activeId").toString().isEmpty(), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("looking").toInt(), 0, 10000);
        ed->setProperty("completions", QStringList{QStringLiteral("contract"), QStringLiteral("contractor"), QStringLiteral("continue")});
        QVERIFY(app.type(edit, QStringLiteral("The con")));
        // The shortest first, then as the alphabet has them.
        QTRY_COMPARE(te->property("offered").toStringList(), (QStringList{QStringLiteral("continue"), QStringLiteral("contract"), QStringLiteral("contractor")}));
        // Escape closes the list and nothing else.
        QTest::keyClick(app.window, Qt::Key_Escape);
        QCOMPARE(te->property("offered").toStringList(), QStringList());
        QCOMPARE(te->property("plainText").toString(), QStringLiteral("The con"));
        // Typing on offers again; the arrows choose and go round; Tab takes one.
        QVERIFY(app.type(edit, QStringLiteral("tr"), false));
        QTRY_COMPARE(te->property("offered").toStringList(), (QStringList{QStringLiteral("contract"), QStringLiteral("contractor")}));
        QTest::keyClick(app.window, Qt::Key_Down);
        QTest::keyClick(app.window, Qt::Key_Tab);
        QCOMPARE(te->property("plainText").toString(), QStringLiteral("The contractor"));
        QVERIFY(edit->property("activeFocus").toBool());
        // In the middle of a word nothing is offered.
        edit->setProperty("cursorPosition", 6);
        QVERIFY(app.type(edit, QStringLiteral("x"), false));
        QCOMPARE(te->property("offered").toStringList(), QStringList());
    }

    void splitAndJoin()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Cuts");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        const QByteArray html = "<html><body><h1>Mietvertrag</h1><p>Der Mieter zahlt. Der Vermieter quittiert.</p></body></html>";
        const ApiResult made = waitFor(app.api()->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings},
            {"files", QJsonArray{QJsonObject{{"name", "v.html"}, {"role", "translatable"}, {"data", QString::fromLatin1(html.toBase64())}}}}}));
        QVERIFY(made.ok());
        app.call(app.window, "openEditor", {app.connections.local()->id(), made.object().value("id").toInteger(), QString(), QString()});
        QObject *ed = app.find(QStringLiteral("editorPage"));
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("segments").toList().size(), 3, 10000);
        QTRY_COMPARE(ed->property("activeRow").toInt(), 0);
        // A heading is joined with the paragraph after it too: a
        // translation may divide a text into paragraphs differently.
        QVERIFY(ed->property("cut").toMap().value("shown").toBool());
        QVERIFY(ed->property("cut").toMap().value("join").toBool());
        // Two sentences of a paragraph are joined; what was typed is joined with them.
        app.call(ed, "move", {1});
        QTRY_COMPARE(ed->property("activeRow").toInt(), 1);
        QTRY_VERIFY(ed->property("cut").toMap().value("join").toBool());
        QVERIFY(app.type(app.find(QStringLiteral("targetEdit"))->property("textEdit").value<QObject *>(), QStringLiteral("The tenant pays.")));
        app.call(ed, "resegment", {QStringLiteral("join"), QVariant(), QVariant()});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("segments").toList().size(), 2, 10000);
        QCOMPARE(app.notice(), QStringLiteral("Joined with the next segment"));
        QCOMPARE(plain(ed->property("segments").toList().at(1).toMap().value("source")), QStringLiteral("Der Mieter zahlt. Der Vermieter quittiert."));
        QVERIFY(ed->property("cut").toMap().value("note").toString().startsWith(QStringLiteral("Joined from 2 segments")));
        // Split through the dialog, where the caret is.
        app.call(ed, "openSplit");
        QTRY_VERIFY(app.get(QStringLiteral("splitDialog"), "opened").toBool());
        app.call(QStringLiteral("splitDialog"), "placeAt", {18});
        QTRY_VERIFY(app.get(QStringLiteral("splitDialog"), "ok").toBool());
        // The place it is split at is seen: a line where the cursor is.
        {
            QObject *caret = app.find(QStringLiteral("splitCaret"));
            QVERIFY(caret);
            QTRY_VERIFY(caret->property("visible").toBool());
            QObject *source = app.find(QStringLiteral("splitSource"));
            QCOMPARE(caret->property("x").toReal(), source->property("cursorRectangle").toRectF().x());
            QVERIFY(caret->property("height").toReal() > 5);
        }
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.call(QStringLiteral("splitDialog"), "split");
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("segments").toList().size(), 3, 10000);
        QCOMPARE(app.notice(), QStringLiteral("Split in two segments"));
        QCOMPARE(plain(ed->property("segments").toList().at(2).toMap().value("source")), QStringLiteral("Der Vermieter quittiert."));
        QVERIFY(!app.get(QStringLiteral("splitDialog"), "opened").toBool());
    }

    void documentFormats()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        // One small document of every format.
        const QDir dir(QStringLiteral(INTERLINEA_FIXTURES "/documents"));
        QJsonArray files;
        for (const QString &name : dir.entryList(QDir::Files)) {
            // A package is a project of its own (below).
            if (name.endsWith(QStringLiteral(".mqout")))
                continue;
            QFile f(dir.filePath(name));
            QVERIFY(f.open(QIODevice::ReadOnly));
            files.append(QJsonObject{{"name", name}, {"role", "translatable"}, {"data", QString::fromLatin1(f.readAll().toBase64())}});
        }
        QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Formats");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        const ApiResult made = waitFor(app.api()->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings}, {"files", files}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        const qint64 id = made.object().value("id").toInteger();
        QObject *ed = app.find(QStringLiteral("editorPage"));
        for (const QJsonValue &v : files) {
            const QString name = v.toObject().value("name").toString();
            app.call(app.window, "openEditor", {app.connections.local()->id(), id, name, QStringLiteral("en-GB")});
            QTRY_VERIFY2_WITH_TIMEOUT(ed->property("file").toString() == name && !ed->property("segments").toList().isEmpty(), qPrintable(name), 15000);
            // The preview of a document shows its text.
            if (app.call(ed, "isDocument", {name}).toBool() && !ed->property("table").toBool()) {
                app.call(ed, "showPreview", {true});
                QTRY_VERIFY2_WITH_TIMEOUT(!ed->property("layoutFile").toString().isEmpty(), qPrintable(name), 10000);
                app.call(ed, "showPreview", {false});
            }
            const QString broken = layoutProblems(app.window);
            QVERIFY2(broken.isEmpty(), qPrintable(name + QStringLiteral(":\n") + broken));
            // The translated file comes back.
            const bool package = ed->property("fileInfo").toMap().value("package").toBool();
            const ApiResult back = waitFor(package ? app.api()->get(QStringLiteral("/api/projects/%1/return-package").arg(id), QUrlQuery{{"lang", "en-GB"}})
                                                   : app.api()->get(QStringLiteral("/api/projects/%1/files/translated").arg(id), QUrlQuery{{"name", name}, {"lang", "en-GB"}}));
            QVERIFY2(back.ok() && !back.body.isEmpty(), qPrintable(name + QStringLiteral(": ") + back.error.message));
        }
        // A memoQ handoff package: its document is a bilingual file of the
        // project, saved back as the package's return.
        QFile handoffFile(dir.filePath(QStringLiteral("Hofmann.mqout")));
        QVERIFY(handoffFile.open(QIODevice::ReadOnly));
        const QByteArray handoff = handoffFile.readAll();
        QJsonObject s2 = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        s2.insert("name", "Hofmann memoQ");
        s2.insert("sourceLang", "de-de");
        s2.insert("targetLangs", QJsonArray{"en-gb"});
        const ApiResult made2 = waitFor(app.api()->post(QStringLiteral("/api/projects"),
            QJsonObject{{"settings", s2}, {"files", QJsonArray{QJsonObject{{"name", "Hofmann.mqout"}, {"role", "package"}, {"data", QString::fromLatin1(handoff.toBase64())}}}}}));
        QVERIFY2(made2.ok(), qPrintable(made2.error.message));
        const QString member = made2.object().value("settings").toObject().value("files").toArray().at(1).toObject().value("name").toString();
        QCOMPARE(member, QStringLiteral("Documents/Mietvertrag.docx.mqxliff"));
        app.call(app.window, "openEditor", {app.connections.local()->id(), made2.object().value("id").toInteger(), member, QStringLiteral("en-gb")});
        QTRY_COMPARE_WITH_TIMEOUT(ed->property("segments").toList().size(), 3, 10000);
        QTRY_COMPARE(ed->property("saveLabel").toString(), QStringLiteral("Save return package"));
        const ApiResult back = waitFor(app.api()->get(QStringLiteral("/api/projects/%1/return-package").arg(made2.object().value("id").toInteger())));
        QVERIFY(back.ok());
        QCOMPARE(back.fileName(), QStringLiteral("Hofmann.mqback"));
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEditor)
#include "tst_editor.moc"
