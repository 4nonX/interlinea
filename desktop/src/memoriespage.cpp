#include "memoriespage.h"

#include "roles.h"

#include "accesspanel.h"
#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "segmentcontent.h"
#include "theme.h"
#include "webui.h"

#include <QCheckBox>
#include <QDateTime>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRegularExpression>
#include <QResizeEvent>
#include <QVBoxLayout>

namespace {
const QString tmxFilter()
{
    return MemoriesPage::tr("TMX files (*.tmx);;All files (*)");
}

bool sameName(const QString &a, const QString &b)
{
    return a.compare(b, Qt::CaseInsensitive) == 0;
}

// Whether a project's or template's settings use a memory.
bool uses(const QJsonObject &memories, const QString &name)
{
    for (const QJsonValue &list : memories) {
        for (const QJsonValue &m : list.toArray()) {
            if (sameName(m.toObject().value(QLatin1String("memory")).toString(), name))
                return true;
        }
    }
    return false;
}
} // namespace

MemoriesPage::MemoriesPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
{
    setObjectName(QStringLiteral("memories"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    Web::Page page = Web::page(this);
    page.body->addWidget(Web::header(tr("Memories"),
                                     tr("Translation memories, in the core. Projects choose which to use for each target language."), {},
                                     page.scroll->widget()));
    m_sections = new QVBoxLayout;
    m_sections->setSpacing(24);
    page.body->addLayout(m_sections);
    page.body->addStretch();
    outer->addWidget(page.scroll);
    m_toast = new Web::Toast(this);

    // Changes that follow one another closely are read once.
    m_reload.setSingleShot(true);
    m_reload.setInterval(150);
    connect(&m_reload, &QTimer::timeout, this, &MemoriesPage::reload);
    connect(manager, &ConnectionManager::added, this, &MemoriesPage::syncParts);
    connect(manager, &ConnectionManager::changed, this, &MemoriesPage::syncParts);
    connect(manager, &ConnectionManager::removed, this, [this](const QString &id) {
        m_listening.remove(id);
        syncParts();
    });
    syncParts();
}

MemoriesPage::~MemoriesPage()
{
    qDeleteAll(m_parts);
}

void MemoriesPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

void MemoriesPage::notify(const QString &text)
{
    m_toast->say(text);
}

QString MemoriesPage::message() const
{
    return m_toast->last();
}

QString MemoriesPage::units(qint64 n)
{
    return n == 1 ? tr("1 unit") : tr("%1 units").arg(QLocale().toString(n));
}

QString MemoriesPage::date(const QString &iso)
{
    // The server writes times to the nanosecond.
    QString text = iso;
    text.replace(QRegularExpression(QStringLiteral("(\\.\\d{3})\\d+")), QStringLiteral("\\1"));
    const QDateTime t = QDateTime::fromString(text, Qt::ISODateWithMs);
    return t.isValid() ? QLocale().toString(t.toLocalTime().date(), QStringLiteral("d MMM yyyy")) : QString();
}

void MemoriesPage::clear(QVBoxLayout *layout)
{
    while (QLayoutItem *item = layout->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        }
        delete item;
    }
}

MemoriesPage::Part *MemoriesPage::part(const QString &connectionId) const
{
    for (Part *p : m_parts) {
        if (p->conn && p->conn->id() == connectionId)
            return p;
    }
    return nullptr;
}

QString MemoriesPage::openMemory(const QString &connectionId) const
{
    const Part *p = part(connectionId);
    return p ? p->open : QString();
}

QJsonObject MemoriesPage::memory(const Part *p, const QString &name) const
{
    for (const QJsonValue &v : p->memories) {
        if (sameName(v.toObject().value(QLatin1String("name")).toString(), name))
            return v.toObject();
    }
    return {};
}

QString MemoriesPage::ownerProject(const Part *p, const QString &memory) const
{
    // A project's own memory is named after it and its language, and is
    // the project's to fill.
    for (const QJsonValue &v : p->projects) {
        const QJsonObject s = v.toObject().value(QLatin1String("settings")).toObject();
        if (!s.value(QLatin1String("projectMemory")).toBool())
            continue;
        const QString project = s.value(QLatin1String("name")).toString();
        const QJsonObject memories = s.value(QLatin1String("memories")).toObject();
        for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray()) {
            if (!sameName(QStringLiteral("%1 (%2)").arg(project, t.toString()), memory))
                continue;
            if (uses(QJsonObject{{t.toString(), memories.value(t.toString())}}, memory))
                return project;
        }
    }
    return {};
}

