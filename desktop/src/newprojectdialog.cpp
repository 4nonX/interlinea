#include "newprojectdialog.h"

#include "apiclient.h"
#include "connectionmanager.h"
#include "languages.h"
#include "settingseditor.h"

#include "theme.h"

#include <QComboBox>
#include <QDirIterator>
#include <QFileDialog>
#include <QFileInfo>
#include <QFormLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QJsonArray>
#include <QLabel>
#include <QPushButton>
#include <QStackedWidget>
#include <QTreeWidget>
#include <QVBoxLayout>

static const qint64 maxFile = 200ll << 20;

NewProjectDialog::NewProjectDialog(ConnectionManager *manager, QWidget *parent)
    : QDialog(parent), m_manager(manager), m_stack(new QStackedWidget(this)), m_editor(new SettingsEditor(this))
{
    setWindowTitle(tr("New project"));
    resize(820, 640);
    auto *layout = new QVBoxLayout(this);
    layout->addWidget(m_stack, 1);

    // Step 1: the documents.
    auto *docs = new QWidget(m_stack);
    auto *dl = new QVBoxLayout(docs);
    auto *top = new QFormLayout;
    m_where = new QComboBox(docs);
    m_where->setAccessibleName(tr("Kept on"));
    auto *wl = new QLabel(tr("&Kept on:"), docs);
    wl->setBuddy(m_where);
    top->addRow(wl, m_where);
    m_template = new QComboBox(docs);
    m_template->setAccessibleName(tr("Template"));
    auto *tl = new QLabel(tr("&Template:"), docs);
    tl->setBuddy(m_template);
    top->addRow(tl, m_template);
    dl->addLayout(top);
    auto *intro = new QLabel(tr("Add the documents to translate: Word files, bilingual files (XLIFF, SDLXLIFF, MQXLIFF), Trados packages and text files. Other files travel with the project for reference."), docs);
    intro->setWordWrap(true);
    Theme::secondary(intro);
    dl->addWidget(intro);
    m_files = new QTreeWidget(docs);
    m_files->setAccessibleName(tr("Documents"));
    m_files->setRootIsDecorated(false);
    m_files->setHeaderLabels({tr("Document"), tr("Used as"), tr("Words"), tr("Language"), tr("Note")});
    m_files->header()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_files->header()->setSectionResizeMode(4, QHeaderView::Stretch);
    dl->addWidget(m_files, 1);
    auto *buttons = new QHBoxLayout;
    auto *addFiles = new QPushButton(tr("Add &files…"), docs);
    auto *addFolder = new QPushButton(tr("Add f&older…"), docs);
    auto *reference = new QPushButton(tr("&Reference only"), docs);
    reference->setToolTip(tr("Keeps the selected document with the project without translating it, or translates it again."));
    auto *remove = new QPushButton(tr("Re&move"), docs);
    for (QPushButton *b : {addFiles, addFolder, reference, remove})
        buttons->addWidget(b);
    buttons->addStretch();
    dl->addLayout(buttons);
    auto *langRow = new QFormLayout;
    m_source = new LanguageCombo(docs);
    m_source->setAccessibleName(tr("Source language"));
    auto *sl = new QLabel(tr("&Source language:"), docs);
    sl->setBuddy(m_source);
    langRow->addRow(sl, m_source);
    m_detected = new QLabel(docs);
    m_detected->setWordWrap(true);
    Theme::secondary(m_detected);
    langRow->addRow(QString(), m_detected);
    dl->addLayout(langRow);
    m_filesError = new QLabel(docs);
    m_filesError->setWordWrap(true);
    QPalette p = m_filesError->palette();
    p.setColor(QPalette::WindowText, QColor(0xaa, 0x22, 0x19));
    m_filesError->setPalette(p);
    m_filesError->setProperty("role", QStringLiteral("error"));
    dl->addWidget(m_filesError);
    m_stack->addWidget(docs);

    // Step 2: the setup.
    m_stack->addWidget(m_editor);

    m_status = new QLabel(this);
    m_status->setWordWrap(true);
    m_status->setAccessibleName(tr("Status"));
    layout->addWidget(m_status);
    auto *nav = new QHBoxLayout;
    nav->addStretch();
    m_back = new QPushButton(tr("&Back"), this);
    m_next = new QPushButton(tr("&Next"), this);
    m_next->setDefault(true);
    Theme::primary(m_next);
    auto *cancel = new QPushButton(tr("Cancel"), this);
    nav->addWidget(m_back);
    nav->addWidget(m_next);
    nav->addWidget(cancel);
    layout->addLayout(nav);
    m_back->setVisible(false);

    connect(addFiles, &QPushButton::clicked, this, &NewProjectDialog::chooseFiles);
    connect(addFolder, &QPushButton::clicked, this, &NewProjectDialog::chooseFolder);
    connect(reference, &QPushButton::clicked, this, [this] {
        const int i = m_files->indexOfTopLevelItem(m_files->currentItem());
        if (i < 0)
            return;
        Doc &d = m_docs[i];
        if (d.role == QLatin1String("reference")) {
            if (d.inspection.value(QLatin1String("translatable")).toBool())
                d.role = d.inspection.value(QLatin1String("format")).toString() == QLatin1String("package") ? QStringLiteral("package") : QStringLiteral("translatable");
        } else {
            d.role = QStringLiteral("reference");
        }
        refreshDocs();
    });
    connect(remove, &QPushButton::clicked, this, [this] {
        const int i = m_files->indexOfTopLevelItem(m_files->currentItem());
        if (i >= 0) {
            m_docs.removeAt(i);
            refreshDocs();
            guessLanguages();
        }
    });
    connect(m_where, &QComboBox::currentIndexChanged, this, [this] {
        // Documents are read by the core or server they go to.
        for (int i = 0; i < m_docs.size(); ++i)
            inspect(i);
        onResult(connection()->api()->get(QStringLiteral("/api/templates")), this, [this](const ApiResult &r) {
            m_templates = r.ok() ? r.array() : QJsonArray();
            m_template->clear();
            m_template->addItem(tr("No template"), QString());
            for (const QJsonValue &v : std::as_const(m_templates))
                m_template->addItem(v.toObject().value(QLatin1String("name")).toString(), v.toObject().value(QLatin1String("name")).toString());
        });
    });
    connect(m_back, &QPushButton::clicked, this, &NewProjectDialog::back);
    connect(m_next, &QPushButton::clicked, this, &NewProjectDialog::next);
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    // The core may still be starting (Interlinea opened with a document).
    connect(manager, &ConnectionManager::changed, this, &NewProjectDialog::refreshWhere);
    connect(manager, &ConnectionManager::removed, this, &NewProjectDialog::refreshWhere);
    refreshWhere();
}

