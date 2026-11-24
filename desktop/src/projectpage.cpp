#include "projectpage.h"

#include "apiclient.h"
#include "connection.h"
#include "eventfeed.h"
#include "languages.h"
#include "progressdelegate.h"
#include "settingseditor.h"

#include "theme.h"

#include <QCheckBox>
#include <QComboBox>
#include <QDateTimeEdit>
#include <QDialog>
#include <QDialogButtonBox>
#include <QDir>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QFormLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QInputDialog>
#include <QJsonDocument>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QMessageBox>
#include <QPushButton>
#include <QSaveFile>
#include <QSpinBox>
#include <QStandardPaths>
#include <QTabWidget>
#include <QTableWidget>
#include <QTreeWidget>
#include <QVBoxLayout>

namespace {

QTableWidgetItem *cell(const QString &text, const QVariant &data = {})
{
    auto *i = new QTableWidgetItem(text);
    i->setFlags(i->flags() & ~Qt::ItemIsEditable);
    if (data.isValid())
        i->setData(Qt::UserRole, data);
    return i;
}

QString roleName(const QString &role)
{
    if (role == QLatin1String("read"))
        return ProjectPage::tr("Reads");
    if (role == QLatin1String("suggest"))
        return ProjectPage::tr("Suggests");
    if (role == QLatin1String("edit"))
        return ProjectPage::tr("Edits");
    if (role == QLatin1String("approve"))
        return ProjectPage::tr("Approves");
    if (role == QLatin1String("manage"))
        return ProjectPage::tr("Manages");
    return role;
}

const QStringList roles{QStringLiteral("read"), QStringLiteral("suggest"), QStringLiteral("edit"), QStringLiteral("approve"), QStringLiteral("manage")};

} // namespace

ProjectPage::ProjectPage(Connection *connection, qint64 projectId, QWidget *parent)
    : QWidget(parent), m_conn(connection), m_id(projectId)
{
    setObjectName(QStringLiteral("project"));
    auto *layout = new QVBoxLayout(this);
    layout->setContentsMargins(28, 18, 28, 20);
    auto *head = new QHBoxLayout;
    auto *backButton = new QPushButton(tr("← &Projects"), this);
    backButton->setFlat(true);
    backButton->setAccessibleName(tr("Back to the projects"));
    head->addWidget(backButton);
    m_title = new QLabel(this);
    Theme::title(m_title);
    head->addWidget(m_title, 1);
    m_delete = new QPushButton(tr("&Delete project…"), this);
    head->addWidget(m_delete);
    layout->addLayout(head);
    m_meta = new QLabel(this);
    m_meta->setWordWrap(true);
    Theme::secondary(m_meta);
    layout->addWidget(m_meta);
    m_message = new QLabel(this);
    m_message->setWordWrap(true);
    m_message->setAccessibleName(tr("Status"));
    layout->addWidget(m_message);
    m_tabs = new QTabWidget(this);
    m_tabs->addTab(filesTab(), tr("&Files"));
    m_tabs->addTab(settingsTab(), tr("&Settings"));
    m_tabs->addTab(analysisTab(), tr("&Analysis"));
    if (!connection->isLocal()) {
        m_peopleTab = peopleTab();
        m_tabs->addTab(m_peopleTab, tr("&People and work"));
    }
    layout->addWidget(m_tabs, 1);

    connect(backButton, &QPushButton::clicked, this, &ProjectPage::back);
    connect(m_delete, &QPushButton::clicked, this, &ProjectPage::deleteProject);
    m_reload.setSingleShot(true);
    m_reload.setInterval(400);
    connect(&m_reload, &QTimer::timeout, this, &ProjectPage::reload);
    connect(connection->feed(), &EventFeed::event, this, [this](const QString &type, const QJsonObject &data) {
        if (type == QLatin1String("project") && data.value(QLatin1String("projectId")).toInteger() == m_id)
            m_reload.start();
        if (type == QLatin1String("account"))
            m_reload.start();
    });
    connect(connection->feed(), &EventFeed::reset, &m_reload, qOverload<>(&QTimer::start));
    reload();
}