QStringList MemoriesPage::usedBy(const Part *p, const QString &memory) const
{
    QStringList out;
    for (const QJsonValue &v : p->projects) {
        const QJsonObject s = v.toObject().value(QLatin1String("settings")).toObject();
        if (uses(s.value(QLatin1String("memories")).toObject(), memory))
            out << s.value(QLatin1String("name")).toString();
    }
    for (const QJsonValue &v : p->templates) {
        const QJsonObject t = v.toObject();
        if (uses(t.value(QLatin1String("memories")).toObject(), memory))
            out << tr("template %1").arg(t.value(QLatin1String("name")).toString());
    }
    return out;
}

void MemoriesPage::syncParts()
{
    // What changes on a place is read again.
    for (Connection *c : m_manager->all()) {
        if (m_listening.contains(c->id()))
            continue;
        m_listening << c->id();
        connect(c->feed(), &EventFeed::event, this, [this](const QString &type) {
            if (type == QLatin1String("memory") || type == QLatin1String("account") || type == QLatin1String("project"))
                m_reload.start();
        });
        connect(c->feed(), &EventFeed::reset, this, [this] { m_reload.start(); });
    }

    // The team servers first, then this computer, as on the web.
    QList<Connection *> wanted;
    for (Connection *c : m_manager->teams()) {
        if (c->isReady())
            wanted << c;
    }
    if (m_manager->local() && m_manager->local()->isReady())
        wanted << m_manager->local();
    bool same = wanted.size() == m_parts.size();
    for (int i = 0; same && i < wanted.size(); ++i)
        same = m_parts.at(i)->conn == wanted.at(i);
    if (same)
        return;

    QHash<QString, QString> open;
    for (Part *p : std::as_const(m_parts)) {
        if (p->conn)
            open.insert(p->conn->id(), p->open);
        p->root->hide();
        p->root->deleteLater();
    }
    qDeleteAll(m_parts);
    m_parts.clear();
    while (QLayoutItem *item = m_sections->takeAt(0))
        delete item;

    QWidget *parent = m_sections->parentWidget();
    const bool severalServers = m_manager->teams().size() > 1;
    for (Connection *c : std::as_const(wanted)) {
        auto *p = new Part;
        p->conn = c;
        p->open = open.value(c->id());
        if (c->isLocal()) {
            p->root = new QWidget(parent);
            auto *v = new QVBoxLayout(p->root);
            v->setContentsMargins(0, 0, 0, 0);
            v->setSpacing(24);
            QVBoxLayout *mainBody = nullptr;
            v->addWidget(Web::section(tr("Main memories on this computer"), p->root, &mainBody));
            p->problem = Web::text(QString(), "error", p->root);
            p->problem->hide();
            mainBody->addWidget(p->problem);
            p->list = new QVBoxLayout;
            p->list->setSpacing(8);
            mainBody->addLayout(p->list);
            p->formHolder = new QVBoxLayout;
            mainBody->addLayout(p->formHolder);
            QVBoxLayout *ownBody = nullptr;
            p->ownSection = Web::section(tr("Project memories"), p->root, &ownBody);
            ownBody->addWidget(Web::text(tr("Each collects one project's confirmed translations. Add them to a main memory from the project's settings; "
                                            "they are deleted with the project."),
                                         "help", p->ownSection));
            p->ownList = new QVBoxLayout;
            ownBody->addLayout(p->ownList);
            p->ownSection->hide();
            v->addWidget(p->ownSection);
            p->list->addWidget(Web::text(tr("Loading the memories…"), "muted", p->root));
        } else {
            QVBoxLayout *body = nullptr;
            p->root = Web::section(severalServers ? tr("On the team server %1").arg(c->displayName()) : tr("On the team server"), parent, &body);
            p->problem = Web::text(QString(), "error", p->root);
            p->problem->hide();
            body->addWidget(p->problem);
            p->list = new QVBoxLayout;
            body->addLayout(p->list);
            p->detail = new QVBoxLayout;
            p->detail->setSpacing(16);
            body->addLayout(p->detail);
            p->formHolder = new QVBoxLayout;
            body->addLayout(p->formHolder);
            p->list->addWidget(Web::text(tr("Loading the memories on %1…").arg(c->displayName()), "muted", p->root));
        }
        m_sections->addWidget(p->root);
        m_parts << p;
    }
    reload();
}

void MemoriesPage::reload()
{
    if (m_parts.isEmpty()) {
        emit loaded();
        return;
    }
    for (Part *p : std::as_const(m_parts))
        load(p);
}

