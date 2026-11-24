#include "projectspage.h"

#include "roles.h"

#include "editorpage.h"

#include <QTimer>

#include "connectionmanager.h"
#include "newprojectdialog.h"
#include "projectpage.h"
#include "projectsmodel.h"

#include "apiclient.h"
#include "connection.h"
#include "icons.h"
#include "theme.h"
#include "webui.h"

#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QDateTime>
#include <QDir>
#include <QDirIterator>
#include <QFileDialog>
#include <QFileInfo>
#include <QJsonArray>
#include <QLocale>
#include <QPointer>
#include <QScrollArea>
#include <QSet>
#include <QStackedWidget>
#include <QVBoxLayout>

ProjectsPage::ProjectsPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent), m_manager(manager), m_model(new ProjectsModel(manager, this)), m_stack(new QStackedWidget(this))
{
    setObjectName(QStringLiteral("projects"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->addWidget(m_stack);

    // As the web's Projects page: the header, the drop zone, and a table
    // for each place projects are kept.
    Web::Page page = Web::page(m_stack);
    page.body->addWidget(Web::header(tr("Projects"), QString(), {}, page.scroll->widget()));

    auto *zone = new Web::DropZone(page.scroll->widget());
    zone->setAccessibleName(tr("Start a project from files"));
    auto *zl = new QHBoxLayout(zone);
    zl->setContentsMargins(24, 24, 24, 24);
    zl->setSpacing(16);
    auto *icon = new QLabel(zone);
    icon->setPixmap(Icons::get(QStringLiteral("upload"), Theme::color("accent")).pixmap(22, 22));
    zl->addWidget(icon);
    auto *zt = new QVBoxLayout;
    zt->setSpacing(2);
    auto *zh = Web::text(tr("Start a project from files"), "sectionTitle16", zone);
    zt->addWidget(zh);
    zt->addWidget(Web::text(tr("Drop files, a folder, or a Trados or memoQ package here. Interlinea detects the source language and suggests memories and termbases for the client."),
                            "secondary", zone));
    zl->addLayout(zt, 1);
    auto *choose = Web::button(tr("Choose files"), Web::Primary, zone);
    auto *folder = Web::button(tr("Choose a folder"), Web::Normal, zone, QStringLiteral("folder-open"));
    auto *blank = Web::button(tr("New project without files"), Web::Quiet, zone);
    auto *zb = new QHBoxLayout;
    zb->setSpacing(8);
    zb->addWidget(choose);
    zb->addWidget(folder);
    zb->addWidget(blank);
    zl->addLayout(zb);
    page.body->addWidget(zone);
    connect(zone, &Web::DropZone::dropped, this, [this](const QStringList &paths) { newProject(expand(paths)); });
    connect(choose, &QPushButton::clicked, this, [this] {
        const QStringList files = QFileDialog::getOpenFileNames(this, tr("Choose files"));
        if (!files.isEmpty())
            newProject(files);
    });
    connect(folder, &QPushButton::clicked, this, [this] {
        const QString dir = QFileDialog::getExistingDirectory(this, tr("Choose a folder"));
        if (!dir.isEmpty())
            newProject(expand({dir}));
    });
    connect(blank, &QPushButton::clicked, this, [this] { newProject(); });

    m_sections = new QVBoxLayout;
    m_sections->setSpacing(24);
    page.body->addLayout(m_sections);
    page.body->addStretch();
    m_stack->addWidget(page.scroll);

    connect(m_model, &ProjectsModel::loaded, this, &ProjectsPage::updateState);
    updateState();
}

QStringList ProjectsPage::expand(const QStringList &paths)
{
    // A folder brings its files, with those of its subfolders.
    QStringList out;
    for (const QString &p : paths) {
        const QFileInfo fi(p);
        if (!fi.isDir()) {
            out << p;
            continue;
        }
        QDirIterator it(p, QDir::Files | QDir::NoDotAndDotDot, QDirIterator::Subdirectories);
        while (it.hasNext())
            out << it.next();
    }
    return out;
}

void ProjectsPage::openInEditor(const QString &connectionId, qint64 projectId, const QJsonObject &settings)
{
    // The first file to translate, in its first language, as the web
    // editor opens a project.
    QString file, lang = settings.value(QLatin1String("targetLangs")).toArray().first().toString();
    for (const QJsonValue &v : settings.value(QLatin1String("files")).toArray()) {
        const QJsonObject f = v.toObject();
        if (f.value(QLatin1String("role")).toString() != QLatin1String("translatable"))
            continue;
        file = f.value(QLatin1String("name")).toString();
        if (!f.value(QLatin1String("targetLang")).toString().isEmpty())
            lang = f.value(QLatin1String("targetLang")).toString();
        break;
    }
    open(connectionId, projectId);
    if (!file.isEmpty() && !lang.isEmpty())
        edit(file, lang);
}

void ProjectsPage::updateState()
{
    clearLayout(m_sections);
    QWidget *parent = m_sections->parentWidget();
    for (Connection *c : m_manager->all()) {
        if (!c->isReady() && m_model->error(c->id()).isEmpty())
            continue;
        const bool local = c->isLocal();
        QVBoxLayout *body = nullptr;
        QString title = local ? tr("On this computer") : tr("On the team server");
        if (!local && m_manager->all().size() > 2)
            title = tr("On the team server %1").arg(c->displayName());
        m_sections->addWidget(Web::section(title, parent, &body));
        if (const QString e = m_model->error(c->id()); !e.isEmpty()) {
            auto *err = Web::text(e, "error", parent);
            body->addWidget(err);
        }
        QList<int> rows;
        for (int r = 0; r < m_model->rowCount(); ++r) {
            if (m_model->index(r, 0).data(ProjectsModel::ConnectionRole).toString() == c->id())
                rows << r;
        }
        if (rows.isEmpty()) {
            body->addWidget(Web::text(local ? tr("No projects yet. Start one from files above.")
                                            : tr("No projects on %1 for you yet. Projects you make there, or work given to you, appear here.").arg(c->displayName()),
                                      "muted", parent));
            continue;
        }
        auto *table = local ? new Web::Table({tr("Project"), tr("Languages"), tr("Terminology"), tr("Words"), tr("Done"), tr("Due"), QString()}, parent)
                            : new Web::Table({tr("Project"), tr("Languages"), tr("Your work"), tr("Done"), tr("Due"), QString()}, parent);
        table->setAccessibleName(title);
        table->setNumeric(local ? QList<int>{3, 4} : QList<int>{3});
        table->setStretch(0);
        for (int r : rows) {
            const QModelIndex i = m_model->index(r, 0);
            const QJsonObject p = i.data(ProjectsModel::ProjectRole).toJsonObject();
            const QJsonObject s = p.value(QLatin1String("settings")).toObject();
            const QJsonObject access = i.data(ProjectsModel::AccessRole).toJsonObject();
            const qint64 id = p.value(QLatin1String("id")).toInteger();
            const QString cid = c->id();
            const QString name = s.value(QLatin1String("name")).toString();
            int translatable = 0, refs = 0;
            QString firstFile;
            for (const QJsonValue &f : s.value(QLatin1String("files")).toArray()) {
                if (f.toObject().value(QLatin1String("role")).toString() == QLatin1String("translatable")) {
                    if (!translatable)
                        firstFile = QFileInfo(f.toObject().value(QLatin1String("name")).toString()).fileName();
                    ++translatable;
                } else {
                    ++refs;
                }
            }
            QString filesLine = translatable == 1 ? firstFile : (translatable == 0 ? tr("No files") : tr("%1 files").arg(translatable));
            if (refs)
                filesLine += refs == 1 ? tr(", 1 reference file") : tr(", %1 reference files").arg(refs);
            if (!local && !s.value(QLatin1String("client")).toString().isEmpty())
                filesLine = s.value(QLatin1String("client")).toString() + QStringLiteral(", ") + filesLine;
            QWidget *title = Web::cell(name, filesLine, table, [this, cid, id, s] { openInEditor(cid, id, s); });
            QStringList targets = s.value(QLatin1String("targetLangs")).toVariant().toStringList();
            QWidget *langs = Web::langPair(s.value(QLatin1String("sourceLang")).toString(), targets, table);
            const int segments = p.value(QLatin1String("segments")).toInt();
            const int confirmed = p.value(QLatin1String("confirmed")).toInt();
            QWidget *done = Web::cellText(QStringLiteral("%1%").arg(segments ? qRound(100.0 * confirmed / segments) : 0), table, true);
            const QString dueText = s.value(QLatin1String("due")).toString();
            QWidget *due = dueText.isEmpty() ? Web::cellText(tr("None"), table, false, true)
                                             : Web::cellText(QLocale().toString(QDateTime::fromString(dueText, Qt::ISODate).toLocalTime().date(), QStringLiteral("d MMM yyyy")), table);
            auto *settingsButton = Web::button(tr("Settings"), Web::Quiet | Web::Small, table, QStringLiteral("settings"));
            settingsButton->setAccessibleName(tr("Settings of %1").arg(name));
            connect(settingsButton, &QPushButton::clicked, this, [this, cid, id] { open(cid, id); });
            if (local) {
                const QJsonObject t = s.value(QLatin1String("terminology")).toObject();
                const QString mode = t.value(QLatin1String("mode")).toString();
                const QStringList masters = t.value(QLatin1String("masters")).toVariant().toStringList();
                auto *term = new QWidget(table);
                auto *tv = new QVBoxLayout(term);
                tv->setContentsMargins(0, 0, 0, 0);
                tv->setSpacing(2);
                const QString kind = mode == QLatin1String("direct") ? tr("Writes to master") : mode == QLatin1String("own") ? tr("Own termbase") : tr("Project termbase");
                auto *kt = new QLabel(kind, term);
                QFont f = kt->font();
                f.setWeight(QFont::DemiBold);
                kt->setFont(f);
                tv->addWidget(kt);
                auto *sub = new QHBoxLayout;
                sub->setSpacing(4);
                QString line = masters.isEmpty() ? tr("No master")
                                                 : (mode == QLatin1String("direct") ? QString() : tr("Based on") + QLatin1Char(' ')) + masters.join(QLatin1String(", "));
                sub->addWidget(Web::text(line, "cellSub", term));
                // On this computer everything is local, even what the core
                // calls its server.
                sub->addWidget(Web::locationBadge(false, term));
                sub->addStretch();
                tv->addLayout(sub);
                term->setAccessibleName(QStringLiteral("%1, %2").arg(kind, line));
                auto *words = Web::cellText(QStringLiteral("…"), table, true);
                QPointer<QLabel> wl(qobject_cast<QLabel *>(words));
                onResult(c->api()->get(QStringLiteral("/api/projects/%1/progress").arg(id)), this, [wl](const ApiResult &pr) {
                    if (!wl || !pr.ok())
                        return;
                    // Each file's words count once, in its first language.
                    QSet<QString> seen;
                    int total = 0;
                    for (const QJsonValue &v : pr.array()) {
                        const QJsonObject fp = v.toObject();
                        if (seen.contains(fp.value(QLatin1String("file")).toString()))
                            continue;
                        seen << fp.value(QLatin1String("file")).toString();
                        total += fp.value(QLatin1String("words")).toInt();
                    }
                    wl->setText(QLocale().toString(total));
                    wl->setAccessibleName(wl->text());
                });
                QList<QWidget *> actions;
                if (mode == QLatin1String("layered") && !t.value(QLatin1String("termbase")).toString().isEmpty()) {
                    const QString tb = t.value(QLatin1String("termbase")).toString();
                    auto *review = Web::button(tr("Review and merge"), Web::Small, table, QStringLiteral("git-merge"));
                    review->hide();
                    QPointer<QPushButton> rp(review);
                    onResult(c->api()->get(QStringLiteral("/api/termbases/%1/layered").arg(ApiClient::seg(tb))), this, [rp, tb, table](const ApiResult &lr) {
                        if (!rp || !lr.ok())
                            return;
                        int own = 0;
                        for (const QJsonValue &v : lr.array()) {
                            if (v.toObject().value(QLatin1String("termbase")).toString() == tb)
                                ++own;
                        }
                        rp->setText(tr("Review and merge (%1)").arg(own));
                        rp->setVisible(own > 0);
                        table->fit();
                    });
                    const QString connId = c->id();
                    connect(review, &QPushButton::clicked, this, [this, connId, tb] { emit navigate(QStringLiteral("terminology"), connId, tb); });
                    actions << review;
                }
                actions << settingsButton;
                table->addRow({title, langs, term, words, done, due, Web::cellActions(actions, table)});
            } else {
                auto *work = Web::cellText(Roles::allows(access.value(QLatin1String("role")).toString(), QStringLiteral("manage")) ? tr("Managing")
                                                                                                                  : tr("No work given to you"),
                                           table, false, !Roles::allows(access.value(QLatin1String("role")).toString(), QStringLiteral("manage")));
                QPointer<QLabel> wk(qobject_cast<QLabel *>(work));
                onResult(c->api()->get(QStringLiteral("/api/me/assignments")), this, [wk, id, table](const ApiResult &ar) {
                    if (!wk || !ar.ok())
                        return;
                    QStringList tasks, scopes;
                    bool allDone = true;
                    for (const QJsonValue &v : ar.array()) {
                        const QJsonObject a = v.toObject();
                        if (a.value(QLatin1String("projectId")).toInteger() != id)
                            continue;
                        const QString task = a.value(QLatin1String("task")).toString() == QLatin1String("translate") ? tr("Translating") : tr("Reviewing");
                        if (!tasks.contains(task))
                            tasks << task;
                        QStringList parts{a.value(QLatin1String("file")).toString().isEmpty() ? tr("all files") : a.value(QLatin1String("file")).toString(),
                                          a.value(QLatin1String("targetLang")).toString().isEmpty() ? tr("all languages") : a.value(QLatin1String("targetLang")).toString()};
                        if (!a.value(QLatin1String("fromSegment")).toString().isEmpty())
                            parts << (a.value(QLatin1String("fromSegment")) == a.value(QLatin1String("toSegment"))
                                          ? tr("segment %1").arg(a.value(QLatin1String("fromSegment")).toString())
                                          : tr("segments %1 to %2").arg(a.value(QLatin1String("fromSegment")).toString(), a.value(QLatin1String("toSegment")).toString()));
                        scopes << parts.join(QLatin1String(", "));
                        allDone = allDone && a.value(QLatin1String("done")).toBool();
                    }
                    if (tasks.isEmpty())
                        return;
                    wk->setText(QStringLiteral("<b>%1</b><br><span style=\"color:%2; font-size:13px\">%3%4</span>")
                                    .arg(tasks.join(QLatin1String(", ")), Theme::color("text3").name(), scopes.join(QLatin1String("; ")).toHtmlEscaped(),
                                         allDone ? tr(", done") : QString()));
                    wk->setStyleSheet(QString());
                    table->fit();
                });
                table->addRow({title, langs, work, done, due, Web::cellActions({settingsButton}, table)});
            }
        }
        table->fit();
        body->addWidget(table);
    }
}

void ProjectsPage::clearLayout(QLayout *layout)
{
    while (QLayoutItem *item = layout->takeAt(0)) {
        if (QWidget *w = item->widget()) {
            w->hide();
            w->deleteLater();
        } else if (QLayout *l = item->layout()) {
            clearLayout(l);
        }
        delete item;
    }
}

void ProjectsPage::open(const QString &connectionId, qint64 projectId)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        return;
    closeEditor();
    if (m_project) {
        m_stack->removeWidget(m_project);
        m_project->deleteLater();
    }
    m_project = new ProjectPage(c, projectId, m_stack);
    m_stack->addWidget(m_project);
    m_stack->setCurrentWidget(m_project);
    connect(m_project, &ProjectPage::back, this, &ProjectsPage::showList);
    connect(m_project, &ProjectPage::openEditor, this, &ProjectsPage::edit);
    connect(m_project, &ProjectPage::gone, this, [this] {
        showList();
        if (Connection *conn = m_project ? m_project->connection() : nullptr)
            m_model->reload(conn);
    });
    m_project->setFocus();
}

void ProjectsPage::edit(const QString &file, const QString &lang)
{
    if (!m_project || !m_project->connection())
        return;
    closeEditor();
    m_editor = new EditorPage(m_project->connection(), m_project->projectId(), file, lang, m_stack);
    m_stack->addWidget(m_editor);
    m_stack->setCurrentWidget(m_editor);
    connect(m_editor, &EditorPage::back, this, &ProjectsPage::showList);
    connect(m_editor, &EditorPage::openProject, this, [this] {
        closeEditor();
        if (m_project) {
            m_stack->setCurrentWidget(m_project);
            m_project->setFocus();
        }
    });
    connect(m_editor, &EditorPage::openFile, this, [this](const QString &f, const QString &l) {
        QTimer::singleShot(0, this, [this, f, l] { edit(f, l); });
    });
    connect(m_editor, &EditorPage::openReview, this, [this](const QString &termbase) {
        emit navigate(QStringLiteral("terminology"), m_project && m_project->connection() ? m_project->connection()->id() : QString(), termbase);
    });
    m_editor->grid()->setFocus();
}

void ProjectsPage::closeEditor()
{
    if (!m_editor)
        return;
    m_editor->saveActive();
    m_stack->removeWidget(m_editor);
    // Hidden at once: until it is deleted, its keys would clash with the
    // next editor's.
    m_editor->hide();
    m_editor->setEnabled(false);
    m_editor->deleteLater();
    m_editor = nullptr;
}

void ProjectsPage::showList()
{
    closeEditor();
    m_stack->setCurrentIndex(0);
}

void ProjectsPage::newProject(const QStringList &files)
{
    NewProjectDialog dlg(m_manager, this);
    if (!files.isEmpty())
        dlg.addFiles(files);
    if (dlg.exec() != QDialog::Accepted)
        return;
    if (Connection *c = m_manager->find(dlg.connectionId()))
        m_model->reload(c);
    open(dlg.connectionId(), dlg.projectId());
}
