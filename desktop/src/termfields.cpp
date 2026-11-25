#include "termfields.h"

#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "theme.h"

#include <QAbstractButton>
#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QEvent>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QJsonArray>
#include <QLabel>
#include <QLineEdit>
#include <QMouseEvent>
#include <QPushButton>
#include <QRadioButton>
#include <QScrollArea>
#include <QSet>
#include <QTimer>
#include <QVBoxLayout>

namespace TermFields {

namespace {

constexpr int S1 = 4, S2 = 8, S3 = 12;

QLabel *bold(const QString &text, QWidget *parent)
{
    return new QLabel(QStringLiteral("<b>%1</b>").arg(text.toHtmlEscaped()), parent);
}

// Clicks a button when its text is clicked, as a <label> does.
class ClickText : public QObject {
public:
    ClickText(QAbstractButton *button, QObject *parent)
        : QObject(parent)
        , m_button(button)
    {
    }

protected:
    bool eventFilter(QObject *o, QEvent *e) override
    {
        if (e->type() == QEvent::MouseButtonRelease && static_cast<QMouseEvent *>(e)->button() == Qt::LeftButton && m_button && m_button->isEnabled()) {
            m_button->click();
            m_button->setFocus(Qt::MouseFocusReason);
            return true;
        }
        return QObject::eventFilter(o, e);
    }

private:
    QAbstractButton *m_button;
};

Draft text(const QString &name, const char *level)
{
    return Draft{name, QString::fromLatin1(level), QStringLiteral("text"), {}};
}

Draft list(const QString &name, const char *level, const QStringList &values)
{
    return Draft{name, QString::fromLatin1(level), QStringLiteral("picklist"), values};
}

QStringList readValues(const QString &typed)
{
    QStringList out;
    for (const QString &line : typed.split(QLatin1Char('\n'))) {
        const QString v = line.trimmed();
        if (!v.isEmpty() && !out.contains(v))
            out << v;
    }
    return out;
}

} // namespace

QList<Kind> kinds()
{
    return {
        {QStringLiteral("text"), QObject::tr("Text")},
        {QStringLiteral("picklist"), QObject::tr("A list of values")},
        {QStringLiteral("number"), QObject::tr("Number")},
        {QStringLiteral("date"), QObject::tr("Date")},
        {QStringLiteral("boolean"), QObject::tr("Yes or no")},
    };
}

QString kindName(const QString &id)
{
    for (const Kind &k : kinds()) {
        if (k.id == id)
            return k.name;
    }
    return kinds().first().name;
}

QList<Level> levels()
{
    return {
        {QStringLiteral("entry"), QObject::tr("Entry"), QObject::tr("the whole entry, the same in every language")},
        {QStringLiteral("language"), QObject::tr("Language"), QObject::tr("each language, such as a definition per language")},
        {QStringLiteral("term"), QObject::tr("Term"), QObject::tr("each term, such as grammar or usage")},
    };
}

QString levelName(const QString &id)
{
    for (const Level &l : levels()) {
        if (l.id == id)
            return l.name;
    }
    return levels().first().name;
}

QJsonObject toJson(const Draft &f)
{
    QJsonObject o{{QLatin1String("name"), f.name.trimmed()}, {QLatin1String("level"), f.level}, {QLatin1String("kind"), f.kind}};
    if (f.kind == QLatin1String("picklist"))
        o.insert(QLatin1String("values"), QJsonArray::fromStringList(f.values));
    return o;
}

Draft fromJson(const QJsonObject &f)
{
    Draft d;
    d.name = f.value(QLatin1String("name")).toString();
    const QString level = f.value(QLatin1String("level")).toString();
    d.level = level == QLatin1String("language") || level == QLatin1String("term") ? level : QStringLiteral("entry");
    const QString kind = f.value(QLatin1String("kind")).toString();
    d.kind = QStringLiteral("text");
    for (const Kind &k : kinds()) {
        if (k.id == kind)
            d.kind = kind;
    }
    for (const QJsonValue &v : f.value(QLatin1String("values")).toArray())
        d.values << v.toString();
    return d;
}

QString describe(const Draft &f)
{
    QString s = QStringLiteral("%1 (%2, %3").arg(f.name.trimmed(), levelName(f.level).toLower(), kindName(f.kind).toLower());
    if (f.kind == QLatin1String("picklist"))
        s += QStringLiteral(": ") + f.values.join(QStringLiteral(", "));
    return s + QLatin1Char(')');
}

QString problem(const Draft &f, const QStringList &others)
{
    const QString name = f.name.trimmed();
    if (name.isEmpty())
        return QObject::tr("Give the field a name.");
    for (const QString &o : others) {
        if (o.trimmed().compare(name, Qt::CaseInsensitive) == 0)
            return QObject::tr("Another field has this name.");
    }
    if (f.kind == QLatin1String("picklist") && f.values.isEmpty())
        return QObject::tr("Give the list its values, one a line.");
    return QString();
}

QList<Structure> structures()
{
    return {
        {QStringLiteral("terms"), QObject::tr("Terms only"), QObject::tr("A glossary: the terms of each language, each with its status. Fields can be added later."), {}},
        {QStringLiteral("notes"),
         QObject::tr("Terms with definitions and notes"),
         QObject::tr("A definition and its source for each language, and a note on the entry."),
         {text(QObject::tr("Definition"), "language"), text(QObject::tr("Source"), "language"), text(QObject::tr("Note"), "entry")}},
        {QStringLiteral("terminology"),
         QObject::tr("Terminology with grammar"),
         QObject::tr("Subject field, notes and the date it was last checked; definitions per language; part of speech, gender, number, context and usage per term."),
         {text(QObject::tr("Subject field"), "entry"), text(QObject::tr("Note"), "entry"), Draft{QObject::tr("Last checked"), QStringLiteral("entry"), QStringLiteral("date"), {}},
          text(QObject::tr("Definition"), "language"), text(QObject::tr("Source of the definition"), "language"),
          list(QObject::tr("Part of speech"), "term",
               {QObject::tr("noun"), QObject::tr("verb"), QObject::tr("adjective"), QObject::tr("adverb"), QObject::tr("proper noun"), QObject::tr("other")}),
          list(QObject::tr("Gender"), "term", {QObject::tr("masculine"), QObject::tr("feminine"), QObject::tr("neuter"), QObject::tr("common")}),
          list(QObject::tr("Number"), "term", {QObject::tr("singular"), QObject::tr("plural")}), text(QObject::tr("Context"), "term"), text(QObject::tr("Usage note"), "term")}},
    };
}

QComboBox *kindCombo(const QString &kind, QWidget *parent)
{
    auto *c = new QComboBox(parent);
    for (const Kind &k : kinds())
        c->addItem(k.name, k.id);
    c->setCurrentIndex(qMax(0, c->findData(kind)));
    return c;
}

QWidget *checkText(QAbstractButton *button, const QString &text, QWidget *parent, bool strong)
{
    auto *w = new QWidget(parent);
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(S2);
    button->setParent(w);
    button->setText(QString());
    button->setAccessibleName(text);
    h->addWidget(button, 0, Qt::AlignTop);
    QString html = text.toHtmlEscaped();
    if (strong) {
        const int stop = html.indexOf(QLatin1Char('.'));
        if (stop > 0)
            html = QStringLiteral("<b>%1</b>%2").arg(html.left(stop + 1), html.mid(stop + 1));
    }
    auto *label = new QLabel(html, w);
    label->setWordWrap(true);
    label->setTextFormat(Qt::RichText);
    label->setCursor(Qt::PointingHandCursor);
    label->installEventFilter(new ClickText(button, label));
    h->addWidget(label, 1);
    return w;
}

// ValuesEdit

ValuesEdit::ValuesEdit(const QStringList &initial, QWidget *parent)
    : QPlainTextEdit(initial.join(QLatin1Char('\n')), parent)
{
    setTabChangesFocus(true);
    setPlaceholderText(tr("One value a line\nsuch as\nnoun\nverb"));
    setMinimumWidth(160);
    setLineWrapMode(QPlainTextEdit::NoWrap);
    connect(this, &QPlainTextEdit::textChanged, this, [this] {
        fit();
        emit changed(values());
    });
    fit();
}

QStringList ValuesEdit::values() const
{
    return readValues(toPlainText());
}

void ValuesEdit::setValues(const QStringList &values)
{
    if (values == this->values())
        return;
    const QSignalBlocker block(this);
    setPlainText(values.join(QLatin1Char('\n')));
    fit();
}

void ValuesEdit::focusOutEvent(QFocusEvent *e)
{
    QPlainTextEdit::focusOutEvent(e);
    const QStringList now = values();
    if (toPlainText() != now.join(QLatin1Char('\n'))) {
        const QSignalBlocker block(this);
        setPlainText(now.join(QLatin1Char('\n')));
        fit();
    }
    emit left(now);
}

void ValuesEdit::fit()
{
    // As rows="n" on the web: three to six lines, one more than there are.
    const int lines = qBound(3, int(document()->blockCount()) + 1, 6);
    const QMargins m = contentsMargins();
    setFixedHeight(lines * fontMetrics().lineSpacing() + int(2 * document()->documentMargin()) + m.top() + m.bottom() + 12);
}

// FieldTable

FieldTable::FieldTable(QWidget *parent)
    : QWidget(parent)
{
    m_layout = new QVBoxLayout(this);
    m_layout->setContentsMargins(0, 0, 0, 0);
    m_layout->setSpacing(S2);
    render();
}

void FieldTable::setFields(const QList<Draft> &fields)
{
    m_fields = fields;
    render();
    emit changed();
}

void FieldTable::setTaken(const QStringList &names)
{
    m_taken = names;
    render();
}

QString FieldTable::problemOf(int i) const
{
    QStringList others = m_taken;
    for (int j = 0; j < m_fields.size(); j++) {
        if (j != i)
            others << m_fields[j].name;
    }
    return problem(m_fields[i], others);
}

QString FieldTable::firstProblem() const
{
    for (int i = 0; i < m_fields.size(); i++) {
        const QString p = problemOf(i);
        if (!p.isEmpty())
            return p;
    }
    return QString();
}

void FieldTable::render()
{
    while (QLayoutItem *item = m_layout->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        }
        delete item;
    }
    m_problems.clear();
    if (m_fields.isEmpty()) {
        auto *help = Web::text(tr("No fields: entries hold their terms and each term's status."), "help", this);
        help->setWordWrap(true);
        m_layout->addWidget(help);
    } else {
        auto *table = new Web::Table({tr("Name"), tr("Belongs to"), tr("Values"), QString()}, this);
        table->setObjectName(QStringLiteral("draftFields"));
        table->setProperty("inDialog", true);
        table->setStretch(2);
        // The name gets room as the values do (32% on the web).
        table->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
        table->horizontalHeaderItem(3)->setData(Qt::AccessibleTextRole, tr("Remove"));
        QLineEdit *last = nullptr;
        for (int i = 0; i < m_fields.size(); i++) {
            const Draft &f = m_fields[i];
            const QString label = f.name.trimmed().isEmpty() ? tr("field %1").arg(i + 1) : f.name.trimmed();

            auto *nameCell = new QWidget(table);
            nameCell->setProperty("fillsCell", true);
            auto *nv = new QVBoxLayout(nameCell);
            nv->setContentsMargins(0, 0, 0, 0);
            nv->setSpacing(S1);
            auto *name = new QLineEdit(f.name, nameCell);
            name->setObjectName(QStringLiteral("draftName:%1").arg(i));
            name->setAccessibleName(tr("Name of field %1").arg(i + 1));
            name->setPlaceholderText(tr("Name of the field"));
            name->setMinimumWidth(160);
            nv->addWidget(name);
            auto *problem = Web::text(QString(), "error", nameCell);
            problem->setObjectName(QStringLiteral("draftProblem:%1").arg(i));
            problem->setWordWrap(true);
            nv->addWidget(problem);
            m_problems << problem;
            last = name;
            connect(name, &QLineEdit::textChanged, this, [this, i, table](const QString &t) {
                m_fields[i].name = t;
                for (int j = 0; j < m_problems.size(); j++) {
                    const QString p = problemOf(j);
                    m_problems[j]->setText(p);
                    m_problems[j]->setVisible(!p.isEmpty());
                }
                table->fit();
                emit changed();
            });

            auto *level = new QComboBox(table);
            level->setObjectName(QStringLiteral("draftLevel:%1").arg(i));
            level->setAccessibleName(tr("Where %1 belongs").arg(label));
            for (const Level &l : levels())
                level->addItem(l.name, l.id);
            level->setCurrentIndex(qMax(0, level->findData(f.level)));
            connect(level, &QComboBox::activated, this, [this, i, level] {
                m_fields[i].level = level->currentData().toString();
                emit changed();
            });

            auto *kindCell = new QWidget(table);
            kindCell->setProperty("fillsCell", true);
            auto *kv = new QVBoxLayout(kindCell);
            kv->setContentsMargins(0, 0, 0, 0);
            kv->setSpacing(S1);
            auto *kind = kindCombo(f.kind, kindCell);
            kind->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
            kind->setObjectName(QStringLiteral("draftKind:%1").arg(i));
            kind->setAccessibleName(tr("Values of %1").arg(label));
            kv->addWidget(kind);
            connect(kind, &QComboBox::activated, this, [this, i, kind] {
                m_fields[i].kind = kind->currentData().toString();
                render();
                emit changed();
            });
            if (f.kind == QLatin1String("picklist")) {
                auto *values = new ValuesEdit(f.values, kindCell);
                values->setObjectName(QStringLiteral("draftValues:%1").arg(i));
                values->setAccessibleName(tr("The values of %1, one a line").arg(label));
                kv->addWidget(values);
                connect(values, &ValuesEdit::changed, this, [this, i, table](const QStringList &vs) {
                    m_fields[i].values = vs;
                    for (int j = 0; j < m_problems.size(); j++) {
                        const QString p = problemOf(j);
                        m_problems[j]->setText(p);
                        m_problems[j]->setVisible(!p.isEmpty());
                    }
                    table->fit();
                    emit changed();
                });
            }

            auto *remove = Web::button(QString(), Web::Quiet | Web::Small, table, QStringLiteral("trash"));
            remove->setObjectName(QStringLiteral("draftRemove:%1").arg(i));
            remove->setAccessibleName(tr("Remove %1").arg(label));
            remove->setToolTip(tr("Remove %1").arg(label));
            connect(remove, &QPushButton::clicked, this, [this, i] {
                // After the click is done with the button.
                QTimer::singleShot(0, this, [this, i] {
                    if (i < m_fields.size())
                        m_fields.removeAt(i);
                    render();
                    emit changed();
                });
            });
            const int row = table->addRow({nameCell, level, kindCell, Web::cellActions({remove}, table)});
            table->item(row, 0)->setData(Qt::AccessibleTextRole, label);
        }
        for (int j = 0; j < m_problems.size(); j++) {
            const QString p = problemOf(j);
            m_problems[j]->setText(p);
            m_problems[j]->setVisible(!p.isEmpty());
        }
        table->fit();
        m_layout->addWidget(table);
        if (m_focusLast && last)
            QTimer::singleShot(0, last, [last] { last->setFocus(); });
    }
    m_focusLast = false;
    auto *add = Web::button(tr("Add a field"), Web::Small, this, QStringLiteral("plus"));
    add->setObjectName(QStringLiteral("draftAdd"));
    connect(add, &QPushButton::clicked, this, [this] {
        QTimer::singleShot(0, this, [this] {
            m_fields << Draft{};
            m_focusLast = true;
            render();
            emit changed();
        });
    });
    m_layout->addWidget(Web::row({add}, this));
}