void NewProjectDialog::refreshWhere()
{
    const QString keep = m_where->currentData().toString();
    QStringList ready;
    for (Connection *c : m_manager->all()) {
        if (c->isReady())
            ready << c->id();
    }
    QStringList shown;
    for (int i = 0; i < m_where->count(); ++i)
        shown << m_where->itemData(i).toString();
    if (shown == ready)
        return;
    {
        const QSignalBlocker block(m_where);
        m_where->clear();
        for (const QString &id : ready)
            m_where->addItem(m_manager->find(id)->displayName(), id);
        m_where->setCurrentIndex(qMax(0, m_where->findData(keep)));
    }
    if (m_where->count() == 0) {
        m_filesError->setText(m_manager->local()->state() == Connection::State::Connecting
                                  ? tr("Starting the work on this computer…")
                                  : tr("Nowhere to keep a project: work on this computer can't be opened and no team server is signed in."));
        return;
    }
    m_filesError->clear();
    if (m_where->currentData().toString() != keep)
        emit m_where->currentIndexChanged(m_where->currentIndex());
}

Connection *NewProjectDialog::connection() const
{
    return m_manager->find(m_where->currentData().toString());
}

void NewProjectDialog::chooseFiles()
{
    addFiles(QFileDialog::getOpenFileNames(this, tr("Documents for the project")));
}

