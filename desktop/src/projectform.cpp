#include "projectform.h"

#include "apiclient.h"
#include "connection.h"
#include "icons.h"
#include "languages.h"
#include "theme.h"
#include "webui.h"
#include "words.h"

#include <QApplication>
#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QDateTimeEdit>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRadioButton>
#include <QSet>
#include <QSpinBox>
#include <QStyle>
#include <QVBoxLayout>

#include <algorithm>
#include <memory>

namespace {

const QLatin1String kName("name");

QStringList strings(const QJsonValue &v)
{
    QStringList out;
    for (const QJsonValue &x : v.toArray())
        out << x.toString();
    return out;
}

QJsonArray array(const QStringList &l)
{
    QJsonArray a;
    for (const QString &s : l)
        a.append(s);
    return a;
}

QString plural(qint64 n, const QString &one, const QString &many)
{
    return n == 1 ? one : many.arg(QLocale().toString(n));
}

void empty(QLayout *l)
{
    while (QLayoutItem *item = l->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->setParent(nullptr);
            w->deleteLater();
        } else if (item->layout()) {
            empty(item->layout());
        }
        delete item;
    }
}

// Empties a section so it can be filled again. The widgets are taken out
// at once (a search of the page must not find them) and deleted when the
// handler that asked for it has returned.
QVBoxLayout *reset(QWidget *section)
{
    auto *v = qobject_cast<QVBoxLayout *>(section->layout());
    if (!v) {
        v = new QVBoxLayout(section);
        v->setContentsMargins(0, 0, 0, 0);
        v->setSpacing(12);
        return v;
    }
    empty(v);
    return v;
}

// Fills a section again and gives the keyboard back to the control that
// had it, found by its name.
void refill(QWidget *section, const std::function<void()> &fill)
{
    if (!section)
        return;
    QString focused;
    QWidget *f = QApplication::focusWidget();
    if (f && section->isAncestorOf(f))
        focused = f->accessibleName();
    fill();
    if (focused.isEmpty())
        return;
    for (QWidget *w : section->findChildren<QWidget *>()) {
        if (w->accessibleName() == focused && w->focusPolicy() != Qt::NoFocus && w->isEnabled()) {
            w->setFocus();
            return;
        }
    }
}

// .field > legend, .field-label: the bold line over a group of controls.
QLabel *legend(const QString &text, QWidget *parent)
{
    auto *l = new QLabel(QStringLiteral("<b>%1</b>").arg(text.toHtmlEscaped()), parent);
    l->setAccessibleName(text);
    return l;
}

QPushButton *iconButton(const QString &icon, const QString &name, QWidget *parent)
{
    auto *b = new QPushButton(parent);
    b->setProperty("iconButton", true);
    b->setIcon(Icons::get(icon, Theme::color("text2")));
    b->setIconSize(QSize(14, 14));
    b->setAccessibleName(name);
    b->setToolTip(name);
    b->setCursor(Qt::PointingHandCursor);
    return b;
}

void repolish(QWidget *w)
{
    w->style()->unpolish(w);
    w->style()->polish(w);
}

// .choice with more than a description under its title: a tag beside the
// title and any parts below it.
QFrame *choiceWith(QAbstractButton *input, QWidget *tag, const QString &description, const QList<QWidget *> &below, QWidget *parent)
{
    auto *f = new QFrame(parent);
    f->setObjectName(QStringLiteral("choice"));
    auto *v = new QVBoxLayout(f);
    v->setContentsMargins(12, 12, 12, 12);
    v->setSpacing(4);
    input->setParent(f);
    QFont bold = input->font();
    bold.setWeight(QFont::DemiBold);
    input->setFont(bold);
    auto *top = new QHBoxLayout;
    top->setSpacing(8);
    top->addWidget(input);
    if (tag) {
        tag->setParent(f);
        top->addWidget(tag, 0, Qt::AlignVCenter);
    }
    top->addStretch();
    v->addLayout(top);
    if (!description.isEmpty()) {
        auto *d = Web::text(description, "secondary", f);
        d->setContentsMargins(26, 0, 0, 0);
        v->addWidget(d);
        input->setAccessibleDescription(description);
    }
    for (QWidget *w : below) {
        auto *row = new QHBoxLayout;
        row->setContentsMargins(26, 0, 0, 0);
        w->setParent(f);
        row->addWidget(w);
        v->addLayout(row);
    }
    const auto update = [f, input] {
        f->setProperty("checked", input->isChecked());
        repolish(f);
    };
    QObject::connect(input, &QAbstractButton::toggled, f, update);
    update();
    return f;
}

QWidget *indent(QWidget *w, int by = 26)
{
    w->setContentsMargins(by, 0, 0, 0);
    return w;
}

bool samePair(const QJsonObject &memory, const QString &source, const QString &target)
{
    return Languages::samePrimary(memory.value(QLatin1String("sourceLang")).toString(), source) &&
           Languages::samePrimary(memory.value(QLatin1String("targetLang")).toString(), target);
}

} // namespace

DueEdit::DueEdit(const QString &name, QWidget *parent)
    : QWidget(parent)
{
    auto *h = new QHBoxLayout(this);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(8);
    m_set = Web::button(tr("Set a date"), Web::Small, this, QStringLiteral("clock"));
    m_set->setAccessibleName(tr("%1: set a date").arg(name));
    m_edit = new QDateTimeEdit(this);
    m_edit->setAccessibleName(name);
    m_edit->setCalendarPopup(true);
    m_edit->setDisplayFormat(QLocale().dateFormat(QLocale::ShortFormat) + QLatin1Char(' ') + QLocale().timeFormat(QLocale::ShortFormat).remove(QStringLiteral(":ss")));
    m_clear = iconButton(QStringLiteral("x"), tr("%1: no date").arg(name), this);
    h->addWidget(m_set);
    h->addWidget(m_edit, 1);
    h->addWidget(m_clear);
    h->addStretch();
    show(false);
    connect(m_set, &QPushButton::clicked, this, [this] {
        // A week from now, at the end of the working day.
        m_edit->setDateTime(QDateTime(QDate::currentDate().addDays(7), QTime(17, 0)));
        show(true);
        m_edit->setFocus();
        emit changed();
    });
    connect(m_clear, &QPushButton::clicked, this, [this] {
        show(false);
        m_set->setFocus();
        emit changed();
    });
    connect(m_edit, &QDateTimeEdit::dateTimeChanged, this, [this] {
        if (m_has)
            emit changed();
    });
}

void DueEdit::show(bool set)
{
    m_has = set;
    m_set->setVisible(!set);
    m_edit->setVisible(set);
    m_clear->setVisible(set);
}

QString DueEdit::iso() const
{
    return m_has ? m_edit->dateTime().toUTC().toString(Qt::ISODate) : QString();
}

void DueEdit::setIso(const QString &iso)
{
    const QDateTime t = QDateTime::fromString(iso, Qt::ISODate);
    const QSignalBlocker block(m_edit);
    if (t.isValid())
        m_edit->setDateTime(t.toLocalTime());
    show(t.isValid());
}

ProjectForm::ProjectForm(QObject *parent)
    : QObject(parent)
{
}

void ProjectForm::setCatalog(const QJsonObject &catalog, const QJsonArray &dictionaries, const QJsonArray &qaKinds)
{
    m_catalog = catalog;
    m_dicts = dictionaries;
    m_kinds = qaKinds;
}

void ProjectForm::load(Connection *c, std::function<void(const QString &)> done)
{
    m_conn = c;
    auto pending = std::make_shared<int>(4);
    auto catalog = std::make_shared<QJsonObject>();
    auto dicts = std::make_shared<QJsonArray>();
    auto kinds = std::make_shared<QJsonArray>();
    auto error = std::make_shared<QString>();
    const auto finish = [this, pending, catalog, dicts, kinds, error, done] {
        if (--*pending > 0)
            return;
        if (error->isEmpty())
            setCatalog(*catalog, *dicts, *kinds);
        done(*error);
    };
    onResult(c->api()->get(QStringLiteral("/api/catalog")), this, [catalog, error, finish](const ApiResult &r) {
        if (r.ok())
            *catalog = r.object();
        else
            *error = r.error.message;
        finish();
    });
    onResult(c->api()->get(QStringLiteral("/api/dictionaries")), this, [dicts, error, finish](const ApiResult &r) {
        if (r.ok())
            *dicts = r.array();
        else
            *error = r.error.message;
        finish();
    });
    onResult(c->api()->get(QStringLiteral("/api/providers")), this, [this, error, finish](const ApiResult &r) {
        m_grammar.clear();
        if (r.ok()) {
            for (const QJsonValue &v : r.array()) {
                if (v.toObject().value(QLatin1String("kind")).toString() == QLatin1String("check"))
                    m_grammar << v.toObject().value(QLatin1String("name")).toString();
            }
        } else {
            *error = r.error.message;
        }
        finish();
    });
    onResult(c->api()->get(QStringLiteral("/api/qa/kinds")), this, [kinds, error, finish](const ApiResult &r) {
        if (r.ok())
            *kinds = r.array();
        else
            *error = r.error.message;
        finish();
    });
}

QString ProjectForm::host() const
{
    return m_conn && !m_conn->isLocal() ? m_conn->displayName() : QString();
}

void ProjectForm::setSettings(const QJsonObject &settings, bool newProject)
{
    m_s = settings;
    m_new = newProject;
    m_checksTouched = false;
    m_problems.clear();
    m_created.clear();
    m_creatingFor.clear();
    pickDictionaries();
    fillDetails();
    fillTargets();
    fillMemories();
    fillTerminology();
    fillChecks();
    fillPrepare();
    fillSegmentation();
}