// AddFieldForm

AddFieldForm::AddFieldForm(const QString &termbase, const QStringList &taken, QWidget *parent)
    : Web::Switcher(parent)
    , m_termbase(termbase)
    , m_taken(taken)
{
    auto *closed = new QWidget(this);
    auto *cl = new QHBoxLayout(closed);
    cl->setContentsMargins(0, 0, 0, 0);
    auto *open = Web::button(tr("Add a field"), Web::Quiet | Web::Small, closed, QStringLiteral("plus"));
    open->setObjectName(QStringLiteral("addField"));
    cl->addWidget(open);
    cl->addStretch();
    add(closed);

    // .add-field
    auto *form = new QFrame(this);
    Theme::role(form, "box");
    form->setAccessibleName(tr("Add a field"));
    auto *v = new QVBoxLayout(form);
    v->setContentsMargins(S3, S3, S3, S3);
    v->setSpacing(S2);
    auto *label = bold(tr("Field name"), form);
    m_name = new QLineEdit(form);
    m_name->setObjectName(QStringLiteral("newFieldName"));
    m_name->setAccessibleName(tr("Field name"));
    m_name->setPlaceholderText(tr("For example Source, Region, Register"));
    label->setBuddy(m_name);
    v->addWidget(label);
    v->addWidget(m_name);
    m_hint = Web::text(QString(), "help", form);
    m_hint->setObjectName(QStringLiteral("newFieldHint"));
    m_hint->setWordWrap(true);
    v->addWidget(m_hint);
    v->addWidget(bold(tr("Where does the value belong?"), form));
    auto *group = new QButtonGroup(form);
    for (const Level &l : levels()) {
        auto *r = new QRadioButton(form);
        r->setObjectName(QStringLiteral("newFieldLevel:%1").arg(l.id));
        r->setProperty("level", l.id);
        group->addButton(r);
        m_levels << r;
        v->addWidget(checkText(r, QStringLiteral("%1: %2").arg(l.name, l.hint), form));
        connect(r, &QRadioButton::toggled, this, [this, r](bool on) {
            if (on)
                m_draft.level = r->property("level").toString();
        });
    }
    auto *kindLabel = bold(tr("Values"), form);
    m_kind = kindCombo(QStringLiteral("text"), form);
    m_kind->setObjectName(QStringLiteral("newFieldKind"));
    m_kind->setAccessibleName(tr("Values"));
    m_kind->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    kindLabel->setBuddy(m_kind);
    v->addWidget(kindLabel);
    v->addWidget(m_kind, 0, Qt::AlignLeft);
    m_valuesBox = new QWidget(form);
    auto *vv = new QVBoxLayout(m_valuesBox);
    vv->setContentsMargins(0, 0, 0, 0);
    m_values = new ValuesEdit({}, m_valuesBox);
    m_values->setObjectName(QStringLiteral("newFieldValues"));
    m_values->setAccessibleName(tr("The values of the list, one a line"));
    vv->addWidget(m_values);
    v->addWidget(m_valuesBox);
    m_add = Web::button(tr("Add field"), Web::Primary | Web::Small, form);
    m_add->setObjectName(QStringLiteral("addFieldSubmit"));
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, form);
    v->addWidget(Web::row({m_add, cancel}, form));
    add(form);

    connect(open, &QPushButton::clicked, this, [this] {
        setCurrent(1);
        m_name->setFocus();
    });
    connect(cancel, &QPushButton::clicked, this, [this] {
        reset();
        setCurrent(0);
    });
    connect(m_name, &QLineEdit::textChanged, this, [this](const QString &t) {
        m_draft.name = t;
        check();
    });
    connect(m_kind, &QComboBox::activated, this, [this] {
        m_draft.kind = m_kind->currentData().toString();
        check();
    });
    connect(m_values, &ValuesEdit::changed, this, [this](const QStringList &vs) {
        m_draft.values = vs;
        check();
    });
    const auto submit = [this] {
        if (!problem(m_draft, m_taken).isEmpty())
            return;
        Draft f = m_draft;
        f.name = f.name.trimmed();
        if (f.kind != QLatin1String("picklist"))
            f.values.clear();
        m_taken << f.name;
        reset();
        setCurrent(0);
        emit added(toJson(f));
    };
    connect(m_add, &QPushButton::clicked, this, submit);
    connect(m_name, &QLineEdit::returnPressed, this, submit);
    reset();
}