void MemoriesPage::load(Part *p)
{
    if (!p->conn)
        return;
    if (p->pending > 0) {
        p->again = true;
        return;
    }
    const QString id = p->conn->id();
    ApiClient *api = p->conn->api();
    p->pending = p->conn->isLocal() ? 3 : 1;
    const int reading = p->reading = ++m_reading;
    onResult(api->get(QStringLiteral("/api/memories")), this, [this, id, reading](const ApiResult &r) {
        Part *q = part(id);
        if (!q || q->reading != reading)
            return;
        if (r.ok()) {
            q->memories = r.array();
            q->problem->hide();
        } else {
            q->problem->setText(r.error.message);
            q->problem->show();
        }
        finished(q);
    });
    if (!p->conn->isLocal())
        return;
    // What uses each memory, and which belong to a project.
    onResult(api->get(QStringLiteral("/api/projects")), this, [this, id, reading](const ApiResult &r) {
        Part *q = part(id);
        if (!q || q->reading != reading)
            return;
        if (r.ok())
            q->projects = r.array();
        finished(q);
    });
    onResult(api->get(QStringLiteral("/api/templates")), this, [this, id, reading](const ApiResult &r) {
        Part *q = part(id);
        if (!q || q->reading != reading)
            return;
        if (r.ok())
            q->templates = r.array();
        finished(q);
    });
}

void MemoriesPage::finished(Part *p)
{
    if (--p->pending > 0)
        return;
    p->read = true;
    showPart(p);
    if (p->again) {
        p->again = false;
        load(p);
        return;
    }
    for (const Part *q : std::as_const(m_parts)) {
        if (q->pending > 0 || !q->read)
            return;
    }
    emit loaded();
}

void MemoriesPage::showPart(Part *p)
{
    if (!p->conn)
        return;
    if (p->conn->isLocal())
        showLocal(p);
    else
        showServer(p);
}

void MemoriesPage::showLocal(Part *p)
{
    QList<QJsonObject> main, own;
    for (const QJsonValue &v : std::as_const(p->memories)) {
        const QJsonObject m = v.toObject();
        (ownerProject(p, m.value(QLatin1String("name")).toString()).isEmpty() ? main : own) << m;
    }
    clear(p->list);
    if (main.isEmpty()) {
        p->list->addWidget(Web::text(tr("No memory yet."), "muted", p->root));
    } else {
        p->list->addWidget(localTable(p, main, true));
        p->list->addWidget(Web::text(tr("A memory a project or template uses can't be deleted; remove it there first."), "help", p->root));
    }
    clear(p->ownList);
    if (!own.isEmpty())
        p->ownList->addWidget(localTable(p, own, false));
    p->ownSection->setVisible(!own.isEmpty());
    if (p->formHolder->count() == 0)
        localForm(p, false);
}

QWidget *MemoriesPage::localTable(Part *p, const QList<QJsonObject> &memories, bool main)
{
    auto *table = new Web::Table({tr("Memory"), tr("Languages"), tr("Units"), tr("Used by"), QString()}, p->root);
    table->setAccessibleName(main ? tr("Main memories on this computer") : tr("Project memories"));
    table->setNumeric({2});
    table->setStretch(0);
    table->setStretch(3);
    const QString id = p->conn->id();
    for (const QJsonObject &m : memories) {
        const QString name = m.value(QLatin1String("name")).toString();
        const QString client = m.value(QLatin1String("client")).toString();
        const qint64 count = m.value(QLatin1String("units")).toInteger();
        QStringList users = usedBy(p, name);
        const QString sub = (client.isEmpty() ? QString() : client + QStringLiteral(", ")) + tr("created %1").arg(date(m.value(QLatin1String("createdAt")).toString()));
        QWidget *used = users.isEmpty() ? Web::cellText(tr("No project"), table, false, true) : Web::cellText(users.join(QLatin1String(", ")), table);

        QList<QWidget *> actions;
        if (main) {
            auto *import = Web::button(tr("Import TMX"), Web::Small, table, QStringLiteral("upload"));
            import->setAccessibleName(tr("Import TMX into %1").arg(name));
            connect(import, &QPushButton::clicked, this, [this, id, name] {
                const QString path = QFileDialog::getOpenFileName(this, tr("Import TMX into %1").arg(name), QString(), tmxFilter());
                if (!path.isEmpty())
                    importFile(id, name, path);
            });
            actions << import;
        }
        auto *exportButton = Web::button(tr("Export TMX"), Web::Quiet | Web::Small, table, QStringLiteral("download"));
        exportButton->setAccessibleName(tr("Export %1 as TMX").arg(name));
        exportButton->setEnabled(count > 0);
        connect(exportButton, &QPushButton::clicked, this, [this, id, name] {
            QString file = name;
            file.replace(QRegularExpression(QStringLiteral("[^\\p{L}\\p{N}]+")), QStringLiteral("_"));
            const QString path = QFileDialog::getSaveFileName(this, tr("Export %1 as TMX").arg(name), file + QStringLiteral(".tmx"), tmxFilter());
            if (!path.isEmpty())
                exportFile(id, name, path);
        });
        actions << exportButton;
        if (main) {
            auto *del = new Web::ConfirmButton(tr("Delete"), tr("Delete %1 and its %2?").arg(name, units(count)), tr("Delete"), table, true, QStringLiteral("trash"));
            del->button()->setAccessibleName(tr("Delete %1").arg(name));
            del->setAvailable(users.isEmpty());
            connect(del, &Web::ConfirmButton::confirmed, this, [this, id, name] {
                Part *q = part(id);
                if (!q)
                    return;
                onResult(q->conn->api()->del(QStringLiteral("/api/memories/%1").arg(ApiClient::seg(name))), this, [this, name](const ApiResult &r) {
                    notify(r.ok() ? tr("Deleted the memory %1").arg(name) : r.error.message);
                    reload();
                });
            });
            actions << del;
        }
        table->addRow({Web::cell(name, sub, table),
                       Web::langPair(m.value(QLatin1String("sourceLang")).toString(), {m.value(QLatin1String("targetLang")).toString()}, table),
                       Web::cellText(QLocale().toString(count), table, true), used, Web::row(actions, table)});
    }
    table->fit();
    return table;
}