QJsonObject ProjectForm::settings() const
{
    QJsonObject s = m_s;
    s.insert(QStringLiteral("name"), s.value(kName).toString().trimmed());
    s.insert(QStringLiteral("client"), s.value(QLatin1String("client")).toString().trimmed());
    s.insert(QStringLiteral("description"), s.value(QLatin1String("description")).toString().trimmed());
    // Only the target languages have memories and dictionaries.
    const QStringList ts = targetList();
    QJsonObject mems;
    for (const QString &t : ts)
        mems.insert(t, s.value(QLatin1String("memories")).toObject().value(t).toArray());
    s.insert(QStringLiteral("memories"), mems);
    QJsonObject qa = s.value(QLatin1String("qa")).toObject();
    QJsonObject dicts;
    const QJsonObject chosen = qa.value(QLatin1String("dictionaries")).toObject();
    for (const QString &t : ts) {
        if (!chosen.value(t).toString().isEmpty())
            dicts.insert(t, chosen.value(t));
    }
    qa.insert(QStringLiteral("dictionaries"), dicts);
    s.insert(QStringLiteral("qa"), qa);
    QJsonObject term = s.value(QLatin1String("terminology")).toObject();
    const QString mode = term.value(QLatin1String("mode")).toString();
    if (mode == QLatin1String("own"))
        term.insert(QStringLiteral("masters"), QJsonArray());
    if (mode != QLatin1String("direct"))
        term.remove(QStringLiteral("writeTo"));
    if (mode != QLatin1String("layered"))
        term.insert(QStringLiteral("autoUpdate"), false);
    // The app's project termbases are kept where the project is.
    term.insert(QStringLiteral("location"), QStringLiteral("server"));
    s.insert(QStringLiteral("terminology"), term);
    return s;
}

void ProjectForm::set(const QString &key, const QJsonValue &value)
{
    if (m_s.value(key) == value)
        return;
    m_s.insert(key, value);
    if (key == QLatin1String("sourceLang") || key == QLatin1String("targetLangs")) {
        dropStaleMemories();
        fillTargets();
        fillMemories();
        fillTerminology();
        fillChecks();
    } else {
        fillDetails();
    }
}

void ProjectForm::setReadOnly(bool readOnly)
{
    m_readOnly = readOnly;
    for (const QPointer<QWidget> &w : {m_details, m_targets, m_memories, m_terminology, m_checks, m_prepare, m_segmentation}) {
        if (w)
            w->setEnabled(!readOnly);
    }
}

void ProjectForm::setSuggestedTargets(const QStringList &targets)
{
    if (m_suggested == targets)
        return;
    m_suggested = targets;
    fillTargets();
}

void ProjectForm::setFixedTargets(const QStringList &targets)
{
    if (m_fixed == targets)
        return;
    m_fixed = targets;
    fillTargets();
}

QStringList ProjectForm::targetList() const
{
    return strings(m_s.value(QLatin1String("targetLangs")));
}

QString ProjectForm::source() const
{
    return m_s.value(QLatin1String("sourceLang")).toString();
}

void ProjectForm::touch()
{
    emit changed();
}

QStringList ProjectForm::problemsAt(const QString &field) const
{
    QStringList out;
    for (const Problem &p : m_problems) {
        if (p.field == field || p.field.startsWith(field + QLatin1Char('.')) || p.field.startsWith(field + QLatin1Char('[')))
            out << p.message;
    }
    return out;
}

QLabel *ProjectForm::problemLabel(const QString &field, QWidget *parent)
{
    auto *l = Web::text(QString(), "error", parent);
    const QStringList ps = problemsAt(field);
    l->setText(ps.join(QLatin1Char('\n')));
    l->setVisible(!ps.isEmpty());
    m_problemLabels.append({field, l});
    return l;
}

QStringList ProjectForm::showProblems(const QList<Problem> &problems)
{
    m_problems = problems;
    QStringList placed;
    for (auto it = m_problemLabels.begin(); it != m_problemLabels.end();) {
        if (!it->second) {
            it = m_problemLabels.erase(it);
            continue;
        }
        const QStringList ps = problemsAt(it->first);
        it->second->setText(ps.join(QLatin1Char('\n')));
        it->second->setVisible(!ps.isEmpty());
        placed << ps;
        ++it;
    }
    QStringList rest;
    for (const Problem &p : problems) {
        if (!placed.contains(p.message))
            rest << p.message;
    }
    return rest;
}

// Details: the name, the client, when it is due, and a description.

QWidget *ProjectForm::details(QWidget *parent)
{
    if (!m_details) {
        m_details = new QWidget(parent);
        fillDetails();
    }
    return m_details;
}

void ProjectForm::fillDetails()
{
    if (!m_details)
        return;
    QWidget *w = m_details;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        auto *grid = new QGridLayout;
        grid->setContentsMargins(0, 0, 0, 0);
        grid->setHorizontalSpacing(16);
        grid->setVerticalSpacing(16);
        grid->setColumnStretch(0, 1);
        grid->setColumnStretch(1, 1);

        auto *name = new QLineEdit(m_s.value(kName).toString(), w);
        name->setAccessibleName(tr("Project name"));
        auto *nameField = Web::field(tr("Project name"), name, w);
        nameField->layout()->addWidget(problemLabel(QStringLiteral("name"), nameField));
        grid->addWidget(nameField, 0, 0, Qt::AlignTop);
        connect(name, &QLineEdit::textEdited, this, [this](const QString &t) {
            m_s.insert(QStringLiteral("name"), t);
            touch();
        });

        auto *client = new QLineEdit(m_s.value(QLatin1String("client")).toString(), w);
        client->setAccessibleName(tr("Client"));
        auto *clientField = Web::field(tr("Client"), client, w, true, m_new ? tr("Used to suggest the client's memories and termbases.") : QString());
        clientField->layout()->addWidget(problemLabel(QStringLiteral("client"), clientField));
        grid->addWidget(clientField, 0, 1, Qt::AlignTop);
        connect(client, &QLineEdit::textEdited, this, [this](const QString &t) {
            m_s.insert(QStringLiteral("client"), t);
            touch();
        });

        auto *due = new DueEdit(tr("Due"), w);
        due->setIso(m_s.value(QLatin1String("due")).toString());
        auto *dueField = Web::field(tr("Due"), due, w, true);
        dueField->layout()->addWidget(problemLabel(QStringLiteral("due"), dueField));
        grid->addWidget(dueField, 1, 0, Qt::AlignTop);
        connect(due, &DueEdit::changed, this, [this, due] {
            m_s.insert(QStringLiteral("due"), due->iso());
            touch();
        });

        auto *desc = new QLineEdit(m_s.value(QLatin1String("description")).toString(), w);
        desc->setAccessibleName(tr("Description"));
        grid->addWidget(Web::field(tr("Description"), desc, w, true), 1, 1, Qt::AlignTop);
        connect(desc, &QLineEdit::textEdited, this, [this](const QString &t) {
            m_s.insert(QStringLiteral("description"), t);
            touch();
        });
        v->addLayout(grid);
        w->setEnabled(!m_readOnly);
    });
}

// Target languages: suggestions as check boxes, any other from the list.

QWidget *ProjectForm::targets(QWidget *parent)
{
    if (!m_targets) {
        m_targets = new QWidget(parent);
        fillTargets();
    }
    return m_targets;
}

void ProjectForm::fillTargets()
{
    if (!m_targets)
        return;
    QWidget *w = m_targets;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(8);
        const QString src = source();
        const QStringList chosen = targetList();
        QStringList quick;
        for (const QString &t : m_suggested + chosen) {
            if (!quick.contains(t) && !t.isEmpty() && t != src)
                quick << t;
        }
        for (const QString &t : std::as_const(quick)) {
            const bool fixed = m_fixed.contains(t);
            auto *box = new QCheckBox(fixed ? tr("%1 (a bilingual file is for it)").arg(Languages::name(t)) : Languages::name(t), w);
            box->setAccessibleName(Languages::name(t));
            box->setChecked(chosen.contains(t));
            box->setEnabled(!fixed);
            v->addWidget(box);
            connect(box, &QCheckBox::toggled, this, [this, t](bool on) {
                QStringList ts = targetList();
                if (on && !ts.contains(t))
                    ts << t;
                if (!on)
                    ts.removeAll(t);
                m_s.insert(QStringLiteral("targetLangs"), array(ts));
                dropStaleMemories();
                // The box stays where it is; what follows the languages is shown again.
                fillMemories();
                fillTerminology();
                fillChecks();
                touch();
            });
        }
        auto *add = new LanguageCombo(w);
        add->setAccessibleName(tr("Add another target language"));
        add->lineEdit()->setPlaceholderText(tr("Add another language…"));
        // The source and the languages already offered are not offered again.
        for (int i = add->count() - 1; i >= 0; --i) {
            const QString t = add->itemData(i).toString();
            if (t == src || quick.contains(t))
                add->removeItem(i);
        }
        add->setCurrentIndex(-1);
        add->setMinimumWidth(280);
        auto *button = Web::button(tr("Add"), Web::Small, w, QStringLiteral("plus"));
        button->setAccessibleName(tr("Add the target language"));
        button->setEnabled(false);
        v->addWidget(Web::row({add, button}, w));
        const auto usable = [this, add] {
            const QString t = add->language();
            return !t.isEmpty() && t != source() && !targetList().contains(t);
        };
        connect(add, &QComboBox::currentTextChanged, button, [button, usable] { button->setEnabled(usable()); });
        const auto addIt = [this, add, usable] {
            if (!usable())
                return;
            QStringList ts = targetList();
            ts << add->language();
            m_s.insert(QStringLiteral("targetLangs"), array(ts));
            pickDictionaries();
            fillTargets();
            fillMemories();
            fillTerminology();
            fillChecks();
            touch();
        };
        connect(button, &QPushButton::clicked, this, addIt);
        connect(add->lineEdit(), &QLineEdit::returnPressed, this, addIt);
        v->addWidget(problemLabel(QStringLiteral("targetLangs"), w));
        w->setEnabled(!m_readOnly);
    });
}

// The dictionaries that spell a language: the same region first, then the
// language; those installed before those that come with Interlinea.
QList<QJsonObject> ProjectForm::dictionariesFor(const QString &lang) const
{
    QList<QJsonObject> out[4];
    for (const QJsonValue &dv : std::as_const(m_dicts)) {
        const QJsonObject d = dv.toObject();
        const QString l = d.value(QLatin1String("lang")).toString();
        if (!Languages::samePrimary(l, lang))
            continue;
        const bool included = d.value(QLatin1String("included")).toBool();
        out[(l == lang ? 0 : 2) + (included ? 1 : 0)].append(d);
    }
    return out[0] + out[1] + out[2] + out[3];
}

