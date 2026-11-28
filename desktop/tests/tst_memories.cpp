#include "testcore.h"

#include <QDir>
#include <QFile>
#include <QJsonArray>
#include <QJsonObject>
#include <QQuickItem>
#include <QQuickWindow>
#include <QSettings>
#include <QStandardPaths>
#include <QTest>
#include <QUrl>

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

// The Memories page worked with as a person does: memories made, filled
// from files and emptied out into them, kept from deletion while a project
// uses them; units searched, added, changed and deleted; and a document
// and its translation aligned into a memory.
class TestMemories : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_memories"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void memoriesOnThisComputer()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        app.show(QStringLiteral("memories"));
        const QString here = app.connections.local()->id();
        QObject *form = app.find(QStringLiteral("newMemory:") + here);
        QVERIFY(form);

        // A new memory for a client, filled from a TMX file whose languages it takes.
        QVERIFY(app.click(app.find(form, QStringLiteral("newMemory"))));
        QTRY_VERIFY(form->property("opened").toBool());
        // A choice of language opened shows its languages: every row of
        // its list has its name.
        {
            QObject *source = app.find(form, QStringLiteral("memorySource"));
            QVERIFY(source);
            QObject *popup = source->property("popup").value<QObject *>();
            QVERIFY(popup);
            app.call(popup, "open");
            QTRY_VERIFY(popup->property("opened").toBool());
            QObject *list = popup->property("contentItem").value<QObject *>();
            QTRY_VERIFY(list && list->property("count").toInt() > 10);
            QStringList shown;
            for (int i = 0; i < 5; ++i) {
                // The list makes its rows as it opens.
                QQuickItem *row = nullptr;
                QTRY_VERIFY2([&] {
                    QMetaObject::invokeMethod(list, "itemAtIndex", Q_RETURN_ARG(QQuickItem *, row), Q_ARG(int, i));
                    return row != nullptr;
                }(), qPrintable(QStringLiteral("row %1 is not there").arg(i)));
                shown << row->property("contentItem").value<QObject *>()->property("text").toString();
                QCOMPARE(shown.last(), source->property("model").toList().at(i).toMap().value("label").toString());
            }
            QVERIFY2(!shown.contains(QString()), qPrintable(shown.join(QLatin1Char('|'))));
            app.call(popup, "close");
        }
        app.call(form, "choose", {QUrl::fromLocalFile(QStringLiteral(INTERLINEA_FIXTURES "/test.tmx")).toString()});
        QVERIFY(app.type(app.find(form, QStringLiteral("memoryName")), QStringLiteral("Weber DE-EN")));
        QVERIFY(app.type(app.find(form, QStringLiteral("memoryClient")), QStringLiteral("Kanzlei Weber")));
        QTRY_VERIFY(app.clickable(QStringLiteral("createMemory")));
        QVERIFY(app.click(QStringLiteral("createMemory")));
        QTRY_VERIFY_WITH_TIMEOUT(app.notice().startsWith(QStringLiteral("Created the memory Weber DE-EN with ")), 10000);
        // The form is put away again.
        QTRY_VERIFY(!form->property("opened").toBool());
        const QJsonObject made = waitFor(app.api()->get(QStringLiteral("/api/memories/Weber%20DE-EN"))).object();
        QCOMPARE(made.value("client").toString(), QStringLiteral("Kanzlei Weber"));
        QVERIFY(made.value("units").toInt() > 0);

        // A name that is taken, and equal languages, are said before anything is sent.
        QVERIFY(app.click(app.find(form, QStringLiteral("newMemory"))));
        QVERIFY(app.type(app.find(form, QStringLiteral("memoryName")), QStringLiteral("weber de-en")));
        QTRY_COMPARE(app.call(form, "problem").toString(), QStringLiteral("A memory with this name exists already."));
        QVERIFY(!app.clickable(QStringLiteral("createMemory")));
        QVERIFY(app.type(app.find(form, QStringLiteral("memoryName")), QStringLiteral("Leer")));
        app.call(app.find(form, QStringLiteral("memoryTarget")), "pick", {QStringLiteral("de-DE")});
        QCOMPARE(app.call(form, "problem").toString(), QStringLiteral("Source and target language must differ."));
        app.call(app.find(form, QStringLiteral("memoryTarget")), "pick", {QStringLiteral("en-GB")});
        // Without a file it is made empty.
        QVERIFY(app.click(QStringLiteral("createMemory")));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Created the memory Leer"), 10000);

        // TMX out, and in again into the empty one.
        const QString out = m_tmp.filePath(QStringLiteral("weber.tmx"));
        app.call(QStringLiteral("memoriesPage"), "exportFile", {here, QStringLiteral("Weber DE-EN"), QUrl::fromLocalFile(out).toString(), QStringLiteral("tmx")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Saved weber.tmx"), 10000);
        QVERIFY(QFile(out).size() > 100);
        app.call(QStringLiteral("memoriesPage"), "importFile", {here, QStringLiteral("Leer"), QUrl::fromLocalFile(out).toString()});
        QTRY_VERIFY_WITH_TIMEOUT(app.notice().startsWith(QStringLiteral("Imported ")) && app.notice().endsWith(QStringLiteral("from weber.tmx into Leer")), 10000);
        QCOMPARE(waitFor(app.api()->get(QStringLiteral("/api/memories/Leer"))).object().value("units").toInt(), made.value("units").toInt());
        // As a .txt memory as well.
        const QString txt = m_tmp.filePath(QStringLiteral("weber.txt"));
        app.call(QStringLiteral("memoriesPage"), "exportFile", {here, QStringLiteral("Weber DE-EN"), QUrl::fromLocalFile(txt).toString(), QStringLiteral("wordfast")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Saved weber.txt"), 10000);
        QFile wf(txt);
        QVERIFY(wf.open(QIODevice::ReadOnly) && wf.size() > 20);

        // A project that uses a memory keeps it from being deleted.
        QJsonObject settings = waitFor(app.api()->get(QStringLiteral("/api/projects/defaults"))).object();
        settings.insert("name", "Mit Speicher");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("memories", QJsonObject{{"en-GB", QJsonArray{QJsonObject{{"memory", "Leer"}, {"lookup", true}}}}});
        QVERIFY(waitFor(app.api()->post(QStringLiteral("/api/projects"), QJsonObject{{"settings", settings},
            {"files", QJsonArray{QJsonObject{{"name", "a.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(QByteArray("Satz.").toBase64())}}}}})).ok());
        app.call(QStringLiteral("memoriesPage"), "reload");
        QTRY_COMPARE_WITH_TIMEOUT(app.call(QStringLiteral("memoriesPage"), "usedBy", {here, QStringLiteral("Leer")}).toStringList(), QStringList{QStringLiteral("Mit Speicher")}, 10000);
        QVERIFY(!waitFor(app.api()->del(QStringLiteral("/api/memories/Leer"))).ok());
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.shot(QStringLiteral("memories"));
    }

    void unitsOfAMemory()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        ApiClient *api = app.api();
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "M"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories/M/units"), QJsonObject{{"source", text("Die Miete ist fällig.")}, {"target", text("The rent is due.")}})).ok());
        // A unit with formatting in it.
        QVERIFY(waitFor(api->post(QStringLiteral("/api/memories/M/units"), QJsonObject{
            {"source", QJsonArray{QJsonObject{{"text", "Die "}}, QJsonObject{{"tag", QJsonObject{{"id", "1"}, {"kind", "open"}, {"type", "bold"}}}}, QJsonObject{{"text", "Kaution"}},
                                  QJsonObject{{"tag", QJsonObject{{"id", "1"}, {"kind", "close"}, {"type", "bold"}}}}, QJsonObject{{"text", " ist fällig."}}}},
            {"target", QJsonArray{QJsonObject{{"text", "The "}}, QJsonObject{{"tag", QJsonObject{{"id", "1"}, {"kind", "open"}, {"type", "bold"}}}}, QJsonObject{{"text", "deposit"}},
                                  QJsonObject{{"tag", QJsonObject{{"id", "1"}, {"kind", "close"}, {"type", "bold"}}}}, QJsonObject{{"text", " is due."}}}}})).ok());
        const QString here = app.connections.local()->id();
        app.show(QStringLiteral("memories"));
        app.call(QStringLiteral("memoriesPage"), "reload");
        app.call(QStringLiteral("memoriesPage"), "open", {here, QStringLiteral("M")});
        QObject *detail = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((detail = app.find(QStringLiteral("memoryDetail:") + here)) && detail->property("visible").toBool(), 10000);

        // Searching the sentences, and the translations.
        QVERIFY(app.type(app.find(detail, QStringLiteral("unitQuery")), QStringLiteral("fällig")));
        app.call(detail, "search");
        QTRY_COMPARE_WITH_TIMEOUT(detail->property("found").toList().size(), 2, 10000);
        QVERIFY(app.type(app.find(detail, QStringLiteral("unitQuery")), QStringLiteral("deposit")));
        app.click(app.find(detail, QStringLiteral("inTranslations")));
        app.call(detail, "search");
        QTRY_COMPARE_WITH_TIMEOUT(detail->property("found").toList().size(), 1, 10000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));

        // A new unit; the search is run again and finds it.
        QVERIFY(app.click(app.find(detail, QStringLiteral("addUnitButton"))));
        QTRY_VERIFY(app.get(QStringLiteral("addUnit"), "opened").toBool());
        QVERIFY(app.type(app.find(app.find(QStringLiteral("addUnit")), QStringLiteral("unitSource"))->property("area").value<QObject *>(), QStringLiteral("Die Kaution ist hoch.")));
        QVERIFY(app.type(app.find(app.find(QStringLiteral("addUnit")), QStringLiteral("unitTarget"))->property("area").value<QObject *>(), QStringLiteral("The deposit is high.")));
        QVERIFY(app.click(QStringLiteral("saveUnit")));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added the unit to M"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(detail->property("found").toList().size(), 2, 10000);

        // A translation with formatting keeps its tags when the words change.
        QVariantMap tagged;
        for (const QVariant &u : detail->property("found").toList())
            if (plain(u.toMap().value("source")).startsWith(QStringLiteral("Die Kaution ist f")))
                tagged = u.toMap();
        QVERIFY(!tagged.isEmpty());
        QVERIFY(app.click(QStringLiteral("changeUnit:") + tagged.value("id").toString()));
        QTRY_VERIFY(app.get(QStringLiteral("editUnit"), "opened").toBool());
        QObject *editTarget = app.find(QStringLiteral("editTarget"));
        QVariantList content = editTarget->property("content").toList();
        QVariantMap last = content.last().toMap();
        last.insert("text", QStringLiteral(" was due."));
        content.last() = last;
        editTarget->setProperty("content", content);
        QVERIFY(app.click(QStringLiteral("saveChange")));
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Saved the unit"), 10000);
        const QJsonObject changed = waitFor(app.api()->get(QStringLiteral("/api/memories/M/units/") + tagged.value("id").toString())).object();
        int tags = 0;
        for (const QJsonValue &run : changed.value("target").toArray())
            tags += run.toObject().contains("tag") ? 1 : 0;
        QCOMPARE(tags, 2);
        QCOMPARE(plain(changed.value("target").toVariant()), QStringLiteral("The deposit was due."));

        // Without a translation it is not a unit.
        QVERIFY(app.click(QStringLiteral("changeUnit:") + tagged.value("id").toString()));
        app.find(QStringLiteral("editTarget"))->setProperty("content", QVariantList{});
        QVERIFY(app.click(QStringLiteral("saveChange")));
        QCOMPARE(app.get(QStringLiteral("editUnit"), "error").toString(), QStringLiteral("A unit needs a translation. Delete the unit instead."));
        app.call(QStringLiteral("editUnit"), "close");

        // Deleting a unit asks first. Its row is the one the search after
        // the change made (the rows are made again with each search): a
        // button of a row made before would be gone before it answered.
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            for (const QVariant &u : detail->property("found").toList())
                if (plain(u.toMap().value("target")) == QLatin1String("The deposit was due."))
                    return true;
            return false;
        }(), 10000);
        QObject *del = app.find(QStringLiteral("deleteUnit:") + tagged.value("id").toString());
        QVERIFY(del);
        // Its button asks; the question's own button deletes.
        QObject *ask = nullptr;
        for (QObject *c : del->findChildren<QObject *>())
            if (c->inherits("QQuickAbstractButton") && c->property("visible").toBool() && !ask)
                ask = c;
        QVERIFY(app.click(ask));
        QTRY_VERIFY(del->property("asking").toBool());
        QMetaObject::invokeMethod(del, "confirmed");
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Deleted the unit"), 10000);
        QVERIFY(!waitFor(app.api()->get(QStringLiteral("/api/memories/M/units/") + tagged.value("id").toString())).ok());
    }

    void alignInto()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        const QString here = app.connections.local()->id();
        QVERIFY(waitFor(app.api()->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Lyon"}, {"sourceLang", "de-DE"}, {"targetLang", "fr-FR"}})).ok());
        app.show(QStringLiteral("memories"));
        app.call(QStringLiteral("memoriesPage"), "reload");
        QTRY_VERIFY_WITH_TIMEOUT(!app.call(QStringLiteral("memoriesPage"), "memory", {here, QStringLiteral("Lyon")}).isNull(), 10000);
        // From the page: into a memory chosen in the dialog, or one made for the documents.
        app.call(QStringLiteral("memoriesPage"), "alignHere", {here});
        QObject *dlg = app.find(QStringLiteral("alignDocuments"));
        QTRY_VERIFY(dlg->property("opened").toBool());
        QCOMPARE(dlg->property("memory").toString(), QStringLiteral("Lyon"));
        app.call(dlg, "chooseMemory", {QString()});
        QVERIFY(dlg->property("isNew").toBool());
        QCOMPARE(dlg->property("destinationProblem").toString(), QStringLiteral("Give the memory a name."));
        dlg->setProperty("newName", QStringLiteral("lyon"));
        QCOMPARE(dlg->property("destinationProblem").toString(), QStringLiteral("A memory with this name exists already."));
        app.call(dlg, "chooseMemory", {QStringLiteral("Lyon")});
        const QString align = QStringLiteral(INTERLINEA_FIXTURES "/align/");
        app.call(dlg, "setFile", {0, QUrl::fromLocalFile(align + "Mietvertrag_de.txt").toString()});
        app.call(dlg, "setFile", {1, QUrl::fromLocalFile(align + "Mietvertrag_fr_umgestellt.txt").toString()});
        app.call(dlg, "pair");
        QTRY_VERIFY_WITH_TIMEOUT(dlg->property("hasResult").toBool(), 15000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.shot(QStringLiteral("align"));
        const auto order = [dlg](int side) { return dlg->property("order").toList().at(side).toList(); };
        QCOMPARE(order(1).size(), 4);
        // The translation's first section is moved down into the document's order.
        app.call(dlg, "nudge", {1, 0, 1});
        QTRY_COMPARE_WITH_TIMEOUT(order(1).value(1).toInt(), 0, 15000);
        QVERIFY(dlg->property("past").toList().size() == 1);
        // A line drawn by hand is the translator's own; undone, it is gone.
        const int lines = dlg->property("links").toList().size();
        app.call(dlg, "choose", {0, 3});
        app.call(dlg, "choose", {1, 3});
        QCOMPARE(dlg->property("links").toList().size(), lines + 1);
        QVERIFY(dlg->property("mine").toList().size() >= 1);
        app.call(dlg, "undo");
        QCOMPARE(dlg->property("links").toList().size(), lines);
        app.call(dlg, "redo");
        QCOMPARE(dlg->property("links").toList().size(), lines + 1);
        // Confirm all makes every line the translator's own.
        app.call(dlg, "confirmAll");
        QCOMPARE(dlg->property("mine").toList().size(), dlg->property("links").toList().size());
        // Closing with work on the lines asks first; staying keeps it open.
        app.call(dlg, "cancel");
        QCOMPARE(dlg->property("leaveQuestion").toString(), QStringLiteral("Close without adding? What you did to the lines is lost."));
        app.call(dlg, "stay");
        QVERIFY(dlg->property("opened").toBool());
        // The units go in.
        const int units = app.call(dlg, "units").toList().size();
        app.call(dlg, "add");
        QTRY_VERIFY_WITH_TIMEOUT(!dlg->property("opened").toBool(), 10000);
        QCOMPARE(app.notice(), QStringLiteral("Added %1 units to Lyon").arg(units));
        QCOMPARE(waitFor(app.api()->get(QStringLiteral("/api/memories/Lyon"))).object().value("units").toInt(), units);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestMemories)
#include "tst_memories.moc"
