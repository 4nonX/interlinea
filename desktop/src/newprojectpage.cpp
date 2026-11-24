#include "newprojectpage.h"

#include "apiclient.h"
#include "bands.h"
#include "connectionmanager.h"
#include "icons.h"
#include "languages.h"
#include "options.h"
#include "projectform.h"
#include "theme.h"
#include "webui.h"
#include "words.h"

#include <QButtonGroup>
#include <QCheckBox>
#include <QComboBox>
#include <QDirIterator>
#include <QCoreApplication>
#include <QFileDialog>
#include <QHash>
#include <QFileInfo>
#include <QGridLayout>
#include <QHash>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QRadioButton>
#include <QRegularExpression>
#include <QTableWidget>
#include <QVBoxLayout>

namespace {

const qint64 maxFile = 200ll << 20;
// Below this the person confirms the language (langdetect.Confident).
const double confident = 0.75;

QString plural(qint64 n, const QString &one, const QString &many)
{
    return n == 1 ? one : many.arg(QLocale().toString(n));
}

QStringList strings(const QJsonValue &v)
{
    QStringList out;
    for (const QJsonValue &x : v.toArray())
        out << x.toString();
    return out;
}

void clear(QLayout *l)
{
    while (QLayoutItem *item = l->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->setParent(nullptr);
            w->deleteLater();
        } else if (item->layout()) {
            clear(item->layout());
        }
        delete item;
    }
}

QLabel *legend(const QString &text, QWidget *parent)
{
    auto *l = new QLabel(QStringLiteral("<b>%1</b>").arg(text.toHtmlEscaped()), parent);
    l->setAccessibleName(text);
    return l;
}

// .panel-actions: a line, and the step's buttons at the right.
QWidget *panelActions(const QList<QPushButton *> &buttons, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(16);
    auto *line = new QFrame(w);
    Theme::role(line, "line");
    line->setFixedHeight(1);
    v->addWidget(line);
    auto *h = new QHBoxLayout;
    h->setSpacing(8);
    h->addStretch();
    for (QPushButton *b : buttons)
        h->addWidget(b);
    v->addLayout(h);
    return w;
}