// A new project spells each target language with the dictionary that fits
// it, until the checks are changed by hand.
void ProjectForm::pickDictionaries()
{
    if (!m_new || m_checksTouched)
        return;
    QJsonObject qa = m_s.value(QLatin1String("qa")).toObject();
    QJsonObject dicts = qa.value(QLatin1String("dictionaries")).toObject();
    bool changed = false;
    for (const QString &t : targetList()) {
        const QString set = dicts.value(t).toString();
        const bool known = std::any_of(m_dicts.begin(), m_dicts.end(), [&set](const QJsonValue &d) {
            return d.toObject().value(QLatin1String("name")).toString() == set;
        });
        if (!set.isEmpty() && known)
            continue;
        const QList<QJsonObject> fit = dictionariesFor(t);
        if (fit.isEmpty())
            continue;
        dicts.insert(t, fit.first().value(QLatin1String("name")).toString());
        changed = true;
    }
    if (!changed)
        return;
    qa.insert(QStringLiteral("dictionaries"), dicts);
    m_s.insert(QStringLiteral("qa"), qa);
}

// Languages no longer targets lose their memories and dictionaries.
void ProjectForm::dropStaleMemories()
{
    const QStringList ts = targetList();
    QJsonObject mems = m_s.value(QLatin1String("memories")).toObject();
    for (const QString &k : mems.keys()) {
        if (!ts.contains(k))
            mems.remove(k);
    }
    m_s.insert(QStringLiteral("memories"), mems);
    QJsonObject qa = m_s.value(QLatin1String("qa")).toObject();
    QJsonObject dicts = qa.value(QLatin1String("dictionaries")).toObject();
    for (const QString &k : dicts.keys()) {
        if (!ts.contains(k))
            dicts.remove(k);
    }
    qa.insert(QStringLiteral("dictionaries"), dicts);
    m_s.insert(QStringLiteral("qa"), qa);
    pickDictionaries();
    // A memory made here for a language that is gone is not made.
    for (int i = int(m_created.size()) - 1; i >= 0; --i) {
        if (!ts.contains(m_created[i].targetLang))
            m_created.removeAt(i);
    }
    if (!ts.contains(m_creatingFor))
        m_creatingFor.clear();
}

// Memories per target language: which, in what order, used for what, and
// with what penalty; new memories are made here, empty or from TMX.

QJsonObject ProjectForm::memoryInfo(const QString &name) const
{
    for (const QJsonValue &v : m_catalog.value(QLatin1String("memories")).toArray()) {
        if (v.toObject().value(kName).toString() == name)
            return v.toObject();
    }
    for (const NewMemory &n : m_created) {
        if (n.name == name)
            return QJsonObject{{QStringLiteral("name"), n.name}, {QStringLiteral("sourceLang"), n.sourceLang}, {QStringLiteral("targetLang"), n.targetLang},
                               {QStringLiteral("new"), true}, {QStringLiteral("tmx"), n.tmxPath}};
    }
    return {};
}

QWidget *ProjectForm::memories(QWidget *parent)
{
    if (!m_memories) {
        m_memories = new QWidget(parent);
        fillMemories();
    }
    return m_memories;
}

void ProjectForm::fillMemories()
{
    if (!m_memories)
        return;
    QWidget *w = m_memories;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(16);
        const bool own = m_s.value(QLatin1String("projectMemory")).toBool();
        if (m_new) {
            auto *box = new QCheckBox(tr("Give the project its own memory for each language"), w);
            box->setChecked(own);
            auto *help = Web::text(QString(), "help", w);
            const auto say = [help](bool on) {
                help->setText(on ? tr("Confirmed translations go into the project memory first. When the project is done, you add them to a main memory from the project's settings. The project memory is searched before the memories below.")
                                 : tr("Confirmed translations go only into the memories below that you update."));
            };
            say(own);
            auto *f = new QWidget(w);
            auto *fv = new QVBoxLayout(f);
            fv->setContentsMargins(0, 0, 0, 0);
            fv->setSpacing(8);
            fv->addWidget(box);
            fv->addWidget(help);
            v->addWidget(f);
            connect(box, &QCheckBox::toggled, this, [this](bool on) {
                m_s.insert(QStringLiteral("projectMemory"), on);
                // The lists say what happens without a memory chosen.
                fillMemories();
                touch();
            });
        }
        const QString src = source();
        const QJsonObject all = m_s.value(QLatin1String("memories")).toObject();
        const auto setFor = [this](const QString &lang, const QJsonArray &list) {
            QJsonObject mems = m_s.value(QLatin1String("memories")).toObject();
            mems.insert(lang, list);
            m_s.insert(QStringLiteral("memories"), mems);
            touch();
        };
        for (const QString &lang : targetList()) {
            auto *set = new QWidget(w);
            set->setAccessibleName(tr("Memories for %1").arg(Languages::name(lang)));
            auto *sv = new QVBoxLayout(set);
            sv->setContentsMargins(0, 0, 0, 0);
            sv->setSpacing(8);
            sv->addWidget(legend(tr("%1 to %2").arg(Languages::name(src), Languages::name(lang)), set));
            const QJsonArray list = all.value(lang).toArray();
            QStringList inList;
            if (list.isEmpty()) {
                sv->addWidget(Web::text(own && m_new ? tr("No memory chosen. Matches come only from the project memory.")
                                                     : m_new ? tr("No memory chosen. The editor will show no matches.") : tr("No memory chosen. The editor shows no matches."),
                                        "muted", set));
            } else {
                auto *table = new Web::Table({tr("Memory"), tr("Lookup"), tr("Concordance"), tr("Update"), tr("Penalty"), QString()}, set);
                table->setAccessibleName(tr("Memories for %1, highest priority first").arg(Languages::name(lang)));
                table->setStretch(0);
                for (int i = 0; i < list.size(); ++i) {
                    const QJsonObject m = list.at(i).toObject();
                    const QString name = m.value(QLatin1String("memory")).toString();
                    inList << name;
                    const QJsonObject info = memoryInfo(name);
                    QString sub;
                    if (info.isEmpty())
                        sub = tr("This memory no longer exists.");
                    else if (info.value(QLatin1String("new")).toBool())
                        sub = info.value(QLatin1String("tmx")).toString().isEmpty()
                            ? tr("%1 → %2, new and empty").arg(info.value(QLatin1String("sourceLang")).toString(), info.value(QLatin1String("targetLang")).toString())
                            : tr("%1 → %2, new, filled from %3")
                                  .arg(info.value(QLatin1String("sourceLang")).toString(), info.value(QLatin1String("targetLang")).toString(),
                                       QFileInfo(info.value(QLatin1String("tmx")).toString()).fileName());
                    else
                        sub = QStringLiteral("%1 → %2 %3")
                                  .arg(info.value(QLatin1String("sourceLang")).toString(), info.value(QLatin1String("targetLang")).toString(),
                                       plural(info.value(QLatin1String("units")).toInt(), tr("1 unit"), tr("%1 units")));
                    QList<QWidget *> cells{Web::cell(name, sub, table)};
                    struct Use {
                        const char *key;
                        QString name;
                    };
                    for (const Use &u : {Use{"lookup", tr("Look up matches in %1").arg(name)}, Use{"concordance", tr("Search with concordance in %1").arg(name)},
                                         Use{"update", tr("Store confirmed translations in %1").arg(name)}}) {
                        auto *box = new QCheckBox(table);
                        box->setAccessibleName(u.name);
                        box->setToolTip(u.name);
                        box->setChecked(m.value(QLatin1String(u.key)).toBool());
                        const QString key = QLatin1String(u.key);
                        connect(box, &QCheckBox::toggled, this, [this, lang, i, key, setFor](bool on) {
                            QJsonArray a = m_s.value(QLatin1String("memories")).toObject().value(lang).toArray();
                            QJsonObject o = a.at(i).toObject();
                            o.insert(key, on);
                            a.replace(i, o);
                            setFor(lang, a);
                        });
                        cells << box;
                    }
                    auto *pen = new QSpinBox(table);
                    pen->setRange(0, 100);
                    pen->setValue(m.value(QLatin1String("penalty")).toInt());
                    pen->setAccessibleName(tr("Penalty for %1, in points").arg(name));
                    pen->setFixedWidth(72);
                    connect(pen, &QSpinBox::valueChanged, this, [this, lang, i, setFor](int value) {
                        QJsonArray a = m_s.value(QLatin1String("memories")).toObject().value(lang).toArray();
                        QJsonObject o = a.at(i).toObject();
                        o.insert(QStringLiteral("penalty"), value);
                        a.replace(i, o);
                        setFor(lang, a);
                    });
                    cells << pen;
                    auto *up = iconButton(QStringLiteral("arrow-up"), tr("Move %1 up").arg(name), table);
                    up->setEnabled(i > 0);
                    auto *down = iconButton(QStringLiteral("arrow-down"), tr("Move %1 down").arg(name), table);
                    down->setEnabled(i < list.size() - 1);
                    auto *remove = iconButton(QStringLiteral("x"), tr("Remove %1").arg(name), table);
                    const auto move = [this, lang, i, setFor](int to) {
                        QJsonArray a = m_s.value(QLatin1String("memories")).toObject().value(lang).toArray();
                        if (to < 0 || to >= a.size())
                            return;
                        const QJsonValue x = a.at(i);
                        a.replace(i, a.at(to));
                        a.replace(to, x);
                        setFor(lang, a);
                        fillMemories();
                    };
                    connect(up, &QPushButton::clicked, this, [move, i] { move(i - 1); });
                    connect(down, &QPushButton::clicked, this, [move, i] { move(i + 1); });
                    connect(remove, &QPushButton::clicked, this, [this, lang, i, name, setFor] {
                        QJsonArray a = m_s.value(QLatin1String("memories")).toObject().value(lang).toArray();
                        a.removeAt(i);
                        // A memory made here and taken out again is not made.
                        bool elsewhere = false;
                        const QJsonObject mems = m_s.value(QLatin1String("memories")).toObject();
                        for (const QString &k : mems.keys()) {
                            for (const QJsonValue &x : (k == lang ? a : mems.value(k).toArray()))
                                elsewhere = elsewhere || x.toObject().value(QLatin1String("memory")).toString() == name;
                        }
                        if (!elsewhere) {
                            for (int j = int(m_created.size()) - 1; j >= 0; --j) {
                                if (m_created[j].name == name)
                                    m_created.removeAt(j);
                            }
                        }
                        setFor(lang, a);
                        fillMemories();
                    });
                    cells << Web::row({up, down, remove}, table, 2);
                    table->addRow(cells);
                }
                table->fit();
                sv->addWidget(table);
            }

            // Adding one: a memory of the language pair, or a new one.
            auto *pick = new QComboBox(set);
            pick->setAccessibleName(tr("Add a memory for %1").arg(Languages::name(lang)));
            pick->setMinimumWidth(280);
            QJsonArray fitting = m_catalog.value(QLatin1String("memories")).toArray();
            for (const NewMemory &n : std::as_const(m_created))
                fitting.append(QJsonObject{{QStringLiteral("name"), n.name}, {QStringLiteral("sourceLang"), n.sourceLang}, {QStringLiteral("targetLang"), n.targetLang}});
            for (const QJsonValue &mv : std::as_const(fitting)) {
                const QJsonObject m = mv.toObject();
                const QString name = m.value(kName).toString();
                if (!inList.contains(name) && samePair(m, src, lang))
                    pick->addItem(name, name);
            }
            pick->insertItem(0, pick->count() ? tr("Add a memory…") : tr("No other memory fits"), QString());
            pick->setCurrentIndex(0);
            auto *add = Web::button(tr("Add"), Web::Small, set, QStringLiteral("plus"));
            add->setAccessibleName(tr("Add the memory for %1").arg(Languages::name(lang)));
            add->setEnabled(false);
            connect(pick, &QComboBox::currentIndexChanged, add, [add, pick] { add->setEnabled(!pick->currentData().toString().isEmpty()); });
            const auto append = [this, lang, setFor](const QString &name) {
                QJsonArray a = m_s.value(QLatin1String("memories")).toObject().value(lang).toArray();
                // Updated only when there is no project memory to collect translations.
                const bool collect = m_new && m_s.value(QLatin1String("projectMemory")).toBool();
                a.append(QJsonObject{{QStringLiteral("memory"), name}, {QStringLiteral("lookup"), true}, {QStringLiteral("concordance"), true},
                                     {QStringLiteral("update"), !collect}, {QStringLiteral("penalty"), 0}});
                setFor(lang, a);
            };
            connect(add, &QPushButton::clicked, this, [this, pick, append] {
                if (pick->currentData().toString().isEmpty())
                    return;
                append(pick->currentData().toString());
                fillMemories();
            });
            QList<QWidget *> addRow{pick, add};
            if (m_creatingFor != lang) {
                auto *fresh = Web::button(tr("New memory"), Web::Quiet | Web::Small, set);
                fresh->setAccessibleName(tr("New memory for %1").arg(Languages::name(lang)));
                connect(fresh, &QPushButton::clicked, this, [this, lang] {
                    m_creatingFor = lang;
                    fillMemories();
                });
                addRow << fresh;
            }
            sv->addWidget(Web::row(addRow, set));

            if (m_creatingFor == lang) {
                auto *form = new QFrame(set);
                form->setObjectName(QStringLiteral("choice"));
                form->setAccessibleName(tr("New memory for %1").arg(Languages::name(lang)));
                auto *fv = new QVBoxLayout(form);
                fv->setContentsMargins(12, 12, 12, 12);
                fv->setSpacing(12);
                const QString base = !m_s.value(QLatin1String("client")).toString().trimmed().isEmpty() ? m_s.value(QLatin1String("client")).toString().trimmed()
                    : !m_s.value(kName).toString().trimmed().isEmpty()                                  ? m_s.value(kName).toString().trimmed()
                                                                                                        : tr("Memory");
                auto *name = new QLineEdit(QStringLiteral("%1 (%2-%3)").arg(base, src.toUpper(), lang.toUpper()), form);
                name->setAccessibleName(tr("Name of the new memory"));
                auto *nameError = Web::text(QString(), "error", form);
                auto *nameField = Web::field(tr("Name of the new memory"), name, form);
                nameField->layout()->addWidget(nameError);
                fv->addWidget(nameField);
                auto *tmx = new QLabel(tr("No file chosen"), form);
                tmx->setProperty("path", QString());
                Theme::role(tmx, "muted");
                auto *choose = Web::button(tr("Choose a file…"), Web::Small, form);
                choose->setAccessibleName(tr("Fill it from a TMX file"));
                fv->addWidget(Web::field(tr("Fill it from a TMX file"), Web::row({choose, tmx}, form), form, true));
                connect(choose, &QPushButton::clicked, form, [form, tmx] {
                    const QString path = QFileDialog::getOpenFileName(form, tr("A TMX file"), QString(), tr("TMX files (*.tmx);;All files (*)"));
                    if (path.isEmpty())
                        return;
                    tmx->setProperty("path", path);
                    tmx->setText(QFileInfo(path).fileName());
                });
                auto *create = Web::button(tr("Create memory"), Web::Primary | Web::Small, form);
                auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, form);
                cancel->setAccessibleName(tr("Cancel the new memory"));
                fv->addWidget(Web::row({create, cancel}, form));
                const auto check = [this, name, nameError, create] {
                    const QString n = name->text().trimmed();
                    bool clash = false;
                    for (const QJsonValue &v : m_catalog.value(QLatin1String("memories")).toArray())
                        clash = clash || v.toObject().value(kName).toString().compare(n, Qt::CaseInsensitive) == 0;
                    for (const NewMemory &x : std::as_const(m_created))
                        clash = clash || x.name.compare(n, Qt::CaseInsensitive) == 0;
                    const QString problem = n.isEmpty() ? tr("Give the memory a name.") : clash ? tr("A memory with this name exists already.") : QString();
                    nameError->setText(problem);
                    nameError->setVisible(!problem.isEmpty());
                    create->setEnabled(problem.isEmpty());
                    return problem.isEmpty();
                };
                check();
                connect(name, &QLineEdit::textChanged, form, check);
                connect(create, &QPushButton::clicked, this, [this, check, name, tmx, lang, src, append] {
                    if (!check())
                        return;
                    const QString path = tmx->property("path").toString();
                    m_created.append({name->text().trimmed(), src, lang, path});
                    m_creatingFor.clear();
                    append(name->text().trimmed());
                    fillMemories();
                });
                connect(cancel, &QPushButton::clicked, this, [this] {
                    m_creatingFor.clear();
                    fillMemories();
                });
                sv->addWidget(form);
            }
            sv->addWidget(problemLabel(QStringLiteral("memories.%1").arg(lang), set));
            v->addWidget(set);
        }
        v->addWidget(Web::text(tr("A penalty takes points off a memory's matches, for memories you trust less: with 2 points, an exact match shows as 98% and is not inserted as an exact match."),
                               "help", w));
        w->setEnabled(!m_readOnly);
    });
}

