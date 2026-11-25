#include "projectpage.h"

#include "accesspanel.h"
#include "apiclient.h"
#include "bands.h"
#include "connection.h"
#include "eventfeed.h"
#include "icons.h"
#include "languages.h"
#include "projectform.h"
#include "roles.h"
#include "theme.h"
#include "webui.h"

#include <QCheckBox>
#include <QComboBox>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QInputDialog>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QPushButton>
#include <QSaveFile>
#include <QStandardPaths>
#include <QTableWidget>
#include <QVBoxLayout>

#include <memory>

namespace {

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

QVBoxLayout *host(QVBoxLayout *into, int spacing = 8)
{
    auto *v = new QVBoxLayout;
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(spacing);
    into->addLayout(v);
    return v;
}

QString packageKind(const QString &name)
{
    return name.endsWith(QLatin1String(".mqout"), Qt::CaseInsensitive) ? ProjectPage::tr("handoff package") : ProjectPage::tr("project package");
}

} // namespace

ProjectPage::ProjectPage(Connection *connection, qint64 projectId, QWidget *parent)
    : QWidget(parent)
    , m_conn(connection)
    , m_id(projectId)
    , m_form(new ProjectForm(this))
{
    setObjectName(QStringLiteral("project"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->setSpacing(0);
    Web::Page page = Web::page(this, 760);
    page.scroll->setAccessibleName(tr("Project"));
    QWidget *p = page.scroll->widget();

    auto *backButton = Web::button(tr("Projects"), Web::Quiet, p, QStringLiteral("arrow-left"));
    backButton->setAccessibleName(tr("Back to the projects"));
    m_open = Web::button(tr("Open in the editor"), Web::Normal, p, QStringLiteral("pencil-line"));
    QWidget *header = Web::header(QString(), QString(), {backButton, m_open}, p);
    m_title = header->findChild<QLabel *>(QStringLiteral("pageTitle"));
    page.body->addWidget(header);
    m_where = Web::text(QString(), "help", p);
    page.body->addWidget(m_where);
    connect(backButton, &QPushButton::clicked, this, &ProjectPage::back);
    connect(m_open, &QPushButton::clicked, this, [this] { emit openEditor(QString(), QString()); });

    QVBoxLayout *body = nullptr;
    page.body->addWidget(Web::panel(tr("Details"), p, &body));
    body->addWidget(m_form->details(p));

    page.body->addWidget(Web::panel(tr("Files"), p, &body));
    m_files = host(body, 12);

    page.body->addWidget(Web::panel(tr("Languages"), p, &body));
    m_languages = host(body);
    {
        auto *targets = new QWidget(p);
        auto *tv = new QVBoxLayout(targets);
        tv->setContentsMargins(0, 0, 0, 0);
        tv->setSpacing(8);
        tv->addWidget(legend(tr("Target languages"), targets));
        tv->addWidget(m_form->targets(targets));
        tv->addWidget(Web::text(tr("A language you add gets every file to translate, with nothing translated yet. A language you take out loses its translations."), "help", targets));
        body->addWidget(targets);
    }

    page.body->addWidget(Web::panel(tr("Memories"), p, &body));
    body->addWidget(m_form->memories(p));
    m_projectMemories = host(body);

    page.body->addWidget(Web::panel(tr("Terminology"), p, &body));
    body->addWidget(m_form->terminology(p));

    page.body->addWidget(Web::panel(tr("Segmentation"), p, &body));
    body->addWidget(m_form->segmentation(p));

    page.body->addWidget(Web::panel(tr("Checks and analysis"), p, &body));
    body->addWidget(m_form->checks(p));
    m_analysisBody = host(body);

    page.body->addWidget(Web::panel(tr("Pre-translation"), p, &body));
    body->addWidget(m_form->prepare(p));
    m_pretranslate = host(body);

    if (!connection->isLocal()) {
        m_workPanel = Web::panel(tr("Work"), p, &body);
        m_work = host(body, 12);
        page.body->addWidget(m_workPanel);
        m_nowPanel = Web::panel(tr("Working now"), p, &body);
        m_now = host(body);
        page.body->addWidget(m_nowPanel);
        auto *access = new QWidget(p);
        m_access_ = new QVBoxLayout(access);
        m_access_->setContentsMargins(0, 0, 0, 0);
        m_accessPanel = access;
        page.body->addWidget(access);
    }

    m_templatePanel = Web::panel(tr("Template"), p, &body);
    m_template = host(body);
    page.body->addWidget(m_templatePanel);

    m_deletePanel = Web::panel(tr("Delete the project"), p, &body);
    m_deleteHelp = Web::text(QString(), "help", m_deletePanel);
    body->addWidget(m_deleteHelp);
    auto *del = new Web::ConfirmButton(tr("Delete project"), QString(), tr("Delete"), m_deletePanel, false, QStringLiteral("trash"));
    del->setObjectName(QStringLiteral("deleteProject"));
    body->addWidget(del);
    connect(del, &Web::ConfirmButton::confirmed, this, &ProjectPage::deleteProject);
    page.body->addWidget(m_deletePanel);
    page.body->addStretch();
    outer->addWidget(page.scroll, 1);

    // .save-bar: what was changed is saved together, from the foot of the page.
    m_bar = new QFrame(this);
    m_bar->setObjectName(QStringLiteral("saveBar"));
    m_bar->setStyleSheet(QStringLiteral("QFrame#saveBar { background: %1; border-top: 1px solid %2; }").arg(Theme::color("surface").name(), Theme::color("border").name()));
    auto *bh = new QHBoxLayout(m_bar);
    bh->setContentsMargins(32, 10, 32, 10);
    bh->setSpacing(8);
    m_barError = Web::text(QString(), "error", m_bar);
    m_barError->setAccessibleName(tr("Why the changes were not saved"));
    m_barError->setVisible(false);
    bh->addWidget(m_barError, 1);
    bh->addStretch();
    m_discard = Web::button(tr("Discard changes"), Web::Quiet, m_bar);
    m_save = Web::button(tr("Save changes"), Web::Primary, m_bar);
    bh->addWidget(m_discard);
    bh->addWidget(m_save);
    outer->addWidget(m_bar);
    connect(m_discard, &QPushButton::clicked, this, &ProjectPage::discard);
    connect(m_save, &QPushButton::clicked, this, &ProjectPage::save);
    m_toast = new Web::Toast(this);

    connect(m_form, &ProjectForm::changed, this, [this] {
        if (!m_loaded)
            return;
        m_dirty = true;
        refreshBar();
        fillPretranslate();
        fillTemplate();
    });
    m_reload.setSingleShot(true);
    m_reload.setInterval(400);
    connect(&m_reload, &QTimer::timeout, this, &ProjectPage::reload);
    connect(connection->feed(), &EventFeed::event, this, [this](const QString &type, const QJsonObject &data) {
        if ((type == QLatin1String("project") || type == QLatin1String("presence")) && data.value(QLatin1String("projectId")).toInteger() == m_id)
            m_reload.start();
        if (type == QLatin1String("account"))
            m_reload.start();
    });
    connect(connection->feed(), &EventFeed::reset, &m_reload, qOverload<>(&QTimer::start));
    refreshBar();
    reload();
}

// The project is gone (deleted here or elsewhere): said once, however many
// reads after it find it missing.
void ProjectPage::leave()
{
    if (m_gone)
        return;
    m_gone = true;
    emit gone();
}

void ProjectPage::resizeEvent(QResizeEvent *e)
{
    QWidget::resizeEvent(e);
    m_toast->place();
}

QString ProjectPage::lastNotice() const
{
    return m_toast->last();
}

void ProjectPage::say(const QString &message)
{
    if (!message.isEmpty())
        m_toast->say(message);
}

bool ProjectPage::manages() const
{
    const QString role = m_access.value(QLatin1String("role")).toString();
    return Roles::allows(role, QStringLiteral("manage")) || (m_conn && m_conn->isAdmin());
}

QJsonObject ProjectPage::settings() const
{
    return m_project.value(QLatin1String("settings")).toObject();
}

void ProjectPage::reload()
{
    if (!m_conn || !m_conn->isReady())
        return;
    const QString base = QStringLiteral("/api/projects/%1").arg(m_id);
    onResult(m_conn->api()->get(base), this, [this, base](const ApiResult &r) {
        if (!r.ok()) {
            if (r.status == 404)
                leave();
            else
                say(r.error.message);
            return;
        }
        m_project = r.object();
        m_access = m_project.value(QLatin1String("access")).toObject();
        showProject();
        onResult(m_conn->api()->get(base + QStringLiteral("/progress")), this, [this](const ApiResult &p) {
            if (!p.ok())
                return;
            m_progress = p.array();
            fillFiles();
        });
        onResult(m_conn->api()->get(QStringLiteral("/api/templates")), this, [this](const ApiResult &t) {
            m_templates = t.ok() ? t.array() : QJsonArray();
            fillTemplate();
        });
        if (m_conn->isLocal())
            return;
        onResult(m_conn->api()->get(base + QStringLiteral("/assignments")), this, [this](const ApiResult &a) {
            if (!a.ok())
                return;
            m_assignments = a.array();
            fillWork();
        });
        onResult(m_conn->api()->get(base + QStringLiteral("/presence")), this, [this](const ApiResult &a) {
            m_presence = a.ok() ? a.array() : QJsonArray();
            fillNow();
        });
        if (manages()) {
            onResult(m_conn->api()->get(QStringLiteral("/api/people")), this, [this](const ApiResult &a) {
                if (!a.ok() || a.array() == m_people)
                    return;
                m_people = a.array();
                fillWork();
            });
        }
    });
}

void ProjectPage::showProject()
{
    const QJsonObject s = settings();
    const QString name = s.value(QLatin1String("name")).toString();
    m_title->setText(tr("%1: settings").arg(name));
    m_title->setAccessibleName(m_title->text());
    QString where = m_conn->isLocal() ? tr("On this computer") : tr("On the team server %1").arg(m_conn->displayName());
    if (!s.value(QLatin1String("client")).toString().isEmpty())
        where += tr(", for %1").arg(s.value(QLatin1String("client")).toString());
    where += QLatin1Char('.');
    const QDateTime made = QDateTime::fromString(m_project.value(QLatin1String("createdAt")).toString(), Qt::ISODate);
    const QString by = m_project.value(QLatin1String("createdBy")).toString();
    if (made.isValid())
        where += QLatin1Char(' ') + (by.isEmpty() ? tr("Made on %1.").arg(QLocale().toString(made.toLocalTime().date(), QLocale::ShortFormat))
                                                 : tr("Made by %1 on %2.").arg(by, QLocale().toString(made.toLocalTime().date(), QLocale::ShortFormat)));
    m_where->setText(where);
    const bool manage = manages();
    m_form->setReadOnly(!manage);
    m_bar->setVisible(manage);
    m_templatePanel->setVisible(manage);
    m_deletePanel->setVisible(manage);
    m_deleteHelp->setText(m_conn->isLocal() ? tr("The project, its files and translations are deleted. Its memories and termbase stay; delete them on their own if nothing else uses them.")
                                            : tr("The project, its files and translations and the work given out in it are removed from the server for everyone. Its memories and termbase stay; delete them on their own if nothing else uses them."));
    if (auto *del = findChild<Web::ConfirmButton *>(QStringLiteral("deleteProject")))
        del->setQuestion(m_conn->isLocal() ? tr("Delete “%1” and its translations?").arg(name) : tr("Delete “%1” on %2 for everyone?").arg(name, m_conn->displayName()));
    if (m_accessPanel) {
        m_accessPanel->setVisible(manage);
        if (manage && m_access_->count() == 0)
            m_access_->addWidget(new AccessPanel(m_conn, QStringLiteral("project"), QString::number(m_id), tr("People with access"), m_accessPanel));
    }
    // Settings being edited are not replaced under the person's hands.
    if (!m_dirty) {
        m_form->load(m_conn, [this](const QString &error) {
            if (!error.isEmpty()) {
                say(error);
                return;
            }
            if (m_dirty)
                return;
            m_loaded = false;
            QStringList fixed;
            for (const QJsonValue &f : settings().value(QLatin1String("files")).toArray()) {
                const QString t = f.toObject().value(QLatin1String("targetLang")).toString();
                if (!t.isEmpty() && !fixed.contains(t))
                    fixed << t;
            }
            m_form->setFixedTargets(fixed);
            m_form->setSuggestedTargets(strings(settings().value(QLatin1String("targetLangs"))));
            m_form->setSettings(settings(), false);
            m_form->setReadOnly(!manages());
            m_loaded = true;
            fillProjectMemories();
            fillPretranslate();
            fillTemplate();
        });
    }
    fillFiles();
    fillLanguages();
    fillProjectMemories();
    fillAnalysis();
    fillPretranslate();
    fillWork();
    fillNow();
    fillTemplate();
    refreshBar();
}

void ProjectPage::refreshBar()
{
    m_discard->setEnabled(m_dirty && !m_saving);
    m_save->setEnabled(m_dirty && !m_saving);
    m_save->setText(m_saving ? tr("Saving…") : tr("Save changes"));
    m_barError->setText(m_saveError);
    m_barError->setVisible(!m_saveError.isEmpty());
}

// Files: what each is for and how far it is, the original and the
// translation to save, and files to add.
void ProjectPage::fillFiles()
{
    clear(m_files);
    QWidget *p = m_files->parentWidget();
    const QJsonObject s = settings();
    const QJsonArray files = s.value(QLatin1String("files")).toArray();
    const QStringList targets = strings(s.value(QLatin1String("targetLangs")));
    const bool manage = manages();
    bool package = false;
    if (files.isEmpty()) {
        m_files->addWidget(Web::text(tr("The project has no files yet."), "muted", p));
    } else {
        auto *table = new Web::Table({tr("File"), tr("Words"), tr("Done"), QString()}, p);
        table->setAccessibleName(tr("Files"));
        table->setNumeric({1});
        table->setStretch(0);
        for (const QJsonValue &fv : files) {
            const QJsonObject f = fv.toObject();
            const QString name = f.value(QLatin1String("name")).toString();
            const QString role = f.value(QLatin1String("role")).toString();
            const QString only = f.value(QLatin1String("targetLang")).toString();
            package = package || role == QLatin1String("package");
            QStringList names;
            for (const QString &t : targets)
                names << Languages::name(t);
            const QString sub = role == QLatin1String("reference") ? tr("Reference file")
                : role == QLatin1String("package")                 ? tr("The %1 the project was made from").arg(packageKind(name))
                : !only.isEmpty()                                  ? tr("Translated into %1 only").arg(Languages::name(only))
                : targets.size() > 1                               ? tr("Translated into every target language")
                                                                   : tr("Translated into %1").arg(names.join(QStringLiteral(", ")));
            auto *cell = Web::cell(name, sub, table);
            // How far it is in each language, each a way into the editor.
            int words = 0;
            auto *done = new QWidget(table);
            auto *dv = new QVBoxLayout(done);
            dv->setContentsMargins(0, 0, 0, 0);
            dv->setSpacing(4);
            QStringList langs;
            for (const QJsonValue &pv : std::as_const(m_progress)) {
                const QJsonObject pr = pv.toObject();
                if (pr.value(QLatin1String("file")).toString() != name)
                    continue;
                const QString lang = pr.value(QLatin1String("lang")).toString();
                langs << lang;
                const int segs = pr.value(QLatin1String("segments")).toInt();
                const int conf = pr.value(QLatin1String("confirmed")).toInt();
                words = qMax(words, pr.value(QLatin1String("words")).toInt());
                const int pct = segs ? qRound(100.0 * conf / segs) : 100;
                auto *open = new QPushButton(QStringLiteral("%1%").arg(pct), done);
                open->setProperty("stepLink", true);
                open->setFlat(true);
                open->setCursor(Qt::PointingHandCursor);
                open->setAccessibleName(tr("Translate %1 into %2: %3 of %4 segments confirmed").arg(name, Languages::name(lang)).arg(conf).arg(segs));
                open->setToolTip(tr("%1 of %2 segments confirmed. Opens the file in the editor.").arg(conf).arg(segs));
                open->setObjectName(QStringLiteral("progress"));
                open->setProperty("file", name);
                open->setProperty("lang", lang);
                open->setProperty("confirmed", conf);
                open->setProperty("segments", segs);
                connect(open, &QPushButton::clicked, this, [this, name, lang] { emit openEditor(name, lang); });
                dv->addWidget(Web::row({Web::langChip(lang, done), open}, done, 6));
            }
            if (langs.isEmpty())
                dv->addWidget(Web::text(QStringLiteral("–"), "muted", done));
            // What can be saved or taken away.
            QList<QWidget *> acts;
            auto *original = Web::button(tr("Original"), Web::Quiet | Web::Small, table, QStringLiteral("download"));
            original->setAccessibleName(tr("Save the original of %1").arg(name));
            connect(original, &QPushButton::clicked, this, [this, name] {
                download(QStringLiteral("/api/projects/%1/files/content").arg(m_id), QUrlQuery{{QStringLiteral("name"), name}}, tr("the original of %1").arg(name));
            });
            acts << original;
            if (role == QLatin1String("translatable")) {
                auto *translated = Web::button(tr("Translation"), Web::Quiet | Web::Small, table, QStringLiteral("download"));
                translated->setAccessibleName(tr("Save the translation of %1").arg(name));
                connect(translated, &QPushButton::clicked, this, [this, name, langs] {
                    QString lang = langs.value(0);
                    if (langs.size() > 1) {
                        QStringList labels;
                        for (const QString &l : langs)
                            labels << Languages::label(l);
                        bool ok = false;
                        const QString pick = QInputDialog::getItem(this, tr("Save the translation"), tr("Language:"), labels, 0, false, &ok);
                        if (!ok)
                            return;
                        lang = langs.value(labels.indexOf(pick));
                    }
                    download(QStringLiteral("/api/projects/%1/files/translated").arg(m_id), QUrlQuery{{QStringLiteral("name"), name}, {QStringLiteral("lang"), lang}},
                             tr("the translation of %1").arg(name));
                });
                acts << translated;
            }
            if (manage) {
                auto *remove = new Web::ConfirmButton(tr("Remove"),
                                                      m_conn->isLocal() ? tr("Remove %1 with its translations?").arg(name)
                                                                        : tr("Remove %1 with its translations and the work given out in it, for everyone?").arg(name),
                                                      tr("Remove"), table, true, QStringLiteral("trash"));
                remove->button()->setAccessibleName(tr("Remove %1").arg(name));
                connect(remove, &Web::ConfirmButton::confirmed, this, [this, name] { removeFile(name); });
                acts << remove;
            }
            table->addRow({cell, Web::cellText(words ? QLocale().toString(words) : QStringLiteral("–"), table, true), done, Web::cellActions(acts, table)});
        }
        table->fit();
        m_files->addWidget(table);
    }
    if (package) {
        auto *back = Web::button(tr("Save the return package"), Web::Small, p, QStringLiteral("download"));
        m_files->addWidget(back, 0, Qt::AlignLeft);
        connect(back, &QPushButton::clicked, this, [this] { download(QStringLiteral("/api/projects/%1/return-package").arg(m_id), {}, tr("the return package")); });
    }
    if (!manage)
        return;
    m_files->addWidget(Web::text(tr("Add files"), "fieldLabel", p));
    auto *zone = new Web::DropZone(p);
    zone->setAccessibleName(tr("Add files"));
    auto *zh = new QHBoxLayout(zone);
    zh->setContentsMargins(16, 12, 16, 12);
    zh->setSpacing(12);
    auto *icon = new QLabel(zone);
    icon->setPixmap(Icons::get(QStringLiteral("upload"), Theme::color("text2")).pixmap(18, 18));
    zh->addWidget(icon, 0, Qt::AlignVCenter);
    zh->addWidget(Web::text(m_busy.isEmpty()
                                ? tr("Drop files or folders here. DOCX, PPTX, XLSX, IDML and web page files keep their formatting; of a PDF the text is read, and its translation is a DOCX document; bilingual files (.sdlxliff, .mqxliff, .mqxlz, .xlf) keep their translations and states.")
                                : m_busy,
                            "secondary", zone),
                  1);
    auto *addFilesButton = Web::button(tr("Add files"), Web::Small, zone, QStringLiteral("file-text"));
    auto *addFolder = Web::button(tr("Add a folder"), Web::Small, zone, QStringLiteral("folder-open"));
    addFilesButton->setEnabled(m_busy.isEmpty());
    addFolder->setEnabled(m_busy.isEmpty());
    zh->addWidget(addFilesButton, 0, Qt::AlignVCenter);
    zh->addWidget(addFolder, 0, Qt::AlignVCenter);
    connect(zone, &Web::DropZone::dropped, this, &ProjectPage::addFiles);
    connect(addFilesButton, &QPushButton::clicked, this, [this] { addFiles(QFileDialog::getOpenFileNames(this, tr("Files to add"))); });
    connect(addFolder, &QPushButton::clicked, this, [this] {
        const QString dir = QFileDialog::getExistingDirectory(this, tr("A folder of files"));
        if (!dir.isEmpty())
            addFiles({dir});
    });
    m_files->addWidget(zone);
    m_files->addWidget(Web::text(tr("Reference files (a style guide, an earlier version) travel with the project for the translators to read; they are not translated."), "help", p));
}

void ProjectPage::fillLanguages()
{
    clear(m_languages);
    QWidget *p = m_languages->parentWidget();
    const QString src = settings().value(QLatin1String("sourceLang")).toString();
    m_languages->addWidget(legend(tr("Source language"), p));
    m_languages->addWidget(Web::row({new QLabel(Languages::name(src), p), Web::langChip(src, p)}, p));
    m_languages->addWidget(Web::text(tr("The source language stays: every file was segmented by its rules."), "help", p));
}

// The project's own memories, and adding their units to a main memory
// once the project is done.
void ProjectPage::fillProjectMemories()
{
    clear(m_projectMemories);
    QWidget *p = m_projectMemories->parentWidget();
    const QJsonObject s = settings();
    const QString name = s.value(QLatin1String("name")).toString();
    const QJsonArray catalog = m_form->catalog().value(QLatin1String("memories")).toArray();
    QList<QJsonObject> own;
    for (const QString &t : strings(s.value(QLatin1String("targetLangs")))) {
        for (const QJsonValue &m : s.value(QLatin1String("memories")).toObject().value(t).toArray()) {
            if (m.toObject().value(QLatin1String("memory")).toString() != QStringLiteral("%1 (%2)").arg(name, t))
                continue;
            for (const QJsonValue &c : catalog) {
                if (c.toObject().value(QLatin1String("name")).toString() == m.toObject().value(QLatin1String("memory")).toString())
                    own << c.toObject();
            }
        }
    }
    if (own.isEmpty() || !manages())
        return;
    m_projectMemories->addWidget(Web::text(tr("Project memories"), "fieldLabel", p));
    for (const QJsonObject &m : std::as_const(own)) {
        const QString mname = m.value(QLatin1String("name")).toString();
        const int units = m.value(QLatin1String("units")).toInt();
        auto *title = new QLabel(QStringLiteral("<b>%1</b> <span style=\"color:%2\">%3</span>")
                                     .arg(mname.toHtmlEscaped(), Theme::color("text3").name(), plural(units, tr("1 unit"), tr("%1 units")).toHtmlEscaped()),
                                 p);
        auto *pick = new QComboBox(p);
        pick->setAccessibleName(tr("Main memory for the units of %1").arg(mname));
        for (const QJsonValue &cv : catalog) {
            const QJsonObject c = cv.toObject();
            const QString cname = c.value(QLatin1String("name")).toString();
            bool projects = false;
            for (const QJsonObject &o : std::as_const(own))
                projects = projects || o.value(QLatin1String("name")).toString() == cname;
            if (!projects && Languages::samePrimary(c.value(QLatin1String("sourceLang")).toString(), m.value(QLatin1String("sourceLang")).toString()) &&
                Languages::samePrimary(c.value(QLatin1String("targetLang")).toString(), m.value(QLatin1String("targetLang")).toString()))
                pick->addItem(cname, cname);
        }
        if (pick->count() == 0) {
            pick->deleteLater();
            pick->hide();
            m_projectMemories->addWidget(
                Web::row({title, Web::text(tr("No main memory for %1 to add them to.").arg(Languages::name(m.value(QLatin1String("targetLang")).toString())), "muted", p)}, p));
            continue;
        }
        auto *add = Web::button(tr("Add its units"), Web::Small, p);
        add->setAccessibleName(tr("Add the units of %1 to the main memory").arg(mname));
        add->setEnabled(units > 0);
        m_projectMemories->addWidget(Web::row({title, pick, add}, p));
        connect(add, &QPushButton::clicked, this, [this, mname, pick, add] {
            const QString main = pick->currentData().toString();
            add->setEnabled(false);
            onResult(m_conn->api()->get(QStringLiteral("/api/memories/%1/export").arg(ApiClient::seg(mname))), this, [this, mname, main](const ApiResult &tmx) {
                if (!tmx.ok()) {
                    say(tmx.error.message);
                    fillProjectMemories();
                    return;
                }
                onResult(m_conn->api()->upload("POST", QStringLiteral("/api/memories/%1/import").arg(ApiClient::seg(main)), tmx.body, QStringLiteral("application/xml")), this,
                         [this, mname, main](const ApiResult &r) {
                             say(r.ok() ? tr("Added %1 of %2 to %3").arg(plural(r.object().value(QLatin1String("stored")).toInt(), tr("1 unit"), tr("%1 units")), mname, main)
                                        : r.error.message);
                             fillProjectMemories();
                         });
            });
        });
    }
    m_projectMemories->addWidget(Web::text(tr("Units already in the main memory with the same translation are updated, not added twice."), "help", p));
}

// The match analysis of the files as they are, against the saved settings.
void ProjectPage::fillAnalysis()
{
    clear(m_analysisBody);
    QWidget *p = m_analysisBody->parentWidget();
    m_analysisBody->addWidget(legend(tr("Match analysis"), p));
    auto *run = Web::button(m_analysing ? tr("Analysing…") : m_analysis.isEmpty() ? tr("Analyse the files") : tr("Analyse again"), Web::Small, p, QStringLiteral("bar-chart"));
    run->setEnabled(!m_analysing);
    connect(run, &QPushButton::clicked, this, &ProjectPage::runAnalysis);
    QList<QWidget *> row{run};
    if (!m_analysis.isEmpty()) {
        auto *csv = Web::button(tr("Save as CSV"), Web::Quiet | Web::Small, p, QStringLiteral("download"));
        connect(csv, &QPushButton::clicked, this, &ProjectPage::saveAnalysis);
        row << csv;
    }
    m_analysisBody->addWidget(Web::row(row, p));
    if (m_analysis.isEmpty()) {
        m_analysisBody->addWidget(Web::text(tr("Counts the words to translate by how well the project's memories match them, with the saved settings."), "help", p));
        return;
    }
    QStringList langs;
    for (const QJsonValue &v : std::as_const(m_analysis))
        langs << v.toObject().value(QLatin1String("targetLang")).toString();
    if (!langs.contains(m_analysisLang))
        m_analysisLang = langs.value(0);
    if (langs.size() > 1) {
        auto *tabs = new Web::Tabs(tr("Target language"), p, true);
        for (const QString &l : std::as_const(langs))
            tabs->add(Languages::name(l));
        tabs->setCurrent(int(langs.indexOf(m_analysisLang)));
        connect(tabs, &Web::Tabs::changed, this, [this, langs](int i) {
            m_analysisLang = langs.value(i);
            fillAnalysis();
        });
        m_analysisBody->addWidget(tabs);
    }
    for (const QJsonValue &v : std::as_const(m_analysis)) {
        const QJsonObject rep = v.toObject();
        if (rep.value(QLatin1String("targetLang")).toString() != m_analysisLang)
            continue;
        QStringList mems;
        for (const QJsonValue &m : settings().value(QLatin1String("memories")).toObject().value(m_analysisLang).toArray()) {
            if (m.toObject().value(QLatin1String("lookup")).toBool())
                mems << m.toObject().value(QLatin1String("memory")).toString();
        }
        m_analysisBody->addWidget(Web::text(mems.isEmpty() ? tr("No memory for this language, so only repetitions count.")
                                                           : tr("Against %1, counted in this order: context matches first, then repetitions within a file and across files, then the best match.")
                                                                 .arg(mems.join(tr(" and "))),
                                            "help", p));
        QTableWidget *table = Bands::analysisTable(rep, p);
        table->setAccessibleName(tr("Match analysis, %1").arg(Languages::name(m_analysisLang)));
        table->setObjectName(QStringLiteral("analysis"));
        // As tall as its rows: the page scrolls, not the table.
        table->setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        table->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        table->setProperty("tallAsRows", true);
        m_analysisBody->addWidget(table);
    }
}

void ProjectPage::runAnalysis()
{
    m_analysing = true;
    fillAnalysis();
    onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/analysis").arg(m_id), QJsonObject{}), this, [this](const ApiResult &r) {
        m_analysing = false;
        if (r.ok())
            m_analysis = r.array();
        else
            say(r.error.message);
        fillAnalysis();
    });
}

