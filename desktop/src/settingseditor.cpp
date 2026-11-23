#include "settingseditor.h"

#include "apiclient.h"
#include "connection.h"
#include "languages.h"

#include <QCheckBox>
#include <QComboBox>
#include <QDateTimeEdit>
#include <QFormLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QLineEdit>
#include <QListWidget>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QRadioButton>
#include <QSpinBox>
#include <QTableWidget>
#include <QVBoxLayout>

#include <memory>

namespace {

QLabel *problemLabel(QWidget *parent)
{
    auto *l = new QLabel(parent);
    l->setWordWrap(true);
    l->setTextFormat(Qt::PlainText);
    QPalette p = l->palette();
    p.setColor(QPalette::WindowText, QColor(0xaa, 0x22, 0x19));
    l->setPalette(p);
    l->setVisible(false);
    return l;
}

QLabel *buddy(const QString &text, QWidget *w, QWidget *parent)
{
    auto *l = new QLabel(text, parent);
    l->setBuddy(w);
    return l;
}

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

} // namespace

SettingsEditor::SettingsEditor(QWidget *parent)
    : QTabWidget(parent)
{
    setObjectName(QStringLiteral("settingsEditor"));
    setAccessibleName(tr("Project settings"));
    addTab(generalTab(), tr("&General"));
    addTab(memoriesTab(), tr("&Memories"));
    addTab(terminologyTab(), tr("&Terminology"));
    addTab(checksTab(), tr("&Checks"));
    addTab(preparationTab(), tr("&Preparation"));
}

QWidget *SettingsEditor::generalTab()
{
    auto *w = new QWidget(this);
    auto *form = new QFormLayout(w);
    m_name = new QLineEdit(w);
    m_name->setAccessibleName(tr("Project name"));
    form->addRow(buddy(tr("&Name:"), m_name, w), m_name);
    m_client = new QLineEdit(w);
    m_client->setAccessibleName(tr("Client"));
    form->addRow(buddy(tr("C&lient:"), m_client, w), m_client);
    m_description = new QPlainTextEdit(w);
    m_description->setAccessibleName(tr("Description"));
    m_description->setTabChangesFocus(true);
    m_description->setMaximumHeight(80);
    form->addRow(buddy(tr("&Description:"), m_description, w), m_description);
    auto *dueRow = new QHBoxLayout;
    m_hasDue = new QCheckBox(tr("D&ue"), w);
    m_due = new QDateTimeEdit(QDateTime::currentDateTime().addDays(7), w);
    m_due->setCalendarPopup(true);
    m_due->setAccessibleName(tr("Due date"));
    m_due->setEnabled(false);
    dueRow->addWidget(m_hasDue);
    dueRow->addWidget(m_due, 1);
    form->addRow(tr("Deadline:"), dueRow);
    connect(m_hasDue, &QCheckBox::toggled, m_due, &QWidget::setEnabled);
    m_source = new LanguageCombo(w);
    m_source->setAccessibleName(tr("Source language"));
    form->addRow(buddy(tr("&Source language:"), m_source, w), m_source);
    m_targets = new QListWidget(w);
    m_targets->setAccessibleName(tr("Target languages"));
    m_targets->setMaximumHeight(110);
    form->addRow(buddy(tr("&Target languages:"), m_targets, w), m_targets);
    auto *addRow = new QHBoxLayout;
    m_newTarget = new LanguageCombo(w);
    m_newTarget->setAccessibleName(tr("Target language to add"));
    auto *add = new QPushButton(tr("&Add"), w);
    auto *remove = new QPushButton(tr("&Remove"), w);
    addRow->addWidget(m_newTarget, 1);
    addRow->addWidget(add);
    addRow->addWidget(remove);
    form->addRow(QString(), addRow);
    connect(add, &QPushButton::clicked, this, &SettingsEditor::addTarget);
    connect(remove, &QPushButton::clicked, this, [this] {
        delete m_targets->currentItem();
        storeMemories();
        refreshTargets();
        emit changed();
    });
    auto *problems = problemLabel(w);
    form->addRow(problems);
    m_problemLabels << problems;
    for (QLineEdit *e : {m_name, m_client})
        connect(e, &QLineEdit::textChanged, this, &SettingsEditor::changed);
    return w;
}