QJsonArray ProjectForm::newMemories() const
{
    QJsonArray out;
    for (const NewMemory &n : m_created)
        out.append(QJsonObject{{QStringLiteral("Name"), n.name}, {QStringLiteral("SourceLang"), n.sourceLang}, {QStringLiteral("TargetLang"), n.targetLang}});
    return out;
}

void ProjectForm::importNewMemories(Connection *c, std::function<void(const QString &)> done)
{
    QList<NewMemory> files;
    for (const NewMemory &n : std::as_const(m_created)) {
        if (!n.tmxPath.isEmpty())
            files << n;
    }
    m_created.clear();
    auto errors = std::make_shared<QStringList>();
    auto next = std::make_shared<std::function<void(int)>>();
    *next = [this, c, files, errors, next, done](int i) {
        if (i >= files.size()) {
            done(errors->join(QLatin1Char(' ')));
            return;
        }
        const NewMemory n = files.at(i);
        QFile f(n.tmxPath);
        if (!f.open(QIODevice::ReadOnly)) {
            *errors << tr("%1 can't be read: %2").arg(QFileInfo(n.tmxPath).fileName(), f.errorString());
            (*next)(i + 1);
            return;
        }
        onResult(c->api()->upload("POST", QStringLiteral("/api/memories/%1/import").arg(ApiClient::seg(n.name)), f.readAll(), QStringLiteral("application/xml")), this,
                 [n, errors, next, i](const ApiResult &r) {
                     if (!r.ok())
                         *errors << ProjectForm::tr("%1 was not read into %2: %3").arg(QFileInfo(n.tmxPath).fileName(), n.name, r.error.message);
                     (*next)(i + 1);
                 });
    };
    (*next)(0);
}

void ProjectForm::createNewMemories(Connection *c, std::function<void(const QString &)> done)
{
    const QList<NewMemory> all = m_created;
    auto next = std::make_shared<std::function<void(int)>>();
    *next = [this, c, all, next, done](int i) {
        if (i >= all.size()) {
            importNewMemories(c, done);
            return;
        }
        const NewMemory n = all.at(i);
        onResult(c->api()->post(QStringLiteral("/api/memories"),
                                QJsonObject{{QStringLiteral("name"), n.name}, {QStringLiteral("sourceLang"), n.sourceLang}, {QStringLiteral("targetLang"), n.targetLang}}),
                 this, [this, n, next, i, done](const ApiResult &r) {
                     // One made by an earlier try that failed later is there already.
                     if (!r.ok() && r.error.code != QLatin1String("exists")) {
                         done(tr("The memory %1 was not made: %2").arg(n.name, r.error.message));
                         return;
                     }
                     // It is on the connection now: the catalog knows it.
                     QJsonArray mems = m_catalog.value(QLatin1String("memories")).toArray();
                     mems.append(QJsonObject{{QStringLiteral("name"), n.name}, {QStringLiteral("sourceLang"), n.sourceLang}, {QStringLiteral("targetLang"), n.targetLang},
                                             {QStringLiteral("units"), 0}});
                     m_catalog.insert(QStringLiteral("memories"), mems);
                     (*next)(i + 1);
                 });
    };
    (*next)(0);
}

// Terminology: which masters the project uses and where new terms go. On
// an existing project the termbase setup itself stays; masters and
// updating can change.

QWidget *ProjectForm::terminology(QWidget *parent)
{
    if (!m_terminology) {
        m_terminology = new QWidget(parent);
        fillTerminology();
    }
    return m_terminology;
}