void ProjectPage::saveAnalysis()
{
    const QString name = settings().value(QLatin1String("name")).toString();
    const QString to = QFileDialog::getSaveFileName(this, tr("Save the analysis"),
                                                    QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation) + QStringLiteral("/%1 analysis.csv").arg(name),
                                                    tr("CSV files (*.csv)"));
    if (to.isEmpty())
        return;
    QByteArray csv;
    const auto quote = [](QString s) { return QLatin1Char('"') + s.replace(QLatin1Char('"'), QStringLiteral("\"\"")) + QLatin1Char('"'); };
    csv += "\"Language\",\"File\",\"Match\",\"Segments\",\"Words\",\"Characters\"\r\n";
    for (const QJsonValue &v : std::as_const(m_analysis)) {
        const QJsonObject rep = v.toObject();
        const QString lang = rep.value(QLatin1String("targetLang")).toString();
        for (const QJsonValue &fv : rep.value(QLatin1String("files")).toArray()) {
            const QJsonObject f = fv.toObject();
            for (const QJsonValue &rv : f.value(QLatin1String("rows")).toArray()) {
                const QJsonObject r = rv.toObject();
                csv += QStringLiteral("%1,%2,%3,%4,%5,%6\r\n")
                           .arg(quote(lang), quote(f.value(QLatin1String("name")).toString()), quote(r.value(QLatin1String("name")).toString()))
                           .arg(r.value(QLatin1String("segments")).toInt())
                           .arg(r.value(QLatin1String("words")).toInt())
                           .arg(r.value(QLatin1String("characters")).toInt())
                           .toUtf8();
            }
        }
    }
    QSaveFile f(to);
    if (!f.open(QIODevice::WriteOnly) || f.write("\xEF\xBB\xBF" + csv) < 0 || !f.commit())
        say(tr("The analysis could not be saved: %1").arg(f.errorString()));
    else
        say(tr("Saved %1").arg(QDir::toNativeSeparators(to)));
}

