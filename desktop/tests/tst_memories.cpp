// The Memories page against the app's own core: new memories with and
// without a TMX file, TMX in and out, what uses a memory, the project
// memories, and deleting.
#include "apiclient.h"
#include "connectionmanager.h"
#include "languages.h"
#include "memoriespage.h"
#include "testcore.h"
#include "theme.h"
#include "webui.h"

#include <QApplication>
#include <QFile>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QSettings>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTableWidget>
#include <QTemporaryDir>
#include <QTest>

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
        button(&page, QStringLiteral("New memory"))->click();
        auto *create = button(&page, QStringLiteral("Create memory"));
        QVERIFY(create);
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
        QVERIFY(!button(&page, QStringLiteral("Create memory")));
        QVERIFY(button(&page, QStringLiteral("New memory")));

        // A name that is taken, and equal languages, are said before anything is sent.
        button(&page, QStringLiteral("New memory"))->click();
        create = button(&page, QStringLiteral("Create memory"));
        auto *name = named<QLineEdit>(&page, QStringLiteral("Name"));
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
        // Nothing to export from an empty memory.
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Export Leer as TMX"))->isEnabled());
        QVERIFY(named<QPushButton>(&page, QStringLiteral("Export Weber DE-EN as TMX"))->isEnabled());

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
        QVERIFY(label(&page, QStringLiteral("Project memories")));
        // A project memory is the project's: it is neither filled nor deleted here.
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Import TMX into Mietsache (en-GB)")));
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Delete Mietsache (en-GB)")));
        main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"));
        QCOMPARE(main->rowCount(), 2);
        QCOMPARE(cell(main, 1, 3), QStringLiteral("Mietsache"));
        QVERIFY(!named<QPushButton>(&page, QStringLiteral("Delete Weber DE-EN"))->isEnabled());

        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            button(&page, QStringLiteral("New memory"))->click();
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/memories-desktop.png"));
            button(&page, QStringLiteral("Cancel"))->click();
        }

        // Deleting asks first.
        auto *del = named<QPushButton>(&page, QStringLiteral("Delete Leer"));
        QVERIFY(del && del->isEnabled());
        del->click();
        QVERIFY(label(&page, QStringLiteral("Delete Leer and its 2 units?")));
        auto *confirm = qobject_cast<Web::ConfirmButton *>(del->parentWidget());
        QVERIFY(confirm && confirm->asking());
        confirm->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted the memory Leer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((main = named<QTableWidget>(&page, QStringLiteral("Main memories on this computer"))) && main->rowCount() == 1, 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/memories"))).array().size(), 2);
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestMemories)
#include "tst_memories.moc"