void ProjectForm::fillTerminology()
{
    if (!m_terminology)
        return;
    QWidget *w = m_terminology;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(16);
        const QJsonObject t = m_s.value(QLatin1String("terminology")).toObject();
        const QString mode = t.value(QLatin1String("mode")).toString(QStringLiteral("own"));
        const QStringList used = strings(t.value(QLatin1String("masters")));
        const QStringList addLangs = strings(t.value(QLatin1String("addLanguages")));
        const QString writeTo = t.value(QLatin1String("writeTo")).toString();
        const QStringList langs = QStringList{source()} + targetList();
        const auto setTerm = [this](const std::function<void(QJsonObject &)> &change) {
            QJsonObject x = m_s.value(QLatin1String("terminology")).toObject();
            change(x);
            m_s.insert(QStringLiteral("terminology"), x);
            touch();
        };

        // The masters: those on the connection, and any the project uses
        // that are kept elsewhere.
        QJsonArray masters;
        QStringList known;
        const QString own = t.value(QLatin1String("termbase")).toString();
        for (const QJsonValue &tv : m_catalog.value(QLatin1String("termbases")).toArray()) {
            const QJsonObject tb = tv.toObject();
            if (tb.value(QLatin1String("kind")).toString() == QLatin1String("project") || tb.value(kName).toString() == own)
                continue;
            masters.append(tb);
            known << tb.value(kName).toString();
        }
        for (const QString &name : used) {
            if (!known.contains(name))
                masters.append(QJsonObject{{QStringLiteral("name"), name}, {QStringLiteral("missing"), true}});
        }

        auto *group = new QWidget(w);
        auto *gv = new QVBoxLayout(group);
        gv->setContentsMargins(0, 0, 0, 0);
        gv->setSpacing(8);
        gv->addWidget(legend(mode == QLatin1String("direct") ? tr("Master termbases") : tr("Master termbases to use"), group));
        gv->addWidget(Web::text(tr("Masters you keep across projects, for a client, a domain or a set of languages."), "help", group));
        if (masters.isEmpty())
            gv->addWidget(Web::text(tr("There is no master termbase yet. The project gets a termbase of its own."), "muted", group));
        for (const QJsonValue &mv : std::as_const(masters)) {
            const QJsonObject m = mv.toObject();
            const QString name = m.value(kName).toString();
            const bool chosen = used.contains(name);
            auto *box = new QCheckBox(name, group);
            box->setChecked(chosen);
            QWidget *tag = nullptr;
            QList<QWidget *> below;
            if (m.value(QLatin1String("missing")).toBool()) {
                below << Web::text(tr("Not kept where this project is."), "muted", group);
            } else {
                QList<QWidget *> meta{Web::locationBadge(!host().isEmpty(), group, host()),
                                      Web::text(plural(m.value(QLatin1String("entries")).toInt(), tr("1 entry"), tr("%1 entries")), "secondary", group)};
                for (const QString &l : strings(m.value(QLatin1String("languages"))))
                    meta << Web::langChip(l, group);
                below << Web::row(meta, group);
            }
            QStringList missing;
            if (chosen && !m.value(QLatin1String("missing")).toBool()) {
                const QStringList has = strings(m.value(QLatin1String("languages")));
                for (const QString &l : langs) {
                    if (!l.isEmpty() && !has.contains(l))
                        missing << Languages::name(l);
                }
            }
            auto *frame = choiceWith(box, tag, QString(), below, group);
            gv->addWidget(frame);
            if (!missing.isEmpty()) {
                const bool adds = addLangs.contains(name);
                auto *note = new QWidget(group);
                auto *nv = new QVBoxLayout(note);
                nv->setContentsMargins(26, 0, 0, 4);
                nv->setSpacing(4);
                auto *addBox = new QCheckBox(missing.size() == 1 ? tr("Add it to %1").arg(name) : tr("Add them to %1").arg(name), note);
                addBox->setAccessibleName(tr("Add the missing languages to %1").arg(name));
                addBox->setChecked(adds);
                QString text = tr("%1 has no %2.").arg(name, QLocale().createSeparatedList(missing));
                if (!adds)
                    text += QLatin1Char(' ') + (mode == QLatin1String("direct") && writeTo == name
                                                    ? tr("New terms go into this master, so it needs the languages.")
                                                    : tr("Otherwise %1 is used for the languages it has, and terms in the others stay in the project termbase.").arg(name));
                nv->addWidget(Web::note(text, QStringLiteral("info"), note));
                nv->addWidget(addBox);
                gv->addWidget(note);
                connect(addBox, &QCheckBox::toggled, this, [this, name, setTerm](bool on) {
                    setTerm([name, on](QJsonObject &x) {
                        QStringList a = strings(x.value(QLatin1String("addLanguages")));
                        a.removeAll(name);
                        if (on)
                            a << name;
                        x.insert(QStringLiteral("addLanguages"), array(a));
                    });
                    fillTerminology();
                });
            }
            connect(box, &QCheckBox::toggled, this, [this, name, setTerm](bool on) {
                setTerm([name, on](QJsonObject &x) {
                    QStringList next = strings(x.value(QLatin1String("masters")));
                    next.removeAll(name);
                    if (on)
                        next << name;
                    // Choosing a master for a project with only its own
                    // termbase layers the project over it; taking the last
                    // one away leaves only its own.
                    const QString was = x.value(QLatin1String("mode")).toString(QStringLiteral("own"));
                    x.insert(QStringLiteral("mode"), next.isEmpty() ? QStringLiteral("own") : was == QLatin1String("own") ? QStringLiteral("layered") : was);
                    x.insert(QStringLiteral("masters"), array(next));
                    QStringList a;
                    for (const QString &l : strings(x.value(QLatin1String("addLanguages")))) {
                        if (next.contains(l))
                            a << l;
                    }
                    x.insert(QStringLiteral("addLanguages"), array(a));
                    if (!next.contains(x.value(QLatin1String("writeTo")).toString()))
                        x.insert(QStringLiteral("writeTo"), next.value(0));
                });
                fillTerminology();
            });
        }
        gv->addWidget(problemLabel(QStringLiteral("terminology.masters"), group));
        gv->addWidget(problemLabel(QStringLiteral("terminology.addLanguages"), group));
        v->addWidget(group);

        auto *where = new QWidget(w);
        auto *wv = new QVBoxLayout(where);
        wv->setContentsMargins(0, 0, 0, 0);
        wv->setSpacing(8);
        wv->addWidget(legend(tr("Where new terms go"), where));
        if (!m_new) {
            const QString said = mode == QLatin1String("direct") ? tr("Straight into a master.")
                : mode == QLatin1String("own")                   ? tr("Into the project's own termbase, without masters.")
                                                                 : tr("Into the %1, then merged into the masters.").arg(Words::get("projectTermbase").toLower());
            auto *line = new QLabel(QStringLiteral("%1 <span style=\"color:%2\">%3</span>")
                                        .arg(said.toHtmlEscaped(), Theme::color("text3").name(),
                                             tr("This stays as the project was set up, so the terms already collected stay where they are.").toHtmlEscaped()),
                                    where);
            line->setWordWrap(true);
            wv->addWidget(line);
        } else {
            struct Mode {
                const char *mode;
                QString title;
                QString desc;
                bool recommended;
            };
            const QList<Mode> modes{
                {"layered", tr("%1, then merge into the masters").arg(Words::get("projectTermbase")),
                 tr("New and changed terms stay in the project. When the project is done, you review them and merge into the masters. The masters stay unchanged until then."), true},
                {"direct", tr("Directly into a master"), tr("New terms are saved to the master right away. Simple when you work alone on this client."), false},
                {"own", tr("Own project termbase only"), tr("No master is used. You can turn it into a master or merge it into one later."), false},
            };
            auto *modeGroup = new QButtonGroup(where);
            for (const Mode &m : modes) {
                const QString key = QLatin1String(m.mode);
                auto *radio = new QRadioButton(m.title, where);
                modeGroup->addButton(radio);
                radio->setChecked(mode == key);
                const bool off = key != QLatin1String("own") && used.isEmpty();
                radio->setEnabled(!off);
                auto *frame = choiceWith(radio, m.recommended ? Web::chip(tr("Recommended"), QString(), where) : nullptr, m.desc, {}, where);
                frame->setEnabled(!off);
                wv->addWidget(frame);
                connect(radio, &QRadioButton::toggled, this, [this, key, setTerm](bool on) {
                    if (!on)
                        return;
                    setTerm([key](QJsonObject &x) {
                        x.insert(QStringLiteral("mode"), key);
                        const QStringList ms = strings(x.value(QLatin1String("masters")));
                        if (key == QLatin1String("own"))
                            x.insert(QStringLiteral("masters"), QJsonArray());
                        if (key == QLatin1String("direct")) {
                            if (x.value(QLatin1String("writeTo")).toString().isEmpty())
                                x.insert(QStringLiteral("writeTo"), ms.value(0));
                        } else {
                            x.remove(QStringLiteral("writeTo"));
                        }
                    });
                    fillTerminology();
                });
            }
            wv->addWidget(problemLabel(QStringLiteral("terminology.mode"), where));
        }
        v->addWidget(where);

        if (mode == QLatin1String("direct") && used.size() > 1) {
            auto *target = new QComboBox(w);
            for (const QString &name : used)
                target->addItem(name, name);
            target->setCurrentIndex(qMax(0, target->findData(writeTo)));
            v->addWidget(Web::field(tr("Master for new terms"), target, w));
            connect(target, &QComboBox::currentIndexChanged, this, [this, target, setTerm] {
                const QString name = target->currentData().toString();
                setTerm([name](QJsonObject &x) { x.insert(QStringLiteral("writeTo"), name); });
                fillTerminology();
            });
        }
        v->addWidget(problemLabel(QStringLiteral("terminology.writeTo"), w));

        if (mode == QLatin1String("layered")) {
            auto *f = new QWidget(w);
            auto *fv = new QVBoxLayout(f);
            fv->setContentsMargins(0, 0, 0, 0);
            fv->setSpacing(8);
            const bool on = t.value(QLatin1String("autoUpdate")).toBool();
            auto *box = new QCheckBox(tr("%1 each time the project opens").arg(Words::get("updateFromMaster")), f);
            box->setChecked(on);
            auto *help = Web::text(QString(), "help", f);
            const auto say = [help](bool yes) {
                help->setText(yes ? tr("Changes made to the masters since the project last opened are taken in when the editor opens.")
                                  : tr("Changes made to the masters are listed under %1 until you take them in yourself.").arg(Words::get("reviewMerge")));
            };
            say(on);
            fv->addWidget(box);
            fv->addWidget(help);
            v->addWidget(f);
            connect(box, &QCheckBox::toggled, this, [say, setTerm](bool yes) {
                say(yes);
                setTerm([yes](QJsonObject &x) { x.insert(QStringLiteral("autoUpdate"), yes); });
            });
        }
        w->setEnabled(!m_readOnly);
    });
}

