#include "accountspage.h"

#include "accesspanel.h"
#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "icons.h"
#include "theme.h"

#include <QCheckBox>
#include <QComboBox>
#include <QDateTime>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRegularExpression>
#include <QResizeEvent>
#include <QUrl>
#include <QUrlQuery>
#include <QVBoxLayout>

namespace {

constexpr int S1 = 4, S2 = 8, S3 = 12, S4 = 16;

QString str(const QJsonObject &o, const char *key)
{
    return o.value(QLatin1String(key)).toString();
}

// "1 Dec 2026", or "never" for the zero time the server sends for no date.
QString day(const QString &iso)
{
    QString text = iso;
    text.replace(QRegularExpression(QStringLiteral("(\\.\\d{3})\\d+")), QStringLiteral("\\1"));
    QDateTime t = QDateTime::fromString(text, Qt::ISODateWithMs);
    if (!t.isValid())
        t = QDateTime::fromString(text, Qt::ISODate);
    return t.isValid() && t.date().year() > 1 ? QLocale().toString(t.toLocalTime().date(), QStringLiteral("d MMM yyyy")) : AccountsPage::tr("never");
}

void clear(QLayout *layout)
{
    while (QLayoutItem *item = layout->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        } else if (QLayout *l = item->layout()) {
            clear(l);
        }
        delete item;
    }
}

QFrame *line(QWidget *parent)
{
    auto *f = new QFrame(parent);
    Theme::role(f, "line");
    f->setFixedHeight(1);
    return f;
}

QString kindText(const QString &kind)
{
    if (kind == QLatin1String("project"))
        return AccountsPage::tr("Project");
    if (kind == QLatin1String("memory"))
        return AccountsPage::tr("Memory");
    if (kind == QLatin1String("termbase"))
        return AccountsPage::tr("Termbase");
    return AccountsPage::tr("Everything");
}

// A field under its small grey label, with a line for what is wrong.
QWidget *formField(const QString &label, QLineEdit *edit, QWidget *parent, const QString &help = QString(), const QString &problem = QString())
{
    auto *w = new QWidget(parent);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S1);
    auto *l = Web::text(label, "fieldLabel", w);
    l->setBuddy(edit);
    edit->setParent(w);
    edit->setAccessibleName(label);
    edit->setProperty("invalid", !problem.isEmpty());
    v->addWidget(l);
    v->addWidget(edit);
    if (!help.isEmpty()) {
        auto *h = Web::text(help, "cellSub", w);
        h->setWordWrap(true);
        v->addWidget(h);
    }
    if (!problem.isEmpty()) {
        auto *p = Web::text(problem, "error", w);
        p->setWordWrap(true);
        v->addWidget(p);
    }
    return w;
}

} // namespace