void MemoriesPage::setNewMemoryFile(const QString &path)
{
    m_newFile = path;
    if (m_newFileLabel)
        m_newFileLabel->setText(path.isEmpty() ? tr("No file chosen") : QFileInfo(path).fileName());
}

void MemoriesPage::localForm(Part *p, bool open)
{
    clear(p->formHolder);
    m_newFile.clear();
    const QString id = p->conn->id();
    if (!open) {
        auto *b = Web::button(tr("New memory"), Web::Small, p->root, QStringLiteral("plus"));
        connect(b, &QPushButton::clicked, this, [this, id] {
            if (Part *q = part(id))
                localForm(q, true);
        });
        p->formHolder->addWidget(b, 0, Qt::AlignLeft);
        return;
    }
    QVBoxLayout *body = nullptr;
    QFrame *panel = Web::panel(tr("New memory"), p->root, &body);
    auto *name = new QLineEdit(panel);
    auto *client = new QLineEdit(panel);
    auto *source = new LanguageCombo(panel);
    source->setLanguage(QStringLiteral("de-DE"));
    auto *target = new LanguageCombo(panel);
    target->setLanguage(QStringLiteral("en-GB"));
    auto *choose = Web::button(tr("Choose file"), Web::Small, panel);
    choose->setAccessibleName(tr("Fill it from a TMX file"));
    auto *chosen = Web::text(tr("No file chosen"), "secondary", panel);
    chosen->setWordWrap(false);
    m_newFileLabel = chosen;
    auto *grid = new QGridLayout;
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setHorizontalSpacing(16);
    grid->setVerticalSpacing(16);
    grid->setColumnStretch(0, 1);
    grid->setColumnStretch(1, 1);
    grid->addWidget(Web::field(tr("Name"), name, panel), 0, 0);
    grid->addWidget(Web::field(tr("Client"), client, panel, true), 0, 1);
    grid->addWidget(Web::field(tr("Source language"), source, panel), 1, 0);
    grid->addWidget(Web::field(tr("Target language"), target, panel), 1, 1);
    grid->addWidget(Web::field(tr("Fill it from a TMX file"), Web::row({choose, chosen}, panel), panel, true), 2, 0);
    body->addLayout(grid);
    auto *error = Web::text(QString(), "error", panel);
    error->hide();
    body->addWidget(error);
    auto *create = Web::button(tr("Create memory"), Web::Primary | Web::Small, panel);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, panel);
    body->addWidget(Web::row({create, cancel}, panel));
    auto *help = Web::text(QString(), "help", panel);
    body->addWidget(help);
    p->formHolder->addWidget(panel);

    // What stops the memory from being made, as the web's form says it.
    auto problem = [this, id, name, source, target]() -> QString {
        const QString n = name->text().trimmed();
        if (n.isEmpty())
            return tr("Give the memory a name.");
        const Part *q = part(id);
        if (q && !memory(q, n).isEmpty())
            return tr("A memory with this name exists already.");
        if (source->language().isEmpty() || target->language().isEmpty())
            return tr("Choose a source and a target language.");
        if (source->language() == target->language())
            return tr("Source and target language must differ.");
        return {};
    };
    auto check = [problem, name, source, target, create, error, help] {
        const QString what = problem();
        create->setEnabled(what.isEmpty());
        error->setText(what);
        error->setVisible(!what.isEmpty() && !name->text().isEmpty());
        help->setText(tr("%1 to %2. Projects with these languages can use it.").arg(Languages::name(source->language()), Languages::name(target->language())));
    };
    connect(name, &QLineEdit::textChanged, panel, check);
    connect(source, &QComboBox::currentTextChanged, panel, check);
    connect(target, &QComboBox::currentTextChanged, panel, check);
    check();

    connect(choose, &QPushButton::clicked, this, [this] {
        const QString path = QFileDialog::getOpenFileName(this, tr("Fill it from a TMX file"), QString(), tmxFilter());
        if (!path.isEmpty())
            setNewMemoryFile(path);
    });
    connect(cancel, &QPushButton::clicked, this, [this, id] {
        if (Part *q = part(id))
            localForm(q, false);
    });
    auto submit = [this, id, problem, name, client, source, target, create, error] {
        if (!problem().isEmpty())
            return;
        Part *q = part(id);
        if (!q)
            return;
        // The file is read first: a memory is not made for a file that can't be read.
        QByteArray tmx;
        const QString path = m_newFile;
        if (!path.isEmpty()) {
            QFile f(path);
            if (!f.open(QIODevice::ReadOnly)) {
                error->setText(tr("Could not read %1: %2").arg(QFileInfo(path).fileName(), f.errorString()));
                error->show();
                return;
            }
            tmx = f.readAll();
        }
        const QString made = name->text().trimmed();
        create->setEnabled(false);
        QPointer<QLabel> err(error);
        QPointer<QPushButton> button(create);
        auto fail = [err, button](const QString &text) {
            if (err) {
                err->setText(text);
                err->show();
            }
            if (button)
                button->setEnabled(true);
        };
        auto done = [this, id, made](qint64 stored) {
            notify(stored > 0 ? tr("Created the memory %1 with %2").arg(made, units(stored)) : tr("Created the memory %1").arg(made));
            if (Part *r = part(id))
                localForm(r, false);
            reload();
        };
        ApiClient *api = q->conn->api();
        onResult(api->post(QStringLiteral("/api/memories"),
                           QJsonObject{{QStringLiteral("name"), made},
                                       {QStringLiteral("client"), client->text().trimmed()},
                                       {QStringLiteral("sourceLang"), source->language()},
                                       {QStringLiteral("targetLang"), target->language()}}),
                 this, [this, api, made, tmx, path, fail, done](const ApiResult &r) {
                     if (!r.ok()) {
                         fail(r.error.message);
                         return;
                     }
                     if (path.isEmpty()) {
                         done(0);
                         return;
                     }
                     const QString at = QStringLiteral("/api/memories/%1").arg(ApiClient::seg(made));
                     onResult(api->upload("POST", at + QStringLiteral("/import"), tmx, QStringLiteral("application/x-tmx+xml")), this,
                              [this, api, at, path, fail, done](const ApiResult &i) {
                                  if (i.ok()) {
                                      done(i.object().value(QLatin1String("stored")).toInteger());
                                      return;
                                  }
                                  // No memory is left behind for a file that isn't TMX.
                                  const QString why = i.error.message;
                                  onResult(api->del(at), this, [this, path, why, fail](const ApiResult &) {
                                      fail(tr("Could not read %1: %2").arg(QFileInfo(path).fileName(), why));
                                      reload();
                                  });
                              });
                 });
    };
    connect(create, &QPushButton::clicked, panel, submit);
    connect(name, &QLineEdit::returnPressed, panel, submit);
    connect(client, &QLineEdit::returnPressed, panel, submit);
    name->setFocus();
}