void AddFieldForm::reset()
{
    m_draft = Draft{};
    const QSignalBlocker b1(m_name);
    m_name->clear();
    m_kind->setCurrentIndex(0);
    m_values->setValues({});
    if (!m_levels.isEmpty())
        m_levels.first()->setChecked(true);
    check();
}

void AddFieldForm::check()
{
    const QString typed = m_draft.name.trimmed();
    QString same;
    QString similar;
    for (const QString &t : m_taken) {
        if (!typed.isEmpty() && t.trimmed().compare(typed, Qt::CaseInsensitive) == 0)
            same = t;
    }
    if (same.isEmpty() && typed.size() >= 3) {
        for (const QString &t : m_taken) {
            if (t.startsWith(typed.left(3), Qt::CaseInsensitive)) {
                similar = t;
                break;
            }
        }
    }
    m_hint->setText(!same.isEmpty() ? tr("%1 already has “%2”.").arg(m_termbase, same)
                    : !similar.isEmpty() ? tr("Did you mean the existing field “%1”?").arg(similar)
                                         : tr("Every entry in this termbase gets the field. Nothing else changes."));
    m_valuesBox->setVisible(m_draft.kind == QLatin1String("picklist"));
    m_add->setEnabled(problem(m_draft, m_taken).isEmpty());
}

