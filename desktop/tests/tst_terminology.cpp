// The Terminology page against the app's own core: termbases, entries with
// their terms, fields and history, search, duplicates, the import wizard,
// export and the termbase's settings.
#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "options.h"
#include "reviewpage.h"
#include "termbasedialogs.h"
#include "termdata.h"
#include "terminologypage.h"
#include "testcore.h"
#include "theme.h"
#include "webui.h"
#include "words.h"

#include <QApplication>
#include <QCheckBox>
#include <QComboBox>
#include <QFile>
#include <QLabel>
#include <QLineEdit>
#include <QListView>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QRadioButton>
#include <QScrollArea>
#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTableWidget>
#include <QTemporaryDir>
#include <QTest>
#include <QUrlQuery>

static ApiResult wait(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

// Whether a widget is there to be seen under root: neither it nor anything
// it sits in was hidden. A widget just made counts, though it is drawn
// only on the next turn of the event loop.
static bool there(const QWidget *w, const QWidget *root)
{
    for (; w; w = w->parentWidget()) {
        if (w->isHidden() && w->testAttribute(Qt::WA_WState_ExplicitShowHide))
            return false;
        if (w == root)
            return true;
    }
    return true;
}

template <typename T>
static T *byName(QWidget *root, const QString &objectName)
{
    for (T *w : root->findChildren<T *>(objectName)) {
        if (there(w, root))
            return w;
    }
    return nullptr;
}

template <typename T>
static T *named(QWidget *root, const QString &accessibleName)
{
    for (T *w : root->findChildren<T *>()) {
        if (w->accessibleName() == accessibleName && there(w, root))
            return w;
    }
    return nullptr;
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>()) {
        if (b->text() == text && there(b, root))
            return b;
    }
    return nullptr;
}

static QLabel *label(QWidget *root, const QString &text)
{
    for (QLabel *l : root->findChildren<QLabel *>()) {
        if (l->text() == text && there(l, root))
            return l;
    }
    return nullptr;
}

static QLabel *labelWith(QWidget *root, const QString &part)
{
    for (QLabel *l : root->findChildren<QLabel *>()) {
        if (l->text().contains(part) && there(l, root))
            return l;
    }
    return nullptr;
}

// A picture of the page for the documentation, once it has settled.
static void shot(QWidget *page, const QString &name)
{
    const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS");
    if (dir.isEmpty())
        return;
    QTest::qWait(200);
    page->grab().save(dir + QLatin1Char('/') + name + QStringLiteral(".png"));
}

static void choose(QComboBox *box, const QVariant &data)
{
    const int i = box->findData(data);
    QVERIFY2(i >= 0, qPrintable(QStringLiteral("no choice %1").arg(data.toString())));
    box->setCurrentIndex(i);
    emit box->activated(i);
}