QWidget *SettingsEditor::memoriesTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    auto *intro = new QLabel(tr("For each target language: the memories the project looks up matches in, searches with the concordance, and stores confirmed translations in. The first wins a tie."), w);
    intro->setWordWrap(true);
    l->addWidget(intro);
    m_projectMemory = new QCheckBox(tr("Give the project a memory of its &own for each language"), w);
    l->addWidget(m_projectMemory);
    auto *row = new QHBoxLayout;
    m_memLang = new QComboBox(w);
    m_memLang->setAccessibleName(tr("Target language"));
    row->addWidget(buddy(tr("&Language:"), m_memLang, w));
    row->addWidget(m_memLang, 1);
    l->addLayout(row);
    m_memTable = new QTableWidget(0, 5, w);
    m_memTable->setAccessibleName(tr("Memories of the language"));
    m_memTable->setHorizontalHeaderLabels({tr("Memory"), tr("Look up"), tr("Concordance"), tr("Update"), tr("Penalty")});
    m_memTable->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_memTable->verticalHeader()->setVisible(false);
    m_memTable->setSelectionBehavior(QAbstractItemView::SelectRows);
    l->addWidget(m_memTable, 1);
    auto *addRow = new QHBoxLayout;
    m_addMemory = new QComboBox(w);
    m_addMemory->setAccessibleName(tr("Memory to add"));
    auto *add = new QPushButton(tr("A&dd memory"), w);
    auto *remove = new QPushButton(tr("Re&move"), w);
    auto *up = new QPushButton(tr("Move &up"), w);
    addRow->addWidget(m_addMemory, 1);
    addRow->addWidget(add);
    addRow->addWidget(remove);
    addRow->addWidget(up);
    l->addLayout(addRow);
    auto *problems = problemLabel(w);
    l->addWidget(problems);
    m_problemLabels << problems;
    connect(m_memLang, &QComboBox::currentIndexChanged, this, [this] { refreshMemories(); });
    connect(m_memTable, &QTableWidget::itemChanged, this, [this] { storeMemories(); emit changed(); });
    connect(add, &QPushButton::clicked, this, &SettingsEditor::addMemory);
    connect(remove, &QPushButton::clicked, this, [this] {
        const int r = m_memTable->currentRow();
        if (r < 0)
            return;
        m_memTable->removeRow(r);
        storeMemories();
        emit changed();
    });
    connect(up, &QPushButton::clicked, this, [this] {
        const int r = m_memTable->currentRow();
        const QString lang = m_memLang->currentData().toString();
        QJsonArray a = memoriesFor(lang);
        if (r <= 0 || r >= a.size())
            return;
        const QJsonValue v = a.at(r);
        a.removeAt(r);
        a.insert(r - 1, v);
        m_memories.insert(lang, a);
        refreshMemories();
        m_memTable->selectRow(r - 1);
        emit changed();
    });
    return w;
}

QWidget *SettingsEditor::terminologyTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    m_own = new QRadioButton(tr("A termbase of the project's &own"), w);
    m_layered = new QRadioButton(tr("A project termbase over &master termbases: new terms go into the project, and go to the masters through a review"), w);
    m_direct = new QRadioButton(tr("Master termbases &directly: new terms go straight into one of them"), w);
    for (QRadioButton *b : {m_own, m_layered, m_direct}) {
        l->addWidget(b);
        connect(b, &QRadioButton::toggled, this, [this] { refreshTerminology(); emit changed(); });
    }
    m_masters = new QListWidget(w);
    m_masters->setAccessibleName(tr("Master termbases"));
    l->addWidget(buddy(tr("Master termbases (&checked ones are used):"), m_masters, w));
    l->addWidget(m_masters, 1);
    auto *row = new QHBoxLayout;
    m_writeTo = new QComboBox(w);
    m_writeTo->setAccessibleName(tr("New terms go into"));
    row->addWidget(buddy(tr("New terms go &into:"), m_writeTo, w));
    row->addWidget(m_writeTo, 1);
    l->addLayout(row);
    m_autoUpdate = new QCheckBox(tr("Take in the masters' changes when the project opens"), w);
    l->addWidget(m_autoUpdate);
    m_termbaseNote = new QLabel(w);
    m_termbaseNote->setWordWrap(true);
    l->addWidget(m_termbaseNote);
    auto *problems = problemLabel(w);
    l->addWidget(problems);
    m_problemLabels << problems;
    connect(m_masters, &QListWidget::itemChanged, this, [this] { refreshTerminology(); emit changed(); });
    return w;
}