void MemoriesPage::importFile(const QString &connectionId, const QString &memoryName, const QString &path)
{
    Part *p = part(connectionId);
    if (!p)
        return;
    const QString file = QFileInfo(path).fileName();
    QFile f(path);
    if (!f.open(QIODevice::ReadOnly)) {
        notify(tr("Could not import %1: %2").arg(file, f.errorString()));
        return;
    }
    const bool local = p->conn->isLocal();
    onResult(p->conn->api()->upload("POST", QStringLiteral("/api/memories/%1/import").arg(ApiClient::seg(memoryName)), f.readAll(),
                                    QStringLiteral("application/x-tmx+xml")),
             this, [this, local, memoryName, file](const ApiResult &r) {
                 if (!r.ok()) {
                     notify(local ? tr("Could not import %1: %2").arg(file, r.error.message) : r.error.message);
                     return;
                 }
                 const QJsonObject st = r.object();
                 const qint64 stored = st.value(QLatin1String("stored")).toInteger();
                 const qint64 skipped = st.value(QLatin1String("skipped")).toInteger();
                 if (local) {
                     QString text = tr("Imported %1 into %2").arg(units(stored), memoryName);
                     if (skipped > 0)
                         text += tr("; %1 without both languages skipped").arg(QLocale().toString(skipped));
                     notify(text);
                 } else {
                     notify(tr("Read %1 from %2; %3 stored").arg(units(st.value(QLatin1String("read")).toInteger()), file, QLocale().toString(stored)));
                 }
                 reload();
             });
}

