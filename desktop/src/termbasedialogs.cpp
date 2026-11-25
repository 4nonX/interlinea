#include "termbasedialogs.h"

#include "accesspanel.h"
#include "apiclient.h"
#include "connection.h"
#include "connectionmanager.h"
#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "termdata.h"
#include "termfields.h"
#include "theme.h"

#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QJsonDocument>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRadioButton>
#include <QRegularExpression>
#include <QScrollArea>
#include <QUrlQuery>
#include <QVBoxLayout>

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
        }
        delete item;
    }
}

QLabel *bold(const QString &text, QWidget *parent)
{
    return new QLabel(QStringLiteral("<b>%1</b>").arg(text.toHtmlEscaped()), parent);
}

QFrame *line(QWidget *parent)
{
    auto *f = new QFrame(parent);
    Theme::role(f, "line");
    f->setFixedHeight(1);
    return f;
}

QString plural(qint64 n, const QString &one, const QString &many)
{
    return n == 1 ? one : many.arg(QLocale().toString(n));
}

QString levelLabel(const QString &level)
{
    if (level == QLatin1String("language"))
        return QObject::tr("Language");
    if (level == QLatin1String("term"))
        return QObject::tr("Term");
    return QObject::tr("Entry");
}

// The first warnings of a list, and how many more there are.
void addWarnings(QVBoxLayout *into, const QJsonArray &items, QWidget *parent)
{
    for (int i = 0; i < items.size() && i < 6; i++)
        into->addWidget(Web::note(items[i].toString(), QStringLiteral("warn"), parent));
    if (items.size() > 6)
        into->addWidget(Web::text(QObject::tr("and %1 more notes").arg(items.size() - 6), "help", parent));
}

} // namespace

QList<TermFormat> termExportFormats()
{
    return {
        {QStringLiteral("tbx"), QObject::tr("TBX 3"), QStringLiteral(".tbx"), QObject::tr("The ISO standard (30042:2019), read by current terminology tools.")},
        {QStringLiteral("tbx2"), QObject::tr("TBX 2"), QStringLiteral(".tbx"), QObject::tr("The older <martif> form, for tools that do not read TBX 3 yet.")},
        {QStringLiteral("multiterm"), QObject::tr("Termbase XML"), QStringLiteral(".xml"), QObject::tr("The XML exchange format of .sdltb termbases.")},
        {QStringLiteral("xlsx"), QObject::tr("XLSX workbook"), QStringLiteral(".xlsx"),
         QObject::tr("One row per entry, one column per language and field. Synonyms share a cell, separated by “; ”.")},
        {QStringLiteral("csv"), QObject::tr("CSV"), QStringLiteral(".csv"), QObject::tr("Comma-separated, UTF-8. Opens in spreadsheet programs.")},
        {QStringLiteral("tsv"), QObject::tr("Tab-separated text"), QStringLiteral(".tsv"), QObject::tr("UTF-8, one row per entry.")},
    };
}

QString termFormatLabel(const QString &id)
{
    for (const TermFormat &f : termExportFormats()) {
        if (f.id == id)
            return f.label;
    }
    if (id == QLatin1String("sdltb"))
        return QObject::tr(".sdltb termbase");
    if (id == QLatin1String("xdt"))
        return QObject::tr(".xdt termbase definition");
    return id;
}

QString termImportFilter()
{
    return QObject::tr("Termbase files (*.tbx *.tbxm *.xml *.mtf *.sdltb *.xdt *.csv *.tsv *.tab *.txt *.xlsx);;All files (*)");
}

// ---- Export ---------------------------------------------------------------

ExportDialog::ExportDialog(Connection *conn, const QString &termbase, int total, const QStringList &shown, QWidget *parent)
    : Web::Dialog(tr("Export %1").arg(termbase), parent)
    , m_conn(conn)
    , m_termbase(termbase)
    , m_total(total)
    , m_shown(shown)
{
    setObjectName(QStringLiteral("exportDialog"));
    body()->addWidget(bold(tr("Format"), this));
    auto *list = new QVBoxLayout;
    list->setSpacing(S2);
    m_formats = new QButtonGroup(this);
    int i = 0;
    for (const TermFormat &f : termExportFormats()) {
        auto *r = new QRadioButton(f.label, this);
        r->setProperty("format", f.id);
        r->setChecked(i == 0);
        m_formats->addButton(r, i++);
        list->addWidget(Web::choice(r, f.note, this));
    }
    body()->addLayout(list);
    if (m_shown.size() < m_total) {
        body()->addWidget(bold(tr("Entries"), this));
        m_scope = new QButtonGroup(this);
        auto *all = new QRadioButton(tr("All %1").arg(TermData::entries(m_total)), this);
        auto *some = new QRadioButton(m_shown.size() == 1 ? tr("The 1 entry the search shows") : tr("The %1 entries the search shows").arg(QLocale().toString(m_shown.size())), this);
        all->setChecked(true);
        m_scope->addButton(all, 0);
        m_scope->addButton(some, 1);
        body()->addWidget(all);
        body()->addWidget(some);
        connect(m_scope, &QButtonGroup::idClicked, this, [this](int id) { setShownOnly(id == 1); });
    }
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, this);
    m_export = Web::button(QString(), Web::Primary | Web::Small, this);
    buttons()->addWidget(cancel);
    buttons()->addWidget(m_export);
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    connect(m_export, &QPushButton::clicked, this, [this] {
        const QString path = QFileDialog::getSaveFileName(this, title(), fileName());
        if (!path.isEmpty())
            exportTo(path);
    });
    update();
}

QString ExportDialog::format() const
{
    return m_formats->checkedButton()->property("format").toString();
}

void ExportDialog::setFormat(const QString &id)
{
    for (QAbstractButton *b : m_formats->buttons()) {
        if (b->property("format").toString() == id)
            b->setChecked(true);
    }
}

void ExportDialog::setShownOnly(bool only)
{
    m_shownOnly = only && m_scope;
    if (m_scope)
        m_scope->button(m_shownOnly ? 1 : 0)->setChecked(true);
    update();
}

void ExportDialog::update()
{
    const int n = m_shownOnly ? int(m_shown.size()) : m_total;
    m_export->setText(tr("Export %1").arg(TermData::entries(n)));
    m_export->setEnabled(n > 0);
}

QString ExportDialog::fileName() const
{
    QString base = m_termbase;
    base.replace(QRegularExpression(QStringLiteral("[\\\\/:*?\"<>|]+")), QStringLiteral(" "));
    for (const TermFormat &f : termExportFormats()) {
        if (f.id == format())
            return base.trimmed() + f.ext;
    }
    return base;
}

void ExportDialog::exportTo(const QString &path)
{
    if (!m_conn)
        return;
    setError(QString());
    m_export->setEnabled(false);
    const QString route = QStringLiteral("/api/termbases/") + ApiClient::seg(m_termbase) + QStringLiteral("/export");
    QUrlQuery q;
    q.addQueryItem(QStringLiteral("format"), format());
    const int n = m_shownOnly ? int(m_shown.size()) : m_total;
    ApiReply *reply = m_shownOnly ? m_conn->api()->post(route, QJsonObject{{QLatin1String("entries"), QJsonArray::fromStringList(m_shown)}}, q) : m_conn->api()->get(route, q);
    onResult(reply, this, [this, path, n](const ApiResult &r) {
        update();
        if (!r.ok()) {
            setError(tr("The export failed: %1").arg(r.error.message));
            return;
        }
        QFile f(path);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size()) {
            setError(tr("The export failed: %1").arg(f.errorString()));
            return;
        }
        f.close();
        emit exported(tr("Exported %1 to %2").arg(TermData::entries(n), QFileInfo(path).fileName()));
        accept();
    });
}

// ---- Settings -------------------------------------------------------------

TermbaseSettingsDialog::TermbaseSettingsDialog(Connection *conn, const QJsonObject &detail, const QList<QJsonObject> &entries, QWidget *parent)
    : Web::Dialog(tr("Termbase settings"), parent, true)
    , m_conn(conn)
    , m_name(str(detail, "name"))
    , m_detail(detail)
    , m_entries(entries)
{
    setObjectName(QStringLiteral("termbaseSettings"));
    m_nameEdit = new QLineEdit(m_name, this);
    m_nameEdit->setObjectName(QStringLiteral("termbaseName"));
    body()->addWidget(Web::field(tr("Name"), m_nameEdit, this));
    connect(m_nameEdit, &QLineEdit::editingFinished, this, &TermbaseSettingsDialog::rename);

    auto *scroll = new QScrollArea(this);
    Theme::role(scroll, "paneScroll");
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    auto *inner = new QWidget(scroll);
    Theme::role(inner, "paneBody");
    m_content = new QVBoxLayout(inner);
    m_content->setContentsMargins(0, 0, 0, 0);
    m_content->setSpacing(S3);
    scroll->setWidget(inner);
    Web::fitScroll(scroll);
    scroll->setObjectName(QStringLiteral("settingsScroll"));
    scroll->setMinimumHeight(520);
    body()->addWidget(scroll, 1);

    auto *done = Web::button(tr("Done"), Web::Primary | Web::Small, this);
    done->setObjectName(QStringLiteral("done"));
    buttons()->addWidget(done);
    connect(done, &QPushButton::clicked, this, [this] {
        rename();
        accept();
    });
    render();
    if (m_conn) {
        const bool master = str(m_detail, "kind") == QLatin1String("master");
        if (master) {
            request(m_conn->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(m_name) + QStringLiteral("/review-rules")), [this](const ApiResult &r) {
                if (r.ok())
                    m_rules = r.object();
                render();
            });
            // Whom to trust is chosen among the people of a team server.
            if (!m_conn->isLocal()) {
                request(m_conn->api()->get(QStringLiteral("/api/people")), [this](const ApiResult &r) {
                    m_people = r.array();
                    render();
                });
            }
        } else {
            request(m_conn->api()->get(QStringLiteral("/api/termbases")), [this](const ApiResult &r) {
                m_masters.clear();
                for (const QJsonValue &v : r.array()) {
                    if (str(v.toObject(), "kind") == QLatin1String("master"))
                        m_masters << str(v.toObject(), "name");
                }
                render();
            });
        }
    }
    // Which projects use it decides whether it can be deleted.
    if (m_conn) {
        request(m_conn->api()->get(QStringLiteral("/api/projects")), [this](const ApiResult &r) {
            m_usedBy.clear();
            for (const QJsonValue &v : r.array()) {
                const QJsonObject set = v.toObject().value(QLatin1String("settings")).toObject();
                const QJsonObject t = set.value(QLatin1String("terminology")).toObject();
                bool uses = str(t, "writeTo").compare(m_name, Qt::CaseInsensitive) == 0 || str(t, "termbase").compare(m_name, Qt::CaseInsensitive) == 0;
                for (const QJsonValue &m : t.value(QLatin1String("masters")).toArray())
                    uses = uses || m.toString().compare(m_name, Qt::CaseInsensitive) == 0;
                if (uses)
                    m_usedBy << str(set, "name");
            }
            render();
        });
    }
}

