#include "settingspage.h"

#include "apiclient.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "options.h"
#include "secretstore.h"
#include "signindialog.h"
#include "theme.h"
#include "webui.h"
#include "words.h"

#include <QButtonGroup>
#include <QCheckBox>
#include <QDesktopServices>
#include <QDir>
#include <QFileDialog>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonObject>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRadioButton>
#include <QSettings>
#include <QUrl>
#include <QVBoxLayout>

namespace {

QString stateText(const Connection *c)
{
    switch (c->state()) {
    case Connection::State::Connecting:
        return c->isLocal() ? SettingsPage::tr("Starting") : SettingsPage::tr("Connecting");
    case Connection::State::Ready:
        return SettingsPage::tr("Ready");
    case Connection::State::SignedOut:
        return SettingsPage::tr("Signed out");
    case Connection::State::Offline:
        return SettingsPage::tr("Can't be reached");
    case Connection::State::Failed:
        return SettingsPage::tr("Not working");
    }
    return {};
}

void clear(QLayout *l)
{
    while (QLayoutItem *item = l->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->setParent(nullptr);
            w->deleteLater();
        } else if (item->layout()) {
            clear(item->layout());
        }
        delete item;
    }
}

QString plural(qint64 n, const QString &one, const QString &many)
{
    return n == 1 ? one : many.arg(QLocale().toString(n));
}

QVBoxLayout *host(QVBoxLayout *into)
{
    auto *v = new QVBoxLayout;
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(8);
    into->addLayout(v);
    return v;
}

// .settings-list li.row: what it is on the left, what to do with it on the right.
QWidget *listRow(QWidget *what, QWidget *action, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(0, 2, 0, 2);
    h->setSpacing(12);
    h->addWidget(what, 1);
    if (action)
        h->addWidget(action, 0, Qt::AlignRight | Qt::AlignVCenter);
    return w;
}

} // namespace

