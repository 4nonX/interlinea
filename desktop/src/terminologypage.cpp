#include "terminologypage.h"

#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "reviewpage.h"
#include "newtermbasedialog.h"
#include "termbasedialogs.h"
#include "termfields.h"
#include "roles.h"
#include "termdata.h"
#include "theme.h"
#include "words.h"

#include <QAbstractListModel>
#include <QCalendarWidget>
#include <QDateEdit>
#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QDateTime>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QCryptographicHash>
#include <QLabel>
#include <QLineEdit>
#include <QListView>
#include <QLocale>
#include <QPainter>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QRadioButton>
#include <QRegularExpression>
#include <QResizeEvent>
#include <QScrollArea>
#include <QScrollBar>
#include <QStackedWidget>
#include <QStyledItemDelegate>
#include <QUrl>
#include <QUrlQuery>
#include <QVBoxLayout>

namespace {

constexpr int S1 = 4, S2 = 8, S3 = 12, S4 = 16, S5 = 24;

QString str(const QJsonObject &o, const char *key)
{
    return o.value(QLatin1String(key)).toString();
}

QDateTime parseTime(const QString &iso)
{
    // The core writes times to the nanosecond.
    QString text = iso;
    text.replace(QRegularExpression(QStringLiteral("(\\.\\d{3})\\d+")), QStringLiteral("\\1"));
    QDateTime t = QDateTime::fromString(text, Qt::ISODateWithMs);
    if (!t.isValid())
        t = QDateTime::fromString(text, Qt::ISODate);
    return t.isValid() && t.date().year() > 1 ? t.toLocalTime() : QDateTime();
}

QString when(const QString &iso)
{
    const QDateTime t = parseTime(iso);
    return t.isValid() ? Languages::when(t) : QString();
}

QString day(const QString &iso)
{
    const QDateTime t = parseTime(iso);
    return t.isValid() ? QLocale().toString(t.date(), QStringLiteral("d MMM yyyy")) : QString();
}

QFrame *line(QWidget *parent)
{
    auto *f = new QFrame(parent);
    Theme::role(f, "line");
    f->setFixedHeight(1);
    return f;
}

QPushButton *link(const QString &text, QWidget *parent, const QString &icon = QString())
{
    auto *b = new QPushButton(text, parent);
    b->setProperty("link", true);
    b->setCursor(Qt::PointingHandCursor);
    if (!icon.isEmpty()) {
        b->setIcon(Icons::get(icon, Theme::color("accentText")));
        b->setIconSize(QSize(14, 14));
    }
    return b;
}

QComboBox *statusCombo(const QString &current, QWidget *parent, bool small)
{
    auto *c = new QComboBox(parent);
    if (small)
        c->setProperty("small", true);
    c->addItem(TermData::statusLabel(QStringLiteral("approved")), QStringLiteral("approved"));
    c->addItem(TermData::statusLabel(QStringLiteral("proposed")), QStringLiteral("proposed"));
    c->addItem(TermData::statusLabel(QStringLiteral("forbidden")), QStringLiteral("forbidden"));
    c->setCurrentIndex(qMax(0, c->findData(current.isEmpty() ? QStringLiteral("proposed") : current)));
    c->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    return c;
}

// A text field of several lines that says when it is left.
class BlurEdit : public QPlainTextEdit {
public:
    using QPlainTextEdit::QPlainTextEdit;
    std::function<void()> left;

protected:
    void focusOutEvent(QFocusEvent *e) override
    {
        QPlainTextEdit::focusOutEvent(e);
        if (left)
            left();
    }
};

// A pane's scrolling middle.
QScrollArea *scroller(QWidget *parent, QVBoxLayout **body, int margin, int spacing)
{
    auto *s = new QScrollArea(parent);
    Theme::role(s, "paneScroll");
    s->setWidgetResizable(true);
    s->setFrameShape(QFrame::NoFrame);
    s->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    auto *w = new QWidget(s);
    Theme::role(w, "paneBody");
    *body = new QVBoxLayout(w);
    (*body)->setContentsMargins(margin, margin, margin, margin);
    (*body)->setSpacing(spacing);
    s->setWidget(w);
    Web::fitScroll(s);
    return s;
}

} // namespace

// The entries the list shows.
class EntryListModel : public QAbstractListModel {
public:
    using QAbstractListModel::QAbstractListModel;
    enum { EntryRole = Qt::UserRole + 1 };

    void set(const QList<QJsonObject> &entries, const QStringList &languages)
    {
        beginResetModel();
        m_entries = entries;
        m_languages = languages;
        endResetModel();
    }
    QStringList languages() const { return m_languages; }
    int rowOf(const QString &id) const
    {
        for (int i = 0; i < m_entries.size(); i++) {
            if (m_entries[i].value(QLatin1String("id")).toString() == id)
                return i;
        }
        return -1;
    }
    QJsonObject at(int row) const { return m_entries.value(row); }
    int rowCount(const QModelIndex &parent = {}) const override { return parent.isValid() ? 0 : int(m_entries.size()); }
    QVariant data(const QModelIndex &index, int role) const override
    {
        if (!index.isValid() || index.row() >= m_entries.size())
            return {};
        const QJsonObject &e = m_entries[index.row()];
        if (role == EntryRole)
            return e;
        if (role == Qt::DisplayRole || role == Qt::AccessibleTextRole) {
            // What a screen reader says: the terms of each language.
            QStringList parts;
            for (const QString &l : m_languages) {
                QStringList ts;
                for (const QJsonValue &t : TermData::terms(e, l))
                    ts << t.toObject().value(QLatin1String("text")).toString();
                parts << (ts.isEmpty() ? TerminologyPage::tr("%1: missing").arg(Languages::name(l)) : QStringLiteral("%1: %2").arg(Languages::name(l), ts.join(QStringLiteral(", "))));
            }
            if (parts.isEmpty())
                parts << TermData::head(e, QString());
            return parts.join(QStringLiteral("; "));
        }
        return {};
    }

private:
    QList<QJsonObject> m_entries;
    QStringList m_languages;
};

namespace {

// Draws an entry as .entry-row: its first term, then the other languages
// with their chips, then its domain.
class EntryDelegate : public QStyledItemDelegate {
public:
    explicit EntryDelegate(EntryListModel *model, QObject *parent)
        : QStyledItemDelegate(parent)
        , m_model(model)
    {
    }

    static QString domain(const QJsonObject &e)
    {
        for (const QJsonValue &f : e.value(QLatin1String("fields")).toArray()) {
            if (f.toObject().value(QLatin1String("field")).toString().compare(QLatin1String("domain"), Qt::CaseInsensitive) == 0)
                return f.toObject().value(QLatin1String("value")).toString();
        }
        return {};
    }

    QSize sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const override
    {
        const QJsonObject e = index.data(EntryListModel::EntryRole).toJsonObject();
        const QStringList langs = m_model->languages();
        int h = 20 + 20;
        h += int(qMax(0, langs.size() - 1)) * 22;
        if (!domain(e).isEmpty())
            h += 19;
        return {option.rect.width(), h + 1};
    }

    void paint(QPainter *p, const QStyleOptionViewItem &option, const QModelIndex &index) const override
    {
        const QJsonObject e = index.data(EntryListModel::EntryRole).toJsonObject();
        const QStringList langs = m_model->languages();
        const QString src = langs.value(0);
        p->save();
        p->setRenderHint(QPainter::Antialiasing);
        const QRect r = option.rect;
        if (option.state & QStyle::State_Selected)
            p->fillRect(r, Theme::color("accentWeak"));
        else if (option.state & QStyle::State_MouseOver)
            p->fillRect(r, Theme::color("surface2"));
        p->setPen(Theme::color("border"));
        p->drawLine(r.left(), r.bottom(), r.right(), r.bottom());
        if (option.state & QStyle::State_HasFocus) {
            p->setPen(QPen(Theme::color("accent"), 2));
            p->drawRect(r.adjusted(1, 1, -2, -2));
        }

        const int x = r.left() + S4, w = r.width() - 2 * S4;
        int y = r.top() + 10;
        QFont bold = option.font;
        bold.setWeight(QFont::DemiBold);
        QFont small = option.font;
        small.setPixelSize(13);
        QFont italic = small;
        italic.setItalic(true);
        QFont chip = option.font;
        chip.setPixelSize(11);
        chip.setWeight(QFont::DemiBold);

        const QJsonArray own = TermData::terms(e, src);
        QString inLang;
        const QString head = own.isEmpty() ? TermData::head(e, src, &inLang) : own.first().toObject().value(QLatin1String("text")).toString();
        if (head.isEmpty()) {
            p->setFont(italic);
            p->setPen(Theme::color("text3"));
            p->drawText(QRect(x, y, w, 20), Qt::AlignVCenter | Qt::AlignLeft, TerminologyPage::tr("No term"));
        } else {
            p->setFont(bold);
            p->setPen(Theme::color("text"));
            const QFontMetrics fm(bold);
            const QString more = own.size() > 1 ? QStringLiteral(" +%1").arg(own.size() - 1) : QString();
            const int moreWidth = more.isEmpty() ? 0 : QFontMetrics(option.font).horizontalAdvance(more);
            const QString shownHead = fm.elidedText(head, Qt::ElideRight, w - moreWidth);
            p->drawText(QRect(x, y, w, 20), Qt::AlignVCenter | Qt::AlignLeft, shownHead);
            if (!more.isEmpty()) {
                p->setFont(option.font);
                p->setPen(Theme::color("text3"));
                p->drawText(QRect(x + fm.horizontalAdvance(shownHead), y, moreWidth + 2, 20), Qt::AlignVCenter | Qt::AlignLeft, more);
            }
        }
        y += 20 + 3;
        for (int i = 1; i < langs.size(); i++) {
            const QString l = langs[i];
            const QFontMetrics cm(chip);
            const int cw = cm.horizontalAdvance(l) + 12;
            const QRectF box(x + 0.5, y + 1.5, cw, 17);
            p->setPen(Theme::color("border"));
            p->setBrush(Qt::NoBrush);
            p->drawRoundedRect(box, 4, 4);
            p->setFont(chip);
            p->setPen(Theme::color("text2"));
            p->drawText(box, Qt::AlignCenter, l);
            const QJsonArray ts = TermData::terms(e, l);
            const QRect textRect(x + cw + 6, y, w - cw - 6, 20);
            if (ts.isEmpty()) {
                p->setFont(italic);
                p->setPen(Theme::color("text3"));
                p->drawText(textRect, Qt::AlignVCenter | Qt::AlignLeft, TerminologyPage::tr("missing"));
            } else {
                QStringList texts;
                for (const QJsonValue &t : ts) {
                    if (t.toObject().value(QLatin1String("status")).toString() != QLatin1String("forbidden"))
                        texts << t.toObject().value(QLatin1String("text")).toString();
                }
                if (texts.isEmpty())
                    texts << ts.first().toObject().value(QLatin1String("text")).toString();
                p->setFont(small);
                p->setPen(Theme::color("text2"));
                p->drawText(textRect, Qt::AlignVCenter | Qt::AlignLeft, QFontMetrics(small).elidedText(texts.join(QStringLiteral(", ")), Qt::ElideRight, textRect.width()));
            }
            y += 22;
        }
        const QString d = domain(e);
        if (!d.isEmpty()) {
            QFont f = option.font;
            f.setPixelSize(12);
            p->setFont(f);
            p->setPen(Theme::color("text3"));
            p->drawText(QRect(x, y + 1, w, 16), Qt::AlignVCenter | Qt::AlignLeft, QFontMetrics(f).elidedText(d, Qt::ElideRight, w));
        }
        p->restore();
    }

private:
    EntryListModel *m_model;
};

} // namespace