void MemoriesPage::exportFile(const QString &connectionId, const QString &memoryName, const QString &path)
{
    Part *p = part(connectionId);
    if (!p)
        return;
    onResult(p->conn->api()->get(QStringLiteral("/api/memories/%1/export").arg(ApiClient::seg(memoryName))), this, [this, path](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        QFile f(path);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size()) {
            notify(tr("Could not save %1: %2").arg(QFileInfo(path).fileName(), f.errorString()));
            return;
        }
        notify(tr("Saved %1").arg(QFileInfo(path).fileName()));
    });
}

void MemoriesPage::showServer(Part *p)
{
    const QString id = p->conn->id();
    const QString host = p->conn->displayName();
    clear(p->list);
    if (p->memories.isEmpty()) {
        p->list->addWidget(Web::text(tr("No memories on %1 for you yet.").arg(host), "muted", p->root));
    } else {
        auto *table = new Web::Table({tr("Memory"), tr("Languages"), tr("Units"), tr("You may"), QString()}, p->root);
        table->setAccessibleName(tr("Memories on %1").arg(host));
        table->setNumeric({2});
        table->setStretch(0);
        for (const QJsonValue &v : std::as_const(p->memories)) {
            const QJsonObject m = v.toObject();
            const QString name = m.value(QLatin1String("name")).toString();
            const bool isOpen = sameName(name, p->open);
            auto *toggle = Web::button(isOpen ? tr("Close") : tr("Open"), Web::Quiet | Web::Small, table);
            toggle->setAccessibleName(isOpen ? tr("Close %1").arg(name) : tr("Open %1").arg(name));
            connect(toggle, &QPushButton::clicked, this, [this, id, name, isOpen] { open(id, isOpen ? QString() : name); });
            table->addRow({Web::cell(name, tr("Since %1").arg(date(m.value(QLatin1String("createdAt")).toString())), table),
                           Web::langPair(m.value(QLatin1String("sourceLang")).toString(), {m.value(QLatin1String("targetLang")).toString()}, table),
                           Web::cellText(QLocale().toString(m.value(QLatin1String("units")).toInteger()), table, true),
                           Web::cellText(m.value(QLatin1String("role")).toString(), table), Web::row({toggle}, table)});
        }
        table->fit();
        p->list->addWidget(table);
    }

    // The opened memory stays as it is while its list is read again, unless
    // it went away or the person's role on it changed.
    const QJsonObject current = p->open.isEmpty() ? QJsonObject() : memory(p, p->open);
    if (!p->open.isEmpty() && current.isEmpty())
        p->open.clear();
    if (p->open.isEmpty() || p->detail->count() == 0 || current.value(QLatin1String("role")).toString() != p->openRole) {
        showDetail(p);
    } else if (p->openDelete) {
        p->openDelete->setQuestion(tr("Delete %1 and its %2 on the server for everyone?")
                                       .arg(current.value(QLatin1String("name")).toString(), units(current.value(QLatin1String("units")).toInteger())));
    }
    if (p->formHolder->count() == 0)
        serverForm(p, false);
}

void MemoriesPage::open(const QString &connectionId, const QString &name)
{
    Part *p = part(connectionId);
    if (!p || p->conn->isLocal())
        return;
    p->open = name;
    clear(p->detail);
    if (p->read)
        showServer(p);
}

