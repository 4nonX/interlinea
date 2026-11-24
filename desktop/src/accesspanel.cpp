#include "accesspanel.h"

#include "roles.h"

#include "apiclient.h"
#include "connection.h"
#include "webui.h"

#include <QComboBox>
#include <QHBoxLayout>
#include <QJsonObject>
#include <QLabel>
#include <QPushButton>
#include <QVBoxLayout>

AccessPanel::AccessPanel(Connection *connection, const QString &kind, const QString &resource, const QString &title, QWidget *parent, bool embedded)
    : QFrame(parent)
    , m_conn(connection)
    , m_kind(kind)
    , m_resource(resource)
{
    setObjectName(embedded ? QStringLiteral("accessSection") : QStringLiteral("panel"));
    setAccessibleName(title);
    auto *v = new QVBoxLayout(this);
    const int pad = embedded ? 0 : 24;
    v->setContentsMargins(pad, pad, pad, pad);
    v->setSpacing(embedded ? 12 : 24);
    v->addWidget(Web::text(title, embedded ? "settingsTitle" : "panelTitle", this));
    m_problem = Web::text(QString(), "error", this);
    m_problem->hide();
    v->addWidget(m_problem);
    m_list = new QVBoxLayout;
    m_list->setSpacing(0);
    v->addLayout(m_list);

    // .role-add: who, the role, and the button, in a row.
    auto *add = new QHBoxLayout;
    add->setSpacing(12);
    m_who = new QComboBox(this);
    m_who->setObjectName(QStringLiteral("who"));
    // As wide as the names in it, as the web's select.
    m_who->setSizeAdjustPolicy(QComboBox::AdjustToContents);
    m_who->setMinimumContentsLength(18);
    m_role = new QComboBox(this);
    m_role->setObjectName(QStringLiteral("role"));
    Roles::fill(m_role, {}, kind == QLatin1String("project") ? QStringLiteral("edit") : QStringLiteral("read"));
    auto labelled = [this](const QString &label, QWidget *control) {
        auto *w = new QWidget(this);
        auto *l = new QVBoxLayout(w);
        l->setContentsMargins(0, 0, 0, 0);
        l->setSpacing(4);
        auto *t = Web::text(label, "fieldLabel", w);
        t->setBuddy(control);
        control->setAccessibleName(label);
        l->addWidget(t);
        l->addWidget(control);
        return w;
    };
    add->addWidget(labelled(tr("Who"), m_who));
    add->addWidget(labelled(tr("Role"), m_role));
    auto *give = Web::button(tr("Give the role"), Web::Normal, this);
    give->setObjectName(QStringLiteral("add"));
    give->setEnabled(false);
    add->addWidget(give, 0, Qt::AlignBottom);
    add->addStretch();
    v->addLayout(add);
    connect(m_who, &QComboBox::currentIndexChanged, give, [this, give] { give->setEnabled(m_who->currentData().toLongLong() > 0); });
    connect(give, &QPushButton::clicked, this, [this] {
        attempt(m_conn->api()->put(QStringLiteral("/api/grants"),
                                   QJsonObject{{QStringLiteral("userId"), m_who->currentData().toLongLong()},
                                               {QStringLiteral("kind"), m_kind},
                                               {QStringLiteral("resource"), m_resource},
                                               {QStringLiteral("role"), m_role->currentData().toString()}}));
    });
    reload();
}

QStringList AccessPanel::roles()
{
    return {QStringLiteral("read"), QStringLiteral("suggest"), QStringLiteral("edit"), QStringLiteral("approve"), QStringLiteral("manage")};
}

QString AccessPanel::roleText(const QString &role)
{
    return Roles::text(role);
}

void AccessPanel::attempt(ApiReply *reply)
{
    m_problem->hide();
    onResult(reply, this, [this](const ApiResult &r) {
        if (!r.ok()) {
            m_problem->setText(r.error.message);
            m_problem->show();
            return;
        }
        reload();
    });
}