void NewProjectDialog::chooseFolder()
{
    const QString dir = QFileDialog::getExistingDirectory(this, tr("A folder of documents"));
    if (dir.isEmpty())
        return;
    QStringList paths;
    QDirIterator it(dir, QDir::Files, QDirIterator::Subdirectories);
    while (it.hasNext())
        paths << it.next();
    paths.sort();
    // Files keep the folder they came from in their name.
    const int before = int(m_docs.size());
    addFiles(paths);
    const QDir base(QFileInfo(dir).absolutePath());
    for (int i = before; i < m_docs.size(); ++i)
        m_docs[i].name = base.relativeFilePath(m_docs[i].path);
    refreshDocs();
}

void NewProjectDialog::addFiles(const QStringList &paths)
{
    for (const QString &path : paths) {
        QFile f(path);
        Doc d{path, QFileInfo(path).fileName(), {}, {}, QStringLiteral("reference")};
        if (f.size() > maxFile) {
            d.inspection = QJsonObject{{QStringLiteral("problem"), tr("The file is larger than 200 MB.")}};
        } else if (!f.open(QIODevice::ReadOnly)) {
            d.inspection = QJsonObject{{QStringLiteral("problem"), tr("The file can't be read: %1").arg(f.errorString())}};
        } else {
            d.data = f.readAll();
        }
        bool dup = false;
        for (const Doc &x : std::as_const(m_docs))
            dup = dup || x.name.compare(d.name, Qt::CaseInsensitive) == 0;
        if (dup)
            continue;
        m_docs.append(d);
        if (!d.data.isEmpty())
            inspect(int(m_docs.size()) - 1);
    }
    refreshDocs();
}

void NewProjectDialog::inspect(int index)
{
    Connection *c = connection();
    if (!c || index >= m_docs.size() || m_docs[index].data.isEmpty())
        return;
    const QString path = m_docs[index].path;
    onResult(c->api()->upload("POST", QStringLiteral("/api/files/inspect"), m_docs[index].data, QStringLiteral("application/octet-stream"),
                              QUrlQuery{{QStringLiteral("name"), m_docs[index].name}}),
             this, [this, path](const ApiResult &r) {
                 for (Doc &d : m_docs) {
                     if (d.path != path)
                         continue;
                     d.inspection = r.ok() ? r.object() : QJsonObject{{QStringLiteral("problem"), r.error.message}};
                     const QString format = d.inspection.value(QLatin1String("format")).toString();
                     d.role = !d.inspection.value(QLatin1String("translatable")).toBool() ? QStringLiteral("reference")
                         : format == QLatin1String("package")                            ? QStringLiteral("package")
                                                                                         : QStringLiteral("translatable");
                 }
                 refreshDocs();
                 guessLanguages();
             });
}

void NewProjectDialog::refreshDocs()
{
    const int keep = m_files->indexOfTopLevelItem(m_files->currentItem());
    m_files->clear();
    for (const Doc &d : std::as_const(m_docs)) {
        const QJsonObject in = d.inspection;
        QString role = d.role == QLatin1String("package") ? tr("Trados package")
            : d.role == QLatin1String("translatable")      ? tr("To translate")
                                                           : tr("Reference");
        QString lang;
        if (const QJsonObject b = in.value(QLatin1String("bilingual")).toObject(); !b.isEmpty())
            lang = QStringLiteral("%1 → %2").arg(b.value(QLatin1String("sourceLang")).toString(), b.value(QLatin1String("targetLang")).toString());
        else if (const QJsonObject p = in.value(QLatin1String("package")).toObject(); !p.isEmpty())
            lang = QStringLiteral("%1 → %2").arg(p.value(QLatin1String("sourceLang")).toString(),
                                                 p.value(QLatin1String("targetLangs")).toVariant().toStringList().join(QStringLiteral(", ")));
        else
            lang = in.value(QLatin1String("detection")).toObject().value(QLatin1String("lang")).toString();
        QString note = in.value(QLatin1String("problem")).toString();
        if (in.isEmpty())
            note = tr("Reading…");
        else if (note.isEmpty())
            note = in.value(QLatin1String("warnings")).toVariant().toStringList().join(QLatin1Char(' '));
        auto *item = new QTreeWidgetItem(m_files, {d.name, role, in.contains(QLatin1String("words")) ? QString::number(in.value(QLatin1String("words")).toInt()) : QString(), lang, note});
        item->setToolTip(0, d.path);
        item->setToolTip(4, note);
    }
    if (keep >= 0 && keep < m_files->topLevelItemCount())
        m_files->setCurrentItem(m_files->topLevelItem(keep));
}

