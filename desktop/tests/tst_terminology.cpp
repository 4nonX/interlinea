#include "testcore.h"

#include <QFile>
#include <QJSValue>
#include <QJsonArray>
#include <QJsonObject>
#include <QQuickWindow>
#include <QSet>
#include <QSettings>
#include <QStandardPaths>
#include <QTest>
#include <QUrlQuery>

static QString terms(const QJsonObject &entry, const QString &lang)
{
    QStringList out;
    for (const QJsonValue &l : entry.value("languages").toArray())
        if (l.toObject().value("lang").toString() == lang)
            for (const QJsonValue &t : l.toObject().value("terms").toArray())
                out << t.toObject().value("text").toString();
    return out.join(QLatin1Char('|'));
}

// Terminology worked with as a person does: an entry's parts, the
// wording of the branches, termbases with entries in all their languages
// and synonyms on equal footing, fields, duplicates, deleting and bringing
// back, a project termbase on a master, and its changes reviewed and
// merged.
class TestTerminology : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_terminology"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
    }

    void entryDataAndWords()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        auto *td = app.engine.singletonInstance<QObject *>("Interlinea", "TermData");
        QVERIFY(td);
        const auto t = [](const QString &text, const QString &status = QString()) {
            QVariantMap m{{"text", text}};
            if (!status.isEmpty())
                m.insert("status", status);
            return QVariant(m);
        };
        // Changing an entry's parts, as the page does before saving.
        QVariant e = QVariantMap();
        e = app.call(td, "withTerms", {e, QStringLiteral("de-DE"), QVariantList{t("Kaution", "approved")}});
        e = app.call(td, "withTerms", {e, QStringLiteral("en-GB"), QVariantList{t("deposit"), t("security deposit", "forbidden")}});
        QCOMPARE(app.call(td, "languages", {e}).toStringList(), (QStringList{QStringLiteral("de-DE"), QStringLiteral("en-GB")}));
        QCOMPARE(app.call(td, "head", {e, QStringLiteral("en-GB")}).toMap().value("text").toString(), QStringLiteral("deposit"));
        // A language without terms is headed by another's first term, and says which.
        const QVariantMap other = app.call(td, "head", {e, QStringLiteral("fr-FR")}).toMap();
        QCOMPARE(other.value("text").toString(), QStringLiteral("Kaution"));
        QCOMPARE(other.value("lang").toString(), QStringLiteral("de-DE"));
        const QVariant en = app.call(td, "terms", {e, QStringLiteral("en-GB")});
        QVERIFY(app.call(td, "hasTerm", {en, QStringLiteral(" deposit "), -1}).toBool());
        QVERIFY(app.call(td, "hasTerm", {en, QStringLiteral("security  deposit"), -1}).toBool());
        // A spelling that differs in case is a term of its own.
        QVERIFY(!app.call(td, "hasTerm", {en, QStringLiteral("Deposit"), -1}).toBool());
        QVERIFY(!app.call(td, "hasTerm", {en, QStringLiteral("deposit"), 0}).toBool());
        QVERIFY(app.call(td, "hasStatus", {e, QStringLiteral("proposed")}).toBool());
        QVERIFY(app.call(td, "hasStatus", {e, QStringLiteral("forbidden")}).toBool());
        e = app.call(td, "withEntryField", {e, QStringLiteral("Definition"), QStringLiteral("Sicherheit des Mieters")});
        e = app.call(td, "withLanguageField", {e, QStringLiteral("en-GB"), QStringLiteral("Note"), QStringLiteral("UK usage")});
        e = app.call(td, "withTermField", {e, QStringLiteral("en-GB"), 1, QStringLiteral("Usage"), QStringLiteral("avoid")});
        QCOMPARE(app.call(td, "value", {e.toMap().value("fields"), QStringLiteral("Definition")}).toString(), QStringLiteral("Sicherheit des Mieters"));
        QCOMPARE(app.call(td, "value", {app.call(td, "language", {e, QStringLiteral("en-GB")}).toMap().value("fields"), QStringLiteral("Note")}).toString(), QStringLiteral("UK usage"));
        QCOMPARE(app.call(td, "value", {app.call(td, "terms", {e, QStringLiteral("en-GB")}).toList().at(1).toMap().value("fields"), QStringLiteral("Usage")}).toString(), QStringLiteral("avoid"));
        QVERIFY(app.call(td, "searchText", {e}).toString().contains(QStringLiteral("sicherheit")));
        e = app.call(td, "withEntryField", {e, QStringLiteral("Definition"), QString()});
        QVERIFY(e.toMap().value("fields").toList().isEmpty());
        // A language left without terms and fields goes.
        e = app.call(td, "withTerms", {e, QStringLiteral("de-DE"), QVariantList{}});
        QCOMPARE(app.call(td, "languages", {e}).toStringList(), QStringList{QStringLiteral("en-GB")});
        QCOMPARE(app.call(td, "entries", {1}).toString(), QStringLiteral("1 entry"));
        QCOMPARE(app.call(td, "termCount", {2}).toString(), QStringLiteral("2 terms"));

        // The branching words, plain or as in version control.
        app.backend.setGitWords(false);
        QCOMPARE(app.backend.word(QStringLiteral("reviewMerge")), QStringLiteral("Review and merge"));
        app.backend.setGitWords(true);
        QCOMPARE(app.backend.word(QStringLiteral("reviewMerge")), QStringLiteral("Diff and merge"));
        QCOMPARE(app.backend.word(QStringLiteral("history")), QStringLiteral("Log"));
        app.backend.setGitWords(false);
        // The editor's choices, on unless turned off.
        QVERIFY(app.backend.option(QStringLiteral("autoInsert")).toBool() && app.backend.option(QStringLiteral("autoPropagate")).toBool());
    }

    void fieldsOfANewTermbase()
    {
        // The fields of a new master are groups ticked in any combination,
        // each putting its fields in the list and taking them out again.
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        app.show(QStringLiteral("terminology"));
        QObject *dlg = app.find(QStringLiteral("newTermbase"));
        QVERIFY(dlg);
        app.call(dlg, "start", {false});
        QTRY_VERIFY(dlg->property("opened").toBool());
        dlg->setProperty("step", 2);
        QTRY_COMPARE(dlg->property("stepName").toString(), QStringLiteral("fields"));
        const auto names = [dlg] {
            QStringList out;
            for (const QVariant &f : dlg->property("fieldDrafts").toList())
                out << f.toMap().value("name").toString();
            return out;
        };
        const auto group = [&](const char *id) {
            QObject *box = app.find(dlg, QStringLiteral("fieldGroup:") + QLatin1String(id));
            QVERIFY(box);
            QVERIFY(app.click(box));
        };
        group("definitions");
        group("grammar");
        QTRY_COMPARE(names(), (QStringList{"Definition", "Source of the definition", "Part of speech", "Gender", "Number"}));
        QVERIFY(app.find(dlg, QStringLiteral("fieldGroup:definitions"))->property("checked").toBool());
        // Taken away, a group takes its fields and leaves the others.
        group("definitions");
        QTRY_COMPARE(names(), (QStringList{"Part of speech", "Gender", "Number"}));
        QVERIFY(!app.find(dlg, QStringLiteral("fieldGroup:definitions"))->property("checked").toBool());
        // With every group, every field of them, and no field twice.
        for (const char *id : {"definitions", "note", "subject", "usage", "checked"})
            group(id);
        QTRY_COMPARE(names().size(), 10);
        const QStringList all = names();
        QCOMPARE(QSet<QString>(all.begin(), all.end()).size(), 10);
        // A field of a group taken out of the list unticks the group, after a click too.
        QVariantList kept;
        for (const QVariant &f : dlg->property("fieldDrafts").toList())
            if (f.toMap().value("name").toString() != QStringLiteral("Note"))
                kept << f;
        dlg->setProperty("fieldDrafts", kept);
        QTRY_VERIFY(!app.find(dlg, QStringLiteral("fieldGroup:note"))->property("checked").toBool());
        QVERIFY(app.find(dlg, QStringLiteral("fieldGroup:usage"))->property("checked").toBool());
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.call(dlg, "close");
    }

    void terminology()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        ApiClient *api = app.api();
        const QString here = app.connections.local()->id();
        QVERIFY(waitFor(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Kanzlei Weber"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        app.show(QStringLiteral("terminology"));
        QObject *tb = app.find(QStringLiteral("terminology"));
        QVERIFY(tb);
        app.call(tb, "open", {here, QStringLiteral("Kanzlei Weber"), QString()});
        QTRY_VERIFY_WITH_TIMEOUT(tb->property("opened").toBool() && !tb->property("loading").toBool(), 10000);
        QCOMPARE(tb->property("languages").toStringList(), (QStringList{QStringLiteral("de-DE"), QStringLiteral("en-GB")}));

        // A new entry; a term in another language makes it a language of the termbase.
        app.call(tb, "addEntry", {QStringLiteral("Kaution"), QStringLiteral("de-DE"), QStringLiteral("approved")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added “Kaution”"), 10000);
        const QString id = tb->property("selected").toString();
        QVERIFY(!id.isEmpty());
        // Every language, every synonym on equal footing: two English terms
        // and a French one, each with its own status.
        const QJSValue addTerms = app.engine.evaluate(QStringLiteral(
            "(function (e) { e.languages.push({ lang: 'en-GB', terms: [{ text: 'deposit', status: 'approved' }, { text: 'security deposit', status: 'proposed' }] });"
            " e.languages.push({ lang: 'fr-FR', terms: [{ text: 'dépôt de garantie', status: 'approved' }] }); return e })"));
        QVERIFY(addTerms.isCallable());
        app.call(tb, "change", {id, QStringLiteral("Terms added"), QVariant::fromValue(addTerms), QVariant()});
        QTRY_COMPARE_WITH_TIMEOUT(terms(waitFor(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries/") + id)).object(), QStringLiteral("en-GB")),
                                  QStringLiteral("deposit|security deposit"), 10000);
        QCOMPARE(terms(waitFor(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries/") + id)).object(), QStringLiteral("fr-FR")), QStringLiteral("dépôt de garantie"));
        // The synonym is found as the first term is.
        tb->setProperty("query", QStringLiteral("security"));
        QTRY_COMPARE(tb->property("shown").toList().size(), 1);
        tb->setProperty("query", QString());
        tb->setProperty("status", QStringLiteral("proposed"));
        QTRY_COMPARE(tb->property("shown").toList().size(), 1);
        tb->setProperty("status", QString());
        // A language is added to the termbase, a field too, and taken away.
        app.call(tb, "addLanguage", {QStringLiteral("fr-FR")});
        QTRY_VERIFY_WITH_TIMEOUT(tb->property("languages").toStringList().contains(QStringLiteral("fr-FR")), 10000);
        app.call(tb, "addField", {QVariantMap{{"name", "Subject"}, {"level", "entry"}, {"kind", "text"}}});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added field “Subject” to Kanzlei Weber"), 10000);
        app.call(tb, "removeField", {QStringLiteral("Subject")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Removed the field “Subject”"), 10000);
        for (const QSize size : {QSize(800, 520), QSize(1024, 700), QSize(1366, 860)}) {
            app.window->resize(size);
            const QString broken = layoutProblems(app.window);
            QVERIFY2(broken.isEmpty(), qPrintable(broken));
        }
        app.shot(QStringLiteral("terminology"));

        // Two entries of the same term are found and merged into one.
        app.call(tb, "addEntry", {QStringLiteral("Miete"), QStringLiteral("de-DE"), QStringLiteral("approved")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Added “Miete”"), 10000);
        app.call(tb, "addEntry", {QStringLiteral("Miete"), QStringLiteral("de-DE"), QStringLiteral("proposed")});
        QTRY_COMPARE_WITH_TIMEOUT(tb->property("entries").toList().size(), 3, 10000);
        // Asked once the page has read again what the new entries changed.
        tb->setProperty("dupLang", QStringLiteral("de-DE"));
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            if (tb->property("duplicates").isNull() && !tb->property("loading").toBool())
                app.call(tb, "loadDuplicates");
            QTest::qWait(300);
            return !tb->property("duplicates").isNull() && tb->property("duplicates").toList().size() == 1;
        }(), 15000);
        app.call(tb, "mergeGroup", {tb->property("duplicates").toList().first()});
        QTRY_COMPARE_WITH_TIMEOUT(waitFor(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber"))).object().value("entries").toInt(), 2, 10000);

        // Deleted, and brought back.
        QVariantMap kaution;
        for (const QVariant &v : tb->property("entries").toList())
            if (v.toMap().value("id").toString() == id)
                kaution = v.toMap();
        app.call(tb, "deleteEntry", {kaution, QStringLiteral("Kaution")});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Deleted the entry “Kaution”"), 10000);
        // The entry merged into another is among the deleted as well. Asked
        // again until it is there: a reload on the server's news of the
        // change may come after the first answer and take its place.
        const auto deletedIds = [tb] {
            QStringList ids;
            for (const QVariant &d : tb->property("deleted").toList())
                ids << d.toMap().value("entryId").toString();
            return ids;
        };
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            app.call(tb, "loadDeleted");
            QTest::qWait(300);
            return deletedIds().contains(id);
        }(), 15000);
        app.call(tb, "bringBack", {id});
        QTRY_VERIFY_WITH_TIMEOUT(waitFor(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries/") + id)).ok(), 10000);

        // A TBX file in: its entries are there.
        QFile tbx(QStringLiteral(INTERLINEA_FIXTURES "/glossar.tbx"));
        QVERIFY(tbx.open(QIODevice::ReadOnly));
        QVERIFY(waitFor(api->upload("POST", QStringLiteral("/api/termbases/Kanzlei%20Weber/import"), tbx.readAll(), QStringLiteral("application/octet-stream"))).ok());
        app.call(tb, "reload");
        QTRY_VERIFY_WITH_TIMEOUT(tb->property("entries").toList().size() > 3, 10000);
    }

    void reviewAndMerge()
    {
        TestCore core;
        QmlApp app(&core.launcher);
        QString why;
        QVERIFY2(app.start(&why), qPrintable(why));
        ApiClient *api = app.api();
        const QString here = app.connections.local()->id();
        QVERIFY(waitFor(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Kanzlei Weber"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        const ApiResult master = waitFor(api->post(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries"), QJsonObject{{"entry", QJsonObject{{"languages", QJsonArray{
            QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Miete"}, {"status", "approved"}}}}},
            QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", "rent"}, {"status", "approved"}}}}}}}}}, {"summary", "Created"}}));
        QVERIFY(master.ok());
        QVERIFY(waitFor(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Hofmann Projekt"}}}, {"upstreams", QJsonArray{"Kanzlei Weber"}}})).ok());
        app.show(QStringLiteral("terminology"));
        QObject *tb = app.find(QStringLiteral("terminology"));
        app.call(tb, "open", {here, QStringLiteral("Hofmann Projekt"), QString()});
        QTRY_VERIFY_WITH_TIMEOUT(tb->property("isProject").toBool(), 10000);
        // A master's entry taken into the project, changed there.
        QTRY_COMPARE_WITH_TIMEOUT(tb->property("masterEntries").toList().size(), 1, 10000);
        app.call(tb, "override", {tb->property("masterEntries").toList().first()});
        QTRY_COMPARE_WITH_TIMEOUT(app.notice(), QStringLiteral("Took the entry into the project"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(tb->property("entries").toList().size(), 1, 10000);
        const QString own = tb->property("entries").toList().first().toMap().value("id").toString();
        const QJSValue synonym = app.engine.evaluate(QStringLiteral(
            "(function (e) { for (const l of e.languages) if (l.lang === 'en-GB') l.terms.push({ text: 'lease payment', status: 'proposed' }); return e })"));
        app.call(tb, "change", {own, QStringLiteral("Synonym"), QVariant::fromValue(synonym), QVariant()});
        QTRY_COMPARE_WITH_TIMEOUT(terms(waitFor(api->get(QStringLiteral("/api/termbases/Hofmann%20Projekt/entries/") + own)).object(), QStringLiteral("en-GB")),
                                  QStringLiteral("rent|lease payment"), 10000);
        // Asked to be merged, reviewed: accepted, and merged into the master.
        const ApiResult mr = waitFor(api->post(QStringLiteral("/api/termbases/Hofmann%20Projekt/merge-requests"), QJsonObject{{"master", "Kanzlei Weber"}, {"title", "Hofmann into Weber"}}));
        QVERIFY2(mr.ok(), qPrintable(mr.error.message));
        app.call(app.find(QStringLiteral("terminologyHost")), "start", {here, mr.object().value("id").toInteger()});
        QObject *review = app.find(QStringLiteral("review"));
        QTRY_VERIFY_WITH_TIMEOUT(review->property("visible").toBool() && review->property("items").toList().size() == 1, 10000);
        QVERIFY2(layoutProblems(app.window).isEmpty(), qPrintable(layoutProblems(app.window)));
        app.shot(QStringLiteral("review"));
        app.call(review, "decide", {QVariantMap{{"ids", QVariantList{review->property("items").toList().first().toMap().value("id")}}}, QStringLiteral("approved"), QVariant(), QString()});
        QTRY_VERIFY_WITH_TIMEOUT(app.notice().endsWith(QStringLiteral(": 1 entry.")), 10000);
        const ApiResult closed = waitFor(api->post(QStringLiteral("/api/merge-requests/%1/close").arg(mr.object().value("id").toInteger())));
        QVERIFY2(closed.ok(), qPrintable(closed.error.message));
        QCOMPARE(terms(waitFor(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries/") + master.object().value("id").toString())).object(), QStringLiteral("en-GB")),
                 QStringLiteral("rent|lease payment"));
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestTerminology)
#include "tst_terminology.moc"