QWidget *SettingsEditor::checksTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    m_checks = new QListWidget(w);
    m_checks->setAccessibleName(tr("Checks that run"));
    l->addWidget(buddy(tr("Checks that &run while translating:"), m_checks, w));
    l->addWidget(m_checks, 2);
    m_uneditedFuzzy = new QCheckBox(tr("Warn when a fuzzy match is confirmed &unchanged"), w);
    l->addWidget(m_uneditedFuzzy);
    m_dictTable = new QTableWidget(0, 2, w);
    m_dictTable->setAccessibleName(tr("Spelling dictionaries"));
    m_dictTable->setHorizontalHeaderLabels({tr("Language"), tr("Spelling dictionary")});
    m_dictTable->horizontalHeader()->setSectionResizeMode(1, QHeaderView::Stretch);
    m_dictTable->verticalHeader()->setVisible(false);
    m_dictTable->setMaximumHeight(120);
    l->addWidget(buddy(tr("&Spelling:"), m_dictTable, w));
    l->addWidget(m_dictTable);
    m_words = new QPlainTextEdit(w);
    m_words->setAccessibleName(tr("Words always spelled right"));
    m_words->setTabChangesFocus(true);
    m_words->setMaximumHeight(70);
    l->addWidget(buddy(tr("&Words always spelled right (one per line):"), m_words, w));
    l->addWidget(m_words);
    m_rules = new QTableWidget(0, 6, w);
    m_rules->setAccessibleName(tr("Checks of your own"));
    m_rules->setHorizontalHeaderLabels({tr("Name"), tr("Where the source matches"), tr("The translation"), tr("Rule"), tr("Severity"), tr("Message")});
    m_rules->horizontalHeader()->setSectionResizeMode(QHeaderView::Stretch);
    m_rules->verticalHeader()->setVisible(false);
    l->addWidget(buddy(tr("Checks of your &own (regular expressions):"), m_rules, w));
    l->addWidget(m_rules, 1);
    auto *row = new QHBoxLayout;
    auto *addRule = new QPushButton(tr("Add c&heck"), w);
    auto *removeRule = new QPushButton(tr("Remove ch&eck"), w);
    row->addWidget(addRule);
    row->addWidget(removeRule);
    row->addStretch();
    l->addLayout(row);
    auto *problems = problemLabel(w);
    l->addWidget(problems);
    m_problemLabels << problems;
    const auto ruleRow = [this](const QJsonObject &r) {
        const int i = m_rules->rowCount();
        m_rules->insertRow(i);
        m_rules->setItem(i, 0, new QTableWidgetItem(r.value(QLatin1String("name")).toString()));
        m_rules->setItem(i, 1, new QTableWidgetItem(r.value(QLatin1String("source")).toString()));
        m_rules->setItem(i, 2, new QTableWidgetItem(r.value(QLatin1String("target")).toString()));
        auto *mode = new QComboBox(m_rules);
        mode->addItem(tr("must match"), QStringLiteral("must"));
        mode->addItem(tr("must not match"), QStringLiteral("must-not"));
        mode->addItem(tr("matches as often"), QStringLiteral("count"));
        mode->setCurrentIndex(qMax(0, mode->findData(r.value(QLatin1String("mode")).toString())));
        m_rules->setCellWidget(i, 3, mode);
        auto *sev = new QComboBox(m_rules);
        sev->addItem(tr("Warning"), QStringLiteral("warning"));
        sev->addItem(tr("Error"), QStringLiteral("error"));
        sev->setCurrentIndex(qMax(0, sev->findData(r.value(QLatin1String("severity")).toString())));
        m_rules->setCellWidget(i, 4, sev);
        m_rules->setItem(i, 5, new QTableWidgetItem(r.value(QLatin1String("message")).toString()));
    };
    connect(addRule, &QPushButton::clicked, this, [this, ruleRow] {
        ruleRow(QJsonObject{{QStringLiteral("mode"), QStringLiteral("must-not")}});
        m_rules->setCurrentCell(m_rules->rowCount() - 1, 0);
        m_rules->editItem(m_rules->item(m_rules->rowCount() - 1, 0));
        emit changed();
    });
    connect(removeRule, &QPushButton::clicked, this, [this] {
        if (m_rules->currentRow() >= 0) {
            m_rules->removeRow(m_rules->currentRow());
            emit changed();
        }
    });
    // setSettings fills the rules with the same rows.
    m_ruleRow = ruleRow;
    return w;
}