void TermbaseSettingsDialog::request(ApiReply *reply, std::function<void(const ApiResult &)> then)
{
    m_pending++;
    onResult(reply, this, [this, then](const ApiResult &r) {
        m_pending--;
        then(r);
        if (m_pending == 0)
            emit shown();
    });
}

int TermbaseSettingsDialog::termsIn(const QString &lang) const
{
    int n = 0;
    for (const QJsonObject &e : m_entries)
        n += int(TermData::terms(e, lang).size());
    return n;
}

int TermbaseSettingsDialog::valuesOf(const QString &field) const
{
    int n = 0;
    const auto has = [&field](const QJsonValue &holder) { return !TermData::value(holder.toObject().value(QLatin1String("fields")).toArray(), field).isEmpty(); };
    for (const QJsonObject &e : m_entries) {
        n += has(e);
        for (const QJsonValue &l : e.value(QLatin1String("languages")).toArray()) {
            n += has(l);
            for (const QJsonValue &t : l.toObject().value(QLatin1String("terms")).toArray())
                n += has(t);
        }
    }
    return n;
}

void TermbaseSettingsDialog::rename()
{
    const QString v = m_nameEdit->text().trimmed();
    if (v.isEmpty() || v == m_name || !m_conn) {
        m_nameEdit->setText(m_name);
        return;
    }
    const QString old = m_name;
    m_name = v;
    request(m_conn->api()->put(QStringLiteral("/api/termbases/") + ApiClient::seg(old) + QStringLiteral("/name"), QJsonObject{{QLatin1String("name"), v}}), [this, old, v](const ApiResult &r) {
        if (!r.ok()) {
            m_name = old;
            m_nameEdit->setText(old);
            setError(r.error.message);
            return;
        }
        setError(QString());
        m_detail = r.object();
        emit said(tr("Renamed to “%1”").arg(v));
        emit changed(m_name);
        // Who may use it is kept under the new name.
        if (m_access) {
            m_access->deleteLater();
            m_access = nullptr;
        }
        render();
    });
}

void TermbaseSettingsDialog::refresh()
{
    if (!m_conn)
        return;
    const QString base = QStringLiteral("/api/termbases/") + ApiClient::seg(m_name);
    request(m_conn->api()->get(base), [this, base](const ApiResult &r) {
        if (r.ok())
            m_detail = r.object();
        auto *all = new QList<QJsonObject>;
        auto *page = new std::function<void(int)>;
        *page = [this, base, all, page](int offset) {
            QUrlQuery q;
            q.addQueryItem(QStringLiteral("limit"), QStringLiteral("1000"));
            q.addQueryItem(QStringLiteral("offset"), QString::number(offset));
            request(m_conn->api()->get(base + QStringLiteral("/entries"), q), [this, all, page, offset](const ApiResult &er) {
                const QJsonArray list = er.object().value(QLatin1String("entries")).toArray();
                for (const QJsonValue &e : list)
                    all->append(e.toObject());
                if (er.ok() && !list.isEmpty() && offset + list.size() < er.object().value(QLatin1String("total")).toInt()) {
                    (*page)(offset + int(list.size()));
                    return;
                }
                m_entries = *all;
                delete all;
                delete page;
                render();
            });
        };
        (*page)(0);
    });
}

void TermbaseSettingsDialog::putDefinition(const QJsonObject &definition, const QString &done)
{
    if (!m_conn)
        return;
    request(m_conn->api()->put(QStringLiteral("/api/termbases/") + ApiClient::seg(m_name) + QStringLiteral("/definition"), definition), [this, done](const ApiResult &r) {
        if (!r.ok()) {
            setError(r.error.message);
            render();
            return;
        }
        setError(QString());
        m_detail = r.object();
        if (!done.isEmpty())
            emit said(done);
        emit changed(m_name);
        render();
    });
}

void TermbaseSettingsDialog::putRules(const QJsonObject &rules, const QString &done)
{
    if (!m_conn)
        return;
    request(m_conn->api()->put(QStringLiteral("/api/termbases/") + ApiClient::seg(m_name) + QStringLiteral("/review-rules"), rules), [this, done](const ApiResult &r) {
        if (!r.ok()) {
            setError(r.error.message);
            render();
            return;
        }
        setError(QString());
        m_rules = r.object();
        if (!done.isEmpty())
            emit said(done);
        render();
    });
}

// How what projects submit to this master is reviewed.
void TermbaseSettingsDialog::reviewSection(QWidget *host)
{
    const QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
    m_content->addWidget(line(host));
    m_content->addWidget(Web::text(tr("Review"), "settingsTitle", host));
    auto *intro = Web::text(tr("Projects submit their new and changed entries to this master, and its reviewers approve them. These rules say what is merged without waiting for them."), "help", host);
    intro->setWordWrap(true);
    m_content->addWidget(intro);

    auto *same = new QCheckBox(tr("Entries the master already has as they are need no review"), host);
    same->setObjectName(QStringLiteral("ruleIdentical"));
    same->setChecked(!m_rules.value(QLatin1String("keepIdentical")).toBool());
    m_content->addWidget(same);
    connect(same, &QCheckBox::toggled, this, [this](bool on) {
        QJsonObject rules = m_rules;
        if (on)
            rules.remove(QLatin1String("keepIdentical"));
        else
            rules.insert(QLatin1String("keepIdentical"), true);
        putRules(rules, on ? tr("Entries the master already has are merged without review") : tr("Entries the master already has wait for review, too"));
    });

    // Fields whose changes need no approval.
    QStringList minor;
    for (const QJsonValue &v : m_rules.value(QLatin1String("minorFields")).toArray())
        minor << v.toString();
    const QJsonArray fields = def.value(QLatin1String("fields")).toArray();
    auto *minorTitle = Web::text(tr("Fields that need no approval"), "fieldLabel", host);
    m_content->addWidget(minorTitle);
    if (fields.isEmpty()) {
        auto *none = Web::text(tr("The termbase has no fields yet. A change to notes or sources only could be merged at once."), "help", host);
        none->setWordWrap(true);
        m_content->addWidget(none);
    } else {
        auto *boxes = new QWidget(host);
        boxes->setAccessibleName(tr("Fields that need no approval"));
        auto *flow = new FlowLayout(boxes, S3);
        flow->setContentsMargins(0, 0, 0, 0);
        for (const QJsonValue &fv : fields) {
            const QString name = str(fv.toObject(), "name");
            auto *box = new QCheckBox(name, boxes);
            box->setObjectName(QStringLiteral("minorField:%1").arg(name));
            box->setChecked(minor.contains(name, Qt::CaseInsensitive));
            flow->addWidget(box);
            connect(box, &QCheckBox::toggled, this, [this, name, minor](bool on) {
                QStringList now = minor;
                now.removeAll(name);
                if (on)
                    now << name;
                QJsonObject rules = m_rules;
                if (now.isEmpty())
                    rules.remove(QLatin1String("minorFields"));
                else
                    rules.insert(QLatin1String("minorFields"), QJsonArray::fromStringList(now));
                putRules(rules, on ? tr("Changes to %1 only are merged without review").arg(name) : tr("Changes to %1 wait for review").arg(name));
            });
        }
        m_content->addWidget(boxes);
        auto *help = Web::text(tr("An entry changed in these fields only is merged at once."), "help", host);
        help->setWordWrap(true);
        m_content->addWidget(help);
    }

    // Whose entries are approved as they come.
    m_content->addWidget(Web::text(tr("Trusted authors"), "fieldLabel", host));
    const QJsonArray trusted = m_rules.value(QLatin1String("trusted")).toArray();
    const auto nameOf = [this](const QString &username) {
        for (const QJsonValue &p : std::as_const(m_people)) {
            if (str(p.toObject(), "username") == username && !str(p.toObject(), "name").isEmpty())
                return QStringLiteral("%1 (%2)").arg(str(p.toObject(), "name"), username);
        }
        return username;
    };
    if (trusted.isEmpty()) {
        auto *none = Web::text(tr("Nobody yet: every entry waits for a reviewer."), "help", host);
        none->setObjectName(QStringLiteral("noTrusted"));
        none->setWordWrap(true);
        m_content->addWidget(none);
    }
    for (int i = 0; i < trusted.size(); i++) {
        const QJsonObject rule = trusted[i].toObject();
        const QString author = str(rule, "author");
        QStringList langs;
        for (const QJsonValue &l : rule.value(QLatin1String("scope")).toObject().value(QLatin1String("langs")).toArray())
            langs << Languages::name(l.toString());
        auto *rowWidget = new QWidget(host);
        rowWidget->setProperty("part", QStringLiteral("trusted"));
        auto *h = new QHBoxLayout(rowWidget);
        h->setContentsMargins(0, 0, 0, 0);
        h->setSpacing(S2);
        auto *who = new QLabel(QStringLiteral("<b>%1</b>").arg(nameOf(author).toHtmlEscaped()), rowWidget);
        h->addWidget(who);
        h->addWidget(Web::text(langs.isEmpty() ? tr("all entries") : tr("entries with terms in %1").arg(langs.join(QStringLiteral(", "))), "meta", rowWidget), 1);
        auto *remove = Web::button(tr("Remove"), Web::Quiet | Web::Small, rowWidget);
        remove->setObjectName(QStringLiteral("untrust:%1").arg(author));
        remove->setAccessibleName(tr("No longer trust %1").arg(author));
        h->addWidget(remove);
        rowWidget->setAccessibleName(QStringLiteral("%1, %2").arg(author, langs.isEmpty() ? tr("all entries") : langs.join(QStringLiteral(", "))));
        m_content->addWidget(rowWidget);
        connect(remove, &QPushButton::clicked, this, [this, trusted, i, author] {
            QJsonArray now = trusted;
            now.removeAt(i);
            QJsonObject rules = m_rules;
            if (now.isEmpty())
                rules.remove(QLatin1String("trusted"));
            else
                rules.insert(QLatin1String("trusted"), now);
            putRules(rules, tr("The entries of %1 wait for review again").arg(author));
        });
    }
    // .role-add
    auto *add = new QWidget(host);
    add->setAccessibleName(tr("Trust an author"));
    auto *al = new QHBoxLayout(add);
    al->setContentsMargins(0, 0, 0, 0);
    al->setSpacing(S2);
    QWidget *whoField = nullptr;
    std::function<QString()> chosen;
    if (m_conn && !m_conn->isLocal()) {
        auto *who = new QComboBox(add);
        who->setObjectName(QStringLiteral("trustAuthor"));
        who->addItem(tr("Choose a person…"), QString());
        for (const QJsonValue &p : std::as_const(m_people)) {
            const QString username = str(p.toObject(), "username");
            who->addItem(nameOf(username), username);
        }
        who->setCurrentIndex(qMax(0, who->findData(m_trustAuthor)));
        connect(who, &QComboBox::activated, this, [this, who](int i) { m_trustAuthor = who->itemData(i).toString(); });
        chosen = [who] { return who->currentData().toString(); };
        whoField = who;
    } else {
        auto *who = new QLineEdit(m_trustAuthor, add);
        who->setObjectName(QStringLiteral("trustAuthor"));
        who->setPlaceholderText(tr("Name, as in the history"));
        connect(who, &QLineEdit::textEdited, this, [this](const QString &t) { m_trustAuthor = t; });
        chosen = [who] { return who->text().trimmed(); };
        whoField = who;
    }
    whoField->setMinimumWidth(220);
    al->addWidget(Web::labelled(tr("Author"), whoField, add));
    auto *lang = new QComboBox(add);
    lang->setObjectName(QStringLiteral("trustLanguage"));
    lang->addItem(tr("All entries"), QString());
    for (const QJsonValue &l : def.value(QLatin1String("languages")).toArray())
        lang->addItem(tr("Terms in %1").arg(Languages::name(l.toString())), l.toString());
    lang->setCurrentIndex(qMax(0, lang->findData(m_trustLang)));
    connect(lang, &QComboBox::activated, this, [this, lang](int i) { m_trustLang = lang->itemData(i).toString(); });
    al->addWidget(Web::labelled(tr("For"), lang, add));
    auto *trust = Web::button(tr("Trust"), Web::Small, add);
    trust->setObjectName(QStringLiteral("trust"));
    al->addWidget(trust, 0, Qt::AlignBottom);
    al->addStretch();
    m_content->addWidget(add);
    connect(trust, &QPushButton::clicked, this, [this, chosen, lang, trusted] {
        const QString author = chosen();
        if (author.isEmpty()) {
            setError(tr("Say whose entries to trust."));
            return;
        }
        QJsonObject rule{{QLatin1String("author"), author}, {QLatin1String("scope"), QJsonObject{}}};
        const QString l = lang->currentData().toString();
        if (!l.isEmpty())
            rule.insert(QLatin1String("scope"), QJsonObject{{QLatin1String("langs"), QJsonArray{l}}});
        // One rule an author: a new one replaces the old.
        QJsonArray now;
        for (const QJsonValue &t : trusted) {
            if (str(t.toObject(), "author").compare(author, Qt::CaseInsensitive) != 0)
                now.append(t);
        }
        now.append(rule);
        QJsonObject rules = m_rules;
        rules.insert(QLatin1String("trusted"), now);
        m_trustAuthor.clear();
        m_trustLang.clear();
        putRules(rules, tr("The entries of %1 are approved without review").arg(author));
    });
    auto *help = Web::text(tr("Their new and changed entries are approved as they are submitted. An entry that conflicts with the master always waits for a reviewer."), "help", host);
    help->setWordWrap(true);
    m_content->addWidget(help);
}