TerminologyPage::TerminologyPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
{
    setObjectName(QStringLiteral("terminology"));
    auto *outer = new QVBoxLayout(this);
    // .page: 24 px from the top, 32 from the sides; .page-wide has no
    // greatest width.
    outer->setContentsMargins(32, S5, 32, S5);
    outer->setSpacing(S5);
    outer->addWidget(Web::header(tr("Terminology"), QString(), {}, this));

    // .workspace: 220 px, 280 to 360 px, and the rest, 12 px apart.
    auto *work = new QHBoxLayout;
    work->setSpacing(S3);
    outer->addLayout(work, 1);

    auto *left = new QFrame(this);
    Theme::role(left, "pane");
    left->setFixedWidth(220);
    left->setAccessibleName(tr("Termbases"));
    auto *leftLayout = new QVBoxLayout(left);
    leftLayout->setContentsMargins(1, 1, 1, 1);
    leftLayout->setSpacing(0);
    QVBoxLayout *listOuter = nullptr;
    QScrollArea *leftScroll = scroller(left, &listOuter, 0, 0);
    listOuter->setContentsMargins(S2, S3, S2, S3);
    listOuter->setSpacing(S4);
    m_listBody = new QVBoxLayout;
    m_listBody->setSpacing(S4);
    listOuter->addLayout(m_listBody);
    m_createHolder = new QVBoxLayout;
    m_createHolder->setSpacing(0);
    listOuter->addLayout(m_createHolder);
    listOuter->addStretch();
    leftLayout->addWidget(leftScroll);
    work->addWidget(left);

    auto *middle = new QFrame(this);
    Theme::role(middle, "pane");
    middle->setObjectName(QStringLiteral("entriesPane"));
    middle->setFixedWidth(360);
    m_middle = new QVBoxLayout(middle);
    m_middle->setContentsMargins(1, 1, 1, 1);
    m_middle->setSpacing(0);
    auto *head = new QFrame(middle);
    Theme::role(head, "paneHead");
    auto *headLayout = new QVBoxLayout(head);
    headLayout->setContentsMargins(S4, S3, S4, S3);
    headLayout->setSpacing(S1);
    m_headHolder = new QVBoxLayout;
    m_headHolder->setSpacing(S1);
    headLayout->addLayout(m_headHolder);
    m_noteHolder = new QVBoxLayout;
    m_noteHolder->setSpacing(S2);
    headLayout->addLayout(m_noteHolder);
    m_middle->addWidget(head);

    m_tabs = new Web::Tabs(tr("Entry lists"), middle, true);
    m_tabs->setObjectName(QStringLiteral("entryViews"));
    m_tabs->add(tr("Entries"));
    m_tabs->add(tr("Duplicates"), QStringLiteral("copy"));
    m_tabs->add(tr("Deleted"));
    m_tabs->add(tr("Master"));
    m_middle->addWidget(m_tabs);
    connect(m_tabs, &Web::Tabs::changed, this, &TerminologyPage::setView);

    m_views = new QStackedWidget(middle);
    m_middle->addWidget(m_views, 1);

    // The entries: search, new entry, the list.
    auto *entriesView = new QWidget(m_views);
    auto *ev = new QVBoxLayout(entriesView);
    ev->setContentsMargins(0, 0, 0, 0);
    ev->setSpacing(0);
    auto *filters = new QFrame(entriesView);
    Theme::role(filters, "paneBar");
    auto *fl = new QHBoxLayout(filters);
    fl->setContentsMargins(S3, S2, S3, S2);
    fl->setSpacing(S2);
    m_search = new QLineEdit(filters);
    m_search->setPlaceholderText(tr("Search terms and fields"));
    m_search->setAccessibleName(tr("Search terms and fields"));
    m_search->setClearButtonEnabled(true);
    m_search->addAction(Icons::get(QStringLiteral("search"), Theme::color("text3")), QLineEdit::LeadingPosition);
    fl->addWidget(m_search, 1);
    m_statusBox = new QComboBox(filters);
    m_statusBox->setAccessibleName(tr("Status"));
    m_statusBox->addItem(tr("All statuses"), QString());
    m_statusBox->addItem(TermData::statusLabel(QStringLiteral("approved")), QStringLiteral("approved"));
    m_statusBox->addItem(TermData::statusLabel(QStringLiteral("proposed")), QStringLiteral("proposed"));
    m_statusBox->addItem(TermData::statusLabel(QStringLiteral("forbidden")), QStringLiteral("forbidden"));
    fl->addWidget(m_statusBox);
    ev->addWidget(filters);
    connect(m_search, &QLineEdit::textChanged, this, &TerminologyPage::setQuery);
    connect(m_statusBox, &QComboBox::currentIndexChanged, this, [this] { setStatus(m_statusBox->currentData().toString()); });

    m_newEntryHolder = new QVBoxLayout;
    m_newEntryHolder->setSpacing(0);
    ev->addLayout(m_newEntryHolder);

    m_listStack = new QStackedWidget(entriesView);
    m_model = new EntryListModel(this);
    m_list = new QListView(m_listStack);
    m_list->setObjectName(QStringLiteral("entryList"));
    m_list->setModel(m_model);
    m_list->setItemDelegate(new EntryDelegate(m_model, m_list));
    m_list->setMouseTracking(true);
    m_list->setUniformItemSizes(false);
    m_list->setVerticalScrollMode(QAbstractItemView::ScrollPerPixel);
    m_list->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    m_list->setSelectionMode(QAbstractItemView::SingleSelection);
    m_list->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_listStack->addWidget(m_list);
    auto *empty = new QWidget(m_listStack);
    m_emptyHolder = new QVBoxLayout(empty);
    m_emptyHolder->setContentsMargins(S5, S5, S5, S5);
    m_emptyHolder->setSpacing(S2);
    m_listStack->addWidget(empty);
    ev->addWidget(m_listStack, 1);
    m_views->addWidget(entriesView);
    connect(m_list->selectionModel(), &QItemSelectionModel::currentChanged, this, [this](const QModelIndex &now) {
        const QString id = m_model->at(now.row()).value(QLatin1String("id")).toString();
        if (now.isValid() && id != m_selected) {
            m_selected = id;
            m_detailTab = 0;
            showDetail();
        }
    });

    // The duplicates.
    QVBoxLayout *dupOuter = nullptr;
    QScrollArea *dups = scroller(m_views, &dupOuter, 0, 0);
    m_dupBody = new QVBoxLayout;
    m_dupBody->setSpacing(0);
    dupOuter->addLayout(m_dupBody);
    dupOuter->addStretch();
    m_views->addWidget(dups);
    // The deleted entries, and the master's entries under a project's.
    for (QVBoxLayout **body : {&m_deletedBody, &m_masterBody}) {
        QVBoxLayout *outerBody = nullptr;
        QScrollArea *view = scroller(m_views, &outerBody, 0, 0);
        *body = new QVBoxLayout;
        (*body)->setSpacing(0);
        outerBody->addLayout(*body);
        outerBody->addStretch();
        m_views->addWidget(view);
    }
    work->addWidget(middle);

    auto *right = new QFrame(this);
    Theme::role(right, "pane");
    right->setAccessibleName(tr("Entry"));
    m_detailHolder = new QVBoxLayout(right);
    m_detailHolder->setContentsMargins(1, 1, 1, 1);
    m_detailHolder->setSpacing(0);
    work->addWidget(right, 1);

    m_toast = new Web::Toast(this);

    // Changes that follow one another closely are read once.
    m_reload.setSingleShot(true);
    m_reload.setInterval(200);
    connect(&m_reload, &QTimer::timeout, this, [this] {
        if (busy())
            m_reload.start();
        else
            reload();
    });
    connect(manager, &ConnectionManager::added, this, [this] { m_reload.start(); });
    connect(manager, &ConnectionManager::changed, this, [this] { m_reload.start(); });
    connect(manager, &ConnectionManager::removed, this, [this](const QString &id) {
        m_listening.remove(id);
        m_lists.remove(id);
        if (m_conn == id) {
            m_conn.clear();
            m_name.clear();
        }
        m_reload.start();
    });
    showCreateForm();
    showNewEntry(false);
    reload();
}

void TerminologyPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    // minmax(280px, 360px): the entries give way to the entry when the
    // window is narrow.
    if (auto *middle = findChild<QFrame *>(QStringLiteral("entriesPane"), Qt::FindDirectChildrenOnly))
        middle->setFixedWidth(qBound(280, width() - 64 - 220 - 24 - 420, 360));
    m_toast->place();
}

void TerminologyPage::notify(const QString &text)
{
    m_toast->say(text);
}

QString TerminologyPage::message() const
{
    return m_toast->last();
}

void TerminologyPage::clear(QLayout *layout)
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

Connection *TerminologyPage::conn() const
{
    return m_manager->find(m_conn);
}

QString TerminologyPage::path(const QString &rest) const
{
    return QStringLiteral("/api/termbases/") + ApiClient::seg(m_name) + rest;
}

bool TerminologyPage::isTeam() const
{
    const Connection *c = conn();
    return c && !c->isLocal();
}

bool TerminologyPage::may(const char *permission) const
{
    // On this computer everything is one's own.
    const QString role = str(m_detail, "role");
    return role.isEmpty() ? !isTeam() : Roles::allows(role, QLatin1String(permission));
}

bool TerminologyPage::isProject() const
{
    return str(m_detail, "kind") == QLatin1String("project");
}

QStringList TerminologyPage::languages() const
{
    QStringList out;
    for (const QJsonValue &l : m_detail.value(QLatin1String("definition")).toObject().value(QLatin1String("languages")).toArray())
        out << l.toString();
    return out;
}

QJsonArray TerminologyPage::fields() const
{
    return m_detail.value(QLatin1String("definition")).toObject().value(QLatin1String("fields")).toArray();
}

QJsonObject TerminologyPage::entry(const QString &id) const
{
    for (const QJsonObject &e : m_entries) {
        if (str(e, "id") == id)
            return e;
    }
    return {};
}

QList<QJsonObject> TerminologyPage::shown() const
{
    const QString q = m_query.trimmed().toLower();
    QList<QJsonObject> out;
    for (const QJsonObject &e : m_entries) {
        if (!m_status.isEmpty() && !TermData::hasStatus(e, m_status))
            continue;
        if (!q.isEmpty() && !TermData::searchText(e).contains(q))
            continue;
        out << e;
    }
    return out;
}

void TerminologyPage::listen()
{
    for (Connection *c : m_manager->all()) {
        if (m_listening.contains(c->id()))
            continue;
        m_listening << c->id();
        connect(c->feed(), &EventFeed::event, this, [this](const QString &type) {
            if (type == QLatin1String("termbase") || type == QLatin1String("account"))
                m_reload.start();
        });
        connect(c->feed(), &EventFeed::reset, this, [this] { m_reload.start(); });
    }
}

void TerminologyPage::reload()
{
    listen();
    loadLists();
}

void TerminologyPage::loadLists()
{
    const int reading = ++m_reading;
    QList<Connection *> ready;
    for (Connection *c : m_manager->all()) {
        if (c->isReady())
            ready << c;
        else
            m_lists.remove(c->id());
    }
    m_pendingLists = int(ready.size());
    if (ready.isEmpty()) {
        showLists();
        m_detail = {};
        m_entries.clear();
        m_opened = false;
        showTermbase();
        emit loaded();
        return;
    }
    for (Connection *c : ready) {
        const QString id = c->id();
        onResult(c->api()->get(QStringLiteral("/api/termbases")), this, [this, id, reading](const ApiResult &r) {
            if (reading != m_reading)
                return;
            if (r.ok())
                m_lists.insert(id, r.array());
            else
                m_lists.remove(id);
            if (--m_pendingLists > 0)
                return;
            pickDefault();
            showLists();
            // What can be made depends on the masters there are.
            showCreateForm();
            loadTermbase();
        });
    }
}

void TerminologyPage::pickDefault()
{
    // The termbase shown is still there, or the first master of this
    // computer is shown, else the first termbase of any place.
    const auto has = [this](const QString &connId, const QString &name) {
        for (const QJsonValue &v : m_lists.value(connId)) {
            if (str(v.toObject(), "name") == name)
                return true;
        }
        return false;
    };
    if (!m_name.isEmpty() && has(m_conn, m_name))
        return;
    m_conn.clear();
    m_name.clear();
    QList<Connection *> order;
    if (m_manager->local())
        order << m_manager->local();
    order << m_manager->teams();
    for (int pass = 0; pass < 2 && m_name.isEmpty(); pass++) {
        for (Connection *c : order) {
            for (const QJsonValue &v : m_lists.value(c->id())) {
                if (pass == 0 && str(v.toObject(), "kind") != QLatin1String("master"))
                    continue;
                m_conn = c->id();
                m_name = str(v.toObject(), "name");
                break;
            }
            if (!m_name.isEmpty())
                break;
        }
    }
}

void TerminologyPage::open(const QString &connectionId, const QString &termbase, const QString &entryId)
{
    const bool same = m_conn == connectionId && m_name == termbase;
    m_conn = connectionId;
    m_name = termbase;
    m_wantEntry = entryId;
    if (!same) {
        m_selected.clear();
        m_view = 0;
        m_tabs->setCurrent(0);
        m_views->setCurrentIndex(0);
        m_dismissed.clear();
        m_dupLang.clear();
        showNewEntry(false);
    }
    showLists();
    ++m_reading;
    loadTermbase();
}

void TerminologyPage::showLists()
{
    // The opened termbase is counted as it is here, not as it was listed.
    if (m_opened && str(m_detail, "name") == m_name) {
        QJsonArray list = m_lists.value(m_conn);
        for (int i = 0; i < list.size(); i++) {
            QJsonObject tb = list[i].toObject();
            if (str(tb, "name") == m_name) {
                tb.insert(QLatin1String("entries"), qint64(m_entries.size()));
                list[i] = tb;
            }
        }
        m_lists.insert(m_conn, list);
    }
    // Drawn again only when it would look different.
    {
        QJsonObject all{{QLatin1String("place"), m_conn}, {QLatin1String("name"), m_name}, {QLatin1String("words"), Words::git()}};
        for (Connection *c : m_manager->all()) {
            if (c->isReady())
                all.insert(QStringLiteral("list:") + c->id(), m_lists.value(c->id()));
        }
        const QByteArray now = QCryptographicHash::hash(QJsonDocument(all).toJson(QJsonDocument::Compact), QCryptographicHash::Sha1);
        if (now == m_listsDrawn && m_listBody->count() > 0)
            return;
        m_listsDrawn = now;
    }
    clear(m_listBody);
    QWidget *host = m_listBody->parentWidget();
    const auto group = [&](const QString &title, Connection *c, const QList<QJsonObject> &list, const QString &emptyNote) {
        auto *g = new QWidget(host);
        g->setAccessibleName(title);
        auto *v = new QVBoxLayout(g);
        v->setContentsMargins(0, 0, 0, 0);
        v->setSpacing(2);
        auto *t = Web::text(title, "groupTitle", g);
        t->setContentsMargins(S2, 0, S2, S1);
        v->addWidget(t);
        if (list.isEmpty() && !emptyNote.isEmpty()) {
            auto *n = Web::text(emptyNote, "meta", g);
            n->setWordWrap(true);
            n->setContentsMargins(S3, 0, S3, 0);
            v->addWidget(n);
        }
        for (const QJsonObject &tb : list) {
            const QString name = str(tb, "name");
            auto *row = new Web::Clickable("tbLink", g);
            row->setObjectName(QStringLiteral("tb:%1:%2").arg(c->id(), name));
            row->setCurrent(c->id() == m_conn && name == m_name);
            const qint64 n = tb.value(QLatin1String("entries")).toInteger();
            row->setAccessibleName(tr("%1, %2, %3").arg(name, c->isLocal() ? tr("on this computer") : tr("on %1").arg(c->displayName()), TermData::entries(n)));
            auto *rv = new QVBoxLayout(row);
            rv->setContentsMargins(S2, 7, S2, 7);
            rv->setSpacing(3);
            // A project's termbase is named after it; the list says which
            // it is by its group.
            QString shownName = name;
            shownName.remove(QRegularExpression(QStringLiteral(" \\(project\\)$")));
            auto *nl = Web::text(shownName, "tbName", row);
            nl->setWordWrap(true);
            rv->addWidget(nl);
            auto *meta = new QHBoxLayout;
            meta->setSpacing(S2);
            meta->addWidget(Web::locationBadge(!c->isLocal(), row, c->displayName()));
            meta->addStretch();
            meta->addWidget(Web::text(QLocale().toString(n), "tbCount", row));
            rv->addLayout(meta);
            const QString connId = c->id();
            connect(row, &Web::Clickable::clicked, this, [this, connId, name] { open(connId, name); });
            v->addWidget(row);
        }
        m_listBody->addWidget(g);
    };
    const auto of = [this](Connection *c, const QString &kind) {
        QList<QJsonObject> out;
        for (const QJsonValue &v : m_lists.value(c->id())) {
            if (kind.isEmpty() || str(v.toObject(), "kind") == kind)
                out << v.toObject();
        }
        return out;
    };
    if (Connection *local = m_manager->local(); local && local->isReady()) {
        group(tr("Masters"), local, of(local, QStringLiteral("master")), QString());
        group(Words::get("projectTermbases"), local, of(local, QStringLiteral("project")), QString());
    }
    for (Connection *c : m_manager->teams()) {
        if (!c->isReady())
            continue;
        const QString title = m_manager->teams().size() > 1 ? tr("On %1").arg(c->displayName()) : tr("On the team server");
        group(title, c, of(c, QString()), tr("No termbases on %1 for you yet.").arg(c->displayName()));
    }
}