QWidget *ProjectPage::filesTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    m_files = new QTreeWidget(w);
    m_files->setAccessibleName(tr("Files and their progress"));
    m_files->setHeaderLabels({tr("File"), tr("Language"), tr("Confirmed"), tr("Words"), tr("Segments")});
    m_files->header()->setSectionResizeMode(QHeaderView::ResizeToContents);
    m_files->header()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_files->header()->setStretchLastSection(false);
    m_files->setRootIsDecorated(true);
    m_files->setItemDelegateForColumn(2, new ProgressDelegate(m_files));
    l->addWidget(m_files, 1);
    auto *row = new QHBoxLayout;
    m_openEditor = new QPushButton(tr("Tra&nslate"), w);
    m_openEditor->setToolTip(tr("Opens the file in the editor, in the language chosen."));
    Theme::primary(m_openEditor);
    m_saveTranslated = new QPushButton(tr("Save &translated file…"), w);
    m_returnPackage = new QPushButton(tr("Save &return package…"), w);
    m_addFiles = new QPushButton(tr("A&dd files…"), w);
    m_removeFile = new QPushButton(tr("Re&move file…"), w);
    m_pretranslate = new QPushButton(tr("Fill from &memories…"), w);
    m_pretranslate->setToolTip(tr("Fills empty segments with the best memory match, as the preparation settings say."));
    for (QPushButton *b : {m_openEditor, m_saveTranslated, m_returnPackage, m_addFiles, m_removeFile, m_pretranslate})
        row->addWidget(b);
    row->addStretch();
    l->addLayout(row);
    connect(m_openEditor, &QPushButton::clicked, this, [this] {
        if (!currentFile().isEmpty() && !currentLang().isEmpty())
            emit openEditor(currentFile(), currentLang());
    });
    connect(m_files, &QTreeWidget::itemActivated, this, [this] {
        if (!currentFile().isEmpty() && !currentLang().isEmpty())
            emit openEditor(currentFile(), currentLang());
    });
    connect(m_saveTranslated, &QPushButton::clicked, this, &ProjectPage::saveTranslated);
    connect(m_returnPackage, &QPushButton::clicked, this, &ProjectPage::saveReturnPackage);
    connect(m_addFiles, &QPushButton::clicked, this, &ProjectPage::addFiles);
    connect(m_removeFile, &QPushButton::clicked, this, &ProjectPage::removeFile);
    connect(m_pretranslate, &QPushButton::clicked, this, &ProjectPage::pretranslate);
    connect(m_files, &QTreeWidget::currentItemChanged, this, [this] {
        m_saveTranslated->setEnabled(!currentFile().isEmpty());
        m_openEditor->setEnabled(!currentFile().isEmpty() && !currentLang().isEmpty());
        m_removeFile->setEnabled(manages() && !currentFile().isEmpty());
    });
    return w;
}

QWidget *ProjectPage::settingsTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    m_editor = new SettingsEditor(w);
    l->addWidget(m_editor, 1);
    auto *row = new QHBoxLayout;
    m_saveSettings = new QPushButton(tr("Sa&ve settings"), w);
    Theme::primary(m_saveSettings);
    m_saveTemplate = new QPushButton(tr("Save as te&mplate…"), w);
    row->addWidget(m_saveSettings);
    row->addWidget(m_saveTemplate);
    row->addStretch();
    l->addLayout(row);
    connect(m_editor, &SettingsEditor::changed, this, [this] { m_settingsDirty = true; });
    connect(m_saveSettings, &QPushButton::clicked, this, &ProjectPage::saveSettings);
    connect(m_saveTemplate, &QPushButton::clicked, this, &ProjectPage::saveTemplate);
    return w;
}

QWidget *ProjectPage::analysisTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    auto *row = new QHBoxLayout;
    m_analysisLang = new QComboBox(w);
    m_analysisLang->setAccessibleName(tr("Target language"));
    auto *ll = new QLabel(tr("&Language:"), w);
    ll->setBuddy(m_analysisLang);
    auto *run = new QPushButton(tr("&Analyse"), w);
    Theme::primary(run);
    m_saveAnalysis = new QPushButton(tr("Save as &CSV…"), w);
    m_saveAnalysis->setEnabled(false);
    row->addWidget(ll);
    row->addWidget(m_analysisLang, 1);
    row->addWidget(run);
    row->addWidget(m_saveAnalysis);
    l->addLayout(row);
    auto *intro = new QLabel(tr("Counts the words to translate by how well the project's memories match them."), w);
    intro->setWordWrap(true);
    Theme::secondary(intro);
    l->addWidget(intro);
    m_analysisTable = new QTableWidget(0, 5, w);
    m_analysisTable->setAccessibleName(tr("Analysis"));
    m_analysisTable->setHorizontalHeaderLabels({tr("Match"), tr("Segments"), tr("Words"), tr("Characters"), tr("Share of words")});
    m_analysisTable->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_analysisTable->verticalHeader()->setVisible(false);
    l->addWidget(m_analysisTable, 1);
    connect(run, &QPushButton::clicked, this, &ProjectPage::runAnalysis);
    connect(m_analysisLang, &QComboBox::currentIndexChanged, this, [this] {
        // Shows the report of the language chosen, if analysed.
        m_analysisTable->setRowCount(0);
        const QString lang = m_analysisLang->currentData().toString();
        for (const QJsonValue &v : std::as_const(m_analysis)) {
            const QJsonObject rep = v.toObject();
            if (rep.value(QLatin1String("targetLang")).toString() != lang)
                continue;
            const int total = rep.value(QLatin1String("total")).toObject().value(QLatin1String("words")).toInt();
            QJsonArray rows = rep.value(QLatin1String("rows")).toArray();
            rows.append(QJsonObject{{QStringLiteral("name"), tr("Total")}, {QStringLiteral("segments"), rep.value(QLatin1String("total")).toObject().value(QLatin1String("segments"))},
                                    {QStringLiteral("words"), total}, {QStringLiteral("characters"), rep.value(QLatin1String("total")).toObject().value(QLatin1String("characters"))}});
            for (const QJsonValue &rv : rows) {
                const QJsonObject r = rv.toObject();
                const int i = m_analysisTable->rowCount();
                m_analysisTable->insertRow(i);
                const int words = r.value(QLatin1String("words")).toInt();
                m_analysisTable->setItem(i, 0, cell(r.value(QLatin1String("name")).toString()));
                m_analysisTable->setItem(i, 1, cell(QLocale().toString(r.value(QLatin1String("segments")).toInt())));
                m_analysisTable->setItem(i, 2, cell(QLocale().toString(words)));
                m_analysisTable->setItem(i, 3, cell(QLocale().toString(r.value(QLatin1String("characters")).toInt())));
                m_analysisTable->setItem(i, 4, cell(total ? QStringLiteral("%1 %").arg(QLocale().toString(100.0 * words / total, 'f', 1)) : QString()));
                for (int c = 1; c < 5; ++c)
                    m_analysisTable->item(i, c)->setTextAlignment(Qt::AlignRight | Qt::AlignVCenter);
            }
        }
        m_saveAnalysis->setEnabled(m_analysisTable->rowCount() > 0);
    });
    connect(m_saveAnalysis, &QPushButton::clicked, this, &ProjectPage::saveAnalysis);
    return w;
}

