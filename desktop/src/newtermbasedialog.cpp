#include "newtermbasedialog.h"

#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "languages.h"
#include "theme.h"

#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QJsonObject>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QRadioButton>
#include <QTimer>
#include <QVBoxLayout>

#include <memory>

namespace {

constexpr int S1 = 4, S2 = 8, S3 = 12, S4 = 16;

QString str(const QJsonObject &o, const char *key)
{
    return o.value(QLatin1String(key)).toString();
}

void clear(QLayout *layout)
{
    while (QLayoutItem *item = layout->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        } else if (QLayout *l = item->layout()) {
            clear(l);
            delete l;
            continue;
        }
        delete item;
    }
}

QLabel *bold(const QString &text, QWidget *parent)
{
    return new QLabel(QStringLiteral("<b>%1</b>").arg(text.toHtmlEscaped()), parent);
}

QLabel *help(const QString &text, QWidget *parent)
{
    auto *l = Web::text(text, "help", parent);
    l->setWordWrap(true);
    return l;
}

QString languageNames(const QStringList &tags)
{
    QStringList names;
    for (const QString &t : tags)
        names << Languages::name(t);
    return names.join(QStringLiteral(", "));
}

} // namespace

NewTermbaseDialog::NewTermbaseDialog(ConnectionManager *manager, const QHash<QString, QJsonArray> &lists, Kind kind, QWidget *parent)
    : Web::Dialog(tr("New termbase"), parent, true)
    , m_manager(manager)
    , m_lists(lists)
    , m_kind(kind)
{
    setObjectName(QStringLiteral("newTermbaseDialog"));
    m_stepper = new Web::Stepper(this);
    m_stepper->setClickable(true);
    body()->addWidget(m_stepper);
    connect(m_stepper, &Web::Stepper::stepClicked, this, [this](int i) {
        if (i < m_at) {
            m_at = i;
            render();
        }
    });
    // The steps go in the dialog's body, which scrolls when a long list of
    // fields makes it taller than the screen.
    m_content = new QVBoxLayout;
    m_content->setContentsMargins(0, 0, 0, 0);
    m_content->setSpacing(S3);
    body()->addLayout(m_content, 1);
    m_hint = help(QString(), this);
    m_hint->setObjectName(QStringLiteral("newTermbaseHint"));
    body()->addWidget(m_hint);

    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, this);
    m_back = Web::button(tr("Back"), Web::Small, this);
    m_next = Web::button(tr("Continue"), Web::Primary | Web::Small, this);
    m_next->setObjectName(QStringLiteral("newTermbaseNext"));
    buttons()->addWidget(cancel);
    buttons()->addWidget(m_back);
    buttons()->addWidget(m_next);
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    connect(m_back, &QPushButton::clicked, this, &NewTermbaseDialog::back);
    connect(m_next, &QPushButton::clicked, this, &NewTermbaseDialog::next);
    // A project termbase needs a master in its place: start where there is one.
    if (m_kind == Project) {
        const QList<Connection *> all = places();
        for (int i = 0; i < all.size(); i++) {
            m_place = i;
            if (!masters().isEmpty())
                break;
        }
        if (masters().isEmpty())
            m_place = 0;
    }
    render();
}

void NewTermbaseDialog::request(ApiReply *reply, std::function<void(const ApiResult &)> then)
{
    m_pending++;
    onResult(reply, this, [this, then](const ApiResult &r) {
        m_pending--;
        then(r);
        if (m_pending == 0)
            emit ready();
    });
}

QList<Connection *> NewTermbaseDialog::places() const
{
    QList<Connection *> out;
    if (m_manager->local() && m_manager->local()->isReady())
        out << m_manager->local();
    for (Connection *c : m_manager->teams()) {
        if (c->isReady())
            out << c;
    }
    return out;
}

Connection *NewTermbaseDialog::place() const
{
    return places().value(m_place);
}

QList<QJsonObject> NewTermbaseDialog::masters() const
{
    QList<QJsonObject> out;
    if (Connection *c = place()) {
        for (const QJsonValue &v : m_lists.value(c->id())) {
            if (str(v.toObject(), "kind") == QLatin1String("master"))
                out << v.toObject();
        }
    }
    return out;
}