void TerminologyPage::showCreateForm()
{
    clear(m_createHolder);
    QWidget *host = m_createHolder->parentWidget();
    // .tb-actions
    auto *w = new QWidget(host);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(0);
    auto *create = Web::button(tr("New master"), Web::Quiet | Web::Small, w, QStringLiteral("plus"));
    create->setObjectName(QStringLiteral("newMaster"));
    auto *project = Web::button(tr("New project termbase"), Web::Quiet | Web::Small, w, QStringLiteral("git-branch"));
    project->setObjectName(QStringLiteral("newProjectTermbase"));
    auto *import = Web::button(tr("Import termbase"), Web::Quiet | Web::Small, w, QStringLiteral("upload"));
    v->addWidget(create, 0, Qt::AlignLeft);
    // A project termbase needs a master to be based on.
    bool anyMaster = false;
    for (auto it = m_lists.constBegin(); it != m_lists.constEnd(); ++it) {
        for (const QJsonValue &tb : it.value())
            anyMaster = anyMaster || str(tb.toObject(), "kind") == QLatin1String("master");
    }
    if (anyMaster)
        v->addWidget(project, 0, Qt::AlignLeft);
    else
        project->hide();
    v->addWidget(import, 0, Qt::AlignLeft);
    connect(create, &QPushButton::clicked, this, [this] { newTermbaseDialog(false); });
    connect(project, &QPushButton::clicked, this, [this] { newTermbaseDialog(true); });
    connect(import, &QPushButton::clicked, this, [this] { importDialog(true); });
    m_createHolder->addWidget(w);
}

NewTermbaseDialog *TerminologyPage::newTermbaseDialog(bool project)
{
    auto *d = new NewTermbaseDialog(m_manager, m_lists, project ? NewTermbaseDialog::Project : NewTermbaseDialog::Master, window());
    d->setAttribute(Qt::WA_DeleteOnClose);
    connect(d, &NewTermbaseDialog::created, this, [this](const QString &connId, const QString &name, const QString &message) {
        notify(message);
        m_conn = connId;
        m_name = name;
        m_selected.clear();
        reload();
    });
    d->open();
    return d;
}

void TerminologyPage::loadTermbase()
{
    const int reading = m_reading;
    Connection *c = conn();
    m_problem.clear();
    m_loading = true;
    if (!c || m_name.isEmpty()) {
        m_loading = false;
        m_detail = {};
        m_entries.clear();
        m_bases.clear();
        m_requests = {};
        m_opened = false;
        showTermbase();
        emit loaded();
        return;
    }
    onResult(c->api()->get(path()), this, [this, reading](const ApiResult &r) {
        if (reading != m_reading)
            return;
        if (!r.ok()) {
            m_loading = false;
            m_detail = {};
            m_entries.clear();
            m_opened = false;
            m_problem = r.error.message;
            showTermbase();
            emit loaded();
            return;
        }
        m_detail = r.object();
        // A layered termbase without languages of its own has its masters'.
        QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
        if (def.value(QLatin1String("languages")).toArray().isEmpty()) {
            QJsonArray langs;
            for (const QJsonValue &u : m_detail.value(QLatin1String("upstreams")).toArray()) {
                for (const QJsonValue &tb : m_lists.value(m_conn)) {
                    if (str(tb.toObject(), "name") != u.toString())
                        continue;
                    for (const QJsonValue &l : tb.toObject().value(QLatin1String("languages")).toArray()) {
                        if (!langs.contains(l))
                            langs.append(l);
                    }
                }
            }
            def.insert(QLatin1String("languages"), langs);
            m_detail.insert(QLatin1String("definition"), def);
        }
        m_pendingEntries.clear();
        loadEntries(0, reading);
    });
}

void TerminologyPage::loadEntries(int offset, int reading)
{
    Connection *c = conn();
    if (!c)
        return;
    QUrlQuery q;
    q.addQueryItem(QStringLiteral("limit"), QStringLiteral("1000"));
    q.addQueryItem(QStringLiteral("offset"), QString::number(offset));
    onResult(c->api()->get(path(QStringLiteral("/entries")), q), this, [this, offset, reading](const ApiResult &r) {
        if (reading != m_reading)
            return;
        const QJsonArray page = r.object().value(QLatin1String("entries")).toArray();
        for (const QJsonValue &e : page)
            m_pendingEntries << e.toObject();
        const int total = r.object().value(QLatin1String("total")).toInt();
        if (r.ok() && !page.isEmpty() && offset + page.size() < total) {
            loadEntries(offset + int(page.size()), reading);
            return;
        }
        if (!r.ok())
            m_problem = r.error.message;
        // What a project termbase's entries are based on, and the merge
        // requests of a termbase on a team server, come with it.
        const auto finish = [this, reading] {
            if (reading != m_reading)
                return;
            const bool was = m_opened;
            m_loading = false;
            m_entries = m_pendingEntries;
            m_pendingEntries.clear();
            m_opened = true;
            // Read again with nothing new, as after one's own change: the
            // page stays as it is, with what is being typed or asked.
            if (was && m_wantEntry.isEmpty() && signature() == m_drawn) {
                emit loaded();
                return;
            }
            showTermbase();
            emit loaded();
        };
        Connection *c = conn();
        if (!c) {
            finish();
            return;
        }
        m_bases.clear();
        m_masterEntries.clear();
        m_requests = {};
        auto *left = new int(1);
        const auto done = [left, finish] {
            if (--*left == 0) {
                delete left;
                finish();
            }
        };
        if (isProject()) {
            ++*left;
            onResult(c->api()->get(path(QStringLiteral("/layered"))), this, [this, reading, done](const ApiResult &lr) {
                if (reading == m_reading) {
                    m_masterEntries.clear();
                    for (const QJsonValue &v : lr.array()) {
                        const QJsonObject o = v.toObject();
                        const QJsonObject base = o.value(QLatin1String("base")).toObject();
                        if (str(o, "termbase") != m_name)
                            m_masterEntries << o;
                        else if (!base.isEmpty())
                            m_bases.insert(str(o, "id"), str(base, "termbase"));
                    }
                }
                done();
            });
        }
        {
            ++*left;
            onResult(c->api()->get(path(QStringLiteral("/merge-requests"))), this, [this, reading, done](const ApiResult &mr) {
                if (reading == m_reading)
                    m_requests = mr.array();
                done();
            });
        }
        done();
    });
}

QByteArray TerminologyPage::signature() const
{
    QJsonArray entries, masters;
    for (const QJsonObject &e : m_entries)
        entries.append(e);
    for (const QJsonObject &e : m_masterEntries)
        masters.append(e);
    QJsonObject bases;
    for (auto it = m_bases.constBegin(); it != m_bases.constEnd(); ++it)
        bases.insert(it.key(), it.value());
    const QJsonObject all{{QLatin1String("place"), m_conn}, {QLatin1String("name"), m_name},       {QLatin1String("opened"), m_opened},   {QLatin1String("problem"), m_problem},
                          {QLatin1String("detail"), m_detail}, {QLatin1String("entries"), entries}, {QLatin1String("masters"), masters}, {QLatin1String("bases"), bases},
                          {QLatin1String("requests"), m_requests}};
    return QCryptographicHash::hash(QJsonDocument(all).toJson(QJsonDocument::Compact), QCryptographicHash::Sha1);
}

void TerminologyPage::showTermbase()
{
    m_drawn = signature();
    showHead();
    showReviewNote();
    m_tabs->setEnabledAt(1, m_entries.size() >= 2);
    // Merging duplicates changes entries: for those who may edit.
    m_tabs->setVisibleAt(1, may("edit"));
    m_tabs->setVisibleAt(3, isProject());
    m_tabs->setText(3, tr("Master (%1)").arg(m_masterEntries.size()));
    m_tabs->tab(3)->setAccessibleName(tr("The master's entries (%1)").arg(m_masterEntries.size()));
    if ((m_view == 1 && (m_entries.size() < 2 || !may("edit"))) || (m_view == 3 && !isProject()))
        setView(0);
    m_tabs->setVisible(m_opened);
    m_views->setVisible(m_opened);
    // Adding entries is offered to those who may, once that is known.
    if (m_opened && may("edit") == m_newEntryHolder->isEmpty())
        showNewEntry(false);
    if (!m_wantEntry.isEmpty()) {
        if (!entry(m_wantEntry).isEmpty())
            m_selected = m_wantEntry;
        m_wantEntry.clear();
    }
    showEntries();
    if (m_view == 1)
        showDuplicates();
    else if (m_view == 2)
        showDeleted();
    else if (m_view == 3)
        showMaster();
    showDetail();
}

void TerminologyPage::showHead()
{
    clear(m_headHolder);
    QWidget *host = m_headHolder->parentWidget();
    if (!m_opened) {
        QString text = m_problem;
        if (text.isEmpty()) {
            bool any = false;
            for (Connection *c : m_manager->all())
                any = any || c->isReady();
            text = !any ? tr("Starting…") : m_name.isEmpty() ? tr("No termbase yet. Make a master on the left, or import a termbase file.") : tr("Opening %1…").arg(m_name);
        }
        auto *l = Web::text(text, m_problem.isEmpty() ? "help" : "error", host);
        l->setWordWrap(true);
        l->setObjectName(QStringLiteral("termbaseState"));
        m_headHolder->addWidget(l);
        return;
    }
    // .pane-head-row: the name and the tools, which go under it when
    // both do not fit.
    auto *rowWidget = new QWidget(host);
    auto *row = new FlowLayout(rowWidget, S2);
    row->setContentsMargins(0, 0, 0, 0);
    auto *title = Web::text(m_name, "paneTitle", rowWidget);
    title->setObjectName(QStringLiteral("termbaseTitle"));
    title->setTextInteractionFlags(Qt::TextSelectableByMouse);
    row->addWidget(title);
    auto *tools = new QWidget(rowWidget);
    tools->setAccessibleName(tr("%1 tools").arg(m_name));
    auto *tl = new QHBoxLayout(tools);
    tl->setContentsMargins(0, 0, 0, 0);
    tl->setSpacing(2);
    auto *import = Web::button(tr("Import"), Web::Quiet | Web::Small, tools, QStringLiteral("upload"));
    auto *exp = Web::button(tr("Export"), Web::Quiet | Web::Small, tools, QStringLiteral("download"));
    exp->setEnabled(!m_entries.isEmpty());
    // Importing changes the termbase: for those who may edit it.
    if (may("edit"))
        tl->addWidget(import);
    else
        import->hide();
    tl->addWidget(exp);
    connect(import, &QPushButton::clicked, this, [this] { importDialog(false); });
    connect(exp, &QPushButton::clicked, this, [this] { exportDialog(); });
    if (isTeam()) {
        auto *again = Web::button(tr("Reload"), Web::Quiet | Web::Small, tools, QStringLiteral("refresh"));
        tl->addWidget(again);
        connect(again, &QPushButton::clicked, this, [this] {
            const QString name = m_name;
            auto *once = new QObject(this);
            connect(this, &TerminologyPage::loaded, once, [this, once, name] {
                once->deleteLater();
                if (m_problem.isEmpty())
                    notify(tr("Loaded %1 again from the server").arg(name));
                else
                    notify(m_problem);
            });
            reload();
        });
    }
    // Its name, languages, fields, who may use it and how it is reviewed:
    // for those who manage it.
    if (may("manage")) {
        auto *settings = Web::button(tr("Settings"), Web::Quiet | Web::Small, tools, QStringLiteral("sliders"));
        settings->setObjectName(QStringLiteral("termbaseSettingsButton"));
        tl->addWidget(settings);
        connect(settings, &QPushButton::clicked, this, [this] { settingsDialog(); });
    }
    row->addWidget(tools);
    m_headHolder->addWidget(rowWidget);

    // .pane-sub
    auto *sub = new QWidget(host);
    auto *sl = new FlowLayout(sub, S2);
    sl->setContentsMargins(0, 0, 0, 0);
    Connection *c = conn();
    sl->addWidget(Web::locationBadge(isTeam(), sub, c ? c->displayName() : QString()));
    auto *count = Web::text(TermData::entries(m_entries.size()), "meta", sub);
    count->setObjectName(QStringLiteral("entryCount"));
    sl->addWidget(count);
    for (const QString &l : languages())
        sl->addWidget(Web::langChip(l, sub));
    // What one may do here, where it is not everything.
    if (isTeam() && !may("manage")) {
        auto *role = Web::chip(Roles::text(str(m_detail, "role")), QString(), sub);
        role->setObjectName(QStringLiteral("termbaseRole"));
        role->setToolTip(tr("What you may do in this termbase"));
        sl->addWidget(role);
    }
    m_headHolder->addWidget(sub);
}