// The masters a project termbase shows the entries of and merges into.
void TermbaseSettingsDialog::basedOnSection(QWidget *host)
{
    QStringList now;
    for (const QJsonValue &u : m_detail.value(QLatin1String("upstreams")).toArray())
        now << u.toString();
    m_content->addWidget(line(host));
    m_content->addWidget(Web::text(tr("Based on"), "settingsTitle", host));
    auto *intro = Web::text(tr("The termbase shows the entries of these masters under its own, and its new and changed entries are submitted to them."), "help", host);
    intro->setWordWrap(true);
    m_content->addWidget(intro);
    QStringList all = m_masters;
    for (const QString &u : std::as_const(now)) {
        if (!all.contains(u))
            all << u;
    }
    for (const QString &master : std::as_const(all)) {
        auto *box = new QCheckBox(master, host);
        box->setObjectName(QStringLiteral("basedOn:%1").arg(master));
        box->setChecked(now.contains(master));
        // A project termbase without a master would be a master itself.
        if (now.size() == 1 && now.contains(master)) {
            box->setEnabled(false);
            box->setToolTip(tr("It needs one master at least."));
        }
        m_content->addWidget(box);
        connect(box, &QCheckBox::toggled, this, [this, master, now](bool on) {
            if (!m_conn)
                return;
            QStringList next = now;
            next.removeAll(master);
            if (on)
                next << master;
            request(m_conn->api()->put(QStringLiteral("/api/termbases/") + ApiClient::seg(m_name) + QStringLiteral("/upstreams"), QJsonObject{{QLatin1String("upstreams"), QJsonArray::fromStringList(next)}}),
                    [this, master, on](const ApiResult &r) {
                        if (!r.ok()) {
                            setError(r.error.message);
                            render();
                            return;
                        }
                        setError(QString());
                        m_detail = r.object();
                        emit said(on ? tr("Now also based on %1").arg(master) : tr("No longer based on %1").arg(master));
                        emit changed(m_name);
                        render();
                    });
        });
    }
}

