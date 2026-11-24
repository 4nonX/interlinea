// The whole app against a real team server (INTERLINEA_TEAM_SERVER, with
// the administrator INTERLINEA_TEAM_USER / INTERLINEA_TEAM_PASSWORD) and
// its own core: sign in, keep a memory on the server, make a project on
// each from a document, translate on the server, and get the translated
// document back. Skipped without a team server.
#include "termbasedialogs.h"
#include "accesspanel.h"
#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "mainwindow.h"
#include "accountspage.h"
#include "memoriespage.h"
#include "reviewpage.h"
#include "termdata.h"
#include "terminologypage.h"
#include "newprojectpage.h"
#include "projectpage.h"
#include "projectsmodel.h"
#include "projectspage.h"
#include "settingspage.h"
#include "testcore.h"
#include "theme.h"
#include "webui.h"

#include <QApplication>
#include <QComboBox>
#include <QCheckBox>
#include <QFile>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QRadioButton>
#include <QScrollArea>
#include <QSettings>
#include <QSignalSpy>
#include <QStackedWidget>
#include <QStandardPaths>
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
static T *named(QWidget *root, const QString &name)
{
    for (T *w : root->findChildren<T *>())
        if (w->accessibleName() == name)
            return w;
    return nullptr;
}

static QPushButton *button(QWidget *root, const QString &text)
{
    for (QPushButton *b : root->findChildren<QPushButton *>())
        if (b->text().remove(QLatin1Char('&')) == text && !b->isHidden())
            return b;
    return nullptr;
}

static QLabel *label(QWidget *root, const QString &text)
{
    for (QLabel *l : root->findChildren<QLabel *>())
        if (l->text() == text && l->isVisibleTo(root))
            return l;
    return nullptr;
}

// Signs in to the team server of the environment; null without one.
static QLabel *labelWith(QWidget *root, const QString &part)
{
    for (QLabel *l : root->findChildren<QLabel *>())
        if (l->text().contains(part) && !(l->isHidden() && l->testAttribute(Qt::WA_WState_ExplicitShowHide)))
            return l;
    return nullptr;
}

static Connection *signIn(ConnectionManager &m)
{
    const QString server = qEnvironmentVariable("INTERLINEA_TEAM_SERVER");
    if (server.isEmpty())
        return nullptr;
    Connection *team = nullptr;
    bool done = false;
    m.signInWithPassword(ConnectionManager::serverUrl(server), qEnvironmentVariable("INTERLINEA_TEAM_USER"), qEnvironmentVariable("INTERLINEA_TEAM_PASSWORD"),
                         [&](Connection *c, const ApiError &) { team = c; done = true; });
    for (int i = 0; i < 400 && !done; i++)
        QTest::qWait(50);
    return team;
}

template <typename T>
static T *byName(QWidget *root, const QString &objectName)
{
    for (T *w : root->findChildren<T *>(objectName)) {
        bool gone = false;
        for (const QWidget *x = w; x && x != root; x = x->parentWidget())
            gone = gone || (x->isHidden() && x->testAttribute(Qt::WA_WState_ExplicitShowHide));
        if (!gone)
            return w;
    }
    return nullptr;
}

static void pick(QComboBox *box, const QVariant &data)
{
    const int i = box->findData(data);
    QVERIFY2(i >= 0, qPrintable(QStringLiteral("no choice %1").arg(data.toString())));
    box->setCurrentIndex(i);
    emit box->activated(i);
}

class TestEndToEnd : public QObject {
    Q_OBJECT
private slots:
    void initTestCase()
    {
        qRegisterMetaType<ApiResult>("ApiResult");
        QStandardPaths::setTestModeEnabled(true);
        QCoreApplication::setOrganizationName(QStringLiteral("InterlineaTests"));
        QCoreApplication::setApplicationName(QStringLiteral("tst_endtoend"));
        QSettings().clear();
        qputenv("INTERLINEA_SECRETS_FILE", m_tmp.filePath(QStringLiteral("sign-ins.json")).toUtf8());
        Theme::apply(*qApp);
    }

    void wholeApp()
    {
        const QString server = qEnvironmentVariable("INTERLINEA_TEAM_SERVER");
        if (server.isEmpty())
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        TestCore core;
        ConnectionManager m(&core.launcher);
        MainWindow w(&m);
        auto *projects = new ProjectsPage(&m, &w);
        w.addPage(QStringLiteral("projects"), QStringLiteral("Projects"), QIcon(), projects);
        auto *memories = new MemoriesPage(&m, &w);
        w.addPage(QStringLiteral("memories"), QStringLiteral("Memories"), QIcon(), memories);
        w.addPage(QStringLiteral("settings"), QStringLiteral("Settings"), QIcon(), new SettingsPage(&m, &w));
        w.show();
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);

        // Sign in to the team server.
        Connection *team = nullptr;
        ApiError err;
        bool done = false;
        m.signInWithPassword(ConnectionManager::serverUrl(server), qEnvironmentVariable("INTERLINEA_TEAM_USER"),
                             qEnvironmentVariable("INTERLINEA_TEAM_PASSWORD"), [&](Connection *c, const ApiError &e) { team = c; err = e; done = true; });
        QTRY_VERIFY_WITH_TIMEOUT(done, 20000);
        QVERIFY2(team, qPrintable(err.message));
        QTRY_VERIFY_WITH_TIMEOUT(team->feed()->isLive(), 10000);