// The source language: a bilingual file or package says it; otherwise the
// text's language, which the person confirms when it is uncertain.
void NewProjectDialog::guessLanguages()
{
    QString said;
    double best = 0;
    QString guess, reason;
    for (const Doc &d : std::as_const(m_docs)) {
        if (d.role == QLatin1String("reference"))
            continue;
        const QJsonObject in = d.inspection;
        const QString b = in.value(QLatin1String("bilingual")).toObject().value(QLatin1String("sourceLang")).toString();
        const QString p = in.value(QLatin1String("package")).toObject().value(QLatin1String("sourceLang")).toString();
        if (said.isEmpty())
            said = !b.isEmpty() ? b : p;
        const QJsonObject det = in.value(QLatin1String("detection")).toObject();
        if (det.value(QLatin1String("confidence")).toDouble() > best) {
            best = det.value(QLatin1String("confidence")).toDouble();
            guess = det.value(QLatin1String("lang")).toString();
        }
        reason = det.value(QLatin1String("reason")).toString();
    }
    if (!said.isEmpty()) {
        m_source->setLanguage(said);
        m_detected->setText(tr("The bilingual file says the source is %1.").arg(Languages::name(said)));
    } else if (!guess.isEmpty()) {
        m_source->setLanguage(guess);
        m_detected->setText(best >= 0.75 ? tr("The documents are in %1.").arg(Languages::name(guess))
                                         : tr("The documents seem to be in %1, but the text gives little to go on. Check the source language.").arg(Languages::name(guess)));
    } else {
        m_detected->setText(reason == QLatin1String("short") ? tr("The text is too short to tell its language. Choose the source language.")
                                                            : tr("Choose the source language."));
    }
}

void NewProjectDialog::setBusy(bool busy, const QString &what)
{
    m_next->setEnabled(!busy);
    m_back->setEnabled(!busy);
    m_status->setText(what);
}