void AccessPanel::reload()
{
    if (!m_conn)
        return;
    // The roles the place has, also those its administrators defined.
    onResult(m_conn->api()->get(QStringLiteral("/api/roles")), this, [this](const ApiResult &r) {
        if (!r.ok() || r.array() == m_roles)
            return;
        m_roles = r.array();
        Roles::fill(m_role, m_roles, m_role->currentData().toString());
        if (!m_grants.isEmpty())
            show(m_grants);
    });
    onResult(m_conn->api()->get(QStringLiteral("/api/people")), this, [this](const ApiResult &p) {
        if (p.ok())
            m_people = p.array();
        onResult(m_conn->api()->get(QStringLiteral("/api/grants"), {{QStringLiteral("kind"), m_kind}, {QStringLiteral("resource"), m_resource}}), this,
                 [this](const ApiResult &r) {
                     if (!r.ok()) {
                         m_problem->setText(r.error.message);
                         m_problem->show();
                         return;
                     }
                     show(r.array());
                 });
    });
}

void AccessPanel::show(const QJsonArray &grants)
{
    m_grants = grants;
    while (QLayoutItem *item = m_list->takeAt(0)) {
        if (QWidget *w = item->widget())
            w->deleteLater();
        delete item;
    }
    if (grants.isEmpty()) {
        m_list->addWidget(Web::text(m_kind == QLatin1String("project")
                                        ? tr("Nobody has a role on it yet; people with work given to them see it all the same.")
                                        : tr("Nobody has a role on it yet."),
                                    "muted", this));
    } else {
        auto *table = new Web::Table({tr("Who"), tr("Role"), QString()}, this);
        table->setStretch(0);
        table->setAccessibleName(tr("Roles"));
        for (const QJsonValue &v : grants) {
            const QJsonObject g = v.toObject();
            const qint64 user = g.value(QLatin1String("userId")).toInteger();
            const QString who = g.value(QLatin1String("name")).toString().isEmpty() ? g.value(QLatin1String("username")).toString()
                                                                                   : g.value(QLatin1String("name")).toString();
            auto *role = new QComboBox(table);
            role->setProperty("small", true);
            role->setAccessibleName(tr("Role of %1").arg(who));
            Roles::fill(role, m_roles, g.value(QLatin1String("role")).toString());
            connect(role, &QComboBox::activated, this, [this, role, user] {
                attempt(m_conn->api()->put(QStringLiteral("/api/grants"),
                                           QJsonObject{{QStringLiteral("userId"), user}, {QStringLiteral("kind"), m_kind},
                                                       {QStringLiteral("resource"), m_resource}, {QStringLiteral("role"), role->currentData().toString()}}));
            });
            auto *remove = Web::button(tr("Remove"), Web::Quiet | Web::Small, table);
            remove->setAccessibleName(tr("Remove the role of %1").arg(who));
            connect(remove, &QPushButton::clicked, this, [this, user] {
                attempt(m_conn->api()->del(QStringLiteral("/api/grants"), {{QStringLiteral("userId"), QString::number(user)},
                                                                         {QStringLiteral("kind"), m_kind},
                                                                         {QStringLiteral("resource"), m_resource}}));
            });
            table->addRow({Web::cell(who, QString(), table), role, Web::cellActions({remove}, table)});
        }
        table->fit();
        m_list->addWidget(table);
    }
    m_who->clear();
    m_who->addItem(tr("Choose someone"), 0);
    for (const QJsonValue &v : std::as_const(m_people)) {
        const QJsonObject p = v.toObject();
        bool has = false;
        for (const QJsonValue &g : grants)
            has = has || g.toObject().value(QLatin1String("userId")).toInteger() == p.value(QLatin1String("id")).toInteger();
        if (!has)
            m_who->addItem(QStringLiteral("%1 (%2)").arg(p.value(QLatin1String("name")).toString(), p.value(QLatin1String("username")).toString()),
                           p.value(QLatin1String("id")).toInteger());
    }
}