void ProjectPage::fillPretranslate()
{
    clear(m_pretranslate);
    QWidget *p = m_pretranslate->parentWidget();
    if (!manages())
        return;
    auto *now = Web::button(tr("Pre-translate now"), Web::Normal, p);
    now->setEnabled(!m_dirty);
    connect(now, &QPushButton::clicked, this, &ProjectPage::pretranslate);
    m_pretranslate->addWidget(now, 0, Qt::AlignLeft);
    m_pretranslate->addWidget(Web::text(m_dirty ? tr("Save your changes first; pre-translation uses the saved settings.")
                                                : tr("Fills the empty segments of every file and language with matches of %1% or more from the memories set for lookup.")
                                                      .arg(settings().value(QLatin1String("prepare")).toObject().value(QLatin1String("minScore")).toInt(100)),
                                        "help", p));
}

void ProjectPage::pretranslate()
{
    onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/pretranslate").arg(m_id), QJsonObject{}), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        int filled = 0;
        for (const QJsonValue &v : r.array())
            filled += v.toObject().value(QLatin1String("filled")).toInt();
        say(filled ? tr("Pre-translated %1").arg(plural(filled, tr("1 segment"), tr("%1 segments"))) : tr("No empty segment has a match good enough to insert"));
        reload();
    });
}