// LanguageChecklist

LanguageChecklist::LanguageChecklist(const QString &label, QWidget *parent)
    : QWidget(parent)
{
    auto *v = new QVBoxLayout(this);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S2);
    m_chips = new QWidget(this);
    m_chips->setObjectName(QStringLiteral("chosenLanguages"));
    m_chips->setAccessibleName(tr("Chosen languages"));
    new FlowLayout(m_chips, S1);
    m_chips->hide();
    v->addWidget(m_chips);
    m_search = new QLineEdit(this);
    m_search->setObjectName(QStringLiteral("languageSearch"));
    m_search->setAccessibleName(tr("Find a language"));
    m_search->setPlaceholderText(tr("Find a language, such as German or de-AT"));
    m_search->setClearButtonEnabled(true);
    m_search->addAction(Icons::get(QStringLiteral("search"), Theme::color("text3")), QLineEdit::LeadingPosition);
    v->addWidget(m_search);

    auto *scroll = new QScrollArea(this);
    scroll->setObjectName(QStringLiteral("languageList"));
    Theme::role(scroll, "box");
    scroll->setWidgetResizable(true);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    scroll->setFixedHeight(260);
    auto *inner = new QWidget(scroll);
    inner->setAccessibleName(label);
    auto *outer = new QVBoxLayout(inner);
    outer->setContentsMargins(S2, S2, S2, S2);
    outer->setSpacing(0);
    auto *listHost = new QWidget(inner);
    auto *flow = new FlowLayout(listHost, S1);
    for (const QString &tag : Languages::common()) {
        auto *item = new QWidget(listHost);
        item->setFixedWidth(200);
        auto *h = new QHBoxLayout(item);
        h->setContentsMargins(0, 0, 0, 0);
        h->setSpacing(S2);
        auto *box = new QCheckBox(item);
        box->setObjectName(QStringLiteral("language:%1").arg(tag));
        box->setAccessibleName(QStringLiteral("%1 %2").arg(Languages::name(tag), tag));
        h->addWidget(box, 0, Qt::AlignTop);
        auto *text = new QLabel(QStringLiteral("%1 <span style=\"color:%2; white-space:nowrap\">%3</span>").arg(Languages::name(tag).toHtmlEscaped(), Theme::color("text3").name(), tag), item);
        text->setTextFormat(Qt::RichText);
        text->setWordWrap(true);
        text->setCursor(Qt::PointingHandCursor);
        text->installEventFilter(new ClickText(box, text));
        h->addWidget(text, 1);
        flow->addWidget(item);
        m_items.append({tag, item});
        connect(box, &QCheckBox::toggled, this, [this, tag](bool on) {
            if (on == m_chosen.contains(tag))
                return;
            if (on)
                m_chosen << tag;
            else
                m_chosen.removeAll(tag);
            showChips();
            emit changed(m_chosen);
        });
    }
    outer->addWidget(listHost);
    m_none = Web::text(tr("No language is called that."), "muted", inner);
    m_none->hide();
    outer->addWidget(m_none);
    outer->addStretch();
    scroll->setWidget(inner);
    v->addWidget(scroll);
    connect(m_search, &QLineEdit::textChanged, this, &LanguageChecklist::filter);
}