QWidget *SettingsEditor::preparationTab()
{
    auto *w = new QWidget(this);
    auto *form = new QFormLayout(w);
    m_pretranslate = new QCheckBox(tr("&Fill empty segments with the best memory match when the project is made"), w);
    form->addRow(m_pretranslate);
    m_minScore = new QSpinBox(w);
    m_minScore->setRange(50, 100);
    m_minScore->setSuffix(QStringLiteral(" %"));
    m_minScore->setAccessibleName(tr("Lowest match inserted"));
    form->addRow(buddy(tr("&Lowest match inserted:"), m_minScore, w), m_minScore);
    m_confirmExact = new QCheckBox(tr("Confirm segments filled with a 100 % &match"), w);
    form->addRow(m_confirmExact);
    m_confirmContext = new QCheckBox(tr("Confirm segments filled with a &context match"), w);
    form->addRow(m_confirmContext);
    m_context = new QComboBox(w);
    m_context->addItem(tr("The segment before matches too (as Trados)"), QStringLiteral("previous"));
    m_context->addItem(tr("The segments before and after match too (as memoQ)"), QStringLiteral("both"));
    m_context->setAccessibleName(tr("Context matches"));
    form->addRow(buddy(tr("Con&text match:"), m_context, w), m_context);
    auto *problems = problemLabel(w);
    form->addRow(problems);
    m_problemLabels << problems;
    connect(m_pretranslate, &QCheckBox::toggled, this, [this](bool on) {
        m_minScore->setEnabled(on);
        m_confirmExact->setEnabled(on);
        m_confirmContext->setEnabled(on);
    });
    return w;
}

void SettingsEditor::setCatalog(const QJsonObject &catalog, const QJsonArray &dictionaries, const QJsonArray &qaKinds)
{
    m_catalog = catalog;
    m_dicts = dictionaries;
    m_kinds = qaKinds;
}

void SettingsEditor::load(Connection *c, std::function<void(const QString &)> done)
{
    auto pending = std::make_shared<int>(3);
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
    onResult(c->api()->get(QStringLiteral("/api/qa/kinds")), this, [kinds, error, finish](const ApiResult &r) {
        if (r.ok())
            *kinds = r.array();
        else
            *error = r.error.message;
        finish();
    });
}