void TerminologyPage::showReviewNote()
{
    clear(m_noteHolder);
    if (!m_opened)
        return;
    QWidget *host = m_noteHolder->parentWidget();
    QStringList masters;
    for (const QJsonValue &u : m_detail.value(QLatin1String("upstreams")).toArray())
        masters << u.toString();
    if (!isTeam() && isProject() && !masters.isEmpty()) {
        // .branch-note
        auto *note = new QWidget(host);
        auto *h = new QHBoxLayout(note);
        h->setContentsMargins(0, S1, 0, 0);
        h->setSpacing(6);
        auto *icon = new QLabel(note);
        icon->setPixmap(Icons::get(QStringLiteral("git-branch"), Theme::color("text2")).pixmap(14, 14));
        h->addWidget(icon, 0, Qt::AlignTop);
        auto *text = Web::text(tr("%1 %2. Shows only this project's new and changed entries.").arg(Words::get("basedOn"), masters.join(QStringLiteral(", "))), "small2", note);
        text->setWordWrap(true);
        text->setObjectName(QStringLiteral("branchNote"));
        h->addWidget(text, 1);
        m_noteHolder->addWidget(note);
    }
    // .server-review: the merge requests, and a new one for a project
    // termbase.
    for (const QJsonValue &v : m_requests) {
        const QJsonObject m = v.toObject();
        auto *item = new QWidget(host);
        auto *iv = new QVBoxLayout(item);
        iv->setContentsMargins(0, 0, 0, 0);
        iv->setSpacing(0);
        auto *open = link(str(m, "title"), item, QStringLiteral("git-pull-request"));
        const int id = m.value(QLatin1String("id")).toInt();
        open->setObjectName(QStringLiteral("mergeRequest:%1").arg(id));
        connect(open, &QPushButton::clicked, this, [this, id] { emit openReview(m_conn, id); });
        iv->addWidget(open, 0, Qt::AlignLeft);
        const bool isOpen = str(m, "state") == QLatin1String("open");
        const QString closed = str(m, "closedAt").isEmpty() ? str(m, "createdAt") : str(m, "closedAt");
        auto *meta = Web::text(tr("%1 into %2, %3").arg(str(m, "project"), str(m, "target"), isOpen ? tr("open since %1").arg(day(str(m, "createdAt"))) : tr("closed %1").arg(day(closed))),
                               "cellSub", item);
        meta->setWordWrap(true);
        iv->addWidget(meta);
        m_noteHolder->addWidget(item);
    }
    if (!isProject() || masters.isEmpty())
        return;
    // .role-add
    auto *form = new QWidget(host);
    form->setAccessibleName(tr("Submit for review"));
    auto *fv = new QVBoxLayout(form);
    fv->setContentsMargins(0, S1, 0, 0);
    fv->setSpacing(S2);
    QComboBox *into = nullptr;
    if (masters.size() > 1) {
        into = new QComboBox(form);
        into->setObjectName(QStringLiteral("reviewInto"));
        into->addItems(masters);
        fv->addWidget(Web::labelled(tr("Into"), into, form));
    }
    auto *title = new QLineEdit(form);
    title->setObjectName(QStringLiteral("reviewTitle"));
    title->setPlaceholderText(tr("%1 into %2").arg(m_name, masters.first()));
    fv->addWidget(Web::labelled(tr("Title of the request"), title, form));
    if (into)
        connect(into, &QComboBox::currentTextChanged, title, [this, title](const QString &m) { title->setPlaceholderText(tr("%1 into %2").arg(m_name, m)); });
    auto *submit = Web::button(!isTeam() ? Words::get("reviewMerge") : masters.size() > 1 ? tr("Submit to the master for review") : tr("Submit to %1 for review").arg(masters.first()),
                               Web::Small, form, QStringLiteral("git-pull-request"));
    submit->setObjectName(QStringLiteral("submitReview"));
    fv->addWidget(submit, 0, Qt::AlignLeft);
    const QString first = masters.first();
    const auto send = [this, into, title, first] {
        Connection *c = conn();
        if (!c)
            return;
        const QString master = into ? into->currentText() : first;
        const QString t = title->text().trimmed().isEmpty() ? tr("%1 into %2").arg(m_name, master) : title->text().trimmed();
        onResult(c->api()->post(path(QStringLiteral("/merge-requests")), QJsonObject{{QLatin1String("master"), master}, {QLatin1String("title"), t}}), this,
                 [this](const ApiResult &r) {
                     if (!r.ok()) {
                         notify(r.error.message);
                         return;
                     }
                     notify(tr("Submitted %1 to %2 for review").arg(m_name, str(r.object(), "target")));
                     const int id = r.object().value(QLatin1String("id")).toInt();
                     m_reload.start();
                     emit openReview(m_conn, id);
                 });
    };
    connect(submit, &QPushButton::clicked, this, send);
    connect(title, &QLineEdit::returnPressed, this, send);
    m_noteHolder->addWidget(form);
}

void TerminologyPage::setQuery(const QString &text)
{
    if (m_query == text)
        return;
    m_query = text;
    if (m_search->text() != text)
        m_search->setText(text);
    showEntries();
    showDetail();
}

void TerminologyPage::setStatus(const QString &status)
{
    if (m_status == status)
        return;
    m_status = status;
    const int i = m_statusBox->findData(status);
    if (i >= 0 && m_statusBox->currentIndex() != i)
        m_statusBox->setCurrentIndex(i);
    showEntries();
    showDetail();
}

void TerminologyPage::setView(int view)
{
    m_view = view;
    m_tabs->setCurrent(view);
    m_views->setCurrentIndex(view);
    // What was set aside as no duplicates is asked again next time.
    if (view != 1)
        m_dismissed.clear();
    if (view == 1)
        showDuplicates();
    else if (view == 2)
        showDeleted();
    else if (view == 3)
        showMaster();
    showDetail();
}

void TerminologyPage::selectEntry(const QString &id)
{
    if (entry(id).isEmpty())
        return;
    if (m_selected != id)
        m_detailTab = 0;
    m_selected = id;
    const int row = m_model->rowOf(id);
    if (row >= 0) {
        const QSignalBlocker block(m_list->selectionModel());
        m_list->setCurrentIndex(m_model->index(row));
        m_list->scrollTo(m_model->index(row));
    }
    showDetail();
}

void TerminologyPage::showEntries()
{
    const QList<QJsonObject> list = shown();
    // The entry shown stays the one chosen, even when the search hides
    // it; without one, the first of the list is shown.
    if (entry(m_selected).isEmpty())
        m_selected = list.isEmpty() ? QString() : str(list.first(), "id");
    const int scroll = m_list->verticalScrollBar()->value();
    {
        const QSignalBlocker block(m_list->selectionModel());
        m_model->set(list, languages());
        const int row = m_model->rowOf(m_selected);
        if (row >= 0)
            m_list->setCurrentIndex(m_model->index(row));
    }
    m_list->verticalScrollBar()->setValue(scroll);
    m_list->setAccessibleName(tr("Entries in %1").arg(m_name));
    clear(m_emptyHolder);
    if (!list.isEmpty() || !m_opened) {
        m_listStack->setCurrentIndex(0);
        return;
    }
    // .empty
    QWidget *host = m_emptyHolder->parentWidget();
    if (m_entries.isEmpty() && !may("edit")) {
        m_emptyHolder->addWidget(Web::text(tr("No entries yet"), "emptyTitle", host));
        auto *p = Web::text(isProject() && !m_masterEntries.isEmpty() ? tr("The project has no entries of its own. The master's are under Master.") : tr("Those who may edit this termbase add its entries."),
                            "secondary", host);
        p->setWordWrap(true);
        m_emptyHolder->addWidget(p);
    } else if (m_entries.isEmpty()) {
        m_emptyHolder->addWidget(Web::text(tr("No entries yet"), "emptyTitle", host));
        auto *p = Web::text(isProject() && !m_masterEntries.isEmpty()
                                ? tr("The project has no entries of its own yet: it uses the master's, which are under Master. Add an entry above, or change one of the master's for this project.")
                                : tr("Add an entry above, import a TBX, .sdltb or XLSX file, or add terms while you translate."),
                            "secondary", host);
        p->setWordWrap(true);
        m_emptyHolder->addWidget(p);
        auto *b = Web::button(tr("Import a file"), Web::Small, host, QStringLiteral("upload"));
        connect(b, &QPushButton::clicked, this, [this] { importDialog(false); });
        m_emptyHolder->addWidget(b, 0, Qt::AlignLeft);
    } else {
        auto *t = Web::text(m_query.trimmed().isEmpty() ? tr("Nothing has this status") : tr("Nothing matches “%1”").arg(m_query), "emptyTitle", host);
        t->setWordWrap(true);
        m_emptyHolder->addWidget(t);
        auto *b = Web::button(tr("Clear search"), Web::Small, host);
        connect(b, &QPushButton::clicked, this, [this] {
            setQuery(QString());
            setStatus(QString());
        });
        m_emptyHolder->addWidget(b, 0, Qt::AlignLeft);
    }
    m_emptyHolder->addStretch();
    m_listStack->setCurrentIndex(1);
}

void TerminologyPage::showNewEntry(bool open)
{
    clear(m_newEntryHolder);
    if (m_opened && !may("edit"))
        return;
    QWidget *host = m_newEntryHolder->parentWidget();
    if (!open) {
        // .new-entry-bar
        auto *bar = new QFrame(host);
        Theme::role(bar, "paneBar");
        auto *h = new QHBoxLayout(bar);
        h->setContentsMargins(S3, S1, S3, S1);
        auto *b = Web::button(tr("New entry"), Web::Quiet | Web::Small, bar, QStringLiteral("plus"));
        b->setObjectName(QStringLiteral("newEntry"));
        h->addWidget(b);
        h->addStretch();
        connect(b, &QPushButton::clicked, this, [this] { showNewEntry(true); });
        m_newEntryHolder->addWidget(bar);
        return;
    }
    // .new-entry
    auto *form = new QFrame(host);
    Theme::role(form, "paneForm");
    form->setAccessibleName(tr("New entry"));
    auto *v = new QVBoxLayout(form);
    v->setContentsMargins(S3, S3, S3, S3);
    v->setSpacing(S2);
    auto *label = new QLabel(tr("First term"), form);
    v->addWidget(label);
    auto *text = new QLineEdit(m_query.trimmed(), form);
    text->setObjectName(QStringLiteral("newEntryText"));
    text->setAccessibleName(tr("First term"));
    label->setBuddy(text);
    v->addWidget(text);
    auto *lang = new QComboBox(form);
    lang->setObjectName(QStringLiteral("newEntryLanguage"));
    lang->setAccessibleName(tr("Language"));
    QStringList langs = languages();
    if (langs.isEmpty())
        langs = Languages::common();
    for (const QString &l : langs)
        lang->addItem(Languages::name(l), l);
    auto *status = statusCombo(QStringLiteral("proposed"), form, false);
    status->setObjectName(QStringLiteral("newEntryStatus"));
    status->setAccessibleName(tr("Status"));
    auto *choices = new QHBoxLayout;
    choices->setSpacing(S2);
    choices->addWidget(lang, 1);
    choices->addWidget(status);
    v->addLayout(choices);
    auto *help = Web::text(tr("Add the other languages and fields in the entry."), "help", form);
    help->setWordWrap(true);
    v->addWidget(help);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, form);
    auto *add = Web::button(tr("Add entry"), Web::Primary | Web::Small, form);
    add->setObjectName(QStringLiteral("addEntry"));
    add->setEnabled(!text->text().trimmed().isEmpty());
    connect(text, &QLineEdit::textChanged, add, [add](const QString &t) { add->setEnabled(!t.trimmed().isEmpty()); });
    auto *buttons = new QHBoxLayout;
    buttons->setSpacing(S2);
    buttons->addStretch();
    buttons->addWidget(cancel);
    buttons->addWidget(add);
    v->addLayout(buttons);
    connect(cancel, &QPushButton::clicked, this, [this] { showNewEntry(false); });
    const auto submit = [this, text, lang, status] {
        const QString t = text->text().trimmed();
        Connection *c = conn();
        if (t.isEmpty() || !c)
            return;
        const QString code = lang->currentData().toString();
        const QJsonObject term{{QLatin1String("text"), t}, {QLatin1String("status"), status->currentData().toString()}};
        const QJsonObject e{{QLatin1String("languages"), QJsonArray{QJsonObject{{QLatin1String("lang"), code}, {QLatin1String("terms"), QJsonArray{term}}}}}};
        onResult(c->api()->post(path(QStringLiteral("/entries")), QJsonObject{{QLatin1String("entry"), e}, {QLatin1String("summary"), tr("Created")}}), this,
                 [this, t, code](const ApiResult &r) {
                     if (!r.ok()) {
                         notify(tr("The new entry was not saved: %1").arg(r.error.message));
                         return;
                     }
                     m_entries << r.object();
                     m_selected = str(r.object(), "id");
                     m_detailTab = 0;
                     if (!languages().contains(code)) {
                         QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
                         QJsonArray ls = def.value(QLatin1String("languages")).toArray();
                         ls.append(code);
                         def.insert(QLatin1String("languages"), ls);
                         m_detail.insert(QLatin1String("definition"), def);
                     }
                     notify(tr("Added “%1”").arg(t));
                     showNewEntry(false);
                     showTermbase();
                     showLists();
                     selectEntry(m_selected);
                     emit saved();
                 });
    };
    connect(add, &QPushButton::clicked, this, submit);
    connect(text, &QLineEdit::returnPressed, this, submit);
    m_newEntryHolder->addWidget(form);
    text->setFocus();
}