void NewProjectDialog::next()
{
    if (m_stack->currentIndex() == 1) {
        create();
        return;
    }
    m_filesError->clear();
    Connection *c = connection();
    if (!c) {
        m_filesError->setText(tr("Choose where the project is kept."));
        return;
    }
    bool any = false, reading = false;
    QStringList targets;
    QString name;
    for (const Doc &d : std::as_const(m_docs)) {
        reading = reading || (d.inspection.isEmpty() && !d.data.isEmpty());
        if (d.role == QLatin1String("reference"))
            continue;
        any = true;
        const QJsonObject in = d.inspection;
        const QString bt = in.value(QLatin1String("bilingual")).toObject().value(QLatin1String("targetLang")).toString();
        if (!bt.isEmpty() && !targets.contains(Languages::normalize(bt)))
            targets << Languages::normalize(bt);
        const QJsonObject pkg = in.value(QLatin1String("package")).toObject();
        for (const QString &t : pkg.value(QLatin1String("targetLangs")).toVariant().toStringList()) {
            if (!targets.contains(Languages::normalize(t)))
                targets << Languages::normalize(t);
        }
        if (name.isEmpty())
            name = !pkg.value(QLatin1String("name")).toString().isEmpty() ? pkg.value(QLatin1String("name")).toString() : QFileInfo(d.name).completeBaseName();
    }
    if (reading) {
        m_filesError->setText(tr("The documents are still being read."));
        return;
    }
    if (!any) {
        m_filesError->setText(tr("Add at least one document to translate."));
        return;
    }
    if (m_source->language().isEmpty()) {
        m_filesError->setText(tr("Choose the source language."));
        m_source->setFocus();
        return;
    }
    setBusy(true, tr("Loading the memories and termbases…"));
    onResult(c->api()->get(QStringLiteral("/api/projects/defaults")), this, [this, c, targets, name](const ApiResult &r) {
        if (!r.ok()) {
            setBusy(false, r.error.message);
            return;
        }
        QJsonObject s = r.object();
        // A template sets everything but the name and the documents.
        const QString tmpl = m_template->currentData().toString();
        for (const QJsonValue &v : std::as_const(m_templates)) {
            const QJsonObject t = v.toObject();
            if (t.value(QLatin1String("name")).toString() != tmpl || tmpl.isEmpty())
                continue;
            for (const char *key : {"client", "description", "targetLangs", "memories", "projectMemory", "terminology", "analysis", "qa", "prepare"}) {
                if (t.contains(QLatin1String(key)))
                    s.insert(QLatin1String(key), t.value(QLatin1String(key)));
            }
        }
        s.insert(QStringLiteral("name"), name);
        s.insert(QStringLiteral("sourceLang"), m_source->language());
        QStringList ts = s.value(QLatin1String("targetLangs")).toVariant().toStringList();
        for (const QString &t : targets) {
            if (!ts.contains(t))
                ts << t;
        }
        s.insert(QStringLiteral("targetLangs"), QJsonArray::fromStringList(ts));
        QJsonObject term = s.value(QLatin1String("terminology")).toObject();
        term.insert(QStringLiteral("location"), QStringLiteral("server"));
        s.insert(QStringLiteral("terminology"), term);
        m_editor->load(c, [this, s](const QString &error) {
            setBusy(false, error);
            if (!error.isEmpty())
                return;
            m_editor->setSettings(s, true);
            m_stack->setCurrentIndex(1);
            m_back->setVisible(true);
            m_next->setText(tr("&Make the project"));
            m_editor->setCurrentIndex(0);
        });
    });
}

void NewProjectDialog::back()
{
    m_stack->setCurrentIndex(0);
    m_back->setVisible(false);
    m_next->setText(tr("&Next"));
    m_status->clear();
}

void NewProjectDialog::create()
{
    Connection *c = connection();
    if (!c)
        return;
    QJsonObject s = m_editor->settings();
    const QString name = s.value(QLatin1String("name")).toString();
    const QString source = s.value(QLatin1String("sourceLang")).toString();
    const QStringList targets = s.value(QLatin1String("targetLangs")).toVariant().toStringList();
    QJsonObject req;
    // The project's own memories come first for each language, so they
    // win ties.
    QJsonArray newMemories;
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
    const QString mode = s.value(QLatin1String("terminology")).toObject().value(QLatin1String("mode")).toString();
    if (mode != QLatin1String("direct"))
        req.insert(QStringLiteral("termbase"), QJsonObject{{QStringLiteral("name"), QStringLiteral("%1 (project)").arg(name)}});
    QJsonArray files;
    for (const Doc &d : std::as_const(m_docs)) {
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
    setBusy(true, tr("Making the project…"));
    onResult(c->api()->post(QStringLiteral("/api/projects"), req), this, [this, c, s](const ApiResult &r) {
        if (!r.ok()) {
            setBusy(false, r.error.message);
            QList<Problem> ps = r.error.problems;
            // Problems of the request's own fields are said in the setup too.
            for (Problem &p : ps) {
                if (p.field.startsWith(QLatin1String("settings.")))
                    p.field = p.field.mid(9);
            }
            m_editor->showProblems(ps);
            return;
        }
        m_connectionId = c->id();
        m_projectId = r.object().value(QLatin1String("id")).toInteger();
        if (!s.value(QLatin1String("prepare")).toObject().value(QLatin1String("pretranslate")).toBool()) {
            accept();
            return;
        }
        setBusy(true, tr("Filling segments from the memories…"));
        onResult(c->api()->post(QStringLiteral("/api/projects/%1/pretranslate").arg(m_projectId), QJsonObject{}), this, [this](const ApiResult &) {
            // The project is made either way; the project page shows what
            // was filled.
            accept();
        });
    });
}