void TermbaseSettingsDialog::render()
{
    // Who may use it stays as it is while the rest is drawn again.
    if (m_access)
        m_content->removeWidget(m_access);
    clear(m_content);
    QWidget *host = m_content->parentWidget();
    const QJsonObject def = m_detail.value(QLatin1String("definition")).toObject();
    const QString base = QStringLiteral("/api/termbases/") + ApiClient::seg(m_name);
    QStringList langs;
    for (const QJsonValue &l : def.value(QLatin1String("languages")).toArray())
        langs << l.toString();

    // Languages (.settings-section, .settings-list)
    m_content->addWidget(line(host));
    m_content->addWidget(Web::text(tr("Languages"), "settingsTitle", host));
    if (langs.isEmpty()) {
        auto *h = Web::text(tr("No languages yet. They are added when you add terms or import a file."), "help", host);
        h->setWordWrap(true);
        m_content->addWidget(h);
    } else {
        auto *list = new QVBoxLayout;
        list->setSpacing(0);
        for (const QString &l : langs) {
            const int n = termsIn(l);
            auto *row = new QWidget(host);
            row->setObjectName(QStringLiteral("language:%1").arg(l));
            row->setMinimumHeight(36);
            auto *h = new QHBoxLayout(row);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(S3);
            auto *name = new QHBoxLayout;
            name->setSpacing(S2);
            name->addWidget(new QLabel(Languages::name(l), row));
            name->addWidget(Web::langChip(l, row));
            name->addStretch();
            h->addLayout(name, 1);
            h->addWidget(Web::text(TermData::termCount(n), "muted", row));
            auto *remove = new Web::ConfirmButton(QString(), n ? tr("Remove %1 and its %2?").arg(Languages::name(l), TermData::termCount(n)) : tr("Remove %1?").arg(Languages::name(l)),
                                                  tr("Remove"), row, true, QStringLiteral("trash"));
            remove->button()->setAccessibleName(tr("Remove %1").arg(Languages::name(l)));
            remove->button()->setToolTip(tr("Remove %1").arg(Languages::name(l)));
            h->addWidget(remove);
            connect(remove, &Web::ConfirmButton::confirmed, this, [this, base, l, n] {
                if (!m_conn)
                    return;
                request(m_conn->api()->del(base + QStringLiteral("/languages/") + ApiClient::seg(l)), [this, l, n](const ApiResult &r) {
                    if (!r.ok()) {
                        setError(r.error.message);
                        return;
                    }
                    setError(QString());
                    emit said(n ? tr("Removed %1 and %2").arg(Languages::name(l), TermData::termCount(n)) : tr("Removed %1").arg(Languages::name(l)));
                    emit changed(m_name);
                    refresh();
                });
            });
            list->addWidget(row);
            list->addWidget(line(host));
        }
        m_content->addLayout(list);
    }
    auto *add = new QComboBox(host);
    add->setObjectName(QStringLiteral("addLanguage"));
    add->setAccessibleName(tr("Language to add"));
    add->addItem(tr("Add a language…"), QString());
    for (const QString &l : Languages::common()) {
        if (!langs.contains(l))
            add->addItem(Languages::name(l), l);
    }
    add->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    auto *addButton = Web::button(tr("Add"), Web::Small, host);
    addButton->setObjectName(QStringLiteral("addLanguageButton"));
    addButton->setEnabled(false);
    connect(add, &QComboBox::currentIndexChanged, addButton, [add, addButton] { addButton->setEnabled(!add->currentData().toString().isEmpty()); });
    connect(addButton, &QPushButton::clicked, this, [this, add, def] {
        const QString l = add->currentData().toString();
        if (l.isEmpty())
            return;
        QJsonObject d = def;
        QJsonArray ls = d.value(QLatin1String("languages")).toArray();
        ls.append(l);
        d.insert(QLatin1String("languages"), ls);
        putDefinition(d, tr("Added %1").arg(Languages::name(l)));
    });
    m_content->addWidget(Web::row({add, addButton}, host));

    // Fields (.settings-fields)
    m_content->addWidget(line(host));
    m_content->addWidget(Web::text(tr("Fields"), "settingsTitle", host));
    const QJsonArray fields = def.value(QLatin1String("fields")).toArray();
    if (fields.isEmpty()) {
        auto *h = Web::text(tr("No fields yet: entries hold their terms and each term's status."), "help", host);
        h->setWordWrap(true);
        m_content->addWidget(h);
    } else {
        auto *table = new Web::Table({tr("Name"), tr("Applies to"), tr("Values"), tr("Used"), QString()}, host);
        table->setObjectName(QStringLiteral("fieldsTable"));
        table->setProperty("inDialog", true);
        table->setNumeric({3});
        table->setStretch(2);
        table->horizontalHeaderItem(4)->setData(Qt::AccessibleTextRole, tr("Remove"));
        for (int i = 0; i < fields.size(); i++) {
            const QJsonObject f = fields[i].toObject();
            const QString name = str(f, "name");
            const int used = valuesOf(name);
            auto *nameEdit = new QLineEdit(name, table);
            nameEdit->setAccessibleName(tr("Field name"));
            nameEdit->setObjectName(QStringLiteral("fieldName:%1").arg(name));
            nameEdit->setMinimumWidth(180);
            connect(nameEdit, &QLineEdit::editingFinished, this, [this, nameEdit, name, base] {
                const QString v = nameEdit->text().trimmed();
                if (v.isEmpty() || v == name || !m_conn) {
                    nameEdit->setText(name);
                    return;
                }
                request(m_conn->api()->put(base + QStringLiteral("/fields/") + ApiClient::seg(name), QJsonObject{{QLatin1String("name"), v}}), [this, nameEdit = QPointer<QLineEdit>(nameEdit), name](const ApiResult &r) {
                    if (!r.ok()) {
                        setError(r.error.message);
                        if (nameEdit)
                            nameEdit->setText(name);
                        return;
                    }
                    setError(QString());
                    emit changed(m_name);
                    refresh();
                });
            });
            auto *kindCell = new QWidget(table);
            auto *kl = new QVBoxLayout(kindCell);
            kl->setContentsMargins(0, 0, 0, 0);
            kl->setSpacing(S1);
            const QString was = TermFields::fromJson(f).kind;
            auto *kind = TermFields::kindCombo(was, kindCell);
            kind->setAccessibleName(tr("Values of %1").arg(name));
            kind->setObjectName(QStringLiteral("fieldKind:%1").arg(name));
            kind->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
            // The row is measured before the style gives the choice its
            // height, so the cell says how tall it is.
            kindCell->setMinimumHeight(34);
            kl->addWidget(kind, 0, Qt::AlignLeft);
            const auto setField = [this, def, i](const QJsonObject &now, const QString &done) {
                QJsonObject d = def;
                QJsonArray fs = d.value(QLatin1String("fields")).toArray();
                fs[i] = now;
                d.insert(QLatin1String("fields"), fs);
                putDefinition(d, done);
            };
            connect(kind, &QComboBox::activated, this, [kind, f, setField, was] {
                const QString now = kind->currentData().toString();
                if (now == was)
                    return;
                QJsonObject changed = f;
                changed.insert(QLatin1String("kind"), now);
                if (now != QLatin1String("picklist"))
                    changed.remove(QLatin1String("values"));
                else if (!changed.contains(QLatin1String("values")))
                    changed.insert(QLatin1String("values"), QJsonArray());
                setField(changed, QString());
            });
            if (was == QLatin1String("picklist")) {
                QStringList values;
                for (const QJsonValue &v : f.value(QLatin1String("values")).toArray())
                    values << v.toString();
                auto *vals = new TermFields::ValuesEdit(values, kindCell);
                vals->setObjectName(QStringLiteral("fieldValues:%1").arg(name));
                vals->setAccessibleName(tr("The values of %1, one a line").arg(name));
                kl->addWidget(vals);
                kindCell->setMinimumHeight(0);
                connect(vals, &TermFields::ValuesEdit::left, this, [f, setField, values](const QStringList &now) {
                    if (now == values)
                        return;
                    QJsonObject changed = f;
                    changed.insert(QLatin1String("values"), QJsonArray::fromStringList(now));
                    setField(changed, QString());
                });
            }
            auto *remove = new Web::ConfirmButton(QString(), used ? tr("Remove “%1” and %2?").arg(name, plural(used, tr("1 value"), tr("%1 values"))) : tr("Remove “%1”?").arg(name),
                                                  tr("Remove"), table, true, QStringLiteral("trash"));
            remove->button()->setAccessibleName(tr("Remove %1").arg(name));
            remove->button()->setToolTip(tr("Remove %1").arg(name));
            connect(remove, &Web::ConfirmButton::confirmed, this, [this, base, name] {
                if (!m_conn)
                    return;
                request(m_conn->api()->del(base + QStringLiteral("/fields/") + ApiClient::seg(name)), [this, name](const ApiResult &r) {
                    if (!r.ok()) {
                        setError(r.error.message);
                        return;
                    }
                    setError(QString());
                    emit said(tr("Removed the field “%1”").arg(name));
                    emit changed(m_name);
                    refresh();
                });
            });
            const int row = table->addRow({nameEdit, Web::cellText(levelLabel(str(f, "level")), table), kindCell, Web::cellText(QLocale().toString(used), table, true),
                                           Web::cellActions({remove}, table)});
            table->item(row, 0)->setData(Qt::AccessibleTextRole, name);
            table->item(row, 1)->setData(Qt::AccessibleTextRole, levelLabel(str(f, "level")));
            table->item(row, 3)->setData(Qt::AccessibleTextRole, QString::number(used));
        }
        table->fit();
        m_content->addWidget(table);
    }
    {
        QStringList taken;
        for (const QJsonValue &f : fields)
            taken << str(f.toObject(), "name");
        auto *add = new TermFields::AddFieldForm(m_name, taken, host);
        add->setObjectName(QStringLiteral("settingsAddField"));
        connect(add, &TermFields::AddFieldForm::added, this, [this, def](const QJsonObject &field) {
            QJsonObject d = def;
            QJsonArray fs = d.value(QLatin1String("fields")).toArray();
            fs.append(field);
            d.insert(QLatin1String("fields"), fs);
            putDefinition(d, tr("Added field “%1” to %2").arg(str(field, "name"), m_name));
        });
        m_content->addWidget(add);
    }

    if (str(m_detail, "kind") == QLatin1String("master"))
        reviewSection(host);
    else
        basedOnSection(host);

    // Who may use it, on a team server.
    if (m_conn && !m_conn->isLocal()) {
        m_content->addWidget(line(host));
        if (!m_access) {
            m_access = new AccessPanel(m_conn, QStringLiteral("termbase"), m_name, tr("Who may use it"), host, true);
            m_access->setObjectName(QStringLiteral("termbaseAccess"));
        }
        m_content->addWidget(m_access);
    }

    // Delete (.settings-danger)
    {
        m_content->addWidget(line(host));
        m_content->addWidget(Web::text(tr("Delete termbase"), "dangerTitle", host));
        QStringList based;
        for (const QJsonValue &v : m_detail.value(QLatin1String("projects")).toArray())
            based << v.toString();
        const bool project = str(m_detail, "kind") == QLatin1String("project");
        if (!based.isEmpty()) {
            auto *h = Web::text((based.size() == 1 ? tr("%1 is based on this termbase. Delete it, or base it on another master, first.")
                                                   : tr("%1 are based on this termbase. Delete them, or base them on another master, first."))
                                    .arg(based.join(QStringLiteral(", "))),
                                "help", host);
            h->setObjectName(QStringLiteral("basedOnIt"));
            h->setWordWrap(true);
            m_content->addWidget(h);
        } else if (!m_usedBy.isEmpty()) {
            auto *h = Web::text((m_usedBy.size() == 1 ? tr("%1 uses this termbase. Remove it from the project first.") : tr("%1 use this termbase. Remove it from the project first."))
                                    .arg(m_usedBy.join(QStringLiteral(", "))),
                                "help", host);
            h->setObjectName(QStringLiteral("usedBy"));
            h->setWordWrap(true);
            m_content->addWidget(h);
        } else {
            auto *h = Web::text(project ? tr("Deletes the project's %1 and their history. The masters keep theirs. Merge what the masters should have first.").arg(TermData::entries(m_entries.size()))
                                        : tr("Deletes %1 and their history. Export the termbase first if you may need it again.").arg(TermData::entries(m_entries.size())),
                                "help", host);
            h->setWordWrap(true);
            m_content->addWidget(h);
            auto *del = new Web::ConfirmButton(tr("Delete termbase"), tr("Delete %1 for good?").arg(m_name), tr("Delete"), host);
            del->setObjectName(QStringLiteral("deleteTermbase"));
            m_content->addWidget(del, 0, Qt::AlignLeft);
            connect(del, &Web::ConfirmButton::confirmed, this, [this, base] {
                if (!m_conn)
                    return;
                request(m_conn->api()->del(base), [this](const ApiResult &r) {
                    if (!r.ok()) {
                        setError(r.error.message);
                        return;
                    }
                    emit said(tr("Deleted %1").arg(m_name));
                    emit changed(QString());
                    accept();
                });
            });
        }
    }
    m_content->addStretch();
}

// ---- Import ---------------------------------------------------------------

ImportDialog::ImportDialog(ConnectionManager *manager, const QString &connectionId, const QString &termbase, QWidget *parent)
    : Web::Dialog(termbase.isEmpty() ? tr("Import a termbase") : tr("Import into %1").arg(termbase), parent, true)
    , m_manager(manager)
    , m_connId(connectionId)
    , m_termbase(termbase)
    , m_intoNew(termbase.isEmpty())
{
    setObjectName(QStringLiteral("importDialog"));
    m_stepper = new Web::Stepper(this);
    body()->addWidget(m_stepper);
    auto *holder = new QWidget(this);
    m_content = new QVBoxLayout(holder);
    m_content->setContentsMargins(0, 0, 0, 0);
    m_content->setSpacing(S3);
    body()->addWidget(holder, 1);

    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, this);
    m_back = Web::button(tr("Back"), Web::Small, this);
    m_next = Web::button(tr("Continue"), Web::Primary | Web::Small, this);
    buttons()->addWidget(cancel);
    buttons()->addWidget(m_back);
    buttons()->addWidget(m_next);
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    connect(m_back, &QPushButton::clicked, this, &ImportDialog::back);
    connect(m_next, &QPushButton::clicked, this, &ImportDialog::next);

    // A new termbase goes where the opened one is, unless said otherwise.
    const QList<Connection *> all = places();
    for (int i = 0; i < all.size(); i++) {
        if (all[i]->id() == connectionId)
            m_place = i;
    }
    render();
    // What the termbase has decides how the file's languages map and
    // whether existing entries can be met.
    if (!m_termbase.isEmpty()) {
        if (Connection *c = m_manager->find(m_connId)) {
            request(c->api()->get(QStringLiteral("/api/termbases/") + ApiClient::seg(m_termbase)), [this](const ApiResult &r) {
                m_destination = r.object();
                m_destinationEntries = r.object().value(QLatin1String("entries")).toInt();
            });
        }
    }
}