// The work given out, on a team server.
void ProjectPage::fillWork()
{
    if (!m_work)
        return;
    clear(m_work);
    QWidget *p = m_workPanel;
    const QJsonObject s = settings();
    const bool manage = manages();
    const qint64 me = m_conn->user().value(QLatin1String("id")).toInteger();
    if (m_assignments.isEmpty()) {
        m_work->addWidget(Web::text(tr("No work given out yet."), "muted", p));
    } else {
        auto *table = new Web::Table({tr("Who"), tr("Task"), tr("Part"), tr("Done"), QString()}, p);
        table->setAccessibleName(tr("Work given out"));
        table->setStretch(2);
        for (const QJsonValue &v : std::as_const(m_assignments)) {
            const QJsonObject a = v.toObject();
            const QString who = a.value(QLatin1String("userName")).toString();
            QStringList part;
            part << (a.value(QLatin1String("file")).toString().isEmpty() ? tr("All files") : a.value(QLatin1String("file")).toString());
            if (!a.value(QLatin1String("targetLang")).toString().isEmpty())
                part << Languages::name(a.value(QLatin1String("targetLang")).toString());
            if (!a.value(QLatin1String("fromSegment")).toString().isEmpty())
                part << tr("segments %1 to %2").arg(a.value(QLatin1String("fromSegment")).toString(), a.value(QLatin1String("toSegment")).toString());
            const QDateTime due = QDateTime::fromString(a.value(QLatin1String("due")).toString(), Qt::ISODate);
            if (due.isValid())
                part << tr("due %1").arg(Languages::when(due));
            auto *done = new QCheckBox(table);
            done->setAccessibleName(tr("%1's work is done").arg(who));
            done->setChecked(a.value(QLatin1String("done")).toBool());
            done->setEnabled(manage || a.value(QLatin1String("userId")).toInteger() == me);
            connect(done, &QCheckBox::toggled, this, [this, a](bool on) {
                QJsonObject next = a;
                next.insert(QStringLiteral("done"), on);
                onResult(m_conn->api()->put(QStringLiteral("/api/projects/%1/assignments/%2").arg(m_id).arg(a.value(QLatin1String("id")).toInteger()), next,
                                            a.value(QLatin1String("version")).toInt()),
                         this, [this, on](const ApiResult &r) {
                             say(r.ok() ? (on ? tr("Reported done") : tr("Reported not done")) : r.error.message);
                             reload();
                         });
            });
            QWidget *take = new QWidget(table);
            if (manage) {
                auto *back = new Web::ConfirmButton(tr("Take back"), tr("Take this work back from %1?").arg(who), tr("Take back"), table);
                back->button()->setAccessibleName(tr("Take the work back from %1").arg(who));
                connect(back, &Web::ConfirmButton::confirmed, this, [this, a] {
                    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1/assignments/%2").arg(m_id).arg(a.value(QLatin1String("id")).toInteger())), this,
                             [this](const ApiResult &r) {
                                 if (!r.ok())
                                     say(r.error.message);
                                 reload();
                             });
                });
                take->deleteLater();
                take = back;
            }
            table->addRow({Web::cell(who, QString(), table), Web::cellText(a.value(QLatin1String("task")).toString() == QLatin1String("review") ? tr("Review") : tr("Translate"), table),
                           Web::cellText(part.join(QStringLiteral(", ")), table), done, take});
        }
        table->fit();
        m_work->addWidget(table);
    }
    if (!manage)
        return;
    // Giving out work.
    auto *form = new QWidget(p);
    form->setAccessibleName(tr("Give out work"));
    auto *fv = new QVBoxLayout(form);
    fv->setContentsMargins(0, 0, 0, 0);
    fv->setSpacing(12);
    fv->addWidget(Web::text(tr("Give out work"), "fieldLabel", form));
    auto *who = new QComboBox(form);
    who->addItem(tr("Choose someone"), 0);
    for (const QJsonValue &v : std::as_const(m_people)) {
        const QJsonObject x = v.toObject();
        who->addItem(QStringLiteral("%1 (%2)").arg(x.value(QLatin1String("name")).toString(), x.value(QLatin1String("username")).toString()), x.value(QLatin1String("id")).toInteger());
    }
    fv->addWidget(Web::labelled(tr("Who"), who, form));
    auto *task = new QComboBox(form);
    task->addItem(tr("Translate"), QStringLiteral("translate"));
    task->addItem(tr("Review"), QStringLiteral("review"));
    fv->addWidget(Web::labelled(tr("Task"), task, form));
    auto *file = new QComboBox(form);
    file->addItem(tr("All files"), QString());
    for (const QJsonValue &v : s.value(QLatin1String("files")).toArray()) {
        if (v.toObject().value(QLatin1String("role")).toString() == QLatin1String("translatable"))
            file->addItem(v.toObject().value(QLatin1String("name")).toString(), v.toObject().value(QLatin1String("name")).toString());
    }
    fv->addWidget(Web::labelled(tr("File"), file, form));
    auto *lang = new QComboBox(form);
    fv->addWidget(Web::labelled(tr("Language"), lang, form));
    auto *from = new QComboBox(form);
    auto *to = new QComboBox(form);
    auto *fromField = Web::labelled(tr("From segment"), from, form);
    auto *toField = Web::labelled(tr("To segment"), to, form);
    auto *range = Web::row({fromField, toField}, form, 16);
    fv->addWidget(range);
    auto *due = new DueEdit(tr("Due"), form);
    fv->addWidget(Web::labelled(tr("Due (optional)"), due, form));
    auto *error = Web::text(QString(), "error", form);
    error->setVisible(false);
    fv->addWidget(error);
    auto *give = Web::button(tr("Give out"), Web::Primary, form, QStringLiteral("user-plus"));
    give->setEnabled(false);
    fv->addWidget(give, 0, Qt::AlignLeft);
    fv->addWidget(Web::text(tr("%1 in this project. Each segment has one translator and one reviewer.").arg(plural(m_assignments.size(), tr("1 piece of work"), tr("%1 pieces of work"))),
                            "cellSub", form));
    m_work->addWidget(form);

    const QStringList targets = strings(s.value(QLatin1String("targetLangs")));
    auto segs = std::make_shared<QStringList>();
    // A file made for one language is given out in that language.
    const auto fileLangs = [s, targets](const QString &name) {
        for (const QJsonValue &v : s.value(QLatin1String("files")).toArray()) {
            if (v.toObject().value(QLatin1String("name")).toString() == name && !v.toObject().value(QLatin1String("targetLang")).toString().isEmpty())
                return QStringList{v.toObject().value(QLatin1String("targetLang")).toString()};
        }
        return targets;
    };
    const auto showRange = [range, toField, from] {
        range->setVisible(from->count() > 1);
        toField->setVisible(!from->currentData().toString().isEmpty());
    };
    const auto loadSegments = [this, form, file, lang, from, to, segs, fileLangs, showRange] {
        const QString f = file->currentData().toString();
        const QStringList ls = fileLangs(f);
        const QString l = lang->currentData().toString().isEmpty() && ls.size() == 1 ? ls.first() : lang->currentData().toString();
        from->clear();
        to->clear();
        segs->clear();
        from->addItem(tr("The whole file"), QString());
        showRange();
        if (f.isEmpty() || l.isEmpty())
            return;
        onResult(m_conn->api()->get(QStringLiteral("/api/projects/%1/segments").arg(m_id), QUrlQuery{{QStringLiteral("file"), f}, {QStringLiteral("lang"), l}}), form,
                 [from, segs, showRange](const ApiResult &r) {
                     int i = 0;
                     for (const QJsonValue &v : r.array()) {
                         const QString id = v.toObject().value(QLatin1String("id")).toString();
                         *segs << id;
                         from->addItem(QString::number(++i), id);
                     }
                     showRange();
                 });
    };
    const auto fillLangs = [lang, file, fileLangs] {
        const QSignalBlocker block(lang);
        lang->clear();
        const QStringList ls = fileLangs(file->currentData().toString());
        lang->addItem(ls.size() == 1 ? Languages::name(ls.first()) : tr("All languages"), QString());
        if (ls.size() > 1) {
            for (const QString &t : ls)
                lang->addItem(Languages::name(t), t);
        }
    };
    fillLangs();
    loadSegments();
    connect(who, &QComboBox::currentIndexChanged, give, [give, who] { give->setEnabled(who->currentData().toLongLong() > 0); });
    connect(file, &QComboBox::currentIndexChanged, form, [fillLangs, loadSegments] {
        fillLangs();
        loadSegments();
    });
    connect(lang, &QComboBox::currentIndexChanged, form, loadSegments);
    connect(from, &QComboBox::currentIndexChanged, form, [from, to, segs, showRange] {
        const QString first = from->currentData().toString();
        to->clear();
        const int at = int(segs->indexOf(first));
        if (!first.isEmpty() && at >= 0) {
            for (int i = at; i < segs->size(); ++i)
                to->addItem(QString::number(i + 1), segs->at(i));
            to->setCurrentIndex(to->count() - 1);
        }
        showRange();
    });
    connect(give, &QPushButton::clicked, this, [this, who, task, file, lang, from, to, due, error] {
        QJsonObject a{{QStringLiteral("userId"), who->currentData().toLongLong()}, {QStringLiteral("task"), task->currentData().toString()}};
        if (!file->currentData().toString().isEmpty())
            a.insert(QStringLiteral("file"), file->currentData().toString());
        if (!lang->currentData().toString().isEmpty())
            a.insert(QStringLiteral("targetLang"), lang->currentData().toString());
        if (!from->currentData().toString().isEmpty()) {
            a.insert(QStringLiteral("fromSegment"), from->currentData().toString());
            a.insert(QStringLiteral("toSegment"), to->currentData().toString().isEmpty() ? from->currentData().toString() : to->currentData().toString());
        }
        if (!due->iso().isEmpty())
            a.insert(QStringLiteral("due"), due->iso());
        onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/assignments").arg(m_id), a), this, [this, error](const ApiResult &r) {
            if (!r.ok()) {
                if (error) {
                    error->setText(r.error.message);
                    error->setVisible(true);
                }
                return;
            }
            say(tr("Work given out"));
            reload();
        });
    });
}