QWidget *ProjectPage::peopleTab()
{
    auto *w = new QWidget(this);
    auto *l = new QVBoxLayout(w);
    auto *gl = new QLabel(tr("&Who may see and change the project:"), w);
    m_grants = new QTableWidget(0, 2, w);
    gl->setBuddy(m_grants);
    m_grants->setAccessibleName(tr("Roles on the project"));
    m_grants->setHorizontalHeaderLabels({tr("Person"), tr("Role")});
    m_grants->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_grants->verticalHeader()->setVisible(false);
    m_grants->setSelectionBehavior(QAbstractItemView::SelectRows);
    l->addWidget(gl);
    l->addWidget(m_grants, 1);
    auto *grow = new QHBoxLayout;
    auto *addGrant = new QPushButton(tr("&Give a role…"), w);
    auto *removeGrant = new QPushButton(tr("Take the role &back"), w);
    grow->addWidget(addGrant);
    grow->addWidget(removeGrant);
    grow->addStretch();
    l->addLayout(grow);
    auto *wl = new QLabel(tr("&Work given out:"), w);
    m_work = new QTableWidget(0, 6, w);
    wl->setBuddy(m_work);
    m_work->setAccessibleName(tr("Work given out"));
    m_work->setHorizontalHeaderLabels({tr("Person"), tr("Task"), tr("File"), tr("Language"), tr("Segments"), tr("Due")});
    m_work->horizontalHeader()->setSectionResizeMode(2, QHeaderView::Stretch);
    m_work->verticalHeader()->setVisible(false);
    m_work->setSelectionBehavior(QAbstractItemView::SelectRows);
    l->addWidget(wl);
    l->addWidget(m_work, 2);
    auto *wrow = new QHBoxLayout;
    auto *assign = new QPushButton(tr("Give out &work…"), w);
    auto *done = new QPushButton(tr("Mark d&one or not done"), w);
    auto *unassign = new QPushButton(tr("Ta&ke the work back"), w);
    wrow->addWidget(assign);
    wrow->addWidget(done);
    wrow->addWidget(unassign);
    wrow->addStretch();
    l->addLayout(wrow);
    connect(addGrant, &QPushButton::clicked, this, &ProjectPage::addGrant);
    connect(removeGrant, &QPushButton::clicked, this, &ProjectPage::removeGrant);
    connect(assign, &QPushButton::clicked, this, &ProjectPage::addAssignment);
    connect(done, &QPushButton::clicked, this, &ProjectPage::toggleDone);
    connect(unassign, &QPushButton::clicked, this, &ProjectPage::removeAssignment);
    // Managers only give roles and work; the person reports their own done.
    for (QPushButton *b : {addGrant, removeGrant, assign, unassign})
        b->setProperty("managersOnly", true);
    return w;
}

void ProjectPage::say(const QString &message)
{
    m_message->setText(message);
}

bool ProjectPage::manages() const
{
    const QString role = m_access.value(QLatin1String("role")).toString();
    return role == QLatin1String("manage") || (m_conn && m_conn->isAdmin());
}

void ProjectPage::reload()
{
    if (!m_conn || !m_conn->isReady())
        return;
    const QString base = QStringLiteral("/api/projects/%1").arg(m_id);
    onResult(m_conn->api()->get(base), this, [this, base](const ApiResult &r) {
        if (!r.ok()) {
            if (r.status == 404)
                emit gone();
            else
                say(r.error.message);
            return;
        }
        m_project = r.object();
        m_access = m_project.value(QLatin1String("access")).toObject();
        showProject();
        onResult(m_conn->api()->get(base + QStringLiteral("/progress")), this, [this](const ApiResult &p) {
            if (p.ok())
                showProgress(p.array());
        });
        if (m_peopleTab)
            reloadPeople();
    });
}