void ImportDialog::request(ApiReply *reply, std::function<void(const ApiResult &)> then)
{
    m_pending++;
    onResult(reply, this, [this, then](const ApiResult &r) {
        m_pending--;
        then(r);
        if (m_pending == 0)
            emit ready();
    });
}

QList<Connection *> ImportDialog::places() const
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

Connection *ImportDialog::place() const
{
    if (!m_intoNew)
        return m_manager->find(m_connId);
    return places().value(m_place);
}

QStringList ImportDialog::destinationLanguages() const
{
    QStringList out;
    if (m_intoNew)
        return out;
    for (const QJsonValue &l : m_destination.value(QLatin1String("definition")).toObject().value(QLatin1String("languages")).toArray())
        out << l.toString();
    return out;
}

void ImportDialog::setIntoNew(bool intoNew)
{
    m_intoNew = intoNew || m_termbase.isEmpty();
    render();
}

void ImportDialog::setNewName(const QString &name)
{
    m_newName = name;
    if (auto *e = findChild<QLineEdit *>(QStringLiteral("newName")); e && e->text() != name)
        e->setText(name);
}

void ImportDialog::setPlace(int index)
{
    m_place = index;
    if (m_step == File)
        render();
}

void ImportDialog::setMapping(const QJsonObject &mapping)
{
    m_mapping = mapping;
    if (m_step == Columns)
        render();
}

void ImportDialog::mapLanguage(const QString &fileLang, const QString &to)
{
    m_langMap.insert(fileLang, to);
    if (m_step == LanguagesStep)
        readLanguages();
}

void ImportDialog::setFallbackStatus(const QString &status)
{
    m_fallback = status;
    if (m_step == LanguagesStep)
        render();
}

void ImportDialog::setPolicy(const QString &match, const QString &lang, const QString &onMatch)
{
    m_match = match;
    m_matchLang = lang;
    m_onMatch = onMatch;
    if (m_step == Review)
        loadPlan();
}

void ImportDialog::render()
{
    clear(m_content);
    setError(QString());
    const bool tabular = m_inspection.value(QLatin1String("tabular")).toBool();
    QStringList names{tr("File")};
    if (tabular)
        names << tr("Columns");
    names << tr("Languages") << tr("Review");
    int at = 0;
    switch (m_step) {
    case File:
        at = 0;
        break;
    case Columns:
        at = 1;
        break;
    case LanguagesStep:
        at = tabular ? 2 : 1;
        break;
    case Review:
        at = tabular ? 3 : 2;
        break;
    }
    m_stepper->setSteps(names, at);
    setTitle(!m_termbase.isEmpty() && !m_intoNew ? tr("Import into %1").arg(m_termbase) : tr("Import a termbase"));
    m_back->setVisible(m_step != File);
    m_next->setVisible(m_step != File);
    m_next->setText(m_step == Review ? tr("Import") : tr("Continue"));
    switch (m_step) {
    case File:
        showFile();
        break;
    case Columns:
        showColumns();
        break;
    case LanguagesStep:
        showLanguages();
        break;
    case Review:
        showReview();
        break;
    }
    adjustSize();
}

void ImportDialog::showFile()
{
    QWidget *host = m_content->parentWidget();
    if (!m_termbase.isEmpty()) {
        m_content->addWidget(bold(tr("Import into"), host));
        auto *group = new QButtonGroup(host);
        auto *here = new QRadioButton(m_termbase, host);
        here->setObjectName(QStringLiteral("intoThis"));
        auto *other = new QRadioButton(tr("A new master termbase"), host);
        other->setObjectName(QStringLiteral("intoNew"));
        here->setChecked(!m_intoNew);
        other->setChecked(m_intoNew);
        group->addButton(here);
        group->addButton(other);
        m_content->addWidget(here);
        m_content->addWidget(other);
        connect(other, &QRadioButton::toggled, this, [this](bool on) {
            if (on != m_intoNew)
                setIntoNew(on);
        });
    }
    if (m_intoNew) {
        // .field-grid
        auto *grid = new QHBoxLayout;
        grid->setSpacing(S4);
        auto *nameBox = new QWidget(host);
        auto *nv = new QVBoxLayout(nameBox);
        nv->setContentsMargins(0, 0, 0, 0);
        nv->setSpacing(S2);
        auto *label = new QLabel(QStringLiteral("<b>%1</b> <span style=\"color:%2\">%3</span>").arg(tr("Name"), Theme::color("text3").name(), tr("from the file if empty")), nameBox);
        auto *name = new QLineEdit(m_newName, nameBox);
        name->setObjectName(QStringLiteral("newName"));
        name->setAccessibleName(tr("Name"));
        label->setBuddy(name);
        nv->addWidget(label);
        nv->addWidget(name);
        connect(name, &QLineEdit::textChanged, this, [this](const QString &t) { m_newName = t; });
        grid->addWidget(nameBox, 1);
        const QList<Connection *> all = places();
        if (all.size() > 1) {
            QStringList choices;
            for (Connection *c : all)
                choices << (c->isLocal() ? tr("This computer") : all.size() > 2 ? c->displayName() : tr("Server"));
            auto *whereBox = new QWidget(host);
            auto *wv = new QVBoxLayout(whereBox);
            wv->setContentsMargins(0, 0, 0, 0);
            wv->setSpacing(S2);
            wv->addWidget(bold(tr("Store on"), whereBox));
            auto *where = new Web::Segmented(tr("Store on"), choices, whereBox);
            where->setObjectName(QStringLiteral("place"));
            where->setCurrent(qBound(0, m_place, int(all.size()) - 1));
            connect(where, &Web::Segmented::changed, this, [this](int i) { m_place = i; });
            wv->addWidget(where);
            wv->addStretch();
            grid->addWidget(whereBox, 1);
        } else {
            grid->addStretch(1);
        }
        m_content->addLayout(grid);
    }
    // .dropzone-small: a file is chosen, or dropped.
    auto *zone = new Web::DropZone(host);
    zone->setObjectName(QStringLiteral("dropzone"));
    auto *zv = new QVBoxLayout(zone);
    zv->setContentsMargins(S4, S4, S4, S4);
    zv->setSpacing(S1);
    auto *choose = Web::button(m_pending > 0 ? tr("Reading…") : tr("Choose a termbase file"), Web::Small, zone, QStringLiteral("file-up"));
    choose->setObjectName(QStringLiteral("chooseFile"));
    choose->setEnabled(place() != nullptr);
    zv->addWidget(choose, 0, Qt::AlignHCenter);
    auto *help = Web::text(tr("TBX (2 or 3), an .sdltb termbase or its XML export, an XLSX workbook, CSV or tab-separated text. A termbase definition (.xdt) brings only the languages."),
                           "help", zone);
    help->setWordWrap(true);
    help->setAlignment(Qt::AlignHCenter);
    zv->addWidget(help);
    m_content->addWidget(zone);
    connect(choose, &QPushButton::clicked, this, [this] {
        const QString path = QFileDialog::getOpenFileName(this, tr("Choose a termbase file"), QString(), termImportFilter());
        if (!path.isEmpty())
            chooseFile(path);
    });
    connect(zone, &Web::DropZone::dropped, this, [this](const QStringList &paths) {
        if (!paths.isEmpty())
            chooseFile(paths.first());
    });
}

void ImportDialog::chooseFile(const QString &path)
{
    QFile f(path);
    const QString name = QFileInfo(path).fileName();
    if (!f.open(QIODevice::ReadOnly)) {
        setError(tr("%1 could not be read: %2").arg(name, f.errorString()));
        return;
    }
    m_data = f.readAll();
    m_fileName = name;
    m_mapping = {};
    m_langMap = {};
    m_read = {};
    if (m_newName.trimmed().isEmpty()) {
        QString base = QFileInfo(path).completeBaseName();
        base.replace(QRegularExpression(QStringLiteral("[_-]+")), QStringLiteral(" "));
        setNewName(base);
    }
    inspect(false, [this](const QJsonObject &o) {
        m_inspection = o;
        if (o.value(QLatin1String("tabular")).toBool()) {
            m_mapping = o.value(QLatin1String("mapping")).toObject();
            m_step = Columns;
            render();
            return;
        }
        if (o.value(QLatin1String("read")).toObject().value(QLatin1String("entries")).toInt() == 0 && str(o, "format") != QLatin1String("xdt")) {
            setError(tr("%1 could not be read: the file has no entries with terms").arg(m_fileName));
            return;
        }
        m_read = o;
        m_step = LanguagesStep;
        readLanguages();
    });
}

QUrlQuery ImportDialog::options(bool withLanguages) const
{
    QUrlQuery q;
    q.addQueryItem(QStringLiteral("filename"), QString::fromUtf8(QUrl::toPercentEncoding(m_fileName)));
    if (!m_mapping.isEmpty() && m_inspection.value(QLatin1String("tabular")).toBool())
        q.addQueryItem(QStringLiteral("mapping"), QString::fromUtf8(QUrl::toPercentEncoding(QString::fromUtf8(QJsonDocument(m_mapping).toJson(QJsonDocument::Compact)))));
    if (withLanguages && !m_langMap.isEmpty())
        q.addQueryItem(QStringLiteral("languages"), QString::fromUtf8(QUrl::toPercentEncoding(QString::fromUtf8(QJsonDocument(m_langMap).toJson(QJsonDocument::Compact)))));
    return q;
}