bool NewTermbaseDialog::taken(const QString &name) const
{
    if (Connection *c = place()) {
        for (const QJsonValue &v : m_lists.value(c->id())) {
            if (str(v.toObject(), "name").compare(name.trimmed(), Qt::CaseInsensitive) == 0)
                return true;
        }
    }
    return false;
}

void NewTermbaseDialog::setKind(Kind kind)
{
    if (kind == m_kind)
        return;
    m_kind = kind;
    m_at = 0;
    m_inheritedLanguages.clear();
    m_inheritedFields.clear();
    if (kind == Project)
        loadInherited();
    render();
}

void NewTermbaseDialog::setName(const QString &name)
{
    m_name = name;
    if (step() == Start) {
        if (auto *edit = findChild<QLineEdit *>(QStringLiteral("newTermbaseName")); edit && edit->text() != name)
            edit->setText(name);
    }
    updateButtons();
}

void NewTermbaseDialog::setPlace(int index)
{
    if (index == m_place || index < 0 || index >= places().size())
        return;
    m_place = index;
    // The masters to base it on, and to take fields from, are those of the place.
    m_based.clear();
    m_inheritedLanguages.clear();
    m_inheritedFields.clear();
    if (m_start.startsWith(QLatin1String("tb:")))
        chooseStart(QStringLiteral("terms"));
    render();
}

void NewTermbaseDialog::setBasedOn(const QStringList &masters)
{
    m_based = masters;
    loadInherited();
    render();
}

void NewTermbaseDialog::setLanguages(const QStringList &tags)
{
    m_langs = tags;
    render();
}

void NewTermbaseDialog::setFields(const QList<TermFields::Draft> &fields)
{
    m_fields = fields;
    render();
}

void NewTermbaseDialog::chooseStart(const QString &id)
{
    m_start = id;
    setError(QString());
    for (const TermFields::Structure &s : TermFields::structures()) {
        if (s.id == id) {
            m_fields = s.fields;
            render();
            return;
        }
    }
    if (!id.startsWith(QLatin1String("tb:")))
        return;
    Connection *c = place();
    if (!c)
        return;
    const int asked = ++m_asked;
    request(c->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(id.mid(3))), [this, asked](const ApiResult &r) {
        if (asked != m_asked)
            return;
        if (!r.ok()) {
            setError(r.error.message);
            return;
        }
        m_fields.clear();
        for (const QJsonValue &f : r.object().value(QLatin1String("definition")).toObject().value(QLatin1String("fields")).toArray())
            m_fields << TermFields::fromJson(f.toObject());
        render();
    });
    render();
}

void NewTermbaseDialog::loadInherited()
{
    m_inheritedLanguages.clear();
    m_inheritedFields.clear();
    Connection *c = place();
    if (m_kind != Project || !c || m_based.isEmpty())
        return;
    const int asked = ++m_asked;
    auto got = std::make_shared<QHash<QString, QJsonObject>>();
    const QStringList based = m_based;
    for (const QString &m : based) {
        request(c->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(m)), [this, asked, got, based, m](const ApiResult &r) {
            if (asked != m_asked)
                return;
            if (!r.ok()) {
                setError(r.error.message);
                return;
            }
            got->insert(m, r.object().value(QLatin1String("definition")).toObject());
            if (got->size() < based.size())
                return;
            // In the order the masters were chosen: the first comes first.
            for (const QString &name : based) {
                const QJsonObject d = got->value(name);
                for (const QJsonValue &l : d.value(QLatin1String("languages")).toArray()) {
                    if (!m_inheritedLanguages.contains(l.toString()))
                        m_inheritedLanguages << l.toString();
                }
                for (const QJsonValue &f : d.value(QLatin1String("fields")).toArray()) {
                    const TermFields::Draft field = TermFields::fromJson(f.toObject());
                    bool have = false;
                    for (const TermFields::Draft &i : m_inheritedFields)
                        have = have || i.name.compare(field.name, Qt::CaseInsensitive) == 0;
                    if (!have)
                        m_inheritedFields << field;
                }
            }
            render();
        });
    }
}