AccountsPage::AccountsPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
{
    setObjectName(QStringLiteral("accounts"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    Web::Page page = Web::page(this);
    m_body = new QVBoxLayout;
    m_body->setSpacing(S4);
    page.body->addLayout(m_body);
    page.body->addStretch();
    outer->addWidget(page.scroll);
    m_toast = new Web::Toast(this);
    m_reload.setSingleShot(true);
    m_reload.setInterval(200);
    connect(&m_reload, &QTimer::timeout, this, &AccountsPage::reload);
    connect(manager, &ConnectionManager::added, this, [this] { m_reload.start(); });
    connect(manager, &ConnectionManager::changed, this, [this] { m_reload.start(); });
    connect(manager, &ConnectionManager::removed, this, [this](const QString &id) {
        m_listening.remove(id);
        if (m_conn == id)
            m_conn.clear();
        m_reload.start();
    });
    render();
    reload();
}

void AccountsPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

void AccountsPage::notify(const QString &text)
{
    m_toast->say(text);
}

QString AccountsPage::message() const
{
    return m_toast->last();
}

Connection *AccountsPage::conn() const
{
    Connection *c = m_manager->find(m_conn);
    return c && !c->isLocal() && c->isReady() ? c : nullptr;
}

QJsonObject AccountsPage::user(qint64 id) const
{
    for (const QJsonValue &v : m_users) {
        if (v.toObject().value(QLatin1String("id")).toInteger() == id)
            return v.toObject();
    }
    return {};
}

void AccountsPage::listen()
{
    for (Connection *c : m_manager->teams()) {
        if (m_listening.contains(c->id()))
            continue;
        m_listening << c->id();
        const QString id = c->id();
        connect(c->feed(), &EventFeed::event, this, [this, id](const QString &type) {
            if (id == m_conn && (type == QLatin1String("account") || type == QLatin1String("project") || type == QLatin1String("memory") || type == QLatin1String("termbase")))
                m_reload.start();
        });
        connect(c->feed(), &EventFeed::reset, this, [this] { m_reload.start(); });
    }
}

void AccountsPage::setConnection(const QString &id)
{
    if (m_conn == id)
        return;
    m_conn = id;
    m_selected = 0;
    m_users = {};
    m_read = false;
    m_newAccount = {};
    m_newProblems = {};
    render();
    reload();
}

void AccountsPage::request(ApiReply *reply, std::function<void(const ApiResult &)> then)
{
    m_pending++;
    onResult(reply, this, [this, then](const ApiResult &r) {
        m_pending--;
        then(r);
        if (m_pending == 0)
            emit loaded();
    });
}

void AccountsPage::reload()
{
    listen();
    // The server shown is one signed in to; the first, unless one is chosen.
    if (!conn()) {
        m_conn.clear();
        for (Connection *c : m_manager->teams()) {
            if (c->isReady()) {
                m_conn = c->id();
                break;
            }
        }
    }
    Connection *c = conn();
    if (!c || !c->isAdmin()) {
        m_users = {};
        m_read = false;
        m_selected = 0;
        render();
        emit loaded();
        return;
    }
    const int reading = ++m_reading;
    request(c->api()->get(QStringLiteral("/api/users")), [this, reading](const ApiResult &r) {
        if (reading != m_reading)
            return;
        m_listProblem = r.ok() ? QString() : r.error.message;
        if (r.ok()) {
            m_users = r.array();
            m_read = true;
        }
        if (user(m_selected).isEmpty())
            m_selected = 0;
        render();
    });
    request(c->api()->get(QStringLiteral("/api/catalog")), [this, reading](const ApiResult &r) {
        if (reading == m_reading && r.ok())
            m_catalog = r.object();
    });
    request(c->api()->get(QStringLiteral("/api/projects")), [this, reading](const ApiResult &r) {
        if (reading == m_reading && r.ok())
            m_projects = r.array();
    });
    if (m_selected)
        loadDetail();
}

void AccountsPage::select(qint64 userId)
{
    if (user(userId).isEmpty())
        return;
    m_selected = userId;
    m_grants = {};
    m_signIns = {};
    m_identities = {};
    m_problem.clear();
    render();
    loadDetail();
}

void AccountsPage::loadDetail()
{
    Connection *c = conn();
    if (!c || !m_selected)
        return;
    const qint64 id = m_selected;
    const QString base = QStringLiteral("/api/users/%1").arg(id);
    // Answers of an earlier reading are passed over: they may come late.
    const int reading = ++m_detailReading;
    const auto part = [this, id, reading](QJsonArray AccountsPage::*into) {
        return [this, id, into, reading](const ApiResult &r) {
            if (id != m_selected || reading != m_detailReading)
                return;
            if (r.ok())
                this->*into = r.array();
            else
                m_problem = r.error.message;
            if (m_pending == 0)
                render();
        };
    };
    request(c->api()->get(base + QStringLiteral("/tokens")), part(&AccountsPage::m_signIns));
    request(c->api()->get(base + QStringLiteral("/identities")), part(&AccountsPage::m_identities));
    request(c->api()->get(base + QStringLiteral("/grants")), part(&AccountsPage::m_grants));
}

void AccountsPage::attempt(ApiReply *reply, const QString &done)
{
    m_problem.clear();
    request(reply, [this, done](const ApiResult &r) {
        if (!r.ok()) {
            if (r.error.code == QLatin1String("lastAdmin")) {
                m_problem = tr("The server needs at least one active administrator: make someone else one first.");
            } else if (!r.error.problems.isEmpty()) {
                QStringList all;
                for (const Problem &p : r.error.problems)
                    all << p.message;
                m_problem = all.join(QLatin1Char(' '));
            } else {
                m_problem = r.error.message;
            }
            render();
            return;
        }
        notify(done);
        reload();
    });
}

QString AccountsPage::resourceName(const QJsonObject &g) const
{
    const QString kind = str(g, "kind"), resource = str(g, "resource");
    if (kind == QLatin1String("all"))
        return tr("Everything");
    if (resource == QLatin1String("*"))
        return kind == QLatin1String("memory") ? tr("All memories") : kind == QLatin1String("project") ? tr("All projects") : tr("All termbases");
    if (kind == QLatin1String("project")) {
        for (const QJsonValue &p : m_projects) {
            if (QString::number(p.toObject().value(QLatin1String("id")).toInteger()) == resource)
                return str(p.toObject().value(QLatin1String("settings")).toObject(), "name");
        }
        return tr("Project %1").arg(resource);
    }
    return resource;
}

void AccountsPage::render()
{
    clear(m_body);
    QWidget *host = m_body->parentWidget();
    m_body->addWidget(Web::header(tr("Accounts"), QString(), {}, host));
    Connection *c = conn();
    if (!c) {
        // .page-narrow
        auto *p = Web::text(tr("Accounts are kept on a team server. Sign in to one in Settings."), nullptr, host);
        p->setObjectName(QStringLiteral("state"));
        p->setWordWrap(true);
        p->setMaximumWidth(760);
        m_body->addWidget(p);
        auto *go = Web::button(tr("Open Settings"), Web::Small, host);
        go->setObjectName(QStringLiteral("openSettings"));
        connect(go, &QPushButton::clicked, this, &AccountsPage::openSettings);
        m_body->addWidget(go, 0, Qt::AlignLeft);
        return;
    }
    // With several servers, the one shown is chosen first.
    QList<Connection *> servers;
    for (Connection *t : m_manager->teams()) {
        if (t->isReady())
            servers << t;
    }
    if (servers.size() > 1) {
        QStringList names;
        int at = 0;
        for (int i = 0; i < servers.size(); i++) {
            names << servers[i]->displayName();
            if (servers[i]->id() == m_conn)
                at = i;
        }
        auto *which = new Web::Segmented(tr("Team server"), names, host);
        which->setObjectName(QStringLiteral("server"));
        which->setCurrent(at);
        connect(which, &Web::Segmented::changed, this, [this, servers](int i) {
            if (Connection *t = servers.value(i))
                setConnection(t->id());
        });
        m_body->addWidget(which, 0, Qt::AlignLeft);
    }
    if (!c->isAdmin()) {
        auto *p = Web::text(tr("Only the server's administrators manage accounts. Managers decide who may use their projects on each project's page."), nullptr, host);
        p->setObjectName(QStringLiteral("state"));
        p->setWordWrap(true);
        p->setMaximumWidth(760);
        m_body->addWidget(p);
        return;
    }
    auto *help = Web::text(tr("The people on %1. Roles go up from read to manage; each includes the ones before it.").arg(c->displayName()), "help", host);
    help->setWordWrap(true);
    m_body->addWidget(help);
    if (!m_listProblem.isEmpty()) {
        auto *e = Web::text(m_listProblem, "error", host);
        e->setWordWrap(true);
        m_body->addWidget(e);
    }

    // .accounts: the list in 16 to 22 rem, the account beside it.
    auto *columns = new QHBoxLayout;
    columns->setSpacing(S4);
    QVBoxLayout *lv = nullptr;
    QFrame *listPanel = Web::panel(m_read ? (m_users.size() == 1 ? tr("1 account") : tr("%1 accounts").arg(m_users.size())) : tr("Accounts"), host, &lv);
    listPanel->setMinimumWidth(256);
    listPanel->setMaximumWidth(352);
    lv->setSpacing(S3);
    showList(lv, listPanel);
    showNewAccount(lv, listPanel);
    columns->addWidget(listPanel, 0, Qt::AlignTop);
    if (!user(m_selected).isEmpty()) {
        QVBoxLayout *dv = nullptr;
        const QJsonObject u = user(m_selected);
        QFrame *detail = Web::panel(str(u, "name").isEmpty() ? str(u, "username") : str(u, "name"), host, &dv);
        detail->setProperty("part", QStringLiteral("detail"));
        dv->setSpacing(S3);
        showDetail(dv, detail);
        columns->addWidget(detail, 1, Qt::AlignTop);
    } else {
        QVBoxLayout *ev = nullptr;
        QFrame *empty = Web::panel(QString(), host, &ev);
        auto *t = Web::text(tr("Choose an account to see and change it."), "muted", empty);
        t->setObjectName(QStringLiteral("choose"));
        ev->addWidget(t);
        columns->addWidget(empty, 1, Qt::AlignTop);
    }
    m_body->addLayout(columns);
}

void AccountsPage::showList(QVBoxLayout *into, QWidget *host)
{
    if (!m_read) {
        into->addWidget(Web::text(tr("Loading…"), "muted", host));
        return;
    }
    // .account-list
    auto *list = new QVBoxLayout;
    list->setSpacing(0);
    for (const QJsonValue &v : m_users) {
        const QJsonObject u = v.toObject();
        const qint64 id = u.value(QLatin1String("id")).toInteger();
        auto *row = new Web::Clickable("tbLink", host);
        row->setObjectName(QStringLiteral("account:%1").arg(str(u, "username")));
        row->setCurrent(id == m_selected);
        auto *rv = new QVBoxLayout(row);
        rv->setContentsMargins(S3, S2, S3, S2);
        rv->setSpacing(2);
        const QString title = str(u, "name").isEmpty() ? str(u, "username") : str(u, "name");
        rv->addWidget(new QLabel(QStringLiteral("<b>%1</b>").arg(title.toHtmlEscaped()), row));
        QString sub = str(u, "username");
        if (u.value(QLatin1String("admin")).toBool())
            sub += tr(", administrator");
        if (u.value(QLatin1String("disabled")).toBool())
            sub += tr(", disabled");
        if (!u.value(QLatin1String("hasPassword")).toBool())
            sub += tr(", provider sign-in only");
        auto *s = Web::text(sub, "cellSub", row);
        s->setWordWrap(true);
        rv->addWidget(s);
        row->setAccessibleName(QStringLiteral("%1, %2").arg(title, sub));
        connect(row, &Web::Clickable::clicked, this, [this, id] { select(id); });
        list->addWidget(row);
    }
    into->addLayout(list);
}

void AccountsPage::showNewAccount(QVBoxLayout *into, QWidget *host)
{
    // .account-new
    into->addWidget(line(host));
    auto *form = new QWidget(host);
    form->setObjectName(QStringLiteral("newAccount"));
    form->setAccessibleName(tr("New account"));
    auto *v = new QVBoxLayout(form);
    v->setContentsMargins(0, S1, 0, 0);
    v->setSpacing(S3);
    v->addWidget(Web::text(tr("New account"), "fieldLabel", form));
    auto *username = new QLineEdit(str(m_newAccount, "username"), form);
    username->setObjectName(QStringLiteral("username"));
    auto *name = new QLineEdit(str(m_newAccount, "name"), form);
    name->setObjectName(QStringLiteral("name"));
    auto *email = new QLineEdit(str(m_newAccount, "email"), form);
    email->setObjectName(QStringLiteral("email"));
    auto *password = new QLineEdit(str(m_newAccount, "password"), form);
    password->setObjectName(QStringLiteral("password"));
    password->setEchoMode(QLineEdit::Password);
    v->addWidget(formField(tr("Username"), username, form, QString(), str(m_newProblems, "username")));
    v->addWidget(formField(tr("Name"), name, form));
    v->addWidget(formField(tr("Email"), email, form, QString(), str(m_newProblems, "email")));
    v->addWidget(formField(tr("Password"), password, form, tr("12 characters or more. Leave it empty for someone who signs in only through the identity provider."),
                           str(m_newProblems, "password")));
    auto *admin = new QCheckBox(tr("Administrator"), form);
    admin->setObjectName(QStringLiteral("admin"));
    admin->setChecked(m_newAccount.value(QLatin1String("admin")).toBool());
    v->addWidget(admin);
    if (!str(m_newProblems, "form").isEmpty()) {
        auto *e = Web::text(str(m_newProblems, "form"), "error", form);
        e->setWordWrap(true);
        v->addWidget(e);
    }
    auto *make = Web::button(tr("Make the account"), Web::Primary, form, QStringLiteral("user-plus"));
    make->setObjectName(QStringLiteral("make"));
    make->setEnabled(!username->text().trimmed().isEmpty());
    v->addWidget(make, 0, Qt::AlignLeft);
    // What is typed is kept when the page is drawn again.
    const auto keep = [this, username, name, email, password, admin, make] {
        m_newAccount = QJsonObject{{QLatin1String("username"), username->text()}, {QLatin1String("name"), name->text()}, {QLatin1String("email"), email->text()},
                                   {QLatin1String("password"), password->text()}, {QLatin1String("admin"), admin->isChecked()}};
        make->setEnabled(!username->text().trimmed().isEmpty());
    };
    for (QLineEdit *e : {username, name, email, password})
        connect(e, &QLineEdit::textChanged, this, keep);
    connect(admin, &QCheckBox::toggled, this, keep);
    const auto submit = [this, keep] {
        Connection *c = conn();
        keep();
        if (!c || str(m_newAccount, "username").trimmed().isEmpty())
            return;
        m_newProblems = {};
        request(c->api()->post(QStringLiteral("/api/users"), m_newAccount), [this](const ApiResult &r) {
            if (!r.ok()) {
                if (!r.error.problems.isEmpty()) {
                    for (const Problem &p : r.error.problems)
                        m_newProblems.insert(p.field, p.message);
                } else if (r.error.code == QLatin1String("exists")) {
                    m_newProblems.insert(QLatin1String("username"), r.error.message);
                } else {
                    m_newProblems.insert(QLatin1String("form"), r.error.message);
                }
                render();
                return;
            }
            notify(tr("Made the account %1").arg(str(r.object(), "username")));
            m_newAccount = {};
            m_selected = r.object().value(QLatin1String("id")).toInteger();
            m_grants = {};
            m_signIns = {};
            m_identities = {};
            m_problem.clear();
            reload();
        });
    };
    connect(make, &QPushButton::clicked, this, submit);
    for (QLineEdit *e : {username, name, email, password})
        connect(e, &QLineEdit::returnPressed, this, submit);
    into->addWidget(form);
}

void AccountsPage::showDetail(QVBoxLayout *into, QWidget *host)
{
    Connection *c = conn();
    const QJsonObject u = user(m_selected);
    if (!c || u.isEmpty())
        return;
    const qint64 id = m_selected;
    const QString username = str(u, "username");
    const QString base = QStringLiteral("/api/users/%1").arg(id);
    const int version = u.value(QLatin1String("version")).toInt();
    const bool me = c->user().value(QLatin1String("id")).toInteger() == id;
    ApiClient *api = c->api();

    auto *sub = Web::text(me ? tr("%1, made on %2. This is you.").arg(username, day(str(u, "createdAt"))) : tr("%1, made on %2.").arg(username, day(str(u, "createdAt"))), "cellSub", host);
    sub->setObjectName(QStringLiteral("about"));
    into->addWidget(sub);
    if (!m_problem.isEmpty()) {
        auto *e = Web::text(m_problem, "error", host);
        e->setObjectName(QStringLiteral("problem"));
        e->setWordWrap(true);
        into->addWidget(e);
    }

    // Details (.server-form, at most 28 rem wide)
    auto *details = new QWidget(host);
    details->setAccessibleName(tr("Details"));
    details->setMaximumWidth(448);
    auto *dv = new QVBoxLayout(details);
    dv->setContentsMargins(0, 0, 0, S1);
    dv->setSpacing(S3);
    auto *name = new QLineEdit(str(u, "name"), details);
    name->setObjectName(QStringLiteral("detailName"));
    auto *email = new QLineEdit(str(u, "email"), details);
    email->setObjectName(QStringLiteral("detailEmail"));
    dv->addWidget(formField(tr("Name"), name, details));
    dv->addWidget(formField(tr("Email"), email, details));
    auto *save = Web::button(tr("Save details"), Web::Normal, details);
    save->setObjectName(QStringLiteral("saveDetails"));
    save->setEnabled(false);
    dv->addWidget(save, 0, Qt::AlignLeft);
    const auto changed = [name, email, save, u] { save->setEnabled(name->text() != str(u, "name") || email->text() != str(u, "email")); };
    connect(name, &QLineEdit::textChanged, this, changed);
    connect(email, &QLineEdit::textChanged, this, changed);
    const auto saveDetails = [this, api, base, version, name, email, save] {
        if (save->isEnabled())
            attempt(api->patch(base, QJsonObject{{QLatin1String("name"), name->text().trimmed()}, {QLatin1String("email"), email->text().trimmed()}}, version), tr("Saved the details"));
    };
    connect(save, &QPushButton::clicked, this, saveDetails);
    connect(name, &QLineEdit::returnPressed, this, saveDetails);
    connect(email, &QLineEdit::returnPressed, this, saveDetails);
    into->addWidget(details);

    // .account-switches
    auto *admin = new QCheckBox(tr("Administrator: may do everything on the server"), host);
    admin->setObjectName(QStringLiteral("isAdmin"));
    admin->setChecked(u.value(QLatin1String("admin")).toBool());
    auto *disabled = new QCheckBox(tr("Disabled: can't sign in; signed out everywhere"), host);
    disabled->setObjectName(QStringLiteral("isDisabled"));
    disabled->setChecked(u.value(QLatin1String("disabled")).toBool());
    into->addWidget(admin);
    into->addWidget(disabled);
    connect(admin, &QCheckBox::clicked, this, [this, api, base, version, username](bool on) {
        attempt(api->patch(base, QJsonObject{{QLatin1String("admin"), on}}, version), on ? tr("%1 is an administrator").arg(username) : tr("%1 is no longer an administrator").arg(username));
    });
    connect(disabled, &QCheckBox::clicked, this, [this, api, base, version, username](bool on) {
        attempt(api->patch(base, QJsonObject{{QLatin1String("disabled"), on}}, version), on ? tr("Disabled %1 and signed them out everywhere").arg(username) : tr("Enabled %1").arg(username));
    });

    // Password (.server-connect)
    const bool hasPassword = u.value(QLatin1String("hasPassword")).toBool();
    into->addWidget(Web::text(tr("Password"), "fieldLabel", host));
    auto *password = new QLineEdit(host);
    password->setObjectName(QStringLiteral("newPassword"));
    password->setEchoMode(QLineEdit::Password);
    auto *setPassword = Web::button(tr("Set the password"), Web::Normal, host);
    setPassword->setObjectName(QStringLiteral("setPassword"));
    setPassword->setEnabled(false);
    connect(password, &QLineEdit::textChanged, setPassword, [setPassword](const QString &t) { setPassword->setEnabled(!t.isEmpty()); });
    auto *pr = new QHBoxLayout;
    pr->setSpacing(S3);
    pr->addWidget(formField(hasPassword ? tr("New password") : tr("Password (the account has none)"), password, host), 1);
    pr->addWidget(setPassword, 0, Qt::AlignBottom);
    into->addLayout(pr);
    const auto sendPassword = [this, api, base, password, username] {
        if (!password->text().isEmpty())
            attempt(api->put(base + QStringLiteral("/password"), QJsonObject{{QLatin1String("password"), password->text()}}), tr("Set a new password for %1; their sign-ins ended").arg(username));
    };
    connect(setPassword, &QPushButton::clicked, this, sendPassword);
    connect(password, &QLineEdit::returnPressed, this, sendPassword);
    if (hasPassword && !m_identities.isEmpty()) {
        auto *remove = new Web::ConfirmButton(tr("Remove the password"), tr("%1 then signs in only through the identity provider.").arg(username), tr("Remove"), host);
        remove->setObjectName(QStringLiteral("removePassword"));
        connect(remove, &Web::ConfirmButton::confirmed, this, [this, api, base, username] {
            attempt(api->put(base + QStringLiteral("/password"), QJsonObject{{QLatin1String("password"), QString()}}), tr("%1 signs in through the provider only").arg(username));
        });
        into->addWidget(remove, 0, Qt::AlignLeft);
    }

    // Sign-ins
    into->addWidget(Web::text(tr("Sign-ins"), "fieldLabel", host));
    if (m_signIns.isEmpty()) {
        auto *none = Web::text(tr("Not signed in anywhere."), "muted", host);
        none->setObjectName(QStringLiteral("noSignIns"));
        into->addWidget(none);
    } else {
        for (const QJsonValue &v : m_signIns) {
            const QJsonObject t = v.toObject();
            auto *l = Web::text(tr("%1: since %2, last used %3, ends %4").arg(str(t, "name"), day(str(t, "createdAt")), day(str(t, "lastUsed")), day(str(t, "expiresAt"))), "cellSub", host);
            l->setProperty("part", QStringLiteral("signIn"));
            l->setWordWrap(true);
            into->addWidget(l);
        }
        auto *out = Web::button(tr("Sign out everywhere"), Web::Small, host);
        out->setObjectName(QStringLiteral("signOutEverywhere"));
        connect(out, &QPushButton::clicked, this, [this, api, base, username] { attempt(api->del(base + QStringLiteral("/tokens")), tr("Signed %1 out everywhere").arg(username)); });
        into->addWidget(out, 0, Qt::AlignLeft);
    }

    showIdentities(into, host, u);
    showRoles(into, host, u);

    if (!me) {
        // .account-delete
        into->addWidget(line(host));
        auto *del = new Web::ConfirmButton(tr("Delete the account"), tr("Delete %1? Their work stays signed with their name; their roles and sign-ins go.").arg(username), tr("Delete"), host, false,
                                           QStringLiteral("trash"));
        del->setObjectName(QStringLiteral("deleteAccount"));
        connect(del, &Web::ConfirmButton::confirmed, this, [this, api, base, username] {
            request(api->del(base), [this, username](const ApiResult &r) {
                if (!r.ok()) {
                    m_problem = r.error.message;
                    render();
                    return;
                }
                notify(tr("Deleted the account %1").arg(username));
                m_selected = 0;
                reload();
            });
        });
        into->addWidget(del, 0, Qt::AlignLeft);
    }
}

void AccountsPage::showIdentities(QVBoxLayout *into, QWidget *host, const QJsonObject &u)
{
    Connection *c = conn();
    const QString provider = c->provider();
    if (provider.isEmpty() && m_identities.isEmpty())
        return;
    const QString username = str(u, "username");
    const QString base = QStringLiteral("/api/users/%1/identities").arg(m_selected);
    ApiClient *api = c->api();
    into->addWidget(Web::text(tr("Sign-in through %1").arg(provider.isEmpty() ? tr("the identity provider") : provider), "fieldLabel", host));
    if (m_identities.isEmpty()) {
        auto *none = Web::text(tr("Not linked. %1 signs in with a password, or links it themselves after signing in.").arg(username), "muted", host);
        none->setWordWrap(true);
        into->addWidget(none);
    } else {
        for (const QJsonValue &v : m_identities) {
            const QJsonObject i = v.toObject();
            // .identity
            auto *row = new QHBoxLayout;
            row->setSpacing(S3);
            row->addWidget(Web::text(tr("%1, subject %2").arg(QUrl(str(i, "issuer")).host(), str(i, "subject")), "cellSub", host));
            auto *unlink = Web::button(tr("Unlink"), Web::Quiet | Web::Small, host);
            unlink->setProperty("part", QStringLiteral("unlink"));
            row->addWidget(unlink);
            row->addStretch();
            connect(unlink, &QPushButton::clicked, this, [this, api, base, i] {
                QUrlQuery q;
                q.addQueryItem(QStringLiteral("issuer"), QString::fromUtf8(QUrl::toPercentEncoding(str(i, "issuer"))));
                q.addQueryItem(QStringLiteral("subject"), QString::fromUtf8(QUrl::toPercentEncoding(str(i, "subject"))));
                attempt(api->del(base, q), tr("Unlinked the sign-in"));
            });
            into->addLayout(row);
        }
    }
    if (!provider.isEmpty()) {
        auto *subject = new QLineEdit(host);
        subject->setObjectName(QStringLiteral("subject"));
        auto *link = Web::button(tr("Link"), Web::Normal, host, QStringLiteral("link"));
        link->setObjectName(QStringLiteral("link"));
        link->setEnabled(false);
        connect(subject, &QLineEdit::textChanged, link, [link](const QString &t) { link->setEnabled(!t.trimmed().isEmpty()); });
        auto *row = new QHBoxLayout;
        row->setSpacing(S3);
        row->addWidget(formField(tr("Subject at %1 (its ID for the person)").arg(provider), subject, host), 1);
        row->addWidget(link, 0, Qt::AlignBottom);
        into->addLayout(row);
        const auto send = [this, api, base, subject, username] {
            if (!subject->text().trimmed().isEmpty())
                attempt(api->post(base, QJsonObject{{QLatin1String("issuer"), QString()}, {QLatin1String("subject"), subject->text().trimmed()}}), tr("Linked %1 to the sign-in").arg(username));
        };
        connect(link, &QPushButton::clicked, this, send);
        connect(subject, &QLineEdit::returnPressed, this, send);
    }
}

void AccountsPage::showRoles(QVBoxLayout *into, QWidget *host, const QJsonObject &u)
{
    Connection *c = conn();
    ApiClient *api = c->api();
    const QString username = str(u, "username");
    const qint64 id = m_selected;
    into->addWidget(Web::text(tr("Roles"), "fieldLabel", host));
    if (u.value(QLatin1String("admin")).toBool()) {
        auto *l = Web::text(tr("As an administrator, %1 may do everything; roles add nothing.").arg(username), "cellSub", host);
        l->setWordWrap(true);
        into->addWidget(l);
    }
    if (m_grants.isEmpty()) {
        auto *none = Web::text(tr("No roles. %1 sees only the projects with work given to them.").arg(username), "muted", host);
        none->setObjectName(QStringLiteral("noRoles"));
        none->setWordWrap(true);
        into->addWidget(none);
    } else {
        auto *table = new Web::Table({tr("On"), tr("Role"), QString()}, host);
        table->setObjectName(QStringLiteral("roles"));
        table->horizontalHeaderItem(2)->setData(Qt::AccessibleTextRole, tr("Remove"));
        table->setStretch(0);
        for (const QJsonValue &v : m_grants) {
            const QJsonObject g = v.toObject();
            const QString on = resourceName(g);
            auto *role = new QComboBox(table);
            role->setProperty("small", true);
            role->setAccessibleName(tr("Role on %1").arg(on));
            for (const QString &r : AccessPanel::roles())
                role->addItem(AccessPanel::roleText(r), r);
            role->setCurrentIndex(qMax(0, role->findData(str(g, "role"))));
            role->setMinimumHeight(26);
            connect(role, &QComboBox::activated, this, [this, api, g, role] {
                if (role->currentData().toString() == str(g, "role"))
                    return;
                QJsonObject now = g;
                now.insert(QLatin1String("role"), role->currentData().toString());
                attempt(api->put(QStringLiteral("/api/grants"), now), tr("Changed the role"));
            });
            auto *remove = Web::button(tr("Remove"), Web::Quiet | Web::Small, table);
            remove->setAccessibleName(tr("Remove the role on %1").arg(on));
            connect(remove, &QPushButton::clicked, this, [this, api, g, id] {
                QUrlQuery q;
                q.addQueryItem(QStringLiteral("userId"), QString::number(id));
                q.addQueryItem(QStringLiteral("kind"), str(g, "kind"));
                q.addQueryItem(QStringLiteral("resource"), QString::fromUtf8(QUrl::toPercentEncoding(str(g, "resource"))));
                attempt(api->del(QStringLiteral("/api/grants"), q), tr("Took the role away"));
            });
            const int row = table->addRow({Web::cell(on, kindText(str(g, "kind")), table), role, remove});
            table->item(row, 0)->setData(Qt::AccessibleTextRole, on);
            table->item(row, 1)->setData(Qt::AccessibleTextRole, AccessPanel::roleText(str(g, "role")));
        }
        table->fit();
        into->addWidget(table);
    }

    // .role-add
    auto *form = new QWidget(host);
    form->setObjectName(QStringLiteral("giveRole"));
    form->setAccessibleName(tr("Give a role"));
    auto *fl = new QHBoxLayout(form);
    fl->setContentsMargins(0, 0, 0, 0);
    fl->setSpacing(S3);
    auto *kind = new QComboBox(form);
    kind->setObjectName(QStringLiteral("roleKind"));
    kind->addItem(tr("Projects"), QStringLiteral("project"));
    kind->addItem(tr("Memories"), QStringLiteral("memory"));
    kind->addItem(tr("Termbases"), QStringLiteral("termbase"));
    kind->addItem(tr("Everything"), QStringLiteral("all"));
    auto *which = new QComboBox(form);
    which->setObjectName(QStringLiteral("roleResource"));
    auto *role = new QComboBox(form);
    role->setObjectName(QStringLiteral("roleRole"));
    for (const QString &r : AccessPanel::roles())
        role->addItem(AccessPanel::roleText(r), r);
    role->setCurrentIndex(qMax(0, role->findData(QStringLiteral("edit"))));
    auto *give = Web::button(tr("Give the role"), Web::Normal, form);
    give->setObjectName(QStringLiteral("give"));
    QWidget *whichBox = Web::labelled(tr("Which"), which, form);
    fl->addWidget(Web::labelled(tr("On"), kind, form));
    fl->addWidget(whichBox);
    fl->addWidget(Web::labelled(tr("Role"), role, form));
    fl->addWidget(give, 0, Qt::AlignBottom);
    fl->addStretch();
    const auto fill = [this, kind, which, whichBox] {
        const QString k = kind->currentData().toString();
        which->clear();
        which->addItem(tr("All of them, also new ones"), QStringLiteral("*"));
        if (k == QLatin1String("project")) {
            for (const QJsonValue &p : m_projects)
                which->addItem(str(p.toObject().value(QLatin1String("settings")).toObject(), "name"), QString::number(p.toObject().value(QLatin1String("id")).toInteger()));
        } else if (k == QLatin1String("memory") || k == QLatin1String("termbase")) {
            for (const QJsonValue &m : m_catalog.value(k == QLatin1String("memory") ? QLatin1String("memories") : QLatin1String("termbases")).toArray())
                which->addItem(str(m.toObject(), "name"), str(m.toObject(), "name"));
        }
        whichBox->setVisible(k != QLatin1String("all"));
    };
    fill();
    connect(kind, &QComboBox::activated, this, fill);
    connect(give, &QPushButton::clicked, this, [this, api, id, kind, which, role] {
        const QString k = kind->currentData().toString();
        attempt(api->put(QStringLiteral("/api/grants"), QJsonObject{{QLatin1String("userId"), id}, {QLatin1String("kind"), k},
                                                                    {QLatin1String("resource"), k == QLatin1String("all") ? QStringLiteral("*") : which->currentData().toString()},
                                                                    {QLatin1String("role"), role->currentData().toString()}}),
                tr("Gave the role"));
    });
    into->addWidget(form);
    auto *note = Web::text(tr("Manage on all projects makes someone a project manager: they make projects and give out work. Manage on all memories or termbases lets them make those."), "cellSub", host);
    note->setWordWrap(true);
    into->addWidget(note);
}