void ImportDialog::inspect(bool withMapping, std::function<void(const QJsonObject &)> then)
{
    Connection *c = place();
    if (!c) {
        setError(tr("Sign in first."));
        return;
    }
    setError(QString());
    QUrlQuery q;
    q.addQueryItem(QStringLiteral("filename"), QString::fromUtf8(QUrl::toPercentEncoding(m_fileName)));
    if (withMapping)
        q = options(false);
    request(c->api()->upload("POST", QStringLiteral("/api/termfiles/inspect"), m_data, QStringLiteral("application/octet-stream"), q), [this, then](const ApiResult &r) {
        if (!r.ok()) {
            setError(tr("%1 could not be read: %2").arg(m_fileName, r.error.message));
            return;
        }
        then(r.object());
    });
}

// Where each of the file's languages goes: the termbase's language that
// is the same, else one of the same language, else the listed language.
static QString suggestLanguage(const QString &fileLang, const QStringList &dest)
{
    for (const QString &l : dest) {
        if (l.compare(fileLang, Qt::CaseInsensitive) == 0)
            return l;
    }
    const QStringList common = Languages::common();
    if (!fileLang.contains(QLatin1Char('-'))) {
        for (const QString &l : dest) {
            if (Languages::samePrimary(l, fileLang))
                return l;
        }
        for (const QString &l : common) {
            if (Languages::samePrimary(l, fileLang))
                return l;
        }
        return fileLang;
    }
    for (const QString &l : common) {
        if (l.compare(fileLang, Qt::CaseInsensitive) == 0)
            return l;
    }
    return fileLang;
}

void ImportDialog::readLanguages()
{
    // The first time, each language of the file is given its place; then
    // the file is read again under the map, which says how many entries
    // it leaves out.
    if (m_langMap.isEmpty()) {
        m_fileLangs.clear();
        const QStringList dest = destinationLanguages();
        for (const QJsonValue &l : m_read.value(QLatin1String("languages")).toArray()) {
            m_fileLangs << l.toString();
            m_langMap.insert(l.toString(), suggestLanguage(l.toString(), dest));
        }
    }
    Connection *c = place();
    if (!c)
        return;
    const int asked = ++m_asked;
    request(c->api()->upload("POST", QStringLiteral("/api/termfiles/inspect"), m_data, QStringLiteral("application/octet-stream"), options(true)), [this, asked](const ApiResult &r) {
        if (asked != m_asked)
            return;
        if (!r.ok()) {
            setError(tr("%1 could not be read: %2").arg(m_fileName, r.error.message));
            return;
        }
        m_read = r.object();
        if (m_step == LanguagesStep)
            render();
    });
}

void ImportDialog::showColumns()
{
    QWidget *host = m_content->parentWidget();
    const QJsonArray rows = m_inspection.value(QLatin1String("rows")).toArray();
    const bool header = m_mapping.value(QLatin1String("header")).toBool();
    const int total = m_inspection.value(QLatin1String("total")).toInt(int(rows.size())) - (header ? 1 : 0);
    auto *help = Web::text(tr("%1: %2 of entries. Each row is one entry. Check what each column holds; the guess comes from the column names.")
                               .arg(m_fileName, plural(total, tr("1 row"), tr("%1 rows"))),
                           "help", host);
    help->setWordWrap(true);
    m_content->addWidget(help);

    auto *first = new QCheckBox(tr("The first row holds column names"), host);
    first->setObjectName(QStringLiteral("headerRow"));
    first->setChecked(header);
    auto *sepLabel = Web::text(tr("Synonyms in one cell separated by"), "small2", host);
    auto *sep = new QLineEdit(m_mapping.contains(QLatin1String("separator")) ? str(m_mapping, "separator") : QStringLiteral(";"), host);
    sep->setObjectName(QStringLiteral("separator"));
    sep->setAccessibleName(tr("Synonyms in one cell separated by"));
    sep->setMaxLength(3);
    sep->setFixedWidth(56);
    sep->setAlignment(Qt::AlignHCenter);
    sepLabel->setBuddy(sep);
    m_content->addWidget(Web::row({first, sepLabel, sep}, host, S3));
    connect(first, &QCheckBox::toggled, this, [this](bool on) {
        m_mapping.insert(QLatin1String("header"), on);
        render();
    });
    connect(sep, &QLineEdit::textChanged, this, [this](const QString &t) { m_mapping.insert(QLatin1String("separator"), t); });

    QJsonArray columns = m_mapping.value(QLatin1String("columns")).toArray();
    int width = 0;
    for (const QJsonValue &c : columns)
        width = qMax(width, c.toObject().value(QLatin1String("index")).toInt() + 1);
    for (const QJsonValue &r : rows)
        width = qMax(width, int(r.toArray().size()));
    const auto column = [columns](int i) {
        for (const QJsonValue &c : columns) {
            if (c.toObject().value(QLatin1String("index")).toInt() == i)
                return c.toObject();
        }
        return QJsonObject{{QLatin1String("index"), i}, {QLatin1String("role"), QStringLiteral("ignore")}};
    };
    const auto set = [this](int i, const QJsonObject &now, bool redraw) {
        QJsonArray cols = m_mapping.value(QLatin1String("columns")).toArray();
        bool found = false;
        for (int k = 0; k < cols.size(); k++) {
            if (cols[k].toObject().value(QLatin1String("index")).toInt() == i) {
                cols[k] = now;
                found = true;
            }
        }
        if (!found)
            cols.append(now);
        m_mapping.insert(QLatin1String("columns"), cols);
        if (redraw)
            render();
        else
            m_next->setEnabled([&] {
                for (const QJsonValue &c : cols) {
                    if (str(c.toObject(), "role") == QLatin1String("term") && !str(c.toObject(), "lang").isEmpty())
                        return true;
                }
                return false;
            }());
    };

    // .mapping-wrap: what each column holds over the first rows.
    auto *scroll = new QScrollArea(host);
    scroll->setObjectName(QStringLiteral("mapping"));
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setVerticalScrollBarPolicy(Qt::ScrollBarAsNeeded);
    Theme::role(scroll, "paneScroll");
    auto *inner = new QFrame(scroll);
    Theme::role(inner, "box");
    auto *grid = new QGridLayout(inner);
    grid->setContentsMargins(1, 1, 1, 1);
    grid->setHorizontalSpacing(0);
    grid->setVerticalSpacing(0);
    QStringList langCodes = Languages::common();
    for (const QJsonValue &c : columns) {
        const QString l = str(c.toObject(), "lang");
        if (!l.isEmpty() && !langCodes.contains(l))
            langCodes << l;
    }
    bool hasTerms = false;
    for (int i = 0; i < width; i++) {
        const QJsonObject col = column(i);
        const QString role = str(col, "role");
        const QString level = str(col, "level").isEmpty() ? QStringLiteral("entry") : str(col, "level");
        hasTerms = hasTerms || (role == QLatin1String("term") && !str(col, "lang").isEmpty());
        auto *head = new QFrame(inner);
        head->setStyleSheet(QStringLiteral("QFrame { background: %1; border: 0; border-bottom: 1px solid %2; }").arg(Theme::color("surface2").name(), Theme::color("border").name()));
        head->setMinimumWidth(150);
        auto *hv = new QVBoxLayout(head);
        hv->setContentsMargins(S3, 10, S3, 10);
        hv->setSpacing(S1);
        const QString title = str(col, "header").isEmpty() ? tr("Column %1").arg(i + 1) : str(col, "header");
        auto *t = new QLabel(QStringLiteral("<b>%1</b>").arg(title.toHtmlEscaped()), head);
        t->setStyleSheet(QStringLiteral("border: 0;"));
        hv->addWidget(t);
        auto *roleBox = new QComboBox(head);
        roleBox->setObjectName(QStringLiteral("role:%1").arg(i));
        roleBox->setAccessibleName(tr("Column %1 holds").arg(i + 1));
        roleBox->addItem(tr("Terms"), QStringLiteral("term"));
        roleBox->addItem(tr("Status"), QStringLiteral("status"));
        roleBox->addItem(tr("Field"), QStringLiteral("field"));
        roleBox->addItem(tr("Entry ID"), QStringLiteral("id"));
        roleBox->addItem(tr("Leave out"), QStringLiteral("ignore"));
        roleBox->setCurrentIndex(qMax(0, roleBox->findData(role)));
        hv->addWidget(roleBox);
        connect(roleBox, &QComboBox::activated, this, [col, roleBox, set, i, title] {
            QJsonObject now = col;
            const QString r = roleBox->currentData().toString();
            now.insert(QLatin1String("role"), r);
            if (r == QLatin1String("field")) {
                if (str(now, "field").isEmpty())
                    now.insert(QLatin1String("field"), title);
                if (str(now, "level").isEmpty())
                    now.insert(QLatin1String("level"), str(now, "lang").isEmpty() ? QStringLiteral("entry") : QStringLiteral("language"));
            } else {
                now.remove(QLatin1String("field"));
                now.remove(QLatin1String("level"));
            }
            set(i, now, true);
        });
        if (role == QLatin1String("term") || role == QLatin1String("status") || (role == QLatin1String("field") && level != QLatin1String("entry"))) {
            auto *langBox = new QComboBox(head);
            langBox->setObjectName(QStringLiteral("lang:%1").arg(i));
            langBox->setAccessibleName(tr("Language of column %1").arg(i + 1));
            langBox->addItem(tr("Language…"), QString());
            for (const QString &l : langCodes)
                langBox->addItem(Languages::name(l), l);
            langBox->setCurrentIndex(qMax(0, langBox->findData(str(col, "lang"))));
            hv->addWidget(langBox);
            connect(langBox, &QComboBox::activated, this, [col, langBox, set, i] {
                QJsonObject now = col;
                if (langBox->currentData().toString().isEmpty())
                    now.remove(QLatin1String("lang"));
                else
                    now.insert(QLatin1String("lang"), langBox->currentData().toString());
                set(i, now, true);
            });
        }
        if (role == QLatin1String("field")) {
            auto *fieldName = new QLineEdit(str(col, "field"), head);
            fieldName->setObjectName(QStringLiteral("field:%1").arg(i));
            fieldName->setAccessibleName(tr("Field name of column %1").arg(i + 1));
            fieldName->setPlaceholderText(tr("Field name"));
            hv->addWidget(fieldName);
            connect(fieldName, &QLineEdit::textChanged, this, [this, i, set](const QString &text) {
                // The column as it is now, so that typing a name keeps
                // what was chosen since the step was drawn.
                for (const QJsonValue &c : m_mapping.value(QLatin1String("columns")).toArray()) {
                    if (c.toObject().value(QLatin1String("index")).toInt() != i)
                        continue;
                    QJsonObject now = c.toObject();
                    now.insert(QLatin1String("field"), text);
                    set(i, now, false);
                    return;
                }
            });
            auto *levelBox = new QComboBox(head);
            levelBox->setObjectName(QStringLiteral("level:%1").arg(i));
            levelBox->setAccessibleName(tr("Column %1 applies to").arg(i + 1));
            levelBox->addItem(tr("Whole entry"), QStringLiteral("entry"));
            levelBox->addItem(tr("One language"), QStringLiteral("language"));
            levelBox->addItem(tr("The terms"), QStringLiteral("term"));
            levelBox->setCurrentIndex(qMax(0, levelBox->findData(level)));
            hv->addWidget(levelBox);
            connect(levelBox, &QComboBox::activated, this, [col, levelBox, set, i] {
                QJsonObject now = col;
                now.insert(QLatin1String("level"), levelBox->currentData().toString());
                set(i, now, true);
            });
        }
        hv->addStretch();
        grid->addWidget(head, 0, i);
        int shownRows = 0;
        for (int r = header ? 1 : 0; r < rows.size() && shownRows < 8; r++, shownRows++) {
            auto *cell = new QLabel(rows[r].toArray().at(i).toString(), inner);
            cell->setTextFormat(Qt::PlainText);
            cell->setMaximumWidth(220);
            cell->setStyleSheet(QStringLiteral("QLabel { border: 0; border-bottom: 1px solid %1; padding: 10px 12px; font-size: 13px; color: %2; }")
                                    .arg(Theme::color("border").name(), (role == QLatin1String("ignore") ? Theme::color("text3") : Theme::color("text")).name()));
            cell->setToolTip(cell->text());
            cell->setText(cell->fontMetrics().elidedText(cell->text(), Qt::ElideRight, 196));
            grid->addWidget(cell, shownRows + 1, i);
        }
    }
    scroll->setWidget(inner);
    Web::fitScroll(scroll);
    scroll->setMinimumHeight(qMin(340, inner->sizeHint().height() + 14));
    scroll->setMaximumHeight(340);
    m_content->addWidget(scroll);
    if (!hasTerms) {
        auto *e = Web::text(tr("Mark at least one column as terms and give it a language."), "error", host);
        e->setObjectName(QStringLiteral("columnsProblem"));
        m_content->addWidget(e);
    }
    m_next->setEnabled(hasTerms);
}