void MemoriesPage::showDetail(Part *p)
{
    clear(p->detail);
    p->openDelete = nullptr;
    p->openRole.clear();
    p->detail->setContentsMargins(0, 0, 0, 0);
    const QJsonObject m = p->open.isEmpty() ? QJsonObject() : memory(p, p->open);
    if (m.isEmpty())
        return;
    p->detail->setContentsMargins(0, 16, 0, 16);
    const QString id = p->conn->id();
    const QString name = m.value(QLatin1String("name")).toString();
    const QString role = m.value(QLatin1String("role")).toString();
    const QString source = m.value(QLatin1String("sourceLang")).toString();
    const QString target = m.value(QLatin1String("targetLang")).toString();
    p->openRole = role;
    const bool edit = Roles::allows(role, QStringLiteral("edit"));
    const bool manage = Roles::allows(role, QStringLiteral("manage"));

    QVBoxLayout *body = nullptr;
    QFrame *panel = Web::panel(name, p->root, &body);

    // Search the units, in the sources or in the translations.
    auto *query = new QLineEdit(panel);
    query->setMinimumWidth(240);
    query->setClearButtonEnabled(true);
    auto *inTarget = new QCheckBox(tr("In the translations"), panel);
    auto *search = Web::button(tr("Search"), Web::Normal, panel, QStringLiteral("search"));
    search->setEnabled(false);
    auto *form = new QWidget(panel);
    form->setAccessibleName(tr("Search %1").arg(name));
    auto *fl = new QHBoxLayout(form);
    fl->setContentsMargins(0, 0, 0, 0);
    fl->setSpacing(12);
    fl->addWidget(Web::labelled(tr("Search the units"), query, form), 0, Qt::AlignBottom);
    fl->addWidget(inTarget, 0, Qt::AlignBottom);
    fl->addWidget(search, 0, Qt::AlignBottom);
    fl->addStretch();
    body->addWidget(form);
    auto *results = new QWidget(panel);
    auto *rl = new QVBoxLayout(results);
    rl->setContentsMargins(0, 0, 0, 0);
    results->hide();
    body->addWidget(results);
    connect(query, &QLineEdit::textChanged, search, [search](const QString &t) { search->setEnabled(!t.trimmed().isEmpty()); });
    auto run = [this, id, name, source, target, query, inTarget, results, rl] {
        Part *q = part(id);
        if (!q || query->text().trimmed().isEmpty())
            return;
        QPointer<QWidget> holder(results);
        onResult(q->conn->api()->post(QStringLiteral("/api/memories/%1/concordance").arg(ApiClient::seg(name)),
                                      QJsonObject{{QStringLiteral("query"), query->text()}, {QStringLiteral("inTarget"), inTarget->isChecked()}}),
                 this, [this, holder, rl, source, target](const ApiResult &r) {
                     if (!holder)
                         return;
                     if (!r.ok()) {
                         notify(r.error.message);
                         return;
                     }
                     clear(rl);
                     if (r.array().isEmpty()) {
                         rl->addWidget(Web::text(tr("Nothing found."), "muted", holder));
                     } else {
                         auto *table = new Web::Table({source, target, tr("Changed")}, holder);
                         table->setAccessibleName(tr("Units found"));
                         table->setStretch(0);
                         table->setStretch(1);
                         for (const QJsonValue &v : r.array()) {
                             const QJsonObject u = v.toObject().value(QLatin1String("unit")).toObject();
                             // Who changed it and when; a unit read from TMX may name nobody.
                             QStringList parts{u.value(QLatin1String("changedBy")).toString(), date(u.value(QLatin1String("changedAt")).toString())};
                             parts.removeAll(QString());
                             const QString changed = parts.join(QLatin1String(", "));
                             auto *by = Web::text(changed, "cellSub", table);
                             by->setAccessibleName(changed);
                             table->addRow({Web::cellText(SegmentContent::plainText(u.value(QLatin1String("source")).toArray()), table),
                                            Web::cellText(SegmentContent::plainText(u.value(QLatin1String("target")).toArray()), table), by});
                         }
                         table->fit();
                         rl->addWidget(table);
                     }
                     holder->show();
                 });
    };
    connect(search, &QPushButton::clicked, panel, run);
    connect(query, &QLineEdit::returnPressed, panel, run);

    // TMX out and in, and deleting, for those who may.
    QList<QWidget *> actions;
    auto *exportButton = Web::button(tr("Export TMX"), Web::Normal, panel, QStringLiteral("download"));
    connect(exportButton, &QPushButton::clicked, this, [this, id, name] {
        const QString path = QFileDialog::getSaveFileName(this, tr("Export %1 as TMX").arg(name), name + QStringLiteral(".tmx"), tmxFilter());
        if (!path.isEmpty())
            exportFile(id, name, path);
    });
    actions << exportButton;
    if (edit) {
        auto *import = Web::button(tr("Import TMX"), Web::Normal, panel, QStringLiteral("upload"));
        import->setAccessibleName(tr("TMX file for %1").arg(name));
        connect(import, &QPushButton::clicked, this, [this, id, name] {
            const QString path = QFileDialog::getOpenFileName(this, tr("TMX file for %1").arg(name), QString(), tmxFilter());
            if (!path.isEmpty())
                importFile(id, name, path);
        });
        actions << import;
    }
    if (manage) {
        auto *del = new Web::ConfirmButton(tr("Delete the memory"),
                                           tr("Delete %1 and its %2 on the server for everyone?").arg(name, units(m.value(QLatin1String("units")).toInteger())),
                                           tr("Delete"), panel, false);
        p->openDelete = del;
        connect(del, &Web::ConfirmButton::confirmed, this, [this, id, name] {
            Part *q = part(id);
            if (!q)
                return;
            onResult(q->conn->api()->del(QStringLiteral("/api/memories/%1").arg(ApiClient::seg(name))), this, [this, id, name](const ApiResult &r) {
                if (!r.ok()) {
                    notify(r.error.message);
                    return;
                }
                notify(tr("Deleted %1 on the server").arg(name));
                open(id, QString());
                reload();
            });
        });
        actions << del;
    }
    body->addWidget(Web::row(actions, panel));
    p->detail->addWidget(panel);
    if (manage)
        p->detail->addWidget(new AccessPanel(p->conn, QStringLiteral("memory"), name, tr("Who may use %1").arg(name), p->root));
}

