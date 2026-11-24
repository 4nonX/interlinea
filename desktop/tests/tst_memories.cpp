// The Memories page against the app's own core: new memories with and
// without a TMX file, TMX in and out, what uses a memory, the project
// memories, and deleting.
#include "alignmentdialog.h"
#include "apiclient.h"
#include "connectionmanager.h"
#include "languages.h"
#include "memoriespage.h"
#include "targeteditor.h"
#include "testcore.h"
#include "theme.h"
#include "webui.h"

#include <QApplication>
#include <QFile>
#include <QLabel>
#include <QLineEdit>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTableWidget>
#include <QTemporaryDir>
#include <QTest>
#include <QTextCursor>

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
        if (w->accessibleName() == accessibleName && !w->isHidden())
            return w;
    }
    return nullptr;
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>()) {
        if (b->text() == text && !b->isHidden())
            return b;
    }
    return nullptr;
}

// A shown label with exactly this text.
static QLabel *label(QWidget *root, const QString &text)
{
    for (QLabel *l : root->findChildren<QLabel *>()) {
        if (l->text() == text && l->isVisibleTo(root))
            return l;
    }
    return nullptr;
}

// What a cell says to a screen reader, which is what it shows.
static QString cell(QTableWidget *table, int row, int column)
{
    return table && table->item(row, column) ? table->item(row, column)->data(Qt::AccessibleTextRole).toString() : QString();
}