void ImportDialog::showLanguages()
{
    QWidget *host = m_content->parentWidget();
    const QJsonObject rep = m_read.value(QLatin1String("read")).toObject();
    const int skipped = rep.value(QLatin1String("skipped")).toInt();
    QString summary = tr("%1: %2, %3").arg(termFormatLabel(str(m_inspection, "format")), TermData::entries(rep.value(QLatin1String("entries")).toInt()),
                                           TermData::termCount(rep.value(QLatin1String("terms")).toInt()));
    if (skipped)
        summary += tr(", %1 rows without terms skipped").arg(skipped);
    summary += QLatin1Char('.');
    auto *help = Web::text(summary, "help", host);
    help->setObjectName(QStringLiteral("fileSummary"));
    help->setWordWrap(true);
    m_content->addWidget(help);

    const QStringList dest = destinationLanguages();
    auto *table = new Web::Table({tr("In the file"), tr("In the termbase")}, host);
    table->setObjectName(QStringLiteral("languages"));
    table->setProperty("inDialog", true);
    table->setAccessibleName(tr("Languages of the file"));
    table->setStretch(1);
    for (const QString &fl : m_fileLangs) {
        const QString to = m_langMap.value(fl).toString();
        auto *box = new QComboBox(table);
        box->setObjectName(QStringLiteral("map:%1").arg(fl));
        box->setAccessibleName(tr("Termbase language for %1").arg(fl));
        box->addItem(tr("Leave out"), QString());
        QStringList codes = dest;
        for (const QString &l : Languages::common()) {
            if (!codes.contains(l))
                codes << l;
        }
        const QString own = to.isEmpty() ? fl : to;
        if (!codes.contains(own))
            codes << own;
        for (const QString &code : codes)
            box->addItem(!m_intoNew && !dest.contains(code) ? tr("%1 (new)").arg(Languages::name(code)) : Languages::name(code), code);
        box->setCurrentIndex(qMax(0, box->findData(to)));
        connect(box, &QComboBox::activated, this, [this, fl, box] { mapLanguage(fl, box->currentData().toString()); });
        box->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
        auto *code = Web::cellText(fl, table);
        const int row = table->addRow({code, box});
        table->item(row, 0)->setData(Qt::AccessibleTextRole, fl);
        table->item(row, 1)->setData(Qt::AccessibleTextRole, to.isEmpty() ? tr("Leave out") : Languages::name(to));
    }
    table->fit();
    m_content->addWidget(table);

    // The fields the termbase does not have yet.
    QStringList have, fresh;
    if (!m_intoNew) {
        for (const QJsonValue &f : m_destination.value(QLatin1String("definition")).toObject().value(QLatin1String("fields")).toArray())
            have << str(f.toObject(), "name").toLower();
    }
    for (const QJsonValue &f : m_read.value(QLatin1String("fields")).toArray()) {
        if (!have.contains(str(f.toObject(), "name").toLower()))
            fresh << QStringLiteral("%1 (%2)").arg(str(f.toObject(), "name"), levelLabel(str(f.toObject(), "level")).toLower());
    }
    if (!fresh.isEmpty()) {
        auto *f = Web::text(tr("New fields: %1.").arg(fresh.join(QStringLiteral(", "))), "help", host);
        f->setObjectName(QStringLiteral("newFields"));
        f->setWordWrap(true);
        m_content->addWidget(f);
    }
    const int dropped = m_read.value(QLatin1String("dropped")).toInt();
    if (dropped > 0) {
        auto *n = Web::note((dropped == 1 ? tr("1 entry has no terms in the languages you import and will be left out.")
                                          : tr("%1 entries have no terms in the languages you import and will be left out.").arg(dropped)),
                            QStringLiteral("warn"), host);
        n->setObjectName(QStringLiteral("dropped"));
        m_content->addWidget(n);
    }
    auto *warnings = new QVBoxLayout;
    warnings->setSpacing(S1);
    addWarnings(warnings, rep.value(QLatin1String("warnings")).toArray(), host);
    m_content->addLayout(warnings);

    m_content->addWidget(bold(tr("Terms without a status"), host));
    auto *group = new QButtonGroup(host);
    auto *approved = new QRadioButton(tr("Approved"), host);
    approved->setObjectName(QStringLiteral("fallbackApproved"));
    auto *proposed = new QRadioButton(tr("Proposed, for lists nobody has checked"), host);
    proposed->setObjectName(QStringLiteral("fallbackProposed"));
    approved->setChecked(m_fallback == QLatin1String("approved"));
    proposed->setChecked(m_fallback != QLatin1String("approved"));
    group->addButton(approved);
    group->addButton(proposed);
    m_content->addWidget(approved);
    m_content->addWidget(proposed);
    connect(approved, &QRadioButton::toggled, this, [this](bool on) { m_fallback = on ? QStringLiteral("approved") : QStringLiteral("proposed"); });
    m_next->setEnabled(rep.value(QLatin1String("entries")).toInt() - dropped > 0);
}

void ImportDialog::loadPlan()
{
    const QJsonObject rep = m_read.value(QLatin1String("read")).toObject();
    const int incoming = rep.value(QLatin1String("entries")).toInt() - m_read.value(QLatin1String("dropped")).toInt();
    if (m_intoNew) {
        // A new termbase takes every entry.
        QJsonArray lines;
        for (const QJsonValue &s : m_read.value(QLatin1String("sample")).toArray())
            lines.append(QJsonObject{{QLatin1String("kind"), QStringLiteral("add")}, {QLatin1String("entry"), s.toString()}});
        m_plan = QJsonObject{{QLatin1String("added"), incoming}, {QLatin1String("actions"), incoming}, {QLatin1String("plan"), lines}};
        if (m_step == Review)
            render();
        return;
    }
    Connection *c = place();
    if (!c)
        return;
    QUrlQuery q = options(true);
    q.addQueryItem(QStringLiteral("dryRun"), QStringLiteral("1"));
    q.addQueryItem(QStringLiteral("status"), m_fallback);
    const bool none = m_destinationEntries == 0;
    q.addQueryItem(QStringLiteral("match"), none ? QStringLiteral("none") : m_match);
    q.addQueryItem(QStringLiteral("onMatch"), none ? QStringLiteral("add") : m_onMatch);
    if (!none && m_match == QLatin1String("term") && !m_matchLang.isEmpty())
        q.addQueryItem(QStringLiteral("lang"), m_matchLang);
    const int asked = ++m_asked;
    request(c->api()->upload("POST", QStringLiteral("/api/termbases/") + ApiClient::seg(m_termbase) + QStringLiteral("/import"), m_data, QStringLiteral("application/octet-stream"), q),
            [this, asked](const ApiResult &r) {
                if (asked != m_asked)
                    return;
                m_plan = r.ok() ? r.object() : QJsonObject();
                if (m_step == Review) {
                    render();
                    if (!r.ok())
                        setError(tr("The import could not be planned: %1").arg(r.error.message));
                }
            });
}