void ProjectPage::fillNow()
{
    if (!m_now)
        return;
    clear(m_now);
    QWidget *p = m_nowPanel;
    if (m_presence.isEmpty()) {
        m_now->addWidget(Web::text(tr("Nobody has the project open."), "muted", p));
        return;
    }
    for (const QJsonValue &v : std::as_const(m_presence)) {
        const QJsonObject x = v.toObject();
        const QDateTime since = QDateTime::fromString(x.value(QLatin1String("since")).toString(), Qt::ISODate);
        m_now->addWidget(Web::cell(x.value(QLatin1String("userName")).toString(),
                                   tr("%1, %2, segment %3, since %4")
                                       .arg(x.value(QLatin1String("file")).toString(), x.value(QLatin1String("targetLang")).toString(), x.value(QLatin1String("segment")).toString(),
                                            since.isValid() ? Languages::when(since) : QString()),
                                   p));
    }
}

void ProjectPage::fillTemplate()
{
    // The name being typed is kept while the rest is shown again.
    QString typed;
    bool had = false;
    if (auto *old = m_templatePanel->findChild<QLineEdit *>(QStringLiteral("templateName"))) {
        typed = old->text();
        had = true;
        if (old->hasFocus() && m_template->count() > 0)
            return;
    }
    clear(m_template);
    QWidget *p = m_templatePanel;
    const QJsonObject s = settings();
    auto *name = new QLineEdit(had ? typed : s.value(QLatin1String("client")).toString().isEmpty() ? s.value(QLatin1String("name")).toString() : s.value(QLatin1String("client")).toString(),
                               p);
    name->setObjectName(QStringLiteral("templateName"));
    name->setAccessibleName(tr("Template name"));
    name->setMinimumWidth(320);
    auto *save = Web::button(QString(), Web::Normal, p);
    save->setObjectName(QStringLiteral("saveTemplate"));
    const auto label = [this, name, save] {
        bool clash = false;
        for (const QJsonValue &v : std::as_const(m_templates))
            clash = clash || v.toObject().value(QLatin1String("name")).toString().compare(name->text().trimmed(), Qt::CaseInsensitive) == 0;
        save->setText(clash ? tr("Replace template") : tr("Save as template"));
        save->setEnabled(!name->text().trimmed().isEmpty() && !m_dirty);
    };
    label();
    connect(name, &QLineEdit::textChanged, save, label);
    connect(save, &QPushButton::clicked, this, [this, name] { saveTemplate(name->text().trimmed()); });
    auto *field = Web::field(tr("Template name"), Web::row({name, save}, p), p);
    if (auto *l = field->findChild<QLabel *>())
        l->setBuddy(name);
    m_template->addWidget(field);
    m_template->addWidget(Web::text(m_dirty ? tr("Save your changes first; the template takes the saved settings.")
                                            : tr("Languages, memories, termbases, checks and pre-translation; not the files, name or due date. New projects can start from it."),
                                    "help", p));
}