static QString cell(QTableWidget *table, int row, int column)
{
    return table && table->item(row, column) ? table->item(row, column)->data(Qt::AccessibleTextRole).toString() : QString();
}

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
        Theme::apply(*qApp);
    }

    void entryData()
    {
        // Changing an entry's parts, as the page does before saving.
        QJsonObject e;
        e = TermData::withTerms(e, QStringLiteral("de-DE"), QJsonArray{QJsonObject{{"text", "Kaution"}, {"status", "approved"}}});
        e = TermData::withTerms(e, QStringLiteral("en-GB"), QJsonArray{QJsonObject{{"text", "deposit"}}, QJsonObject{{"text", "security deposit"}, {"status", "forbidden"}}});
        QCOMPARE(TermData::languages(e), QStringList({QStringLiteral("de-DE"), QStringLiteral("en-GB")}));
        QCOMPARE(TermData::head(e, QStringLiteral("en-GB")), QStringLiteral("deposit"));
        QString in;
        QCOMPARE(TermData::head(e, QStringLiteral("fr-FR"), &in), QStringLiteral("Kaution"));
        QCOMPARE(in, QStringLiteral("de-DE"));
        QVERIFY(TermData::hasTerm(TermData::terms(e, QStringLiteral("en-GB")), QStringLiteral(" Deposit ")));
        QVERIFY(!TermData::hasTerm(TermData::terms(e, QStringLiteral("en-GB")), QStringLiteral("deposit"), 0));
        QVERIFY(TermData::hasStatus(e, QStringLiteral("proposed")));
        QVERIFY(TermData::hasStatus(e, QStringLiteral("forbidden")));
        e = TermData::withEntryField(e, QStringLiteral("Definition"), QStringLiteral("Sicherheit des Mieters"));
        e = TermData::withLanguageField(e, QStringLiteral("en-GB"), QStringLiteral("Note"), QStringLiteral("UK usage"));
        e = TermData::withTermField(e, QStringLiteral("en-GB"), 1, QStringLiteral("Usage"), QStringLiteral("avoid"));
        QCOMPARE(TermData::value(e.value("fields").toArray(), QStringLiteral("Definition")), QStringLiteral("Sicherheit des Mieters"));
        QCOMPARE(TermData::value(TermData::language(e, QStringLiteral("en-GB")).value("fields").toArray(), QStringLiteral("Note")), QStringLiteral("UK usage"));
        QCOMPARE(TermData::value(TermData::terms(e, QStringLiteral("en-GB"))[1].toObject().value("fields").toArray(), QStringLiteral("Usage")), QStringLiteral("avoid"));
        QVERIFY(TermData::searchText(e).contains(QStringLiteral("sicherheit")));
        e = TermData::withEntryField(e, QStringLiteral("Definition"), QString());
        QVERIFY(e.value("fields").toArray().isEmpty());
        // A language left without terms and fields goes.
        e = TermData::withTerms(e, QStringLiteral("de-DE"), {});
        QCOMPARE(TermData::languages(e), QStringList({QStringLiteral("en-GB")}));
        e = TermData::withTerms(e, QStringLiteral("en-GB"), {});
        QCOMPARE(TermData::languages(e), QStringList({QStringLiteral("en-GB")}));
        QCOMPARE(TermData::entries(1), QStringLiteral("1 entry"));
        QCOMPARE(TermData::termCount(2), QStringLiteral("2 terms"));
    }

    void words()
    {
        Words::setGit(false);
        QCOMPARE(Words::get("reviewMerge"), QStringLiteral("Review and merge"));
        QCOMPARE(Words::get("basedOn"), QStringLiteral("Based on"));
        Words::setGit(true);
        QCOMPARE(Words::get("reviewMerge"), QStringLiteral("Diff and merge"));
        QCOMPARE(Words::get("history"), QStringLiteral("Log"));
        Words::setGit(false);
        // The editor's choices, on unless turned off.
        QVERIFY(Options::autoInsert() && Options::autoPropagate());
        QCOMPARE(Options::contextRule(), QStringLiteral("previous"));
        Options::setAutoInsert(false);
        Options::setContextRule(QStringLiteral("both"));
        QVERIFY(!Options::autoInsert());
        QCOMPARE(Options::contextRule(), QStringLiteral("both"));
        Options::setContextRule(QStringLiteral("nonsense"));
        QCOMPARE(Options::contextRule(), QStringLiteral("previous"));
        Options::setAutoInsert(true);
    }

    void parts()
    {
        Web::Tabs tabs(QStringLiteral("Views"), nullptr);
        tabs.add(QStringLiteral("One"));
        tabs.add(QStringLiteral("Two"));
        QSignalSpy changed(&tabs, &Web::Tabs::changed);
        tabs.tab(1)->click();
        QCOMPARE(tabs.current(), 1);
        QCOMPARE(changed.size(), 1);
        QVERIFY(tabs.tab(1)->isChecked() && !tabs.tab(0)->isChecked());
        tabs.tab(1)->click();
        QCOMPARE(changed.size(), 1);
        QVERIFY(tabs.tab(1)->isChecked());

        Web::Segmented seg(QStringLiteral("Store on"), {QStringLiteral("Local"), QStringLiteral("Server")}, nullptr);
        QSignalSpy picked(&seg, &Web::Segmented::changed);
        seg.segment(1)->click();
        QCOMPARE(seg.current(), 1);
        QCOMPARE(picked.size(), 1);

        Web::Stepper steps(nullptr);
        steps.setSteps({QStringLiteral("File"), QStringLiteral("Languages"), QStringLiteral("Review")}, 1);
        QCOMPARE(steps.current(), 1);
        QCOMPARE(steps.steps().size(), 3);

        Web::Clickable link("tbLink", nullptr);
        QSignalSpy clicked(&link, &Web::Clickable::clicked);
        link.show();
        QTest::keyClick(&link, Qt::Key_Return);
        QTest::mouseClick(&link, Qt::LeftButton);
        QCOMPARE(clicked.size(), 2);
        link.setCurrent(true);
        QVERIFY(link.isCurrent());
    }

    void terminology()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();

        TerminologyPage page(&m);
        page.resize(1360, 900);
        page.show();
        QSignalSpy loaded(&page, &TerminologyPage::loaded);
        QSignalSpy saved(&page, &TerminologyPage::saved);
        QTRY_VERIFY_WITH_TIMEOUT(label(&page, QStringLiteral("No termbase yet. Make a master on the left, or import a termbase file.")), 15000);
        QVERIFY(label(&page, QStringLiteral("Masters")));
        QVERIFY(label(&page, QStringLiteral("Project termbases")));

        // A new master with two languages.
        QVERIFY(button(&page, QStringLiteral("New master")));
        button(&page, QStringLiteral("New master"))->click();
        QLineEdit *name = nullptr;
        QTRY_VERIFY((name = byName<QLineEdit>(&page, QStringLiteral("newMasterName"))));
        QVERIFY(!button(&page, QStringLiteral("Create master"))->isEnabled());
        name->setText(QStringLiteral("Kanzlei Weber"));
        QVERIFY(byName<QComboBox>(&page, QStringLiteral("newMasterLanguage")));
        choose(byName<QComboBox>(&page, QStringLiteral("newMasterLanguage")), QStringLiteral("de-DE"));
        QTRY_VERIFY(button(&page, QStringLiteral("German (Germany) ×")));
        choose(byName<QComboBox>(&page, QStringLiteral("newMasterLanguage")), QStringLiteral("fr-FR"));
        choose(byName<QComboBox>(&page, QStringLiteral("newMasterLanguage")), QStringLiteral("en-GB"));
        QTRY_VERIFY(button(&page, QStringLiteral("French (France) ×")));
        button(&page, QStringLiteral("French (France) ×"))->click();
        QTRY_VERIFY(!button(&page, QStringLiteral("French (France) ×")));
        QCOMPARE(byName<QLineEdit>(&page, QStringLiteral("newMasterName"))->text(), QStringLiteral("Kanzlei Weber"));
        QTRY_VERIFY(button(&page, QStringLiteral("Create master")) && button(&page, QStringLiteral("Create master"))->isEnabled());
        button(&page, QStringLiteral("Create master"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Kanzlei Weber"), 10000);
        QTRY_VERIFY(label(&page, QStringLiteral("No entries yet")));
        QCOMPARE(page.message(), QStringLiteral("Created master “Kanzlei Weber”"));
        QCOMPARE(page.languages(), QStringList({QStringLiteral("de-DE"), QStringLiteral("en-GB")}));
        QVERIFY(byName<Web::Clickable>(&page, QStringLiteral("tb:%1:Kanzlei Weber").arg(here))->isCurrent());
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("entryCount"))->text(), QStringLiteral("0 entries"));
        QVERIFY(button(&page, QStringLiteral("Settings")));
        QVERIFY(!button(&page, QStringLiteral("Reload")));
        QVERIFY(!button(&page, QStringLiteral("Export"))->isEnabled());

        // A first entry, named by its first term.
        byName<QPushButton>(&page, QStringLiteral("newEntry"))->click();
        QTRY_VERIFY(byName<QLineEdit>(&page, QStringLiteral("newEntryText")));
        QVERIFY(!byName<QPushButton>(&page, QStringLiteral("addEntry"))->isEnabled());
        byName<QLineEdit>(&page, QStringLiteral("newEntryText"))->setText(QStringLiteral("Kaution"));
        QCOMPARE(byName<QComboBox>(&page, QStringLiteral("newEntryLanguage"))->currentData().toString(), QStringLiteral("de-DE"));
        choose(byName<QComboBox>(&page, QStringLiteral("newEntryStatus")), QStringLiteral("approved"));
        byName<QPushButton>(&page, QStringLiteral("addEntry"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 1, 10000);
        QCOMPARE(page.message(), QStringLiteral("Added “Kaution”"));
        QTRY_COMPARE(byName<QLabel>(&page, QStringLiteral("entryTitle"))->text(), QStringLiteral("Kaution"));
        const QString id = page.entryId();
        QVERIFY(!id.isEmpty());
        QVERIFY(byName<QPushButton>(&page, QStringLiteral("newEntry")));
        auto *list = page.findChild<QListView *>(QStringLiteral("entryList"));
        QCOMPARE(list->model()->rowCount(), 1);
        QCOMPARE(list->model()->index(0, 0).data().toString(), QStringLiteral("German (Germany): Kaution; English (United Kingdom): missing"));
        QVERIFY(label(&page, QStringLiteral("No term yet")));
        QVERIFY(button(&page, QStringLiteral("Add a synonym")) && button(&page, QStringLiteral("Add a term")));

        // Terms: a translation, a synonym, none twice.
        const auto addTerm = [&](const QString &lang, const QString &text, const QString &status) {
            QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("addTerm:%1").arg(lang)));
            byName<QPushButton>(&page, QStringLiteral("addTerm:%1").arg(lang))->click();
            QLineEdit *t = nullptr;
            QTRY_VERIFY((t = byName<QLineEdit>(&page, QStringLiteral("newTerm:%1").arg(lang))));
            t->setText(text);
            QWidget *form = t->parentWidget();
            choose(named<QComboBox>(form, QStringLiteral("Status")), status);
            QVERIFY(button(form, QStringLiteral("Add"))->isEnabled());
            button(form, QStringLiteral("Add"))->click();
        };
        addTerm(QStringLiteral("en-GB"), QStringLiteral("deposit"), QStringLiteral("approved"));
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("en-GB")).size(), 1, 10000);
        QTRY_VERIFY(!page.busy());
        addTerm(QStringLiteral("en-GB"), QStringLiteral("security deposit"), QStringLiteral("proposed"));
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("en-GB")).size(), 2, 10000);
        QTRY_VERIFY(!page.busy());
        QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("addTerm:en-GB")));
        byName<QPushButton>(&page, QStringLiteral("addTerm:en-GB"))->click();
        QTRY_VERIFY(byName<QLineEdit>(&page, QStringLiteral("newTerm:en-GB")));
        byName<QLineEdit>(&page, QStringLiteral("newTerm:en-GB"))->setText(QStringLiteral("Deposit"));
        QTRY_VERIFY(label(&page, QStringLiteral("This entry already has “Deposit”.")));
        QVERIFY(!button(byName<QLineEdit>(&page, QStringLiteral("newTerm:en-GB"))->parentWidget(), QStringLiteral("Add"))->isEnabled());
        button(byName<QLineEdit>(&page, QStringLiteral("newTerm:en-GB"))->parentWidget(), QStringLiteral("Cancel"))->click();
        QCOMPARE(list->model()->index(0, 0).data().toString(), QStringLiteral("German (Germany): Kaution; English (United Kingdom): deposit, security deposit"));

        // A term's status, its text, and taking it out.
        QWidget *second = nullptr;
        QTRY_VERIFY((second = byName<QFrame>(&page, QStringLiteral("term:en-GB:1"))));
        choose(named<QComboBox>(second, QStringLiteral("Status of security deposit")), QStringLiteral("forbidden"));
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("en-GB"))[1].toObject().value("status").toString(), QStringLiteral("forbidden"), 10000);
        QTRY_VERIFY(!page.busy());
        QTRY_VERIFY((second = byName<QFrame>(&page, QStringLiteral("term:en-GB:1"))) && label(second, QStringLiteral("Forbidden")));
        // A forbidden term is not what the list names the entry by.
        QCOMPARE(list->model()->index(0, 0).data().toString(), QStringLiteral("German (Germany): Kaution; English (United Kingdom): deposit, security deposit"));
        named<QPushButton>(second, QStringLiteral("Edit security deposit"))->click();
        QLineEdit *draft = nullptr;
        QTRY_VERIFY((draft = named<QLineEdit>(second, QStringLiteral("Term"))));
        draft->setText(QStringLiteral("deposit"));
        QTRY_VERIFY(label(second, QStringLiteral("This entry already has “deposit” in English (United Kingdom).")));
        QVERIFY(!button(second, QStringLiteral("Save"))->isEnabled());
        draft->setText(QStringLiteral("rent deposit"));
        QVERIFY(button(second, QStringLiteral("Save"))->isEnabled());
        button(second, QStringLiteral("Save"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("en-GB"))[1].toObject().value("text").toString(), QStringLiteral("rent deposit"), 10000);
        QTRY_VERIFY(!page.busy());
        addTerm(QStringLiteral("de-DE"), QStringLiteral("Mietsicherheit"), QStringLiteral("proposed"));
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("de-DE")).size(), 2, 10000);
        QTRY_VERIFY(!page.busy());
        QWidget *synonym = nullptr;
        QTRY_VERIFY((synonym = byName<QFrame>(&page, QStringLiteral("term:de-DE:1"))));
        auto *remove = synonym->findChild<Web::ConfirmButton *>();
        QVERIFY(remove);
        remove->button()->click();
        QVERIFY(label(synonym, QStringLiteral("Remove “Mietsicherheit”?")));
        remove->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("de-DE")).size(), 1, 10000);
        QCOMPARE(page.message(), QStringLiteral("Removed “Mietsicherheit”"));
        QTRY_VERIFY(!page.busy());

        // Fields: one for the whole entry, one for each term.
        const auto addField = [&](const QString &fieldName, int scope) {
            QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("addField")));
            byName<QPushButton>(&page, QStringLiteral("addField"))->click();
            QLineEdit *f = nullptr;
            QTRY_VERIFY((f = byName<QLineEdit>(&page, QStringLiteral("newFieldName"))));
            f->setText(fieldName);
            QWidget *form = f->parentWidget();
            form->findChildren<QRadioButton *>().at(scope)->setChecked(true);
            QVERIFY(button(form, QStringLiteral("Add field"))->isEnabled());
            button(form, QStringLiteral("Add field"))->click();
        };
        addField(QStringLiteral("Definition"), 0);
        QTRY_COMPARE_WITH_TIMEOUT(page.fields().size(), 1, 10000);
        QCOMPARE(page.message(), QStringLiteral("Added field “Definition” to Kanzlei Weber"));
        addField(QStringLiteral("Usage"), 2);
        QTRY_COMPARE_WITH_TIMEOUT(page.fields().size(), 2, 10000);
        byName<QPushButton>(&page, QStringLiteral("addField"))->click();
        QTRY_VERIFY(byName<QLineEdit>(&page, QStringLiteral("newFieldName")));
        byName<QLineEdit>(&page, QStringLiteral("newFieldName"))->setText(QStringLiteral("definition"));
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("newFieldHint"))->text(), QStringLiteral("Kanzlei Weber already has “Definition”."));
        QVERIFY(!button(&page, QStringLiteral("Add field"))->isEnabled());
        byName<QLineEdit>(&page, QStringLiteral("newFieldName"))->setText(QStringLiteral("Usa"));
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("newFieldHint"))->text(), QStringLiteral("Did you mean the existing field “Usage”?"));
        button(byName<QLineEdit>(&page, QStringLiteral("newFieldName"))->parentWidget(), QStringLiteral("Cancel"))->click();

        QPlainTextEdit *definition = nullptr;
        QTRY_VERIFY((definition = named<QPlainTextEdit>(&page, QStringLiteral("Definition"))));
        definition->setFocus();
        definition->setPlainText(QStringLiteral("Sicherheit, die der Mieter leistet"));
        QLineEdit *usage = named<QLineEdit>(byName<QFrame>(&page, QStringLiteral("term:en-GB:0")), QStringLiteral("Usage"));
        QVERIFY(usage);
        usage->setFocus();
        QTRY_COMPARE_WITH_TIMEOUT(TermData::value(page.entry(id).value("fields").toArray(), QStringLiteral("Definition")), QStringLiteral("Sicherheit, die der Mieter leistet"), 10000);
        // A value typed stays where it is: the entry is not drawn again.
        QVERIFY(named<QLineEdit>(byName<QFrame>(&page, QStringLiteral("term:en-GB:0")), QStringLiteral("Usage")) == usage);
        usage->setText(QStringLiteral("tenancy law"));
        emit usage->editingFinished();
        QTRY_COMPARE_WITH_TIMEOUT(TermData::value(TermData::terms(page.entry(id), QStringLiteral("en-GB"))[0].toObject().value("fields").toArray(), QStringLiteral("Usage")),
                                  QStringLiteral("tenancy law"), 10000);
        QTRY_VERIFY(!page.busy());
        // What the page shows is what the core has.
        const ApiResult stored = wait(api->get(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries/") + id));
        QVERIFY(stored.ok());
        QCOMPARE(TermData::forSaving(stored.object()), TermData::forSaving(page.entry(id)));
        QCOMPARE(stored.object().value("version").toInt(), page.entry(id).value("version").toInt());
        QVERIFY(labelWith(&page, QStringLiteral("Last changed ")));

        // The history, newest first.
        auto *tabs = byName<Web::Tabs>(&page, QStringLiteral("entryTabs"));
        QCOMPARE(tabs->tab(1)->text(), QStringLiteral("History (%1)").arg(page.entry(id).value("version").toInt()));
        QSignalSpy history(&page, &TerminologyPage::historyShown);
        tabs->tab(1)->click();
        QTRY_COMPARE_WITH_TIMEOUT(history.size(), 1, 10000);
        QWidget *log = byName<QWidget>(&page, QStringLiteral("history"));
        QVERIFY(log);
        QList<QLabel *> lines;
        for (QLabel *l : log->findChildren<QLabel *>()) {
            if (l->property("role").toString() == QLatin1String("historySummary"))
                lines << l;
        }
        QCOMPARE(lines.size(), page.entry(id).value("version").toInt());
        QCOMPARE(lines.first()->text(), QStringLiteral("Set Usage of “deposit”"));
        QCOMPARE(lines.last()->text(), QStringLiteral("Created"));
        byName<Web::Tabs>(&page, QStringLiteral("entryTabs"))->tab(0)->click();

        // More entries; the search and the status filter.
        const auto addEntry = [&](const QString &de, const QString &en) {
            const QJsonObject e{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", de}, {"status", "approved"}}}}},
                                                          QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", en}, {"status", "proposed"}}}}}}}};
            const ApiResult r = wait(api->post(QStringLiteral("/api/termbases/Kanzlei%20Weber/entries"), QJsonObject{{"entry", e}}));
            return r.ok() ? r.object().value("id").toString() : QString();
        };
        QVERIFY(!addEntry(QStringLiteral("Nebenkosten"), QStringLiteral("service charge")).isEmpty());
        QVERIFY(!addEntry(QStringLiteral("Mietsicherheit"), QStringLiteral("deposit")).isEmpty());
        // What others add is shown without asking.
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 3, 15000);
        QTRY_COMPARE(byName<QLabel>(&page, QStringLiteral("entryCount"))->text(), QStringLiteral("3 entries"));
        QCOMPARE(page.entryId(), id);
        page.setQuery(QStringLiteral("neben"));
        QCOMPARE(page.shown().size(), 1);
        QCOMPARE(list->model()->rowCount(), 1);
        // The entry shown stays; the list only narrows.
        QCOMPARE(page.entryId(), id);
        page.setQuery(QStringLiteral("mieter leistet"));
        QCOMPARE(page.shown().size(), 1);
        page.setQuery(QStringLiteral("nichts davon"));
        QVERIFY(label(&page, QStringLiteral("Nothing matches “nichts davon”")));
        button(&page, QStringLiteral("Clear search"))->click();
        QCOMPARE(list->model()->rowCount(), 3);
        page.setStatus(QStringLiteral("forbidden"));
        QCOMPARE(page.shown().size(), 1);
        page.setStatus(QString());
        list->setCurrentIndex(list->model()->index(1, 0));
        QTRY_COMPARE(byName<QLabel>(&page, QStringLiteral("entryTitle"))->text(), QStringLiteral("Nebenkosten"));
        page.selectEntry(id);
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("entryTitle"))->text(), QStringLiteral("Kaution"));

        // Duplicates: two entries share "deposit".
        QSignalSpy dups(&page, &TerminologyPage::duplicatesShown);
        page.setView(1);
        QTRY_COMPARE_WITH_TIMEOUT(dups.size(), 1, 10000);
        QCOMPARE(page.findChildren<QFrame *>(QStringLiteral("duplicateGroup")).size(), 1);
        choose(byName<QComboBox>(&page, QStringLiteral("duplicateLanguage")), QStringLiteral("de-DE"));
        QTRY_COMPARE_WITH_TIMEOUT(dups.size(), 2, 10000);
        QVERIFY(byName<QLabel>(&page, QStringLiteral("noDuplicates")));
        QVERIFY(label(&page, QStringLiteral("No two entries share a term in German (Germany).")));
        choose(byName<QComboBox>(&page, QStringLiteral("duplicateLanguage")), QString());
        QTRY_COMPARE_WITH_TIMEOUT(dups.size(), 3, 10000);
        QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("notDuplicates")));
        byName<QPushButton>(&page, QStringLiteral("notDuplicates"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(dups.size(), 4, 10000);
        QVERIFY(byName<QLabel>(&page, QStringLiteral("noDuplicates")));
        // Leaving the list and coming back shows them again.
        page.setView(0);
        page.setView(1);
        QTRY_VERIFY_WITH_TIMEOUT(byName<QPushButton>(&page, QStringLiteral("mergeDuplicates")), 10000);
        byName<QPushButton>(&page, QStringLiteral("mergeDuplicates"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 2, 10000);
        QCOMPARE(page.message(), QStringLiteral("Merged 2 entries into one"));
        // The older entry is kept and has the other's terms.
        QCOMPARE(page.entryId(), id);
        QCOMPARE(TermData::terms(page.entry(id), QStringLiteral("de-DE")).size(), 2);
        page.setView(0);

        // Export: all entries as TBX, then those the search shows as CSV.
        ExportDialog *exp = page.exportDialog();
        QVERIFY(exp);
        QCOMPARE(exp->title(), QStringLiteral("Export Kanzlei Weber"));
        QCOMPARE(exp->exportButton()->text(), QStringLiteral("Export 2 entries"));
        QVERIFY(!label(exp, QStringLiteral("<b>Entries</b>")));
        QCOMPARE(exp->fileName(), QStringLiteral("Kanzlei Weber.tbx"));
        const QString tbxFile = m_tmp.filePath(QStringLiteral("out.tbx"));
        exp->exportTo(tbxFile);
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Exported 2 entries to out.tbx"), 10000);
        {
            QFile f(tbxFile);
            QVERIFY(f.open(QIODevice::ReadOnly));
            const QByteArray data = f.readAll();
            QVERIFY(data.contains("<tbx") && data.contains("Nebenkosten") && data.contains("rent deposit"));
        }
        page.setQuery(QStringLiteral("Nebenkosten"));
        exp = page.exportDialog();
        QVERIFY(button(exp, QStringLiteral("All 2 entries")) == nullptr);
        QVERIFY(exp->findChild<QRadioButton *>() != nullptr);
        exp->setFormat(QStringLiteral("csv"));
        exp->setShownOnly(true);
        QCOMPARE(exp->exportButton()->text(), QStringLiteral("Export 1 entry"));
        QCOMPARE(exp->fileName(), QStringLiteral("Kanzlei Weber.csv"));
        const QString csvFile = m_tmp.filePath(QStringLiteral("out.csv"));
        exp->exportTo(csvFile);
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Exported 1 entry to out.csv"), 10000);
        {
            QFile f(csvFile);
            QVERIFY(f.open(QIODevice::ReadOnly));
            const QByteArray data = f.readAll();
            QVERIFY(data.contains("Nebenkosten") && !data.contains("Kaution"));
        }
        page.setQuery(QString());

        // Import a spreadsheet into the termbase: columns, languages, review.
        const QString sheet = m_tmp.filePath(QStringLiteral("Mietrecht-Liste.csv"));
        {
            QFile f(sheet);
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write("Deutsch,Englisch,Quelle\nKaution,deposit; tenancy deposit,BGB\nHausordnung,house rules,\nBetriebskosten,operating costs,BetrKV\n,,ohne Benennung\n");
        }
        ImportDialog *imp = page.importDialog(false);
        QSignalSpy ready(imp, &ImportDialog::ready);
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy(), 10000);
        QCOMPARE(imp->title(), QStringLiteral("Import into Kanzlei Weber"));
        QCOMPARE(imp->stepper()->steps(), QStringList({QStringLiteral("File"), QStringLiteral("Languages"), QStringLiteral("Review")}));
        QVERIFY(byName<QRadioButton>(imp, QStringLiteral("intoThis"))->isChecked());
        QVERIFY(!imp->nextButton()->isVisibleTo(imp));
        imp->chooseFile(sheet);
        QTRY_COMPARE_WITH_TIMEOUT(imp->step(), ImportDialog::Columns, 10000);
        QCOMPARE(imp->stepper()->steps().size(), 4);
        QCOMPARE(imp->stepper()->current(), 1);
        QVERIFY(labelWith(imp, QStringLiteral("Mietrecht-Liste.csv: 4 rows of entries.")));
        // The guess: German and English terms, and a field.
        QCOMPARE(byName<QComboBox>(imp, QStringLiteral("role:0"))->currentData().toString(), QStringLiteral("term"));
        QCOMPARE(byName<QComboBox>(imp, QStringLiteral("lang:0"))->currentData().toString().left(2), QStringLiteral("de"));
        QCOMPARE(byName<QComboBox>(imp, QStringLiteral("role:2"))->currentData().toString(), QStringLiteral("field"));
        QCOMPARE(byName<QLineEdit>(imp, QStringLiteral("field:2"))->text(), QStringLiteral("Quelle"));
        // Without a column of terms there is nothing to import.
        choose(byName<QComboBox>(imp, QStringLiteral("role:0")), QStringLiteral("ignore"));
        choose(byName<QComboBox>(imp, QStringLiteral("role:1")), QStringLiteral("ignore"));
        QTRY_VERIFY(byName<QLabel>(imp, QStringLiteral("columnsProblem")));
        QVERIFY(!imp->nextButton()->isEnabled());
        choose(byName<QComboBox>(imp, QStringLiteral("role:0")), QStringLiteral("term"));
        choose(byName<QComboBox>(imp, QStringLiteral("lang:0")), QStringLiteral("de-DE"));
        choose(byName<QComboBox>(imp, QStringLiteral("role:1")), QStringLiteral("term"));
        choose(byName<QComboBox>(imp, QStringLiteral("lang:1")), QStringLiteral("en-GB"));
        QTRY_VERIFY(imp->nextButton()->isEnabled());
        byName<QLineEdit>(imp, QStringLiteral("field:2"))->setText(QStringLiteral("Source"));
        imp->next();
        QTRY_COMPARE_WITH_TIMEOUT(imp->step(), ImportDialog::LanguagesStep, 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy(), 10000);
        QCOMPARE(byName<QLabel>(imp, QStringLiteral("fileSummary"))->text(), QStringLiteral("CSV: 3 entries, 7 terms, 1 rows without terms skipped."));
        QCOMPARE(imp->fileLanguages(), QStringList({QStringLiteral("de-DE"), QStringLiteral("en-GB")}));
        QCOMPARE(imp->mappedTo(QStringLiteral("en-GB")), QStringLiteral("en-GB"));
        QCOMPARE(byName<QLabel>(imp, QStringLiteral("newFields"))->text(), QStringLiteral("New fields: Source (entry)."));
        // Leaving a language out leaves entries out only if none is left.
        imp->mapLanguage(QStringLiteral("de-DE"), QString());
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy(), 10000);
        QVERIFY(!byName<QFrame>(imp, QStringLiteral("dropped")));
        imp->mapLanguage(QStringLiteral("en-GB"), QString());
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && byName<QFrame>(imp, QStringLiteral("dropped")), 10000);
        QVERIFY(!imp->nextButton()->isEnabled());
        imp->mapLanguage(QStringLiteral("de-DE"), QStringLiteral("de-DE"));
        imp->mapLanguage(QStringLiteral("en-GB"), QStringLiteral("en-GB"));
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && imp->nextButton()->isEnabled(), 10000);
        byName<QRadioButton>(imp, QStringLiteral("fallbackProposed"))->setChecked(true);
        imp->next();
        QTRY_VERIFY_WITH_TIMEOUT(imp->step() == ImportDialog::Review && !imp->busy() && !imp->plan().isEmpty(), 10000);
        // Kaution is there already: its new synonym and source are merged.
        QCOMPARE(imp->plan().value("added").toInt(), 2);
        QCOMPARE(imp->plan().value("merged").toInt(), 1);
        auto *planTable = byName<QTableWidget>(imp, QStringLiteral("plan"));
        QVERIFY(planTable);
        QCOMPARE(planTable->rowCount(), 3);
        QCOMPARE(cell(planTable, 0, 0), QStringLiteral("Kaution, deposit, tenancy deposit"));
        QCOMPARE(cell(planTable, 0, 1), QStringLiteral("Merged"));
        QVERIFY(cell(planTable, 0, 2).startsWith(QStringLiteral("Kaution")));
        QCOMPARE(cell(planTable, 1, 1), QStringLiteral("New"));
        QCOMPARE(cell(planTable, 1, 2), QStringLiteral("none"));
        QVERIFY(byName<QRadioButton>(imp, QStringLiteral("match:term"))->isChecked());
        // Keeping what is there leaves Kaution as it is.
        byName<QRadioButton>(imp, QStringLiteral("onMatch:skip"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && imp->plan().value("merged").toInt() == 0, 10000);
        QCOMPARE(imp->plan().value("skipped").toInt() + imp->plan().value("unchanged").toInt(), 1);
        // Adding everything makes a second Kaution.
        byName<QRadioButton>(imp, QStringLiteral("match:none"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && imp->plan().value("added").toInt() == 3, 10000);
        QVERIFY(!byName<QRadioButton>(imp, QStringLiteral("onMatch:merge")));
        imp->setPolicy(QStringLiteral("term"), QStringLiteral("de-DE"), QStringLiteral("merge"));
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && imp->plan().value("merged").toInt() == 1, 10000);
        QCOMPARE(byName<QComboBox>(imp, QStringLiteral("matchLanguage"))->currentData().toString(), QStringLiteral("de-DE"));
        // Back and forth keeps what was chosen.
        imp->back();
        QCOMPARE(imp->step(), ImportDialog::LanguagesStep);
        QVERIFY(byName<QRadioButton>(imp, QStringLiteral("fallbackProposed"))->isChecked());
        imp->next();
        QTRY_VERIFY_WITH_TIMEOUT(imp->step() == ImportDialog::Review && !imp->busy() && imp->nextButton()->isEnabled(), 10000);
        QCOMPARE(imp->nextButton()->text(), QStringLiteral("Import"));
        imp->next();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Imported Mietrecht-Liste.csv into Kanzlei Weber: 2 added, 1 merged"), 15000);
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 4, 10000);
        QCOMPARE(TermData::terms(page.entry(id), QStringLiteral("en-GB")).size(), 3);
        QCOMPARE(TermData::terms(page.entry(id), QStringLiteral("en-GB"))[2].toObject().value("status").toString(), QStringLiteral("proposed"));
        QCOMPARE(TermData::value(page.entry(id).value("fields").toArray(), QStringLiteral("Source")), QStringLiteral("BGB"));
        QTRY_COMPARE(page.fields().size(), 3);

        // Import a MultiTerm termbase file as a new master.
        const QString sdltb = QFINDTESTDATA("../../term/testdata/orchard.sdltb");
        QVERIFY2(!sdltb.isEmpty(), "the made-up MultiTerm termbase is missing");
        imp = page.importDialog(true);
        QCOMPARE(imp->title(), QStringLiteral("Import a termbase"));
        QVERIFY(!byName<QRadioButton>(imp, QStringLiteral("intoThis")));
        QVERIFY(byName<QLineEdit>(imp, QStringLiteral("newName")));
        imp->chooseFile(sdltb);
        QTRY_VERIFY_WITH_TIMEOUT(imp->step() == ImportDialog::LanguagesStep && !imp->busy(), 15000);
        QCOMPARE(byName<QLineEdit>(imp, QStringLiteral("newName")), nullptr);
        QCOMPARE(byName<QLabel>(imp, QStringLiteral("fileSummary"))->text(), QStringLiteral("MultiTerm termbase: 5 entries, 15 terms."));
        QCOMPARE(imp->fileLanguages().size(), 5);
        // MultiTerm's "EN" goes under a listed English.
        QVERIFY(imp->mappedTo(QStringLiteral("en")).startsWith(QStringLiteral("en")));
        imp->mapLanguage(QStringLiteral("en"), QStringLiteral("en-GB"));
        imp->mapLanguage(QStringLiteral("de"), QStringLiteral("de-DE"));
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy(), 10000);
        imp->next();
        QTRY_VERIFY_WITH_TIMEOUT(imp->step() == ImportDialog::Review && !imp->plan().isEmpty(), 10000);
        QCOMPARE(imp->plan().value("added").toInt(), 5);
        QVERIFY(!byName<QRadioButton>(imp, QStringLiteral("match:term")));
        QCOMPARE(byName<QTableWidget>(imp, QStringLiteral("plan"))->rowCount(), 5);
        imp->next();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("orchard"), 15000);
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 5, 10000);
        QCOMPARE(page.message(), QStringLiteral("Imported orchard.sdltb into orchard: 5 added"));
        QVERIFY(page.languages().contains(QStringLiteral("ja")) || page.languages().contains(QStringLiteral("ja-JP")));
        QTRY_VERIFY(byName<Web::Clickable>(&page, QStringLiteral("tb:%1:orchard").arg(here)));
        // A file that is no termbase says so and stays on the first step.
        const QString notes = m_tmp.filePath(QStringLiteral("notes.xml"));
        {
            QFile f(notes);
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write("<html><body>Nothing here</body></html>");
        }
        imp = page.importDialog(false);
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy(), 10000);
        imp->chooseFile(notes);
        QTRY_VERIFY_WITH_TIMEOUT(!imp->busy() && imp->error().startsWith(QStringLiteral("notes.xml could not be read: ")), 10000);
        QCOMPARE(imp->step(), ImportDialog::File);
        imp->reject();

        // The termbase list: both masters with their counts; open the first.
        Web::Clickable *weber = byName<Web::Clickable>(&page, QStringLiteral("tb:%1:Kanzlei Weber").arg(here));
        QVERIFY(weber && !weber->isCurrent());
        QVERIFY(weber->accessibleName().contains(QStringLiteral("4 entries")));
        QTest::mouseClick(weber, Qt::LeftButton);
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Kanzlei Weber"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 4, 10000);

        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            page.selectEntry(id);
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/terminology.png"));
        }

        // Settings: name, languages, fields, deleting.
        TermbaseSettingsDialog *set = page.settingsDialog();
        QVERIFY(set);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy(), 10000);
        QVERIFY(byName<QWidget>(set, QStringLiteral("language:de-DE")));
        QVERIFY(label(byName<QWidget>(set, QStringLiteral("language:en-GB")), QStringLiteral("6 terms")));
        auto *fieldsTable = byName<QTableWidget>(set, QStringLiteral("fieldsTable"));
        QVERIFY(fieldsTable);
        QCOMPARE(fieldsTable->rowCount(), 3);
        QCOMPARE(cell(fieldsTable, 0, 0), QStringLiteral("Definition"));
        QCOMPARE(cell(fieldsTable, 0, 1), QStringLiteral("Entry"));
        QCOMPARE(cell(fieldsTable, 0, 3), QStringLiteral("1"));
        QCOMPARE(cell(fieldsTable, 1, 1), QStringLiteral("Term"));
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(200);
            set->grab().save(dir + QStringLiteral("/terminology-settings.png"));
        }
        // A new language.
        QVERIFY(!byName<QPushButton>(set, QStringLiteral("addLanguageButton"))->isEnabled());
        byName<QComboBox>(set, QStringLiteral("addLanguage"))->setCurrentIndex(byName<QComboBox>(set, QStringLiteral("addLanguage"))->findData(QStringLiteral("fr-FR")));
        byName<QPushButton>(set, QStringLiteral("addLanguageButton"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QWidget>(set, QStringLiteral("language:fr-FR")), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Added French (France)"), 10000);
        // A field becomes a list to pick from, with its values.
        choose(byName<QComboBox>(set, QStringLiteral("fieldKind:Source")), QStringLiteral("picklist"));
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QLineEdit>(set, QStringLiteral("fieldValues:Source")), 10000);
        byName<QLineEdit>(set, QStringLiteral("fieldValues:Source"))->setText(QStringLiteral("BGB; BetrKV; BGB;"));
        emit byName<QLineEdit>(set, QStringLiteral("fieldValues:Source"))->editingFinished();
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QLineEdit>(set, QStringLiteral("fieldValues:Source")) &&
                                     byName<QLineEdit>(set, QStringLiteral("fieldValues:Source"))->text() == QStringLiteral("BGB; BetrKV"),
                                 10000);
        // Renaming a field renames it in the entries.
        byName<QLineEdit>(set, QStringLiteral("fieldName:Usage"))->setText(QStringLiteral("Register"));
        emit byName<QLineEdit>(set, QStringLiteral("fieldName:Usage"))->editingFinished();
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QLineEdit>(set, QStringLiteral("fieldName:Register")), 10000);
        QCOMPARE(cell(byName<QTableWidget>(set, QStringLiteral("fieldsTable")), 1, 3), QStringLiteral("1"));
        // Removing a field asks first and says what goes with it.
        {
            auto *t = byName<QTableWidget>(set, QStringLiteral("fieldsTable"));
            auto *rm = t->cellWidget(0, 4)->findChild<Web::ConfirmButton *>();
            QVERIFY(rm);
            rm->button()->click();
            QVERIFY(label(t, QStringLiteral("Remove “Definition” and 1 value?")));
            rm->yes()->click();
        }
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Removed the field “Definition”"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QTableWidget>(set, QStringLiteral("fieldsTable")) && byName<QTableWidget>(set, QStringLiteral("fieldsTable"))->rowCount() == 2, 10000);
        // Removing a language takes its terms.
        {
            auto *rm = byName<QWidget>(set, QStringLiteral("language:fr-FR"))->findChild<Web::ConfirmButton *>();
            rm->button()->click();
            QVERIFY(label(set, QStringLiteral("Remove French (France)?")));
            rm->yes()->click();
        }
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Removed French (France)"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && !byName<QWidget>(set, QStringLiteral("language:fr-FR")), 10000);
        // A new name, on leaving the field.
        byName<QLineEdit>(set, QStringLiteral("termbaseName"))->setText(QStringLiteral("Weber Mietrecht"));
        emit byName<QLineEdit>(set, QStringLiteral("termbaseName"))->editingFinished();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Renamed to “Weber Mietrecht”"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy(), 10000);
        byName<QPushButton>(set, QStringLiteral("done"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Weber Mietrecht"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(byName<QLabel>(&page, QStringLiteral("termbaseTitle")) && byName<QLabel>(&page, QStringLiteral("termbaseTitle"))->text() == QStringLiteral("Weber Mietrecht"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 4, 10000);
        // The page shows the renamed field and the list of values.
        QTRY_VERIFY(named<QComboBox>(&page, QStringLiteral("Source")));
        QCOMPARE(named<QComboBox>(&page, QStringLiteral("Source"))->count(), 3);
        QVERIFY(named<QLineEdit>(byName<QFrame>(&page, QStringLiteral("term:en-GB:0")), QStringLiteral("Register")));
        const ApiResult after = wait(api->get(QStringLiteral("/api/termbases/Weber%20Mietrecht/entries/") + id));
        QCOMPARE(TermData::value(TermData::terms(after.object(), QStringLiteral("en-GB"))[0].toObject().value("fields").toArray(), QStringLiteral("Register")), QStringLiteral("tenancy law"));

        // Deleting an entry asks, then shows the next.
        auto *del = byName<Web::ConfirmButton>(&page, QStringLiteral("deleteEntry"));
        QVERIFY(del);
        del->button()->click();
        QVERIFY(label(&page, QStringLiteral("Delete this entry? It can be brought back from Deleted, without its history.")));
        del->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 3, 10000);
        QVERIFY(page.message().startsWith(QStringLiteral("Deleted the entry “")));
        QVERIFY(page.entry(id).isEmpty());
        QTRY_VERIFY(!page.entryId().isEmpty() && page.entryId() != id);

        // Deleting the termbase, from its settings.
        set = page.settingsDialog();
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<Web::ConfirmButton>(set, QStringLiteral("deleteTermbase")), 10000);
        QVERIFY(labelWith(set, QStringLiteral("Deletes 3 entries and their history.")));
        auto *gone = byName<Web::ConfirmButton>(set, QStringLiteral("deleteTermbase"));
        gone->button()->click();
        QVERIFY(label(set, QStringLiteral("Delete Weber Mietrecht for good?")));
        gone->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted Weber Mietrecht"), 10000);
        // The other master is shown instead.
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("orchard"), 10000);
        QTRY_VERIFY(!byName<Web::Clickable>(&page, QStringLiteral("tb:%1:Weber Mietrecht").arg(here)));
        QVERIFY(saved.size() > 0);
    }

    void reviewAndMerge()
    {
        // A project termbase over a master: its changes are submitted,
        // decided one by one or together, and end up in the master.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();
        const auto entry = [](const QString &de, const QString &en) {
            return QJsonObject{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", de}, {"status", "approved"}}}}},
                                                        QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", en}, {"status", "approved"}}}}}}}};
        };
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Recht"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        const ApiResult kaution = wait(api->post(QStringLiteral("/api/termbases/Recht/entries"), QJsonObject{{"entry", entry(QStringLiteral("Kaution"), QStringLiteral("deposit"))}}));
        const ApiResult miete = wait(api->post(QStringLiteral("/api/termbases/Recht/entries"), QJsonObject{{"entry", entry(QStringLiteral("Miete"), QStringLiteral("rent"))}}));
        QVERIFY(kaution.ok() && miete.ok());
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Fall Hofmann"}}}, {"upstreams", QJsonArray{"Recht"}}})).ok());
        // In the project: a new entry, a changed one, and one the master
        // changes in the same place afterwards.
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases/Fall%20Hofmann/entries"), QJsonObject{{"entry", entry(QStringLiteral("Staffelmiete"), QStringLiteral("stepped rent"))}})).ok());
        const auto change = [&](const QString &id, const QString &en, const QString &status) {
            const ApiResult copy = wait(api->post(QStringLiteral("/api/termbases/Fall%20Hofmann/overrides"), QJsonObject{{"master", "Recht"}, {"entry", id}}));
            if (!copy.ok())
                return false;
            QJsonObject e = TermData::forSaving(copy.object());
            e = TermData::withTerms(e, QStringLiteral("en-GB"), QJsonArray{QJsonObject{{"text", en}, {"status", status}}});
            return wait(api->put(QStringLiteral("/api/termbases/Fall%20Hofmann/entries/") + copy.object().value("id").toString(), QJsonObject{{"entry", e}}, copy.object().value("version").toInt())).ok();
        };
        QVERIFY(change(kaution.object().value("id").toString(), QStringLiteral("security deposit"), QStringLiteral("approved")));
        // The project forbids "rent"; the master, meanwhile, only proposes it.
        QVERIFY(change(miete.object().value("id").toString(), QStringLiteral("rent"), QStringLiteral("forbidden")));
        QJsonObject theirs = TermData::forSaving(miete.object());
        theirs = TermData::withTerms(theirs, QStringLiteral("en-GB"), QJsonArray{QJsonObject{{"text", "rent"}, {"status", "proposed"}}});
        QVERIFY(wait(api->put(QStringLiteral("/api/termbases/Recht/entries/") + miete.object().value("id").toString(), QJsonObject{{"entry", theirs}}, 1)).ok());

        TerminologySection section(&m);
        section.resize(1360, 900);
        section.show();
        TerminologyPage *page = section.termbases();
        ReviewPage *review = section.review();
        QTRY_COMPARE_WITH_TIMEOUT(page->termbase(), QStringLiteral("Recht"), 15000);
        // The project termbase is in its own group and says what it is based on.
        Web::Clickable *project = nullptr;
        QTRY_VERIFY((project = byName<Web::Clickable>(page, QStringLiteral("tb:%1:Fall Hofmann").arg(here))));
        QTest::mouseClick(project, Qt::LeftButton);
        QTRY_COMPARE_WITH_TIMEOUT(page->entries().size(), 3, 10000);
        QTRY_VERIFY(byName<QLabel>(page, QStringLiteral("branchNote")));
        QCOMPARE(byName<QLabel>(page, QStringLiteral("branchNote"))->text(), QStringLiteral("Based on Recht. Shows only this project's new and changed entries."));
        QCOMPARE(page->languages(), QStringList({QStringLiteral("de-DE"), QStringLiteral("en-GB")}));
        // A changed entry names its master; a new one is in none yet.
        for (const QJsonObject &e : page->entries()) {
            page->selectEntry(e.value("id").toString());
            if (TermData::head(e, QStringLiteral("de-DE")) == QStringLiteral("Staffelmiete"))
                QVERIFY(label(page, QStringLiteral("Not in a master yet")));
            else
                QVERIFY(label(page, QStringLiteral("Based on Recht")));
        }

        // Submitting opens the request.
        QPushButton *submit = byName<QPushButton>(page, QStringLiteral("submitReview"));
        QVERIFY(submit);
        QCOMPARE(submit->text(), QStringLiteral("Review and merge"));
        QCOMPARE(byName<QLineEdit>(page, QStringLiteral("reviewTitle"))->placeholderText(), QStringLiteral("Fall Hofmann into Recht"));
        QSignalSpy shown(review, &ReviewPage::loaded);
        submit->click();
        QTRY_VERIFY_WITH_TIMEOUT(section.reviewing() && !shown.isEmpty() && review->items().size() == 3, 15000);
        QCOMPARE(page->message(), QStringLiteral("Submitted Fall Hofmann to Recht for review"));
        QCOMPARE(review->mergeRequest().value("title").toString(), QStringLiteral("Fall Hofmann into Recht"));
        QVERIFY(byName<QLabel>(review, QStringLiteral("about"))->text().startsWith(QStringLiteral("Fall Hofmann into Recht. Submitted by ")));
        QCOMPARE(byName<QLabel>(review, QStringLiteral("summary"))->text(), QStringLiteral("3 entries: 3 waiting; 1 conflict with the master."));
        QCOMPARE(byName<Web::Tabs>(review, QStringLiteral("views"))->tab(0)->text(), QStringLiteral("Waiting (3)"));
        QVERIFY(label(review, QStringLiteral("<b>New entry</b>")) && label(review, QStringLiteral("<b>Changed entry</b>")));
        QVERIFY(label(review, QStringLiteral("<b>The master changed the same places</b>")));
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            section.grab().save(dir + QStringLiteral("/review.png"));
        }

        // All without conflicts at once: the conflict stays for a decision.
        byName<QPushButton>(review, QStringLiteral("approveAll"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(review->message().startsWith(QStringLiteral("Approved all waiting entries without conflicts: 2 entries")), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(review->items().size() == 1 && review->progress().value("states").toObject().value("waiting").toInt() == 1, 10000);
        // The conflict offers both ways out and names what each puts into
        // the master; there is no plain approval for it.
        QPushButton *useProject = nullptr;
        QTRY_VERIFY((useProject = byName<QPushButton>(review, QStringLiteral("useProject"))));
        QCOMPARE(useProject->text(), QStringLiteral("Use project version: “forbidden”"));
        QCOMPARE(byName<QPushButton>(review, QStringLiteral("keepMaster"))->text(), QStringLiteral("Keep master version: “proposed”"));
        QVERIFY(!byName<QPushButton>(review, QStringLiteral("approve")));
        QVERIFY(label(review, QStringLiteral("Choose which of the two goes into Recht. What only one side changed is merged either way.")));
        QVERIFY(byName<QPushButton>(review, QStringLiteral("postpone")) && byName<QPushButton>(review, QStringLiteral("reject")));
        const auto rentStatus = [&] {
            const ApiResult r = wait(api->get(QStringLiteral("/api/termbases/Recht/entries/") + miete.object().value("id").toString()));
            return TermData::terms(r.object(), QStringLiteral("en-GB")).first().toObject().value("status").toString();
        };
        QCOMPARE(rentStatus(), QStringLiteral("proposed"));
        useProject->click();
        QTRY_COMPARE_WITH_TIMEOUT(review->message(), QStringLiteral("Used the project version (“forbidden”): 1 entry"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(review->items().isEmpty() && byName<QLabel>(review, QStringLiteral("none")), 10000);
        QCOMPARE(byName<QLabel>(review, QStringLiteral("none"))->text(), QStringLiteral("Nothing is waiting."));
        // The master now forbids the term, as the project did.
        QCOMPARE(rentStatus(), QStringLiteral("forbidden"));
        byName<Web::Tabs>(review, QStringLiteral("views"))->tab(1)->click();
        QTRY_VERIFY_WITH_TIMEOUT(review->view() == 1 && review->items().size() == 3, 10000);
        QVERIFY(label(review, QStringLiteral("Approved with edits")) && label(review, QStringLiteral("Approved")));
        QVERIFY(!byName<QPushButton>(review, QStringLiteral("approve")) && !byName<QPushButton>(review, QStringLiteral("useProject")));

        // The master has what was approved.
        const ApiResult master = wait(api->get(QStringLiteral("/api/termbases/Recht/entries"), [] {
            QUrlQuery q;
            q.addQueryItem(QStringLiteral("limit"), QStringLiteral("100"));
            return q;
        }()));
        QStringList english;
        for (const QJsonValue &v : master.object().value("entries").toArray()) {
            for (const QJsonValue &t : TermData::terms(v.toObject(), QStringLiteral("en-GB")))
                english << t.toObject().value("text").toString() + QLatin1Char('/') + t.toObject().value("status").toString();
        }
        QVERIFY2(english.contains(QStringLiteral("stepped rent/approved")) && english.contains(QStringLiteral("security deposit/approved")) && english.contains(QStringLiteral("rent/forbidden")),
                 qPrintable(english.join(QStringLiteral(", "))));

        // Closing asks, and says what is still open.
        auto *close = byName<Web::ConfirmButton>(review, QStringLiteral("close"));
        QVERIFY(close);
        close->button()->click();
        QVERIFY(label(review, QStringLiteral("Close the request?")));
        close->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(review->message(), QStringLiteral("Closed the request"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(review->mergeRequest().value("state").toString() == QStringLiteral("closed"), 10000);
        QTRY_VERIFY(!byName<QPushButton>(review, QStringLiteral("approveAll")));
        QVERIFY(byName<QLabel>(review, QStringLiteral("about"))->text().contains(QStringLiteral("Closed on ")));

        // Back at the termbase, the request is listed and opens again.
        byName<QPushButton>(review, QStringLiteral("back"))->click();
        QVERIFY(!section.reviewing());
        QTRY_COMPARE_WITH_TIMEOUT(page->termbase(), QStringLiteral("Fall Hofmann"), 10000);
        const int id = review->request();
        QPushButton *listed = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((listed = byName<QPushButton>(page, QStringLiteral("mergeRequest:%1").arg(id))), 10000);
        QCOMPARE(listed->text(), QStringLiteral("Fall Hofmann into Recht"));
        QTRY_VERIFY_WITH_TIMEOUT(labelWith(page, QStringLiteral("Fall Hofmann into Recht, closed ")), 10000);
        listed = byName<QPushButton>(page, QStringLiteral("mergeRequest:%1").arg(id));
        listed->click();
        QTRY_VERIFY_WITH_TIMEOUT(section.reviewing() && review->mergeRequest().value("state").toString() == QStringLiteral("closed"), 10000);

        // A request that is not there says so.
        section.openReview(here, 99999);
        QTRY_VERIFY_WITH_TIMEOUT(byName<QLabel>(review, QStringLiteral("state")) && byName<QLabel>(review, QStringLiteral("state"))->property("role").toString() == QStringLiteral("error"), 10000);
    }

    void lifecycle()
    {
        // What was changed or deleted can be taken back: an earlier version
        // of an entry, a deleted entry, and, in a project's termbase, what
        // was done with the master's entries.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();
        const auto entry = [](const QString &de, const QString &en) {
            return QJsonObject{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", de}, {"status", "approved"}}}}},
                                                        QJsonObject{{"lang", "en-GB"}, {"terms", QJsonArray{QJsonObject{{"text", en}, {"status", "approved"}}}}}}}};
        };
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Recht"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        const ApiResult kaution = wait(api->post(QStringLiteral("/api/termbases/Recht/entries"), QJsonObject{{"entry", entry(QStringLiteral("Kaution"), QStringLiteral("deposit"))}}));
        const ApiResult miete = wait(api->post(QStringLiteral("/api/termbases/Recht/entries"), QJsonObject{{"entry", entry(QStringLiteral("Miete"), QStringLiteral("rent"))}}));
        QVERIFY(kaution.ok() && miete.ok());
        const QString kid = kaution.object().value("id").toString(), mid = miete.object().value("id").toString();
        // "deposit" became "security deposit".
        QJsonObject changed = TermData::withTerms(TermData::forSaving(kaution.object()), QStringLiteral("en-GB"), QJsonArray{QJsonObject{{"text", "security deposit"}, {"status", "approved"}}});
        QVERIFY(wait(api->put(QStringLiteral("/api/termbases/Recht/entries/") + kid, QJsonObject{{"entry", changed}}, 1)).ok());
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Fall Hofmann"}}}, {"upstreams", QJsonArray{"Recht"}}})).ok());

        TerminologyPage page(&m);
        page.resize(1360, 900);
        page.show();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Recht"), 15000);
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 2, 10000);
        auto *views = byName<Web::Tabs>(&page, QStringLiteral("entryViews"));
        QVERIFY(views);
        // A master has no master of its own.
        QVERIFY(!there(views->tab(3), &page));

        // The history says what each version held; an earlier one is restored.
        page.selectEntry(kid);
        QSignalSpy history(&page, &TerminologyPage::historyShown);
        byName<Web::Tabs>(&page, QStringLiteral("entryTabs"))->tab(1)->click();
        QTRY_COMPARE_WITH_TIMEOUT(history.size(), 1, 10000);
        QWidget *log = byName<QWidget>(&page, QStringLiteral("history"));
        QVERIFY(labelWith(log, QStringLiteral("en-GB: security deposit")));
        QVERIFY(labelWith(log, QStringLiteral("en-GB: deposit")));
        // The version the entry is at has nothing to restore.
        QVERIFY(!byName<Web::ConfirmButton>(&page, QStringLiteral("restore:2")));
        auto *restore = byName<Web::ConfirmButton>(&page, QStringLiteral("restore:1"));
        QVERIFY(restore);
        restore->button()->click();
        QVERIFY(label(&page, QStringLiteral("Set the entry back to this version? What it is now stays in the history.")));
        shot(&page, QStringLiteral("terminology-restore"));
        restore = byName<Web::ConfirmButton>(&page, QStringLiteral("restore:1"));
        QVERIFY(restore && restore->asking());
        restore->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entry(kid).value("version").toInt(), 3, 10000);
        QVERIFY(page.message().startsWith(QStringLiteral("Restored the version of ")));
        QCOMPARE(TermData::terms(page.entry(kid), QStringLiteral("en-GB"))[0].toObject().value("text").toString(), QStringLiteral("deposit"));
        QCOMPARE(wait(api->get(QStringLiteral("/api/termbases/Recht/entries/") + kid + QStringLiteral("/history"))).array().last().toObject().value("summary").toString(),
                 QStringLiteral("Reverted to version 1"));

        // Nothing is deleted yet.
        QSignalSpy deleted(&page, &TerminologyPage::deletedShown);
        views->tab(2)->click();
        QTRY_COMPARE_WITH_TIMEOUT(deleted.size(), 1, 10000);
        QVERIFY(byName<QLabel>(&page, QStringLiteral("noDeleted")));
        views->tab(0)->click();
        // A deleted entry is listed, and brought back under its ID.
        page.selectEntry(mid);
        auto *del = byName<Web::ConfirmButton>(&page, QStringLiteral("deleteEntry"));
        QVERIFY(del);
        del->button()->click();
        del->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 1, 10000);
        views->tab(2)->click();
        QTRY_VERIFY_WITH_TIMEOUT(byName<QPushButton>(&page, QStringLiteral("bringBack:%1").arg(mid)), 10000);
        QCOMPARE(page.deleted().size(), 1);
        QVERIFY(label(&page, QStringLiteral("Miete")) && label(&page, QStringLiteral("rent")));
        QVERIFY(labelWith(&page, QStringLiteral("Deleted ")));
        shot(&page, QStringLiteral("terminology-deleted"));
        byName<QPushButton>(&page, QStringLiteral("bringBack:%1").arg(mid))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 2, 10000);
        QCOMPARE(page.message(), QStringLiteral("Brought back “Miete”"));
        QCOMPARE(page.entryId(), mid);
        QTRY_VERIFY_WITH_TIMEOUT(byName<QLabel>(&page, QStringLiteral("noDeleted")), 10000);
        QVERIFY(wait(api->get(QStringLiteral("/api/termbases/Recht/deleted"))).array().isEmpty());
        views->tab(0)->click();

        // The project termbase shows the master's entries in a view of their own.
        Web::Clickable *project = nullptr;
        QTRY_VERIFY((project = byName<Web::Clickable>(&page, QStringLiteral("tb:%1:Fall Hofmann").arg(here))));
        QTest::mouseClick(project, Qt::LeftButton);
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Fall Hofmann"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page.masterEntries().size(), 2, 10000);
        QVERIFY(page.entries().isEmpty());
        QVERIFY(there(views->tab(3), &page));
        QCOMPARE(views->tab(3)->text(), QStringLiteral("Master (2)"));
        QSignalSpy master(&page, &TerminologyPage::masterShown);
        views->tab(3)->click();
        QTRY_VERIFY_WITH_TIMEOUT(!master.isEmpty(), 10000);
        QVERIFY(byName<Web::Clickable>(&page, QStringLiteral("master:Recht:%1").arg(kid)));
        page.selectMasterEntry(QStringLiteral("Recht"), mid);
        QTRY_VERIFY(byName<QLabel>(&page, QStringLiteral("masterEntryTitle")));
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("masterEntryTitle"))->text(), QStringLiteral("Miete"));
        // The master's entry is read here, not changed.
        QVERIFY(!byName<Web::ConfirmButton>(&page, QStringLiteral("deleteEntry")));
        shot(&page, QStringLiteral("terminology-master"));

        // Left out of the project, and brought back.
        auto *leave = byName<Web::ConfirmButton>(&page, QStringLiteral("leaveOut"));
        QVERIFY(leave);
        leave->button()->click();
        QVERIFY(label(&page, QStringLiteral("Leave this entry out of the project? Merging asks the master's reviewers to delete it there.")));
        leave->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.masterEntries().size(), 1, 10000);
        QCOMPARE(page.message(), QStringLiteral("Left the entry out of the project"));
        QPushButton *back = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((back = byName<QPushButton>(&page, QStringLiteral("restoreMaster:Recht:%1").arg(mid))), 10000);
        QCOMPARE(page.removals().size(), 1);
        QVERIFY(label(&page, QStringLiteral("Left out of this project")));
        // The entry left out is named by its term.
        QTRY_VERIFY_WITH_TIMEOUT(label(back->parentWidget(), QStringLiteral("Miete")), 10000);
        shot(&page, QStringLiteral("terminology-left-out"));
        back = byName<QPushButton>(&page, QStringLiteral("restoreMaster:Recht:%1").arg(mid));
        QVERIFY(back);
        back->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.masterEntries().size(), 2, 10000);
        QCOMPARE(page.message(), QStringLiteral("Brought the entry back into the project"));
        QTRY_VERIFY_WITH_TIMEOUT(page.removals().isEmpty() && !byName<QPushButton>(&page, QStringLiteral("restoreMaster:Recht:%1").arg(mid)), 10000);

        // Changed for this project only: the project's copy opens to be edited.
        page.selectMasterEntry(QStringLiteral("Recht"), kid);
        QPushButton *change = nullptr;
        QTRY_VERIFY((change = byName<QPushButton>(&page, QStringLiteral("changeInProject"))));
        change->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 1, 10000);
        QCOMPARE(page.message(), QStringLiteral("Took the entry into the project"));
        QCOMPARE(page.view(), 0);
        QTRY_COMPARE_WITH_TIMEOUT(page.entryId(), page.entries().first().value("id").toString(), 10000);
        QTRY_VERIFY(label(&page, QStringLiteral("Based on Recht")));
        QCOMPARE(page.masterEntries().size(), 1);
        QCOMPARE(views->tab(3)->text(), QStringLiteral("Master (1)"));
        QVERIFY(byName<Web::ConfirmButton>(&page, QStringLiteral("deleteEntry")));
    }

    void managing()
    {
        // A project termbase made by hand, the masters it is based on, how
        // a master is reviewed, and deleting both kinds.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();
        const QJsonObject note{{"name", "Note"}, {"level", "entry"}, {"kind", "text"}};
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Recht"}, {"languages", QJsonArray{"de-DE", "en-GB"}}, {"fields", QJsonArray{note}}}}})).ok());
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Steuer"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());

        TerminologyPage page(&m);
        page.resize(1360, 900);
        page.show();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Recht"), 15000);
        // On this computer everything is one's own.
        QVERIFY(page.may("manage"));

        // A project termbase, based on a master chosen in the form.
        QPushButton *fresh = nullptr;
        QTRY_VERIFY((fresh = byName<QPushButton>(&page, QStringLiteral("newProjectTermbase"))));
        fresh->click();
        QTRY_VERIFY(byName<QLineEdit>(&page, QStringLiteral("newMasterName")));
        QPushButton *create = byName<QPushButton>(&page, QStringLiteral("createProjectTermbase"));
        QVERIFY(create && !create->isEnabled());
        byName<QLineEdit>(&page, QStringLiteral("newMasterName"))->setText(QStringLiteral("Fall Weber"));
        // Without a master it would be a master itself.
        QVERIFY(!create->isEnabled());
        QVERIFY(byName<QCheckBox>(&page, QStringLiteral("basedOn:Steuer")));
        byName<QCheckBox>(&page, QStringLiteral("basedOn:Recht"))->click();
        QVERIFY(create->isEnabled());
        shot(&page, QStringLiteral("terminology-new-project-termbase"));
        byName<QPushButton>(&page, QStringLiteral("createProjectTermbase"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Fall Weber"), 10000);
        QCOMPARE(page.message(), QStringLiteral("Created project termbase “Fall Weber”"));
        QTRY_VERIFY(byName<Web::Clickable>(&page, QStringLiteral("tb:%1:Fall Weber").arg(here)));
        QTRY_COMPARE_WITH_TIMEOUT(page.detail().value("kind").toString(), QStringLiteral("project"), 10000);
        QCOMPARE(page.languages(), QStringList({QStringLiteral("de-DE"), QStringLiteral("en-GB")}));

        // Its settings: also based on the second master, and back.
        TermbaseSettingsDialog *set = page.settingsDialog();
        QVERIFY(set);
        QCheckBox *recht = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && (recht = byName<QCheckBox>(set, QStringLiteral("basedOn:Recht"))) && byName<QCheckBox>(set, QStringLiteral("basedOn:Steuer")), 10000);
        // The only master cannot be taken away.
        QVERIFY(recht->isChecked() && !recht->isEnabled());
        QVERIFY(!byName<QCheckBox>(set, QStringLiteral("ruleIdentical")));
        byName<QCheckBox>(set, QStringLiteral("basedOn:Steuer"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Now also based on Steuer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QCheckBox>(set, QStringLiteral("basedOn:Recht")) && byName<QCheckBox>(set, QStringLiteral("basedOn:Recht"))->isEnabled(), 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/termbases/Fall%20Weber"))).object().value("upstreams").toArray(), QJsonArray({"Recht", "Steuer"}));
        byName<QScrollArea>(set, QStringLiteral("settingsScroll"))->ensureWidgetVisible(byName<QCheckBox>(set, QStringLiteral("basedOn:Steuer")), 0, 200);
        shot(set, QStringLiteral("termbase-settings-project"));
        byName<QCheckBox>(set, QStringLiteral("basedOn:Steuer"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("No longer based on Steuer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy(), 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/termbases/Fall%20Weber"))).object().value("upstreams").toArray(), QJsonArray({"Recht"}));
        set->accept();

        // The master: it says what is based on it, and so stays.
        page.open(here, QStringLiteral("Recht"));
        QTRY_VERIFY_WITH_TIMEOUT(page.termbase() == QStringLiteral("Recht") && page.detail().value("projects").toArray().size() == 1, 10000);
        set = page.settingsDialog();
        QVERIFY(set);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QCheckBox>(set, QStringLiteral("ruleIdentical")), 10000);
        QVERIFY(byName<QLabel>(set, QStringLiteral("basedOnIt")));
        QCOMPARE(byName<QLabel>(set, QStringLiteral("basedOnIt"))->text(), QStringLiteral("Fall Weber is based on this termbase. Delete it, or base it on another master, first."));
        QVERIFY(!byName<Web::ConfirmButton>(set, QStringLiteral("deleteTermbase")));
        QVERIFY(!byName<QCheckBox>(set, QStringLiteral("basedOn:Steuer")));

        // How it is reviewed: what the master has already, a field that
        // needs no approval, and an author who is trusted.
        QVERIFY(byName<QCheckBox>(set, QStringLiteral("ruleIdentical"))->isChecked());
        QVERIFY(byName<QLabel>(set, QStringLiteral("noTrusted")));
        byName<QCheckBox>(set, QStringLiteral("ruleIdentical"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Entries the master already has wait for review, too"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QCheckBox>(set, QStringLiteral("minorField:Note")), 10000);
        byName<QCheckBox>(set, QStringLiteral("minorField:Note"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Changes to Note only are merged without review"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QPushButton>(set, QStringLiteral("trust")), 10000);
        // Nobody named: nothing is saved.
        byName<QPushButton>(set, QStringLiteral("trust"))->click();
        QVERIFY(label(set, QStringLiteral("Say whose entries to trust.")));
        auto *who = byName<QLineEdit>(set, QStringLiteral("trustAuthor"));
        QVERIFY(who);
        QTest::keyClicks(who, QStringLiteral("Eva"));
        choose(byName<QComboBox>(set, QStringLiteral("trustLanguage")), QStringLiteral("en-GB"));
        byName<QPushButton>(set, QStringLiteral("trust"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("The entries of Eva are approved without review"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QPushButton>(set, QStringLiteral("untrust:Eva")), 10000);
        QVERIFY(label(set, QStringLiteral("entries with terms in English (United Kingdom)")));
        QJsonObject rules = wait(api->get(QStringLiteral("/api/termbases/Recht/review-rules"))).object();
        QCOMPARE(rules.value("keepIdentical").toBool(), true);
        QCOMPARE(rules.value("minorFields").toArray(), QJsonArray({"Note"}));
        QCOMPARE(rules.value("trusted").toArray().size(), 1);
        QCOMPARE(rules.value("trusted").toArray()[0].toObject().value("scope").toObject().value("langs").toArray(), QJsonArray({"en-GB"}));
        byName<QScrollArea>(set, QStringLiteral("settingsScroll"))->ensureWidgetVisible(byName<QPushButton>(set, QStringLiteral("trust")), 0, 80);
        shot(set, QStringLiteral("termbase-settings-review"));
        byName<QPushButton>(set, QStringLiteral("untrust:Eva"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("The entries of Eva wait for review again"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<QLabel>(set, QStringLiteral("noTrusted")), 10000);
        QVERIFY(wait(api->get(QStringLiteral("/api/termbases/Recht/review-rules"))).object().value("trusted").toArray().isEmpty());
        set->accept();

        // The project termbase is deleted from its settings; then the master can be.
        page.open(here, QStringLiteral("Fall Weber"));
        QTRY_VERIFY_WITH_TIMEOUT(page.termbase() == QStringLiteral("Fall Weber") && page.detail().value("kind").toString() == QStringLiteral("project"), 10000);
        set = page.settingsDialog();
        QVERIFY(set);
        Web::ConfirmButton *gone = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && (gone = byName<Web::ConfirmButton>(set, QStringLiteral("deleteTermbase"))), 10000);
        QVERIFY(labelWith(set, QStringLiteral("The masters keep theirs.")));
        gone->button()->click();
        gone->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted Fall Weber"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(page.termbase(), QStringLiteral("Recht"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(page.detail().value("projects").toArray().isEmpty(), 10000);
        set = page.settingsDialog();
        QVERIFY(set);
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && byName<Web::ConfirmButton>(set, QStringLiteral("deleteTermbase")), 10000);
        QVERIFY(!byName<QLabel>(set, QStringLiteral("basedOnIt")));
        set->accept();
    }

    void savingMeetsAnotherChange()
    {
        // Someone else saves the entry first: the change here is refused,
        // said, and what the core has is shown.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Shared"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}})).ok());
        const QJsonObject e{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Miete"}, {"status", "approved"}}}}}}}};
        const ApiResult made = wait(api->post(QStringLiteral("/api/termbases/Shared/entries"), QJsonObject{{"entry", e}}));
        QVERIFY(made.ok());
        const QString id = made.object().value("id").toString();

        TerminologyPage page(&m);
        page.resize(1280, 800);
        page.show();
        QTRY_COMPARE_WITH_TIMEOUT(page.entries().size(), 1, 15000);
        QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("addTerm:en-GB")));
        // The page still has version 1 when the entry changes underneath.
        QJsonObject theirs = TermData::forSaving(made.object());
        theirs = TermData::withTerms(theirs, QStringLiteral("en-GB"), QJsonArray{QJsonObject{{"text", "rent"}, {"status", "approved"}}});
        m.local()->feed()->blockSignals(true);
        QVERIFY(wait(api->put(QStringLiteral("/api/termbases/Shared/entries/") + id, QJsonObject{{"entry", theirs}, {"summary", "Added by a colleague"}}, 1)).ok());
        byName<QPushButton>(&page, QStringLiteral("addTerm:en-GB"))->click();
        QLineEdit *t = nullptr;
        QTRY_VERIFY((t = byName<QLineEdit>(&page, QStringLiteral("newTerm:en-GB"))));
        t->setText(QStringLiteral("rental"));
        button(t->parentWidget(), QStringLiteral("Add"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(page.message().startsWith(QStringLiteral("The entry was not saved: ")), 10000);
        m.local()->feed()->blockSignals(false);
        // Their term is shown, not the one refused.
        QTRY_VERIFY_WITH_TIMEOUT(TermData::terms(page.entry(id), QStringLiteral("en-GB")).size() == 1 &&
                                     TermData::terms(page.entry(id), QStringLiteral("en-GB"))[0].toObject().value("text").toString() == QStringLiteral("rent"),
                                 10000);
        QCOMPARE(page.entry(id).value("version").toInt(), 2);
        QTRY_VERIFY(!page.busy());
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestTerminology)
#include "tst_terminology.moc"
