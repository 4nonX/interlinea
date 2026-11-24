#include "reviewpage.h"

#include "apiclient.h"
#include "connection.h"
#include "eventfeed.h"
#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "termdata.h"
#include "theme.h"

#include <QDateTime>
#include <QHBoxLayout>
#include <QLabel>
#include <QLocale>
#include <QProgressBar>
#include <QPushButton>
#include <QRegularExpression>
#include <QResizeEvent>
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
    m_read = false;
    m_request = {};
    m_progress = {};
    m_items = {};
    m_problem.clear();
    show();
    reload();
}

void ReviewPage::setView(int view)
{
    if (m_view == view)
        return;
    m_view = view;
    reload();
}

void ReviewPage::reload()
{
    if (!m_conn || !m_id)
        return;
    const int reading = ++m_reading;
    const QString base = QStringLiteral("/api/merge-requests/%1").arg(m_id);
    auto *left = new int(3);
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
        show();
        emit loaded();
    };
    onResult(m_conn->api()->get(base), this, [this, reading, done](const ApiResult &r) {
        if (reading == m_reading && r.ok())
            m_request = r.object();
        done(r);
    });
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
    onResult(m_conn->api()->post(base + QStringLiteral("/items"), QJsonObject{{QLatin1String("filter"), filter}, {QLatin1String("limit"), 50}, {QLatin1String("offset"), 0}}), this,
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

void ReviewPage::show()
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
    if (open) {
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

    // .tabs-small
    auto *tabs = new Web::Tabs(tr("Entries to show"), host, true);
    tabs->setObjectName(QStringLiteral("views"));
    tabs->add(tr("Waiting (%1)").arg(waiting));
    tabs->add(tr("Decided"));
    tabs->add(tr("All"));
    tabs->setCurrent(m_view);
    connect(tabs, &Web::Tabs::changed, this, &ReviewPage::setView);
    m_body->addWidget(tabs);

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
    if (m_total > m_items.size())
        m_body->addWidget(Web::text(tr("Showing the first %1 of %2.").arg(m_items.size()).arg(m_total), "cellSub", host));
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
    head->addWidget(Web::text(tr("by %1, %2").arg(str(it, "author"), day(str(it, "changedAt"))), "cellSub", panel));
    head->addStretch();
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

    if (open && state == QLatin1String("waiting")) {
        // .server-actions
        const QJsonObject only{{QLatin1String("ids"), QJsonArray{id}}};
        const QJsonObject merged = it.value(QLatin1String("merged")).toObject();
        QPushButton *approve = nullptr;
        if (!conflicts.isEmpty() && !merged.isEmpty()) {
            approve = Web::button(tr("Approve the merged entry"), Web::Primary | Web::Small, panel, QStringLiteral("check"));
            connect(approve, &QPushButton::clicked, this, [this, only, merged] { decide(only, QStringLiteral("approved-with-edits"), merged, tr("Approved the merged entry")); });
        } else {
            approve = Web::button(tr("Approve"), Web::Primary | Web::Small, panel, QStringLiteral("check"));
            connect(approve, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("approved"), {}, QString()); });
        }
        approve->setObjectName(QStringLiteral("approve"));
        auto *postpone = Web::button(tr("Postpone"), Web::Small, panel, QStringLiteral("clock"));
        postpone->setObjectName(QStringLiteral("postpone"));
        auto *reject = Web::button(tr("Reject"), Web::Small, panel, QStringLiteral("x"));
        reject->setObjectName(QStringLiteral("reject"));
        connect(postpone, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("postponed"), {}, QString()); });
        connect(reject, &QPushButton::clicked, this, [this, only] { decide(only, QStringLiteral("rejected"), {}, QString()); });
        v->addWidget(Web::row({approve, postpone, reject}, panel));
    }
    return panel;
}