void ProjectPage::saveTemplate(const QString &name)
{
    if (name.isEmpty())
        return;
    // Saving over a template of the same name needs its version.
    int version = 0;
    for (const QJsonValue &v : std::as_const(m_templates)) {
        if (v.toObject().value(QLatin1String("name")).toString().compare(name, Qt::CaseInsensitive) == 0)
            version = v.toObject().value(QLatin1String("version")).toInt();
    }
    onResult(m_conn->api()->put(QStringLiteral("/api/templates"), ProjectForm::templateOf(name, settings()), version), this, [this, name, version](const ApiResult &r) {
        say(!r.ok() ? r.error.message : version ? tr("Updated the template “%1”").arg(name) : tr("Saved the template “%1”").arg(name));
        onResult(m_conn->api()->get(QStringLiteral("/api/templates")), this, [this](const ApiResult &t) {
            if (t.ok())
                m_templates = t.array();
            fillTemplate();
        });
    });
}

void ProjectPage::download(const QString &path, const QUrlQuery &query, const QString &what)
{
    onResult(m_conn->api()->get(path, query), this, [this, what](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        const QString dir = QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation);
        const QString to = QFileDialog::getSaveFileName(this, tr("Save %1").arg(what), dir + QLatin1Char('/') + r.fileName());
        if (to.isEmpty())
            return;
        QSaveFile f(to);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size() || !f.commit()) {
            say(tr("%1 could not be saved: %2").arg(QDir::toNativeSeparators(to), f.errorString()));
            return;
        }
        say(tr("Saved %1").arg(QDir::toNativeSeparators(to)));
    });
}