void ProjectPage::showProject()
{
    const QJsonObject s = m_project.value(QLatin1String("settings")).toObject();
    m_title->setText(s.value(QLatin1String("name")).toString());
    QStringList meta;
    if (!s.value(QLatin1String("client")).toString().isEmpty())
        meta << s.value(QLatin1String("client")).toString();
    QStringList targets;
    for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray())
        targets << Languages::name(t.toString());
    meta << tr("from %1 into %2").arg(Languages::name(s.value(QLatin1String("sourceLang")).toString()), QLocale().createSeparatedList(targets));
    const QDateTime due = QDateTime::fromString(s.value(QLatin1String("due")).toString(), Qt::ISODate);
    if (due.isValid())
        meta << tr("due %1").arg(Languages::when(due));
    meta << (m_conn->isLocal() ? tr("kept on this computer") : tr("kept on %1").arg(m_conn->displayName()));
    QString line = meta.join(QStringLiteral(", "));
    line[0] = line[0].toUpper();
    m_meta->setText(line + QLatin1Char('.'));
    const bool manage = manages();
    m_delete->setVisible(manage);
    m_addFiles->setEnabled(manage);
    m_pretranslate->setEnabled(manage);
    m_saveSettings->setEnabled(manage);
    m_editor->setReadOnly(!manage);
    bool package = false;
    for (const QJsonValue &v : s.value(QLatin1String("files")).toArray())
        package = package || v.toObject().value(QLatin1String("role")).toString() == QLatin1String("package");
    m_returnPackage->setVisible(package);
    if (m_peopleTab) {
        for (QPushButton *b : m_peopleTab->findChildren<QPushButton *>()) {
            if (b->property("managersOnly").toBool())
                b->setEnabled(manage);
        }
    }
    // Languages for the analysis.
    const QString keep = m_analysisLang->currentData().toString();
    {
        const QSignalBlocker block(m_analysisLang);
        m_analysisLang->clear();
        m_analysisLang->addItem(tr("All languages"), QString());
        for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray())
            m_analysisLang->addItem(Languages::label(t.toString()), t.toString());
        m_analysisLang->setCurrentIndex(qMax(0, m_analysisLang->findData(keep)));
    }
    // Settings being edited are not replaced under the person's hands.
    if (!m_settingsDirty) {
        m_editor->load(m_conn, [this, s](const QString &error) {
            if (!error.isEmpty()) {
                say(error);
                return;
            }
            m_editor->setSettings(s, false);
            m_settingsDirty = false;
        });
    }
}

void ProjectPage::showProgress(const QJsonArray &progress)
{
    const QString keepFile = currentFile(), keepLang = currentLang();
    m_files->clear();
    const QJsonObject s = m_project.value(QLatin1String("settings")).toObject();
    for (const QJsonValue &fv : s.value(QLatin1String("files")).toArray()) {
        const QJsonObject f = fv.toObject();
        const QString name = f.value(QLatin1String("name")).toString();
        const QString role = f.value(QLatin1String("role")).toString();
        auto *item = new QTreeWidgetItem(m_files, {name});
        item->setData(0, Qt::UserRole, name);
        if (role == QLatin1String("reference"))
            item->setText(1, tr("reference"));
        else if (role == QLatin1String("package"))
            item->setText(1, tr("Trados package"));
        for (const QJsonValue &pv : progress) {
            const QJsonObject p = pv.toObject();
            if (p.value(QLatin1String("file")).toString() != name)
                continue;
            const int segs = p.value(QLatin1String("segments")).toInt();
            const int conf = p.value(QLatin1String("confirmed")).toInt();
            const QString lang = p.value(QLatin1String("lang")).toString();
            auto *child = new QTreeWidgetItem(item, {QString(), Languages::label(lang),
                                                     segs ? QStringLiteral("%1 %").arg(qRound(100.0 * conf / segs)) : QStringLiteral("100 %"),
                                                     QLocale().toString(p.value(QLatin1String("words")).toInt()),
                                                     tr("%1 of %2").arg(conf).arg(segs)});
            child->setData(0, Qt::UserRole, name);
            child->setData(1, Qt::UserRole, lang);
            child->setData(2, ProgressDelegate::ShareRole, segs ? double(conf) / segs : 1.0);
            child->setData(2, Qt::AccessibleTextRole, tr("%1 of %2 segments confirmed").arg(conf).arg(segs));
            if (name == keepFile && lang == keepLang)
                m_files->setCurrentItem(child);
        }
        item->setExpanded(true);
        if (name == keepFile && keepLang.isEmpty() && !m_files->currentItem())
            m_files->setCurrentItem(item);
    }
    m_saveTranslated->setEnabled(!currentFile().isEmpty());
    m_openEditor->setEnabled(!currentFile().isEmpty() && !currentLang().isEmpty());
    m_removeFile->setEnabled(manages() && !currentFile().isEmpty());
}

QString ProjectPage::currentFile() const
{
    QTreeWidgetItem *i = m_files->currentItem();
    return i ? i->data(0, Qt::UserRole).toString() : QString();
}

QString ProjectPage::currentLang() const
{
    QTreeWidgetItem *i = m_files->currentItem();
    return i ? i->data(1, Qt::UserRole).toString() : QString();
}

void ProjectPage::download(const QString &path, const QUrlQuery &query, const QString &what)
{
    say(tr("Writing %1…").arg(what));
    onResult(m_conn->api()->get(path, query), this, [this, what](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        const QString dir = QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation);
        const QString to = QFileDialog::getSaveFileName(this, tr("Save %1").arg(what), dir + QLatin1Char('/') + r.fileName());
        if (to.isEmpty()) {
            say(QString());
            return;
        }
        QSaveFile f(to);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size() || !f.commit()) {
            say(tr("%1 could not be saved: %2").arg(QDir::toNativeSeparators(to), f.errorString()));
            return;
        }
        say(tr("Saved %1.").arg(QDir::toNativeSeparators(to)));
    });
}