void ImportDialog::showReview()
{
    QWidget *host = m_content->parentWidget();
    if (!m_intoNew && m_destinationEntries > 0) {
        // .field-grid: how existing entries are found, and what happens.
        auto *grid = new QHBoxLayout;
        grid->setSpacing(S4);
        auto *left = new QVBoxLayout;
        left->setSpacing(S2);
        left->addWidget(bold(tr("Find existing entries by"), host));
        auto *match = new QButtonGroup(host);
        const std::pair<const char *, QString> ways[] = {
            {"term", tr("A shared term")},
            {"id", tr("The entry ID (for a file exported from this termbase)")},
            {"none", tr("Nothing: add every entry")},
        };
        for (const auto &[id, label] : ways) {
            auto *r = new QRadioButton(label, host);
            r->setObjectName(QStringLiteral("match:%1").arg(QLatin1String(id)));
            r->setChecked(m_match == QLatin1String(id));
            match->addButton(r);
            left->addWidget(r);
            const QString way = QString::fromLatin1(id);
            connect(r, &QRadioButton::clicked, this, [this, way] { setPolicy(way, m_matchLang, m_onMatch); });
            if (way == QLatin1String("term") && m_match == way) {
                auto *lang = new QComboBox(host);
                lang->setObjectName(QStringLiteral("matchLanguage"));
                lang->setAccessibleName(tr("Language of the shared term"));
                lang->addItem(tr("In any language"), QString());
                QStringList used;
                for (const QString &fl : m_fileLangs) {
                    const QString to = m_langMap.value(fl).toString();
                    if (!to.isEmpty() && !used.contains(to))
                        used << to;
                }
                for (const QString &l : used)
                    lang->addItem(tr("In %1").arg(Languages::name(l)), l);
                lang->setCurrentIndex(qMax(0, lang->findData(m_matchLang)));
                lang->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
                auto *nested = new QHBoxLayout;
                nested->setContentsMargins(26, 0, 0, 0);
                nested->addWidget(lang);
                nested->addStretch();
                left->addLayout(nested);
                connect(lang, &QComboBox::activated, this, [this, lang] { setPolicy(m_match, lang->currentData().toString(), m_onMatch); });
            }
        }
        left->addStretch();
        grid->addLayout(left, 1);
        auto *right = new QVBoxLayout;
        right->setSpacing(S2);
        if (m_match != QLatin1String("none")) {
            right->addWidget(bold(tr("When an entry exists"), host));
            auto *on = new QButtonGroup(host);
            const std::pair<const char *, QString> choices[] = {
                {"merge", tr("Merge: add the new terms and field values")},
                {"overwrite", tr("Replace it with the entry from the file")},
                {"skip", tr("Keep it and leave the file's entry out")},
                {"add", tr("Add the file's entry as a separate entry")},
            };
            for (const auto &[id, label] : choices) {
                auto *r = new QRadioButton(label, host);
                r->setObjectName(QStringLiteral("onMatch:%1").arg(QLatin1String(id)));
                r->setChecked(m_onMatch == QLatin1String(id));
                on->addButton(r);
                right->addWidget(r);
                const QString what = QString::fromLatin1(id);
                connect(r, &QRadioButton::clicked, this, [this, what] { setPolicy(m_match, m_matchLang, what); });
            }
        }
        right->addStretch();
        grid->addLayout(right, 1);
        m_content->addLayout(grid);
    }

    const bool planned = !m_plan.isEmpty();
    const int added = m_plan.value(QLatin1String("added")).toInt();
    const int merged = m_plan.value(QLatin1String("merged")).toInt();
    const int replaced = m_plan.value(QLatin1String("overwritten")).toInt();
    const int unchanged = m_plan.value(QLatin1String("skipped")).toInt() + m_plan.value(QLatin1String("unchanged")).toInt();
    if (planned) {
        // .plan
        m_content->addWidget(Web::text(tr("What the import does"), "settingsTitle", host));
        auto *counts = new QHBoxLayout;
        counts->setSpacing(S2);
        const std::pair<QString, int> boxes[] = {{tr("New entries"), added}, {tr("Merged"), merged}, {tr("Replaced"), replaced}, {tr("Unchanged"), unchanged}};
        for (const auto &[label, n] : boxes) {
            auto *box = new QFrame(host);
            Theme::role(box, "box");
            box->setAccessibleName(QStringLiteral("%1: %2").arg(label).arg(n));
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(S3, S2, S3, S2);
            bv->setSpacing(0);
            bv->addWidget(Web::text(label, "planLabel", box));
            auto *num = Web::text(QLocale().toString(n), "planCount", box);
            num->setObjectName(QStringLiteral("count:%1").arg(label));
            bv->addWidget(num);
            counts->addWidget(box, 1);
        }
        m_content->addLayout(counts);
        auto *warnings = new QVBoxLayout;
        warnings->setSpacing(S1);
        addWarnings(warnings, m_plan.value(QLatin1String("warnings")).toArray(), host);
        m_content->addLayout(warnings);

        const QJsonArray lines = m_plan.value(QLatin1String("plan")).toArray();
        auto *table = new Web::Table({tr("Entry in the file"), tr("Result"), tr("Existing entry")}, host);
        table->setObjectName(QStringLiteral("plan"));
        table->setProperty("inDialog", true);
        table->setAccessibleName(tr("Entries of the file and what happens to them"));
        table->setStretch(0);
        for (const QJsonValue &v : lines) {
            const QJsonObject a = v.toObject();
            const QString kind = str(a, "kind");
            const QString label = kind == QLatin1String("add") ? tr("New") : kind == QLatin1String("merge") ? tr("Merged") : kind == QLatin1String("overwrite") ? tr("Replaced") : tr("Unchanged");
            const QString chipKind = kind == QLatin1String("add") ? QStringLiteral("new") : kind == QLatin1String("merge") ? QStringLiteral("accent") : kind == QLatin1String("overwrite") ? QStringLiteral("changed") : QString();
            const int row = table->addRow({Web::cellText(str(a, "entry"), table), Web::chip(label, chipKind, table),
                                           Web::cellText(kind == QLatin1String("add") ? tr("none") : str(a, "existing"), table, false, kind == QLatin1String("add"))});
            table->item(row, 0)->setData(Qt::AccessibleTextRole, str(a, "entry"));
            table->item(row, 1)->setData(Qt::AccessibleTextRole, label);
            table->item(row, 2)->setData(Qt::AccessibleTextRole, kind == QLatin1String("add") ? tr("none") : str(a, "existing"));
        }
        table->fit();
        auto *wrap = new QScrollArea(host);
        Theme::role(wrap, "paneScroll");
        wrap->setWidgetResizable(true);
        wrap->setFrameShape(QFrame::NoFrame);
        wrap->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        wrap->setWidget(table);
        Web::fitScroll(wrap);
        wrap->setMaximumHeight(260);
        wrap->setMinimumHeight(qMin(260, table->sizeHint().height() + 4));
        m_content->addWidget(wrap);
        const int all = m_plan.value(QLatin1String("actions")).toInt();
        if (all > lines.size()) {
            const int more = all - int(lines.size());
            m_content->addWidget(Web::text(more == 1 ? tr("and 1 more entry") : tr("and %1 more entries").arg(QLocale().toString(more)), "help", host));
        }
    } else if (m_pending > 0) {
        m_content->addWidget(Web::text(tr("Working out what the import does…"), "help", host));
    }
    m_next->setEnabled(planned && (m_intoNew ? added > 0 : added + merged + replaced > 0));
}

void ImportDialog::back()
{
    setError(QString());
    const bool tabular = m_inspection.value(QLatin1String("tabular")).toBool();
    switch (m_step) {
    case File:
        return;
    case Columns:
        m_step = File;
        break;
    case LanguagesStep:
        m_step = tabular ? Columns : File;
        break;
    case Review:
        m_step = LanguagesStep;
        break;
    }
    render();
}

void ImportDialog::next()
{
    switch (m_step) {
    case File:
        return;
    case Columns:
        // The file is read with the columns as they are set now.
        inspect(true, [this](const QJsonObject &o) {
            if (o.value(QLatin1String("read")).toObject().value(QLatin1String("entries")).toInt() == 0) {
                setError(tr("%1 could not be read: the file has no entries with terms").arg(m_fileName));
                return;
            }
            m_read = o;
            m_langMap = {};
            m_step = LanguagesStep;
            readLanguages();
        });
        return;
    case LanguagesStep:
        m_step = Review;
        m_plan = {};
        render();
        loadPlan();
        return;
    case Review:
        doImport();
        return;
    }
}

void ImportDialog::doImport()
{
    Connection *c = place();
    if (!c)
        return;
    setError(QString());
    m_next->setEnabled(false);
    const QString connId = c->id();
    const auto send = [this, c, connId](const QString &name, bool created) {
        QUrlQuery q = options(true);
        q.addQueryItem(QStringLiteral("status"), m_fallback);
        const bool none = m_intoNew || m_destinationEntries == 0;
        q.addQueryItem(QStringLiteral("match"), none ? QStringLiteral("none") : m_match);
        q.addQueryItem(QStringLiteral("onMatch"), none ? QStringLiteral("add") : m_onMatch);
        if (!none && m_match == QLatin1String("term") && !m_matchLang.isEmpty())
            q.addQueryItem(QStringLiteral("lang"), m_matchLang);
        const QString route = QStringLiteral("/api/termbases/") + ApiClient::seg(name);
        request(c->api()->upload("POST", route + QStringLiteral("/import"), m_data, QStringLiteral("application/octet-stream"), q), [this, c, connId, name, created, route](const ApiResult &r) {
            if (!r.ok()) {
                // A termbase made for an import that failed is taken back.
                if (created)
                    c->api()->del(route);
                setError(tr("The import failed: %1").arg(r.error.message));
                m_next->setEnabled(true);
                return;
            }
            const QJsonObject o = r.object();
            QStringList parts;
            if (const int n = o.value(QLatin1String("added")).toInt())
                parts << tr("%1 added").arg(n);
            if (const int n = o.value(QLatin1String("merged")).toInt())
                parts << tr("%1 merged").arg(n);
            if (const int n = o.value(QLatin1String("overwritten")).toInt())
                parts << tr("%1 replaced").arg(n);
            if (const int n = o.value(QLatin1String("skipped")).toInt() + o.value(QLatin1String("unchanged")).toInt())
                parts << tr("%1 left as they were").arg(n);
            emit imported(connId, name, tr("Imported %1 into %2: %3").arg(m_fileName, name, parts.isEmpty() ? tr("nothing changed") : parts.join(QStringLiteral(", "))));
            accept();
        });
    };
    if (!m_intoNew) {
        send(m_termbase, false);
        return;
    }
    QString name = m_newName.trimmed();
    if (name.isEmpty())
        name = str(m_read, "name").trimmed();
    if (name.isEmpty())
        name = tr("Imported termbase");
    request(c->api()->post(QStringLiteral("/api/termbases"), QJsonObject{{QLatin1String("definition"), QJsonObject{{QLatin1String("name"), name}}}}), [this, send](const ApiResult &r) {
        if (!r.ok()) {
            setError(r.error.message);
            m_next->setEnabled(true);
            return;
        }
        send(str(r.object(), "name"), true);
    });
}