// Checks: how context matches count, the quality checks the editor runs,
// the spelling dictionary of each target language, the project's own
// words and its rules.

int ProjectForm::checksOf() const
{
    return int(m_kinds.size()) + 1;
}

int ProjectForm::checksOn() const
{
    const QJsonObject qa = m_s.value(QLatin1String("qa")).toObject();
    const QStringList disabled = strings(qa.value(QLatin1String("disabled")));
    const QStringList enabled = strings(qa.value(QLatin1String("enabled")));
    int n = qa.value(QLatin1String("uneditedFuzzy")).toBool() ? 1 : 0;
    for (const QJsonValue &v : m_kinds) {
        const QJsonObject k = v.toObject();
        const QString kind = k.value(QLatin1String("kind")).toString();
        if (k.value(QLatin1String("default")).toBool() ? !disabled.contains(kind) : enabled.contains(kind))
            ++n;
    }
    return n;
}

QWidget *ProjectForm::checks(QWidget *parent)
{
    if (!m_checks) {
        m_checks = new QWidget(parent);
        fillChecks();
    }
    return m_checks;
}

void ProjectForm::fillChecks()
{
    if (!m_checks)
        return;
    QWidget *w = m_checks;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(16);
        const QJsonObject qa = m_s.value(QLatin1String("qa")).toObject();
        const auto setQa = [this](const std::function<void(QJsonObject &)> &change) {
            QJsonObject x = m_s.value(QLatin1String("qa")).toObject();
            change(x);
            m_s.insert(QStringLiteral("qa"), x);
            m_checksTouched = true;
            touch();
        };

        // Context matches.
        auto *ctx = new QWidget(w);
        auto *cv = new QVBoxLayout(ctx);
        cv->setContentsMargins(0, 0, 0, 0);
        cv->setSpacing(8);
        cv->addWidget(legend(tr("Context matches"), ctx));
        auto *ctxGroup = new QButtonGroup(ctx);
        const QString rule = m_s.value(QLatin1String("analysis")).toObject().value(QLatin1String("context")).toString(QStringLiteral("previous"));
        for (const auto &[key, title, desc] : {std::tuple{QStringLiteral("previous"), tr("The segment before matches"), tr("The usual rule for a context match.")},
                                               std::tuple{QStringLiteral("both"), tr("The segments before and after match"), tr("Stricter: both neighbours must match.")}}) {
            auto *radio = new QRadioButton(title, ctx);
            ctxGroup->addButton(radio);
            radio->setChecked(rule == key);
            cv->addWidget(Web::choice(radio, desc, ctx));
            const QString k = key;
            connect(radio, &QRadioButton::toggled, this, [this, k](bool on) {
                if (!on)
                    return;
                QJsonObject an = m_s.value(QLatin1String("analysis")).toObject();
                an.insert(QStringLiteral("context"), k);
                m_s.insert(QStringLiteral("analysis"), an);
                touch();
            });
        }
        v->addWidget(ctx);

        // The checks, in two columns.
        auto *list = new QWidget(w);
        auto *lv = new QVBoxLayout(list);
        lv->setContentsMargins(0, 0, 0, 0);
        lv->setSpacing(8);
        lv->addWidget(legend(tr("Quality checks while translating"), list));
        auto *grid = new QGridLayout;
        grid->setContentsMargins(0, 0, 0, 0);
        grid->setHorizontalSpacing(24);
        grid->setVerticalSpacing(10);
        grid->setColumnStretch(0, 1);
        grid->setColumnStretch(1, 1);
        const QStringList disabled = strings(qa.value(QLatin1String("disabled")));
        const QStringList enabled = strings(qa.value(QLatin1String("enabled")));
        bool spellingOn = false;
        int at = 0;
        const auto addCheck = [&](const QString &name, const QString &desc, bool on, const std::function<void(bool)> &toggle) {
            auto *cell = new QWidget(list);
            auto *cl = new QVBoxLayout(cell);
            cl->setContentsMargins(0, 0, 0, 0);
            cl->setSpacing(2);
            auto *box = new QCheckBox(name, cell);
            box->setChecked(on);
            box->setAccessibleDescription(desc);
            cl->addWidget(box);
            cl->addWidget(indent(Web::text(desc, "help", cell), 24));
            grid->addWidget(cell, at / 2, at % 2, Qt::AlignTop);
            ++at;
            connect(box, &QCheckBox::toggled, this, toggle);
        };
        for (const QJsonValue &kv : std::as_const(m_kinds)) {
            const QJsonObject k = kv.toObject();
            const QString kind = k.value(QLatin1String("kind")).toString();
            const bool byDefault = k.value(QLatin1String("default")).toBool();
            const bool on = byDefault ? !disabled.contains(kind) : enabled.contains(kind);
            if (kind == QLatin1String("spelling"))
                spellingOn = on;
            QString desc = k.value(QLatin1String("description")).toString();
            if (kind == QLatin1String("length"))
                desc += QLatin1Char(' ') + tr("Limits come from the file (XLIFF maxwidth); documents have none.");
            else if (kind == QLatin1String("spelling"))
                desc += QLatin1Char(' ') + tr("With the dictionaries below.");
            else if (kind == QLatin1String("rule"))
                desc += QLatin1Char(' ') + tr("The rules below.");
            addCheck(k.value(kName).toString(), desc, on, [this, kind, byDefault, setQa](bool yes) {
                setQa([kind, byDefault, yes](QJsonObject &x) {
                    const QString key = byDefault ? QStringLiteral("disabled") : QStringLiteral("enabled");
                    QStringList l = strings(x.value(key));
                    l.removeAll(kind);
                    if (byDefault != yes)
                        l << kind;
                    x.insert(key, array(l));
                });
                // The dictionaries are chosen only while spelling is checked.
                if (kind == QLatin1String("spelling"))
                    fillChecks();
            });
        }
        addCheck(tr("Unedited fuzzy matches"), tr("A fuzzy match is confirmed without a change."), qa.value(QLatin1String("uneditedFuzzy")).toBool(), [setQa](bool yes) {
            setQa([yes](QJsonObject &x) { x.insert(QStringLiteral("uneditedFuzzy"), yes); });
        });
        if (m_kinds.isEmpty())
            lv->addWidget(Web::text(tr("Loading the checks…"), "muted", list));
        lv->addLayout(grid);
        v->addWidget(list);

        // Spelling.
        auto *spell = new QWidget(w);
        auto *sv = new QVBoxLayout(spell);
        sv->setContentsMargins(0, 0, 0, 0);
        sv->setSpacing(8);
        sv->addWidget(legend(tr("Spelling"), spell));
        if (!spellingOn)
            sv->addWidget(Web::text(tr("The spelling check is off."), "muted", spell));
        const QJsonObject chosen = qa.value(QLatin1String("dictionaries")).toObject();
        for (const QString &t : targetList()) {
            auto *combo = new QComboBox(spell);
            combo->setAccessibleName(tr("Spelling dictionary for %1").arg(Languages::name(t)));
            combo->setEnabled(spellingOn);
            const QList<QJsonObject> fit = dictionariesFor(t);
            combo->addItem(fit.isEmpty() ? tr("No dictionary for this language") : tr("No spelling check"), QString());
            for (const QJsonObject &d : std::as_const(fit))
                combo->addItem(QStringLiteral("%1 (%2)").arg(d.value(kName).toString(), Languages::name(d.value(QLatin1String("lang")).toString())), d.value(kName).toString());
            combo->setCurrentIndex(qMax(0, combo->findData(chosen.value(t).toString())));
            auto *label = new QLabel(Languages::name(t), spell);
            label->setBuddy(combo);
            sv->addWidget(Web::row({label, combo}, spell));
            sv->addWidget(problemLabel(QStringLiteral("qa.dictionaries.%1").arg(t), spell));
            connect(combo, &QComboBox::currentIndexChanged, this, [combo, t, setQa] {
                const QString name = combo->currentData().toString();
                setQa([t, name](QJsonObject &x) {
                    QJsonObject d = x.value(QLatin1String("dictionaries")).toObject();
                    if (name.isEmpty())
                        d.remove(t);
                    else
                        d.insert(t, name);
                    x.insert(QStringLiteral("dictionaries"), d);
                });
            });
        }
        QSet<QString> includedLangs;
        for (const QJsonValue &dv : std::as_const(m_dicts)) {
            if (dv.toObject().value(QLatin1String("included")).toBool())
                includedLangs.insert(dv.toObject().value(QLatin1String("lang")).toString().section(QLatin1Char('-'), 0, 0));
        }
        QString note = tr("Dictionaries are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files.");
        if (includedLangs.size() == 1)
            note = tr("A dictionary for one language comes with Interlinea. Others are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files.");
        else if (includedLangs.size() > 1)
            note = tr("Dictionaries for %1 languages come with Interlinea. Others are added once under Settings, from a LibreOffice extension (.oxt) or .aff and .dic files.").arg(includedLangs.size());
        sv->addWidget(Web::text(note, "help", spell));
        v->addWidget(spell);

        // Words of the project's own.
        auto *words = new QWidget(w);
        auto *wv = new QVBoxLayout(words);
        wv->setContentsMargins(0, 0, 0, 0);
        wv->setSpacing(8);
        wv->addWidget(Web::text(tr("Words always spelled right in this project"), "fieldLabel", words));
        const QStringList have = strings(qa.value(QLatin1String("words")));
        if (!have.isEmpty()) {
            QList<QWidget *> chips;
            for (const QString &word : have) {
                auto *chip = new QFrame(words);
                chip->setObjectName(QStringLiteral("wordChip"));
                chip->setStyleSheet(QStringLiteral("QFrame#wordChip { background: %1; border-radius: 4px; } QLabel { background: transparent; color: %2; font-size: 12px; font-weight: 600; }")
                                        .arg((Theme::dark() ? QColor(0x2a, 0x30, 0x38) : QColor(0xec, 0xef, 0xf3)).name(), Theme::color("text2").name()));
                auto *ch = new QHBoxLayout(chip);
                ch->setContentsMargins(6, 1, 2, 1);
                ch->setSpacing(2);
                ch->addWidget(new QLabel(word, chip));
                auto *x = iconButton(QStringLiteral("x"), tr("Remove %1").arg(word), chip);
                ch->addWidget(x);
                chips << chip;
                connect(x, &QPushButton::clicked, this, [this, word, setQa] {
                    setQa([word](QJsonObject &q) {
                        QStringList l = strings(q.value(QLatin1String("words")));
                        l.removeAll(word);
                        q.insert(QStringLiteral("words"), array(l));
                    });
                    fillChecks();
                });
            }
            auto *listRow = Web::row(chips, words, 6);
            listRow->setAccessibleName(tr("Words always spelled right in this project"));
            wv->addWidget(listRow);
        }
        auto *word = new QLineEdit(words);
        word->setAccessibleName(tr("Add a word"));
        word->setPlaceholderText(tr("A name or term"));
        word->setMinimumWidth(280);
        auto *addWord = Web::button(tr("Add"), Web::Small, words, QStringLiteral("plus"));
        addWord->setAccessibleName(tr("Add the word"));
        addWord->setEnabled(false);
        wv->addWidget(Web::row({word, addWord}, words));
        wv->addWidget(problemLabel(QStringLiteral("qa.words"), words));
        connect(word, &QLineEdit::textChanged, addWord, [addWord](const QString &t) { addWord->setEnabled(!t.trimmed().isEmpty()); });
        const auto addIt = [this, word, setQa] {
            const QString x = word->text().trimmed();
            if (x.isEmpty() || strings(m_s.value(QLatin1String("qa")).toObject().value(QLatin1String("words"))).contains(x))
                return;
            setQa([x](QJsonObject &q) {
                QStringList l = strings(q.value(QLatin1String("words")));
                l << x;
                q.insert(QStringLiteral("words"), array(l));
            });
            fillChecks();
        };
        connect(addWord, &QPushButton::clicked, this, addIt);
        connect(word, &QLineEdit::returnPressed, this, addIt);
        v->addWidget(words);

        // The grammar service that reads the translations, if any.
        {
            auto *grammar = new QWidget(w);
            auto *gv = new QVBoxLayout(grammar);
            gv->setContentsMargins(0, 0, 0, 0);
            gv->setSpacing(8);
            gv->addWidget(legend(tr("Grammar"), grammar));
            auto *combo = new QComboBox(grammar);
            combo->setObjectName(QStringLiteral("grammarService"));
            combo->setAccessibleName(tr("Grammar service"));
            const QString set = qa.value(QLatin1String("grammar")).toString();
            const bool gone = !set.isEmpty() && !m_grammar.contains(set, Qt::CaseInsensitive);
            combo->addItem(m_grammar.isEmpty() && !gone ? tr("No grammar service yet") : tr("No grammar check"), QString());
            if (gone)
                combo->addItem(tr("%1 (no longer there)").arg(set), set);
            for (const QString &n : std::as_const(m_grammar))
                combo->addItem(n, n);
            int at = 0;
            for (int i = 1; i < combo->count(); ++i) {
                if (combo->itemData(i).toString().compare(set, Qt::CaseInsensitive) == 0)
                    at = i;
            }
            combo->setCurrentIndex(at);
            auto *label = new QLabel(tr("Grammar service"), grammar);
            label->setBuddy(combo);
            gv->addWidget(Web::row({label, combo}, grammar));
            gv->addWidget(problemLabel(QStringLiteral("qa.grammar"), grammar));
            auto *help = Web::text(host().isEmpty()
                                       ? tr("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under Settings.")
                                       : tr("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under Settings, by an administrator of the server."),
                                   "help", grammar);
            help->setWordWrap(true);
            gv->addWidget(help);
            connect(combo, &QComboBox::currentIndexChanged, this, [combo, setQa] {
                const QString name = combo->currentData().toString();
                setQa([name](QJsonObject &x) {
                    if (name.isEmpty())
                        x.remove(QStringLiteral("grammar"));
                    else
                        x.insert(QStringLiteral("grammar"), name);
                });
            });
            v->addWidget(grammar);
        }

        // Rules of the project's own.
        auto *rules = new QWidget(w);
        auto *rv = new QVBoxLayout(rules);
        rv->setContentsMargins(0, 0, 0, 0);
        rv->setSpacing(8);
        rv->addWidget(legend(tr("Rules of your own"), rules));
        rv->addWidget(Web::text(tr("Regular expressions the translation must or must not match, only where the source matches one if you give it, or must match as often as the source matches."),
                                "help", rules));
        const QJsonArray have2 = qa.value(QLatin1String("rules")).toArray();
        for (int i = 0; i < have2.size(); ++i) {
            const QJsonObject r = have2.at(i).toObject();
            auto *box = new QFrame(rules);
            box->setObjectName(QStringLiteral("choice"));
            box->setAccessibleName(r.value(kName).toString().isEmpty() ? tr("Rule %1").arg(i + 1) : r.value(kName).toString());
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(12, 12, 12, 12);
            bv->setSpacing(12);
            auto *g = new QGridLayout;
            g->setHorizontalSpacing(16);
            g->setVerticalSpacing(12);
            g->setColumnStretch(0, 1);
            g->setColumnStretch(1, 1);
            const auto setRule = [this, i, setQa](const QString &key, const QString &value, bool optional) {
                setQa([i, key, value, optional](QJsonObject &q) {
                    QJsonArray a = q.value(QLatin1String("rules")).toArray();
                    QJsonObject o = a.at(i).toObject();
                    if (optional && value.isEmpty())
                        o.remove(key);
                    else
                        o.insert(key, value);
                    a.replace(i, o);
                    q.insert(QStringLiteral("rules"), a);
                });
            };
            const auto line = [&](const QString &label, const QString &a11y, const QString &key, bool optional, bool mono, int row, int col, const QString &optionalWord = QString()) {
                auto *e = new QLineEdit(r.value(key).toString(), box);
                e->setAccessibleName(a11y);
                if (mono) {
                    QFont f(QStringLiteral("monospace"));
                    f.setStyleHint(QFont::Monospace);
                    f.setPointSizeF(e->font().pointSizeF());
                    e->setFont(f);
                }
                QWidget *field = Web::field(label, e, box, optional && optionalWord.isEmpty());
                if (!optionalWord.isEmpty()) {
                    if (auto *l = field->findChild<QLabel *>())
                        l->setText(QStringLiteral("<b>%1</b> <span style=\"color:%2\">%3</span>").arg(label.toHtmlEscaped(), Theme::color("text3").name(), optionalWord.toHtmlEscaped()));
                }
                g->addWidget(field, row, col, Qt::AlignTop);
                connect(e, &QLineEdit::textEdited, this, [setRule, key, optional](const QString &t) { setRule(key, t, optional); });
                return e;
            };
            const QString n = tr("rule %1").arg(i + 1);
            line(tr("Name"), tr("Name of %1").arg(n), QStringLiteral("name"), false, false, 0, 0);
            auto *modeBox = new QComboBox(box);
            modeBox->setAccessibleName(tr("What the translation must do, %1").arg(n));
            modeBox->addItem(tr("must not match"), QStringLiteral("must-not"));
            modeBox->addItem(tr("must match"), QStringLiteral("must"));
            modeBox->addItem(tr("must match as often as the source"), QStringLiteral("count"));
            modeBox->setCurrentIndex(qMax(0, modeBox->findData(r.value(QLatin1String("mode")).toString())));
            g->addWidget(Web::field(tr("The translation"), modeBox, box), 0, 1, Qt::AlignTop);
            connect(modeBox, &QComboBox::currentIndexChanged, this, [this, modeBox, setRule] {
                setRule(QStringLiteral("mode"), modeBox->currentData().toString(), false);
                // The source pattern is needed for counting.
                fillChecks();
            });
            line(tr("Translation pattern"), tr("Translation pattern of %1").arg(n), QStringLiteral("target"), false, true, 1, 0);
            line(tr("Source pattern"), tr("Source pattern of %1").arg(n), QStringLiteral("source"), true, true, 1, 1,
                 r.value(QLatin1String("mode")).toString() == QLatin1String("count") ? tr("needed") : tr("optional"));
            auto *sev = new QComboBox(box);
            sev->setAccessibleName(tr("How %1 is shown").arg(n));
            sev->addItem(tr("Warning"), QStringLiteral("warning"));
            sev->addItem(tr("Error"), QStringLiteral("error"));
            sev->setCurrentIndex(qMax(0, sev->findData(r.value(QLatin1String("severity")).toString())));
            g->addWidget(Web::field(tr("Shown as"), sev, box), 2, 0, Qt::AlignTop);
            connect(sev, &QComboBox::currentIndexChanged, this, [sev, setRule] { setRule(QStringLiteral("severity"), sev->currentData().toString(), false); });
            line(tr("Message"), tr("Message of %1").arg(n), QStringLiteral("message"), true, false, 2, 1);
            bv->addLayout(g);
            bv->addWidget(problemLabel(QStringLiteral("qa.rules[%1]").arg(i), box));
            auto *remove = Web::button(tr("Remove rule"), Web::Quiet | Web::Small, box, QStringLiteral("x"));
            remove->setAccessibleName(tr("Remove %1").arg(n));
            bv->addWidget(remove, 0, Qt::AlignLeft);
            connect(remove, &QPushButton::clicked, this, [this, i, setQa] {
                setQa([i](QJsonObject &q) {
                    QJsonArray a = q.value(QLatin1String("rules")).toArray();
                    a.removeAt(i);
                    q.insert(QStringLiteral("rules"), a);
                });
                fillChecks();
            });
            rv->addWidget(box);
        }
        auto *addRule = Web::button(tr("Add a rule"), Web::Small, rules, QStringLiteral("plus"));
        rv->addWidget(addRule, 0, Qt::AlignLeft);
        connect(addRule, &QPushButton::clicked, this, [this, setQa] {
            setQa([](QJsonObject &q) {
                QJsonArray a = q.value(QLatin1String("rules")).toArray();
                a.append(QJsonObject{{QStringLiteral("name"), QString()}, {QStringLiteral("target"), QString()}, {QStringLiteral("mode"), QStringLiteral("must-not")},
                                     {QStringLiteral("severity"), QStringLiteral("warning")}});
                q.insert(QStringLiteral("rules"), a);
            });
            fillChecks();
        });
        v->addWidget(rules);
        w->setEnabled(!m_readOnly);
    });
}