QString NewTermbaseDialog::blocking() const
{
    switch (step()) {
    case Start:
        if (m_name.trimmed().isEmpty())
            return tr("Give the termbase a name.");
        if (taken(m_name))
            return tr("A termbase with this name is there already.");
        if (m_kind == Project && m_based.isEmpty())
            return tr("Choose the masters it is based on.");
        return QString();
    case LanguagesStep:
        return m_langs.isEmpty() ? tr("Choose its languages, at least one.") : QString();
    case Fields: {
        QStringList inherited;
        if (m_kind == Project) {
            for (const TermFields::Draft &f : m_inheritedFields)
                inherited << f.name;
        }
        for (int i = 0; i < m_fields.size(); i++) {
            QStringList others = inherited;
            for (int j = 0; j < m_fields.size(); j++) {
                if (j != i)
                    others << m_fields[j].name;
            }
            const QString p = TermFields::problem(m_fields[i], others);
            if (!p.isEmpty())
                return p;
        }
        return QString();
    }
    case Review:
        return QString();
    }
    return QString();
}

void NewTermbaseDialog::updateButtons()
{
    const QString b = blocking();
    m_back->setVisible(m_at > 0);
    const bool last = step() == Review;
    m_next->setText(!last ? tr("Continue") : m_kind == Project ? tr("Create project termbase") : tr("Create master"));
    m_next->setEnabled(last ? !busy() : b.isEmpty());
    m_next->setToolTip(b);
    // As the web: a step's problem is said once there is something to say
    // it about; the fields' problems are also shown at the fields.
    const bool say = !b.isEmpty() && (step() == LanguagesStep || step() == Fields || !m_name.trimmed().isEmpty());
    m_hint->setText(say ? b : QString());
    m_hint->setVisible(say);
}

void NewTermbaseDialog::next()
{
    if (step() == Review) {
        create();
        return;
    }
    if (!blocking().isEmpty())
        return;
    m_at++;
    setError(QString());
    render();
}

void NewTermbaseDialog::back()
{
    if (m_at == 0)
        return;
    m_at--;
    setError(QString());
    render();
}

void NewTermbaseDialog::render()
{
    m_steps = m_kind == Master ? QList<Step>{Start, LanguagesStep, Fields, Review} : QList<Step>{Start, Fields, Review};
    m_at = qBound(0, m_at, int(m_steps.size()) - 1);
    QStringList names;
    for (Step s : m_steps)
        names << (s == Start ? tr("Start") : s == LanguagesStep ? tr("Languages") : s == Fields ? tr("Fields") : tr("Review"));
    m_stepper->setSteps(names, m_at);
    clear(m_content);
    m_table = nullptr;
    switch (step()) {
    case Start:
        showStart();
        break;
    case LanguagesStep:
        showLanguages();
        break;
    case Fields:
        showFields();
        break;
    case Review:
        showReview();
        break;
    }
    updateButtons();
}