void TerminologyPage::showDuplicates()
{
    clear(m_dupBody);
    QWidget *host = m_dupBody->parentWidget();
    // .filters
    auto *filters = new QFrame(host);
    Theme::role(filters, "paneBar");
    auto *fl = new QHBoxLayout(filters);
    fl->setContentsMargins(S3, S2, S3, S2);
    fl->setSpacing(S2);
    auto *label = Web::text(tr("Shared term in"), "small2", filters);
    auto *lang = new QComboBox(filters);
    lang->setObjectName(QStringLiteral("duplicateLanguage"));
    lang->setAccessibleName(tr("Shared term in"));
    label->setBuddy(lang);
    lang->addItem(tr("Any language"), QString());
    for (const QString &l : languages())
        lang->addItem(Languages::name(l), l);
    lang->setCurrentIndex(qMax(0, lang->findData(m_dupLang)));
    fl->addWidget(label);
    fl->addWidget(lang, 1);
    m_dupBody->addWidget(filters);
    connect(lang, &QComboBox::activated, this, [this, lang] {
        m_dupLang = lang->currentData().toString();
        showDuplicates();
    });
    auto *holder = new QWidget(host);
    auto *list = new QVBoxLayout(holder);
    list->setContentsMargins(S3, S3, S3, S3);
    list->setSpacing(S2);
    m_dupBody->addWidget(holder);

    Connection *c = conn();
    if (!c)
        return;
    QUrlQuery q;
    if (!m_dupLang.isEmpty())
        q.addQueryItem(QStringLiteral("lang"), m_dupLang);
    const int reading = m_reading;
    const QString forLang = m_dupLang;
    QPointer<QWidget> alive(holder);
    onResult(c->api()->get(path(QStringLiteral("/duplicates")), q), this, [this, alive, list, reading, forLang](const ApiResult &r) {
        if (!alive || reading != m_reading || forLang != m_dupLang)
            return;
        QWidget *host = alive;
        if (!r.ok()) {
            auto *e = Web::text(r.error.message, "error", host);
            e->setWordWrap(true);
            list->addWidget(e);
            return;
        }
        int groups = 0;
        for (const QJsonValue &g : r.array()) {
            QList<QJsonObject> group;
            QStringList ids;
            for (const QJsonValue &id : g.toArray()) {
                const QJsonObject e = entry(id.toString());
                if (!e.isEmpty()) {
                    group << e;
                    ids << id.toString();
                }
            }
            if (group.size() < 2)
                continue;
            const QString key = ids.join(QLatin1Char('|'));
            if (m_dismissed.contains(key))
                continue;
            groups++;
            // The oldest entry is kept, so links to it stay valid.
            std::stable_sort(group.begin(), group.end(), [](const QJsonObject &a, const QJsonObject &b) { return str(a, "createdAt") < str(b, "createdAt"); });
            auto *box = new QFrame(host);
            Theme::role(box, "box");
            box->setObjectName(QStringLiteral("duplicateGroup"));
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(S3, S3, S3, S3);
            bv->setSpacing(S2);
            for (const QJsonObject &e : group) {
                auto *row = new Web::Clickable("dupEntry", box);
                auto *rl = new FlowLayout(row, S2);
                rl->setContentsMargins(S2, 6, S2, 6);
                QStringList said;
                for (const QString &l : TermData::languages(e)) {
                    QStringList ts;
                    for (const QJsonValue &t : TermData::terms(e, l))
                        ts << str(t.toObject(), "text");
                    if (ts.isEmpty())
                        continue;
                    auto *pair = new QWidget(row);
                    auto *pl = new QHBoxLayout(pair);
                    pl->setContentsMargins(0, 0, 0, 0);
                    pl->setSpacing(6);
                    pl->addWidget(Web::langChip(l, pair));
                    pl->addWidget(new QLabel(ts.join(QStringLiteral(", ")), pair));
                    pair->setAttribute(Qt::WA_TransparentForMouseEvents);
                    rl->addWidget(pair);
                    said << QStringLiteral("%1: %2").arg(Languages::name(l), ts.join(QStringLiteral(", ")));
                }
                row->setAccessibleName(said.join(QStringLiteral("; ")));
                const QString id = str(e, "id");
                connect(row, &Web::Clickable::clicked, this, [this, id] { selectEntry(id); });
                bv->addWidget(row);
            }
            auto *merge = Web::button(tr("Merge into one entry"), Web::Small, box);
            merge->setObjectName(QStringLiteral("mergeDuplicates"));
            auto *no = Web::button(tr("Not duplicates"), Web::Quiet | Web::Small, box);
            no->setObjectName(QStringLiteral("notDuplicates"));
            bv->addWidget(Web::row({merge, no}, box));
            connect(no, &QPushButton::clicked, this, [this, key] {
                m_dismissed << key;
                showDuplicates();
            });
            QStringList others;
            for (int i = 1; i < group.size(); i++)
                others << str(group[i], "id");
            const QString keep = str(group.first(), "id");
            const int n = int(group.size());
            connect(merge, &QPushButton::clicked, this, [this, keep, others, n] {
                Connection *c = conn();
                if (!c)
                    return;
                onResult(c->api()->post(path(QStringLiteral("/merge")), QJsonObject{{QLatin1String("keep"), keep}, {QLatin1String("others"), QJsonArray::fromStringList(others)}}), this,
                         [this, keep, others, n](const ApiResult &r) {
                             if (!r.ok()) {
                                 notify(r.error.message);
                                 return;
                             }
                             for (const QString &id : others) {
                                 for (int i = 0; i < m_entries.size(); i++) {
                                     if (str(m_entries[i], "id") == id) {
                                         m_entries.removeAt(i);
                                         break;
                                     }
                                 }
                             }
                             replaceEntry(r.object());
                             m_selected = keep;
                             notify(tr("Merged %1 entries into one").arg(n));
                             showTermbase();
                             showLists();
                             emit saved();
                         });
            });
            list->addWidget(box);
        }
        if (groups == 0) {
            // .empty
            auto *t = Web::text(tr("No duplicates"), "emptyTitle", host);
            t->setObjectName(QStringLiteral("noDuplicates"));
            list->addWidget(t);
            auto *p = Web::text(forLang.isEmpty() ? tr("No two entries share a term.") : tr("No two entries share a term in %1.").arg(Languages::name(forLang)), "secondary", host);
            p->setWordWrap(true);
            list->addWidget(p);
        }
        emit duplicatesShown();
    });
}

void TerminologyPage::showDetail()
{
    clear(m_detailHolder);
    QWidget *host = m_detailHolder->parentWidget();
    if (m_view == 3 && m_opened) {
        showMasterDetail();
        return;
    }
    const QJsonObject e = entry(m_selected);
    if (e.isEmpty() || !m_opened) {
        m_detailHolder->addStretch();
        return;
    }
    const QString id = m_selected;
    const QString src = languages().value(0);
    QString headLang;
    const QString head = TermData::head(e, src, &headLang);

    // .pane-head
    auto *headFrame = new QFrame(host);
    Theme::role(headFrame, "paneHead");
    auto *hv = new QVBoxLayout(headFrame);
    hv->setContentsMargins(S4, S3, S4, S3);
    hv->setSpacing(S1);
    auto *row = new QHBoxLayout;
    row->setSpacing(S2);
    auto *title = Web::text(head.isEmpty() ? tr("Entry") : head, "paneTitle", headFrame);
    title->setObjectName(QStringLiteral("entryTitle"));
    title->setWordWrap(true);
    if (Languages::rtl(headLang))
        title->setLayoutDirection(Qt::RightToLeft);
    row->addWidget(title, 1);
    auto *del = new Web::ConfirmButton(tr("Delete entry"), tr("Delete this entry? It can be brought back from Deleted, without its history."), tr("Delete"), headFrame, true,
                                       QStringLiteral("trash"));
    del->setObjectName(QStringLiteral("deleteEntry"));
    if (may("edit"))
        row->addWidget(del, 0, Qt::AlignTop);
    else
        del->hide();
    hv->addLayout(row);
    connect(del, &Web::ConfirmButton::confirmed, this, [this, id, head] {
        Connection *c = conn();
        if (!c)
            return;
        onResult(c->api()->del(path(QStringLiteral("/entries/") + ApiClient::seg(id))), this, [this, id, head](const ApiResult &r) {
            if (!r.ok()) {
                failed(tr("Deleting the entry"), r.error.message);
                return;
            }
            for (int i = 0; i < m_entries.size(); i++) {
                if (str(m_entries[i], "id") == id) {
                    m_entries.removeAt(i);
                    break;
                }
            }
            m_selected.clear();
            notify(head.isEmpty() ? tr("Deleted the entry") : tr("Deleted the entry “%1”").arg(head));
            showTermbase();
            showLists();
            emit saved();
        });
    });

    // .pane-sub
    auto *sub = new QWidget(headFrame);
    auto *sl = new QHBoxLayout(sub);
    sl->setContentsMargins(0, 0, 0, 0);
    sl->setSpacing(S2);
    const QString changed = when(str(e, "changedAt").isEmpty() ? str(e, "createdAt") : str(e, "changedAt"));
    const QString by = str(e, "changedBy").isEmpty() ? str(e, "createdBy") : str(e, "changedBy");
    if (!changed.isEmpty()) {
        auto *l = Web::text(by.isEmpty() ? tr("Last changed %1").arg(changed) : tr("Last changed %1 by %2").arg(changed, by), "meta", sub);
        l->setObjectName(QStringLiteral("lastChanged"));
        l->setWordWrap(false);
        sl->addWidget(l);
    }
    if (m_bases.contains(id))
        sl->addWidget(Web::text(QStringLiteral("%1 %2").arg(Words::get("basedOn"), m_bases.value(id)), "meta", sub));
    else if (isProject())
        sl->addWidget(Web::chip(tr("Not in a master yet"), QString(), sub));
    sl->addStretch();
    hv->addWidget(sub);
    m_detailHolder->addWidget(headFrame);

    // .tabs
    auto *tabs = new Web::Tabs(tr("Entry views"), host);
    tabs->setObjectName(QStringLiteral("entryTabs"));
    tabs->add(tr("Entry"));
    tabs->add(QStringLiteral("%1 (%2)").arg(Words::get("history")).arg(qMax(1, e.value(QLatin1String("version")).toInt())), QStringLiteral("history"));
    tabs->setCurrent(m_detailTab);
    m_detailHolder->addWidget(tabs);
    connect(tabs, &Web::Tabs::changed, this, [this](int i) {
        m_detailTab = i;
        showDetail();
    });

    // .detail-body
    QVBoxLayout *body = nullptr;
    QScrollArea *scroll = scroller(host, &body, S4, S4);
    scroll->setObjectName(QStringLiteral("entryBody"));
    m_detailHolder->addWidget(scroll, 1);
    QWidget *inner = scroll->widget();
    if (m_detailTab == 1) {
        showHistory(body);
        body->addStretch();
        return;
    }
    if (!may("edit")) {
        readEntry(e, body, inner);
        body->addStretch();
        return;
    }

    // The fields of the whole entry.
    bool any = false;
    auto *entryFields = new QWidget(inner);
    auto *efl = new QVBoxLayout(entryFields);
    efl->setContentsMargins(0, 0, 0, 0);
    efl->setSpacing(S2);
    for (const QJsonValue &fv : fields()) {
        const QJsonObject f = fv.toObject();
        if (str(f, "level") != QLatin1String("entry"))
            continue;
        any = true;
        const QString name = str(f, "name");
        efl->addWidget(fieldRow(f, TermData::value(e.value(QLatin1String("fields")).toArray(), name), [this, id, name](const QString &v) {
            change(id, v.isEmpty() ? tr("Cleared %1").arg(name) : tr("Set %1").arg(name), [name, v](QJsonObject entry) { return TermData::withEntryField(entry, name, v); }, false);
        }, false, entryFields));
    }
    if (any)
        body->addWidget(entryFields);
    else
        entryFields->deleteLater();
    body->addWidget(addField(inner), 0, Qt::AlignLeft);

    // The languages: those of the termbase, then any others the entry has.
    QStringList langs = languages();
    for (const QString &l : TermData::languages(e)) {
        if (!langs.contains(l))
            langs << l;
    }
    for (const QString &l : langs)
        body->addWidget(languageBlock(l, inner));

    // .add-lang
    auto *more = new QComboBox(inner);
    more->setObjectName(QStringLiteral("addLanguage"));
    more->setAccessibleName(tr("Add a language to %1").arg(m_name));
    more->addItem(tr("Add a language to the termbase…"), QString());
    for (const QString &l : Languages::common()) {
        if (!langs.contains(l))
            more->addItem(Languages::name(l), l);
    }
    more->setMaximumWidth(420);
    if (more->count() > 1)
        body->addWidget(more);
    else
        more->deleteLater();
    connect(more, &QComboBox::activated, this, [this, more](int i) {
        const QString l = more->itemData(i).toString();
        if (!l.isEmpty())
            addLanguageToDefinition(l);
    });
    body->addStretch();
}

QWidget *TerminologyPage::languageBlock(const QString &lang, QWidget *parent)
{
    const QJsonObject e = entry(m_selected);
    const QString id = m_selected;
    // .lang-block
    auto *block = new QWidget(parent);
    block->setObjectName(QStringLiteral("language:%1").arg(lang));
    block->setAccessibleName(Languages::name(lang));
    auto *v = new QVBoxLayout(block);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S2);
    v->addWidget(line(block));
    v->addSpacing(S2);
    auto *titleRow = new QHBoxLayout;
    titleRow->setSpacing(S2);
    titleRow->addWidget(Web::text(Languages::name(lang), "langTitle", block));
    titleRow->addWidget(Web::langChip(lang, block));
    titleRow->addStretch();
    v->addLayout(titleRow);

    const QJsonObject section = TermData::language(e, lang);
    for (const QJsonValue &fv : fields()) {
        const QJsonObject f = fv.toObject();
        if (str(f, "level") != QLatin1String("language"))
            continue;
        const QString name = str(f, "name");
        v->addWidget(fieldRow(f, TermData::value(section.value(QLatin1String("fields")).toArray(), name), [this, id, lang, name](const QString &val) {
            change(id, val.isEmpty() ? tr("Cleared %1 (%2)").arg(name, Languages::name(lang)) : tr("Set %1 (%2)").arg(name, Languages::name(lang)),
                   [lang, name, val](QJsonObject entry) { return TermData::withLanguageField(entry, lang, name, val); }, false);
        }, true, block));
    }
    const QJsonArray terms = section.value(QLatin1String("terms")).toArray();
    if (terms.isEmpty()) {
        v->addWidget(Web::text(tr("No term yet"), "missing", block));
    } else {
        // .terms
        auto *list = new QVBoxLayout;
        list->setSpacing(S3);
        for (int i = 0; i < terms.size(); i++)
            list->addWidget(termItem(lang, i, block));
        v->addLayout(list);
    }
    v->addWidget(addTerm(lang, block), 0, Qt::AlignLeft);
    return block;
}