// Pre-translation: which matches go into empty segments, and which are
// confirmed.

QWidget *ProjectForm::prepare(QWidget *parent)
{
    if (!m_prepare) {
        m_prepare = new QWidget(parent);
        fillPrepare();
    }
    return m_prepare;
}

void ProjectForm::setPrepareSummary(const QString &summary)
{
    if (m_summary == summary)
        return;
    m_summary = summary;
    fillPrepare();
}

void ProjectForm::fillPrepare()
{
    if (!m_prepare)
        return;
    QWidget *w = m_prepare;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(8);
        const QJsonObject p = m_s.value(QLatin1String("prepare")).toObject();
        const auto setPrep = [this](const QString &key, const QJsonValue &value) {
            QJsonObject x = m_s.value(QLatin1String("prepare")).toObject();
            x.insert(key, value);
            m_s.insert(QStringLiteral("prepare"), x);
            touch();
        };
        v->addWidget(legend(m_new ? tr("Pre-translation") : tr("Matches to insert"), w));
        const bool on = p.value(QLatin1String("pretranslate")).toBool();
        if (m_new) {
            auto *box = new QCheckBox(tr("Fill empty segments with memory matches"), w);
            box->setChecked(on);
            v->addWidget(box);
            connect(box, &QCheckBox::toggled, this, [this, setPrep](bool yes) {
                setPrep(QStringLiteral("pretranslate"), yes);
                fillPrepare();
            });
        }
        if (on || !m_new) {
            auto *nested = new QWidget(w);
            auto *nv = new QVBoxLayout(nested);
            nv->setContentsMargins(m_new ? 26 : 0, 0, 0, 0);
            nv->setSpacing(8);
            auto *min = new QSpinBox(nested);
            min->setRange(50, 100);
            min->setValue(p.value(QLatin1String("minScore")).toInt(100));
            min->setAccessibleName(tr("Lowest match to insert"));
            min->setFixedWidth(84);
            auto *label = new QLabel(tr("Lowest match to insert"), nested);
            label->setBuddy(min);
            nv->addWidget(Web::row({label, min, new QLabel(QStringLiteral("%"), nested)}, nested));
            nv->addWidget(problemLabel(QStringLiteral("prepare.minScore"), nested));
            auto *context = new QCheckBox(tr("Confirm segments filled with a context match"), nested);
            context->setChecked(p.value(QLatin1String("confirmContext")).toBool());
            auto *exact = new QCheckBox(tr("Confirm segments filled with a 100% match"), nested);
            exact->setChecked(p.value(QLatin1String("confirmExact")).toBool());
            nv->addWidget(context);
            nv->addWidget(exact);
            nv->addWidget(Web::text(tr("Other matches are inserted as drafts for you to check. Locked and translated segments are left alone."), "help", nested));
            if (!m_summary.isEmpty())
                nv->addWidget(Web::note(m_summary, QStringLiteral("info"), nested));
            v->addWidget(nested);
            connect(min, &QSpinBox::valueChanged, this, [setPrep](int value) { setPrep(QStringLiteral("minScore"), value); });
            connect(context, &QCheckBox::toggled, this, [setPrep](bool yes) { setPrep(QStringLiteral("confirmContext"), yes); });
            connect(exact, &QCheckBox::toggled, this, [setPrep](bool yes) { setPrep(QStringLiteral("confirmExact"), yes); });
        }
        w->setEnabled(!m_readOnly);
    });
}