void ProjectPage::saveTranslated()
{
    const QString file = currentFile();
    if (file.isEmpty())
        return;
    QString lang = currentLang();
    const QJsonObject s = m_project.value(QLatin1String("settings")).toObject();
    if (lang.isEmpty()) {
        // A monolingual file chosen without a language: ask which.
        QStringList targets;
        for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray())
            targets << t.toString();
        for (const QJsonValue &fv : s.value(QLatin1String("files")).toArray()) {
            const QJsonObject f = fv.toObject();
            if (f.value(QLatin1String("name")).toString() == file && !f.value(QLatin1String("targetLang")).toString().isEmpty())
                targets = {f.value(QLatin1String("targetLang")).toString()};
        }
        if (targets.size() > 1) {
            bool ok = false;
            lang = QInputDialog::getItem(this, tr("Save translated file"), tr("Language:"), targets, 0, false, &ok);
            if (!ok)
                return;
        } else if (!targets.isEmpty()) {
            lang = targets.first();
        }
    }
    download(QStringLiteral("/api/projects/%1/files/translated").arg(m_id), QUrlQuery{{QStringLiteral("name"), file}, {QStringLiteral("lang"), lang}},
             tr("the translated file"));
}

void ProjectPage::saveReturnPackage()
{
    download(QStringLiteral("/api/projects/%1/return-package").arg(m_id), {}, tr("the return package"));
}

void ProjectPage::addFiles()
{
    const QStringList paths = QFileDialog::getOpenFileNames(this, tr("Files to add"));
    for (const QString &path : paths) {
        QFile f(path);
        if (!f.open(QIODevice::ReadOnly)) {
            say(tr("%1 can't be read: %2").arg(QDir::toNativeSeparators(path), f.errorString()));
            return;
        }
        const QString name = QFileInfo(path).fileName();
        const QByteArray data = f.readAll();
        // Inspected first: what can't be translated goes in for reference.
        onResult(m_conn->api()->upload("POST", QStringLiteral("/api/files/inspect"), data, QStringLiteral("application/octet-stream"), QUrlQuery{{QStringLiteral("name"), name}}), this,
                 [this, name, data](const ApiResult &in) {
                     if (!in.ok()) {
                         say(in.error.message);
                         return;
                     }
                     QJsonObject body{{QStringLiteral("name"), name}, {QStringLiteral("data"), QString::fromLatin1(data.toBase64())}};
                     if (in.object().value(QLatin1String("translatable")).toBool()) {
                         body.insert(QStringLiteral("role"), QStringLiteral("translatable"));
                     } else {
                         body.insert(QStringLiteral("role"), QStringLiteral("reference"));
                         body.insert(QStringLiteral("format"), QFileInfo(name).suffix().toLower());
                     }
                     say(tr("Adding %1…").arg(name));
                     onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/files").arg(m_id), body,
                                                  QUrlQuery{{QStringLiteral("version"), QString::number(m_project.value(QLatin1String("version")).toInt())}}),
                              this, [this, name](const ApiResult &r) {
                                  say(r.ok() ? tr("Added %1.").arg(name) : r.error.message);
                                  reload();
                              });
                 });
    }
}

void ProjectPage::removeFile()
{
    const QString file = currentFile();
    if (file.isEmpty())
        return;
    if (QMessageBox::question(this, tr("Remove the file?"), tr("%1 and its translations are removed from the project.").arg(file),
                              QMessageBox::Yes | QMessageBox::Cancel, QMessageBox::Cancel) != QMessageBox::Yes)
        return;
    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1/files").arg(m_id), QUrlQuery{{QStringLiteral("name"), file}},
                                m_project.value(QLatin1String("version")).toInt()),
             this, [this, file](const ApiResult &r) {
                 say(r.ok() ? tr("Removed %1.").arg(file) : r.error.message);
                 reload();
             });
}

void ProjectPage::pretranslate()
{
    say(tr("Filling segments from the memories…"));
    onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/pretranslate").arg(m_id), QJsonObject{}), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        int filled = 0;
        for (const QJsonValue &v : r.array())
            filled += v.toObject().value(QLatin1String("filled")).toInt();
        say(filled ? tr("%n segment(s) filled.", nullptr, filled) : tr("No segment had a good enough match."));
        reload();
    });
}

void ProjectPage::saveSettings()
{
    const QJsonObject s = m_editor->settings();
    say(tr("Saving the settings…"));
    onResult(m_conn->api()->put(QStringLiteral("/api/projects/%1/settings").arg(m_id), s, m_project.value(QLatin1String("version")).toInt()), this,
             [this](const ApiResult &r) {
                 if (!r.ok()) {
                     say(r.error.code == QLatin1String("conflict") ? tr("Someone changed the project meanwhile. The settings were read again; make your changes once more.")
                                                                   : r.error.message);
                     m_editor->showProblems(r.error.problems);
                     if (r.error.code == QLatin1String("conflict")) {
                         m_settingsDirty = false;
                         reload();
                     }
                     return;
                 }
                 m_settingsDirty = false;
                 say(tr("Settings saved."));
                 m_project = r.object();
                 showProject();
             });
}