QWidget *TerminologyPage::fieldRow(const QJsonObject &field, const QString &value, std::function<void(const QString &)> save, bool small, QWidget *parent)
{
    // .field-row: the name in 120 px (96 for a language's or term's
    // field), then the value.
    auto *row = new QWidget(parent);
    auto *h = new QHBoxLayout(row);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(S3);
    const QString name = str(field, "name");
    auto *label = Web::text(name, "small2", row);
    label->setFixedWidth(small ? 96 : 120);
    label->setWordWrap(true);
    label->setContentsMargins(0, small ? 4 : 7, 0, 0);
    h->addWidget(label, 0, Qt::AlignTop);
    // The control the field's kind asks for, as the web's FieldValue: a
    // list, yes or no, a date, a number or text. A value that does not fit
    // its kind (from an import, or set before the kind changed) is shown as
    // it is and can be corrected as text.
    const QString kind = str(field, "kind");
    static const QRegularExpression isoDate(QStringLiteral("^\\d{4}-\\d{2}-\\d{2}"));
    static const QRegularExpression number(QStringLiteral("^[-+]?(\\d+([.,]\\d*)?|[.,]\\d+)$"));
    if (kind == QLatin1String("date") && (value.isEmpty() || isoDate.match(value).hasMatch())) {
        auto *d = new QDateEdit(row);
        if (small)
            d->setProperty("small", true);
        d->setAccessibleName(name);
        d->setCalendarPopup(true);
        d->setDisplayFormat(QStringLiteral("yyyy-MM-dd"));
        // The earliest date stands for none.
        const QDate none(1752, 9, 14);
        d->setMinimumDate(none);
        d->setSpecialValueText(tr("Not set"));
        const QDate was = value.isEmpty() ? none : QDate::fromString(value.left(10), Qt::ISODate);
        d->setDate(was.isValid() ? was : none);
        label->setBuddy(d);
        auto *last = new QString(value.left(10));
        connect(d, &QObject::destroyed, this, [last] { delete last; });
        const auto store = [d, save, last, none] {
            const QString now = d->date() == none ? QString() : d->date().toString(Qt::ISODate);
            if (now != *last) {
                *last = now;
                save(now);
            }
        };
        connect(d, &QDateEdit::editingFinished, this, store);
        if (QCalendarWidget *cal = d->calendarWidget())
            connect(cal, &QCalendarWidget::clicked, this, store);
        h->addWidget(d, 1);
    } else if (kind == QLatin1String("number")) {
        auto *box = new QWidget(row);
        auto *bv = new QVBoxLayout(box);
        bv->setContentsMargins(0, 0, 0, 0);
        bv->setSpacing(S1);
        auto *t = new QLineEdit(value, box);
        if (small)
            t->setProperty("small", true);
        t->setAccessibleName(name);
        t->setPlaceholderText(tr("Not set"));
        t->setInputMethodHints(Qt::ImhFormattedNumbersOnly);
        auto *problem = Web::text(tr("Type a number, such as 12 or 0.5."), "error", box);
        problem->setWordWrap(true);
        problem->hide();
        bv->addWidget(t);
        bv->addWidget(problem);
        label->setBuddy(t);
        auto *last = new QString(value);
        connect(t, &QObject::destroyed, this, [last] { delete last; });
        connect(t, &QLineEdit::textEdited, problem, &QWidget::hide);
        connect(t, &QLineEdit::editingFinished, this, [t, save, last, problem] {
            const QString v = t->text().trimmed();
            if (!v.isEmpty() && !number.match(v).hasMatch()) {
                problem->show();
                return;
            }
            problem->hide();
            if (v != *last) {
                *last = v;
                save(v);
            }
        });
        h->addWidget(box, 1);
    } else if (kind == QLatin1String("picklist") || kind == QLatin1String("boolean")) {
        auto *c = new QComboBox(row);
        if (small)
            c->setProperty("small", true);
        c->setAccessibleName(name);
        c->addItem(tr("Not set"), QString());
        bool known = value.isEmpty();
        QList<std::pair<QString, QString>> options;
        if (kind == QLatin1String("boolean")) {
            options = {{QStringLiteral("true"), tr("Yes")}, {QStringLiteral("false"), tr("No")}};
        } else {
            for (const QJsonValue &v : field.value(QLatin1String("values")).toArray())
                options.append({v.toString(), v.toString()});
        }
        for (const auto &[stored, shown] : options) {
            c->addItem(shown, stored);
            known = known || stored == value;
        }
        if (!known)
            c->addItem(value, value);
        c->setCurrentIndex(qMax(0, c->findData(value)));
        label->setBuddy(c);
        connect(c, &QComboBox::activated, this, [c, save, value] {
            if (c->currentData().toString() != value)
                save(c->currentData().toString());
        });
        h->addWidget(c, 1);
    } else if (!small) {
        auto *t = new BlurEdit(value, row);
        t->setAccessibleName(name);
        t->setPlaceholderText(tr("Not set"));
        t->setTabChangesFocus(true);
        t->setFixedHeight(2 * t->fontMetrics().lineSpacing() + 18);
        label->setBuddy(t);
        auto *last = new QString(value);
        connect(t, &QObject::destroyed, this, [last] { delete last; });
        t->left = [t, save, last] {
            const QString now = t->toPlainText();
            if (now != *last) {
                *last = now;
                save(now.trimmed());
            }
        };
        h->addWidget(t, 1);
    } else {
        auto *t = new QLineEdit(value, row);
        t->setProperty("small", true);
        t->setAccessibleName(name);
        t->setPlaceholderText(tr("Not set"));
        label->setBuddy(t);
        auto *last = new QString(value);
        connect(t, &QObject::destroyed, this, [last] { delete last; });
        connect(t, &QLineEdit::editingFinished, this, [t, save, last] {
            if (t->text() != *last) {
                *last = t->text();
                save(t->text().trimmed());
            }
        });
        h->addWidget(t, 1);
    }
    return row;
}

QWidget *TerminologyPage::termItem(const QString &lang, int index, QWidget *parent)
{
    const QString id = m_selected;
    const QJsonArray terms = TermData::terms(entry(id), lang);
    const QJsonObject t = terms[index].toObject();
    const QString text = str(t, "text");
    const QString status = str(t, "status").isEmpty() ? QStringLiteral("proposed") : str(t, "status");
    // .term
    auto *box = new QFrame(parent);
    Theme::role(box, "term");
    box->setObjectName(QStringLiteral("term:%1:%2").arg(lang).arg(index));
    auto *v = new QVBoxLayout(box);
    v->setContentsMargins(S3, S3, S3, S3);
    v->setSpacing(S2);

    auto *stack = new Web::Switcher(box);
    // .term-line
    auto *shown = new QWidget(stack);
    auto *sl = new QHBoxLayout(shown);
    sl->setContentsMargins(0, 0, 0, 0);
    sl->setSpacing(S2);
    auto *label = Web::text(text, "termText", shown);
    label->setWordWrap(true);
    label->setTextInteractionFlags(Qt::TextSelectableByMouse);
    if (Languages::rtl(lang))
        label->setLayoutDirection(Qt::RightToLeft);
    sl->addWidget(label, 1);
    sl->addWidget(Web::chip(TermData::statusLabel(status), status, shown), 0, Qt::AlignVCenter);
    auto *statusBox = statusCombo(status, shown, true);
    statusBox->setAccessibleName(tr("Status of %1").arg(text));
    sl->addWidget(statusBox);
    auto *edit = new QPushButton(shown);
    edit->setProperty("iconButton", true);
    edit->setIcon(Icons::get(QStringLiteral("pencil"), Theme::color("text2")));
    edit->setIconSize(QSize(14, 14));
    edit->setAccessibleName(tr("Edit %1").arg(text));
    edit->setToolTip(tr("Edit %1").arg(text));
    sl->addWidget(edit);
    auto *remove = new Web::ConfirmButton(QString(), tr("Remove “%1”?").arg(text), tr("Remove"), shown, true, QStringLiteral("trash"));
    remove->button()->setAccessibleName(tr("Remove %1").arg(text));
    remove->button()->setToolTip(tr("Remove %1").arg(text));
    sl->addWidget(remove);
    stack->add(shown);

    // .term-edit
    auto *editor = new QWidget(stack);
    auto *el = new QVBoxLayout(editor);
    el->setContentsMargins(0, 0, 0, 0);
    el->setSpacing(S1);
    auto *er = new QHBoxLayout;
    er->setSpacing(S1);
    auto *draft = new QLineEdit(text, editor);
    draft->setAccessibleName(tr("Term"));
    if (Languages::rtl(lang))
        draft->setLayoutDirection(Qt::RightToLeft);
    auto *save = Web::button(tr("Save"), Web::Primary | Web::Small, editor);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, editor);
    er->addWidget(draft, 1);
    er->addWidget(save);
    er->addWidget(cancel);
    el->addLayout(er);
    auto *error = Web::text(QString(), "error", editor);
    error->setWordWrap(true);
    error->hide();
    el->addWidget(error);
    stack->add(editor);
    v->addWidget(stack);

    connect(edit, &QPushButton::clicked, this, [stack, draft] {
        stack->setCurrent(1);
        draft->setFocus();
        draft->selectAll();
    });
    connect(cancel, &QPushButton::clicked, this, [stack, draft, text] {
        draft->setText(text);
        stack->setCurrent(0);
    });
    const auto check = [draft, save, error, terms, index, lang] {
        const QString d = draft->text().trimmed();
        const bool dup = TermData::hasTerm(terms, d, index);
        save->setEnabled(!d.isEmpty() && !dup);
        draft->setProperty("invalid", dup);
        error->setText(dup ? TerminologyPage::tr("This entry already has “%1” in %2.").arg(d, Languages::name(lang)) : QString());
        error->setVisible(dup);
    };
    connect(draft, &QLineEdit::textChanged, this, check);
    const auto submit = [this, id, lang, index, draft, text, stack, terms] {
        const QString d = draft->text().trimmed();
        if (d.isEmpty() || TermData::hasTerm(terms, d, index))
            return;
        stack->setCurrent(0);
        if (d == text)
            return;
        change(id, tr("Changed “%1” to “%2”").arg(text, d), [lang, index, d](QJsonObject entry) {
            QJsonArray ts = TermData::terms(entry, lang);
            if (index < ts.size()) {
                QJsonObject t = ts[index].toObject();
                t.insert(QLatin1String("text"), d);
                ts[index] = t;
            }
            return TermData::withTerms(entry, lang, ts);
        });
    };
    connect(save, &QPushButton::clicked, this, submit);
    connect(draft, &QLineEdit::returnPressed, this, submit);
    connect(statusBox, &QComboBox::activated, this, [this, id, lang, index, statusBox, text, status] {
        const QString s = statusBox->currentData().toString();
        if (s == status)
            return;
        change(id, tr("Set “%1” to %2").arg(text, s), [lang, index, s](QJsonObject entry) {
            QJsonArray ts = TermData::terms(entry, lang);
            if (index < ts.size()) {
                QJsonObject t = ts[index].toObject();
                t.insert(QLatin1String("status"), s);
                ts[index] = t;
            }
            return TermData::withTerms(entry, lang, ts);
        });
    });
    connect(remove, &Web::ConfirmButton::confirmed, this, [this, id, lang, index, text] {
        change(id, tr("Removed “%1”").arg(text), [lang, index](QJsonObject entry) {
            QJsonArray ts = TermData::terms(entry, lang);
            if (index < ts.size())
                ts.removeAt(index);
            return TermData::withTerms(entry, lang, ts);
        });
        notify(tr("Removed “%1”").arg(text));
    });

    // The term's own fields.
    for (const QJsonValue &fv : fields()) {
        const QJsonObject f = fv.toObject();
        if (str(f, "level") != QLatin1String("term"))
            continue;
        const QString name = str(f, "name");
        v->addWidget(fieldRow(f, TermData::value(t.value(QLatin1String("fields")).toArray(), name), [this, id, lang, index, name, text](const QString &val) {
            change(id, val.isEmpty() ? tr("Cleared %1 of “%2”").arg(name, text) : tr("Set %1 of “%2”").arg(name, text),
                   [lang, index, name, val](QJsonObject entry) { return TermData::withTermField(entry, lang, index, name, val); }, false);
        }, true, box));
    }
    return box;
}

QWidget *TerminologyPage::addTerm(const QString &lang, QWidget *parent)
{
    const QString id = m_selected;
    const QJsonArray terms = TermData::terms(entry(id), lang);
    auto *stack = new Web::Switcher(parent);
    auto *closed = new QWidget(stack);
    auto *cl = new QHBoxLayout(closed);
    cl->setContentsMargins(0, 0, 0, 0);
    auto *open = new QPushButton(terms.isEmpty() ? tr("Add a term") : tr("Add a synonym"), closed);
    open->setProperty("tiny", true);
    open->setProperty("quiet", true);
    open->setObjectName(QStringLiteral("addTerm:%1").arg(lang));
    open->setAccessibleName(tr("%1 in %2").arg(open->text(), Languages::name(lang)));
    cl->addWidget(open);
    cl->addStretch();
    stack->add(closed);

    auto *form = new QWidget(stack);
    auto *fv = new QVBoxLayout(form);
    fv->setContentsMargins(0, S1, 0, 0);
    fv->setSpacing(S1);
    auto *fr = new QHBoxLayout;
    fr->setSpacing(S1);
    auto *text = new QLineEdit(form);
    text->setObjectName(QStringLiteral("newTerm:%1").arg(lang));
    text->setAccessibleName(tr("New term in %1").arg(Languages::name(lang)));
    text->setPlaceholderText(tr("Term in %1").arg(Languages::name(lang)));
    if (Languages::rtl(lang))
        text->setLayoutDirection(Qt::RightToLeft);
    auto *status = statusCombo(QStringLiteral("proposed"), form, true);
    status->setAccessibleName(tr("Status"));
    auto *add = Web::button(tr("Add"), Web::Primary | Web::Small, form);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, form);
    add->setEnabled(false);
    fr->addWidget(text, 1);
    fr->addWidget(status);
    fr->addWidget(add);
    fr->addWidget(cancel);
    fv->addLayout(fr);
    auto *error = Web::text(QString(), "error", form);
    error->setWordWrap(true);
    error->hide();
    fv->addWidget(error);
    stack->add(form);
    // The form takes the block's width; the button only its own.
    connect(stack, &Web::Switcher::changed, parent, [stack, parent](int i) {
        if (auto *l = qobject_cast<QBoxLayout *>(parent->layout()))
            l->setAlignment(stack, i == 1 ? Qt::Alignment() : Qt::AlignLeft);
    });

    connect(open, &QPushButton::clicked, this, [stack, text] {
        stack->setCurrent(1);
        text->setFocus();
    });
    connect(cancel, &QPushButton::clicked, this, [stack, text] {
        text->clear();
        stack->setCurrent(0);
    });
    connect(text, &QLineEdit::textChanged, this, [text, add, error, terms] {
        const QString d = text->text().trimmed();
        const bool dup = !d.isEmpty() && TermData::hasTerm(terms, d);
        add->setEnabled(!d.isEmpty() && !dup);
        text->setProperty("invalid", dup);
        error->setText(dup ? TerminologyPage::tr("This entry already has “%1”.").arg(d) : QString());
        error->setVisible(dup);
    });
    const auto submit = [this, id, lang, text, status, terms] {
        const QString d = text->text().trimmed();
        if (d.isEmpty() || TermData::hasTerm(terms, d))
            return;
        const QString s = status->currentData().toString();
        change(id, tr("Added “%1”").arg(d), [lang, d, s](QJsonObject entry) {
            QJsonArray ts = TermData::terms(entry, lang);
            ts.append(QJsonObject{{QLatin1String("text"), d}, {QLatin1String("status"), s}});
            return TermData::withTerms(entry, lang, ts);
        });
        if (!languages().contains(lang))
            addLanguageToDefinition(lang);
    };
    connect(add, &QPushButton::clicked, this, submit);
    connect(text, &QLineEdit::returnPressed, this, submit);
    return stack;
}