// Segmentation: the project's own rules for where sentences end.

QWidget *ProjectForm::segmentation(QWidget *parent)
{
    if (!m_segmentation) {
        m_segmentation = new QWidget(parent);
        fillSegmentation();
    }
    return m_segmentation;
}

void ProjectForm::fillSegmentation()
{
    if (!m_segmentation)
        return;
    QWidget *w = m_segmentation;
    refill(w, [this, w] {
        QVBoxLayout *v = reset(w);
        v->setSpacing(8);
        const QString key = QStringLiteral("segmentation");
        const auto setRules = [this, key](const std::function<void(QJsonArray &)> &change) {
            QJsonArray a = m_s.value(key).toArray();
            change(a);
            if (a.isEmpty())
                m_s.remove(key);
            else
                m_s.insert(key, a);
            touch();
        };
        v->addWidget(legend(tr("Segmentation rules of your own"), w));
        auto *help = Web::text(tr("Sentences end as is usual for the source language. A rule changes that at every place where the text before it ends with the first pattern and the "
                                  "text after it begins with the second (regular expressions; one may stay empty). Your rules come first. %1 Files that bring their own segments keep them.")
                                   .arg(m_new ? tr("They cut the documents of the project.")
                                              : tr("They cut the documents added from now on; the files in the project stay as they are.")),
                               "help", w);
        help->setWordWrap(true);
        v->addWidget(help);
        const QJsonArray have = m_s.value(key).toArray();
        for (int i = 0; i < have.size(); ++i) {
            const QJsonObject r = have.at(i).toObject();
            auto *box = new QFrame(w);
            box->setObjectName(QStringLiteral("choice"));
            box->setAccessibleName(tr("Segmentation rule %1").arg(i + 1));
            auto *bv = new QVBoxLayout(box);
            bv->setContentsMargins(12, 12, 12, 12);
            bv->setSpacing(12);
            auto *g = new QGridLayout;
            g->setHorizontalSpacing(16);
            g->setVerticalSpacing(12);
            for (int c = 0; c < 3; ++c)
                g->setColumnStretch(c, 1);
            const QString n = tr("segmentation rule %1").arg(i + 1);
            auto *what = new QComboBox(box);
            what->setObjectName(QStringLiteral("segBreak%1").arg(i));
            what->setAccessibleName(tr("What happens at the place, %1").arg(n));
            what->addItem(tr("a sentence does not end"), false);
            what->addItem(tr("a sentence ends"), true);
            what->setCurrentIndex(r.value(QLatin1String("break")).toBool() ? 1 : 0);
            g->addWidget(Web::field(tr("At this place"), what, box), 0, 0, Qt::AlignTop);
            connect(what, &QComboBox::currentIndexChanged, this, [what, setRules, i] {
                setRules([&](QJsonArray &a) {
                    QJsonObject o = a.at(i).toObject();
                    o.insert(QStringLiteral("break"), what->currentData().toBool());
                    a.replace(i, o);
                });
            });
            const auto line = [&](const QString &label, const QString &a11y, const QString &field, int col) {
                auto *e = new QLineEdit(r.value(field).toString(), box);
                e->setObjectName(QStringLiteral("seg%1%2").arg(field).arg(i));
                e->setAccessibleName(a11y);
                QFont f(QStringLiteral("monospace"));
                f.setStyleHint(QFont::Monospace);
                f.setPointSizeF(e->font().pointSizeF());
                e->setFont(f);
                g->addWidget(Web::field(label, e, box), 0, col, Qt::AlignTop);
                connect(e, &QLineEdit::textEdited, this, [setRules, field, i](const QString &t) {
                    setRules([&](QJsonArray &a) {
                        QJsonObject o = a.at(i).toObject();
                        if (t.isEmpty())
                            o.remove(field);
                        else
                            o.insert(field, t);
                        a.replace(i, o);
                    });
                });
            };
            line(tr("Text before the place"), tr("Text before the place, %1").arg(n), QStringLiteral("before"), 1);
            line(tr("Text after the place"), tr("Text after the place, %1").arg(n), QStringLiteral("after"), 2);
            bv->addLayout(g);
            bv->addWidget(problemLabel(QStringLiteral("segmentation[%1]").arg(i), box));
            auto *remove = Web::button(tr("Remove rule"), Web::Quiet | Web::Small, box, QStringLiteral("x"));
            remove->setAccessibleName(tr("Remove %1").arg(n));
            bv->addWidget(remove, 0, Qt::AlignLeft);
            connect(remove, &QPushButton::clicked, this, [this, i, setRules] {
                setRules([i](QJsonArray &a) { a.removeAt(i); });
                fillSegmentation();
            });
            v->addWidget(box);
        }
        auto *add = Web::button(tr("Add a segmentation rule"), Web::Small, w, QStringLiteral("plus"));
        add->setObjectName(QStringLiteral("addSegmentationRule"));
        v->addWidget(add, 0, Qt::AlignLeft);
        connect(add, &QPushButton::clicked, this, [this, setRules] {
            setRules([](QJsonArray &a) { a.append(QJsonObject{{QStringLiteral("break"), false}}); });
            fillSegmentation();
        });
        auto *example = Web::text(tr("An abbreviation that should not end a sentence: “does not end”, before \\bca\\., after \\s. A new segment after every semicolon: “ends”, before ;, after \\s."),
                                  "help", w);
        example->setWordWrap(true);
        v->addWidget(example);
        w->setEnabled(!m_readOnly);
    });
}

QJsonObject ProjectForm::templateOf(const QString &name, const QJsonObject &s)
{
    QJsonObject t{{QStringLiteral("name"), name}};
    for (const char *key : {"client", "sourceLang", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare", "segmentation"})
        t.insert(QLatin1String(key), s.value(QLatin1String(key)));
    // The project termbase belongs to the project; a new project makes
    // its own.
    QJsonObject term = t.value(QLatin1String("terminology")).toObject();
    term.remove(QStringLiteral("termbase"));
    t.insert(QStringLiteral("terminology"), term);
    // So do the project's own memories.
    QJsonObject mems = t.value(QLatin1String("memories")).toObject();
    const QString pname = s.value(kName).toString();
    for (const QString &lang : mems.keys()) {
        QJsonArray kept;
        for (const QJsonValue &m : mems.value(lang).toArray()) {
            if (m.toObject().value(QLatin1String("memory")).toString() != QStringLiteral("%1 (%2)").arg(pname, lang))
                kept.append(m);
        }
        mems.insert(lang, kept);
    }
    t.insert(QStringLiteral("memories"), mems);
    return t;
}