void ProjectPage::saveTemplate()
{
    bool ok = false;
    const QString name = QInputDialog::getText(this, tr("Save as template"), tr("Template name:"), QLineEdit::Normal,
                                               m_project.value(QLatin1String("settings")).toObject().value(QLatin1String("client")).toString(), &ok)
                             .trimmed();
    if (!ok || name.isEmpty())
        return;
    QJsonObject s = m_editor->settings();
    QJsonObject t{{QStringLiteral("name"), name}};
    for (const char *key : {"description", "client", "sourceLang", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare"})
        t.insert(QLatin1String(key), s.value(QLatin1String(key)));
    QJsonObject term = t.value(QLatin1String("terminology")).toObject();
    term.remove(QStringLiteral("termbase")); // a new project makes its own
    t.insert(QStringLiteral("terminology"), term);
    // The project's own memories belong to it, not to the template.
    QJsonObject mems = t.value(QLatin1String("memories")).toObject();
    const QString pname = s.value(QLatin1String("name")).toString();
    for (const QString &lang : mems.keys()) {
        QJsonArray kept;
        for (const QJsonValue &m : mems.value(lang).toArray()) {
            if (m.toObject().value(QLatin1String("memory")).toString() != QStringLiteral("%1 (%2)").arg(pname, lang))
                kept.append(m);
        }
        mems.insert(lang, kept);
    }
    t.insert(QStringLiteral("memories"), mems);
    // Saving over a template of the same name needs its version.
    onResult(m_conn->api()->get(QStringLiteral("/api/templates")), this, [this, t, name](const ApiResult &list) {
        int version = 0;
        for (const QJsonValue &v : list.array()) {
            if (v.toObject().value(QLatin1String("name")).toString().compare(name, Qt::CaseInsensitive) == 0)
                version = v.toObject().value(QLatin1String("version")).toInt();
        }
        if (version && QMessageBox::question(this, tr("Replace the template?"), tr("A template called %1 exists. Replace it?").arg(name)) != QMessageBox::Yes)
            return;
        onResult(m_conn->api()->put(QStringLiteral("/api/templates"), t, version), this, [this, name](const ApiResult &r) {
            say(r.ok() ? tr("Saved the template %1.").arg(name) : r.error.message);
        });
    });
}

void ProjectPage::runAnalysis()
{
    say(tr("Analysing…"));
    onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/analysis").arg(m_id), QJsonObject{}), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        m_analysis = r.array();
        say(QString());
        if (m_analysisLang->currentIndex() == 0 && m_analysisLang->count() > 1)
            m_analysisLang->setCurrentIndex(1);
        else
            emit m_analysisLang->currentIndexChanged(m_analysisLang->currentIndex());
    });
}

void ProjectPage::saveAnalysis()
{
    const QString to = QFileDialog::getSaveFileName(this, tr("Save the analysis"),
                                                    QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation) + QStringLiteral("/%1 analysis.csv").arg(m_title->text()),
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
        say(tr("Saved %1.").arg(QDir::toNativeSeparators(to)));
}

void ProjectPage::reloadPeople()
{
    const QUrlQuery q{{QStringLiteral("kind"), QStringLiteral("project")}, {QStringLiteral("resource"), QString::number(m_id)}};
    if (manages()) {
        onResult(m_conn->api()->get(QStringLiteral("/api/grants"), q), this, [this](const ApiResult &r) {
            m_grants->setRowCount(0);
            for (const QJsonValue &v : r.array()) {
                const QJsonObject g = v.toObject();
                const int i = m_grants->rowCount();
                m_grants->insertRow(i);
                const QString who = g.value(QLatin1String("name")).toString().isEmpty() ? g.value(QLatin1String("username")).toString() : g.value(QLatin1String("name")).toString();
                m_grants->setItem(i, 0, cell(who, g.value(QLatin1String("userId")).toInteger()));
                m_grants->setItem(i, 1, cell(roleName(g.value(QLatin1String("role")).toString())));
            }
        });
        onResult(m_conn->api()->get(QStringLiteral("/api/people")), this, [this](const ApiResult &r) {
            if (r.ok())
                m_people = r.array();
        });
    }
    onResult(m_conn->api()->get(QStringLiteral("/api/projects/%1/assignments").arg(m_id)), this, [this](const ApiResult &r) {
        if (!r.ok())
            return;
        m_assignments = r.array();
        m_work->setRowCount(0);
        for (const QJsonValue &v : std::as_const(m_assignments)) {
            const QJsonObject a = v.toObject();
            const int i = m_work->rowCount();
            m_work->insertRow(i);
            const QString range = a.value(QLatin1String("fromSegment")).toString().isEmpty()
                ? tr("all")
                : QStringLiteral("%1 to %2").arg(a.value(QLatin1String("fromSegment")).toString(), a.value(QLatin1String("toSegment")).toString());
            const QDateTime due = QDateTime::fromString(a.value(QLatin1String("due")).toString(), Qt::ISODate);
            QString person = a.value(QLatin1String("userName")).toString();
            if (a.value(QLatin1String("done")).toBool())
                person = tr("%1 (done)").arg(person);
            m_work->setItem(i, 0, cell(person, a.value(QLatin1String("id")).toInteger()));
            m_work->setItem(i, 1, cell(a.value(QLatin1String("task")).toString() == QLatin1String("review") ? tr("Review") : tr("Translate")));
            m_work->setItem(i, 2, cell(a.value(QLatin1String("file")).toString().isEmpty() ? tr("all files") : a.value(QLatin1String("file")).toString()));
            m_work->setItem(i, 3, cell(a.value(QLatin1String("targetLang")).toString().isEmpty() ? tr("all") : a.value(QLatin1String("targetLang")).toString()));
            m_work->setItem(i, 4, cell(range));
            m_work->setItem(i, 5, cell(due.isValid() ? Languages::when(due) : tr("the project's")));
        }
    });
}