QWidget *TerminologyPage::addField(QWidget *parent)
{
    QStringList taken;
    for (const QJsonValue &f : fields())
        taken << str(f.toObject(), "name");
    auto *form = new TermFields::AddFieldForm(m_name, taken, parent);
    connect(form, &Web::Switcher::changed, parent, [form, parent](int i) {
        if (auto *l = qobject_cast<QBoxLayout *>(parent->layout()))
            l->setAlignment(form, i == 1 ? Qt::Alignment() : Qt::AlignLeft);
    });
    connect(form, &TermFields::AddFieldForm::added, this, [this](const QJsonObject &field) {
        QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
        QJsonArray fs = def.value(QLatin1String("fields")).toArray();
        fs.append(field);
        def.insert(QLatin1String("fields"), fs);
        putDefinition(def, tr("Added field “%1” to %2").arg(str(field, "name"), m_name));
    });
    return form;
}

void TerminologyPage::showHistory(QVBoxLayout *into)
{
    Connection *c = conn();
    if (!c)
        return;
    QWidget *host = into->parentWidget();
    auto *holder = new QWidget(host);
    holder->setObjectName(QStringLiteral("history"));
    auto *list = new QVBoxLayout(holder);
    list->setContentsMargins(4, 0, 0, 0);
    list->setSpacing(0);
    into->addWidget(holder);
    const QString id = m_selected;
    QPointer<QWidget> alive(holder);
    onResult(c->api()->get(path(QStringLiteral("/entries/") + ApiClient::seg(id) + QStringLiteral("/history"))), this, [this, alive, list](const ApiResult &r) {
        if (!alive)
            return;
        if (!r.ok()) {
            auto *e = Web::text(r.error.message, "error", alive);
            e->setWordWrap(true);
            list->addWidget(e);
            return;
        }
        // .history: the newest first.
        const QJsonArray versions = r.array();
        for (int i = int(versions.size()) - 1; i >= 0; i--) {
            const QJsonObject ver = versions[i].toObject();
            auto *item = new QFrame(alive);
            Theme::role(item, "historyItem");
            auto *v = new QVBoxLayout(item);
            v->setContentsMargins(S4, S3, 0, S3);
            v->setSpacing(2);
            const QString summary = str(ver, "summary").isEmpty() ? (i == 0 ? tr("Created") : tr("Changed")) : str(ver, "summary");
            auto *s = Web::text(summary, "historySummary", item);
            s->setWordWrap(true);
            v->addWidget(s);
            const QString at = when(str(ver, "at"));
            auto *m = Web::text(str(ver, "by").isEmpty() ? at : QStringLiteral("%1, %2").arg(str(ver, "by"), at), "meta", item);
            m->setWordWrap(true);
            v->addWidget(m);
            // What the entry held then, and setting it back to that.
            const QJsonObject then = ver.value(QLatin1String("entry")).toObject();
            QStringList held;
            for (const QString &l : TermData::languages(then)) {
                QStringList ts;
                for (const QJsonValue &t : TermData::terms(then, l))
                    ts << str(t.toObject(), "text");
                if (!ts.isEmpty())
                    held << QStringLiteral("%1: %2").arg(l, ts.join(QStringLiteral(", ")));
            }
            if (!held.isEmpty()) {
                auto *h = Web::text(held.join(QStringLiteral("; ")), "small2", item);
                h->setWordWrap(true);
                v->addWidget(h);
            }
            if (i < versions.size() - 1 && may("edit")) {
                const int number = ver.value(QLatin1String("version")).toInt();
                auto *restore = new Web::ConfirmButton(tr("Restore this version"), tr("Set the entry back to this version? What it is now stays in the history."), tr("Restore"), item);
                restore->setObjectName(QStringLiteral("restore:%1").arg(number));
                restore->setDanger(false);
                v->addWidget(restore, 0, Qt::AlignLeft);
                connect(restore, &Web::ConfirmButton::confirmed, this, [this, number, at] {
                    Connection *c = conn();
                    if (!c)
                        return;
                    const QString id = m_selected;
                    onResult(c->api()->post(path(QStringLiteral("/entries/") + ApiClient::seg(id) + QStringLiteral("/revert")), QJsonObject{{QLatin1String("version"), number}}), this,
                             [this, at](const ApiResult &r) {
                                 if (!r.ok()) {
                                     notify(tr("The version was not restored: %1").arg(r.error.message));
                                     return;
                                 }
                                 replaceEntry(r.object());
                                 notify(tr("Restored the version of %1").arg(at));
                                 m_detailTab = 0;
                                 showEntries();
                                 showDetail();
                                 emit saved();
                             });
                });
            }
            item->setAccessibleName(QStringLiteral("%1. %2").arg(summary, m->text()));
            list->addWidget(item);
        }
        emit historyShown();
    });
}

void TerminologyPage::change(const QString &entryId, const QString &summary, std::function<QJsonObject(QJsonObject)> apply, bool rebuild)
{
    m_queue << Change{entryId, summary, std::move(apply), rebuild};
    if (!m_saving)
        sendNext();
}

void TerminologyPage::sendNext()
{
    Connection *c = conn();
    if (m_queue.isEmpty() || !c) {
        m_queue.clear();
        m_saving = false;
        emit saved();
        return;
    }
    // Each change is made to the entry as the core last returned it, and
    // saved from that version, one after the other.
    const Change ch = m_queue.takeFirst();
    const QJsonObject before = entry(ch.entry);
    if (before.isEmpty()) {
        sendNext();
        return;
    }
    m_saving = true;
    const QJsonObject after = ch.apply(TermData::forSaving(before));
    const QJsonObject body{{QLatin1String("entry"), after}, {QLatin1String("summary"), ch.summary}};
    const int reading = m_reading;
    const bool rebuild = ch.rebuild;
    const QString id = ch.entry;
    onResult(c->api()->put(path(QStringLiteral("/entries/") + ApiClient::seg(id)), body, before.value(QLatin1String("version")).toInt()), this,
             [this, reading, rebuild, id](const ApiResult &r) {
                 if (reading != m_reading) {
                     m_queue.clear();
                     m_saving = false;
                     emit saved();
                     return;
                 }
                 if (!r.ok()) {
                     m_queue.clear();
                     m_saving = false;
                     failed(tr("The entry"), r.error.message);
                     emit saved();
                     return;
                 }
                 replaceEntry(r.object());
                 // A changed value stays where it was typed; what changes
                 // the entry's parts is drawn again.
                 const bool more = !m_queue.isEmpty();
                 if (!more) {
                     const int scroll = m_list->verticalScrollBar()->value();
                     showEntries();
                     m_list->verticalScrollBar()->setValue(scroll);
                     if (rebuild && id == m_selected) {
                         auto *body = findChild<QScrollArea *>(QStringLiteral("entryBody"));
                         const int at = body ? body->verticalScrollBar()->value() : 0;
                         showDetail();
                         if (auto *again = findChild<QScrollArea *>(QStringLiteral("entryBody")))
                             QTimer::singleShot(0, again, [again, at] { again->verticalScrollBar()->setValue(at); });
                     } else if (id == m_selected) {
                         const QJsonObject e = entry(id);
                         if (auto *l = findChild<QLabel *>(QStringLiteral("lastChanged")))
                             l->setText(tr("Last changed %1 by %2").arg(when(str(e, "changedAt")), str(e, "changedBy")));
                         if (auto *tabs = findChild<Web::Tabs *>(QStringLiteral("entryTabs")))
                             tabs->setText(1, QStringLiteral("%1 (%2)").arg(Words::get("history")).arg(e.value(QLatin1String("version")).toInt()));
                     }
                 }
                 sendNext();
             });
}

void TerminologyPage::replaceEntry(const QJsonObject &stored)
{
    const QString id = str(stored, "id");
    for (int i = 0; i < m_entries.size(); i++) {
        if (str(m_entries[i], "id") == id) {
            m_entries[i] = stored;
            return;
        }
    }
    m_entries << stored;
}

void TerminologyPage::failed(const QString &what, const QString &why)
{
    // What the core has replaces what is shown.
    notify(tr("%1 was not saved: %2").arg(what, why));
    m_reload.start();
}

void TerminologyPage::addLanguageToDefinition(const QString &lang)
{
    if (languages().contains(lang))
        return;
    QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
    QJsonArray ls = def.value(QLatin1String("languages")).toArray();
    ls.append(lang);
    def.insert(QLatin1String("languages"), ls);
    putDefinition(def, QString());
}

void TerminologyPage::putDefinition(const QJsonObject &definition, const QString &done)
{
    Connection *c = conn();
    if (!c)
        return;
    const int reading = m_reading;
    onResult(c->api()->put(path(QStringLiteral("/definition")), definition), this, [this, reading, done](const ApiResult &r) {
        if (reading != m_reading)
            return;
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        const QJsonObject def = r.object().value(QLatin1String("definition")).toObject();
        m_detail.insert(QLatin1String("definition"), def);
        if (!done.isEmpty())
            notify(done);
        showHead();
        showEntries();
        showDetail();
        emit saved();
    });
}

ImportDialog *TerminologyPage::importDialog(bool intoNew)
{
    auto *d = new ImportDialog(m_manager, intoNew || !m_opened ? QString() : m_conn, intoNew || !m_opened ? QString() : m_name, window());
    d->setAttribute(Qt::WA_DeleteOnClose);
    connect(d, &ImportDialog::imported, this, [this](const QString &connId, const QString &name, const QString &summary) {
        notify(summary);
        m_conn = connId;
        m_name = name;
        reload();
    });
    d->open();
    return d;
}

ExportDialog *TerminologyPage::exportDialog()
{
    Connection *c = conn();
    if (!c || !m_opened)
        return nullptr;
    QStringList shownIds;
    const QList<QJsonObject> list = shown();
    for (const QJsonObject &e : list)
        shownIds << str(e, "id");
    auto *d = new ExportDialog(c, m_name, int(m_entries.size()), shownIds, window());
    d->setAttribute(Qt::WA_DeleteOnClose);
    connect(d, &ExportDialog::exported, this, &TerminologyPage::notify);
    d->open();
    return d;
}

TermbaseSettingsDialog *TerminologyPage::settingsDialog()
{
    Connection *c = conn();
    if (!c || !m_opened)
        return nullptr;
    auto *d = new TermbaseSettingsDialog(c, m_detail, m_entries, window());
    d->setAttribute(Qt::WA_DeleteOnClose);
    connect(d, &TermbaseSettingsDialog::said, this, &TerminologyPage::notify);
    connect(d, &TermbaseSettingsDialog::changed, this, [this](const QString &name) {
        // Renamed, changed or deleted: read it again under its name.
        m_name = name;
        if (name.isEmpty())
            m_selected.clear();
        reload();
    });
    d->open();
    return d;
}

TerminologySection::TerminologySection(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
{
    auto *v = new QVBoxLayout(this);
    v->setContentsMargins(0, 0, 0, 0);
    m_stack = new QStackedWidget(this);
    v->addWidget(m_stack);
    m_termbases = new TerminologyPage(manager, m_stack);
    m_review = new ReviewPage(m_stack);
    m_stack->addWidget(m_termbases);
    m_stack->addWidget(m_review);
    connect(m_termbases, &TerminologyPage::openReview, this, &TerminologySection::openReview);
    connect(m_review, &ReviewPage::back, this, &TerminologySection::open);
}

bool TerminologySection::reviewing() const
{
    return m_stack->currentWidget() == m_review;
}

void TerminologySection::open(const QString &connectionId, const QString &termbase)
{
    m_stack->setCurrentWidget(m_termbases);
    if (!connectionId.isEmpty() && !termbase.isEmpty())
        m_termbases->open(connectionId, termbase);
    else
        m_termbases->reload();
}

void TerminologySection::openReview(const QString &connectionId, int mergeRequest)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        return;
    m_review->open(c, mergeRequest);
    m_stack->setCurrentWidget(m_review);
}

void TerminologyPage::showDeleted()
{
    clear(m_deletedBody);
    QWidget *host = m_deletedBody->parentWidget();
    auto *holder = new QWidget(host);
    auto *list = new QVBoxLayout(holder);
    list->setContentsMargins(S3, S3, S3, S3);
    list->setSpacing(S2);
    m_deletedBody->addWidget(holder);
    Connection *c = conn();
    if (!c)
        return;
    const int reading = m_reading;
    QPointer<QWidget> alive(holder);
    onResult(c->api()->get(path(QStringLiteral("/deleted"))), this, [this, alive, list, reading](const ApiResult &r) {
        if (!alive || reading != m_reading)
            return;
        QWidget *host = alive;
        if (!r.ok()) {
            auto *e = Web::text(r.error.message, "error", host);
            e->setWordWrap(true);
            list->addWidget(e);
            return;
        }
        // Of an entry deleted more than once, the last deletion counts.
        QJsonArray gone;
        QSet<QString> seen;
        for (const QJsonValue &v : r.array()) {
            const QString id = str(v.toObject(), "entryId");
            if (seen.contains(id))
                continue;
            seen << id;
            gone.append(v);
        }
        m_deleted = gone;
        if (gone.isEmpty()) {
            auto *t = Web::text(tr("Nothing deleted"), "emptyTitle", host);
            t->setObjectName(QStringLiteral("noDeleted"));
            list->addWidget(t);
            auto *p = Web::text(tr("Entries you delete are kept here and can be brought back."), "secondary", host);
            p->setWordWrap(true);
            list->addWidget(p);
            emit deletedShown();
            return;
        }
        for (const QJsonValue &v : gone) {
            const QJsonObject d = v.toObject();
            const QJsonObject e = d.value(QLatin1String("entry")).toObject();
            const QString id = str(d, "entryId");
            auto *box = new QFrame(host);
            Theme::role(box, "box");
            box->setProperty("part", QStringLiteral("deletedEntry"));
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(S3, S3, S3, S3);
            bv->setSpacing(S1);
            QStringList said;
            for (const QString &l : TermData::languages(e)) {
                QStringList ts;
                for (const QJsonValue &t : TermData::terms(e, l))
                    ts << str(t.toObject(), "text");
                if (ts.isEmpty())
                    continue;
                auto *row = new QHBoxLayout;
                row->setSpacing(6);
                row->addWidget(Web::langChip(l, box));
                auto *terms = new QLabel(ts.join(QStringLiteral(", ")), box);
                terms->setWordWrap(true);
                row->addWidget(terms, 1);
                bv->addLayout(row);
                said << QStringLiteral("%1: %2").arg(Languages::name(l), ts.join(QStringLiteral(", ")));
            }
            const QString whenText = when(str(d, "at"));
            QString meta = str(d, "by").isEmpty() ? tr("Deleted %1").arg(whenText) : tr("Deleted %1 by %2").arg(whenText, str(d, "by"));
            // What was written down as the reason, if anything was.
            if (!str(d, "summary").isEmpty() && str(d, "summary") != QLatin1String("Deleted"))
                meta += QStringLiteral(". ") + str(d, "summary");
            auto *m = Web::text(meta, "meta", box);
            m->setWordWrap(true);
            bv->addWidget(m);
            box->setAccessibleName(said.join(QStringLiteral("; ")) + QStringLiteral(". ") + meta);
            auto *restore = Web::button(tr("Bring back"), Web::Small, box);
            restore->setObjectName(QStringLiteral("bringBack:%1").arg(id));
            if (may("edit"))
                bv->addWidget(restore, 0, Qt::AlignLeft);
            else
                restore->hide();
            connect(restore, &QPushButton::clicked, this, [this, id] {
                Connection *c = conn();
                if (!c)
                    return;
                onResult(c->api()->post(path(QStringLiteral("/deleted/") + ApiClient::seg(id) + QStringLiteral("/restore")), QJsonObject{}),
                         this, [this](const ApiResult &r) {
                             if (!r.ok()) {
                                 notify(tr("The entry was not brought back: %1").arg(r.error.message));
                                 return;
                             }
                             m_entries << r.object();
                             m_selected = str(r.object(), "id");
                             const QString head = TermData::head(r.object(), languages().value(0));
                             notify(head.isEmpty() ? tr("Brought the entry back") : tr("Brought back “%1”").arg(head));
                             showHead();
                             showEntries();
                             showLists();
                             showDeleted();
                             emit saved();
                         });
            });
            list->addWidget(box);
        }
        emit deletedShown();
    });
}

