#include "reviewpage.h"

#include "apiclient.h"
#include "connection.h"
#include "eventfeed.h"
#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "roles.h"
#include "termdata.h"
#include "theme.h"
#include "words.h"

#include <QDateTime>
#include <QHBoxLayout>
#include <QCheckBox>
#include <QComboBox>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QProgressBar>
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

QString day(const QString &iso)
{
    QString text = iso;
    text.replace(QRegularExpression(QStringLiteral("(\\.\\d{3})\\d+")), QStringLiteral("\\1"));
    QDateTime t = QDateTime::fromString(text, Qt::ISODateWithMs);
    if (!t.isValid())
        t = QDateTime::fromString(text, Qt::ISODate);
    return t.isValid() && t.date().year() > 1 ? QLocale().toString(t.toLocalTime().date(), QStringLiteral("d MMM yyyy")) : QString();
}

QString stateText(const QString &state)
{
    if (state == QLatin1String("waiting"))
        return ReviewPage::tr("Waiting");
    if (state == QLatin1String("approved"))
        return ReviewPage::tr("Approved");
    if (state == QLatin1String("approved-with-edits"))
        return ReviewPage::tr("Approved with edits");
    if (state == QLatin1String("changes-requested"))
        return ReviewPage::tr("Changes requested");
    if (state == QLatin1String("rejected"))
        return ReviewPage::tr("Rejected");
    if (state == QLatin1String("postponed"))
        return ReviewPage::tr("Postponed");
    if (state == QLatin1String("reverted"))
        return ReviewPage::tr("Reverted");
    return state;
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

} // namespace