void SettingsEditor::setSettings(const QJsonObject &s, bool newProject)
{
    m_settings = s;
    m_new = newProject;
    const QSignalBlocker block(this);
    m_name->setText(s.value(QLatin1String("name")).toString());
    m_client->setText(s.value(QLatin1String("client")).toString());
    m_description->setPlainText(s.value(QLatin1String("description")).toString());
    const QDateTime due = QDateTime::fromString(s.value(QLatin1String("due")).toString(), Qt::ISODate);
    m_hasDue->setChecked(due.isValid());
    if (due.isValid())
        m_due->setDateTime(due.toLocalTime());
    m_source->setLanguage(s.value(QLatin1String("sourceLang")).toString());
    m_source->setEnabled(newProject);
    m_targets->clear();
    for (const QString &t : strings(s.value(QLatin1String("targetLangs")))) {
        auto *item = new QListWidgetItem(Languages::label(t), m_targets);
        item->setData(Qt::UserRole, t);
    }
    m_memories = s.value(QLatin1String("memories")).toObject();
    m_projectMemory->setChecked(s.value(QLatin1String("projectMemory")).toBool());
    m_projectMemory->setVisible(newProject);

    const QJsonObject t = s.value(QLatin1String("terminology")).toObject();
    const QString mode = t.value(QLatin1String("mode")).toString();
    (mode == QLatin1String("layered") ? m_layered : mode == QLatin1String("direct") ? m_direct : m_own)->setChecked(true);
    m_masters->clear();
    const QStringList used = strings(t.value(QLatin1String("masters")));
    for (const QJsonValue &v : m_catalog.value(QLatin1String("termbases")).toArray()) {
        const QJsonObject tb = v.toObject();
        if (tb.value(QLatin1String("kind")).toString() == QLatin1String("project"))
            continue;
        const QString name = tb.value(QLatin1String("name")).toString();
        auto *item = new QListWidgetItem(QStringLiteral("%1 (%2)").arg(name, strings(tb.value(QLatin1String("languages"))).join(QStringLiteral(", "))), m_masters);
        item->setData(Qt::UserRole, name);
        item->setFlags(item->flags() | Qt::ItemIsUserCheckable);
        item->setCheckState(used.contains(name) ? Qt::Checked : Qt::Unchecked);
    }
    // Masters kept elsewhere stay in the settings.
    for (const QString &name : used) {
        bool found = false;
        for (int i = 0; i < m_masters->count(); ++i)
            found = found || m_masters->item(i)->data(Qt::UserRole).toString() == name;
        if (!found) {
            auto *item = new QListWidgetItem(tr("%1 (not on this connection)").arg(name), m_masters);
            item->setData(Qt::UserRole, name);
            item->setFlags(item->flags() | Qt::ItemIsUserCheckable);
            item->setCheckState(Qt::Checked);
        }
    }
    m_autoUpdate->setChecked(t.value(QLatin1String("autoUpdate")).toBool());
    // The kind of terminology is chosen when the project is made.
    for (QRadioButton *b : {m_own, m_layered, m_direct})
        b->setEnabled(newProject);

    const QJsonObject qa = s.value(QLatin1String("qa")).toObject();
    const QStringList disabled = strings(qa.value(QLatin1String("disabled")));
    const QStringList enabled = strings(qa.value(QLatin1String("enabled")));
    m_checks->clear();
    for (const QJsonValue &v : m_kinds) {
        const QJsonObject k = v.toObject();
        const QString kind = k.value(QLatin1String("kind")).toString();
        const bool on = k.value(QLatin1String("default")).toBool() ? !disabled.contains(kind) : enabled.contains(kind);
        auto *item = new QListWidgetItem(k.value(QLatin1String("name")).toString(), m_checks);
        item->setToolTip(k.value(QLatin1String("description")).toString());
        item->setData(Qt::UserRole, kind);
        item->setData(Qt::UserRole + 1, k.value(QLatin1String("default")).toBool());
        item->setFlags(item->flags() | Qt::ItemIsUserCheckable);
        item->setCheckState(on ? Qt::Checked : Qt::Unchecked);
    }
    m_uneditedFuzzy->setChecked(qa.value(QLatin1String("uneditedFuzzy")).toBool());
    m_words->setPlainText(strings(qa.value(QLatin1String("words"))).join(QLatin1Char('\n')));
    m_rules->setRowCount(0);
    for (const QJsonValue &v : qa.value(QLatin1String("rules")).toArray())
        m_ruleRow(v.toObject());

    const QJsonObject prep = s.value(QLatin1String("prepare")).toObject();
    m_pretranslate->setChecked(prep.value(QLatin1String("pretranslate")).toBool());
    m_minScore->setValue(prep.value(QLatin1String("minScore")).toInt(100));
    m_confirmExact->setChecked(prep.value(QLatin1String("confirmExact")).toBool());
    m_confirmContext->setChecked(prep.value(QLatin1String("confirmContext")).toBool());
    m_minScore->setEnabled(m_pretranslate->isChecked());
    m_confirmExact->setEnabled(m_pretranslate->isChecked());
    m_confirmContext->setEnabled(m_pretranslate->isChecked());
    // Pre-translation is a step of making a project; later it is a button.
    m_pretranslate->setVisible(newProject);
    m_context->setCurrentIndex(qMax(0, m_context->findData(s.value(QLatin1String("analysis")).toObject().value(QLatin1String("context")).toString())));
    refreshTargets();
    refreshTerminology();
    showProblems({});
}

QStringList SettingsEditor::targets() const
{
    QStringList out;
    for (int i = 0; i < m_targets->count(); ++i)
        out << m_targets->item(i)->data(Qt::UserRole).toString();
    return out;
}

void SettingsEditor::addTarget()
{
    const QString t = m_newTarget->language();
    if (t.isEmpty() || targets().contains(t))
        return;
    auto *item = new QListWidgetItem(Languages::label(t), m_targets);
    item->setData(Qt::UserRole, t);
    m_newTarget->setCurrentIndex(-1);
    refreshTargets();
    emit changed();
}

void SettingsEditor::refreshTargets()
{
    const QString keep = m_memLang->currentData().toString();
    {
        const QSignalBlocker b(m_memLang);
        m_memLang->clear();
        for (const QString &t : targets())
            m_memLang->addItem(Languages::label(t), t);
        const int i = m_memLang->findData(keep);
        m_memLang->setCurrentIndex(i >= 0 ? i : 0);
    }
    refreshMemories();
    refreshDictionaries();
}

QJsonArray SettingsEditor::memoriesFor(const QString &lang) const
{
    return m_memories.value(lang).toArray();
}