// What a document format is called and what it keeps of the file besides
// its text, as the web client says it; empty for any other format.
QString documentFormatText(const QString &format)
{
    static const QHash<QString, std::pair<const char *, const char *>> names{
        {QStringLiteral("docx"), {QT_TRANSLATE_NOOP("NewProjectPage", "DOCX document"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("pptx"), {QT_TRANSLATE_NOOP("NewProjectPage", "PPTX presentation"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("xlsx"), {QT_TRANSLATE_NOOP("NewProjectPage", "XLSX workbook"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("html"), {QT_TRANSLATE_NOOP("NewProjectPage", "Web page"), QT_TRANSLATE_NOOP("NewProjectPage", "markup kept")}},
        {QStringLiteral("idml"), {QT_TRANSLATE_NOOP("NewProjectPage", "IDML document"), QT_TRANSLATE_NOOP("NewProjectPage", "layout kept")}},
        {QStringLiteral("odt"), {QT_TRANSLATE_NOOP("NewProjectPage", "ODT document"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("ods"), {QT_TRANSLATE_NOOP("NewProjectPage", "ODS spreadsheet"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("odp"), {QT_TRANSLATE_NOOP("NewProjectPage", "ODP presentation"), QT_TRANSLATE_NOOP("NewProjectPage", "formatting kept")}},
        {QStringLiteral("xml"), {QT_TRANSLATE_NOOP("NewProjectPage", "XML file"), QT_TRANSLATE_NOOP("NewProjectPage", "markup kept")}},
        {QStringLiteral("md"), {QT_TRANSLATE_NOOP("NewProjectPage", "Markdown file"), QT_TRANSLATE_NOOP("NewProjectPage", "markup kept")}},
        {QStringLiteral("properties"), {QT_TRANSLATE_NOOP("NewProjectPage", "Properties file"), QT_TRANSLATE_NOOP("NewProjectPage", "keys and placeholders kept")}},
        {QStringLiteral("json"), {QT_TRANSLATE_NOOP("NewProjectPage", "JSON file"), QT_TRANSLATE_NOOP("NewProjectPage", "keys and placeholders kept")}},
        {QStringLiteral("yaml"), {QT_TRANSLATE_NOOP("NewProjectPage", "YAML file"), QT_TRANSLATE_NOOP("NewProjectPage", "keys and placeholders kept")}},
        {QStringLiteral("po"), {QT_TRANSLATE_NOOP("NewProjectPage", "PO file"), QT_TRANSLATE_NOOP("NewProjectPage", "keys and placeholders kept")}},
        {QStringLiteral("resx"), {QT_TRANSLATE_NOOP("NewProjectPage", "RESX file"), QT_TRANSLATE_NOOP("NewProjectPage", "keys and placeholders kept")}},
        {QStringLiteral("srt"), {QT_TRANSLATE_NOOP("NewProjectPage", "SRT subtitles"), QT_TRANSLATE_NOOP("NewProjectPage", "times kept")}},
        {QStringLiteral("vtt"), {QT_TRANSLATE_NOOP("NewProjectPage", "WebVTT subtitles"), QT_TRANSLATE_NOOP("NewProjectPage", "times kept")}},
    };
    const auto it = names.constFind(format);
    if (it == names.constEnd())
        return {};
    return QStringLiteral("%1, %2").arg(QCoreApplication::translate("NewProjectPage", it->first), QCoreApplication::translate("NewProjectPage", it->second));
}

// "Mietvertrag_Hofmann.docx" names the project "Mietvertrag Hofmann".
QString nameFromFile(QString file)
{
    file = file.section(QLatin1Char('/'), -1);
    const int dot = file.lastIndexOf(QLatin1Char('.'));
    if (dot > 0)
        file.truncate(dot);
    file.replace(QRegularExpression(QStringLiteral("_+")), QStringLiteral(" "));
    return file.trimmed();
}

QString packageKind(const QString &name)
{
    return name.endsWith(QLatin1String(".mqout"), Qt::CaseInsensitive) ? NewProjectPage::tr("handoff package") : NewProjectPage::tr("project package");
}

QString bilingualName(const QString &format)
{
    if (format == QLatin1String("sdlxliff"))
        return QStringLiteral("SDLXLIFF");
    if (format == QLatin1String("mqxliff"))
        return QStringLiteral("MQXLIFF");
    if (format == QLatin1String("xliff-2"))
        return QStringLiteral("XLIFF 2");
    if (format == QLatin1String("xliff-1.2"))
        return QStringLiteral("XLIFF 1.2");
    return QStringLiteral("XLIFF");
}

// The most common of a list, the first on a tie.
QStringList byCount(const QStringList &all)
{
    QStringList order;
    QHash<QString, int> n;
    for (const QString &x : all) {
        if (x.isEmpty())
            continue;
        if (!n.contains(x))
            order << x;
        ++n[x];
    }
    std::stable_sort(order.begin(), order.end(), [&n](const QString &a, const QString &b) { return n.value(a) > n.value(b); });
    return order;
}

} // namespace

NewProjectPage::NewProjectPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent)
    , m_manager(manager)
    , m_form(new ProjectForm(this))
{
    setObjectName(QStringLiteral("newProject"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    Web::Page page = Web::page(this, 760);
    page.scroll->setAccessibleName(tr("New project"));
    QWidget *host = page.scroll->widget();
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet, host);
    cancel->setAccessibleName(tr("Cancel the new project"));
    page.body->addWidget(Web::header(tr("New project"), QString(), {cancel}, host));
    m_stepper = new Web::Stepper(host);
    m_stepper->setClickable(true);
    page.body->addWidget(m_stepper);
    connect(cancel, &QPushButton::clicked, this, &NewProjectPage::cancelled);
    connect(m_stepper, &Web::Stepper::stepClicked, this, &NewProjectPage::setStep);

    const auto back = [this, host](int to) {
        auto *b = Web::button(tr("Back"), Web::Quiet, host);
        connect(b, &QPushButton::clicked, this, [this, to] { setStep(to); });
        return b;
    };
    const auto next = [this, host](const QString &text, int from) {
        auto *b = Web::button(text, Web::Primary, host);
        m_continue[from] = b;
        connect(b, &QPushButton::clicked, this, [this, from] { setStep(from + 1); });
        return b;
    };

    // Start.
    QVBoxLayout *body = nullptr;
    m_panels[0] = Web::panel(tr("Start"), host, &body);
    m_startBody = new QVBoxLayout;
    m_startBody->setSpacing(16);
    body->addLayout(m_startBody);
    body->addWidget(panelActions({next(tr("Continue to files and languages"), 0)}, m_panels[0]));

    // Files and languages.
    m_panels[1] = Web::panel(tr("Files and languages"), host, &body);
    m_filesBody = new QVBoxLayout;
    m_filesBody->setSpacing(8);
    body->addLayout(m_filesBody);
    m_filesError = Web::text(QString(), "error", m_panels[1]);
    m_filesError->setVisible(false);
    body->addWidget(m_filesError);
    m_sourceBody = new QVBoxLayout;
    m_sourceBody->setSpacing(8);
    body->addLayout(m_sourceBody);
    {
        auto *targets = new QWidget(m_panels[1]);
        auto *tv = new QVBoxLayout(targets);
        tv->setContentsMargins(0, 0, 0, 0);
        tv->setSpacing(8);
        tv->addWidget(legend(tr("Target languages"), targets));
        tv->addWidget(m_form->targets(targets));
        auto *each = Web::text(tr("Each file is translated into every target language; a bilingual file only into the language it was made for."), "help", targets);
        each->setObjectName(QStringLiteral("eachTarget"));
        each->setVisible(false);
        tv->addWidget(each);
        body->addWidget(targets);
    }
    body->addWidget(m_form->details(m_panels[1]));
    body->addWidget(panelActions({back(0), next(tr("Continue to memories"), 1)}, m_panels[1]));

    // Memories.
    m_panels[2] = Web::panel(tr("Memories"), host, &body);
    body->addWidget(m_form->memories(m_panels[2]));
    body->addWidget(panelActions({back(1), next(tr("Continue to terminology"), 2)}, m_panels[2]));

    // Terminology.
    m_panels[3] = Web::panel(tr("Terminology"), host, &body);
    body->addWidget(m_form->terminology(m_panels[3]));
    body->addWidget(panelActions({back(2), next(tr("Continue to review"), 3)}, m_panels[3]));

    // Review.
    m_panels[4] = Web::panel(tr("Review"), host, &body);
    m_reviewBody = new QVBoxLayout;
    m_reviewBody->setSpacing(16);
    body->addLayout(m_reviewBody);
    m_create = Web::button(tr("Create project"), Web::Primary, host);
    connect(m_create, &QPushButton::clicked, this, &NewProjectPage::create);
    body->addWidget(panelActions({back(3), m_create}, m_panels[4]));

    for (QWidget *p : m_panels)
        page.body->addWidget(p);
    page.body->addStretch();
    outer->addWidget(page.scroll);
    m_toast = new Web::Toast(this);

    m_previewTimer.setSingleShot(true);
    m_previewTimer.setInterval(350);
    connect(&m_previewTimer, &QTimer::timeout, this, &NewProjectPage::preview);
    connect(m_form, &ProjectForm::changed, this, [this] {
        // A name the person typed stays theirs.
        const QJsonObject s = m_form->settings();
        if (s.value(QLatin1String("name")).toString() != m_autoName)
            m_nameTouched = true;
        if (auto *each = findChild<QLabel *>(QStringLiteral("eachTarget")))
            each->setVisible(s.value(QLatin1String("targetLangs")).toArray().size() > 1);
        refreshContinue();
        if (m_step == 4) {
            fillReview();
            m_previewTimer.start();
        }
    });
    // The core may still be starting (Interlinea opened with a document).
    connect(manager, &ConnectionManager::changed, this, [this] {
        QString ids;
        for (Connection *x : m_manager->all())
            ids += x->id() + QLatin1Char(x->isReady() ? '1' : '0');
        if (ids == m_places)
            return;
        m_places = ids;
        if (!connection() || !connection()->isReady())
            reloadWhere();
        else
            fillStart();
    });
    connect(manager, &ConnectionManager::removed, this, &NewProjectPage::reloadWhere);
    reloadWhere();
    showStep();
}

void NewProjectPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

Connection *NewProjectPage::connection() const
{
    return m_manager->find(m_connectionId);
}

void NewProjectPage::setConnection(const QString &connectionId)
{
    Connection *c = m_manager->find(connectionId);
    if (!c || !c->isReady() || connectionId == m_connectionId)
        return;
    m_connectionId = connectionId;
    reloadWhere();
}

// Where the project is kept decides what it can use: the memories,
// termbases, templates and earlier projects there. The files are read
// there, too.
void NewProjectPage::reloadWhere()
{
    Connection *c = connection();
    if (!c || !c->isReady()) {
        c = nullptr;
        for (Connection *x : m_manager->all()) {
            if (x->isReady()) {
                c = x;
                break;
            }
        }
        m_connectionId = c ? c->id() : QString();
    }
    m_defaults = {};
    m_templates = {};
    m_projects = {};
    if (!c) {
        fillStart();
        refreshContinue();
        return;
    }
    m_loading = true;
    fillStart();
    refreshContinue();
    const QString id = c->id();
    auto pending = std::make_shared<int>(4);
    auto error = std::make_shared<QString>();
    const auto done = [this, id, pending, error] {
        if (--*pending > 0 || id != m_connectionId)
            return;
        m_loading = false;
        if (!error->isEmpty())
            m_toast->say(*error);
        applyStart();
        for (int i = 0; i < m_docs.size(); ++i)
            inspect(i);
        fillStart();
        filesChanged();
    };
    onResult(c->api()->get(QStringLiteral("/api/projects/defaults")), this, [this, error, done](const ApiResult &r) {
        if (r.ok())
            m_defaults = r.object();
        else
            *error = r.error.message;
        done();
    });
    onResult(c->api()->get(QStringLiteral("/api/templates")), this, [this, done](const ApiResult &r) {
        m_templates = r.ok() ? r.array() : QJsonArray();
        done();
    });
    onResult(c->api()->get(QStringLiteral("/api/projects")), this, [this, done](const ApiResult &r) {
        m_projects = r.ok() ? r.array() : QJsonArray();
        done();
    });
    m_form->load(c, [error, done](const QString &e) {
        if (!e.isEmpty())
            *error = e;
        done();
    });
}

// The settings the project starts with: the defaults, or those of a
// template or an earlier project, with what was typed here kept.
void NewProjectPage::applyStart()
{
    const QJsonObject had = m_form->settings();
    QJsonObject s = m_defaults;
    QJsonObject an = s.value(QLatin1String("analysis")).toObject();
    an.insert(QStringLiteral("context"), Options::contextRule());
    s.insert(QStringLiteral("analysis"), an);
    QJsonObject from;
    if (m_startKind == QLatin1String("template")) {
        for (const QJsonValue &v : std::as_const(m_templates)) {
            if (v.toObject().value(QLatin1String("name")).toString() == m_startId)
                from = v.toObject();
        }
    } else if (m_startKind == QLatin1String("project")) {
        for (const QJsonValue &v : std::as_const(m_projects)) {
            if (QString::number(v.toObject().value(QLatin1String("id")).toInteger()) != m_startId)
                continue;
            from = v.toObject().value(QLatin1String("settings")).toObject();
            // Its memories, but not its own project memories, and a
            // project termbase of its own.
            const QString pname = from.value(QLatin1String("name")).toString();
            QJsonObject mems = from.value(QLatin1String("memories")).toObject();
            for (const QString &lang : mems.keys()) {
                QJsonArray kept;
                for (const QJsonValue &m : mems.value(lang).toArray()) {
                    if (m.toObject().value(QLatin1String("memory")).toString() != QStringLiteral("%1 (%2)").arg(pname, lang))
                        kept.append(m);
                }
                mems.insert(lang, kept);
            }
            from.insert(QStringLiteral("memories"), mems);
        }
    }
    if (!from.isEmpty()) {
        for (const char *key : {"client", "sourceLang", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare"}) {
            if (from.contains(QLatin1String(key)))
                s.insert(QLatin1String(key), from.value(QLatin1String(key)));
        }
        QJsonObject term = s.value(QLatin1String("terminology")).toObject();
        term.remove(QStringLiteral("termbase"));
        term.remove(QStringLiteral("addLanguages"));
        s.insert(QStringLiteral("terminology"), term);
        m_sourceTouched = true;
    } else {
        // A blank project keeps the languages chosen so far.
        s.insert(QStringLiteral("sourceLang"), had.value(QLatin1String("sourceLang")));
        if (had.contains(QLatin1String("targetLangs")))
            s.insert(QStringLiteral("targetLangs"), had.value(QLatin1String("targetLangs")));
        if (!had.value(QLatin1String("client")).toString().isEmpty())
            s.insert(QStringLiteral("client"), had.value(QLatin1String("client")));
    }
    s.insert(QStringLiteral("name"), had.value(QLatin1String("name")));
    s.insert(QStringLiteral("due"), had.value(QLatin1String("due")).toString());
    s.insert(QStringLiteral("description"), had.value(QLatin1String("description")).toString());
    QJsonObject term = s.value(QLatin1String("terminology")).toObject();
    term.insert(QStringLiteral("location"), QStringLiteral("server"));
    s.insert(QStringLiteral("terminology"), term);
    s.insert(QStringLiteral("files"), QJsonArray());
    m_form->setSettings(s, true);
    // Target languages of earlier projects are offered first.
    QStringList recent;
    for (const QJsonValue &v : std::as_const(m_projects))
        recent << strings(v.toObject().value(QLatin1String("settings")).toObject().value(QLatin1String("targetLangs")));
    m_form->setSuggestedTargets(byCount(recent));
    filesChanged();
}

void NewProjectPage::setStep(int step)
{
    if (step < 0 || step > 4 || step == m_step)
        return;
    m_step = step;
    showStep();
    if (m_step == 4) {
        fillReview();
        preview();
    }
}

void NewProjectPage::showStep()
{
    m_stepper->setSteps({tr("Start"), tr("Files and languages"), tr("Memories"), tr("Terminology"), tr("Review")}, m_step);
    for (int i = 0; i < 5; ++i)
        m_panels[i]->setVisible(i == m_step);
    refreshContinue();
    // The keyboard starts at the step's first control.
    if (QWidget *first = m_panels[m_step]->nextInFocusChain())
        first->setFocus();
}

void NewProjectPage::fillStart()
{
    clear(m_startBody);
    QWidget *p = m_panels[0];
    Connection *c = connection();
    if (!c) {
        m_startBody->addWidget(Web::text(m_manager->local()->state() == Connection::State::Connecting
                                             ? tr("Starting the work on this computer…")
                                             : tr("Nowhere to keep a project: work on this computer can't be opened and no team server is signed in."),
                                         m_manager->local()->state() == Connection::State::Connecting ? "muted" : "error", p));
        return;
    }
    auto *from = new QWidget(p);
    auto *fv = new QVBoxLayout(from);
    fv->setContentsMargins(0, 0, 0, 0);
    fv->setSpacing(8);
    fv->addWidget(legend(tr("Start from"), from));
    const auto choose = [this](const QString &kind, const QString &id) {
        if (m_startKind == kind && m_startId == id)
            return;
        m_startKind = kind;
        m_startId = id;
        applyStart();
        fillStart();
    };
    // Each choice is framed on its own, so one group keeps them exclusive.
    auto *starts = new QButtonGroup(from);
    auto *blank = new QRadioButton(tr("A blank project"), from);
    starts->addButton(blank);
    blank->setChecked(m_startKind == QLatin1String("blank"));
    fv->addWidget(Web::choice(blank, tr("Languages from the files, memories and termbases suggested for the client."), from));
    connect(blank, &QRadioButton::toggled, this, [choose](bool on) {
        if (on)
            choose(QStringLiteral("blank"), QString());
    });
    for (const QJsonValue &v : std::as_const(m_templates)) {
        const QJsonObject t = v.toObject();
        const QString name = t.value(QLatin1String("name")).toString();
        auto *radio = new QRadioButton(tr("Template: %1").arg(name), from);
        starts->addButton(radio);
        radio->setChecked(m_startKind == QLatin1String("template") && m_startId == name);
        int memories = 0;
        for (const QJsonValue &ms : t.value(QLatin1String("memories")).toObject())
            memories += int(ms.toArray().size());
        QStringList parts;
        if (!t.value(QLatin1String("client")).toString().isEmpty())
            parts << t.value(QLatin1String("client")).toString();
        parts << QStringLiteral("%1 → %2").arg(t.value(QLatin1String("sourceLang")).toString(), strings(t.value(QLatin1String("targetLangs"))).join(QLatin1Char(' ')));
        parts << plural(memories, tr("1 memory"), tr("%1 memories"));
        fv->addWidget(Web::choice(radio, parts.join(QStringLiteral(", ")), from));
        connect(radio, &QRadioButton::toggled, this, [choose, name](bool on) {
            if (on)
                choose(QStringLiteral("template"), name);
        });
    }
    if (!m_projects.isEmpty()) {
        auto *radio = new QRadioButton(tr("Like an earlier project"), from);
        starts->addButton(radio);
        const bool chosen = m_startKind == QLatin1String("project");
        radio->setChecked(chosen);
        QFrame *frame = Web::choice(radio, tr("Its languages, memories, termbases and checks; new files and a new name."), from);
        const QString first = QString::number(m_projects.at(0).toObject().value(QLatin1String("id")).toInteger());
        if (chosen) {
            auto *pick = new QComboBox(frame);
            pick->setAccessibleName(tr("Project"));
            for (const QJsonValue &v : std::as_const(m_projects)) {
                const QJsonObject s = v.toObject().value(QLatin1String("settings")).toObject();
                const QString client = s.value(QLatin1String("client")).toString();
                pick->addItem(client.isEmpty() ? s.value(QLatin1String("name")).toString() : QStringLiteral("%1 (%2)").arg(s.value(QLatin1String("name")).toString(), client),
                              QString::number(v.toObject().value(QLatin1String("id")).toInteger()));
            }
            pick->setCurrentIndex(qMax(0, pick->findData(m_startId)));
            auto *label = new QLabel(tr("Project"), frame);
            label->setBuddy(pick);
            auto *row = Web::row({label, pick}, frame);
            row->setContentsMargins(26, 4, 0, 0);
            frame->layout()->addWidget(row);
            connect(pick, &QComboBox::currentIndexChanged, this, [choose, pick] { choose(QStringLiteral("project"), pick->currentData().toString()); });
        }
        fv->addWidget(frame);
        connect(radio, &QRadioButton::toggled, this, [choose, first](bool on) {
            if (on)
                choose(QStringLiteral("project"), first);
        });
    }
    if (m_templates.isEmpty())
        fv->addWidget(Web::text(tr("You can save any project's settings as a template, here at the end or later from the project's settings."), "help", from));
    m_startBody->addWidget(from);

    // Where it is kept, when there is more than one place.
    QList<Connection *> ready;
    for (Connection *x : m_manager->all()) {
        if (x->isReady())
            ready << x;
    }
    if (ready.size() > 1) {
        auto *where = new QWidget(p);
        auto *wv = new QVBoxLayout(where);
        wv->setContentsMargins(0, 0, 0, 0);
        wv->setSpacing(8);
        wv->addWidget(legend(tr("Where the project is kept"), where));
        auto *places = new QButtonGroup(where);
        for (Connection *x : std::as_const(ready)) {
            auto *radio = new QRadioButton(x->isLocal() ? tr("On this computer") : tr("On the team server %1").arg(x->displayName()), where);
            places->addButton(radio);
            radio->setChecked(x->id() == m_connectionId);
            wv->addWidget(Web::choice(radio,
                                      x->isLocal() ? tr("Only you work on it, with the memories and termbases kept on this computer.")
                                                   : tr("Shared: you give work to translators and reviewers there, and everyone's translations are saved on the server. It uses the server's memories and termbases."),
                                      where));
            const QString id = x->id();
            connect(radio, &QRadioButton::toggled, this, [this, id](bool on) {
                if (!on || id == m_connectionId)
                    return;
                m_connectionId = id;
                // Templates and earlier projects are those of the place.
                m_startKind = QStringLiteral("blank");
                m_startId.clear();
                reloadWhere();
            });
        }
        m_startBody->addWidget(where);
    }
}

bool NewProjectPage::canTranslate(const Doc &d) const
{
    return d.inspection.value(QLatin1String("translatable")).toBool();
}

QStringList NewProjectPage::bilingualSources() const
{
    QStringList out;
    for (const Doc &d : m_docs) {
        if (d.role == QLatin1String("reference"))
            continue;
        QString src = d.inspection.value(QLatin1String("bilingual")).toObject().value(QLatin1String("sourceLang")).toString();
        if (src.isEmpty())
            src = d.inspection.value(QLatin1String("package")).toObject().value(QLatin1String("sourceLang")).toString();
        src = Languages::normalize(src);
        if (!src.isEmpty() && !out.contains(src))
            out << src;
    }
    return out;
}

QStringList NewProjectPage::fixedTargets() const
{
    QStringList out;
    for (const Doc &d : m_docs) {
        if (d.role == QLatin1String("reference"))
            continue;
        QStringList ts{d.inspection.value(QLatin1String("bilingual")).toObject().value(QLatin1String("targetLang")).toString()};
        ts += strings(d.inspection.value(QLatin1String("package")).toObject().value(QLatin1String("targetLangs")));
        for (const QString &t : std::as_const(ts)) {
            const QString n = Languages::normalize(t);
            if (!n.isEmpty() && !out.contains(n))
                out << n;
        }
    }
    return out;
}

void NewProjectPage::chooseFiles()
{
    addFiles(QFileDialog::getOpenFileNames(this, tr("Files for the project")));
}

void NewProjectPage::chooseFolder()
{
    const QString dir = QFileDialog::getExistingDirectory(this, tr("A folder of files"));
    if (!dir.isEmpty())
        addFiles({dir});
}

void NewProjectPage::addFiles(const QStringList &paths)
{
    // A folder brings its files, which keep the folder in their names.
    QList<QPair<QString, QString>> files;
    for (const QString &path : paths) {
        const QFileInfo info(path);
        if (info.isDir()) {
            const QDir base(info.absolutePath());
            QStringList inside;
            QDirIterator it(path, QDir::Files, QDirIterator::Subdirectories);
            while (it.hasNext())
                inside << it.next();
            inside.sort();
            for (const QString &f : std::as_const(inside))
                files.append({f, base.relativeFilePath(f)});
        } else if (info.exists()) {
            files.append({path, info.fileName()});
        }
    }
    for (const auto &[path, name] : std::as_const(files)) {
        bool dup = false;
        for (const Doc &x : std::as_const(m_docs))
            dup = dup || x.name.compare(name, Qt::CaseInsensitive) == 0;
        if (dup)
            continue;
        Doc d{path, name, {}, {}, QStringLiteral("reference"), false};
        QFile f(path);
        if (f.size() > maxFile) {
            d.inspection = QJsonObject{{QStringLiteral("problem"), tr("The file is larger than 200 MB.")}};
            d.read = true;
        } else if (!f.open(QIODevice::ReadOnly)) {
            d.inspection = QJsonObject{{QStringLiteral("problem"), tr("The file can't be read: %1").arg(f.errorString())}};
            d.read = true;
        } else {
            d.data = f.readAll();
        }
        m_docs.append(d);
        if (!d.read)
            inspect(int(m_docs.size()) - 1);
    }
    filesChanged();
}

void NewProjectPage::inspect(int index)
{
    Connection *c = connection();
    if (!c || index >= m_docs.size() || m_docs[index].data.isEmpty())
        return;
    m_docs[index].read = false;
    const QString path = m_docs[index].path;
    const QString where = m_connectionId;
    onResult(c->api()->upload("POST", QStringLiteral("/api/files/inspect"), m_docs[index].data, QStringLiteral("application/octet-stream"),
                              QUrlQuery{{QStringLiteral("name"), m_docs[index].name}}),
             this, [this, path, where](const ApiResult &r) {
                 if (where != m_connectionId)
                     return;
                 for (Doc &d : m_docs) {
                     if (d.path != path)
                         continue;
                     d.inspection = r.ok() ? r.object() : QJsonObject{{QStringLiteral("problem"), r.error.message}};
                     d.read = true;
                     const QString format = d.inspection.value(QLatin1String("format")).toString();
                     d.role = !d.inspection.value(QLatin1String("translatable")).toBool() ? QStringLiteral("reference")
                         : format == QLatin1String("package")                            ? QStringLiteral("package")
                                                                                         : QStringLiteral("translatable");
                 }
                 filesChanged();
             });
}

// What follows the files until the person sets it: the project's name,
// its source language and the target languages bilingual files need.
void NewProjectPage::filesChanged()
{
    if (!m_nameTouched) {
        QString name;
        if (!m_docs.isEmpty()) {
            const Doc &first = m_docs.first();
            const QString pkg = first.inspection.value(QLatin1String("package")).toObject().value(QLatin1String("name")).toString();
            name = nameFromFile(pkg.isEmpty() ? first.name : pkg);
        }
        m_autoName = name;
        m_form->set(QStringLiteral("name"), name);
    }
    const QStringList said = bilingualSources();
    QJsonObject best;
    for (const Doc &d : std::as_const(m_docs)) {
        if (d.role != QLatin1String("translatable"))
            continue;
        const QJsonObject det = d.inspection.value(QLatin1String("detection")).toObject();
        if (det.value(QLatin1String("confidence")).toDouble() > best.value(QLatin1String("confidence")).toDouble() || best.isEmpty())
            best = det.isEmpty() ? best : det;
    }
    QStringList sources;
    for (const QJsonValue &v : std::as_const(m_projects))
        sources << v.toObject().value(QLatin1String("settings")).toObject().value(QLatin1String("sourceLang")).toString();
    const QString fallback = byCount(sources).value(0);
    if (said.size() == 1) {
        m_form->set(QStringLiteral("sourceLang"), said.first());
    } else if (!m_sourceTouched) {
        const QString guess = best.value(QLatin1String("lang")).toString();
        if (!guess.isEmpty() || !fallback.isEmpty())
            m_form->set(QStringLiteral("sourceLang"), guess.isEmpty() ? fallback : guess);
    }
    const QString src = m_form->settings().value(QLatin1String("sourceLang")).toString();
    QStringList targets = strings(m_form->settings().value(QLatin1String("targetLangs")));
    targets.removeAll(src);
    const QStringList fixed = fixedTargets();
    for (const QString &t : fixed) {
        if (!targets.contains(t))
            targets << t;
    }
    m_form->setFixedTargets(fixed);
    m_form->set(QStringLiteral("targetLangs"), QJsonArray::fromStringList(targets));
    fillFiles();
    fillSource();
    refreshContinue();
    if (m_step == 4) {
        fillReview();
        m_previewTimer.start();
    }
}

void NewProjectPage::fillFiles()
{
    clear(m_filesBody);
    QWidget *p = m_panels[1];
    m_filesBody->addWidget(Web::text(tr("Files"), "fieldLabel", p));
    if (!m_docs.isEmpty()) {
        auto *table = new Web::Table({tr("File"), tr("Words"), tr("Use"), QString()}, p);
        table->setAccessibleName(tr("Files"));
        table->setNumeric({1});
        table->setStretch(0);
        for (int i = 0; i < m_docs.size(); ++i) {
            const Doc &d = m_docs.at(i);
            const QJsonObject in = d.inspection;
            const QString format = in.value(QLatin1String("format")).toString();
            QString sub;
            if (!d.read) {
                sub = tr("Reading…");
            } else if (const QJsonObject pkg = in.value(QLatin1String("package")).toObject(); !pkg.isEmpty()) {
                QStringList ts;
                for (const QString &t : strings(pkg.value(QLatin1String("targetLangs"))))
                    ts << Languages::name(Languages::normalize(t));
                sub = tr("%1, %2 to %3, %4")
                          .arg(packageKind(d.name), Languages::name(Languages::normalize(pkg.value(QLatin1String("sourceLang")).toString())), QLocale().createSeparatedList(ts),
                               plural(pkg.value(QLatin1String("files")).toArray().size(), tr("1 file"), tr("%1 files")));
            } else if (const QJsonObject b = in.value(QLatin1String("bilingual")).toObject(); !b.isEmpty()) {
                sub = bilingualName(b.value(QLatin1String("format")).toString());
                if (!b.value(QLatin1String("targetLang")).toString().isEmpty())
                    sub += tr(", %1 to %2").arg(Languages::name(Languages::normalize(b.value(QLatin1String("sourceLang")).toString())),
                                                Languages::name(Languages::normalize(b.value(QLatin1String("targetLang")).toString())));
                sub += tr(", %1 of %2 translated").arg(QLocale().toString(b.value(QLatin1String("translated")).toInt()),
                                                       plural(in.value(QLatin1String("segments")).toInt(), tr("1 segment"), tr("%1 segments")));
            } else if (const QString doc = documentFormatText(format); !doc.isEmpty()) {
                sub = doc;
            } else if (format == QLatin1String("txt")) {
                sub = tr("Read as plain text");
            } else {
                sub = tr("No text Interlinea can read");
            }
            auto *cell = Web::cell(d.name, sub, table);
            cell->setToolTip(QDir::toNativeSeparators(d.path));
            for (const QString &warning : strings(in.value(QLatin1String("warnings"))))
                cell->layout()->addWidget(Web::note(warning, QStringLiteral("warn"), cell));
            if (d.read && !canTranslate(d)) {
                const QString problem = in.value(QLatin1String("problem")).toString();
                const QString why = !problem.isEmpty() ? tr("It could not be read: %1").arg(problem)
                    : !in.value(QLatin1String("bilingual")).toObject().isEmpty()
                        ? tr("The file names no target language, so it can't be translated here.")
                        : tr("Interlinea can't read text from this file (for example a scan, or a format it has no filter for). It can travel with the project for reference.");
                cell->layout()->addWidget(Web::note(tr("%1 It is added as a reference file.").arg(why), QStringLiteral("warn"), cell));
            }
            auto *use = new QComboBox(table);
            use->setAccessibleName(tr("Use of %1").arg(d.name));
            use->addItem(tr("Translate"), d.role == QLatin1String("package") ? QStringLiteral("package") : QStringLiteral("translatable"));
            use->addItem(tr("Reference"), QStringLiteral("reference"));
            use->setCurrentIndex(d.role == QLatin1String("reference") ? 1 : 0);
            use->setEnabled(d.read && canTranslate(d));
            const QString path = d.path;
            connect(use, &QComboBox::currentIndexChanged, this, [this, use, path] {
                for (Doc &x : m_docs) {
                    if (x.path != path)
                        continue;
                    if (use->currentIndex() == 1)
                        x.role = QStringLiteral("reference");
                    else
                        x.role = x.inspection.value(QLatin1String("format")).toString() == QLatin1String("package") ? QStringLiteral("package") : QStringLiteral("translatable");
                }
                filesChanged();
            });
            auto *remove = new QPushButton(table);
            remove->setProperty("iconButton", true);
            remove->setIcon(Icons::get(QStringLiteral("x"), Theme::color("text2")));
            remove->setIconSize(QSize(14, 14));
            remove->setAccessibleName(tr("Remove %1").arg(d.name));
            remove->setToolTip(tr("Remove %1").arg(d.name));
            connect(remove, &QPushButton::clicked, this, [this, path] {
                for (int j = 0; j < m_docs.size(); ++j) {
                    if (m_docs[j].path == path) {
                        m_docs.removeAt(j);
                        break;
                    }
                }
                filesChanged();
            });
            const int words = in.value(QLatin1String("words")).toInt();
            table->addRow({cell, Web::cellText(words ? QLocale().toString(words) : QStringLiteral("–"), table, true), use, remove});
        }
        table->fit();
        m_filesBody->addWidget(table);
    }
    auto *zone = new Web::DropZone(p);
    zone->setAccessibleName(tr("Add files"));
    auto *zh = new QHBoxLayout(zone);
    zh->setContentsMargins(16, 12, 16, 12);
    zh->setSpacing(12);
    auto *icon = new QLabel(zone);
    icon->setPixmap(Icons::get(QStringLiteral("upload"), Theme::color("text2")).pixmap(18, 18));
    zh->addWidget(icon, 0, Qt::AlignVCenter);
    zh->addWidget(Web::text(tr("Drop files or folders here. DOCX, PPTX, XLSX, IDML and web page files keep their formatting; bilingual files (.sdlxliff, .mqxliff, .mqxlz, .xlf), project packages (.sdlppx) and handoff packages (.mqout) keep their translations and states."),
                            "secondary", zone),
                  1);
    auto *addFilesButton = Web::button(tr("Add files"), Web::Small, zone, QStringLiteral("file-text"));
    auto *addFolder = Web::button(tr("Add a folder"), Web::Small, zone, QStringLiteral("folder-open"));
    zh->addWidget(addFilesButton, 0, Qt::AlignVCenter);
    zh->addWidget(addFolder, 0, Qt::AlignVCenter);
    connect(zone, &Web::DropZone::dropped, this, &NewProjectPage::addFiles);
    connect(addFilesButton, &QPushButton::clicked, this, &NewProjectPage::chooseFiles);
    connect(addFolder, &QPushButton::clicked, this, &NewProjectPage::chooseFolder);
    m_filesBody->addWidget(zone);
    m_filesBody->addWidget(Web::text(tr("Reference files (a style guide, an earlier version) travel with the project for the translators to read; they are not translated."), "help", p));
}

void NewProjectPage::fillSource()
{
    clear(m_sourceBody);
    QWidget *p = m_panels[1];
    const QString src = m_form->settings().value(QLatin1String("sourceLang")).toString();
    const QStringList said = bilingualSources();
    if (said.size() == 1) {
        m_sourceBody->addWidget(legend(tr("Source language"), p));
        auto *name = new QLabel(Languages::name(src), p);
        m_sourceBody->addWidget(Web::row({name, Web::langChip(src, p)}, p));
        m_sourceBody->addWidget(Web::text(tr("From the bilingual files, which are made for it."), "help", p));
        return;
    }
    // What the text of the files says, the surest of them.
    QJsonObject det;
    bool any = false;
    for (const Doc &d : std::as_const(m_docs)) {
        if (d.role != QLatin1String("translatable"))
            continue;
        any = true;
        const QJsonObject x = d.inspection.value(QLatin1String("detection")).toObject();
        if (det.isEmpty() || x.value(QLatin1String("confidence")).toDouble() > det.value(QLatin1String("confidence")).toDouble())
            det = x.isEmpty() ? det : x;
    }
    auto *combo = new LanguageCombo(p);
    combo->setAccessibleName(tr("Source language"));
    combo->setLanguage(src);
    auto *label = legend(tr("Source language"), p);
    label->setBuddy(combo);
    m_sourceBody->addWidget(label);
    if (any) {
        const QString code = det.value(QLatin1String("lang")).toString();
        const double conf = det.value(QLatin1String("confidence")).toDouble();
        const int pct = qRound(conf * 100);
        const QString reason = det.value(QLatin1String("reason")).toString();
        const int basis = det.value(QLatin1String("basis")).toInt();
        QString text, kind = QStringLiteral("warn");
        if (m_sourceTouched || m_startKind != QLatin1String("blank")) {
            const bool differs = !code.isEmpty() && conf >= confident && !Languages::samePrimary(code, src);
            text = tr("Set to %1.").arg(Languages::name(src));
            if (differs)
                text += QLatin1Char(' ') + tr("The files look %1 (%2%). Check the source language.").arg(Languages::name(code)).arg(pct);
            else
                kind = QStringLiteral("info");
        } else if (reason == QLatin1String("empty")) {
            text = src.isEmpty() ? tr("No text found in the files (for example a scanned PDF), so the language can't be detected. Choose the source language.")
                                 : tr("No text found in the files (for example a scanned PDF), so the language can't be detected. Preselected %1, the source language of most of your projects. Change it if it's wrong.")
                                       .arg(Languages::name(src));
        } else if (reason == QLatin1String("short") || code.isEmpty()) {
            const QString little = tr("Too little text to tell (%1).").arg(plural(basis, tr("1 word"), tr("%1 words")));
            text = src.isEmpty() ? little + QLatin1Char(' ') + tr("Choose the source language.")
                                 : little + QLatin1Char(' ') + tr("Preselected %1, the source language of most of your projects.").arg(Languages::name(src));
        } else if (conf < confident) {
            QStringList alt;
            for (const QJsonValue &a : det.value(QLatin1String("alternatives")).toArray())
                alt << QStringLiteral("%1 %2%").arg(Languages::name(a.toObject().value(QLatin1String("lang")).toString())).arg(qRound(a.toObject().value(QLatin1String("confidence")).toDouble() * 100));
            text = alt.isEmpty() ? tr("Probably %1 (%2%). Please check.").arg(Languages::name(code)).arg(pct)
                                 : tr("Probably %1 (%2%), but it could also be %3. Please check.").arg(Languages::name(code)).arg(pct).arg(alt.join(QStringLiteral(", ")));
        } else {
            kind = QStringLiteral("ok");
            text = tr("Detected %1, %2% confident, from %3 of the files.")
                       .arg(Languages::name(code))
                       .arg(pct)
                       .arg(det.value(QLatin1String("unit")).toString() == QLatin1String("characters") ? plural(basis, tr("1 character"), tr("%1 characters"))
                                                                                                       : plural(basis, tr("1 word"), tr("%1 words")));
        }
        auto *note = Web::note(text, kind, p);
        note->setObjectName(QStringLiteral("sourceDetection"));
        note->setAccessibleName(text);
        m_sourceBody->addWidget(note);
    }
    m_sourceBody->addWidget(combo);
    m_sourceBody->addWidget(Web::text(tr("Text shows the language, not the region. Check the variant (for example German (Austria)) if the client needs it."), "help", p));
    auto *problem = Web::text(m_form->problemsAt(QStringLiteral("sourceLang")).join(QLatin1Char('\n')), "error", p);
    problem->setVisible(!problem->text().isEmpty());
    m_sourceBody->addWidget(problem);
    const auto chosen = [this, combo] {
        const QString l = combo->language();
        if (l.isEmpty() || l == m_form->settings().value(QLatin1String("sourceLang")).toString())
            return;
        m_sourceTouched = true;
        m_form->set(QStringLiteral("sourceLang"), l);
        filesChanged();
    };
    connect(combo, &QComboBox::activated, this, chosen);
    connect(combo->lineEdit(), &QLineEdit::editingFinished, this, chosen);
}

void NewProjectPage::refreshContinue()
{
    const QJsonObject s = m_form->settings();
    const bool where = connection() && !m_loading;
    bool reading = false, any = false;
    QStringList packages;
    for (const Doc &d : std::as_const(m_docs)) {
        reading = reading || !d.read;
        any = any || d.role != QLatin1String("reference");
        if (d.role == QLatin1String("package"))
            packages << d.name;
    }
    QStringList problems;
    const QStringList sources = bilingualSources();
    if (sources.size() > 1) {
        QStringList names;
        for (const QString &l : sources)
            names << Languages::name(l);
        problems << tr("The bilingual files have different source languages (%1). A project has one source language.").arg(names.join(QStringLiteral(", ")));
    }
    if (packages.size() > 1)
        problems << tr("A project holds the files of one package, so it can be saved back as one return package.");
    problems << m_form->problemsAt(QStringLiteral("files"));
    if (m_filesError) {
        m_filesError->setText(problems.join(QLatin1Char('\n')));
        m_filesError->setVisible(!problems.isEmpty());
    }
    if (m_continue[0])
        m_continue[0]->setEnabled(where);
    const bool files = where && any && !reading && problems.isEmpty() && !s.value(QLatin1String("sourceLang")).toString().isEmpty() &&
                       !s.value(QLatin1String("targetLangs")).toArray().isEmpty() && !s.value(QLatin1String("name")).toString().trimmed().isEmpty();
    if (m_continue[1]) {
        m_continue[1]->setEnabled(files);
        m_continue[1]->setToolTip(files ? QString()
                                      : !any             ? tr("Add a file to translate first.")
                                      : reading          ? tr("The files are still being read.")
                                                         : tr("Give the project a name, its source language and a target language first."));
    }
    for (int i : {2, 3}) {
        if (m_continue[i])
            m_continue[i]->setEnabled(files);
    }
}

QJsonObject NewProjectPage::request() const
{
    QJsonObject s = m_form->settings();
    const QString name = s.value(QLatin1String("name")).toString();
    const QString source = s.value(QLatin1String("sourceLang")).toString();
    const QStringList targets = strings(s.value(QLatin1String("targetLangs")));
    QJsonObject req;
    // The project's own memories come first for each language, so they
    // win ties.
    QJsonArray newMemories = m_form->newMemories();
    if (s.value(QLatin1String("projectMemory")).toBool()) {
        QJsonObject mems = s.value(QLatin1String("memories")).toObject();
        for (const QString &t : targets) {
            const QString mname = QStringLiteral("%1 (%2)").arg(name, t);
            newMemories.append(QJsonObject{{QStringLiteral("Name"), mname}, {QStringLiteral("SourceLang"), source}, {QStringLiteral("TargetLang"), t}});
            QJsonArray list = mems.value(t).toArray();
            list.prepend(QJsonObject{{QStringLiteral("memory"), mname}, {QStringLiteral("lookup"), true}, {QStringLiteral("concordance"), true},
                                     {QStringLiteral("update"), true}, {QStringLiteral("penalty"), 0}});
            mems.insert(t, list);
        }
        s.insert(QStringLiteral("memories"), mems);
    }
    req.insert(QStringLiteral("memories"), newMemories);
    if (s.value(QLatin1String("terminology")).toObject().value(QLatin1String("mode")).toString() != QLatin1String("direct"))
        req.insert(QStringLiteral("termbase"), QJsonObject{{QStringLiteral("name"), QStringLiteral("%1 (project)").arg(name)}});
    QJsonArray files;
    for (const Doc &d : m_docs) {
        if (d.data.isEmpty())
            continue;
        QJsonObject f{{QStringLiteral("name"), d.name}, {QStringLiteral("role"), d.role}, {QStringLiteral("data"), QString::fromLatin1(d.data.toBase64())}};
        if (d.role == QLatin1String("reference"))
            f.insert(QStringLiteral("format"), QFileInfo(d.name).suffix().toLower());
        files.append(f);
    }
    req.insert(QStringLiteral("files"), files);
    s.insert(QStringLiteral("files"), QJsonArray());
    req.insert(QStringLiteral("settings"), s);
    return req;
}

// Problems of the page's own, besides those the core finds in the setup.
QStringList NewProjectPage::problems() const
{
    QStringList out;
    if (m_asTemplate) {
        if (m_templateName.trimmed().isEmpty()) {
            out << tr("Give the template a name.");
        } else {
            for (const QJsonValue &v : m_templates) {
                if (v.toObject().value(QLatin1String("name")).toString().compare(m_templateName.trimmed(), Qt::CaseInsensitive) == 0)
                    out << tr("A template with this name exists already.");
            }
        }
    }
    return out;
}

// Asks the core what the project would be: the setup's problems, the
// analysis of the files, and what pre-translation would fill.
void NewProjectPage::preview()
{
    Connection *c = connection();
    if (!c || m_step != 4 || m_creating)
        return;
    m_previewing = true;
    const int seq = ++m_previewSeq;
    fillAnalysis();
    m_create->setEnabled(false);
    onResult(c->api()->post(QStringLiteral("/api/projects/preview"), request()), this, [this, seq](const ApiResult &r) {
        if (seq != m_previewSeq)
            return;
        m_previewing = false;
        m_previewError.clear();
        m_createError.clear();
        QList<Problem> ps;
        if (r.ok()) {
            m_preview = r.array();
        } else {
            m_preview = {};
            ps = r.error.problems;
            for (Problem &p : ps) {
                if (p.field.startsWith(QLatin1String("settings.")))
                    p.field = p.field.mid(9);
            }
            if (ps.isEmpty())
                m_previewError = r.error.message;
        }
        m_form->showProblems(ps);
        // What pre-translation would do, with the preparation.
        QStringList said;
        for (const QJsonValue &v : std::as_const(m_preview)) {
            const QJsonObject p = v.toObject();
            said << tr("%1: %2 filled, %3 confirmed")
                        .arg(Languages::name(p.value(QLatin1String("lang")).toString()), plural(p.value(QLatin1String("filled")).toInt(), tr("1 segment"), tr("%1 segments")))
                        .arg(p.value(QLatin1String("confirmed")).toInt());
        }
        m_form->setPrepareSummary(said.isEmpty() || !m_form->settings().value(QLatin1String("prepare")).toObject().value(QLatin1String("pretranslate")).toBool()
                                      ? QString()
                                      : said.join(QStringLiteral(". ")) + QLatin1Char('.'));
        fillReview();
    });
}

void NewProjectPage::fillReview()
{
    // The parts of the form shown here are kept; the rest is made again.
    QWidget *p = m_panels[4];
    QWidget *checks = m_form->checks(p);
    QWidget *prepare = m_form->prepare(p);
    m_reviewBody->removeWidget(checks);
    m_reviewBody->removeWidget(prepare);
    clear(m_reviewBody);
    m_analysisBody = nullptr;
    const QJsonObject s = m_form->settings();

    // The summary.
    auto *summary = new QWidget(p);
    summary->setAccessibleName(tr("Summary"));
    auto *grid = new QGridLayout(summary);
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setHorizontalSpacing(24);
    grid->setVerticalSpacing(8);
    grid->setColumnStretch(1, 1);
    int row = 0;
    const auto line = [&](const QString &label, const QString &html) {
        auto *l = Web::text(label, "secondary", summary);
        auto *v = new QLabel(html, summary);
        v->setWordWrap(true);
        v->setTextFormat(Qt::RichText);
        l->setBuddy(v);
        grid->addWidget(l, row, 0, Qt::AlignTop);
        grid->addWidget(v, row, 1);
        ++row;
    };
    const QString muted = Theme::color("text3").name();
    const auto grey = [&muted](const QString &t) { return QStringLiteral("<span style=\"color:%1\">%2</span>").arg(muted, t.toHtmlEscaped()); };
    QString project = s.value(QLatin1String("name")).toString().toHtmlEscaped();
    if (!s.value(QLatin1String("client")).toString().isEmpty())
        project += grey(tr(" for %1").arg(s.value(QLatin1String("client")).toString()));
    const QDateTime due = QDateTime::fromString(s.value(QLatin1String("due")).toString(), Qt::ISODate);
    if (due.isValid())
        project += grey(tr(", due %1").arg(Languages::when(due.toLocalTime())));
    line(tr("Project"), project);
    int translatable = 0, reference = 0;
    QString package;
    for (const Doc &d : std::as_const(m_docs)) {
        if (d.role == QLatin1String("reference")) {
            ++reference;
        } else if (d.role == QLatin1String("package")) {
            package = d.name;
            translatable += int(d.inspection.value(QLatin1String("package")).toObject().value(QLatin1String("files")).toArray().size());
        } else {
            ++translatable;
        }
    }
    QString files = tr("%1 to translate").arg(plural(translatable, tr("1 file"), tr("%1 files"))).toHtmlEscaped();
    if (reference)
        files += QStringLiteral(", ") + plural(reference, tr("1 reference file"), tr("%1 reference files")).toHtmlEscaped();
    if (!package.isEmpty())
        files += grey(tr(" from the %1 %2").arg(packageKind(package), package));
    line(tr("Files"), files);
    QStringList targetNames;
    const QStringList targets = strings(s.value(QLatin1String("targetLangs")));
    for (const QString &t : targets)
        targetNames << Languages::name(t);
    line(tr("Languages"), tr("%1 into %2").arg(Languages::name(s.value(QLatin1String("sourceLang")).toString()), targetNames.join(QStringLiteral(", "))).toHtmlEscaped());
    QStringList mems;
    for (const QString &t : targets) {
        QStringList names;
        for (const QJsonValue &m : s.value(QLatin1String("memories")).toObject().value(t).toArray()) {
            const int pen = m.toObject().value(QLatin1String("penalty")).toInt();
            names << (pen ? QStringLiteral("%1 (−%2)").arg(m.toObject().value(QLatin1String("memory")).toString()).arg(pen) : m.toObject().value(QLatin1String("memory")).toString());
        }
        QString one = QStringLiteral("<b>%1</b> %2").arg(t.toHtmlEscaped(), (names.isEmpty() ? tr("none") : names.join(QStringLiteral(", "))).toHtmlEscaped());
        if (s.value(QLatin1String("projectMemory")).toBool())
            one += grey(tr(", and a project memory"));
        mems << one;
    }
    line(tr("Memories"), mems.join(QStringLiteral("<br>")));
    const QJsonObject term = s.value(QLatin1String("terminology")).toObject();
    const QString mode = term.value(QLatin1String("mode")).toString();
    const QStringList masters = strings(term.value(QLatin1String("masters")));
    Connection *c = connection();
    const QString kept = c && !c->isLocal() ? tr("on %1").arg(c->displayName()) : tr("on this computer");
    QString terms;
    if (mode == QLatin1String("direct")) {
        terms = tr("New terms go straight into %1.").arg(term.value(QLatin1String("writeTo")).toString().isEmpty() ? masters.join(QStringLiteral(", ")) : term.value(QLatin1String("writeTo")).toString());
    } else if (mode == QLatin1String("layered")) {
        terms = tr("A new project termbase %1, based on %2.").arg(kept, masters.join(QStringLiteral(", "))) + QLatin1Char(' ') +
                (term.value(QLatin1String("autoUpdate")).toBool() ? tr("Takes in master changes each time the project opens.") : tr("Master changes wait until you take them in."));
        const QStringList adds = strings(term.value(QLatin1String("addLanguages")));
        if (!adds.isEmpty())
            terms += QLatin1Char(' ') + tr("Languages are added to %1.").arg(adds.join(QStringLiteral(", ")));
    } else {
        terms = tr("A new project termbase %1, no master.").arg(kept);
    }
    line(tr("Terminology"), terms.toHtmlEscaped());
    m_reviewBody->addWidget(summary);

    // Checks and context matches, folded away until asked for.
    const bool both = s.value(QLatin1String("analysis")).toObject().value(QLatin1String("context")).toString() == QLatin1String("both");
    auto *toggle = new QPushButton(p);
    toggle->setProperty("stepLink", true);
    toggle->setFlat(true);
    toggle->setCursor(Qt::PointingHandCursor);
    toggle->setCheckable(true);
    toggle->setChecked(m_checksOpen);
    toggle->setIcon(Icons::get(QStringLiteral("chevron-right"), Theme::color("accentText")));
    toggle->setText(tr("Checks and context matches"));
    toggle->setAccessibleName(m_checksOpen ? tr("Hide the checks and context matches") : tr("Show the checks and context matches"));
    auto *state = Web::text(tr("%1 of %2 checks on, context matches %3").arg(m_form->checksOn()).arg(m_form->checksOf()).arg(both ? tr("by the segments before and after") : tr("by the segment before")),
                            "muted", p);
    state->setWordWrap(false);
    m_reviewBody->addWidget(Web::row({toggle, state}, p));
    connect(toggle, &QPushButton::toggled, this, [this](bool open) {
        m_checksOpen = open;
        fillReview();
    });
    m_reviewBody->addWidget(checks);
    checks->setVisible(m_checksOpen);

    m_reviewBody->addWidget(prepare);
    prepare->show();

    // The match analysis.
    auto *analysis = new QWidget(p);
    m_analysisBody = new QVBoxLayout(analysis);
    m_analysisBody->setContentsMargins(0, 0, 0, 0);
    m_analysisBody->setSpacing(8);
    m_reviewBody->addWidget(analysis);
    fillAnalysis();

    // Saving the settings as a template.
    auto *tpl = new QWidget(p);
    auto *tv = new QVBoxLayout(tpl);
    tv->setContentsMargins(0, 0, 0, 0);
    tv->setSpacing(8);
    auto *as = new QCheckBox(tr("Save these settings as a template"), tpl);
    as->setChecked(m_asTemplate);
    tv->addWidget(as);
    if (m_asTemplate) {
        auto *name = new QLineEdit(m_templateName, tpl);
        name->setAccessibleName(tr("Template name"));
        auto *field = Web::field(tr("Template name"), name, tpl, false, tr("Languages, memories, termbases, checks and pre-translation; not the files, name or due date."));
        field->setContentsMargins(26, 0, 0, 0);
        auto *error = Web::text(problems().join(QLatin1Char('\n')), "error", field);
        error->setVisible(!error->text().isEmpty());
        field->layout()->addWidget(error);
        tv->addWidget(field);
        connect(name, &QLineEdit::textEdited, this, [this, error](const QString &t) {
            m_templateName = t;
            error->setText(problems().join(QLatin1Char('\n')));
            error->setVisible(!error->text().isEmpty());
            m_create->setEnabled(!m_previewing && !m_creating && m_previewError.isEmpty() && !m_preview.isEmpty() && problems().isEmpty());
        });
    }
    connect(as, &QCheckBox::toggled, this, [this](bool on) {
        m_asTemplate = on;
        if (on && m_templateName.isEmpty())
            m_templateName = m_form->settings().value(QLatin1String("client")).toString().isEmpty() ? m_form->settings().value(QLatin1String("name")).toString()
                                                                                                 : m_form->settings().value(QLatin1String("client")).toString();
        fillReview();
    });
    m_reviewBody->addWidget(tpl);

    // Why the project can't be made yet.
    QStringList all;
    for (const char *f : {"name", "client", "due", "files", "sourceLang", "targetLangs", "memories", "terminology", "qa", "analysis", "prepare"})
        all << m_form->problemsAt(QLatin1String(f));
    all.removeDuplicates();
    if (!all.isEmpty() || !m_previewError.isEmpty() || !m_createError.isEmpty()) {
        QString text = !m_createError.isEmpty() ? tr("The project was not made: %1").arg(m_createError)
            : !m_previewError.isEmpty()         ? tr("The project can't be checked: %1").arg(m_previewError)
                                                : tr("The project can't be created yet") + QStringLiteral("\n• ") + all.join(QStringLiteral("\n• "));
        auto *note = Web::note(text, QStringLiteral("warn"), p);
        note->setObjectName(QStringLiteral("cantCreate"));
        note->setAccessibleName(text);
        m_reviewBody->addWidget(note);
    }
    m_create->setText(m_creating ? tr("Creating…") : c && !c->isLocal() ? tr("Create on the server") : tr("Create project"));
    m_create->setEnabled(!m_previewing && !m_creating && m_previewError.isEmpty() && !m_preview.isEmpty() && all.isEmpty() && problems().isEmpty());
}

void NewProjectPage::fillAnalysis()
{
    if (!m_analysisBody)
        return;
    clear(m_analysisBody);
    QWidget *p = m_analysisBody->parentWidget();
    m_analysisBody->addWidget(Web::text(tr("Match analysis"), "fieldLabel", p));
    if (m_preview.isEmpty()) {
        m_analysisBody->addWidget(Web::text(m_previewing ? tr("Analysing the files…") : tr("The analysis follows once the project can be created."), "muted", p));
        return;
    }
    QStringList langs;
    for (const QJsonValue &v : std::as_const(m_preview))
        langs << v.toObject().value(QLatin1String("lang")).toString();
    if (!langs.contains(m_reviewLang))
        m_reviewLang = langs.value(0);
    if (langs.size() > 1) {
        auto *tabs = new Web::Tabs(tr("Target language"), p, true);
        for (const QString &l : std::as_const(langs))
            tabs->add(Languages::name(l));
        tabs->setCurrent(int(langs.indexOf(m_reviewLang)));
        connect(tabs, &Web::Tabs::changed, this, [this, langs](int i) {
            m_reviewLang = langs.value(i);
            fillAnalysis();
        });
        m_analysisBody->addWidget(tabs);
    }
    for (const QJsonValue &v : std::as_const(m_preview)) {
        const QJsonObject pv = v.toObject();
        if (pv.value(QLatin1String("lang")).toString() != m_reviewLang)
            continue;
        const QStringList mems = strings(pv.value(QLatin1String("memories")));
        m_analysisBody->addWidget(Web::text(mems.isEmpty() ? tr("No memory for this language, so only repetitions count.")
                                                           : tr("Against %1, counted in this order: context matches first, then repetitions within a file and across files, then the best match.")
                                                                 .arg(mems.join(tr(" and "))),
                                            "help", p));
        QTableWidget *table = Bands::analysisTable(pv.value(QLatin1String("report")).toObject(), p);
        table->setAccessibleName(tr("Match analysis, %1").arg(Languages::name(m_reviewLang)));
        // As tall as its rows: the page scrolls, not the table.
        table->setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        table->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        table->resizeRowsToContents();
        int height = table->horizontalHeader()->height() + 2 * table->frameWidth();
        for (int r = 0; r < table->rowCount(); ++r)
            height += table->rowHeight(r);
        table->setFixedHeight(height);
        m_analysisBody->addWidget(table);
    }
}

void NewProjectPage::create()
{
    Connection *c = connection();
    if (!c || m_creating)
        return;
    m_creating = true;
    m_createError.clear();
    fillReview();
    const QJsonObject req = request();
    const QJsonObject s = req.value(QLatin1String("settings")).toObject();
    const QString id = c->id();
    onResult(c->api()->post(QStringLiteral("/api/projects"), req), this, [this, c, s, id](const ApiResult &r) {
        if (!r.ok()) {
            m_creating = false;
            QList<Problem> ps = r.error.problems;
            for (Problem &p : ps) {
                if (p.field.startsWith(QLatin1String("settings.")))
                    p.field = p.field.mid(9);
            }
            m_form->showProblems(ps);
            m_createError = ps.isEmpty() ? r.error.message : QString();
            fillReview();
            return;
        }
        const qint64 project = r.object().value(QLatin1String("id")).toInteger();
        // What follows the project: the memories filled from TMX files,
        // the template, and pre-translation. The project is made either
        // way; what went wrong is said on the project's page.
        m_form->importNewMemories(c, [this, c, s, id, project](const QString &) {
            const auto finish = [this, c, s, id, project] {
                if (!s.value(QLatin1String("prepare")).toObject().value(QLatin1String("pretranslate")).toBool()) {
                    emit created(id, project);
                    return;
                }
                onResult(c->api()->post(QStringLiteral("/api/projects/%1/pretranslate").arg(project), QJsonObject{}), this,
                         [this, id, project](const ApiResult &) { emit created(id, project); });
            };
            if (!m_asTemplate) {
                finish();
                return;
            }
            onResult(c->api()->put(QStringLiteral("/api/templates"), ProjectForm::templateOf(m_templateName.trimmed(), m_form->settings()), 0), this,
                     [finish](const ApiResult &) { finish(); });
        });
    });
}