void TerminologyPage::selectMasterEntry(const QString &master, const QString &id)
{
    m_masterSelected = master + QLatin1Char('\n') + id;
    if (m_view == 3) {
        showMaster();
        showDetail();
    }
}

void TerminologyPage::showMaster()
{
    clear(m_masterBody);
    QWidget *host = m_masterBody->parentWidget();
    auto *holder = new QWidget(host);
    auto *list = new QVBoxLayout(holder);
    list->setContentsMargins(0, 0, 0, 0);
    list->setSpacing(0);
    m_masterBody->addWidget(holder);
    auto *intro = new QFrame(holder);
    Theme::role(intro, "paneBar");
    auto *il = new QVBoxLayout(intro);
    il->setContentsMargins(S3, S2, S3, S2);
    auto *text = Web::text(may("edit") ? tr("The master's entries this project uses as they are. Change one for this project only, or leave it out.") : tr("The master's entries this project uses as they are."),
                           "help", intro);
    text->setWordWrap(true);
    il->addWidget(text);
    list->addWidget(intro);

    const QStringList langs = languages();
    const auto line = [&langs](const QJsonObject &e) {
        QStringList parts;
        for (const QString &l : langs) {
            QStringList ts;
            for (const QJsonValue &t : TermData::terms(e, l))
                ts << str(t.toObject(), "text");
            if (!ts.isEmpty())
                parts << ts.join(QStringLiteral(", "));
        }
        return parts;
    };
    if (m_masterEntries.isEmpty()) {
        auto *none = Web::text(tr("The masters have no entries the project does not have itself."), "muted", holder);
        none->setObjectName(QStringLiteral("noMasterEntries"));
        none->setWordWrap(true);
        none->setContentsMargins(S4, S3, S4, S3);
        list->addWidget(none);
    }
    for (const QJsonObject &e : std::as_const(m_masterEntries)) {
        const QString key = str(e, "termbase") + QLatin1Char('\n') + str(e, "id");
        if (m_masterSelected.isEmpty())
            m_masterSelected = key;
        auto *row = new Web::Clickable("tbLink", holder);
        row->setObjectName(QStringLiteral("master:%1:%2").arg(str(e, "termbase"), str(e, "id")));
        row->setCurrent(key == m_masterSelected);
        auto *rv = new QVBoxLayout(row);
        rv->setContentsMargins(S4, 10, S4, 10);
        rv->setSpacing(3);
        const QStringList parts = line(e);
        auto *head = Web::text(parts.value(0, tr("No term")), "entryTerm", row);
        head->setWordWrap(true);
        rv->addWidget(head);
        if (parts.size() > 1) {
            auto *rest = Web::text(parts.mid(1).join(QStringLiteral(" · ")), "small2", row);
            rest->setWordWrap(true);
            rv->addWidget(rest);
        }
        rv->addWidget(Web::text(tr("In %1").arg(str(e, "termbase")), "tbCount", row));
        row->setAccessibleName(tr("%1, in %2").arg(parts.join(QStringLiteral("; ")), str(e, "termbase")));
        const QString master = str(e, "termbase"), id = str(e, "id");
        connect(row, &Web::Clickable::clicked, this, [this, master, id] { selectMasterEntry(master, id); });
        list->addWidget(row);
        auto *sep = new QFrame(holder);
        Theme::role(sep, "line");
        sep->setFixedHeight(1);
        list->addWidget(sep);
    }

    // What was left out of the project, to bring back.
    Connection *c = conn();
    if (!c)
        return;
    const int reading = m_reading;
    QPointer<QWidget> alive(holder);
    onResult(c->api()->get(path(QStringLiteral("/removals"))), this, [this, alive, list, reading](const ApiResult &r) {
        if (!alive || reading != m_reading)
            return;
        QWidget *host = alive;
        m_removals = r.array();
        if (!m_removals.isEmpty()) {
            auto *title = Web::text(tr("Left out of this project"), "groupTitle", host);
            title->setContentsMargins(S4, S4, S4, S1);
            list->addWidget(title);
            auto *note = Web::text(tr("Merging asks the master's reviewers to delete these there."), "meta", host);
            note->setWordWrap(true);
            note->setContentsMargins(S4, 0, S4, S2);
            list->addWidget(note);
        }
        for (const QJsonValue &v : std::as_const(m_removals)) {
            const QJsonObject rem = v.toObject();
            const QString master = str(rem, "termbase"), id = str(rem, "entryId");
            auto *box = new QWidget(host);
            box->setProperty("part", QStringLiteral("removal"));
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(S4, S2, S4, S2);
            bv->setSpacing(S1);
            auto *label = Web::text(id, "entryTerm", box);
            label->setWordWrap(true);
            bv->addWidget(label);
            bv->addWidget(Web::text(str(rem, "by").isEmpty() ? tr("In %1, left out %2").arg(master, when(str(rem, "at"))) : tr("In %1, left out %2 by %3").arg(master, when(str(rem, "at")), str(rem, "by")),
                                    "meta", box));
            auto *back = Web::button(tr("Bring back"), Web::Small, box);
            back->setObjectName(QStringLiteral("restoreMaster:%1:%2").arg(master, id));
            if (may("edit"))
                bv->addWidget(back, 0, Qt::AlignLeft);
            else
                back->hide();
            list->addWidget(box);
            // The entry is named by its terms, as the master has it.
            if (Connection *c = conn()) {
                QPointer<QLabel> named(label);
                onResult(c->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(master) + QStringLiteral("/entries/") + ApiClient::seg(id)), this, [this, named](const ApiResult &er) {
                    if (named && er.ok()) {
                        const QString head = TermData::head(er.object(), languages().value(0));
                        if (!head.isEmpty())
                            named->setText(head);
                    }
                });
            }
            connect(back, &QPushButton::clicked, this, [this, master, id] {
                Connection *c = conn();
                if (!c)
                    return;
                QUrlQuery q;
                q.addQueryItem(QStringLiteral("master"), QString::fromUtf8(QUrl::toPercentEncoding(master)));
                q.addQueryItem(QStringLiteral("entry"), QString::fromUtf8(QUrl::toPercentEncoding(id)));
                onResult(c->api()->del(path(QStringLiteral("/removals")), q), this, [this](const ApiResult &r) {
                    if (!r.ok()) {
                        notify(r.error.message);
                        return;
                    }
                    notify(tr("Brought the entry back into the project"));
                    reload();
                });
            });
        }
        emit masterShown();
    });
}

void TerminologyPage::showMasterDetail()
{
    QWidget *host = m_detailHolder->parentWidget();
    QJsonObject e;
    for (const QJsonObject &m : std::as_const(m_masterEntries)) {
        if (str(m, "termbase") + QLatin1Char('\n') + str(m, "id") == m_masterSelected)
            e = m;
    }
    if (e.isEmpty()) {
        m_detailHolder->addStretch();
        return;
    }
    const QString master = str(e, "termbase"), id = str(e, "id");
    const QString head = TermData::head(e, languages().value(0));

    auto *headFrame = new QFrame(host);
    Theme::role(headFrame, "paneHead");
    auto *hv = new QVBoxLayout(headFrame);
    hv->setContentsMargins(S4, S3, S4, S3);
    hv->setSpacing(S1);
    auto *title = Web::text(head.isEmpty() ? tr("Entry") : head, "paneTitle", headFrame);
    title->setObjectName(QStringLiteral("masterEntryTitle"));
    title->setWordWrap(true);
    hv->addWidget(title);
    const QString changed = when(str(e, "changedAt").isEmpty() ? str(e, "createdAt") : str(e, "changedAt"));
    hv->addWidget(Web::text(changed.isEmpty() ? tr("In %1").arg(master) : tr("In %1, last changed %2").arg(master, changed), "meta", headFrame));
    m_detailHolder->addWidget(headFrame);

    QVBoxLayout *body = nullptr;
    QScrollArea *scroll = scroller(host, &body, S4, S4);
    m_detailHolder->addWidget(scroll, 1);
    QWidget *inner = scroll->widget();

    if (!may("edit")) {
        readEntry(e, body, inner);
        body->addStretch();
        return;
    }
    auto *note = Web::note(tr("This is the master's entry. To change it, take it into the project: your version is used here and goes to the master's reviewers when you merge."),
                           QStringLiteral("info"), inner);
    body->addWidget(note);
    auto *change = Web::button(tr("Change in this project"), Web::Small, inner, QStringLiteral("pencil"));
    change->setObjectName(QStringLiteral("changeInProject"));
    auto *leave = new Web::ConfirmButton(tr("Leave out of this project"), tr("Leave this entry out of the project? Merging asks the master's reviewers to delete it there."), tr("Leave out"), inner);
    leave->setObjectName(QStringLiteral("leaveOut"));
    body->addWidget(Web::row({change, leave}, inner));
    const QJsonObject which{{QLatin1String("master"), master}, {QLatin1String("entry"), id}};
    connect(change, &QPushButton::clicked, this, [this, which] {
        Connection *c = conn();
        if (!c)
            return;
        onResult(c->api()->post(path(QStringLiteral("/overrides")), which), this, [this](const ApiResult &r) {
            if (!r.ok()) {
                notify(r.error.message);
                return;
            }
            // The project's own copy is opened to be changed.
            notify(tr("Took the entry into the project"));
            m_wantEntry = str(r.object(), "id");
            m_view = 0;
            m_tabs->setCurrent(0);
            m_views->setCurrentIndex(0);
            m_detailTab = 0;
            reload();
        });
    });
    connect(leave, &Web::ConfirmButton::confirmed, this, [this, which] {
        Connection *c = conn();
        if (!c)
            return;
        onResult(c->api()->post(path(QStringLiteral("/removals")), which), this, [this](const ApiResult &r) {
            if (!r.ok()) {
                notify(r.error.message);
                return;
            }
            notify(tr("Left the entry out of the project"));
            m_masterSelected.clear();
            reload();
        });
    });

    readEntry(e, body, inner);
    body->addStretch();
}

// The entry as it is, to read: its fields, languages and terms.
void TerminologyPage::readEntry(const QJsonObject &e, QVBoxLayout *body, QWidget *inner)
{
    const auto values = [&](const QJsonArray &fs, QVBoxLayout *into, QWidget *parent) {
        for (const QJsonValue &v : fs) {
            auto *l = new QLabel(QStringLiteral("<span style=\"color:%1\">%2:</span> %3").arg(Theme::color("text2").name(), str(v.toObject(), "field").toHtmlEscaped(), str(v.toObject(), "value").toHtmlEscaped()), parent);
            l->setWordWrap(true);
            l->setTextInteractionFlags(Qt::TextSelectableByMouse);
            into->addWidget(l);
        }
    };
    values(e.value(QLatin1String("fields")).toArray(), body, inner);
    for (const QJsonValue &lv : e.value(QLatin1String("languages")).toArray()) {
        const QJsonObject l = lv.toObject();
        const QString code = str(l, "lang");
        auto *sep = new QFrame(inner);
        Theme::role(sep, "line");
        sep->setFixedHeight(1);
        body->addWidget(sep);
        auto *tr1 = new QHBoxLayout;
        tr1->setSpacing(S2);
        tr1->addWidget(Web::text(Languages::name(code), "langTitle", inner));
        tr1->addWidget(Web::langChip(code, inner));
        tr1->addStretch();
        body->addLayout(tr1);
        values(l.value(QLatin1String("fields")).toArray(), body, inner);
        for (const QJsonValue &tv : l.value(QLatin1String("terms")).toArray()) {
            const QJsonObject t = tv.toObject();
            const QString status = str(t, "status").isEmpty() ? QStringLiteral("proposed") : str(t, "status");
            auto *box = new QFrame(inner);
            Theme::role(box, "term");
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(S3, S3, S3, S3);
            bv->setSpacing(S1);
            auto *tl = new QHBoxLayout;
            tl->setSpacing(S2);
            auto *text = Web::text(str(t, "text"), "termText", box);
            text->setWordWrap(true);
            text->setTextInteractionFlags(Qt::TextSelectableByMouse);
            if (Languages::rtl(code))
                text->setLayoutDirection(Qt::RightToLeft);
            tl->addWidget(text, 1);
            tl->addWidget(Web::chip(TermData::statusLabel(status), status, box));
            bv->addLayout(tl);
            values(t.value(QLatin1String("fields")).toArray(), bv, box);
            body->addWidget(box);
        }
    }
}