// Files are added one after the other: each is read where the project is
// kept first, and what can't be translated goes in for reference.
void ProjectPage::addFiles(const QStringList &paths)
{
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
    if (files.isEmpty())
        return;
    auto added = std::make_shared<int>(0);
    auto next = std::make_shared<std::function<void(int)>>();
    *next = [this, files, added, next](int i) {
        if (i >= files.size()) {
            m_busy.clear();
            if (*added)
                say(tr("Added %1").arg(plural(*added, tr("1 file"), tr("%1 files"))));
            reload();
            return;
        }
        const auto &[path, name] = files.at(i);
        for (const QJsonValue &v : settings().value(QLatin1String("files")).toArray()) {
            if (v.toObject().value(QLatin1String("name")).toString().compare(name, Qt::CaseInsensitive) == 0) {
                say(tr("%1 is in the project already; remove it first to replace it").arg(name));
                (*next)(i + 1);
                return;
            }
        }
        QFile f(path);
        if (!f.open(QIODevice::ReadOnly)) {
            say(tr("%1 can't be read: %2").arg(QDir::toNativeSeparators(path), f.errorString()));
            (*next)(i + 1);
            return;
        }
        const QByteArray data = f.readAll();
        m_busy = tr("Reading %1…").arg(name);
        fillFiles();
        const QString fileName = name;
        onResult(m_conn->api()->upload("POST", QStringLiteral("/api/files/inspect"), data, QStringLiteral("application/octet-stream"), QUrlQuery{{QStringLiteral("name"), fileName}}), this,
                 [this, fileName, data, added, next, i](const ApiResult &in) {
                     if (!in.ok()) {
                         say(in.error.message);
                         (*next)(i + 1);
                         return;
                     }
                     if (in.object().value(QLatin1String("format")).toString() == QLatin1String("package")) {
                         say(tr("%1 is a package: a project is made from it, it is not added to one").arg(fileName));
                         (*next)(i + 1);
                         return;
                     }
                     QJsonObject body{{QStringLiteral("name"), fileName}, {QStringLiteral("data"), QString::fromLatin1(data.toBase64())}};
                     if (in.object().value(QLatin1String("translatable")).toBool()) {
                         body.insert(QStringLiteral("role"), QStringLiteral("translatable"));
                     } else {
                         body.insert(QStringLiteral("role"), QStringLiteral("reference"));
                         body.insert(QStringLiteral("format"), QFileInfo(fileName).suffix().toLower());
                     }
                     m_busy = tr("Adding %1…").arg(fileName);
                     fillFiles();
                     onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/files").arg(m_id), body,
                                                  QUrlQuery{{QStringLiteral("version"), QString::number(m_project.value(QLatin1String("version")).toInt())}}),
                              this, [this, added, next, i](const ApiResult &r) {
                                  if (r.ok()) {
                                      // The next file is added to the project as it is now.
                                      const QJsonObject access = m_project.value(QLatin1String("access")).toObject();
                                      m_project = r.object();
                                      if (!m_project.contains(QLatin1String("access")))
                                          m_project.insert(QStringLiteral("access"), access);
                                      ++*added;
                                  } else {
                                      say(r.error.message);
                                  }
                                  (*next)(i + 1);
                              });
                 });
    };
    (*next)(0);
}