ReviewPage::ReviewPage(QWidget *parent)
    : QWidget(parent)
{
    setObjectName(QStringLiteral("review"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    Web::Page page = Web::page(this);
    m_body = new QVBoxLayout;
    m_body->setSpacing(24);
    page.body->addLayout(m_body);
    page.body->addStretch();
    outer->addWidget(page.scroll);
    m_toast = new Web::Toast(this);
    m_reload.setSingleShot(true);
    m_reload.setInterval(200);
    connect(&m_reload, &QTimer::timeout, this, &ReviewPage::reload);
}

void ReviewPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

void ReviewPage::notify(const QString &text)
{
    m_toast->say(text);
}

QString ReviewPage::message() const
{
    return m_toast->last();
}

void ReviewPage::open(Connection *conn, int id)
{
    if (m_conn && m_conn != conn)
        disconnect(m_conn->feed(), nullptr, this, nullptr);
    if (conn && m_conn != conn) {
        // What others decide is shown as it happens.
        connect(conn->feed(), &EventFeed::event, this, [this](const QString &type) {
            if (type == QLatin1String("termbase") && m_id)
                m_reload.start();
        });
    }
    m_conn = conn;
    m_id = id;
    m_view = 0;
    m_limit = 50;
    m_mine = false;
    m_read = false;
    m_request = {};
    m_progress = {};
    m_items = {};
    m_target = {};
    m_reviewers = {};
    m_people = {};
    m_comments.clear();
    m_assignWho.clear();
    m_assignPart.clear();
    m_problem.clear();
    render();
    reload();
}

void ReviewPage::setView(int view)
{
    if (m_view == view)
        return;
    m_view = view;
    m_limit = 50;
    reload();
}

void ReviewPage::setMine(bool mine)
{
    if (m_mine == mine)
        return;
    m_mine = mine;
    m_limit = 50;
    reload();
}

bool ReviewPage::may(const char *permission) const
{
    // On this computer everything is one's own.
    const QString role = str(m_target, "role");
    if (role.isEmpty())
        return m_conn && m_conn->isLocal();
    return Roles::allows(role, QLatin1String(permission));
}

void ReviewPage::reload()
{
    if (!m_conn || !m_id)
        return;
    const int reading = ++m_reading;
    const QString base = QStringLiteral("/api/merge-requests/%1").arg(m_id);
    const bool team = !m_conn->isLocal();
    auto *left = new int(team ? 6 : 4);
    auto *failed = new QString;
    const auto done = [this, left, failed, reading](const ApiResult &r) {
        if (!r.ok() && failed->isEmpty())
            *failed = r.error.message;
        if (--*left > 0)
            return;
        const QString problem = *failed;
        delete left;
        delete failed;
        if (reading != m_reading)
            return;
        m_problem = problem;
        m_read = problem.isEmpty();
        render();
        emit loaded();
    };
    onResult(m_conn->api()->get(base), this, [this, reading, done](const ApiResult &r) {
        if (reading == m_reading && r.ok())
            m_request = r.object();
        done(r);
        // The master says what one may do with the request.
        if (reading != m_reading || !r.ok() || !m_conn) {
            done(ApiResult{});
            return;
        }
        onResult(m_conn->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(str(r.object(), "target"))), this, [this, reading, done](const ApiResult &t) {
            if (reading == m_reading && t.ok())
                m_target = t.object();
            // Without a look at the master the request is still shown.
            done(ApiResult{});
        });
    });
    if (team) {
        onResult(m_conn->api()->get(base + QStringLiteral("/reviewers")), this, [this, reading, done](const ApiResult &r) {
            if (reading == m_reading && r.ok())
                m_reviewers = r.array();
            done(r);
        });
        onResult(m_conn->api()->get(QStringLiteral("/api/people")), this, [this, reading, done](const ApiResult &r) {
            if (reading == m_reading && r.ok())
                m_people = r.array();
            done(ApiResult{});
        });
    }
    onResult(m_conn->api()->get(base + QStringLiteral("/progress")), this, [this, reading, done](const ApiResult &r) {
        if (reading == m_reading && r.ok())
            m_progress = r.object();
        done(r);
    });
    QJsonObject filter;
    if (m_view == 0)
        filter.insert(QLatin1String("states"), QJsonArray{QStringLiteral("waiting")});
    else if (m_view == 1)
        filter.insert(QLatin1String("states"), QJsonArray{QStringLiteral("approved"), QStringLiteral("approved-with-edits"), QStringLiteral("changes-requested"), QStringLiteral("rejected"),
                                                          QStringLiteral("postponed"), QStringLiteral("reverted")});
    QJsonObject query{{QLatin1String("filter"), filter}, {QLatin1String("limit"), m_limit}, {QLatin1String("offset"), 0}};
    if (m_mine)
        query.insert(QLatin1String("reviewer"), m_conn->login());
    onResult(m_conn->api()->post(base + QStringLiteral("/items"), query), this,
             [this, reading, done](const ApiResult &r) {
                 if (reading == m_reading && r.ok()) {
                     m_items = r.object().value(QLatin1String("items")).toArray();
                     m_total = r.object().value(QLatin1String("total")).toInt();
                 }
                 done(r);
             });
}

void ReviewPage::decide(const QJsonObject &filter, const QString &state, const QJsonObject &edited, const QString &what)
{
    if (!m_conn)
        return;
    QJsonObject decision{{QLatin1String("state"), state}};
    if (!edited.isEmpty())
        decision.insert(QLatin1String("edited"), edited);
    // What was written for the one entry goes with its decision.
    const QJsonArray ids = filter.value(QLatin1String("ids")).toArray();
    if (ids.size() == 1) {
        const QString comment = m_comments.take(ids.first().toInt()).trimmed();
        if (!comment.isEmpty())
            decision.insert(QLatin1String("comment"), comment);
    }
    const QString target = str(m_request, "target");
    onResult(m_conn->api()->post(QStringLiteral("/api/merge-requests/%1/decide").arg(m_id), QJsonObject{{QLatin1String("filter"), filter}, {QLatin1String("decision"), decision}}), this,
             [this, state, what, target](const ApiResult &r) {
                 if (!r.ok()) {
                     notify(r.status == 403 ? tr("%1 Ask a manager of %2 for the approve role.").arg(r.error.message, target) : r.error.message);
                     return;
                 }
                 const int left = int(r.object().value(QLatin1String("needResolution")).toArray().size());
                 QString text = QStringLiteral("%1: %2").arg(what.isEmpty() ? stateText(state) : what, TermData::entries(r.object().value(QLatin1String("decided")).toInt()));
                 if (left)
                     text += left == 1 ? tr("; 1 entry needs a decision one by one (the master changed the same places)")
                                       : tr("; %1 entries need a decision one by one (the master changed the same places)").arg(left);
                 notify(text);
                 reload();
             });
}

void ReviewPage::render()
{
    clear(m_body);
    QWidget *host = m_body->parentWidget();
    const QString project = str(m_request, "project");
    auto *back = Web::button(tr("Back to the termbase"), Web::Quiet, host);
    back->setObjectName(QStringLiteral("back"));
    connect(back, &QPushButton::clicked, this, [this, project] { emit this->back(m_conn ? m_conn->id() : QString(), project); });
    if (!m_read) {
        m_body->addWidget(Web::header(tr("Review"), QString(), {back}, host));
        auto *state = Web::text(m_problem.isEmpty() ? tr("Loading the request…") : m_problem, m_problem.isEmpty() ? "muted" : "error", host);
        state->setObjectName(QStringLiteral("state"));
        state->setWordWrap(true);
        m_body->addWidget(state);
        return;
    }
    const bool open = str(m_request, "state") == QLatin1String("open");
    const QJsonObject states = m_progress.value(QLatin1String("states")).toObject();
    const int waiting = states.value(QLatin1String("waiting")).toInt();
    const int total = m_progress.value(QLatin1String("total")).toInt();
    const QString target = str(m_request, "target");

    m_body->addWidget(Web::header(str(m_request, "title"), QString(), {back}, host));
    QString about = tr("%1 into %2. Submitted by %3 on %4").arg(project, target, str(m_request, "createdBy"), day(str(m_request, "createdAt")));
    if (const QString by = day(str(m_request, "deadline")); !by.isEmpty())
        about += tr(", to be decided by %1").arg(by);
    about += QLatin1Char('.');
    if (!open)
        about += QLatin1Char(' ') + tr("Closed on %1.").arg(day(str(m_request, "closedAt").isEmpty() ? str(m_request, "createdAt") : str(m_request, "closedAt")));
    auto *help = Web::text(about, "help", host);
    help->setObjectName(QStringLiteral("about"));
    help->setWordWrap(true);
    m_body->addWidget(help);

    // Progress
    QVBoxLayout *pv = nullptr;
    QFrame *panel = Web::panel(tr("Progress"), host, &pv);
    QStringList parts;
    for (const QString &s : {QStringLiteral("waiting"), QStringLiteral("approved"), QStringLiteral("approved-with-edits"), QStringLiteral("changes-requested"), QStringLiteral("rejected"),
                             QStringLiteral("postponed"), QStringLiteral("reverted")}) {
        if (const int n = states.value(s).toInt())
            parts << QStringLiteral("%1 %2").arg(n).arg(stateText(s).toLower());
    }
    QString summary = QStringLiteral("%1: %2").arg(TermData::entries(total), parts.join(QStringLiteral(", ")));
    if (const int c = m_progress.value(QLatin1String("conflicts")).toInt())
        summary += c == 1 ? tr("; 1 conflict with the master") : tr("; %1 conflicts with the master").arg(c);
    summary += QLatin1Char('.');
    auto *sum = new QLabel(summary, panel);
    sum->setObjectName(QStringLiteral("summary"));
    sum->setWordWrap(true);
    pv->addWidget(sum);
    auto *bar = new QProgressBar(panel);
    bar->setObjectName(QStringLiteral("track"));
    bar->setTextVisible(false);
    bar->setRange(0, qMax(1, total));
    bar->setValue(total - waiting);
    bar->setAccessibleName(tr("Decided"));
    pv->addWidget(bar);
    if (open && !may("approve")) {
        auto *wait = Web::text(tr("Those who may approve changes to %1 decide. You see how far they are.").arg(target), "help", panel);
        wait->setObjectName(QStringLiteral("readOnly"));
        wait->setWordWrap(true);
        pv->addWidget(wait);
    } else if (open) {
        auto *all = Web::button(tr("Approve all waiting without conflicts"), Web::Primary, panel, QStringLiteral("check"));
        all->setObjectName(QStringLiteral("approveAll"));
        all->setEnabled(waiting > 0);
        connect(all, &QPushButton::clicked, this, [this] {
            decide(QJsonObject{{QLatin1String("states"), QJsonArray{QStringLiteral("waiting")}}, {QLatin1String("conflicts"), false}}, QStringLiteral("approved"), {},
                   tr("Approved all waiting entries without conflicts"));
        });
        auto *close = new Web::ConfirmButton(tr("Close the request"),
                                             waiting == 0 ? tr("Close the request?") : waiting == 1 ? tr("1 entry is still waiting. Close anyway?") : tr("%1 entries are still waiting. Close anyway?").arg(waiting),
                                             tr("Close"), panel, false);
        close->setObjectName(QStringLiteral("close"));
        connect(close, &Web::ConfirmButton::confirmed, this, [this] {
            if (!m_conn)
                return;
            onResult(m_conn->api()->post(QStringLiteral("/api/merge-requests/%1/close").arg(m_id)), this, [this](const ApiResult &r) {
                notify(r.ok() ? tr("Closed the request") : r.error.message);
                reload();
            });
        });
        pv->addWidget(Web::row({all, close}, panel));
    }
    m_body->addWidget(panel);
    if (m_conn && !m_conn->isLocal())
        m_body->addWidget(reviewersPanel(host));

    // .tabs-small
    auto *tabs = new Web::Tabs(tr("Entries to show"), host, true);
    tabs->setObjectName(QStringLiteral("views"));
    tabs->add(tr("Waiting (%1)").arg(waiting));
    tabs->add(tr("Decided"));
    tabs->add(tr("All"));
    tabs->setCurrent(m_view);
    connect(tabs, &Web::Tabs::changed, this, &ReviewPage::setView);
    m_body->addWidget(tabs);
    // Someone given a part of the request can look at that part alone.
    bool assigned = false;
    for (const QJsonValue &a : std::as_const(m_reviewers))
        assigned = assigned || (m_conn && str(a.toObject(), "reviewer") == m_conn->login());
    if (assigned || m_mine) {
        auto *mine = new QCheckBox(tr("Only my part"), host);
        mine->setObjectName(QStringLiteral("mine"));
        mine->setChecked(m_mine);
        connect(mine, &QCheckBox::toggled, this, &ReviewPage::setMine);
        m_body->addWidget(mine);
    }

    if (m_items.isEmpty()) {
        auto *none = Web::text(m_view == 0 ? tr("Nothing is waiting.") : tr("No entries here."), "muted", host);
        none->setObjectName(QStringLiteral("none"));
        m_body->addWidget(none);
        return;
    }
    // .mr-items
    auto *list = new QVBoxLayout;
    list->setSpacing(S3);
    for (const QJsonValue &v : m_items)
        list->addWidget(item(v.toObject(), host));
    m_body->addLayout(list);
    if (m_total > m_items.size()) {
        auto *more = Web::button(tr("Show more"), Web::Small, host);
        more->setObjectName(QStringLiteral("more"));
        connect(more, &QPushButton::clicked, this, [this] {
            m_limit += 50;
            reload();
        });
        auto *shown = Web::text(tr("Showing the first %1 of %2.").arg(m_items.size()).arg(m_total), "cellSub", host);
        shown->setObjectName(QStringLiteral("shown"));
        m_body->addWidget(Web::row({shown, more}, host));
    }
}

// Who reviews which part of the request, on a team server.
QWidget *ReviewPage::reviewersPanel(QWidget *host)
{
    const QString target = str(m_request, "target");
    const bool open = str(m_request, "state") == QLatin1String("open");
    QVBoxLayout *v = nullptr;
    QFrame *panel = Web::panel(tr("Reviewers"), host, &v);
    panel->setProperty("part", QStringLiteral("reviewers"));
    const auto nameOf = [this](const QString &username) {
        for (const QJsonValue &p : std::as_const(m_people)) {
            if (str(p.toObject(), "username") == username && !str(p.toObject(), "name").isEmpty())
                return str(p.toObject(), "name");
        }
        return username;
    };
    const auto partText = [](const QJsonObject &scope) {
        QStringList parts;
        QStringList langs;
        for (const QJsonValue &l : scope.value(QLatin1String("langs")).toArray())
            langs << Languages::name(l.toString());
        if (!langs.isEmpty())
            parts << tr("entries with terms in %1").arg(langs.join(QStringLiteral(", ")));
        for (const QJsonValue &k : scope.value(QLatin1String("kinds")).toArray())
            parts << (k.toString() == QLatin1String("new") ? tr("new entries") : k.toString() == QLatin1String("changed") ? tr("changed entries") : tr("entries deleted in the project"));
        if (scope.value(QLatin1String("conflicts")).toBool())
            parts << tr("entries with conflicts");
        QStringList authors;
        for (const QJsonValue &a : scope.value(QLatin1String("authors")).toArray())
            authors << a.toString();
        if (!authors.isEmpty())
            parts << tr("entries by %1").arg(authors.join(QStringLiteral(", ")));
        return parts.isEmpty() ? tr("all of it") : parts.join(QStringLiteral("; "));
    };
    if (m_reviewers.isEmpty()) {
        auto *none = Web::text(tr("Nobody is given a part: everyone who may approve changes to %1 sees all of it.").arg(target), "help", panel);
        none->setObjectName(QStringLiteral("noReviewers"));
        none->setWordWrap(true);
        v->addWidget(none);
    }
    const QJsonObject by = m_progress.value(QLatin1String("byReviewer")).toObject();
    for (const QJsonValue &av : std::as_const(m_reviewers)) {
        const QJsonObject a = av.toObject();
        const QString reviewer = str(a, "reviewer");
        auto *rowWidget = new QWidget(panel);
        rowWidget->setProperty("part", QStringLiteral("reviewer"));
        auto *h = new QHBoxLayout(rowWidget);
        h->setContentsMargins(0, 0, 0, 0);
        h->setSpacing(S3);
        h->addWidget(new QLabel(QStringLiteral("<b>%1</b>").arg(nameOf(reviewer).toHtmlEscaped()), rowWidget));
        // How far the reviewer is with the part.
        const QJsonObject counts = by.value(reviewer).toObject();
        int all = 0;
        for (auto it = counts.constBegin(); it != counts.constEnd(); ++it)
            all += it.value().toInt();
        const int waiting = counts.value(QLatin1String("waiting")).toInt() + counts.value(QLatin1String("postponed")).toInt();
        QString what = partText(a.value(QLatin1String("scope")).toObject());
        if (all > 0)
            what += QStringLiteral(": ") + (waiting == 0 ? tr("all %1 decided").arg(all) : tr("%1 of %2 to decide").arg(waiting).arg(all));
        auto *sub = Web::text(what, "cellSub", rowWidget);
        sub->setWordWrap(true);
        h->addWidget(sub, 1);
        rowWidget->setAccessibleName(QStringLiteral("%1, %2").arg(nameOf(reviewer), what));
        if (open && may("manage")) {
            auto *remove = Web::button(tr("Remove"), Web::Quiet | Web::Small, rowWidget);
            remove->setObjectName(QStringLiteral("unassign:%1").arg(reviewer));
            remove->setAccessibleName(tr("Take the part from %1").arg(nameOf(reviewer)));
            h->addWidget(remove);
            connect(remove, &QPushButton::clicked, this, [this, reviewer, nameOf] {
                if (!m_conn)
                    return;
                QUrlQuery q;
                q.addQueryItem(QStringLiteral("reviewer"), QString::fromUtf8(QUrl::toPercentEncoding(reviewer)));
                const QString name = nameOf(reviewer);
                onResult(m_conn->api()->del(QStringLiteral("/api/merge-requests/%1/reviewers").arg(m_id), q), this, [this, name](const ApiResult &r) {
                    notify(r.ok() ? tr("%1 no longer has a part of the request").arg(name) : r.error.message);
                    reload();
                });
            });
        }
        v->addWidget(rowWidget);
    }
    if (!open || !may("manage"))
        return panel;

    // .role-add: who, which part, and the button.
    auto *add = new QWidget(panel);
    add->setAccessibleName(tr("Give someone a part"));
    auto *al = new QHBoxLayout(add);
    al->setContentsMargins(0, 0, 0, 0);
    al->setSpacing(S3);
    auto *who = new QComboBox(add);
    who->setObjectName(QStringLiteral("assignWho"));
    who->setSizeAdjustPolicy(QComboBox::AdjustToContents);
    who->setMinimumContentsLength(18);
    who->addItem(tr("Choose someone"), QString());
    for (const QJsonValue &p : std::as_const(m_people)) {
        const QString username = str(p.toObject(), "username");
        who->addItem(str(p.toObject(), "name").isEmpty() ? username : QStringLiteral("%1 (%2)").arg(str(p.toObject(), "name"), username), username);
    }
    who->setCurrentIndex(qMax(0, who->findData(m_assignWho)));
    connect(who, &QComboBox::activated, this, [this, who](int i) { m_assignWho = who->itemData(i).toString(); });
    al->addWidget(Web::labelled(tr("Who"), who, add));
    auto *part = new QComboBox(add);
    part->setObjectName(QStringLiteral("assignPart"));
    part->addItem(tr("All of it"), QString());
    for (const QJsonValue &l : m_target.value(QLatin1String("definition")).toObject().value(QLatin1String("languages")).toArray())
        part->addItem(tr("Terms in %1").arg(Languages::name(l.toString())), QStringLiteral("lang:") + l.toString());
    part->addItem(tr("New entries"), QStringLiteral("kind:new"));
    part->addItem(tr("Changed entries"), QStringLiteral("kind:changed"));
    part->addItem(tr("Entries with conflicts"), QStringLiteral("conflicts"));
    part->setCurrentIndex(qMax(0, part->findData(m_assignPart)));
    connect(part, &QComboBox::activated, this, [this, part](int i) { m_assignPart = part->itemData(i).toString(); });
    al->addWidget(Web::labelled(tr("Part"), part, add));
    auto *give = Web::button(tr("Give the part"), Web::Normal, add);
    give->setObjectName(QStringLiteral("assign"));
    al->addWidget(give, 0, Qt::AlignBottom);
    al->addStretch();
    v->addWidget(add);
    auto *help = Web::text(tr("A reviewer must be able to approve changes to %1. Giving someone a part again replaces the one they had.").arg(target), "help", panel);
    help->setWordWrap(true);
    v->addWidget(help);
    connect(give, &QPushButton::clicked, this, [this, who, part, nameOf] {
        const QString reviewer = who->currentData().toString();
        if (reviewer.isEmpty() || !m_conn) {
            notify(tr("Choose who reviews."));
            return;
        }
        QJsonObject scope;
        const QString p = part->currentData().toString();
        if (p.startsWith(QLatin1String("lang:")))
            scope.insert(QLatin1String("langs"), QJsonArray{p.mid(5)});
        else if (p.startsWith(QLatin1String("kind:")))
            scope.insert(QLatin1String("kinds"), QJsonArray{p.mid(5)});
        else if (p == QLatin1String("conflicts"))
            scope.insert(QLatin1String("conflicts"), true);
        const QString name = nameOf(reviewer);
        onResult(m_conn->api()->put(QStringLiteral("/api/merge-requests/%1/reviewers").arg(m_id), QJsonObject{{QLatin1String("reviewer"), reviewer}, {QLatin1String("scope"), scope}}), this,
                 [this, name](const ApiResult &r) {
                     if (!r.ok()) {
                         notify(r.error.message);
                         return;
                     }
                     m_assignWho.clear();
                     m_assignPart.clear();
                     notify(tr("%1 reviews a part of the request").arg(name));
                     reload();
                 });
    });
    return panel;
}

QWidget *ReviewPage::item(const QJsonObject &it, QWidget *parent)
{
    const int id = it.value(QLatin1String("id")).toInt();
    const QString state = str(it, "state");
    const QString kind = str(it, "kind");
    const bool open = str(m_request, "state") == QLatin1String("open");
    QVBoxLayout *v = nullptr;
    QFrame *panel = Web::panel(QString(), parent, &v);
    panel->setProperty("item", id);
    v->setSpacing(S2);

    // .mr-head
    auto *head = new QHBoxLayout;
    head->setSpacing(S3);
    const QString kindText = kind == QLatin1String("new") ? tr("New entry") : kind == QLatin1String("changed") ? tr("Changed entry") : tr("Deleted in the project");
    auto *title = new QLabel(QStringLiteral("<b>%1</b>").arg(kindText), panel);
    head->addWidget(title);
    QString meta = tr("by %1, %2").arg(str(it, "author"), day(str(it, "changedAt")));
    QStringList reviewers;
    for (const QJsonValue &r : it.value(QLatin1String("reviewers")).toArray())
        reviewers << r.toString();
    if (!reviewers.isEmpty() && (state == QLatin1String("waiting") || state == QLatin1String("postponed")))
        meta += tr("; for %1 to review").arg(reviewers.join(QStringLiteral(", ")));
    head->addWidget(Web::text(meta, "cellSub", panel), 1);
    head->addWidget(Web::chip(stateText(state), state == QLatin1String("waiting") ? QStringLiteral("proposed") : state.startsWith(QLatin1String("approved")) ? QStringLiteral("approved") : QStringLiteral("forbidden"), panel));
    v->addLayout(head);

    // .mr-terms
    const QJsonObject entry = it.value(QLatin1String("entry")).toObject();
    QStringList said{kindText};
    for (const QString &l : TermData::languages(entry)) {
        QStringList ts;
        for (const QJsonValue &t : TermData::terms(entry, l))
            ts << str(t.toObject(), "text");
        auto *row = new QHBoxLayout;
        row->setSpacing(S2);
        row->addWidget(Web::langChip(l, panel));
        auto *terms = new QLabel(ts.join(QStringLiteral("; ")), panel);
        terms->setWordWrap(true);
        terms->setTextFormat(Qt::PlainText);
        if (Languages::rtl(l))
            terms->setLayoutDirection(Qt::RightToLeft);
        row->addWidget(terms, 1);
        v->addLayout(row);
        said << QStringLiteral("%1: %2").arg(Languages::name(l), ts.join(QStringLiteral("; ")));
    }
    panel->setAccessibleName(said.join(QStringLiteral(". ")));

    // .mr-changes: what the project changed.
    const QString okBg = Theme::color("okBg").name(), ok = Theme::dark() ? Theme::color("ok").name() : QStringLiteral("#145f36");
    const QString nothing = tr("nothing");
    if (kind == QLatin1String("changed")) {
        for (const QJsonValue &cv : it.value(QLatin1String("changes")).toArray()) {
            const QJsonObject c = cv.toObject();
            const QString before = str(c, "old").isEmpty() ? nothing : str(c, "old"), after = str(c, "new").isEmpty() ? nothing : str(c, "new");
            auto *l = new QLabel(QStringLiteral("<span style=\"color:%1; font-size:13px\">%2:</span> <span style=\"color:%1; text-decoration: line-through\">%3</span> → "
                                                "<span style=\"background:%4; color:%5; font-weight:600\">&nbsp;%6&nbsp;</span>")
                                     .arg(Theme::color("text3").name(), str(c, "path").toHtmlEscaped(), before.toHtmlEscaped(), okBg, ok, after.toHtmlEscaped()),
                                 panel);
            l->setWordWrap(true);
            l->setContentsMargins(S4, 0, 0, 0);
            l->setAccessibleName(tr("%1: %2 becomes %3").arg(str(c, "path"), before, after));
            v->addWidget(l);
        }
    }
    const QJsonArray conflicts = it.value(QLatin1String("conflicts")).toArray();
    if (!conflicts.isEmpty()) {
        // .notice
        QStringList lines;
        for (const QJsonValue &cv : conflicts) {
            const QJsonObject c = cv.toObject();
            QString line = tr("%1: the project has “%2”, %3 has “%4”")
                               .arg(str(c, "path"), str(c, "ours").isEmpty() ? nothing : str(c, "ours"), str(m_request, "target"), str(c, "theirs").isEmpty() ? nothing : str(c, "theirs"));
            if (!str(c, "base").isEmpty())
                line += tr(" (was “%1”)").arg(str(c, "base"));
            lines << line;
        }
        auto *notice = new QFrame(panel);
        notice->setObjectName(QStringLiteral("message"));
        auto *nh = new QHBoxLayout(notice);
        nh->setContentsMargins(S4, S3, S4, S3);
        nh->setSpacing(S3);
        auto *icon = new QLabel(notice);
        icon->setPixmap(Icons::get(QStringLiteral("alert-triangle"), Theme::color("accentText")).pixmap(16, 16));
        nh->addWidget(icon, 0, Qt::AlignTop);
        auto *nv = new QVBoxLayout;
        nv->setSpacing(S1);
        nv->addWidget(new QLabel(QStringLiteral("<b>%1</b>").arg(tr("The master changed the same places")), notice));
        for (const QString &line : lines) {
            auto *l = new QLabel(line, notice);
            l->setWordWrap(true);
            l->setTextFormat(Qt::PlainText);
            nv->addWidget(l);
        }
        if (open && may("approve") && (state == QLatin1String("waiting") || state == QLatin1String("postponed")) && !it.value(QLatin1String("merged")).toObject().isEmpty()) {
            auto *choose = Web::text(tr("Choose which of the two goes into %1. What only one side changed is merged either way.").arg(str(m_request, "target")), "cellSub", notice);
            choose->setProperty("part", QStringLiteral("chooseSide"));
            choose->setWordWrap(true);
            nv->addWidget(choose);
        }
        nh->addLayout(nv, 1);
        v->addWidget(notice);
    }
    const auto warn = [&](const QString &text) {
        auto *l = new QLabel(text, panel);
        l->setStyleSheet(QStringLiteral("color: %1; font-size: 13px;").arg(Theme::color("warn").name()));
        l->setWordWrap(true);
        v->addWidget(l);
    };
    if (it.value(QLatin1String("forbidden")).toBool())
        warn(tr("It has a forbidden term."));
    if (const int n = int(it.value(QLatin1String("duplicates")).toArray().size()))
        warn(n == 1 ? tr("1 master entry shares a term with it.") : tr("%1 master entries share a term with it.").arg(n));
    if (!str(it, "comment").isEmpty()) {
        auto *c = Web::text(str(it, "reviewer").isEmpty() ? tr("“%1”").arg(str(it, "comment")) : tr("“%1”, %2").arg(str(it, "comment"), str(it, "reviewer")), "cellSub", panel);
        c->setWordWrap(true);
        v->addWidget(c);
    }

    // What becomes of an entry that did not go into the master.
    if (state == QLatin1String("reverted") || state == QLatin1String("rejected") || state == QLatin1String("changes-requested")) {
        const QString target = str(m_request, "target");
        auto *next = Web::text(state == QLatin1String("reverted") ? tr("Taken out of %1 again. It is submitted again once the project has changed it.").arg(target)
                                                                 : tr("Not in %1. It is submitted again once the project has changed it.").arg(target),
                               "cellSub", panel);
        next->setProperty("part", QStringLiteral("whatNext"));
        next->setWordWrap(true);
        v->addWidget(next);
    }
    // What was merged can be taken out of the master again.
    if (open && may("approve") && (state == QLatin1String("approved") || state == QLatin1String("approved-with-edits")) && str(it, "rule") != QLatin1String("identical")) {
        auto *undo = new Web::ConfirmButton(tr("Undo the merge"), tr("Take this out of %1 again? The entry there goes back to what it was before.").arg(str(m_request, "target")), tr("Undo"), panel);
        undo->setObjectName(QStringLiteral("undo"));
        v->addWidget(undo, 0, Qt::AlignLeft);
        connect(undo, &Web::ConfirmButton::confirmed, this, [this, id] {
            if (!m_conn)
                return;
            const QString target = str(m_request, "target");
            onResult(m_conn->api()->post(QStringLiteral("/api/merge-requests/%1/items/%2/revert").arg(m_id).arg(id)), this, [this, target](const ApiResult &r) {
                if (r.ok())
                    notify(tr("Took the merge back out of %1").arg(target));
                else if (r.status == 409)
                    notify(tr("The entry in %1 was changed after the merge. Change it there by hand.").arg(target));
                else
                    notify(r.error.message);
                reload();
            });
        });
    }
    const bool waits = state == QLatin1String("waiting") || state == QLatin1String("postponed");
    if (open && waits && may("approve")) {
        // What the author is told with the decision.
        auto *comment = new QLineEdit(m_comments.value(id), panel);
        comment->setObjectName(QStringLiteral("comment"));
        comment->setPlaceholderText(tr("A note for the author, with your decision"));
        comment->setAccessibleName(tr("A note for the author"));
        comment->setMaximumWidth(520);
        connect(comment, &QLineEdit::textEdited, this, [this, id](const QString &t) { m_comments.insert(id, t); });
        v->addWidget(comment);
    }
    if (open && waits && may("approve")) {
        // .server-actions
        const QJsonObject only{{QLatin1String("ids"), QJsonArray{id}}};
        const QJsonObject merged = it.value(QLatin1String("merged")).toObject();
        QList<QWidget *> actions;
        if (!conflicts.isEmpty() && !merged.isEmpty()) {
            // Each way out names what it puts into the master: the value
            // itself for one conflict, how many places for several.
            const auto result = [&conflicts, &nothing](const char *side) {
                if (conflicts.size() != 1)
                    return tr("%1 places").arg(conflicts.size());
                const QString value = str(conflicts.first().toObject(), side);
                return tr("“%1”").arg(value.isEmpty() ? nothing : value);
            };
            QJsonObject ours = it.value(QLatin1String("mergedOurs")).toObject();
            if (ours.isEmpty())
                ours = entry;
            const QString mine = result("ours"), theirs = result("theirs");
            auto *useProject = Web::button(QStringLiteral("%1: %2").arg(Words::get("useProject"), mine), Web::Small, panel, QStringLiteral("check"));
            useProject->setObjectName(QStringLiteral("useProject"));
            connect(useProject, &QPushButton::clicked, this, [this, only, ours, mine] { decide(only, QStringLiteral("approved-with-edits"), ours, tr("Used the project version (%1)").arg(mine)); });
            auto *keepMaster = Web::button(QStringLiteral("%1: %2").arg(Words::get("keepMaster"), theirs), Web::Small, panel, QStringLiteral("check"));
            keepMaster->setObjectName(QStringLiteral("keepMaster"));
            connect(keepMaster, &QPushButton::clicked, this, [this, only, merged, theirs] { decide(only, QStringLiteral("approved-with-edits"), merged, tr("Kept the master version (%1)").arg(theirs)); });
            actions << useProject << keepMaster;
        } else {
            auto *approve = Web::button(tr("Approve"), Web::Primary | Web::Small, panel, QStringLiteral("check"));
            approve->setObjectName(QStringLiteral("approve"));
            connect(approve, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("approved"), {}, QString()); });
            actions << approve;
        }
        auto *postpone = Web::button(tr("Postpone"), Web::Small, panel, QStringLiteral("clock"));
        postpone->setObjectName(QStringLiteral("postpone"));
        auto *reject = Web::button(tr("Reject"), Web::Small, panel, QStringLiteral("x"));
        reject->setObjectName(QStringLiteral("reject"));
        connect(postpone, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("postponed"), {}, QString()); });
        connect(reject, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("rejected"), {}, QString()); });
        // What was postponed is decided now, not put off again.
        if (state == QLatin1String("postponed"))
            postpone->hide();
        else
            actions << postpone;
        actions << reject;
        v->addWidget(Web::row(actions, panel));
    }
    return panel;
}