void MemoriesPage::serverForm(Part *p, bool open)
{
    clear(p->formHolder);
    const QString id = p->conn->id();
    if (!open) {
        auto *b = Web::button(tr("New memory on the server"), Web::Normal, p->root);
        connect(b, &QPushButton::clicked, this, [this, id] {
            if (Part *q = part(id))
                serverForm(q, true);
        });
        p->formHolder->addWidget(b, 0, Qt::AlignLeft);
        return;
    }
    auto *form = new QWidget(p->root);
    form->setAccessibleName(tr("New memory on the server"));
    auto *v = new QVBoxLayout(form);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(12);
    auto *h = new QHBoxLayout;
    h->setSpacing(12);
    auto *name = new QLineEdit(form);
    name->setMinimumWidth(240);
    auto *source = new LanguageCombo(form);
    source->setLanguage(QStringLiteral("de-DE"));
    auto *target = new LanguageCombo(form);
    target->setLanguage(QStringLiteral("en-GB"));
    auto *make = Web::button(tr("Make the memory"), Web::Primary, form);
    make->setEnabled(false);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet, form);
    h->addWidget(Web::labelled(tr("Name"), name, form), 0, Qt::AlignBottom);
    h->addWidget(Web::labelled(tr("From"), source, form), 0, Qt::AlignBottom);
    h->addWidget(Web::labelled(tr("To"), target, form), 0, Qt::AlignBottom);
    h->addWidget(make, 0, Qt::AlignBottom);
    h->addWidget(cancel, 0, Qt::AlignBottom);
    h->addStretch();
    v->addLayout(h);
    auto *error = Web::text(QString(), "error", form);
    error->hide();
    v->addWidget(error);
    p->formHolder->addWidget(form);

    connect(name, &QLineEdit::textChanged, make, [make](const QString &t) { make->setEnabled(!t.trimmed().isEmpty()); });
    connect(cancel, &QPushButton::clicked, this, [this, id] {
        if (Part *q = part(id))
            serverForm(q, false);
    });
    auto submit = [this, id, name, source, target, error] {
        Part *q = part(id);
        const QString wanted = name->text().trimmed();
        if (!q || wanted.isEmpty())
            return;
        error->hide();
        QPointer<QLabel> err(error);
        const QString host = q->conn->displayName();
        onResult(q->conn->api()->post(QStringLiteral("/api/memories"),
                                      QJsonObject{{QStringLiteral("name"), wanted},
                                                  {QStringLiteral("sourceLang"), source->language()},
                                                  {QStringLiteral("targetLang"), target->language()}}),
                 this, [this, id, host, err](const ApiResult &r) {
                     if (!r.ok()) {
                         if (err) {
                             err->setText(r.status == 403
                                              ? tr("You may not make memories on the server. Ask an administrator for the manage role on all memories.")
                                              : r.error.message);
                             err->show();
                         }
                         return;
                     }
                     const QString made = r.object().value(QLatin1String("name")).toString();
                     Part *s = part(id);
                     if (s) {
                         serverForm(s, false);
                         // Shown opened once the list has it.
                         s->open = made;
                         clear(s->detail);
                     }
                     notify(tr("Made the memory %1 on %2").arg(made, host));
                     reload();
                 });
    };
    connect(make, &QPushButton::clicked, form, submit);
    connect(name, &QLineEdit::returnPressed, form, submit);
    name->setFocus();
}