void ProjectPage::removeFile(const QString &name)
{
    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1/files").arg(m_id), QUrlQuery{{QStringLiteral("name"), name}}, m_project.value(QLatin1String("version")).toInt()), this,
             [this, name](const ApiResult &r) {
                 say(r.ok() ? tr("Removed %1").arg(name) : r.error.message);
                 reload();
             });
}

void ProjectPage::discard()
{
    m_dirty = false;
    m_saveError.clear();
    m_loaded = false;
    m_form->setSettings(settings(), false);
    m_form->setReadOnly(!manages());
    m_loaded = true;
    refreshBar();
    fillPretranslate();
    fillTemplate();
}

// Saving: the memories made here first, then the project termbase where
// the masters changed, then the settings themselves.
void ProjectPage::save()
{
    if (m_saving)
        return;
    m_saving = true;
    m_saveError.clear();
    refreshBar();
    const auto failed = [this](const QString &why) {
        m_saving = false;
        m_saveError = tr("Could not save: %1").arg(why);
        refreshBar();
    };
    m_form->createNewMemories(m_conn, [this, failed](const QString &error) {
        if (!error.isEmpty()) {
            failed(error);
            return;
        }
        QJsonObject s = m_form->settings();
        const QJsonObject was = settings();
        // Files are added and removed on their own.
        s.insert(QStringLiteral("files"), was.value(QLatin1String("files")));
        QJsonObject t = s.value(QLatin1String("terminology")).toObject();
        const QString mode = t.value(QLatin1String("mode")).toString();
        const QJsonArray masters = t.value(QLatin1String("masters")).toArray();
        QString termbase = mode == QLatin1String("direct") ? QString() : was.value(QLatin1String("terminology")).toObject().value(QLatin1String("termbase")).toString();
        if (mode == QLatin1String("direct")) {
            t.remove(QStringLiteral("termbase"));
            s.insert(QStringLiteral("terminology"), t);
            putSettings(s);
        } else if (termbase.isEmpty()) {
            // A project that now keeps its own terms gets its termbase.
            termbase = QStringLiteral("%1 (project)").arg(s.value(QLatin1String("name")).toString());
            QJsonObject def{{QStringLiteral("name"), termbase}};
            QJsonObject body{{QStringLiteral("definition"), def}};
            if (mode == QLatin1String("layered")) {
                body.insert(QStringLiteral("upstreams"), masters);
            } else {
                QJsonArray langs{s.value(QLatin1String("sourceLang"))};
                for (const QJsonValue &l : s.value(QLatin1String("targetLangs")).toArray())
                    langs.append(l);
                def.insert(QStringLiteral("languages"), langs);
                body.insert(QStringLiteral("definition"), def);
            }
            onResult(m_conn->api()->post(QStringLiteral("/api/termbases"), body), this, [this, s, t, termbase, failed](const ApiResult &r) mutable {
                if (!r.ok() && r.error.code != QLatin1String("exists")) {
                    failed(r.error.message);
                    return;
                }
                t.insert(QStringLiteral("termbase"), termbase);
                s.insert(QStringLiteral("terminology"), t);
                putSettings(s);
            });
        } else if (mode == QLatin1String("layered") && masters != was.value(QLatin1String("terminology")).toObject().value(QLatin1String("masters")).toArray()) {
            // A layered one follows the masters chosen.
            onResult(m_conn->api()->put(QStringLiteral("/api/termbases/%1/upstreams").arg(ApiClient::seg(termbase)), QJsonObject{{QStringLiteral("upstreams"), masters}}), this,
                     [this, s, t, termbase, failed](const ApiResult &r) mutable {
                         if (!r.ok()) {
                             failed(r.error.message);
                             return;
                         }
                         t.insert(QStringLiteral("termbase"), termbase);
                         s.insert(QStringLiteral("terminology"), t);
                         putSettings(s);
                     });
        } else {
            t.insert(QStringLiteral("termbase"), termbase);
            s.insert(QStringLiteral("terminology"), t);
            putSettings(s);
        }
    });
}

void ProjectPage::putSettings(QJsonObject s)
{
    const QStringList before = strings(settings().value(QLatin1String("targetLangs")));
    onResult(m_conn->api()->put(QStringLiteral("/api/projects/%1/settings").arg(m_id), s, m_project.value(QLatin1String("version")).toInt()), this,
             [this, before](const ApiResult &r) {
                 m_saving = false;
                 if (!r.ok()) {
                     const bool conflict = r.error.code == QLatin1String("conflict");
                     const QStringList rest = m_form->showProblems(r.error.problems);
                     m_saveError = conflict ? tr("The project changed in the meantime. It is loaded again; make your change once more.")
                         : r.error.problems.isEmpty() ? tr("Could not save: %1").arg(r.error.message)
                         : rest.isEmpty()             ? tr("Could not save: see what is marked above.")
                                                      : tr("Could not save: %1").arg(rest.join(QLatin1Char(' ')));
                     if (conflict) {
                         m_dirty = false;
                         reload();
                     }
                     refreshBar();
                     return;
                 }
                 m_dirty = false;
                 m_saveError.clear();
                 const QJsonObject access = m_project.value(QLatin1String("access")).toObject();
                 m_project = r.object();
                 if (!m_project.contains(QLatin1String("access")))
                     m_project.insert(QStringLiteral("access"), access);
                 const QJsonObject s = settings();
                 QStringList fresh;
                 for (const QString &t : strings(s.value(QLatin1String("targetLangs")))) {
                     if (!before.contains(t))
                         fresh << Languages::name(t);
                 }
                 say(fresh.isEmpty() ? tr("Saved the settings of “%1”").arg(s.value(QLatin1String("name")).toString())
                                     : tr("Saved the settings of “%1”; %2 added to every file").arg(s.value(QLatin1String("name")).toString(), fresh.join(QStringLiteral(", "))));
                 // The analysis was of the settings before.
                 m_analysis = {};
                 showProject();
                 reload();
             });
}

void ProjectPage::deleteProject()
{
    const QString name = settings().value(QLatin1String("name")).toString();
    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1").arg(m_id)), this, [this, name](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        leave();
    });
}