SettingsPage::SettingsPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
{
    setObjectName(QStringLiteral("settings"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    Web::Page page = Web::page(this, 760);
    page.scroll->setAccessibleName(tr("Settings"));
    QWidget *p = page.scroll->widget();
    page.body->addWidget(Web::header(tr("Settings"), QString(), {}, p));

    // Your name.
    QVBoxLayout *body = nullptr;
    page.body->addWidget(Web::panel(tr("Your name"), p, &body));
    m_name = new QLineEdit(p);
    m_name->setObjectName(QStringLiteral("userName"));
    m_name->setAccessibleName(tr("Your name"));
    m_name->setPlaceholderText(tr("Me"));
    body->addWidget(Web::field(tr("Name"), m_name, p, false,
                               tr("Entries and translations you change on this computer are signed with it, in their history and in exported files. On a team server your account's name is used.")));
    m_nameStatus = Web::text(QString(), "error", p);
    m_nameStatus->setVisible(false);
    body->addWidget(m_nameStatus);
    connect(m_name, &QLineEdit::editingFinished, this, &SettingsPage::saveName);

    // Terminology wording.
    page.body->addWidget(Web::panel(tr("Terminology wording"), p, &body));
    body->addWidget(Web::text(tr("Project termbases work like branches of a master. Choose how the interface names this. The behaviour is the same either way."), "help", p));
    {
        auto *group = new QWidget(p);
        group->setAccessibleName(tr("Wording"));
        auto *gv = new QVBoxLayout(group);
        gv->setContentsMargins(0, 0, 0, 0);
        gv->setSpacing(8);
        auto *plain = new QRadioButton(tr("Plain words"), group);
        auto *git = new QRadioButton(tr("Git terms"), group);
        auto *wording = new QButtonGroup(group);
        wording->addButton(plain);
        wording->addButton(git);
        (Words::git() ? git : plain)->setChecked(true);
        gv->addWidget(Web::choice(plain, tr("Project termbase, review and merge, update from master, history."), group));
        gv->addWidget(Web::choice(git, tr("Branch, diff and merge, pull, log, ours and theirs."), group));
        body->addWidget(group);
        connect(git, &QRadioButton::toggled, this, [](bool on) { Words::setGit(on); });
    }

    // Editor.
    page.body->addWidget(Web::panel(tr("Editor"), p, &body));
    {
        auto *group = new QWidget(p);
        auto *gv = new QVBoxLayout(group);
        gv->setContentsMargins(0, 0, 0, 0);
        gv->setSpacing(8);
        auto *insert = new QCheckBox(tr("Insert the best memory match when a segment opens"), group);
        insert->setChecked(Options::autoInsert());
        gv->addWidget(Web::choice(insert, tr("Only into empty segments. The match percentage stays next to the segment until you confirm."), group));
        auto *fill = new QCheckBox(tr("Fill repetitions when you confirm"), group);
        fill->setChecked(Options::autoPropagate());
        gv->addWidget(Web::choice(fill, tr("Other segments with the same text and tags get the same translation and are confirmed. Segments you already confirmed are left alone."), group));
        body->addWidget(group);
        connect(insert, &QCheckBox::toggled, this, [](bool on) { Options::setAutoInsert(on); });
        connect(fill, &QCheckBox::toggled, this, [](bool on) { Options::setAutoPropagate(on); });

        auto *ctx = new QWidget(p);
        ctx->setAccessibleName(tr("Context matches (CM) in new projects"));
        auto *cv = new QVBoxLayout(ctx);
        cv->setContentsMargins(0, 0, 0, 0);
        cv->setSpacing(8);
        cv->addWidget(Web::text(tr("Context matches (CM) in new projects"), "fieldLabel", ctx));
        cv->addWidget(Web::text(tr("Each project counts context matches its own way; change it in the project's settings."), "help", ctx));
        auto *previous = new QRadioButton(tr("The segment before also matches"), ctx);
        auto *both = new QRadioButton(tr("The segments before and after also match"), ctx);
        auto *rules = new QButtonGroup(ctx);
        rules->addButton(previous);
        rules->addButton(both);
        (Options::contextRule() == QLatin1String("both") ? both : previous)->setChecked(true);
        cv->addWidget(Web::choice(previous, tr("The usual rule."), ctx));
        cv->addWidget(Web::choice(both, tr("Stricter: such a match is often shown as 101%."), ctx));
        body->addWidget(ctx);
        connect(both, &QRadioButton::toggled, this, [](bool on) { Options::setContextRule(on ? QStringLiteral("both") : QStringLiteral("previous")); });
    }

    page.body->addWidget(Web::panel(tr("Project templates"), p, &body));
    m_templates = host(body);

    page.body->addWidget(Web::panel(tr("Spelling dictionaries"), p, &body));
    body->addWidget(Web::text(tr("Hunspell dictionaries, as LibreOffice and Firefox use them, checked by the core. Each project chooses one per target language in its checks."), "help", p));
    m_dictionaries = host(body);

    page.body->addWidget(Web::panel(tr("Team server"), p, &body));
    body->addWidget(Web::text(tr("A team server keeps a team's termbases in one place: everyone translates with the same terms, and new and changed entries reach the masters through review. Projects and memories can be shared there as well, with work given to translators and reviewers."),
                              "help", p));
    m_servers = host(body);

    page.body->addWidget(Web::panel(tr("Saved work"), p, &body));
    m_saved = host(body);

    page.body->addWidget(Web::text(tr("Interlinea %1. Open source: a translation workspace with its editor, terminology, memories and projects.").arg(QStringLiteral(INTERLINEA_VERSION)),
                                   "muted", p));
    page.body->addStretch();
    outer->addWidget(page.scroll);
    m_toast = new Web::Toast(this);

    connect(manager, &ConnectionManager::added, this, &SettingsPage::refresh);
    connect(manager, &ConnectionManager::removed, this, &SettingsPage::refresh);
    connect(manager, &ConnectionManager::changed, this, &SettingsPage::refresh);
    refresh();
}

void SettingsPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

void SettingsPage::showEvent(QShowEvent *e)
{
    QWidget::showEvent(e);
    // Templates saved from a project show when the page is come back to.
    reload();
}

QString SettingsPage::lastNotice() const
{
    return m_toast->last();
}

void SettingsPage::say(const QString &message)
{
    if (!message.isEmpty())
        m_toast->say(message);
}

QList<Connection *> SettingsPage::ready() const
{
    QList<Connection *> out;
    for (Connection *c : m_manager->all()) {
        if (c->isReady())
            out << c;
    }
    return out;
}

void SettingsPage::refresh()
{
    Connection *local = m_manager->local();
    if (!m_name->hasFocus() && local->isReady())
        m_name->setText(local->user().value(QLatin1String("name")).toString());
    m_name->setEnabled(local->isReady());
    fillServers();
    fillSaved();
    // What each place holds is read again when a place comes or goes.
    QString states;
    for (Connection *c : m_manager->all())
        states += c->id() + QLatin1Char(c->isReady() ? '1' : '0');
    if (states != m_states) {
        m_states = states;
        reload();
    } else {
        fillTemplates();
        fillDictionaries();
    }
}

void SettingsPage::reload()
{
    for (const QString &id : m_templateList.keys()) {
        if (!m_manager->find(id) || !m_manager->find(id)->isReady()) {
            m_templateList.remove(id);
            m_dictionaryList.remove(id);
        }
    }
    for (Connection *c : ready()) {
        const QString id = c->id();
        onResult(c->api()->get(QStringLiteral("/api/templates")), this, [this, id](const ApiResult &r) {
            if (r.ok())
                m_templateList.insert(id, r.array());
            fillTemplates();
        });
        onResult(c->api()->get(QStringLiteral("/api/dictionaries")), this, [this, id](const ApiResult &r) {
            if (r.ok())
                m_dictionaryList.insert(id, r.array());
            fillDictionaries();
        });
    }
    fillTemplates();
    fillDictionaries();
}

void SettingsPage::fillTemplates()
{
    clear(m_templates);
    QWidget *p = m_templates->parentWidget();
    const QList<Connection *> places = ready();
    int n = 0;
    for (Connection *c : places) {
        const QJsonArray list = m_templateList.value(c->id());
        if (list.isEmpty())
            continue;
        if (places.size() > 1)
            m_templates->addWidget(Web::text(c->isLocal() ? tr("On this computer") : tr("On %1").arg(c->displayName()), "fieldLabel", p));
        for (const QJsonValue &v : list) {
            const QJsonObject t = v.toObject();
            const QString name = t.value(QLatin1String("name")).toString();
            ++n;
            QStringList targets;
            for (const QJsonValue &l : t.value(QLatin1String("targetLangs")).toArray())
                targets << l.toString();
            auto *what = new QWidget(p);
            auto *h = new QHBoxLayout(what);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(8);
            auto *title = new QLabel(QStringLiteral("<b>%1</b>").arg(name.toHtmlEscaped()), what);
            h->addWidget(title);
            h->addWidget(Web::langPair(t.value(QLatin1String("sourceLang")).toString(), targets, what));
            const QDateTime saved = QDateTime::fromString(t.value(QLatin1String("changedAt")).toString().isEmpty() ? t.value(QLatin1String("createdAt")).toString()
                                                                                                                 : t.value(QLatin1String("changedAt")).toString(),
                                                          Qt::ISODate);
            QStringList meta;
            if (!t.value(QLatin1String("client")).toString().isEmpty())
                meta << t.value(QLatin1String("client")).toString();
            if (saved.isValid())
                meta << tr("saved %1").arg(QLocale().toString(saved.toLocalTime().date(), QLocale::ShortFormat));
            h->addWidget(Web::text(meta.join(QStringLiteral(", ")), "muted", what), 1);
            auto *del = new Web::ConfirmButton(tr("Delete"), tr("Delete the template “%1”? Projects made from it stay as they are.").arg(name), tr("Delete"), p);
            del->button()->setAccessibleName(tr("Delete the template %1").arg(name));
            const QString id = c->id();
            connect(del, &Web::ConfirmButton::confirmed, this, [this, id, name] {
                Connection *conn = m_manager->find(id);
                if (!conn)
                    return;
                onResult(conn->api()->del(QStringLiteral("/api/templates"), QUrlQuery{{QStringLiteral("name"), name}}), this, [this, name](const ApiResult &r) {
                    say(r.ok() ? tr("Deleted the template “%1”").arg(name) : r.error.message);
                    reload();
                });
            });
            m_templates->addWidget(listRow(what, del, p));
        }
    }
    if (n == 0)
        m_templates->addWidget(Web::text(tr("No template yet. Save a project's settings as a template at the end of New project, or from a project's settings."), "help", p));
}

void SettingsPage::fillDictionaries()
{
    clear(m_dictionaries);
    QWidget *p = m_dictionaries->parentWidget();
    const QList<Connection *> places = ready();
    for (Connection *c : places) {
        const QString id = c->id();
        if (places.size() > 1)
            m_dictionaries->addWidget(Web::text(c->isLocal() ? tr("On this computer") : tr("On %1").arg(c->displayName()), "fieldLabel", p));
        const QJsonArray list = m_dictionaryList.value(id);
        if (list.isEmpty())
            m_dictionaries->addWidget(Web::text(tr("No dictionary yet."), "muted", p));
        for (const QJsonValue &v : list) {
            const QJsonObject d = v.toObject();
            const QString name = d.value(QLatin1String("name")).toString();
            auto *what = new QWidget(p);
            auto *h = new QHBoxLayout(what);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(8);
            h->addWidget(new QLabel(QStringLiteral("<b>%1</b>").arg(name.toHtmlEscaped()), what));
            h->addWidget(Web::langChip(d.value(QLatin1String("lang")).toString(), what));
            h->addWidget(Web::text(tr("%1 words").arg(QLocale().toString(d.value(QLatin1String("words")).toInt())), "muted", what), 1);
            auto *del = new Web::ConfirmButton(tr("Delete"), tr("Delete the dictionary %1?").arg(name), tr("Delete"), p);
            del->button()->setAccessibleName(tr("Delete the dictionary %1").arg(name));
            connect(del, &Web::ConfirmButton::confirmed, this, [this, id, name] {
                Connection *conn = m_manager->find(id);
                if (!conn)
                    return;
                onResult(conn->api()->del(QStringLiteral("/api/dictionaries/%1").arg(ApiClient::seg(name))), this, [this, name](const ApiResult &r) {
                    // One a project or template spells with stays; the answer names them.
                    say(r.ok() ? tr("Deleted the dictionary %1").arg(name) : r.error.message);
                    reload();
                });
            });
            m_dictionaries->addWidget(listRow(what, del, p));
        }
        // Dictionaries chosen as loose files whose language is still to say.
        bool asking = false;
        for (int i = 0; i < m_loose.size(); ++i) {
            if (m_loose[i].connectionId != id)
                continue;
            if (!asking) {
                m_dictionaries->addWidget(Web::text(tr("Found in the files"), "fieldLabel", p));
                asking = true;
            }
            auto *lang = new LanguageCombo(p);
            lang->setAccessibleName(tr("Language of %1").arg(m_loose[i].name));
            lang->lineEdit()->setPlaceholderText(tr("Choose the language"));
            lang->setLanguage(m_loose[i].lang);
            lang->setMinimumWidth(240);
            auto *add = Web::button(tr("Add dictionary"), Web::Primary | Web::Small, p);
            add->setAccessibleName(tr("Add the dictionary %1").arg(m_loose[i].name));
            add->setEnabled(!m_loose[i].lang.isEmpty());
            auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, p);
            cancel->setAccessibleName(tr("Do not add %1").arg(m_loose[i].name));
            const QString name = m_loose[i].name;
            const auto at = [this, id, name] {
                for (int j = 0; j < m_loose.size(); ++j) {
                    if (m_loose[j].connectionId == id && m_loose[j].name == name)
                        return j;
                }
                return -1;
            };
            connect(lang, &QComboBox::currentTextChanged, this, [this, lang, add, at] {
                const int j = at();
                if (j < 0)
                    return;
                m_loose[j].lang = lang->language();
                add->setEnabled(!m_loose[j].lang.isEmpty());
            });
            connect(add, &QPushButton::clicked, this, [this, at] {
                if (at() >= 0)
                    sendLoose(at());
            });
            connect(cancel, &QPushButton::clicked, this, [this, at] {
                if (at() >= 0)
                    m_loose.removeAt(at());
                fillDictionaries();
            });
            m_dictionaries->addWidget(listRow(new QLabel(QStringLiteral("<b>%1</b>").arg(name.toHtmlEscaped()), p), Web::row({lang, add, cancel}, p), p));
        }
        auto *add = Web::button(tr("Add dictionaries"), Web::Small, p, QStringLiteral("upload"));
        add->setAccessibleName(places.size() > 1 ? tr("Add dictionaries on %1").arg(c->displayName()) : tr("Add dictionaries"));
        m_dictionaries->addWidget(add, 0, Qt::AlignLeft);
        connect(add, &QPushButton::clicked, this, [this, id] {
            if (Connection *conn = m_manager->find(id))
                addDictionaries(conn);
        });
    }
    if (places.isEmpty())
        m_dictionaries->addWidget(Web::text(tr("No dictionary yet."), "muted", p));
    for (const QString &problem : std::as_const(m_dictionaryProblems))
        m_dictionaries->addWidget(Web::text(problem, "error", p));
}

// LibreOffice extensions and ZIP files hold their dictionaries; a loose
// dictionary is its .aff and its .dic file, chosen together.
void SettingsPage::addDictionaries(Connection *c)
{
    const QStringList paths = QFileDialog::getOpenFileNames(this, tr("Spelling dictionaries"), QString(),
                                                            tr("Dictionaries (*.oxt *.zip *.aff *.dic);;All files (*)"));
    if (paths.isEmpty())
        return;
    m_dictionaryProblems.clear();
    const QString id = c->id();
    QHash<QString, Loose> pairs;
    for (const QString &path : paths) {
        const QFileInfo info(path);
        const QString ext = info.suffix().toLower();
        QFile f(path);
        if (!f.open(QIODevice::ReadOnly)) {
            m_dictionaryProblems << tr("%1 can't be read: %2").arg(info.fileName(), f.errorString());
            continue;
        }
        const QByteArray data = f.readAll();
        if (ext == QLatin1String("aff") || ext == QLatin1String("dic")) {
            Loose &l = pairs[info.completeBaseName()];
            l.connectionId = id;
            l.name = info.completeBaseName();
            (ext == QLatin1String("aff") ? l.aff : l.dic) = data;
            continue;
        }
        const QString file = info.fileName();
        onResult(c->api()->upload("POST", QStringLiteral("/api/dictionaries"), data), this, [this, file](const ApiResult &r) {
            if (r.ok()) {
                QStringList names;
                for (const QJsonValue &v : r.array())
                    names << tr("%1 (%2)").arg(v.toObject().value(QLatin1String("name")).toString(),
                                               plural(v.toObject().value(QLatin1String("words")).toInt(), tr("1 word"), tr("%1 words")));
                say(tr("Added %1").arg(names.join(QStringLiteral(", "))));
            } else {
                m_dictionaryProblems << tr("%1: %2").arg(file, r.error.message);
            }
            reload();
        });
    }
    for (const Loose &l : std::as_const(pairs)) {
        if (l.aff.isEmpty() || l.dic.isEmpty()) {
            m_dictionaryProblems << tr("%1: a dictionary is two files, its .aff and its .dic. Choose both.").arg(l.name);
            continue;
        }
        m_loose.append(l);
        sendLoose(int(m_loose.size()) - 1);
    }
    fillDictionaries();
}

void SettingsPage::sendLoose(int index)
{
    if (index < 0 || index >= m_loose.size())
        return;
    const Loose l = m_loose.at(index);
    Connection *c = m_manager->find(l.connectionId);
    if (!c)
        return;
    QJsonObject body{{QStringLiteral("name"), l.name}, {QStringLiteral("aff"), QString::fromLatin1(l.aff.toBase64())}, {QStringLiteral("dic"), QString::fromLatin1(l.dic.toBase64())}};
    if (!l.lang.isEmpty())
        body.insert(QStringLiteral("lang"), l.lang);
    onResult(c->api()->post(QStringLiteral("/api/dictionaries"), body), this, [this, l](const ApiResult &r) {
        const auto drop = [this, l] {
            for (int j = 0; j < m_loose.size(); ++j) {
                if (m_loose[j].connectionId == l.connectionId && m_loose[j].name == l.name) {
                    m_loose.removeAt(j);
                    return;
                }
            }
        };
        if (r.ok()) {
            drop();
            const QJsonObject d = r.array().at(0).toObject();
            say(tr("Added %1 (%2)").arg(d.value(QLatin1String("name")).toString(), plural(d.value(QLatin1String("words")).toInt(), tr("1 word"), tr("%1 words"))));
        } else if (r.error.problemWith(QStringLiteral("lang")).isEmpty() || !l.lang.isEmpty()) {
            drop();
            m_dictionaryProblems << tr("%1: %2").arg(l.name, r.error.message);
        }
        // Otherwise the files do not say the language: it is asked for.
        reload();
    });
}

void SettingsPage::fillServers()
{
    clear(m_servers);
    QWidget *p = m_servers->parentWidget();
    for (Connection *c : m_manager->teams()) {
        auto *box = new QFrame(p);
        box->setObjectName(QStringLiteral("choice"));
        box->setAccessibleName(c->displayName());
        auto *v = new QVBoxLayout(box);
        v->setContentsMargins(12, 12, 12, 12);
        v->setSpacing(6);
        auto *top = new QHBoxLayout;
        top->setSpacing(8);
        auto *name = new QLabel(QStringLiteral("<b>%1</b>").arg(c->displayName().toHtmlEscaped()), box);
        name->setObjectName(QStringLiteral("serverName"));
        top->addWidget(name);
        auto *state = Web::chip(stateText(c), c->isReady() ? QStringLiteral("approved") : c->state() == Connection::State::Connecting ? QString() : QStringLiteral("changed"), box);
        state->setObjectName(QStringLiteral("serverState"));
        top->addWidget(state);
        top->addStretch();
        v->addLayout(top);
        QStringList lines{c->url().toString()};
        if (c->isReady())
            lines << (c->isAdmin() ? tr("Signed in as %1 (%2), an administrator there.").arg(c->userName(), c->login()) : tr("Signed in as %1 (%2).").arg(c->userName(), c->login()));
        if (!c->serverVersion().isEmpty())
            lines << tr("Interlinea %1 on the server.").arg(c->serverVersion());
        if (!c->message().isEmpty())
            lines << c->message();
        auto *details = Web::text(lines.join(QLatin1Char('\n')), "secondary", box);
        details->setTextInteractionFlags(Qt::TextSelectableByMouse | Qt::TextSelectableByKeyboard);
        details->setAccessibleName(details->text());
        v->addWidget(details);
        QList<QWidget *> acts;
        const QString id = c->id();
        if (c->state() != Connection::State::Ready) {
            auto *in = Web::button(tr("Sign in"), Web::Small, box);
            in->setAccessibleName(tr("Sign in to %1").arg(c->displayName()));
            connect(in, &QPushButton::clicked, this, [this, id] {
                if (Connection *conn = m_manager->find(id)) {
                    SignInDialog dlg(m_manager, conn->url(), this);
                    dlg.exec();
                }
            });
            acts << in;
        }
        if (c->state() != Connection::State::SignedOut) {
            auto *out = Web::button(tr("Sign out"), Web::Small, box);
            out->setAccessibleName(tr("Sign out of %1").arg(c->displayName()));
            connect(out, &QPushButton::clicked, this, [this, id] {
                if (Connection *conn = m_manager->find(id))
                    m_manager->signOut(conn);
            });
            acts << out;
        }
        auto *forget = new Web::ConfirmButton(tr("Remove"), tr("Sign out of %1 and forget it? Your work on the server stays there.").arg(c->displayName()), tr("Remove"), box);
        forget->button()->setAccessibleName(tr("Remove %1").arg(c->displayName()));
        connect(forget, &Web::ConfirmButton::confirmed, this, [this, id] {
            if (Connection *conn = m_manager->find(id))
                m_manager->remove(conn);
        });
        acts << forget;
        v->addWidget(Web::row(acts, box));
        m_servers->addWidget(box);
    }
    auto *address = new QLineEdit(p);
    address->setObjectName(QStringLiteral("serverAddress"));
    address->setAccessibleName(tr("Server address"));
    address->setPlaceholderText(QStringLiteral("https://interlinea.example.com"));
    auto *connectButton = Web::button(tr("Connect"), Web::Normal, p);
    auto *row = new QWidget(p);
    auto *h = new QHBoxLayout(row);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(8);
    h->addWidget(address, 1);
    h->addWidget(connectButton);
    auto *field = Web::field(m_manager->teams().isEmpty() ? tr("Server address") : tr("Another server's address"), row, p);
    if (auto *l = field->findChild<QLabel *>())
        l->setBuddy(address);
    m_servers->addWidget(field);
    const auto go = [this, address] {
        const QString typed = address->text().trimmed();
        SignInDialog dlg(m_manager, typed.isEmpty() ? QUrl() : ConnectionManager::serverUrl(typed), this);
        dlg.exec();
    };
    connect(connectButton, &QPushButton::clicked, this, go);
    connect(address, &QLineEdit::returnPressed, this, go);
    if (!m_manager->teams().isEmpty())
        m_servers->addWidget(Web::text(SecretStore::secure() ? tr("Sign-ins to team servers are kept in %1.").arg(SecretStore::where())
                                                             : tr("Sign-ins to team servers are kept in %1, because the keychain can't be used: %2")
                                                                   .arg(SecretStore::where(), SecretStore::fallbackReason()),
                                       "help", p));
}

void SettingsPage::fillSaved()
{
    clear(m_saved);
    QWidget *p = m_saved->parentWidget();
    Connection *local = m_manager->local();
    if (local->state() == Connection::State::Failed) {
        auto *note = Web::note(local->message().isEmpty() ? tr("Work on this computer can't be opened.") : local->message(), QStringLiteral("warn"), p);
        note->setObjectName(QStringLiteral("coreFailed"));
        m_saved->addWidget(note);
    } else if (local->state() == Connection::State::Connecting) {
        m_saved->addWidget(Web::text(tr("Starting the work on this computer…"), "muted", p));
    }
    QString where = tr("Projects, translations, termbases, memories, templates and dictionaries are saved on this computer as you work, in %1. They stay on this computer.")
                        .arg(QDir::toNativeSeparators(m_manager->core()->dataDir()));
    if (!local->serverVersion().isEmpty())
        where += QLatin1Char(' ') + tr("Core %1.").arg(local->serverVersion());
    auto *help = Web::text(where, "help", p);
    help->setTextInteractionFlags(Qt::TextSelectableByMouse | Qt::TextSelectableByKeyboard);
    help->setAccessibleName(where);
    m_saved->addWidget(help);
    QList<QWidget *> acts;
    auto *open = Web::button(tr("Open the folder"), Web::Small, p, QStringLiteral("folder-open"));
    connect(open, &QPushButton::clicked, this, [this] { QDesktopServices::openUrl(QUrl::fromLocalFile(m_manager->core()->dataDir())); });
    acts << open;
    if (local->state() == Connection::State::Failed) {
        auto *again = Web::button(tr("Start the core again"), Web::Small, p, QStringLiteral("refresh"));
        connect(again, &QPushButton::clicked, this, [this] { m_manager->core()->start(); });
        acts << again;
    }
    m_saved->addWidget(Web::row(acts, p));
}

void SettingsPage::saveName()
{
    Connection *local = m_manager->local();
    const QString name = m_name->text().trimmed();
    const QJsonObject user = local->user();
    if (!local->isReady() || name.isEmpty() || name == user.value(QLatin1String("name")).toString())
        return;
    const qint64 id = user.value(QLatin1String("id")).toInteger();
    onResult(local->api()->patch(QStringLiteral("/api/users/%1").arg(id), QJsonObject{{QStringLiteral("name"), name}}, user.value(QLatin1String("version")).toInt()), this,
             [this, local](const ApiResult &r) {
                 m_nameStatus->setText(r.ok() ? QString() : r.error.message);
                 m_nameStatus->setVisible(!r.ok());
                 if (!r.ok())
                     return;
                 local->setUser(r.object());
                 // The core is started with it next time.
                 QSettings().setValue(QStringLiteral("you/name"), r.object().value(QLatin1String("name")).toString());
                 say(tr("Your name is %1").arg(r.object().value(QLatin1String("name")).toString()));
             });
}