void ProjectPage::addGrant()
{
    QDialog dlg(this);
    dlg.setWindowTitle(tr("Give a role"));
    auto *form = new QFormLayout(&dlg);
    auto *person = new QComboBox(&dlg);
    for (const QJsonValue &v : std::as_const(m_people)) {
        const QJsonObject p = v.toObject();
        person->addItem(QStringLiteral("%1 (%2)").arg(p.value(QLatin1String("name")).toString(), p.value(QLatin1String("username")).toString()),
                        p.value(QLatin1String("id")).toInteger());
    }
    auto *role = new QComboBox(&dlg);
    for (const QString &r : roles)
        role->addItem(roleName(r), r);
    role->setCurrentIndex(2);
    auto *pl = new QLabel(tr("&Person:"), &dlg);
    pl->setBuddy(person);
    auto *rl = new QLabel(tr("&Role:"), &dlg);
    rl->setBuddy(role);
    form->addRow(pl, person);
    form->addRow(rl, role);
    auto *buttons = new QDialogButtonBox(QDialogButtonBox::Ok | QDialogButtonBox::Cancel, &dlg);
    form->addRow(buttons);
    connect(buttons, &QDialogButtonBox::accepted, &dlg, &QDialog::accept);
    connect(buttons, &QDialogButtonBox::rejected, &dlg, &QDialog::reject);
    if (dlg.exec() != QDialog::Accepted || person->currentIndex() < 0)
        return;
    const QJsonObject g{{QStringLiteral("userId"), person->currentData().toLongLong()}, {QStringLiteral("kind"), QStringLiteral("project")},
                        {QStringLiteral("resource"), QString::number(m_id)}, {QStringLiteral("role"), role->currentData().toString()}};
    onResult(m_conn->api()->put(QStringLiteral("/api/grants"), g), this, [this](const ApiResult &r) {
        say(r.ok() ? tr("Role given.") : r.error.message);
        reloadPeople();
    });
}

void ProjectPage::removeGrant()
{
    const int row = m_grants->currentRow();
    if (row < 0)
        return;
    const QUrlQuery q{{QStringLiteral("userId"), m_grants->item(row, 0)->data(Qt::UserRole).toString()},
                      {QStringLiteral("kind"), QStringLiteral("project")},
                      {QStringLiteral("resource"), QString::number(m_id)}};
    onResult(m_conn->api()->del(QStringLiteral("/api/grants"), q), this, [this](const ApiResult &r) {
        say(r.ok() ? tr("Role taken back.") : r.error.message);
        reloadPeople();
    });
}