QStringList LanguageChecklist::shown() const
{
    QStringList out;
    for (const auto &[tag, item] : m_items) {
        if (!item->isHidden())
            out << tag;
    }
    return out;
}

void LanguageChecklist::setChosen(const QStringList &tags)
{
    m_chosen = tags;
    for (const auto &[tag, item] : m_items) {
        if (auto *box = item->findChild<QCheckBox *>()) {
            const QSignalBlocker b(box);
            box->setChecked(tags.contains(tag));
        }
    }
    showChips();
}

void LanguageChecklist::showChips()
{
    QLayout *flow = m_chips->layout();
    while (QLayoutItem *item = flow->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        }
        delete item;
    }
    for (const QString &tag : m_chosen) {
        auto *b = new QPushButton(QStringLiteral("%1 ×").arg(Languages::name(tag)), m_chips);
        b->setProperty("chipButton", true);
        b->setCursor(Qt::PointingHandCursor);
        b->setAccessibleName(tr("Remove %1").arg(Languages::name(tag)));
        connect(b, &QPushButton::clicked, this, [this, tag] {
            QTimer::singleShot(0, this, [this, tag] {
                for (const auto &[t, item] : m_items) {
                    if (t == tag) {
                        if (auto *box = item->findChild<QCheckBox *>())
                            box->setChecked(false);
                    }
                }
            });
        });
        flow->addWidget(b);
    }
    m_chips->setVisible(!m_chosen.isEmpty());
}

void LanguageChecklist::filter()
{
    const QString f = m_search->text().trimmed();
    int shownCount = 0;
    for (const auto &[tag, item] : m_items) {
        const bool show = f.isEmpty() || Languages::name(tag).contains(f, Qt::CaseInsensitive) || tag.contains(f, Qt::CaseInsensitive);
        item->setVisible(show);
        shownCount += show ? 1 : 0;
    }
    m_none->setVisible(shownCount == 0);
    if (QWidget *host = m_items.isEmpty() ? nullptr : m_items.first().second->parentWidget())
        host->updateGeometry();
}

} // namespace TermFields