void SettingsEditor::refreshMemories()
{
    const QString lang = m_memLang->currentData().toString();
    const QJsonArray ms = memoriesFor(lang);
    const QSignalBlocker b(m_memTable);
    m_memTable->setRowCount(0);
    for (const QJsonValue &v : ms) {
        const QJsonObject m = v.toObject();
        const int r = m_memTable->rowCount();
        m_memTable->insertRow(r);
        auto *name = new QTableWidgetItem(m.value(QLatin1String("memory")).toString());
        name->setFlags(name->flags() & ~Qt::ItemIsEditable);
        m_memTable->setItem(r, 0, name);
        int col = 1;
        for (const char *key : {"lookup", "concordance", "update"}) {
            auto *c = new QTableWidgetItem;
            c->setFlags(Qt::ItemIsUserCheckable | Qt::ItemIsEnabled | Qt::ItemIsSelectable);
            c->setCheckState(m.value(QLatin1String(key)).toBool() ? Qt::Checked : Qt::Unchecked);
            m_memTable->setItem(r, col++, c);
        }
        auto *pen = new QSpinBox(m_memTable);
        pen->setRange(0, 100);
        pen->setValue(m.value(QLatin1String("penalty")).toInt());
        pen->setAccessibleName(tr("Penalty of %1").arg(m.value(QLatin1String("memory")).toString()));
        m_memTable->setCellWidget(r, 4, pen);
        connect(pen, &QSpinBox::valueChanged, this, [this] { storeMemories(); emit changed(); });
    }
    // Memories of the language pair that are not in the list yet.
    m_addMemory->clear();
    const QString src = m_source->language();
    QStringList inList;
    for (const QJsonValue &v : ms)
        inList << v.toObject().value(QLatin1String("memory")).toString();
    for (const QJsonValue &v : m_catalog.value(QLatin1String("memories")).toArray()) {
        const QJsonObject m = v.toObject();
        const QString name = m.value(QLatin1String("name")).toString();
        if (inList.contains(name) || !Languages::samePrimary(m.value(QLatin1String("sourceLang")).toString(), src) ||
            !Languages::samePrimary(m.value(QLatin1String("targetLang")).toString(), lang))
            continue;
        m_addMemory->addItem(tr("%1 (%2 units)").arg(name).arg(m.value(QLatin1String("units")).toInt()), name);
    }
}

void SettingsEditor::storeMemories()
{
    const QString lang = m_memLang->currentData().toString();
    if (lang.isEmpty())
        return;
    QJsonArray a;
    for (int r = 0; r < m_memTable->rowCount(); ++r) {
        auto *pen = qobject_cast<QSpinBox *>(m_memTable->cellWidget(r, 4));
        a.append(QJsonObject{{QStringLiteral("memory"), m_memTable->item(r, 0)->text()},
                             {QStringLiteral("lookup"), m_memTable->item(r, 1)->checkState() == Qt::Checked},
                             {QStringLiteral("concordance"), m_memTable->item(r, 2)->checkState() == Qt::Checked},
                             {QStringLiteral("update"), m_memTable->item(r, 3)->checkState() == Qt::Checked},
                             {QStringLiteral("penalty"), pen ? pen->value() : 0}});
    }
    m_memories.insert(lang, a);
    // Languages no longer targets lose their memories.
    for (const QString &k : m_memories.keys()) {
        if (!targets().contains(k))
            m_memories.remove(k);
    }
}

void SettingsEditor::addMemory()
{
    const QString name = m_addMemory->currentData().toString();
    const QString lang = m_memLang->currentData().toString();
    if (name.isEmpty() || lang.isEmpty())
        return;
    QJsonArray a = memoriesFor(lang);
    a.append(QJsonObject{{QStringLiteral("memory"), name}, {QStringLiteral("lookup"), true}, {QStringLiteral("concordance"), true},
                         {QStringLiteral("update"), false}, {QStringLiteral("penalty"), 0}});
    m_memories.insert(lang, a);
    refreshMemories();
    emit changed();
}