static QByteArray tmx()
{
    return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
           "<tmx version=\"1.4\"><header creationtool=\"test\" creationtoolversion=\"1\" datatype=\"plaintext\" segtype=\"sentence\" "
           "adminlang=\"en\" srclang=\"de-DE\" o-tmf=\"test\"/><body>"
           "<tu><tuv xml:lang=\"de-DE\"><seg>Der Mieter zahlt die Miete.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The tenant pays the rent.</seg></tuv></tu>"
           "<tu><tuv xml:lang=\"de-DE\"><seg>Die Kaution ist fällig.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The deposit is due.</seg></tuv></tu>"
           "</body></tmx>\n";
}

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
        Theme::apply(*qApp);
    }

    void confirmButton()
    {
        // It asks first, and acts only on the confirming button.
        Web::ConfirmButton b(QStringLiteral("Delete"), QStringLiteral("Delete it?"), QStringLiteral("Delete"), nullptr);
        b.show();
        QSignalSpy confirmed(&b, &Web::ConfirmButton::confirmed);
        QVERIFY(!b.asking());
        b.button()->click();
        QVERIFY(b.asking());
        QVERIFY(b.button()->isHidden());
        button(&b, QStringLiteral("Cancel"))->click();
        QVERIFY(!b.asking());
        QCOMPARE(confirmed.size(), 0);
        b.button()->click();
        b.yes()->click();
        QCOMPARE(confirmed.size(), 1);
        QVERIFY(!b.asking());
        b.setAvailable(false);
        QVERIFY(!b.button()->isEnabled());
    }

    void memoriesOnThisComputer()
    {
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();

        MemoriesPage page(&m);
        page.resize(1280, 900);
        page.show();
        QTRY_VERIFY_WITH_TIMEOUT(label(&page, QStringLiteral("No memory yet.")), 10000);
        QVERIFY(!label(&page, QStringLiteral("Project memories")));

        const QString file = m_tmp.filePath(QStringLiteral("Weber.tmx"));
        {
            QFile f(file);
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write(tmx());
        }
        const QString notTmx = m_tmp.filePath(QStringLiteral("Notizen.txt"));
        {
            QFile f(notTmx);
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write("Keine TMX-Datei.\n");
        }

        // A new memory for a client, filled from a TMX file.
        QTRY_VERIFY(button(&page, QStringLiteral("New memory")));
        button(&page, QStringLiteral("New memory"))->click();
        QPushButton *create = nullptr;
        QTRY_VERIFY((create = button(&page, QStringLiteral("Create memory"))) && named<QLineEdit>(&page, QStringLiteral("Name")) &&
                    named<QLineEdit>(&page, QStringLiteral("Client")));
        QVERIFY(!create->isEnabled());
        QTRY_VERIFY(label(&page, QStringLiteral("German (Germany) to English (United Kingdom). Projects with these languages can use it.")));
        named<QLineEdit>(&page, QStringLiteral("Name"))->setText(QStringLiteral("Weber DE-EN"));
        named<QLineEdit>(&page, QStringLiteral("Client"))->setText(QStringLiteral("Kanzlei Weber"));
        QVERIFY(create->isEnabled());
        page.setNewMemoryFile(file);
        QVERIFY(label(&page, QStringLiteral("Weber.tmx")));
        create->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Created the memory Weber DE-EN with 2 units"), 10000);
        QTableWidget *main = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && main->rowCount() == 1, 10000);
        QVERIFY2(cell(main, 0, 0).startsWith(QStringLiteral("Weber DE-EN, Kanzlei Weber, created ")), qPrintable(cell(main, 0, 0)));
        QCOMPARE(cell(main, 0, 1), QStringLiteral("German (Germany) to English (United Kingdom)"));
        QCOMPARE(cell(main, 0, 2), QStringLiteral("2"));
        QCOMPARE(cell(main, 0, 3), QStringLiteral("No project"));
        // The form is put away again.
        QTRY_VERIFY(!button(&page, QStringLiteral("Create memory")) && button(&page, QStringLiteral("New memory")));

        // A name that is taken, and equal languages, are said before anything is sent.
        button(&page, QStringLiteral("New memory"))->click();
        QLineEdit *name = nullptr;
        QTRY_VERIFY((create = button(&page, QStringLiteral("Create memory"))) && (name = named<QLineEdit>(&page, QStringLiteral("Name"))) &&
                    named<LanguageCombo>(&page, QStringLiteral("Target language")));
        name->setText(QStringLiteral("weber de-en"));
        QVERIFY(!create->isEnabled());
        QTRY_VERIFY(label(&page, QStringLiteral("A memory with this name exists already.")));
        name->setText(QStringLiteral("Leer"));
        QVERIFY(create->isEnabled());
        named<LanguageCombo>(&page, QStringLiteral("Target language"))->setLanguage(QStringLiteral("de-DE"));
        QVERIFY(!create->isEnabled());
        QVERIFY(label(&page, QStringLiteral("Source and target language must differ.")));
        named<LanguageCombo>(&page, QStringLiteral("Target language"))->setLanguage(QStringLiteral("en-GB"));

        // A file that isn't TMX makes no memory.
        page.setNewMemoryFile(notTmx);
        create->click();
        QLabel *error = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            for (QLabel *l : page.findChildren<QLabel *>()) {
                if (l->text().startsWith(QStringLiteral("Could not read Notizen.txt: ")) && !l->isHidden())
                    error = l;
            }
            return error != nullptr;
        }(), 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/memories"))).array().size(), 1);
        QTRY_VERIFY_WITH_TIMEOUT(create->isEnabled(), 5000);

        // Without a file it is made empty.
        page.setNewMemoryFile(QString());
        QVERIFY(label(&page, QStringLiteral("No file chosen")));
        create->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Created the memory Leer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && main->rowCount() == 2, 10000);
        QCOMPARE(cell(main, 0, 0).section(QLatin1Char(','), 0, 0), QStringLiteral("Leer"));
        QCOMPARE(cell(main, 0, 2), QStringLiteral("0"));
        // Nothing to export from an empty memory. (A table just rebuilt is
        // shown with the next turn of the event loop, hence the waiting.)
        QPushButton *exportEmpty = nullptr, *exportFilled = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((exportEmpty = named<QPushButton>(&page, QStringLiteral("Export Leer as TMX"))) &&
                                     (exportFilled = named<QPushButton>(&page, QStringLiteral("Export Weber DE-EN as TMX"))),
                                 10000);
        QVERIFY(!exportEmpty->isEnabled());
        QVERIFY(exportFilled->isEnabled());

        // TMX in, and out again.
        page.importFile(here, QStringLiteral("Leer"), file);
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Imported 2 units into Leer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && cell(main, 0, 2) == QStringLiteral("2"), 10000);
        page.importFile(here, QStringLiteral("Leer"), notTmx);
        QTRY_VERIFY_WITH_TIMEOUT(page.message().startsWith(QStringLiteral("Could not import Notizen.txt: ")), 10000);
        const QString out = m_tmp.filePath(QStringLiteral("Leer.tmx"));
        page.exportFile(here, QStringLiteral("Leer"), out);
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Saved Leer.tmx"), 10000);
        {
            QFile f(out);
            QVERIFY(f.open(QIODevice::ReadOnly));
            const QByteArray written = f.readAll();
            QVERIFY(written.contains("<tmx"));
            QVERIFY(written.contains("The deposit is due."));
        }

        // A project that uses a memory keeps it from being deleted, and its
        // own memory is listed with the project memories.
        QJsonObject settings = wait(api->get(QStringLiteral("/api/projects/defaults"))).object();
        QVERIFY(!settings.isEmpty());
        settings.insert("name", "Mietsache");
        settings.insert("sourceLang", "de-DE");
        settings.insert("targetLangs", QJsonArray{"en-GB"});
        settings.insert("projectMemory", true);
        settings.insert("memories", QJsonObject{{"en-GB", QJsonArray{QJsonObject{{"memory", "Mietsache (en-GB)"}, {"lookup", true}, {"concordance", true}, {"update", true}},
                                                                     QJsonObject{{"memory", "Weber DE-EN"}, {"lookup", true}, {"concordance", true}}}}});
        QJsonObject terminology = settings.value("terminology").toObject();
        terminology.insert("termbase", "Mietsache terms");
        settings.insert("terminology", terminology);
        const QByteArray txt = "Der Mieter zahlt die Miete.\n";
        const ApiResult made = wait(api->post(QStringLiteral("/api/projects"),
                                              QJsonObject{{"settings", settings},
                                                          {"files", QJsonArray{QJsonObject{{"name", "Mietsache.txt"}, {"role", "translatable"}, {"data", QString::fromLatin1(txt.toBase64())}}}},
                                                          {"memories", QJsonArray{QJsonObject{{"Name", "Mietsache (en-GB)"}, {"SourceLang", "de-DE"}, {"TargetLang", "en-GB"}}}},
                                                          {"termbase", QJsonObject{{"name", "Mietsache terms"}, {"languages", QJsonArray{"de-DE", "en-GB"}}}}}));
        QVERIFY2(made.ok(), qPrintable(made.error.message));
        QTableWidget *own = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((own = named<QTableWidget>(&page, QStringLiteral("Project memories"))) && own->rowCount() == 1, 10000);
        QCOMPARE(cell(own, 0, 0).section(QLatin1Char(','), 0, 0), QStringLiteral("Mietsache (en-GB)"));
        QCOMPARE(cell(own, 0, 3), QStringLiteral("Mietsache"));
        QTRY_VERIFY_WITH_TIMEOUT(label(&page, QStringLiteral("Project memories")), 10000);
        QPushButton *deleteUsed = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && main->rowCount() == 2 &&
                                     cell(main, 1, 3) == QStringLiteral("Mietsache") &&
                                     (deleteUsed = named<QPushButton>(&page, QStringLiteral("Delete Weber DE-EN"))),
                                 10000);
        QVERIFY(!deleteUsed->isEnabled());
        // A project memory is the project's: it is neither filled nor deleted here.
        QVERIFY(named<QPushButton>(&page, QStringLiteral("Export Mietsache (en-GB) as TMX")));
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Import TMX into Mietsache (en-GB)")));
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Delete Mietsache (en-GB)")));

        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTRY_VERIFY(button(&page, QStringLiteral("New memory")));
            button(&page, QStringLiteral("New memory"))->click();
            QTRY_VERIFY(button(&page, QStringLiteral("Cancel")));
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/memories-desktop.png"));
            button(&page, QStringLiteral("Cancel"))->click();
        }

        // Deleting asks first.
        QPushButton *del = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((del = named<QPushButton>(&page, QStringLiteral("Delete Leer"))), 10000);
        QVERIFY(del->isEnabled());
        del->click();
        QTRY_VERIFY(label(&page, QStringLiteral("Delete Leer and its 2 units?")));
        auto *confirm = qobject_cast<Web::ConfirmButton *>(del->parentWidget());
        QVERIFY(confirm && confirm->asking());
        confirm->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted the memory Leer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && main->rowCount() == 1, 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/memories"))).array().size(), 2);
    }

    void unitsOfAMemory()
    {
        // The units of a memory on this computer: searched, one added,
        // its translation changed, and deleted.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        ApiClient *api = m.local()->api();
        const QString here = m.local()->id();
        QVERIFY(wait(api->post(QStringLiteral("/api/memories"), QJsonObject{{"name", "Mietrecht"}, {"sourceLang", "de-DE"}, {"targetLang", "en-GB"}})).ok());
        QVERIFY(wait(api->upload("POST", QStringLiteral("/api/memories/Mietrecht/import"), tmx(), QStringLiteral("application/x-tmx+xml"))).ok());
        // A unit with formatting in it.
        const QJsonObject bold{{"kind", "open"}, {"id", "1"}, {"type", "bold"}};
        const QJsonObject boldEnd{{"kind", "close"}, {"id", "1"}, {"type", "bold"}};
        const ApiResult tagged = wait(api->post(QStringLiteral("/api/memories/Mietrecht/units"),
                                                QJsonObject{{"source", QJsonArray{QJsonObject{{"text", "Die "}}, QJsonObject{{"tag", bold}}, QJsonObject{{"text", "Nebenkosten"}}, QJsonObject{{"tag", boldEnd}},
                                                                                  QJsonObject{{"text", " steigen."}}}},
                                                            {"target", QJsonArray{QJsonObject{{"text", "The "}}, QJsonObject{{"tag", bold}}, QJsonObject{{"text", "service charges"}}, QJsonObject{{"tag", boldEnd}},
                                                                                  QJsonObject{{"text", " rise."}}}}}));
        QVERIFY2(tagged.ok(), qPrintable(tagged.error.message));

        MemoriesPage page(&m);
        page.resize(1280, 900);
        page.show();
        QPushButton *open = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((open = page.findChild<QPushButton *>(QStringLiteral("open:Mietrecht"))), 15000);
        QCOMPARE(open->text(), QStringLiteral("Open"));
        open->click();
        QTRY_VERIFY_WITH_TIMEOUT(page.findChild<QLineEdit *>(QStringLiteral("unitQuery")), 10000);
        QCOMPARE(page.openMemory(here), QStringLiteral("Mietrecht"));
        QTRY_COMPARE(page.findChild<QPushButton *>(QStringLiteral("open:Mietrecht"))->text(), QStringLiteral("Close"));

        // Searching the sentences, and the translations.
        QSignalSpy searched(&page, &MemoriesPage::searched);
        page.search(here, QStringLiteral("Kaution"));
        QTRY_COMPARE_WITH_TIMEOUT(searched.size(), 1, 10000);
        QCOMPARE(page.found(here).size(), 1);
        auto *table = page.findChild<QTableWidget *>(QStringLiteral("unitsFound"));
        QVERIFY(table);
        QCOMPARE(table->rowCount(), 1);
        QCOMPARE(cell(table, 0, 1), QStringLiteral("The deposit is due."));
        page.search(here, QStringLiteral("tenant"), true);
        QTRY_COMPARE_WITH_TIMEOUT(searched.size(), 2, 10000);
        QCOMPARE(page.found(here).size(), 1);
        page.search(here, QStringLiteral("Hausordnung"));
        QTRY_COMPARE_WITH_TIMEOUT(searched.size(), 3, 10000);
        QVERIFY(page.findChild<QLabel *>(QStringLiteral("noUnits")));

        // A new unit.
        QVERIFY(page.findChild<QPushButton *>(QStringLiteral("addUnitButton")));
        Web::Dialog *add = page.addUnitDialog(here);
        QVERIFY(add);
        QCOMPARE(add->title(), QStringLiteral("Add a unit to Mietrecht"));
        auto *save = add->findChild<QPushButton *>(QStringLiteral("save"));
        QVERIFY(save && !save->isEnabled());
        add->findChild<QPlainTextEdit *>(QStringLiteral("unitSource"))->setPlainText(QStringLiteral("Die Hausordnung gilt."));
        QVERIFY(!save->isEnabled());
        add->findChild<QPlainTextEdit *>(QStringLiteral("unitTarget"))->setPlainText(QStringLiteral("The house rules apply."));
        QVERIFY(save->isEnabled());
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(200);
            add->grab().save(dir + QStringLiteral("/memory-add-unit.png"));
        }
        save->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Added the unit to Mietrecht"), 10000);
        // The search is run again and now finds it.
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).size() == 1 && (table = page.findChild<QTableWidget *>(QStringLiteral("unitsFound"))) && table->rowCount() == 1, 10000);
        QCOMPARE(cell(table, 0, 1), QStringLiteral("The house rules apply."));
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/memories/Mietrecht"))).object().value("units").toInt(), 4, 10000);

        // Its translation is changed.
        QJsonObject unit = page.found(here).first().toObject();
        QVERIFY(unit.value("id").toInteger() > 0 && unit.value("version").toInt() > 0);
        QVERIFY(page.findChild<QPushButton *>(QStringLiteral("changeUnit:%1").arg(unit.value("id").toInteger())));
        Web::Dialog *change = page.editUnitDialog(here, unit);
        QVERIFY(change);
        auto *target = change->findChild<TargetEditor *>(QStringLiteral("unitTarget"));
        QVERIFY(target && change->findChild<TargetEditor *>(QStringLiteral("unitSource"))->isReadOnly());
        // Without a translation it is not a unit.
        target->setContent({});
        change->findChild<QPushButton *>(QStringLiteral("save"))->click();
        QCOMPARE(change->error(), QStringLiteral("A unit needs a translation. Delete the unit instead."));
        target->setContent(QJsonArray{QJsonObject{{"text", "The house rules are binding."}}});
        change->findChild<QPushButton *>(QStringLiteral("save"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Saved the unit"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).size() == 1 && page.found(here).first().toObject().value("version").toInt() == unit.value("version").toInt() + 1, 10000);
        QTRY_VERIFY((table = page.findChild<QTableWidget *>(QStringLiteral("unitsFound"))));
        QCOMPARE(cell(table, 0, 1), QStringLiteral("The house rules are binding."));

        // Saved over a newer version: said, and nothing is overwritten.
        change = page.editUnitDialog(here, unit);
        change->findChild<TargetEditor *>(QStringLiteral("unitTarget"))->setContent(QJsonArray{QJsonObject{{"text", "Too late."}}});
        change->findChild<QPushButton *>(QStringLiteral("save"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(change->error(), QStringLiteral("Someone else changed this unit in the meantime. Search again to see it as it is now."), 10000);
        change->reject();

        // A translation with formatting keeps its tags when the words change.
        page.search(here, QStringLiteral("Nebenkosten"));
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).size() == 1 && page.found(here).first().toObject().value("id").toInteger() == tagged.object().value("id").toInteger(), 10000);
        unit = page.found(here).first().toObject();
        change = page.editUnitDialog(here, unit);
        target = change->findChild<TargetEditor *>(QStringLiteral("unitTarget"));
        QCOMPARE(target->content().size(), 5);
        QTextCursor end(target->document());
        end.movePosition(QTextCursor::End);
        end.insertText(QStringLiteral(" Again."));
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(200);
            change->grab().save(dir + QStringLiteral("/memory-change-unit.png"));
        }
        change->findChild<QPushButton *>(QStringLiteral("save"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/memories/Mietrecht/units/%1").arg(unit.value("id").toInteger()))).object().value("version").toInt(),
                                  unit.value("version").toInt() + 1, 10000);
        const QJsonArray saved = wait(api->get(QStringLiteral("/api/memories/Mietrecht/units/%1").arg(unit.value("id").toInteger()))).object().value("target").toArray();
        QCOMPARE(saved.size(), 5);
        QVERIFY(saved[1].toObject().contains("tag") && saved[3].toObject().contains("tag"));
        QCOMPARE(saved[4].toObject().value("text").toString(), QStringLiteral(" rise. Again."));

        // Deleting a unit asks first.
        QTRY_VERIFY_WITH_TIMEOUT(page.findChild<Web::ConfirmButton *>(QStringLiteral("deleteUnit:%1").arg(unit.value("id").toInteger())), 10000);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/memory-units.png"));
        }
        auto *del = page.findChild<Web::ConfirmButton *>(QStringLiteral("deleteUnit:%1").arg(unit.value("id").toInteger()));
        del->button()->click();
        QVERIFY(label(&page, QStringLiteral("Delete this unit?")));
        del->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted the unit"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).isEmpty() && page.findChild<QLabel *>(QStringLiteral("noUnits")), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/memories/Mietrecht"))).object().value("units").toInt(), 3, 10000);

        // A document and its translation, done before, are aligned into the
        // memory: the sentences are paired with lines between the two texts,
        // lines are taken away and drawn, and what is paired is added.
        QVERIFY(page.findChild<QPushButton *>(QStringLiteral("alignButton")));
        AlignmentDialog *al = page.alignDialog(here);
        QVERIFY(al);
        QCOMPARE(al->title(), QStringLiteral("Align documents into Mietrecht"));
        QCOMPARE(al->findChild<QPushButton *>(QStringLiteral("chooseSource"))->accessibleName(), QStringLiteral("Document in German (Germany)"));
        QCOMPARE(al->findChild<QPushButton *>(QStringLiteral("chooseTarget"))->accessibleName(), QStringLiteral("Its translation in English (United Kingdom)"));
        auto *go = al->findChild<QPushButton *>(QStringLiteral("save"));
        QCOMPARE(go->text(), QStringLiteral("Pair the sentences"));
        QVERIFY(!go->isEnabled() && al->findChild<QPushButton *>(QStringLiteral("otherDocuments"))->isHidden());
        const QString fixtures = QStringLiteral(INTERLINEA_FIXTURES);
        // A file that is no text says so.
        al->setFile(AlignmentDialog::Source, fixtures + QStringLiteral("/Styleguide.pdf"));
        QVERIFY(!go->isEnabled());
        al->setFile(AlignmentDialog::Target, fixtures + QStringLiteral("/align/Mietvertrag_fr.txt"));
        QCOMPARE(al->findChild<QLabel *>(QStringLiteral("chooseTargetName"))->text(), QStringLiteral("Mietvertrag_fr.txt"));
        QVERIFY(go->isEnabled());
        go->click();
        QTRY_VERIFY_WITH_TIMEOUT(al->error().startsWith(QStringLiteral("Styleguide.pdf could not be read")), 10000);
        al->setFile(AlignmentDialog::Source, fixtures + QStringLiteral("/align/Mietvertrag_de.txt"));
        QSignalSpy pairedSpy(al, &AlignmentDialog::paired);
        al->findChild<QPushButton *>(QStringLiteral("save"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(pairedSpy.size(), 1, 20000);
        QCOMPARE(al->segmentCount(AlignmentDialog::Source), 5);
        QCOMPARE(al->segmentCount(AlignmentDialog::Target), 4);
        QCOMPARE(al->status(), QStringLiteral("4 units from 5 and 4 segments. All of them fit."));
        QCOMPARE(al->findChild<QLabel *>(QStringLiteral("alignStatus"))->text(), al->status());
        // Two sentences translated as one both lead to it.
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 3|2 4|3"));
        QCOMPARE(al->text(AlignmentDialog::Source, 1), QStringLiteral("Die Kaution beträgt 2.400,00 Euro."));
        go = al->findChild<QPushButton *>(QStringLiteral("save"));
        QCOMPARE(go->text(), QStringLiteral("Add 4 units to Mietrecht"));
        QVERIFY(!al->findChild<QPushButton *>(QStringLiteral("otherDocuments"))->isHidden());
        // A line runs from the sentence to its translation.
        AlignBoard *board = al->board();
        QVERIFY(board);
        QTRY_VERIFY(board->segments[0].at(0)->width() > 100);
        auto *s0 = board->segments[0].at(0);
        auto *t0 = board->segments[1].at(0);
        QLineF line = board->place({0, 0});
        QVERIFY(qAbs(line.x1() - (s0->geometry().right() + 1)) < 2 && qAbs(line.x2() - t0->geometry().left()) < 2);
        QVERIFY(qAbs(line.y1() - s0->geometry().center().y()) < 2 && qAbs(line.y2() - t0->geometry().center().y()) < 2);
        QVERIFY(line.x2() - line.x1() > 80);
        // Between segments at different heights the line is a curve that
        // leaves and arrives level: a place just beside an end, at its
        // height, is on it, and one beside it at the other end's height is not.
        const QLineF slanted = board->place({3, 2});
        QVERIFY(qAbs(slanted.y1() - slanted.y2()) > 15);
        QCOMPARE(board->lineIndexAt(QPointF(slanted.x1() + 8, slanted.y1())), int(board->lines.indexOf({3, 2})));
        QCOMPARE(board->lineIndexAt(QPointF(slanted.x1() + 8, slanted.y2())), -1);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            al->grab().save(dir + QStringLiteral("/memory-align-paired.png"));
        }
        // What was paired for the translator is dotted. A pair confirmed is
        // solid and stays when the rest is paired again around it.
        auto *confirm = al->findChild<QPushButton *>(QStringLiteral("confirmLines"));
        auto *again = al->findChild<QPushButton *>(QStringLiteral("pairAgain"));
        QVERIFY(confirm && again && !confirm->isEnabled() && !again->isEnabled());
        QVERIFY(al->mineLines().isEmpty() && board->mineLines.isEmpty());
        al->choose(AlignmentDialog::Source, 1);
        QVERIFY(confirm->isEnabled());
        confirm->click();
        QCOMPARE(al->mineLines().join(' '), QStringLiteral("1|1"));
        QVERIFY(board->mineLines.contains({1, 1}) && !confirm->isEnabled());
        QCOMPARE(al->status(), QStringLiteral("4 units from 5 and 4 segments. 1 unit is checked. All of them fit."));
        al->takeLine(0, 0);
        al->takeLine(4, 3);
        QVERIFY(again->isEnabled());
        again->click();
        QTRY_COMPARE_WITH_TIMEOUT(pairedSpy.size(), 2, 20000);
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 3|2 4|3"));
        QCOMPARE(al->mineLines().join(' '), QStringLiteral("1|1"));
        board = al->board();
        go = al->findChild<QPushButton *>(QStringLiteral("save"));
        QTRY_VERIFY(board->segments[0].at(0)->width() > 100);
        s0 = board->segments[0].at(0);
        t0 = board->segments[1].at(0);
        // A line drawn by hand is the translator's own, with the unit it joins.
        al->takeLine(3, 2);
        al->choose(AlignmentDialog::Source, 3);
        al->choose(AlignmentDialog::Target, 2);
        QCOMPARE(al->mineLines().join(' '), QStringLiteral("1|1 2|2 3|2"));
        // Every segment has a point in the gap beside it, where its lines begin.
        QVERIFY(qAbs(board->port(0, 0).x() - (s0->geometry().right() + 7)) < 2 && qAbs(board->port(0, 0).y() - line.y1()) < 2);
        QVERIFY(qAbs(board->port(1, 0).x() - (t0->geometry().left() - 6)) < 2 && qAbs(board->port(1, 0).y() - line.y2()) < 2);
        line = board->place({0, 0});
        QCOMPARE(board->segmentAt(board->port(1, 3).toPoint()), board->segments[1].at(3));
        // Pulled from one point to the other, a line is taken up by both; the
        // point pressed alone chooses its segment.
        al->takeLine(4, 3);
        QVERIFY(board->segments[0].at(4)->alone() && board->segments[1].at(3)->alone());
        QTest::mousePress(board, Qt::LeftButton, {}, board->port(0, 4).toPoint());
        QTest::mouseRelease(board, Qt::LeftButton, {}, board->port(1, 3).toPoint());
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 3|2 4|3"));
        QVERIFY(!board->segments[0].at(4)->alone());
        QTest::mouseClick(board, Qt::LeftButton, {}, board->port(1, 1).toPoint());
        QVERIFY(board->segments[1].at(1)->picked());
        QTest::mouseClick(board, Qt::LeftButton, {}, board->port(1, 1).toPoint());
        QVERIFY(!board->segments[1].at(1)->picked());
        // A sentence chosen and then its translation takes their line away.
        QTest::mouseClick(board->segments[0].at(3), Qt::LeftButton);
        QVERIFY(board->segments[0].at(3)->property("picked").toBool());
        QTest::mouseClick(board->segments[1].at(2), Qt::LeftButton);
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 4|3"));
        QCOMPARE(al->status(), QStringLiteral("4 units from 5 and 4 segments. 3 units are checked. 1 segment has no pair and is left out."));
        QVERIFY(board->segments[0].at(3)->property("alone").toBool() && !board->segments[0].at(3)->property("picked").toBool());
        QVERIFY(board->segments[0].at(3)->accessibleName().startsWith(QStringLiteral("Sentence 4, no pair: Der Vermieter")));
        // The same draws it again, from the keyboard as well.
        QTest::keyClick(board->segments[1].at(2), Qt::Key_Space);
        QTest::keyClick(board->segments[0].at(3), Qt::Key_Return);
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 3|2 4|3"));
        QCOMPARE(al->status(), QStringLiteral("4 units from 5 and 4 segments. 3 units are checked. All of them fit."));
        // A line is taken away by choosing it, and drawn by pulling from one text to the other.
        QTest::mouseClick(board, Qt::LeftButton, {}, line.center().toPoint());
        QCOMPARE(al->lines().join(' '), QStringLiteral("1|1 2|2 3|2 4|3"));
        QCOMPARE(al->status(), QStringLiteral("3 units from 5 and 4 segments. 3 units are checked. 2 segments have no pair and are left out."));
        QCOMPARE(go->text(), QStringLiteral("Add 3 units to Mietrecht"));
        QTest::mousePress(s0, Qt::LeftButton, {}, s0->rect().center());
        QTest::mouseRelease(s0, Qt::LeftButton, {}, s0->mapFromGlobal(t0->mapToGlobal(t0->rect().center())));
        QCOMPARE(al->lines().join(' '), QStringLiteral("0|0 1|1 2|2 3|2 4|3"));
        // A sentence joined to a second translation makes one unit of all three.
        al->choose(AlignmentDialog::Source, 0);
        al->choose(AlignmentDialog::Target, 1);
        QCOMPARE(al->unitCount(), 3);
        al->choose(AlignmentDialog::Source, 0);
        al->choose(AlignmentDialog::Target, 1);
        QCOMPARE(al->unitCount(), 4);
        // The first sentence is left out, and the rest is added.
        al->takeLine(0, 0);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            al->choose(AlignmentDialog::Source, 1);
            QTest::qWait(300);
            al->grab().save(dir + QStringLiteral("/memory-align.png"));
            al->choose(AlignmentDialog::Source, 1);
        }
        QCOMPARE(go->text(), QStringLiteral("Add 3 units to Mietrecht"));
        QVERIFY(al->findChild<QPushButton *>(QStringLiteral("nextToLook"))->isEnabled());
        go->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Added 3 units to Mietrecht"), 10000);
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/memories/Mietrecht"))).object().value("units").toInt(), 6, 10000);
        page.search(here, QStringLiteral("Vermögen"));
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).size() == 1 && (table = page.findChild<QTableWidget *>(QStringLiteral("unitsFound"))) && table->rowCount() == 1, 10000);
        QCOMPARE(cell(table, 0, 0), QStringLiteral("Sie wird bei Beginn des Mietverhältnisses fällig. Der Vermieter legt sie getrennt von seinem Vermögen an."));
        QCOMPARE(cell(table, 0, 1), QStringLiteral("Il est exigible au début du bail et le bailleur le place séparément de son patrimoine."));
        page.search(here, QStringLiteral("Werktag"));
        QTRY_VERIFY_WITH_TIMEOUT(page.found(here).isEmpty() && page.findChild<QLabel *>(QStringLiteral("noUnits")), 10000);

        // Closing puts the units away.
        page.findChild<QPushButton *>(QStringLiteral("open:Mietrecht"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(page.openMemory(here).isEmpty() && !page.findChild<QLineEdit *>(QStringLiteral("unitQuery")), 10000);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestMemories)
#include "tst_memories.moc"