void NewTermbaseDialog::showStart()
{
    QWidget *host = m_content->parentWidget();
    auto *nameLabel = bold(tr("Name"), host);
    auto *name = new QLineEdit(m_name, host);
    name->setObjectName(QStringLiteral("newTermbaseName"));
    name->setAccessibleName(tr("Name"));
    name->setPlaceholderText(m_kind == Project ? tr("For example a case or an order") : tr("For example a client or a subject"));
    nameLabel->setBuddy(name);
    m_content->addWidget(nameLabel);
    m_content->addWidget(name);
    connect(name, &QLineEdit::textChanged, this, [this](const QString &t) {
        m_name = t;
        updateButtons();
    });
    connect(name, &QLineEdit::returnPressed, this, &NewTermbaseDialog::next);
    QTimer::singleShot(0, name, [name] { name->setFocus(); });

    // Where it is kept: this computer, or a team server signed in to.
    const QList<Connection *> all = places();
    if (all.size() > 1) {
        m_content->addWidget(bold(tr("Where it is kept"), host));
        auto *group = new QButtonGroup(host);
        const int teams = int(all.size()) - (all.first()->isLocal() ? 1 : 0);
        for (int i = 0; i < all.size(); i++) {
            Connection *c = all[i];
            auto *r = new QRadioButton(host);
            r->setObjectName(QStringLiteral("newTermbasePlace:%1").arg(i));
            r->setChecked(i == m_place);
            group->addButton(r, i);
            const QString text = c->isLocal() ? tr("On this computer")
                                 : teams > 1 ? tr("On %1, for everyone working with it").arg(c->displayName())
                                             : tr("On the team server, for everyone working with it");
            m_content->addWidget(TermFields::checkText(r, text, host));
            connect(r, &QRadioButton::toggled, this, [this, i](bool on) {
                if (on)
                    QTimer::singleShot(0, this, [this, i] { setPlace(i); });
            });
        }
    }

    m_content->addWidget(bold(tr("What kind of termbase"), host));
    auto *kinds = new QButtonGroup(host);
    auto *master = new QRadioButton(host);
    master->setObjectName(QStringLiteral("newTermbaseKind:master"));
    master->setChecked(m_kind == Master);
    auto *project = new QRadioButton(host);
    project->setObjectName(QStringLiteral("newTermbaseKind:project"));
    project->setChecked(m_kind == Project);
    kinds->addButton(master);
    kinds->addButton(project);
    m_content->addWidget(TermFields::checkText(
        master, tr("A master. A termbase of its own, kept across projects, for a client, a subject or a company. You choose its languages and fields."), host, true));
    m_content->addWidget(TermFields::checkText(
        project, tr("A project termbase. Based on masters: it shows their entries, gets their languages and fields, and sends its new and changed entries to them for review."), host,
        true));
    connect(master, &QRadioButton::toggled, this, [this](bool on) {
        if (on)
            QTimer::singleShot(0, this, [this] { setKind(Master); });
    });
    connect(project, &QRadioButton::toggled, this, [this](bool on) {
        if (on)
            QTimer::singleShot(0, this, [this] { setKind(Project); });
    });

    if (m_kind == Project) {
        m_content->addWidget(bold(tr("Based on"), host));
        const QList<QJsonObject> ms = masters();
        if (ms.isEmpty())
            m_content->addWidget(help(tr("There is no master here yet. Make one first."), host));
        for (const QJsonObject &m : ms) {
            const QString n = str(m, "name");
            QStringList langs;
            for (const QJsonValue &l : m.value(QLatin1String("languages")).toArray())
                langs << l.toString();
            auto *box = new QCheckBox(host);
            box->setObjectName(QStringLiteral("basedOn:%1").arg(n));
            box->setChecked(m_based.contains(n));
            auto *row = new QWidget(host);
            auto *h = new QHBoxLayout(row);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(S2);
            QWidget *check = TermFields::checkText(box, n, row);
            check->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Preferred);
            h->addWidget(check);
            if (!langs.isEmpty())
                h->addWidget(Web::text(langs.join(QStringLiteral(", ")), "muted", row));
            h->addStretch();
            box->setAccessibleName(n);
            m_content->addWidget(row);
            connect(box, &QCheckBox::toggled, this, [this, n](bool on) {
                QStringList b = m_based;
                b.removeAll(n);
                if (on)
                    b << n;
                QTimer::singleShot(0, this, [this, b] { setBasedOn(b); });
            });
        }
        m_content->addWidget(help(tr("The first master chosen comes first where masters have the same entry."), host));
    }
    m_content->addStretch();
}

void NewTermbaseDialog::showLanguages()
{
    QWidget *host = m_content->parentWidget();
    m_content->addWidget(Web::text(tr("The languages of %1").arg(m_name.trimmed()), "fieldLabel", host));
    auto *list = new TermFields::LanguageChecklist(tr("Languages of the termbase"), host);
    list->setObjectName(QStringLiteral("newTermbaseLanguages"));
    list->setChosen(m_langs);
    m_content->addWidget(list);
    connect(list, &TermFields::LanguageChecklist::changed, this, [this](const QStringList &tags) {
        m_langs = tags;
        updateButtons();
    });
    m_content->addWidget(help(tr("A termbase has no source and target: it has languages, and a project looks up terms in its own. More languages can be added later."), host));
    QTimer::singleShot(0, list->search(), [s = list->search()] { s->setFocus(); });
}