void SettingsEditor::refreshTerminology()
{
    const bool usesMasters = !m_own->isChecked();
    m_masters->setEnabled(usesMasters);
    m_writeTo->setEnabled(m_direct->isChecked());
    m_autoUpdate->setEnabled(m_layered->isChecked());
    const QString keep = m_writeTo->currentData().toString().isEmpty()
        ? m_settings.value(QLatin1String("terminology")).toObject().value(QLatin1String("writeTo")).toString()
        : m_writeTo->currentData().toString();
    m_writeTo->clear();
    for (int i = 0; i < m_masters->count(); ++i) {
        if (m_masters->item(i)->checkState() == Qt::Checked)
            m_writeTo->addItem(m_masters->item(i)->data(Qt::UserRole).toString(), m_masters->item(i)->data(Qt::UserRole));
    }
    m_writeTo->setCurrentIndex(qMax(0, m_writeTo->findData(keep)));
    const QString tb = m_settings.value(QLatin1String("terminology")).toObject().value(QLatin1String("termbase")).toString();
    if (!m_new && !tb.isEmpty())
        m_termbaseNote->setText(tr("The project termbase is %1.").arg(tb));
    else if (m_new && !m_direct->isChecked())
        m_termbaseNote->setText(tr("The project termbase is made with the project."));
    else
        m_termbaseNote->clear();
}

void SettingsEditor::refreshDictionaries()
{
    const QJsonObject current = m_settings.value(QLatin1String("qa")).toObject().value(QLatin1String("dictionaries")).toObject();
    QHash<QString, QString> keep;
    for (int r = 0; r < m_dictTable->rowCount(); ++r) {
        if (auto *c = qobject_cast<QComboBox *>(m_dictTable->cellWidget(r, 1)))
            keep.insert(m_dictTable->item(r, 0)->data(Qt::UserRole).toString(), c->currentData().toString());
    }
    m_dictTable->setRowCount(0);
    for (const QString &t : targets()) {
        const int r = m_dictTable->rowCount();
        m_dictTable->insertRow(r);
        auto *lang = new QTableWidgetItem(Languages::label(t));
        lang->setData(Qt::UserRole, t);
        lang->setFlags(lang->flags() & ~Qt::ItemIsEditable);
        m_dictTable->setItem(r, 0, lang);
        auto *combo = new QComboBox(m_dictTable);
        combo->setAccessibleName(tr("Spelling dictionary for %1").arg(Languages::name(t)));
        combo->addItem(tr("No spelling check"), QString());
        for (const QJsonValue &v : m_dicts) {
            const QJsonObject d = v.toObject();
            if (Languages::samePrimary(d.value(QLatin1String("lang")).toString(), t))
                combo->addItem(QStringLiteral("%1 (%2)").arg(d.value(QLatin1String("name")).toString(), d.value(QLatin1String("lang")).toString()),
                               d.value(QLatin1String("name")).toString());
        }
        const QString want = keep.contains(t) ? keep.value(t) : current.value(t).toString();
        combo->setCurrentIndex(qMax(0, combo->findData(want)));
        m_dictTable->setCellWidget(r, 1, combo);
        connect(combo, &QComboBox::currentIndexChanged, this, &SettingsEditor::changed);
    }
}