        // A memory kept on the team server: made, filled from TMX, searched,
        // shared, written out and deleted, all from the Memories page.
        w.showPage(QStringLiteral("memories"));
        const QString host = team->displayName();
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("On the team server")), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(button(memories, QStringLiteral("New memory on the server")), 10000);
        button(memories, QStringLiteral("New memory on the server"))->click();
        QTRY_VERIFY(button(memories, QStringLiteral("Make the memory")) && named<QLineEdit>(memories, QStringLiteral("Name")));
        QVERIFY(!button(memories, QStringLiteral("Make the memory"))->isEnabled());
        named<QLineEdit>(memories, QStringLiteral("Name"))->setText(QStringLiteral("Kanzlei Weber DE-EN"));
        button(memories, QStringLiteral("Make the memory"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Made the memory Kanzlei Weber DE-EN on %1").arg(host), 10000);
        // It is listed and opened, with who may use it for its manager.
        QTRY_COMPARE_WITH_TIMEOUT(memories->openMemory(team->id()), QStringLiteral("Kanzlei Weber DE-EN"), 10000);
        QTableWidget *listed = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((listed = named<QTableWidget>(memories, QStringLiteral("Memories on %1").arg(host))) && listed->rowCount() == 1, 10000);
        QCOMPARE(listed->item(0, 3)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("manage"));
        QTRY_VERIFY_WITH_TIMEOUT(memories->findChild<AccessPanel *>(), 10000);
        QTRY_VERIFY(button(memories, QStringLiteral("Close")));

        const QString tmxFile = m_tmp.filePath(QStringLiteral("Weber.tmx"));
        {
            QFile t(tmxFile);
            QVERIFY(t.open(QIODevice::WriteOnly));
            t.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tmx version=\"1.4\"><header creationtool=\"test\" creationtoolversion=\"1\" "
                    "datatype=\"plaintext\" segtype=\"sentence\" adminlang=\"en\" srclang=\"de-DE\" o-tmf=\"test\"/><body>"
                    "<tu><tuv xml:lang=\"de-DE\"><seg>Der Vermieter haftet nicht.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The landlord is not liable.</seg></tuv></tu>"
                    "<tu><tuv xml:lang=\"de-DE\"><seg>Die Nebenkosten sind fällig.</seg></tuv><tuv xml:lang=\"en-GB\"><seg>The service charges are due.</seg></tuv></tu>"
                    "</body></tmx>\n");
        }
        memories->importFile(team->id(), QStringLiteral("Kanzlei Weber DE-EN"), tmxFile);
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Read 2 units from Weber.tmx; 2 stored"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT((listed = named<QTableWidget>(memories, QStringLiteral("Memories on %1").arg(host))) &&
                                     listed->item(0, 2)->data(Qt::AccessibleTextRole).toString() == QStringLiteral("2"),
                                 10000);
        // The open memory was not put away by the list being read again.
        QCOMPARE(memories->openMemory(team->id()), QStringLiteral("Kanzlei Weber DE-EN"));

        // Its units are searched, in the sources and in the translations.
        QLineEdit *query = nullptr;
        QTRY_VERIFY((query = named<QLineEdit>(memories, QStringLiteral("Search the units"))) && button(memories, QStringLiteral("Search")) &&
                    button(memories, QStringLiteral("Delete the memory")));
        QVERIFY(!button(memories, QStringLiteral("Search"))->isEnabled());
        query->setText(QStringLiteral("Nebenkosten"));
        button(memories, QStringLiteral("Search"))->click();
        QTableWidget *found = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((found = named<QTableWidget>(memories, QStringLiteral("Units found"))) && found->rowCount() == 1, 10000);
        QCOMPARE(found->item(0, 1)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("The service charges are due."));
        // When it changed, without a stray comma where the TMX names nobody.
        const QString changed = found->item(0, 2)->data(Qt::AccessibleTextRole).toString();
        QVERIFY2(!changed.isEmpty() && !changed.startsWith(QLatin1Char(',')), qPrintable(changed));
        query->setText(QStringLiteral("landlord"));
        button(memories, QStringLiteral("Search"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("Nothing found.")), 10000);
        for (QCheckBox *c : memories->findChildren<QCheckBox *>())
            if (c->text() == QStringLiteral("In the translations"))
                c->setChecked(true);
        button(memories, QStringLiteral("Search"))->click();
        QTRY_VERIFY_WITH_TIMEOUT((found = named<QTableWidget>(memories, QStringLiteral("Units found"))) && found->rowCount() == 1 &&
                                     found->item(0, 0)->data(Qt::AccessibleTextRole).toString() == QStringLiteral("Der Vermieter haftet nicht."),
                                 10000);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            w.resize(1440, 900);
            QTest::qWait(400);
            w.grab().save(dir + QStringLiteral("/memories-server-desktop.png"));
        }

        const QString written = m_tmp.filePath(QStringLiteral("Weber-out.tmx"));
        memories->exportFile(team->id(), QStringLiteral("Kanzlei Weber DE-EN"), written);
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Saved Weber-out.tmx"), 10000);
        {
            QFile t(written);
            QVERIFY(t.open(QIODevice::ReadOnly));
            QVERIFY(t.readAll().contains("The landlord is not liable."));
        }

        // Deleting it for everyone asks first.
        button(memories, QStringLiteral("Delete the memory"))->click();
        QTRY_VERIFY(label(memories, QStringLiteral("Delete Kanzlei Weber DE-EN and its 2 units on the server for everyone?")));
        for (Web::ConfirmButton *c : memories->findChildren<Web::ConfirmButton *>())
            if (c->asking())
                c->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(memories->message(), QStringLiteral("Deleted Kanzlei Weber DE-EN on the server"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(label(memories, QStringLiteral("No memories on %1 for you yet.").arg(host)), 10000);
        QVERIFY(memories->openMemory(team->id()).isEmpty());
        QVERIFY(!memories->findChild<AccessPanel *>() || memories->findChild<AccessPanel *>()->isHidden());
        w.showPage(QStringLiteral("projects"));

        const QString doc = m_tmp.filePath(QStringLiteral("Mietvertrag.txt"));
        QFile f(doc);
        QVERIFY(f.open(QIODevice::WriteOnly));
        f.write("Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats. Die Kaution beträgt drei Monatsmieten.\n");
        f.close();

        // A project on each: this computer and the team server.
        for (Connection *where : {m.local(), team}) {
            projects->newProject({doc});
            NewProjectPage *np = projects->newProjectPage();
            QVERIFY(np);
            // With a team server signed in, the first step asks where the project is kept.
            QRadioButton *place = nullptr;
            const QString placeText = where->isLocal() ? QStringLiteral("On this computer") : QStringLiteral("On the team server %1").arg(where->displayName());
            QTRY_VERIFY_WITH_TIMEOUT([&] {
                place = nullptr;
                for (QRadioButton *b : np->findChildren<QRadioButton *>())
                    if (b->text() == placeText)
                        place = b;
                return place != nullptr;
            }(), 15000);
            place->setChecked(true);
            QTRY_VERIFY_WITH_TIMEOUT(np->connection() == where && button(np, QStringLiteral("Continue to files and languages")) &&
                                         button(np, QStringLiteral("Continue to files and languages"))->isEnabled(),
                                     15000);
            button(np, QStringLiteral("Continue to files and languages"))->click();
            QTRY_VERIFY_WITH_TIMEOUT(named<QComboBox>(np, QStringLiteral("Use of Mietvertrag.txt")) && named<QComboBox>(np, QStringLiteral("Use of Mietvertrag.txt"))->isEnabled(), 15000);
            named<LanguageCombo>(np, QStringLiteral("Add another target language"))->setLanguage(QStringLiteral("en-GB"));
            named<QPushButton>(np, QStringLiteral("Add the target language"))->click();
            QTRY_VERIFY(button(np, QStringLiteral("Continue to memories"))->isEnabled());
            button(np, QStringLiteral("Continue to memories"))->click();
            button(np, QStringLiteral("Continue to terminology"))->click();
            button(np, QStringLiteral("Continue to review"))->click();
            const QString createText = where->isLocal() ? QStringLiteral("Create project") : QStringLiteral("Create on the server");
            QTRY_VERIFY_WITH_TIMEOUT(button(np, createText) && button(np, createText)->isEnabled(), 20000);
            QSignalSpy made(np, &NewProjectPage::created);
            button(np, createText)->click();
            QTRY_COMPARE_WITH_TIMEOUT(made.size(), 1, 20000);
            QCOMPARE(made.first().at(0).toString(), where->id());
            const qint64 madeId = made.first().at(1).toLongLong();

            // Translate both segments where the project is kept.
            const QString base = QStringLiteral("/api/projects/%1").arg(madeId);
            const QUrlQuery fq{{"file", "Mietvertrag.txt"}, {"lang", "en-GB"}};
            ApiResult r = wait(where->api()->get(base + "/segments", fq));
            QVERIFY2(r.ok(), qPrintable(r.error.message));
            QCOMPARE(r.array().size(), 2);
            const QStringList en{QStringLiteral("The tenant pays the rent by the third working day of each month."),
                                 QStringLiteral("The deposit is three months' rent.")};
            for (int i = 0; i < 2; ++i) {
                const QJsonObject s = r.array().at(i).toObject();
                QUrlQuery q = fq;
                q.addQueryItem("segment", s.value("id").toString());
                const ApiResult saved = wait(where->api()->put(base + "/segments", QJsonObject{{"target", QJsonArray{QJsonObject{{"text", en[i]}}}}, {"status", "confirmed"}},
                                                               s.value("version").toInt(), q));
                QVERIFY2(saved.ok(), qPrintable(saved.error.message));
            }
            // The project page shows it done, and the document comes back translated.
            projects->open(where->id(), madeId);
            ProjectPage *pp = projects->openProject();
            QTRY_VERIFY_WITH_TIMEOUT(pp->findChild<QPushButton *>(QStringLiteral("progress")) &&
                                         pp->findChild<QPushButton *>(QStringLiteral("progress"))->text() == QStringLiteral("100%"),
                                     15000);
            // On the server it has the work, who is working now and who may use it.
            QStringList panels;
            for (QFrame *f : pp->findChildren<QFrame *>(QStringLiteral("panel")))
                if (!f->isHidden())
                    panels << f->accessibleName();
            QCOMPARE(panels.contains(QStringLiteral("Work")), !where->isLocal());
            QCOMPARE(panels.contains(QStringLiteral("Working now")), !where->isLocal());
            QVERIFY(panels.contains(QStringLiteral("Delete the project")));
            r = wait(where->api()->get(base + "/files/translated", QUrlQuery{{"name", "Mietvertrag.txt"}, {"lang", "en-GB"}}));
            QVERIFY2(r.ok(), qPrintable(r.error.message));
            QCOMPARE(QString::fromUtf8(r.body).trimmed(), en.join(QLatin1Char(' ')));
            // Confirmed translations went into the project's own memory.
            r = wait(where->api()->post(base + "/lookup", QJsonObject{{"lang", "en-GB"}, {"source", QJsonArray{QJsonObject{{"text", "Die Kaution beträgt drei Monatsmieten."}}}}}));
            QVERIFY2(r.ok() && !r.array().isEmpty(), qPrintable(r.error.message));
            QCOMPARE(r.array().at(0).toObject().value("score").toInt(), 100);
            projects->showList();
        }
        // Both projects are listed, each kept where it was made.
        QTRY_COMPARE_WITH_TIMEOUT(projects->model()->rowCount(), 2, 10000);
    }

    void accounts()
    {
        // The Accounts page against the team server: a new account, its
        // details, roles, password and sign-ins, and deleting it.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        AccountsPage page(&m);
        page.resize(1280, 1000);
        page.show();
        // Without a team server the page says where accounts are kept.
        QTRY_VERIFY(byName<QLabel>(&page, QStringLiteral("state")));
        QCOMPARE(byName<QLabel>(&page, QStringLiteral("state"))->text(), QStringLiteral("Accounts are kept on a team server. Sign in to one in Settings."));
        QSignalSpy settings(&page, &AccountsPage::openSettings);
        byName<QPushButton>(&page, QStringLiteral("openSettings"))->click();
        QCOMPARE(settings.size(), 1);

        Connection *team = signIn(m);
        if (!team)
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        ApiClient *api = team->api();
        // Left from an earlier run.
        for (const QJsonValue &v : wait(api->get(QStringLiteral("/api/users"))).array()) {
            if (v.toObject().value("username").toString() == QStringLiteral("ben.review"))
                wait(api->del(QStringLiteral("/api/users/%1").arg(v.toObject().value("id").toInteger())));
        }
        QTRY_VERIFY_WITH_TIMEOUT(page.connectionId() == team->id() && !page.users().isEmpty() && !page.busy(), 15000);
        const QString admin = qEnvironmentVariable("INTERLINEA_TEAM_USER");
        QVERIFY(byName<Web::Clickable>(&page, QStringLiteral("account:%1").arg(admin)));
        QVERIFY(label(&page, QStringLiteral("Choose an account to see and change it.")));
        QVERIFY(label(&page, QStringLiteral("The people on %1. A role is a set of permissions: five come with the server, each including the ones before it, and you define others below.").arg(team->displayName())));

        // A new account: what the server refuses is said at its field.
        QWidget *form = byName<QWidget>(&page, QStringLiteral("newAccount"));
        QVERIFY(form);
        QVERIFY(!byName<QPushButton>(form, QStringLiteral("make"))->isEnabled());
        byName<QLineEdit>(form, QStringLiteral("username"))->setText(QStringLiteral("ben.review"));
        byName<QLineEdit>(form, QStringLiteral("name"))->setText(QStringLiteral("Ben Reviewer"));
        byName<QLineEdit>(form, QStringLiteral("password"))->setText(QStringLiteral("short"));
        byName<QPushButton>(form, QStringLiteral("make"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
        form = byName<QWidget>(&page, QStringLiteral("newAccount"));
        QCOMPARE(byName<QLineEdit>(form, QStringLiteral("username"))->text(), QStringLiteral("ben.review"));
        QVERIFY(byName<QLineEdit>(form, QStringLiteral("password"))->property("invalid").toBool());
        byName<QLineEdit>(form, QStringLiteral("password"))->setText(QStringLiteral("a long enough password for ben"));
        byName<QPushButton>(form, QStringLiteral("make"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Made the account ben.review"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.selected() != 0, 10000);
        const qint64 ben = page.selected();
        QTRY_VERIFY(byName<Web::Clickable>(&page, QStringLiteral("account:ben.review")) && byName<Web::Clickable>(&page, QStringLiteral("account:ben.review"))->isCurrent());
        QVERIFY(byName<QLabel>(&page, QStringLiteral("about"))->text().startsWith(QStringLiteral("ben.review, made on ")));
        QVERIFY(!byName<QLabel>(&page, QStringLiteral("about"))->text().endsWith(QStringLiteral("This is you.")));
        QVERIFY(byName<QLabel>(&page, QStringLiteral("noRoles")) && byName<QLabel>(&page, QStringLiteral("noSignIns")));
        QCOMPARE(byName<QLineEdit>(byName<QWidget>(&page, QStringLiteral("newAccount")), QStringLiteral("username"))->text(), QString());
        // The same name again is refused at the username.
        form = byName<QWidget>(&page, QStringLiteral("newAccount"));
        byName<QLineEdit>(form, QStringLiteral("username"))->setText(QStringLiteral("ben.review"));
        byName<QPushButton>(form, QStringLiteral("make"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QLineEdit>(byName<QWidget>(&page, QStringLiteral("newAccount")), QStringLiteral("username"))->property("invalid").toBool(), 10000);
        byName<QLineEdit>(byName<QWidget>(&page, QStringLiteral("newAccount")), QStringLiteral("username"))->clear();

        // Details.
        QVERIFY(!byName<QPushButton>(&page, QStringLiteral("saveDetails"))->isEnabled());
        byName<QLineEdit>(&page, QStringLiteral("detailEmail"))->setText(QStringLiteral("ben@example.org"));
        QVERIFY(byName<QPushButton>(&page, QStringLiteral("saveDetails"))->isEnabled());
        byName<QPushButton>(&page, QStringLiteral("saveDetails"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Saved the details"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
        QCOMPARE(wait(api->get(QStringLiteral("/api/users/%1").arg(ben))).object().value("email").toString(), QStringLiteral("ben@example.org"));

        // Roles: given, changed, taken away.
        pick(byName<QComboBox>(&page, QStringLiteral("roleKind")), QStringLiteral("termbase"));
        QCOMPARE(byName<QComboBox>(&page, QStringLiteral("roleResource"))->itemData(0).toString(), QStringLiteral("*"));
        pick(byName<QComboBox>(&page, QStringLiteral("roleRole")), QStringLiteral("suggest"));
        byName<QPushButton>(&page, QStringLiteral("give"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Gave the role"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().size() == 1, 10000);
        QTableWidget *roles = nullptr;
        QTRY_VERIFY((roles = byName<QTableWidget>(&page, QStringLiteral("roles"))));
        QCOMPARE(roles->item(0, 0)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("All termbases"));
        QCOMPARE(roles->item(0, 1)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("Suggest changes"));
        pick(roles->findChild<QComboBox *>(), QStringLiteral("approve"));
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Changed the role"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().first().toObject().value("role").toString() == QStringLiteral("approve"), 10000);
        pick(byName<QComboBox>(&page, QStringLiteral("roleKind")), QStringLiteral("all"));
        QVERIFY(byName<QComboBox>(&page, QStringLiteral("roleResource")) == nullptr || !byName<QComboBox>(&page, QStringLiteral("roleResource"))->parentWidget()->isVisibleTo(&page));
        byName<QPushButton>(&page, QStringLiteral("give"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().size() == 2, 10000);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/accounts.png"));
        }
        for (int left = 2; left > 0; left--) {
            QTRY_VERIFY((roles = byName<QTableWidget>(&page, QStringLiteral("roles"))));
            QPushButton *remove = nullptr;
            for (QPushButton *b : roles->findChildren<QPushButton *>()) {
                if (b->text() == QStringLiteral("Remove"))
                    remove = b;
            }
            QVERIFY(remove);
            remove->click();
            QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().size() == left - 1, 10000);
        }
        QCOMPARE(page.message(), QStringLiteral("Took the role away"));

        // Roles of one's own: a reviewer who suggests and approves but
        // does not edit, defined, given, changed, renamed and deleted.
        for (const QJsonValue &v : wait(api->get(QStringLiteral("/api/roles"))).array()) {
            if (!v.toObject().value("builtIn").toBool())
                wait(api->del(QStringLiteral("/api/roles/") + ApiClient::seg(v.toObject().value("name").toString())));
        }
        page.reload();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.roles().size() == 5, 10000);
        QTableWidget *roleTable = nullptr;
        QTRY_VERIFY((roleTable = byName<QTableWidget>(&page, QStringLiteral("roleTable"))));
        QCOMPARE(roleTable->rowCount(), 5);
        // The built-in roles show what they allow and are fixed.
        QVERIFY(byName<QCheckBox>(&page, QStringLiteral("allows:edit:suggest"))->isChecked());
        QVERIFY(!byName<QCheckBox>(&page, QStringLiteral("allows:edit:approve"))->isChecked());
        QVERIFY(!byName<QCheckBox>(&page, QStringLiteral("allows:edit:approve"))->isEnabled());
        QVERIFY(label(&page, QStringLiteral("Comes with the server")));
        QVERIFY(!byName<QPushButton>(&page, QStringLiteral("makeRole"))->isEnabled());
        byName<QLineEdit>(&page, QStringLiteral("newRoleName"))->setText(QStringLiteral("Reviewer"));
        QVERIFY(byName<QCheckBox>(&page, QStringLiteral("newRoleAllows:read"))->isChecked() && !byName<QCheckBox>(&page, QStringLiteral("newRoleAllows:read"))->isEnabled());
        byName<QCheckBox>(&page, QStringLiteral("newRoleAllows:suggest"))->click();
        byName<QCheckBox>(&page, QStringLiteral("newRoleAllows:approve"))->click();
        byName<QPushButton>(&page, QStringLiteral("makeRole"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Made the role Reviewer"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.roles().size() == 6, 10000);
        QTRY_VERIFY(byName<QCheckBox>(&page, QStringLiteral("allows:Reviewer:approve")) && byName<QCheckBox>(&page, QStringLiteral("allows:Reviewer:approve"))->isChecked());
        QVERIFY(!byName<QCheckBox>(&page, QStringLiteral("allows:Reviewer:edit"))->isChecked());
        QCOMPARE(byName<QLineEdit>(&page, QStringLiteral("newRoleName"))->text(), QString());
        // A name that is taken by a built-in role is refused, and said.
        byName<QLineEdit>(&page, QStringLiteral("newRoleName"))->setText(QStringLiteral("Edit"));
        byName<QPushButton>(&page, QStringLiteral("makeRole"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QLabel>(&page, QStringLiteral("roleProblem")), 10000);
        byName<QLineEdit>(&page, QStringLiteral("newRoleName"))->clear();
        // The role is offered where roles are given.
        pick(byName<QComboBox>(&page, QStringLiteral("roleKind")), QStringLiteral("termbase"));
        pick(byName<QComboBox>(&page, QStringLiteral("roleRole")), QStringLiteral("Reviewer"));
        byName<QPushButton>(&page, QStringLiteral("give"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().size() == 1 && page.grants().first().toObject().value("role").toString() == QStringLiteral("Reviewer"), 10000);
        QTRY_VERIFY((roles = byName<QTableWidget>(&page, QStringLiteral("roles"))));
        QCOMPARE(roles->item(0, 1)->data(Qt::AccessibleTextRole).toString(), QStringLiteral("Reviewer"));
        // What the reviewer may do on a termbase, as the server sees it.
        wait(api->del(QStringLiteral("/api/termbases/Rollenprobe")));
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "Rollenprobe"}, {"languages", QJsonArray{"de-DE"}}}}})).ok());
        const ApiResult benIn = wait(api->post(QStringLiteral("/api/auth/password"), QJsonObject{{"username", "ben.review"}, {"password", "a long enough password for ben"}, {"device", "role check"}}));
        QVERIFY2(benIn.ok(), qPrintable(benIn.error.message));
        ApiClient asBen(api->base(), benIn.object().value("token").toString());
        QCOMPARE(wait(asBen.get(QStringLiteral("/api/termbases/Rollenprobe"))).object().value("role").toString(), QStringLiteral("read+suggest+approve"));
        const QJsonObject probe{{"entry", QJsonObject{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Probe"}}}}}}}}}};
        QCOMPARE(wait(asBen.post(QStringLiteral("/api/termbases/Rollenprobe/entries"), probe)).status, 403);
        // In use, the role cannot be deleted.
        QTRY_VERIFY(byName<Web::ConfirmButton>(&page, QStringLiteral("deleteRole:Reviewer")));
        QTRY_VERIFY(!byName<Web::ConfirmButton>(&page, QStringLiteral("deleteRole:Reviewer"))->button()->isEnabled());
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty()) {
            QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
            QTest::qWait(500);
            page.resize(1280, 1500);
            QTest::qWait(300);
            page.grab().save(dir + QStringLiteral("/accounts-roles.png"));
            page.resize(1280, 1000);
        }
        // Editing is added to it: Ben may add the entry at once.
        byName<QCheckBox>(&page, QStringLiteral("allows:Reviewer:edit"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Reviewer now allows: edit"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
        QCOMPARE(wait(asBen.post(QStringLiteral("/api/termbases/Rollenprobe/entries"), probe)).status, 201);
        // Renamed, the grant follows the role.
        QLineEdit *roleName = nullptr;
        QTRY_VERIFY((roleName = byName<QLineEdit>(&page, QStringLiteral("roleName:Reviewer"))));
        roleName->setText(QStringLiteral("Lektorat"));
        emit roleName->editingFinished();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Renamed the role to Lektorat"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().size() == 1 && page.grants().first().toObject().value("role").toString() == QStringLiteral("Lektorat"), 10000);
        // Taken from Ben, it can be deleted.
        QTRY_VERIFY((roles = byName<QTableWidget>(&page, QStringLiteral("roles"))));
        for (QPushButton *b : roles->findChildren<QPushButton *>()) {
            if (b->text() == QStringLiteral("Remove")) {
                b->click();
                break;
            }
        }
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.grants().isEmpty(), 10000);
        Web::ConfirmButton *deleteRole = nullptr;
        QTRY_VERIFY((deleteRole = byName<Web::ConfirmButton>(&page, QStringLiteral("deleteRole:Lektorat"))) && deleteRole->button()->isEnabled());
        deleteRole->button()->click();
        deleteRole->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted the role Lektorat"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.roles().size() == 5, 10000);
        wait(api->del(QStringLiteral("/api/termbases/Rollenprobe")));
        wait(api->del(QStringLiteral("/api/users/%1/tokens").arg(ben)));

        // Ben signs in; the page lists it and ends it.
        const ApiResult token = wait(api->post(QStringLiteral("/api/auth/password"), QJsonObject{{"username", "ben.review"}, {"password", "a long enough password for ben"}, {"device", "test device"}}));
        QVERIFY2(token.ok(), qPrintable(token.error.message));
        page.reload();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.signIns().size() == 1, 10000);
        QTRY_VERIFY(byName<QPushButton>(&page, QStringLiteral("signOutEverywhere")));
        byName<QPushButton>(&page, QStringLiteral("signOutEverywhere"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Signed ben.review out everywhere"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && page.signIns().isEmpty(), 10000);

        // A new password; too short a one is said.
        byName<QLineEdit>(&page, QStringLiteral("newPassword"))->setText(QStringLiteral("short"));
        byName<QPushButton>(&page, QStringLiteral("setPassword"))->click();
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QLabel>(&page, QStringLiteral("problem")), 10000);
        byName<QLineEdit>(&page, QStringLiteral("newPassword"))->setText(QStringLiteral("another long enough password"));
        byName<QPushButton>(&page, QStringLiteral("setPassword"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Set a new password for ben.review; their sign-ins ended"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && !byName<QLabel>(&page, QStringLiteral("problem")), 10000);

        // Administrator and disabled.
        byName<QCheckBox>(&page, QStringLiteral("isAdmin"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("ben.review is an administrator"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QCheckBox>(&page, QStringLiteral("isAdmin"))->isChecked(), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(label(&page, QStringLiteral("As an administrator, ben.review may do everything; roles add nothing.")), 10000);
        byName<QCheckBox>(&page, QStringLiteral("isAdmin"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("ben.review is no longer an administrator"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
        byName<QCheckBox>(&page, QStringLiteral("isDisabled"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Disabled ben.review and signed them out everywhere"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy(), 10000);
        QVERIFY(!wait(api->post(QStringLiteral("/api/auth/password"), QJsonObject{{"username", "ben.review"}, {"password", "another long enough password"}, {"device", "test device"}})).ok());

        // One's own account can't be deleted here, and the last
        // administrator stays one.
        Web::Clickable *own = byName<Web::Clickable>(&page, QStringLiteral("account:%1").arg(admin));
        QTest::mouseClick(own, Qt::LeftButton);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QLabel>(&page, QStringLiteral("about")) && byName<QLabel>(&page, QStringLiteral("about"))->text().endsWith(QStringLiteral("This is you.")), 10000);
        QVERIFY(!byName<Web::ConfirmButton>(&page, QStringLiteral("deleteAccount")));
        QVERIFY(!page.signIns().isEmpty());
        bool otherAdmins = false;
        for (const QJsonValue &v : page.users())
            otherAdmins = otherAdmins || (v.toObject().value("admin").toBool() && v.toObject().value("username").toString() != admin && !v.toObject().value("disabled").toBool());
        if (!otherAdmins) {
            byName<QCheckBox>(&page, QStringLiteral("isAdmin"))->click();
            QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && byName<QLabel>(&page, QStringLiteral("problem")), 10000);
            QCOMPARE(byName<QLabel>(&page, QStringLiteral("problem"))->text(), QStringLiteral("The server needs at least one active administrator: make someone else one first."));
            QVERIFY(byName<QCheckBox>(&page, QStringLiteral("isAdmin"))->isChecked());
        }

        // Deleting the account asks first.
        page.select(ben);
        Web::ConfirmButton *del = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && (del = byName<Web::ConfirmButton>(&page, QStringLiteral("deleteAccount"))), 10000);
        del->button()->click();
        QVERIFY(label(&page, QStringLiteral("Delete ben.review? Their work stays signed with their name; their roles and sign-ins go.")));
        del->yes()->click();
        QTRY_COMPARE_WITH_TIMEOUT(page.message(), QStringLiteral("Deleted the account ben.review"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(!page.busy() && !byName<Web::Clickable>(&page, QStringLiteral("account:ben.review")), 10000);
        QCOMPARE(page.selected(), 0);
    }

    void terminologyOnTheServer()
    {
        // Termbases of the team server beside those of this computer: a
        // master made there, its entries, a project termbase submitted for
        // review and decided.
        TestCore core;
        ConnectionManager m(&core.launcher);
        m.start();
        QTRY_VERIFY_WITH_TIMEOUT(m.local()->isReady(), 30000);
        Connection *team = signIn(m);
        if (!team)
            QSKIP("set INTERLINEA_TEAM_SERVER to a running team server");
        ApiClient *api = team->api();
        for (const QString &old : {QStringLiteral("E2E Fall"), QStringLiteral("E2E Recht")})
            wait(api->del(QStringLiteral("/api/termbases/") + ApiClient::seg(old)));

        TerminologySection section(&m);
        section.resize(1360, 900);
        section.show();
        TerminologyPage *page = section.termbases();
        QTRY_VERIFY_WITH_TIMEOUT(label(page, QStringLiteral("On the team server")), 15000);
        QVERIFY(label(page, QStringLiteral("Masters")));

        // A new master, stored on the server.
        QPushButton *create = nullptr;
        for (QPushButton *b : page->findChildren<QPushButton *>()) {
            if (b->text() == QStringLiteral("New master"))
                create = b;
        }
        QVERIFY(create);
        create->click();
        QTRY_VERIFY(byName<QLineEdit>(page, QStringLiteral("newMasterName")));
        auto *place = byName<Web::Segmented>(page, QStringLiteral("newMasterPlace"));
        QVERIFY(place);
        QCOMPARE(place->segment(0)->text(), QStringLiteral("Local"));
        QCOMPARE(place->segment(1)->text(), QStringLiteral("Server"));
        place->segment(1)->click();
        byName<QLineEdit>(page, QStringLiteral("newMasterName"))->setText(QStringLiteral("E2E Recht"));
        pick(byName<QComboBox>(page, QStringLiteral("newMasterLanguage")), QStringLiteral("de-DE"));
        QTRY_VERIFY(byName<QComboBox>(page, QStringLiteral("newMasterLanguage")));
        pick(byName<QComboBox>(page, QStringLiteral("newMasterLanguage")), QStringLiteral("en-GB"));
        QPushButton *make = nullptr;
        QTRY_VERIFY([&] {
            for (QPushButton *b : page->findChildren<QPushButton *>()) {
                if (b->text() == QStringLiteral("Create master") && b->isEnabled() && !(b->isHidden() && b->testAttribute(Qt::WA_WState_ExplicitShowHide)))
                    make = b;
            }
            return make != nullptr;
        }());
        make->click();
        QTRY_VERIFY_WITH_TIMEOUT(page->termbase() == QStringLiteral("E2E Recht") && page->connectionId() == team->id(), 10000);
        QTRY_VERIFY(byName<Web::Clickable>(page, QStringLiteral("tb:%1:E2E Recht").arg(team->id())));
        // A termbase on the server is read again on demand; who manages
        // it has its settings, with who may use it.
        QVERIFY(button(page, QStringLiteral("Reload")));
        QTRY_VERIFY(byName<QPushButton>(page, QStringLiteral("termbaseSettingsButton")));
        QVERIFY(page->may("manage"));
        QVERIFY(!byName<QLabel>(page, QStringLiteral("termbaseRole")));
        qint64 rosa = 0;
        for (const QJsonValue &v : wait(api->get(QStringLiteral("/api/users"))).array()) {
            if (v.toObject().value("username").toString() == QStringLiteral("rosa"))
                wait(api->del(QStringLiteral("/api/users/%1").arg(v.toObject().value("id").toInteger())));
        }
        const ApiResult made = wait(api->post(QStringLiteral("/api/users"), QJsonObject{{"username", "rosa"}, {"name", "Rosa Lind"}, {"password", "another long password"}}));
        QVERIFY(made.ok());
        rosa = made.object().value("id").toInteger();
        TermbaseSettingsDialog *set = page->settingsDialog();
        QVERIFY(set);
        AccessPanel *access = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT(!set->busy() && (access = byName<AccessPanel>(set, QStringLiteral("termbaseAccess"))), 10000);
        QVERIFY(label(access, QStringLiteral("Who may use it")));
        // Trusted authors are chosen among the people of the server.
        QComboBox *author = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((author = byName<QComboBox>(set, QStringLiteral("trustAuthor"))) && author->findData(QStringLiteral("rosa")) > 0, 10000);
        QCOMPARE(author->itemText(author->findData(QStringLiteral("rosa"))), QStringLiteral("Rosa Lind (rosa)"));
        // Rosa may read it.
        QComboBox *who = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((who = access->findChild<QComboBox *>(QStringLiteral("who"))) && who->findData(rosa) >= 0, 10000);
        pick(who, rosa);
        pick(access->findChild<QComboBox *>(QStringLiteral("role")), QStringLiteral("read"));
        access->findChild<QPushButton *>(QStringLiteral("add"))->click();
        QTRY_VERIFY_WITH_TIMEOUT([&] {
            for (const QJsonValue &g : wait(api->get(QStringLiteral("/api/users/%1/grants").arg(rosa))).array()) {
                if (g.toObject().value("resource").toString() == QStringLiteral("E2E Recht") && g.toObject().value("role").toString() == QStringLiteral("read"))
                    return true;
            }
            return false;
        }(), 10000);
        byName<QScrollArea>(set, QStringLiteral("settingsScroll"))->ensureWidgetVisible(access, 0, 40);
        QTest::qWait(200);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty())
            set->grab().save(dir + QStringLiteral("/termbase-settings-access.png"));
        set->accept();

        // An entry, saved on the server.
        byName<QPushButton>(page, QStringLiteral("newEntry"))->click();
        QTRY_VERIFY(byName<QLineEdit>(page, QStringLiteral("newEntryText")));
        byName<QLineEdit>(page, QStringLiteral("newEntryText"))->setText(QStringLiteral("Kaution"));
        byName<QPushButton>(page, QStringLiteral("addEntry"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(page->entries().size(), 1, 10000);
        const ApiResult onServer = wait(api->get(QStringLiteral("/api/termbases/E2E%20Recht/entries")));
        QCOMPARE(onServer.object().value("total").toInt(), 1);
        // What a colleague adds on the server appears by itself.
        const QJsonObject theirs{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Miete"}, {"status", "approved"}}}}}}}};
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases/E2E%20Recht/entries"), QJsonObject{{"entry", theirs}})).ok());
        QTRY_COMPARE_WITH_TIMEOUT(page->entries().size(), 2, 15000);

        // A project termbase over it, submitted and decided.
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases"), QJsonObject{{"definition", QJsonObject{{"name", "E2E Fall"}}}, {"upstreams", QJsonArray{"E2E Recht"}}})).ok());
        const QJsonObject fresh{{"languages", QJsonArray{QJsonObject{{"lang", "de-DE"}, {"terms", QJsonArray{QJsonObject{{"text", "Staffelmiete"}, {"status", "proposed"}}}}}}}};
        QVERIFY(wait(api->post(QStringLiteral("/api/termbases/E2E%20Fall/entries"), QJsonObject{{"entry", fresh}})).ok());
        Web::Clickable *fall = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((fall = byName<Web::Clickable>(page, QStringLiteral("tb:%1:E2E Fall").arg(team->id()))), 15000);
        QTest::mouseClick(fall, Qt::LeftButton);
        QTRY_VERIFY_WITH_TIMEOUT(page->termbase() == QStringLiteral("E2E Fall") && page->entries().size() == 1, 10000);
        QPushButton *submit = nullptr;
        QTRY_VERIFY((submit = byName<QPushButton>(page, QStringLiteral("submitReview"))));
        QCOMPARE(submit->text(), QStringLiteral("Submit to E2E Recht for review"));
        byName<QLineEdit>(page, QStringLiteral("reviewTitle"))->setText(QStringLiteral("Neue Benennungen"));
        submit->click();
        ReviewPage *review = section.review();
        QTRY_VERIFY_WITH_TIMEOUT(section.reviewing() && review->items().size() == 1, 15000);
        QCOMPARE(review->mergeRequest().value("title").toString(), QStringLiteral("Neue Benennungen"));
        // Reviewers: nobody has a part yet. Rosa cannot be given one while
        // she may only read the master; once she may approve, she can.
        QTRY_VERIFY_WITH_TIMEOUT(byName<QLabel>(review, QStringLiteral("noReviewers")), 10000);
        QCOMPARE(byName<QLabel>(review, QStringLiteral("noReviewers"))->text(), QStringLiteral("Nobody is given a part: everyone who may approve changes to E2E Recht sees all of it."));
        QVERIFY(review->may("manage"));
        byName<QPushButton>(review, QStringLiteral("assign"))->click();
        QCOMPARE(review->message(), QStringLiteral("Choose who reviews."));
        pick(byName<QComboBox>(review, QStringLiteral("assignWho")), QStringLiteral("rosa"));
        byName<QPushButton>(review, QStringLiteral("assign"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(review->message(), QStringLiteral("Rosa Lind may not approve changes to E2E Recht. Give them the approve role first."), 10000);
        QVERIFY(review->reviewers().isEmpty());
        QVERIFY(wait(api->put(QStringLiteral("/api/grants"), QJsonObject{{"userId", rosa}, {"kind", "termbase"}, {"resource", "E2E Recht"}, {"role", "approve"}})).ok());
        QTRY_VERIFY(byName<QComboBox>(review, QStringLiteral("assignWho")));
        pick(byName<QComboBox>(review, QStringLiteral("assignWho")), QStringLiteral("rosa"));
        pick(byName<QComboBox>(review, QStringLiteral("assignPart")), QStringLiteral("lang:de-DE"));
        byName<QPushButton>(review, QStringLiteral("assign"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(review->message(), QStringLiteral("Rosa Lind reviews a part of the request"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(review->reviewers().size() == 1 && byName<QPushButton>(review, QStringLiteral("unassign:rosa")), 10000);
        QVERIFY(labelWith(review, QStringLiteral("entries with terms in German (Germany): 1 of 1 to decide")));
        QVERIFY(labelWith(review, QStringLiteral("; for rosa to review")));
        QTest::qWait(200);
        if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty())
            review->grab().save(dir + QStringLiteral("/review-reviewers.png"));
        // Rosa sees her part alone, and decides it with a note.
        {
            TestCore hers;
            ConnectionManager rm(&hers.launcher);
            rm.start();
            QTRY_VERIFY_WITH_TIMEOUT(rm.local()->isReady(), 30000);
            Connection *asRosa = nullptr;
            bool done = false;
            rm.signInWithPassword(ConnectionManager::serverUrl(qEnvironmentVariable("INTERLINEA_TEAM_SERVER")), QStringLiteral("rosa"), QStringLiteral("another long password"),
                                  [&](Connection *c, const ApiError &) { asRosa = c; done = true; });
            QTRY_VERIFY_WITH_TIMEOUT(done, 20000);
            QVERIFY(asRosa);
            ReviewPage hersPage;
            hersPage.resize(1360, 900);
            hersPage.show();
            hersPage.open(asRosa, review->request());
            QTRY_VERIFY2_WITH_TIMEOUT(hersPage.items().size() == 1 && byName<QCheckBox>(&hersPage, QStringLiteral("mine")),
                                      qPrintable(QStringLiteral("%1 items, %2 reviewers, as %3, %4")
                                                     .arg(hersPage.items().size())
                                                     .arg(hersPage.reviewers().size())
                                                     .arg(asRosa->userName(), byName<QLabel>(&hersPage, QStringLiteral("state")) ? byName<QLabel>(&hersPage, QStringLiteral("state"))->text() : QString())),
                                      15000);
            QVERIFY(hersPage.may("approve") && !hersPage.may("manage"));
            // She decides, but does not hand out parts.
            QVERIFY(!byName<QPushButton>(&hersPage, QStringLiteral("assign")) && !byName<QPushButton>(&hersPage, QStringLiteral("unassign:rosa")));
            byName<QCheckBox>(&hersPage, QStringLiteral("mine"))->click();
            QTRY_VERIFY_WITH_TIMEOUT(hersPage.items().size() == 1 && byName<QLineEdit>(&hersPage, QStringLiteral("comment")), 10000);
            QTest::keyClicks(byName<QLineEdit>(&hersPage, QStringLiteral("comment")), QStringLiteral("Passt"));
            byName<QPushButton>(&hersPage, QStringLiteral("approve"))->click();
            QTRY_COMPARE_WITH_TIMEOUT(hersPage.message(), QStringLiteral("Approved: 1 entry"), 10000);
        }
        // What she decided shows here by itself, with her note and name.
        QTRY_VERIFY_WITH_TIMEOUT(review->progress().value("states").toObject().value("approved").toInt() == 1, 15000);
        QTRY_VERIFY_WITH_TIMEOUT(labelWith(review, QStringLiteral("all 1 decided")), 10000);
        byName<Web::Tabs>(review, QStringLiteral("views"))->tab(1)->click();
        QTRY_VERIFY_WITH_TIMEOUT(review->view() == 1 && review->items().size() == 1 && label(review, QStringLiteral("“Passt”, rosa")), 10000);
        // The part is taken from her again, and she is back to reading.
        byName<QPushButton>(review, QStringLiteral("unassign:rosa"))->click();
        QTRY_COMPARE_WITH_TIMEOUT(review->message(), QStringLiteral("Rosa Lind no longer has a part of the request"), 10000);
        QTRY_VERIFY_WITH_TIMEOUT(review->reviewers().isEmpty() && byName<QLabel>(review, QStringLiteral("noReviewers")), 10000);
        QVERIFY(wait(api->put(QStringLiteral("/api/grants"), QJsonObject{{"userId", rosa}, {"kind", "termbase"}, {"resource", "E2E Recht"}, {"role", "read"}})).ok());
        QTRY_COMPARE_WITH_TIMEOUT(wait(api->get(QStringLiteral("/api/termbases/E2E%20Recht/entries"))).object().value("total").toInt(), 3, 10000);


        // Rosa, who may only read the master: the entries are there to
        // read, and nothing that changes them is offered.
        {
            TestCore hers;
            ConnectionManager rm(&hers.launcher);
            rm.start();
            QTRY_VERIFY_WITH_TIMEOUT(rm.local()->isReady(), 30000);
            Connection *asRosa = nullptr;
            bool done = false;
            rm.signInWithPassword(ConnectionManager::serverUrl(qEnvironmentVariable("INTERLINEA_TEAM_SERVER")), QStringLiteral("rosa"), QStringLiteral("another long password"),
                                  [&](Connection *c, const ApiError &) { asRosa = c; done = true; });
            QTRY_VERIFY_WITH_TIMEOUT(done, 20000);
            QVERIFY(asRosa);
            TerminologyPage reader(&rm);
            reader.resize(1360, 900);
            reader.show();
            Web::Clickable *recht = nullptr;
            QTRY_VERIFY_WITH_TIMEOUT((recht = byName<Web::Clickable>(&reader, QStringLiteral("tb:%1:E2E Recht").arg(asRosa->id()))), 15000);
            // The project termbase is not hers to see.
            QVERIFY(!byName<Web::Clickable>(&reader, QStringLiteral("tb:%1:E2E Fall").arg(asRosa->id())));
            QTest::mouseClick(recht, Qt::LeftButton);
            QTRY_VERIFY_WITH_TIMEOUT(reader.termbase() == QStringLiteral("E2E Recht") && reader.entries().size() == 3, 15000);
            QVERIFY(reader.may("read") && !reader.may("edit") && !reader.may("manage"));
            QTRY_VERIFY(byName<QLabel>(&reader, QStringLiteral("termbaseRole")));
            QCOMPARE(byName<QLabel>(&reader, QStringLiteral("termbaseRole"))->text(), QStringLiteral("Read"));
            QVERIFY(!byName<QPushButton>(&reader, QStringLiteral("termbaseSettingsButton")));
            QVERIFY(!byName<QPushButton>(&reader, QStringLiteral("newEntry")));
            QVERIFY(!button(&reader, QStringLiteral("Import")));
            QVERIFY(button(&reader, QStringLiteral("Export")) && button(&reader, QStringLiteral("Reload")));
            QTRY_VERIFY(byName<QLabel>(&reader, QStringLiteral("entryTitle")));
            QVERIFY(!byName<Web::ConfirmButton>(&reader, QStringLiteral("deleteEntry")));
            QVERIFY(!byName<QPushButton>(&reader, QStringLiteral("addField")));
            auto *views = byName<Web::Tabs>(&reader, QStringLiteral("entryViews"));
            QVERIFY(views && !byName<QPushButton>(&reader, QStringLiteral("newEntry")));
            QVERIFY(views->tab(1)->isHidden());
            QTest::qWait(200);
            if (const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS"); !dir.isEmpty())
                reader.grab().save(dir + QStringLiteral("/terminology-reader.png"));
        }
        wait(api->del(QStringLiteral("/api/users/%1").arg(rosa)));
        wait(api->del(QStringLiteral("/api/termbases/") + ApiClient::seg(QStringLiteral("E2E Fall"))));
        wait(api->del(QStringLiteral("/api/termbases/") + ApiClient::seg(QStringLiteral("E2E Recht"))));
    }

private:
    QTemporaryDir m_tmp;
};

QTEST_MAIN(TestEndToEnd)
#include "tst_endtoend.moc"