void NewTermbaseDialog::showFields()
{
    QWidget *host = m_content->parentWidget();
    if (m_kind == Master) {
        m_content->addWidget(bold(tr("Start from"), host));
        auto *group = new QButtonGroup(host);
        for (const TermFields::Structure &s : TermFields::structures()) {
            auto *r = new QRadioButton(host);
            r->setObjectName(QStringLiteral("newTermbaseStart:%1").arg(s.id));
            r->setChecked(m_start == s.id);
            group->addButton(r);
            m_content->addWidget(TermFields::checkText(r, QStringLiteral("%1. %2").arg(s.name, s.text), host, true));
            const QString id = s.id;
            connect(r, &QRadioButton::clicked, this, [this, id] { QTimer::singleShot(0, this, [this, id] { chooseStart(id); }); });
        }
        const QList<QJsonObject> ms = masters();
        if (!ms.isEmpty()) {
            auto *row = new QWidget(host);
            auto *h = new QHBoxLayout(row);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(S2);
            auto *r = new QRadioButton(tr("The fields of"), row);
            r->setObjectName(QStringLiteral("newTermbaseStart:tb"));
            r->setChecked(m_start.startsWith(QLatin1String("tb:")));
            group->addButton(r);
            auto *from = new QComboBox(row);
            from->setObjectName(QStringLiteral("newTermbaseFrom"));
            from->setAccessibleName(tr("The termbase to take the fields from"));
            from->addItem(tr("a termbase…"), QString());
            for (const QJsonObject &m : ms)
                from->addItem(str(m, "name"), QStringLiteral("tb:") + str(m, "name"));
            from->setCurrentIndex(qMax(0, from->findData(m_start)));
            from->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
            h->addWidget(r);
            h->addWidget(from);
            h->addStretch();
            m_content->addWidget(row);
            connect(r, &QRadioButton::clicked, this, [this, from] {
                const QString id = from->currentData().toString().isEmpty() ? from->itemData(1).toString() : from->currentData().toString();
                QTimer::singleShot(0, this, [this, id] { chooseStart(id); });
            });
            connect(from, &QComboBox::activated, this, [this, from] {
                const QString id = from->currentData().toString();
                if (!id.isEmpty())
                    QTimer::singleShot(0, this, [this, id] { chooseStart(id); });
            });
        }
        m_content->addWidget(help(tr("Choosing a start sets the fields below; change them as you need. Every term has a status (approved, proposed or forbidden) without a field for it."), host));
    } else {
        m_content->addWidget(Web::text(tr("From %1").arg(m_based.join(tr(" and "))), "fieldLabel", host));
        QString got;
        if (!m_inheritedLanguages.isEmpty())
            got = tr("Languages: %1. ").arg(languageNames(m_inheritedLanguages));
        if (!m_inheritedFields.isEmpty()) {
            QStringList lines;
            for (const TermFields::Draft &f : m_inheritedFields)
                lines << TermFields::describe(f);
            got += tr("Fields: %1.").arg(lines.join(QStringLiteral("; ")));
        } else {
            got += tr("The masters have no fields.");
        }
        auto *inherited = help(busy() ? tr("Loading…") : got, host);
        inherited->setObjectName(QStringLiteral("newTermbaseInherited"));
        m_content->addWidget(inherited);
        m_content->addWidget(Web::text(tr("Fields of its own"), "fieldLabel", host));
        m_content->addWidget(help(tr("Only if the project needs more than the masters have. A field of its own reaches a master with the first entry that uses it, once a reviewer approves the entry."), host));
    }
    auto *table = new TermFields::FieldTable(host);
    table->setObjectName(QStringLiteral("newTermbaseFields"));
    QStringList inherited;
    if (m_kind == Project) {
        for (const TermFields::Draft &f : m_inheritedFields)
            inherited << f.name;
    }
    table->setTaken(inherited);
    table->setFields(m_fields);
    connect(table, &TermFields::FieldTable::changed, this, [this, table] {
        m_fields = table->fields();
        updateButtons();
    });
    m_table = table;
    m_content->addWidget(table);
    m_content->addStretch();
}