QJsonObject SettingsEditor::settings() const
{
    QJsonObject s = m_settings;
    s.insert(QStringLiteral("name"), m_name->text().trimmed());
    s.insert(QStringLiteral("client"), m_client->text().trimmed());
    s.insert(QStringLiteral("description"), m_description->toPlainText().trimmed());
    s.insert(QStringLiteral("due"), m_hasDue->isChecked() ? m_due->dateTime().toUTC().toString(Qt::ISODate) : QString());
    s.insert(QStringLiteral("sourceLang"), m_source->language());
    s.insert(QStringLiteral("targetLangs"), array(targets()));
    QJsonObject mems;
    for (const QString &t : targets())
        mems.insert(t, m_memories.value(t).toArray());
    s.insert(QStringLiteral("memories"), mems);
    if (m_new)
        s.insert(QStringLiteral("projectMemory"), m_projectMemory->isChecked());

    QJsonObject t = s.value(QLatin1String("terminology")).toObject();
    t.insert(QStringLiteral("mode"), m_layered->isChecked() ? QStringLiteral("layered") : m_direct->isChecked() ? QStringLiteral("direct") : QStringLiteral("own"));
    QStringList masters;
    for (int i = 0; i < m_masters->count(); ++i) {
        if (m_masters->item(i)->checkState() == Qt::Checked && !m_own->isChecked())
            masters << m_masters->item(i)->data(Qt::UserRole).toString();
    }
    t.insert(QStringLiteral("masters"), array(masters));
    t.insert(QStringLiteral("writeTo"), m_direct->isChecked() ? m_writeTo->currentData().toString() : QString());
    t.insert(QStringLiteral("autoUpdate"), m_layered->isChecked() && m_autoUpdate->isChecked());
    t.insert(QStringLiteral("location"), QStringLiteral("server"));
    s.insert(QStringLiteral("terminology"), t);

    QJsonObject qa = s.value(QLatin1String("qa")).toObject();
    QStringList disabled, enabled;
    for (int i = 0; i < m_checks->count(); ++i) {
        const QListWidgetItem *it = m_checks->item(i);
        const bool on = it->checkState() == Qt::Checked;
        const bool byDefault = it->data(Qt::UserRole + 1).toBool();
        if (byDefault && !on)
            disabled << it->data(Qt::UserRole).toString();
        if (!byDefault && on)
            enabled << it->data(Qt::UserRole).toString();
    }
    qa.insert(QStringLiteral("disabled"), array(disabled));
    qa.insert(QStringLiteral("enabled"), array(enabled));
    qa.insert(QStringLiteral("uneditedFuzzy"), m_uneditedFuzzy->isChecked());
    QStringList words;
    for (const QString &w : m_words->toPlainText().split(QLatin1Char('\n'))) {
        if (!w.trimmed().isEmpty())
            words << w.trimmed();
    }
    qa.insert(QStringLiteral("words"), array(words));
    QJsonObject dicts;
    for (int r = 0; r < m_dictTable->rowCount(); ++r) {
        auto *c = qobject_cast<QComboBox *>(m_dictTable->cellWidget(r, 1));
        if (c && !c->currentData().toString().isEmpty())
            dicts.insert(m_dictTable->item(r, 0)->data(Qt::UserRole).toString(), c->currentData().toString());
    }
    qa.insert(QStringLiteral("dictionaries"), dicts);
    QJsonArray rules;
    for (int r = 0; r < m_rules->rowCount(); ++r) {
        const auto text = [this, r](int c) { return m_rules->item(r, c) ? m_rules->item(r, c)->text().trimmed() : QString(); };
        auto *mode = qobject_cast<QComboBox *>(m_rules->cellWidget(r, 3));
        auto *sev = qobject_cast<QComboBox *>(m_rules->cellWidget(r, 4));
        QJsonObject rule{{QStringLiteral("name"), text(0)}, {QStringLiteral("target"), text(2)},
                         {QStringLiteral("mode"), mode ? mode->currentData().toString() : QStringLiteral("must-not")},
                         {QStringLiteral("severity"), sev ? sev->currentData().toString() : QStringLiteral("warning")}};
        if (!text(1).isEmpty())
            rule.insert(QStringLiteral("source"), text(1));
        if (!text(5).isEmpty())
            rule.insert(QStringLiteral("message"), text(5));
        rules.append(rule);
    }
    qa.insert(QStringLiteral("rules"), rules);
    s.insert(QStringLiteral("qa"), qa);

    QJsonObject prep = s.value(QLatin1String("prepare")).toObject();
    prep.insert(QStringLiteral("pretranslate"), m_pretranslate->isChecked());
    prep.insert(QStringLiteral("minScore"), m_minScore->value());
    prep.insert(QStringLiteral("confirmExact"), m_confirmExact->isChecked());
    prep.insert(QStringLiteral("confirmContext"), m_confirmContext->isChecked());
    s.insert(QStringLiteral("prepare"), prep);
    QJsonObject an = s.value(QLatin1String("analysis")).toObject();
    an.insert(QStringLiteral("context"), m_context->currentData().toString());
    s.insert(QStringLiteral("analysis"), an);
    return s;
}

void SettingsEditor::setReadOnly(bool readOnly)
{
    for (int i = 0; i < count(); ++i)
        widget(i)->setEnabled(!readOnly);
}

void SettingsEditor::showProblems(const QList<Problem> &problems)
{
    QList<QStringList> byTab(m_problemLabels.size());
    for (const Problem &p : problems) {
        const QString f = p.field;
        int tab = 0;
        if (f.startsWith(QLatin1String("memories")))
            tab = 1;
        else if (f.startsWith(QLatin1String("terminology")))
            tab = 2;
        else if (f.startsWith(QLatin1String("qa")))
            tab = 3;
        else if (f.startsWith(QLatin1String("prepare")) || f.startsWith(QLatin1String("analysis")))
            tab = 4;
        byTab[tab] << p.message;
    }
    int first = -1;
    for (int i = 0; i < m_problemLabels.size(); ++i) {
        m_problemLabels[i]->setText(byTab[i].join(QLatin1Char('\n')));
        m_problemLabels[i]->setVisible(!byTab[i].isEmpty());
        if (first < 0 && !byTab[i].isEmpty())
            first = i;
    }
    if (first >= 0)
        setCurrentIndex(first);
}