void ProjectPage::addAssignment()
{
    const QJsonObject s = m_project.value(QLatin1String("settings")).toObject();
    QDialog dlg(this);
    dlg.setWindowTitle(tr("Give out work"));
    auto *form = new QFormLayout(&dlg);
    const auto row = [&](const QString &label, QWidget *w) {
        auto *l = new QLabel(label, &dlg);
        l->setBuddy(w);
        form->addRow(l, w);
    };
    auto *person = new QComboBox(&dlg);
    for (const QJsonValue &v : std::as_const(m_people)) {
        const QJsonObject p = v.toObject();
        person->addItem(p.value(QLatin1String("name")).toString(), p.value(QLatin1String("id")).toInteger());
    }
    row(tr("&Person:"), person);
    auto *task = new QComboBox(&dlg);
    task->addItem(tr("Translate"), QStringLiteral("translate"));
    task->addItem(tr("Review"), QStringLiteral("review"));
    row(tr("&Task:"), task);
    auto *file = new QComboBox(&dlg);
    file->addItem(tr("All files"), QString());
    for (const QJsonValue &v : s.value(QLatin1String("files")).toArray()) {
        const QJsonObject f = v.toObject();
        if (f.value(QLatin1String("role")).toString() == QLatin1String("translatable"))
            file->addItem(f.value(QLatin1String("name")).toString(), f.value(QLatin1String("name")).toString());
    }
    row(tr("&File:"), file);
    auto *lang = new QComboBox(&dlg);
    lang->addItem(tr("All languages"), QString());
    for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray())
        lang->addItem(Languages::label(t.toString()), t.toString());
    row(tr("&Language:"), lang);
    auto *whole = new QCheckBox(tr("The &whole file"), &dlg);
    whole->setChecked(true);
    form->addRow(QString(), whole);
    auto *from = new QSpinBox(&dlg);
    auto *to = new QSpinBox(&dlg);
    from->setMinimum(1);
    to->setMinimum(1);
    row(tr("From se&gment:"), from);
    row(tr("T&o segment:"), to);
    auto *hasDue = new QCheckBox(tr("&Due earlier than the project"), &dlg);
    auto *due = new QDateTimeEdit(QDateTime::currentDateTime().addDays(3), &dlg);
    due->setCalendarPopup(true);
    due->setAccessibleName(tr("Due"));
    form->addRow(QString(), hasDue);
    form->addRow(QString(), due);
    auto *error = new QLabel(&dlg);
    error->setWordWrap(true);
    form->addRow(error);
    auto *buttons = new QDialogButtonBox(QDialogButtonBox::Ok | QDialogButtonBox::Cancel, &dlg);
    form->addRow(buttons);
    // A range needs one file in one language, whose segments are counted.
    auto segIds = std::make_shared<QStringList>();
    const auto update = [&, segIds] {
        const bool one = !file->currentData().toString().isEmpty() && !lang->currentData().toString().isEmpty();
        whole->setEnabled(one);
        if (!one)
            whole->setChecked(true);
        from->setEnabled(!whole->isChecked());
        to->setEnabled(!whole->isChecked());
        due->setEnabled(hasDue->isChecked());
        if (!one)
            return;
        onResult(m_conn->api()->get(QStringLiteral("/api/projects/%1/segments").arg(m_id),
                                    QUrlQuery{{QStringLiteral("file"), file->currentData().toString()}, {QStringLiteral("lang"), lang->currentData().toString()}}),
                 &dlg, [segIds, from, to](const ApiResult &r) {
                     segIds->clear();
                     for (const QJsonValue &v : r.array())
                         *segIds << v.toObject().value(QLatin1String("id")).toString();
                     from->setMaximum(qMax(1, int(segIds->size())));
                     to->setMaximum(qMax(1, int(segIds->size())));
                     to->setValue(int(segIds->size()));
                 });
    };
    connect(file, &QComboBox::currentIndexChanged, &dlg, update);
    connect(lang, &QComboBox::currentIndexChanged, &dlg, update);
    connect(whole, &QCheckBox::toggled, &dlg, update);
    connect(hasDue, &QCheckBox::toggled, &dlg, update);
    update();
    connect(buttons, &QDialogButtonBox::rejected, &dlg, &QDialog::reject);
    connect(buttons, &QDialogButtonBox::accepted, &dlg, [&, segIds] {
        QJsonObject a{{QStringLiteral("userId"), person->currentData().toLongLong()}, {QStringLiteral("task"), task->currentData().toString()},
                      {QStringLiteral("file"), file->currentData().toString()}, {QStringLiteral("targetLang"), lang->currentData().toString()}};
        if (!whole->isChecked()) {
            if (from->value() > to->value() || to->value() > segIds->size()) {
                error->setText(tr("The range runs from a segment to a later one."));
                return;
            }
            a.insert(QStringLiteral("fromSegment"), segIds->at(from->value() - 1));
            a.insert(QStringLiteral("toSegment"), segIds->at(to->value() - 1));
        }
        if (hasDue->isChecked())
            a.insert(QStringLiteral("due"), due->dateTime().toUTC().toString(Qt::ISODate));
        onResult(m_conn->api()->post(QStringLiteral("/api/projects/%1/assignments").arg(m_id), a), &dlg, [&](const ApiResult &r) {
            if (!r.ok()) {
                error->setText(r.error.message);
                return;
            }
            dlg.accept();
        });
    });
    if (dlg.exec() == QDialog::Accepted) {
        say(tr("Work given out."));
        reloadPeople();
    }
}

void ProjectPage::toggleDone()
{
    const int row = m_work->currentRow();
    if (row < 0 || row >= m_assignments.size())
        return;
    QJsonObject a = m_assignments.at(row).toObject();
    a.insert(QStringLiteral("done"), !a.value(QLatin1String("done")).toBool());
    onResult(m_conn->api()->put(QStringLiteral("/api/projects/%1/assignments/%2").arg(m_id).arg(a.value(QLatin1String("id")).toInteger()), a,
                                a.value(QLatin1String("version")).toInt()),
             this, [this](const ApiResult &r) {
                 say(r.ok() ? QString() : r.error.message);
                 reloadPeople();
             });
}

void ProjectPage::removeAssignment()
{
    const int row = m_work->currentRow();
    if (row < 0 || row >= m_assignments.size())
        return;
    const qint64 id = m_assignments.at(row).toObject().value(QLatin1String("id")).toInteger();
    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1/assignments/%2").arg(m_id).arg(id)), this, [this](const ApiResult &r) {
        say(r.ok() ? tr("Work taken back.") : r.error.message);
        reloadPeople();
    });
}

void ProjectPage::deleteProject()
{
    const QString name = m_title->text();
    if (QMessageBox::question(this, tr("Delete the project?"),
                              tr("%1 is deleted with its files and translations. Its memories and termbase stay; delete them on their own pages.").arg(name),
                              QMessageBox::Yes | QMessageBox::Cancel, QMessageBox::Cancel) != QMessageBox::Yes)
        return;
    onResult(m_conn->api()->del(QStringLiteral("/api/projects/%1").arg(m_id)), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            say(r.error.message);
            return;
        }
        emit gone();
    });
}