void NewTermbaseDialog::showReview()
{
    QWidget *host = m_content->parentWidget();
    auto *grid = new QWidget(host);
    grid->setObjectName(QStringLiteral("newTermbaseReview"));
    auto *g = new QGridLayout(grid);
    g->setContentsMargins(0, 0, 0, 0);
    g->setHorizontalSpacing(S4);
    g->setVerticalSpacing(S2);
    int row = 0;
    const auto add = [&](const QString &term, QWidget *value) {
        auto *t = Web::text(term, "fieldLabel", grid);
        g->addWidget(t, row, 0, Qt::AlignTop);
        g->addWidget(value, row, 1);
        row++;
    };
    const auto plain = [grid](const QString &text) {
        auto *l = new QLabel(text, grid);
        l->setWordWrap(true);
        return l;
    };
    add(tr("Name"), plain(m_name.trimmed()));
    if (places().size() > 1) {
        Connection *c = place();
        const int teams = int(places().size()) - (places().first()->isLocal() ? 1 : 0);
        add(tr("Kept"), plain(!c || c->isLocal() ? tr("On this computer") : teams > 1 ? tr("On %1").arg(c->displayName()) : tr("On the team server")));
    }
    add(tr("Kind"), plain(m_kind == Master ? tr("A master") : tr("A project termbase based on %1").arg(m_based.join(tr(" and ")))));
    const QStringList langs = m_kind == Master ? m_langs : m_inheritedLanguages;
    add(tr("Languages"), plain(langs.isEmpty() ? tr("None yet") : languageNames(langs)));
    QList<TermFields::Draft> all;
    if (m_kind == Project)
        all = m_inheritedFields;
    if (all.isEmpty() && m_fields.isEmpty()) {
        add(tr("Fields"), plain(tr("None: terms and their status")));
    } else {
        auto *list = new QWidget(grid);
        auto *lv = new QVBoxLayout(list);
        lv->setContentsMargins(0, 0, 0, 0);
        lv->setSpacing(2);
        for (const TermFields::Draft &f : all) {
            auto *l = new QLabel(QStringLiteral("%1 <span style=\"color:%2\">%3</span>").arg(TermFields::describe(f).toHtmlEscaped(), Theme::color("text3").name(), tr("from the masters")), list);
            l->setTextFormat(Qt::RichText);
            l->setWordWrap(true);
            lv->addWidget(l);
        }
        for (const TermFields::Draft &f : m_fields) {
            auto *l = new QLabel(TermFields::describe(f), list);
            l->setWordWrap(true);
            lv->addWidget(l);
        }
        add(tr("Fields"), list);
    }
    g->setColumnStretch(1, 1);
    m_content->addWidget(grid);
    m_content->addStretch();
}

void NewTermbaseDialog::create()
{
    Connection *c = place();
    const QString n = m_name.trimmed();
    if (!c || n.isEmpty() || busy())
        return;
    QJsonArray own;
    for (const TermFields::Draft &f : m_fields)
        own.append(TermFields::toJson(f));
    QJsonObject definition{{QLatin1String("name"), n}, {QLatin1String("fields"), own}};
    QJsonObject body;
    if (m_kind == Project) {
        body.insert(QLatin1String("upstreams"), QJsonArray::fromStringList(m_based));
    } else {
        definition.insert(QLatin1String("languages"), QJsonArray::fromStringList(m_langs));
    }
    body.insert(QLatin1String("definition"), definition);
    const QString connId = c->id();
    const bool project = m_kind == Project;
    setError(QString());
    request(c->api()->post(QStringLiteral("/api/termbases"), body), [this, connId, n, project](const ApiResult &r) {
        if (!r.ok()) {
            setError(r.error.message);
            updateButtons();
            return;
        }
        const QString name = str(r.object(), "name").isEmpty() ? n : str(r.object(), "name");
        emit created(connId, name, project ? tr("Created project termbase “%1”").arg(name) : tr("Created master “%1”").arg(name));
        accept();
    });
    updateButtons();
}
