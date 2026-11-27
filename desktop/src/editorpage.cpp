#include "editorpage.h"
#include "documentpreview.h"

#include "options.h"

#include "apiclient.h"
#include "bands.h"
#include "connection.h"
#include "eventfeed.h"
#include "flowlayout.h"
#include "icons.h"
#include "languages.h"
#include "memoriespage.h"
#include "roles.h"
#include "segmentcontent.h"
#include "webui.h"
#include "targeteditor.h"
#include "theme.h"

#include <QHash>
#include <QMap>
#include <QButtonGroup>
#include <QComboBox>
#include <QDateTime>
#include <QDialog>
#include <QDir>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QJsonDocument>
#include <QKeyEvent>
#include <QLabel>
#include <QLineEdit>
#include <QLocale>
#include <QSet>
#include <QTimer>
#include <QMouseEvent>
#include <QProgressBar>
#include <QPushButton>
#include <QCheckBox>
#include <QPlainTextEdit>
#include <QPointer>
#include <QRegularExpression>
#include <QResizeEvent>
#include <QSaveFile>
#include <QScrollArea>
#include <QScrollBar>
#include <QShortcut>
#include <QTextDocumentFragment>
#include <QStandardPaths>
#include <QStyle>
#include <QStyleOptionViewItem>
#include <QTableView>
#include <QTableWidget>
#include <QTextBrowser>
#include <QTextDocument>
#include <QUrlQuery>
#include <QVBoxLayout>

#include <memory>

using SegmentContent::plainText;

namespace {

// The spacing steps of the design system (--s1 … --s5).
constexpr int S1 = 4, S2 = 8, S3 = 12, S4 = 16, S5 = 24;

QString wordsText(int n)
{
    return n == 1 ? QObject::tr("1 word") : QObject::tr("%1 words").arg(QLocale().toString(n));
}

QString plural(int n, const QString &one, const QString &many)
{
    return n == 1 ? QStringLiteral("1 %1").arg(one) : QStringLiteral("%1 %2").arg(n).arg(many);
}

int wordCount(const QJsonArray &content)
{
    static const QRegularExpression word(QStringLiteral("[\\p{L}\\p{N}]+(?:['’.,-][\\p{L}\\p{N}]+)*"));
    int n = 0;
    auto it = word.globalMatch(plainText(content));
    while (it.hasNext()) {
        it.next();
        ++n;
    }
    return n;
}

QString sourceKey(const QJsonArray &content)
{
    return QString::fromUtf8(QJsonDocument(SegmentContent::normalized(content)).toJson(QJsonDocument::Compact));
}

QLabel *label(const QString &text, const char *role, QWidget *parent)
{
    auto *l = new QLabel(text, parent);
    if (role)
        Theme::role(l, role);
    l->setWordWrap(true);
    return l;
}

// A section of the results panel, as the web's .results-section: a title
// with a count, and its content.
QWidget *section(const QString &title, QLabel **count, QWidget *parent, QVBoxLayout **body)
{
    auto *w = new QWidget(parent);
    w->setAccessibleName(title);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S2);
    auto *head = new QHBoxLayout;
    head->setSpacing(S2);
    head->addWidget(label(title, "sectionTitle", w), 0);
    if (count) {
        *count = new QLabel(w);
        Theme::role(*count, "count");
        (*count)->hide();
        head->addWidget(*count);
    }
    head->addStretch();
    v->addLayout(head);
    *body = new QVBoxLayout;
    (*body)->setSpacing(S2);
    v->addLayout(*body);
    return w;
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

// A result card, as the web's .result button: the key, a score or term
// badge, and the source, translation and where it is from.
class ResultCard : public QFrame {
public:
    std::function<void()> activated;
    explicit ResultCard(QWidget *parent) : QFrame(parent)
    {
        setObjectName(QStringLiteral("result"));
        setFocusPolicy(Qt::StrongFocus);
        setCursor(Qt::PointingHandCursor);
    }
    void setSelected(bool on)
    {
        setProperty("selected", on);
        style()->unpolish(this);
        style()->polish(this);
    }

protected:
    void mouseReleaseEvent(QMouseEvent *e) override
    {
        if (rect().contains(e->position().toPoint()) && activated)
            activated();
    }
    void keyPressEvent(QKeyEvent *e) override
    {
        if ((e->key() == Qt::Key_Return || e->key() == Qt::Key_Enter || e->key() == Qt::Key_Space) && activated) {
            activated();
            return;
        }
        QFrame::keyPressEvent(e);
    }
};

} // namespace

// The formats a filter of the core reads as a document, and the bilingual
// ones, by the file's name (the web's documentFormat and XLIFF_FILE).
static bool hasSuffix(const QString &name, std::initializer_list<const char *> suffixes)
{
    for (const char *s : suffixes) {
        if (name.endsWith(QLatin1String(s), Qt::CaseInsensitive))
            return true;
    }
    return false;
}

// The kinds of issue a reviewer marks, as the server names them, and what
// they are called; the same list as in the web app.
static QList<std::pair<QString, QString>> issueCategories()
{
    return {{QStringLiteral("accuracy"), EditorPage::tr("Accuracy")},     {QStringLiteral("fluency"), EditorPage::tr("Fluency")},
            {QStringLiteral("terminology"), EditorPage::tr("Terminology")}, {QStringLiteral("style"), EditorPage::tr("Style")},
            {QStringLiteral("locale"), EditorPage::tr("Locale conventions")}, {QStringLiteral("formatting"), EditorPage::tr("Formatting")},
            {QStringLiteral("other"), EditorPage::tr("Other")}};
}

// How much an issue weighs: its name and its penalty points.
static QList<std::tuple<QString, QString, int>> issueSeverities()
{
    return {{QStringLiteral("minor"), EditorPage::tr("Minor"), 1}, {QStringLiteral("major"), EditorPage::tr("Major"), 5}, {QStringLiteral("critical"), EditorPage::tr("Critical"), 25}};
}

// Content without the whitespace at its end, or at its start.
static QJsonArray trimmedContent(QJsonArray c, bool end)
{
    if (c.isEmpty())
        return c;
    const int i = end ? int(c.size()) - 1 : 0;
    QJsonObject run = c.at(i).toObject();
    if (!run.contains(QLatin1String("text")))
        return c;
    QString t = run.value(QLatin1String("text")).toString();
    if (end) {
        while (!t.isEmpty() && t.back().isSpace())
            t.chop(1);
    } else {
        while (!t.isEmpty() && t.front().isSpace())
            t.remove(0, 1);
    }
    run.insert(QStringLiteral("text"), t);
    c.replace(i, run);
    return c;
}

static bool isDocument(const QString &name)
{
    return hasSuffix(name, {".docx", ".docm", ".pptx", ".pptm", ".ppsx", ".xlsx", ".xlsm", ".html", ".htm", ".xhtml", ".idml", ".odt", ".ods", ".odp", ".xml",
                            ".md", ".markdown", ".properties", ".json", ".yaml", ".yml", ".po", ".pot", ".resx", ".srt", ".vtt", ".sbv", ".ass", ".ssa", ".sub", ".ttml", ".dfxp", ".itt", ".stl", ".scc", ".txt", ".csv", ".tsv", ".pdf"});
}

static bool isBilingual(const QString &name)
{
    return hasSuffix(name, {".xlf", ".xliff", ".sdlxliff", ".mqxliff", ".mxliff", ".txlf", ".txml", ".ttx", ".mqxlz", ".rtf"});
}

EditorPage::EditorPage(Connection *connection, qint64 projectId, const QString &file, const QString &lang,
                       QWidget *parent)
    : QWidget(parent)
    , m_conn(connection)
    , m_id(projectId)
    , m_file(file)
    , m_lang(lang)
    , m_model(new SegmentModel(this))
{
    setAccessibleName(tr("Editor"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->setSpacing(0);
    outer->addWidget(buildBar());

    auto *body = new QHBoxLayout;
    body->setContentsMargins(0, 0, 0, 0);
    body->setSpacing(0);

    m_grid = new QTableView(this);
    m_grid->setObjectName(QStringLiteral("segments"));
    m_grid->setAccessibleName(tr("Segments"));
    m_grid->setModel(m_model);
    m_delegate = new SegmentDelegate(m_grid);
    m_delegate->rightToLeft = [this](int column) { return Languages::rtl(column == SegmentModel::Source ? m_sourceLang : m_lang); };
    m_delegate->factory = [this](QWidget *parent, const QModelIndex &index) { return makeCellWidget(parent, index); };
    m_delegate->activeHeight = [this](int column, int width) { return activeHeight(column, width); };
    m_grid->setItemDelegate(m_delegate);
    m_grid->setSelectionMode(QAbstractItemView::NoSelection);
    m_grid->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_grid->setShowGrid(false);
    m_grid->setFrameShape(QFrame::NoFrame);
    m_grid->setWordWrap(true);
    m_grid->horizontalHeader()->hide();
    m_grid->verticalHeader()->hide();
    m_grid->verticalHeader()->setSectionResizeMode(QHeaderView::Interactive);
    m_grid->setVerticalScrollMode(QAbstractItemView::ScrollPerPixel);
    m_grid->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    // .seg: 44 px for the number, two equal columns, 52 px for the status.
    auto *h = m_grid->horizontalHeader();
    h->setSectionResizeMode(SegmentModel::Number, QHeaderView::Fixed);
    h->resizeSection(SegmentModel::Number, 44);
    h->setSectionResizeMode(SegmentModel::Source, QHeaderView::Stretch);
    h->setSectionResizeMode(SegmentModel::Target, QHeaderView::Stretch);
    h->setSectionResizeMode(SegmentModel::State, QHeaderView::Fixed);
    h->resizeSection(SegmentModel::State, 56);
    connect(h, &QHeaderView::sectionResized, this, [this] { m_rowsTimer.start(); });
    // Queued: the current row also moves while rows are hidden, and the
    // model must not change under the view then.
    connect(
        m_grid->selectionModel(), &QItemSelectionModel::currentRowChanged, this,
        [this] {
            const QModelIndex current = m_grid->currentIndex();
            if (current.isValid() && m_model->segment(current.row()).value(QLatin1String("id")).toString() != m_activeId)
                activate(current.row());
        },
        Qt::QueuedConnection);
    connect(m_grid, &QTableView::activated, this, [this] {
        if (m_target)
            m_target->setFocus();
    });
    body->addWidget(m_grid, 1);

    // The preview, beside the segments (.preview-pane).
    m_previewBox = new QWidget(this);
    m_previewBox->setAccessibleName(tr("Preview of the translated document"));
    auto *pv = new QVBoxLayout(m_previewBox);
    pv->setContentsMargins(S4, S3, S4, S3);
    pv->setSpacing(S2);
    // Wrapped: on one line it would make the pane wider than the window
    // has room for, and push what is right of it out of sight.
    auto *previewNote = label(tr("Layout from the document. Untranslated text appears in the source language, grey and underlined. The saved file is exact."), "muted", m_previewBox);
    previewNote->setWordWrap(true);
    pv->addWidget(previewNote);
    m_previewPane = new QTextBrowser(m_previewBox);
    m_previewPane->setOpenLinks(false);
    m_previewPane->setAccessibleName(tr("Preview of the translated document"));
    connect(m_previewPane, &QTextBrowser::anchorClicked, this, [this](const QUrl &url) {
        if (url.scheme() == QLatin1String("seg"))
            goToSegment(url.path());
    });
    // The sheet in the middle of the pane, as wide as it may be.
    auto *sheet = new QHBoxLayout;
    sheet->addStretch(0);
    sheet->addWidget(m_previewPane, 1);
    sheet->addStretch(0);
    pv->addLayout(sheet, 1);
    // A sheet of paper on the pane, as the document is: white in either theme.
    m_previewPane->setObjectName(QStringLiteral("previewPage"));
    m_previewPane->document()->setDocumentMargin(32);
    m_previewPane->setMaximumWidth(680);
    m_previewPane->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Expanding);
    m_previewBox->setStyleSheet(QStringLiteral("QWidget { background: %1; } QTextBrowser#previewPage { background: #ffffff; color: #111111; border: 1px solid %2; border-radius: 2px; }")
                                    .arg(Theme::color("surface2").name(), Theme::color("border").name()));
    m_previewBox->setMinimumWidth(320);
    m_previewBox->hide();
    body->addWidget(m_previewBox, 1);

    body->addWidget(buildPanel());
    outer->addLayout(body, 1);

    // Notices (.toast): over the page, at the bottom, for a while.
    m_toast = new QLabel(this);
    m_toast->setAccessibleName(tr("Notice"));
    m_toast->setStyleSheet(QStringLiteral("QLabel { background: #1f2630; color: #ffffff; border-radius: 8px; padding: 10px 16px; font-weight: 500; }"));
    m_toast->setWordWrap(true);
    m_toast->setMaximumWidth(560);
    m_toast->hide();
    m_toastTimer.setSingleShot(true);
    m_toastTimer.setInterval(4000);
    connect(&m_toastTimer, &QTimer::timeout, m_toast, &QWidget::hide);

    // The web editor's keys (Trados, memoQ and Phrase where they agree).
    auto bind = [this](const QKeySequence &k, std::function<void()> fn) {
        auto *s = new QShortcut(k, this);
        s->setContext(Qt::WidgetWithChildrenShortcut);
        connect(s, &QShortcut::activated, this, fn);
    };
    bind(QKeySequence(Qt::CTRL | Qt::Key_H), [this] { showReplaceDialog(); });
    bind(QKeySequence(Qt::ALT | Qt::SHIFT | Qt::Key_S), [this] {
        if (m_split && m_split->isEnabled() && m_cutBox->isVisibleTo(this))
            showSplitDialog();
    });
    bind(QKeySequence(Qt::ALT | Qt::SHIFT | Qt::Key_J), [this] {
        if (m_join && m_join->isEnabled() && m_cutBox->isVisibleTo(this))
            resegment(QStringLiteral("join"));
    });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Return), [this] { confirm(); });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Enter), [this] { confirm(); });
    bind(QKeySequence(Qt::ALT | Qt::Key_Down), [this] { move(1); });
    bind(QKeySequence(Qt::ALT | Qt::Key_Up), [this] { move(-1); });
    for (int n = 1; n <= 9; ++n)
        bind(QKeySequence(Qt::CTRL | Qt::Key(Qt::Key_0 + n)), [this, n] { insertResult(n); });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Down), [this] {
        if (!m_results.isEmpty()) {
            m_selected = qMin(m_selected + 1, int(m_results.size()) - 1);
            showResults();
        }
    });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Up), [this] {
        if (!m_results.isEmpty()) {
            m_selected = qMax(m_selected - 1, 0);
            showResults();
        }
    });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Space), [this] {
        if (!m_results.isEmpty())
            insertResult(qMin(m_selected, int(m_results.size()) - 1) + 1);
    });
    bind(QKeySequence(Qt::CTRL | Qt::Key_Insert), [this] { copySource(); });
    bind(QKeySequence(Qt::CTRL | Qt::Key_E), [this] { openCapture(); });
    bind(QKeySequence(Qt::CTRL | Qt::Key_K), [this] { openLookup(); });
    bind(QKeySequence(Qt::Key_F3), [this] { openLookup(); });
    bind(QKeySequence(Qt::Key_F9), [this] { insertNextTag(); });

    m_checkTimer.setSingleShot(true);
    m_checkTimer.setInterval(500);
    connect(&m_checkTimer, &QTimer::timeout, this, &EditorPage::runChecks);
    // What is typed is kept as a draft once typing pauses, as the web
    // editor keeps every change.
    m_saveTimer.setSingleShot(true);
    m_saveTimer.setInterval(1500);
    connect(&m_saveTimer, &QTimer::timeout, this, &EditorPage::saveActive);
    m_presenceTimer.setInterval(30000);
    connect(&m_presenceTimer, &QTimer::timeout, this, &EditorPage::sendPresence);
    m_presenceTimer.start();
    m_refreshTimer.setSingleShot(true);
    m_refreshTimer.setInterval(250);
    connect(&m_refreshTimer, &QTimer::timeout, this, &EditorPage::refreshFromServer);
    m_searchTimer.setSingleShot(true);
    m_searchTimer.setInterval(250);
    connect(&m_searchTimer, &QTimer::timeout, this, &EditorPage::searchMemory);
    m_rowsTimer.setSingleShot(true);
    m_rowsTimer.setInterval(60);
    connect(&m_rowsTimer, &QTimer::timeout, this, [this] {
        m_grid->resizeRowsToContents();
        resizeActiveRow();
    });

    if (m_conn) {
        connect(m_conn->feed(), &EventFeed::event, this, [this](const QString &type, const QJsonObject &data) {
            if (data.value(QLatin1String("projectId")).toInteger() != m_id)
                return;
            if (type == QLatin1String("presence")) {
                reloadPresence();
                return;
            }
            if (type != QLatin1String("project"))
                return;
            const QString kind = data.value(QLatin1String("kind")).toString();
            if (kind == QLatin1String("segment-changed") && data.value(QLatin1String("file")).toString() == m_file
                && data.value(QLatin1String("targetLang")).toString() == m_lang) {
                const QJsonObject mine = m_model->segment(m_model->rowOf(data.value(QLatin1String("segment")).toString()));
                if (data.value(QLatin1String("version")).toInt() > mine.value(QLatin1String("version")).toInt())
                    m_refreshTimer.start();
            } else if ((kind == QLatin1String("segment-commented") || kind == QLatin1String("segment-reviewed")) && data.value(QLatin1String("file")).toString() == m_file
                       && data.value(QLatin1String("targetLang")).toString() == m_lang) {
                // Only the comments or the issues changed: the translations here stay as they are.
                refreshComments();
                if (kind == QLatin1String("segment-reviewed"))
                    loadQuality();
            } else if (kind == QLatin1String("file-resegmented") && data.value(QLatin1String("file")).toString() == m_file
                       && data.value(QLatin1String("targetLang")).toString() == m_lang) {
                // Someone split or joined segments: the file's list is another one.
                const int row = m_model->rowOf(data.value(QLatin1String("segment")).toString());
                if (row >= 0 && m_model->segment(row).value(QLatin1String("version")).toInt() >= data.value(QLatin1String("version")).toInt())
                    return;
                const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
                onResult(m_conn->api()->get(projectPath() + QStringLiteral("/segments"), q), this, [this](const ApiResult &r) {
                    if (!r.ok())
                        return;
                    // What is being typed stays, if its segment is still the one it was.
                    const QString was = m_activeId;
                    const int version = m_model->segment(activeRow()).value(QLatin1String("version")).toInt();
                    const bool typing = dirty();
                    const QJsonArray typed = typing ? m_target->content() : QJsonArray();
                    takeSegments(r.array(), qMax(0, activeRow()));
                    if (const int now = m_model->rowOf(was); now >= 0) {
                        goToSegment(was);
                        if (typing && m_target && m_model->segment(now).value(QLatin1String("version")).toInt() == version)
                            m_target->setContent(typed);
                    }
                });
            } else if (kind == QLatin1String("settings-changed")) {
                loadProject();
            } else if (kind == QLatin1String("file-removed") || kind == QLatin1String("project-deleted")) {
                m_refreshTimer.start();
            }
        });
        connect(m_conn->feed(), &EventFeed::reset, &m_refreshTimer, qOverload<>(&QTimer::start));
    }

    loadProject();
    load();
}

EditorPage::~EditorPage()
{
    if (m_conn && m_conn->isReady() && !m_activeId.isEmpty())
        m_conn->api()->del(projectPath() + QStringLiteral("/presence"),
                           {{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}});
}

QWidget *EditorPage::buildBar()
{
    // .editor-bar: one row, 12 px by 24 px of padding, 16 px between.
    // The file and the search stay on the first line; the actions follow
    // on it while there is room, and go on in lines of their own when the
    // window is narrower (flex-wrap).
    auto *bar = new QFrame(this);
    bar->setObjectName(QStringLiteral("editorBar"));
    auto *lines = new FlowLayout(bar, S2);
    lines->setContentsMargins(S5, S3, S5, S3);
    auto *first = new QWidget(bar);
    auto *row = new QHBoxLayout(first);
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(S4);
    lines->addWidget(first);

    auto *toProjects = new QPushButton(Icons::get(QStringLiteral("arrow-left"), Theme::color("accentText")), tr("Projects"), bar);
    Theme::smallButton(toProjects);
    Theme::quietButton(toProjects);
    toProjects->setIconSize(QSize(14, 14));
    connect(toProjects, &QPushButton::clicked, this, [this] {
        saveActive();
        emit back();
    });
    row->addWidget(toProjects);

    auto *titleBox = new QVBoxLayout;
    titleBox->setSpacing(0);
    m_title = new QLabel(bar);
    Theme::role(m_title, "h1");
    titleBox->addWidget(m_title);
    auto *meta = new QHBoxLayout;
    meta->setSpacing(S3);
    m_fileChoice = new QComboBox(bar);
    m_fileChoice->setProperty("small", true);
    m_fileChoice->setAccessibleName(tr("File"));
    m_fileChoice->hide();
    connect(m_fileChoice, &QComboBox::activated, this, [this] {
        const QString f = m_fileChoice->currentData().toString();
        if (f != m_file) {
            saveActive();
            emit openFile(f, m_lang);
        }
    });
    m_fileName = new QLabel(QFileInfo(m_file).fileName(), bar);
    Theme::role(m_fileName, "meta");
    meta->addWidget(m_fileChoice);
    meta->addWidget(m_fileName);
    auto *pair = new QHBoxLayout;
    pair->setSpacing(S1);
    m_sourceChip = new QLabel(bar);
    m_sourceChip->setProperty("chip", QStringLiteral("lang"));
    m_sourceChip->setFixedHeight(18);
    auto *arrow = new QLabel(QStringLiteral("→"), bar);
    Theme::role(arrow, "meta");
    m_targetChip = new QLabel(m_lang, bar);
    m_targetChip->setProperty("chip", QStringLiteral("lang"));
    m_targetChip->setFixedHeight(18);
    m_langChoice = new QComboBox(bar);
    m_langChoice->setProperty("small", true);
    m_langChoice->setAccessibleName(tr("Target language"));
    m_langChoice->hide();
    connect(m_langChoice, &QComboBox::activated, this, [this] {
        const QString l = m_langChoice->currentData().toString();
        if (l != m_lang) {
            saveActive();
            emit openFile(m_file, l);
        }
    });
    pair->addWidget(m_sourceChip);
    pair->addWidget(arrow);
    pair->addWidget(m_targetChip);
    pair->addWidget(m_langChoice);
    meta->addLayout(pair);
    meta->addStretch();
    titleBox->addLayout(meta);
    row->addLayout(titleBox, 1);

    // .editor-progress: a 6 px track and the words confirmed.
    auto *progress = new QWidget(bar);
    progress->setMinimumWidth(180);
    auto *pl = new QVBoxLayout(progress);
    pl->setContentsMargins(0, 0, 0, 0);
    pl->setSpacing(3);
    m_progressTrack = new QProgressBar(progress);
    m_progressTrack->setObjectName(QStringLiteral("track"));
    m_progressTrack->setTextVisible(false);
    m_progressText = new QLabel(progress);
    Theme::role(m_progressText, "progress");
    pl->addWidget(m_progressTrack);
    pl->addWidget(m_progressText);
    row->addWidget(progress);

    // .segmented: All, To do, Warnings.
    auto *seg = new QFrame(bar);
    seg->setObjectName(QStringLiteral("segmented"));
    seg->setAccessibleName(tr("Show segments"));
    auto *sl = new QHBoxLayout(seg);
    sl->setContentsMargins(2, 2, 2, 2);
    sl->setSpacing(0);
    m_show = new QButtonGroup(this);
    const std::pair<QString, SegmentFilter::Show> filters[] = {
        {tr("All"), SegmentFilter::Show::All}, {tr("To do"), SegmentFilter::Show::ToDo}, {tr("Warnings"), SegmentFilter::Show::Warnings}};
    for (const auto &[text, show] : filters) {
        auto *b = new QPushButton(text, seg);
        b->setCheckable(true);
        b->setChecked(show == SegmentFilter::Show::All);
        m_show->addButton(b, int(show));
        sl->addWidget(b);
        if (show == SegmentFilter::Show::Warnings)
            m_warningsFilter = b;
    }
    connect(m_show, &QButtonGroup::idClicked, this, [this](int id) {
        m_filter.setShow(SegmentFilter::Show(id));
        applyFilter();
    });
    row->addWidget(seg);

    // .editor-find: text the segments shown must hold.
    m_find = new QLineEdit(bar);
    m_find->setObjectName(QStringLiteral("findInSegments"));
    m_find->setPlaceholderText(tr("Find in the segments"));
    m_find->setAccessibleName(tr("Find in the segments"));
    m_find->setClearButtonEnabled(true);
    m_find->setFixedWidth(200);
    m_find->setProperty("small", true);
    m_found = new QLabel(bar);
    Theme::role(m_found, "cellSub");
    m_found->setObjectName(QStringLiteral("foundSegments"));
    m_found->hide();
    connect(m_find, &QLineEdit::textChanged, this, [this](const QString &t) {
        m_filter.setText(t);
        applyFilter();
    });
    row->addWidget(m_find);
    row->addWidget(m_found);

    // .editor-actions.
    auto *actions = lines;
    auto action = [bar, actions](const QString &icon, const QString &text) {
        auto *b = new QPushButton(Icons::get(icon), text, bar);
        Theme::smallButton(b);
        b->setIconSize(QSize(14, 14));
        actions->addWidget(b);
        return b;
    };
    m_review = action(QStringLiteral("git-merge"), tr("Review and merge"));
    m_review->hide();
    connect(m_review, &QPushButton::clicked, this, [this] { emit openReview(m_reviewTermbase); });
    connect(action(QStringLiteral("settings"), tr("Project settings")), &QPushButton::clicked, this, [this] {
        saveActive();
        emit openProject();
    });
    auto *replace = action(QStringLiteral("replace"), tr("Replace"));
    replace->setObjectName(QStringLiteral("replaceButton"));
    connect(replace, &QPushButton::clicked, this, [this] { showReplaceDialog(); });
    connect(action(QStringLiteral("memories"), tr("Memories")), &QPushButton::clicked, this, &EditorPage::showMemoriesDialog);
    connect(action(QStringLiteral("bar-chart"), tr("Analysis")), &QPushButton::clicked, this, &EditorPage::showAnalysisDialog);
    m_preview = action(QStringLiteral("eye"), tr("Preview"));
    m_preview->setCheckable(true);
    m_preview->setVisible(isDocument(m_file));
    connect(m_preview, &QPushButton::toggled, this, [this](bool on) {
        m_previewBox->setVisible(on);
        if (on) {
            loadLayout();
            updatePreview();
        }
        m_rowsTimer.start();
    });
    m_save = action(QStringLiteral("download"), tr("Save translated document"));
    m_save->hide();
    connect(m_save, &QPushButton::clicked, this, &EditorPage::saveDocument);
    return bar;
}

QWidget *EditorPage::buildPanel()
{
    // .results: 360 px, 16 px in, 16 px between sections, on the page
    // background with a line to the left.
    m_panel = new QScrollArea(this);
    m_panel->setObjectName(QStringLiteral("results"));
    m_panel->setAccessibleName(tr("Translation results"));
    m_panel->setFixedWidth(360);
    m_panel->setWidgetResizable(true);
    m_panel->setFrameShape(QFrame::NoFrame);
    m_panel->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    auto *body = new QWidget(m_panel);
    body->setObjectName(QStringLiteral("resultsBody"));
    auto *v = new QVBoxLayout(body);
    v->setContentsMargins(S4, S4, S4, S4);
    v->setSpacing(S4);

    // Someone else saved the segment being worked on.
    m_conflict = new QFrame(body);
    m_conflict->setObjectName(QStringLiteral("issue"));
    auto *cl = new QVBoxLayout(m_conflict);
    cl->setContentsMargins(S3, S2, S3, S2);
    m_conflictText = label(QString(), nullptr, m_conflict);
    cl->addWidget(m_conflictText);
    auto *crow = new QHBoxLayout;
    auto *keep = new QPushButton(tr("Keep mine"), m_conflict);
    auto *take = new QPushButton(tr("Take theirs"), m_conflict);
    for (QPushButton *b : {keep, take}) {
        Theme::smallButton(b);
        crow->addWidget(b);
    }
    crow->addStretch();
    cl->addLayout(crow);
    m_conflict->hide();
    v->addWidget(m_conflict);
    connect(keep, &QPushButton::clicked, this, [this] {
        const QString id = m_conflictId;
        const QJsonObject theirs = m_conflicts.take(id);
        m_conflict->hide();
        if (theirs.isEmpty() || !m_target)
            return;
        // Saved over theirs, from their version.
        QJsonObject s = m_model->segment(m_model->rowOf(id));
        s.insert(QStringLiteral("version"), theirs.value(QLatin1String("version")));
        m_model->update(s);
        save(id, m_target->content(), QStringLiteral("draft"), {}, m_origin);
    });
    connect(take, &QPushButton::clicked, this, [this] {
        const QString id = m_conflictId;
        const QJsonObject theirs = m_conflicts.take(id);
        m_conflict->hide();
        if (theirs.isEmpty())
            return;
        m_model->update(theirs);
        m_model->clearPending(id);
        if (id == m_activeId && m_target) {
            m_loadedTarget = theirs.value(QLatin1String("target")).toArray();
            m_target->setContent(m_loadedTarget);
        }
        notify(tr("Their translation is in."));
    });

    {
        QVBoxLayout *resultsBody = nullptr;
        v->addWidget(section(tr("Translation results"), &m_resultsCount, body, &resultsBody));
        m_resultsList = new QVBoxLayout;
        m_resultsList->setSpacing(S2);
        resultsBody->addLayout(m_resultsList);
        // Under the results: the translation services, and what one answered.
        m_suggestList = new QVBoxLayout;
        m_suggestList->setSpacing(S2);
        resultsBody->addLayout(m_suggestList);
    }
    v->addWidget(section(tr("Checks"), &m_checksCount, body, &m_issuesList));

    // Memory search, open while in use.
    QVBoxLayout *lookupBody = nullptr;
    m_lookupSection = section(tr("Memory search"), nullptr, body, &lookupBody);
    auto *lrow = new QHBoxLayout;
    lrow->setSpacing(S2);
    auto *icon = new QLabel(m_lookupSection);
    icon->setPixmap(Icons::get(QStringLiteral("search"), Theme::color("text3")).pixmap(14, 14));
    m_lookupField = new QLineEdit(m_lookupSection);
    m_lookupField->setAccessibleName(tr("Search the memory"));
    m_lookupField->setPlaceholderText(tr("Words in either language"));
    auto *close = new QPushButton(Icons::get(QStringLiteral("x")), QString(), m_lookupSection);
    close->setProperty("tiny", true);
    close->setAccessibleName(tr("Close memory search"));
    lrow->addWidget(icon);
    lrow->addWidget(m_lookupField, 1);
    lrow->addWidget(close);
    lookupBody->addLayout(lrow);
    m_lookupList = new QVBoxLayout;
    m_lookupList->setSpacing(S2);
    lookupBody->addLayout(m_lookupList);
    connect(m_lookupField, &QLineEdit::textChanged, &m_searchTimer, qOverload<>(&QTimer::start));
    auto closeLookup = [this] {
        m_lookupSection->hide();
        if (m_target)
            m_target->setFocus();
    };
    connect(close, &QPushButton::clicked, this, closeLookup);
    auto *esc = new QShortcut(QKeySequence(Qt::Key_Escape), m_lookupSection);
    esc->setContext(Qt::WidgetWithChildrenShortcut);
    connect(esc, &QShortcut::activated, this, closeLookup);
    m_lookupSection->hide();
    v->addWidget(m_lookupSection);

    // Scored review: the issues marked on the open segment, a form for a
    // new one, and the file's score.
    QVBoxLayout *reviewBody = nullptr;
    v->addWidget(section(tr("Review"), &m_reviewCount, body, &reviewBody));
    m_reviewList = new QVBoxLayout;
    m_reviewList->setSpacing(S2);
    reviewBody->addLayout(m_reviewList);
    m_issueCategory = new QComboBox(body);
    m_issueCategory->setObjectName(QStringLiteral("issueCategory"));
    for (const auto &[id, name] : issueCategories())
        m_issueCategory->addItem(name, id);
    m_issueSeverity = new QComboBox(body);
    m_issueSeverity->setObjectName(QStringLiteral("issueSeverity"));
    for (const auto &[id, name, points] : issueSeverities())
        m_issueSeverity->addItem(tr("%1 (%2)").arg(name, points == 1 ? tr("1 point") : tr("%1 points").arg(points)), id);
    // Side by side, each half of the panel wide, whatever their longest choice.
    for (QComboBox *c : {m_issueCategory, m_issueSeverity}) {
        c->setSizeAdjustPolicy(QComboBox::AdjustToMinimumContentsLengthWithIcon);
        c->setMinimumContentsLength(6);
        c->setSizePolicy(QSizePolicy::Ignored, QSizePolicy::Fixed);
    }
    auto *kinds = new QHBoxLayout;
    kinds->setSpacing(S2);
    kinds->addWidget(Web::field(tr("Kind of issue"), m_issueCategory, body), 1);
    kinds->addWidget(Web::field(tr("Severity"), m_issueSeverity, body), 1);
    reviewBody->addLayout(kinds);
    m_issueNote = new QLineEdit(body);
    m_issueNote->setObjectName(QStringLiteral("issueNote"));
    m_issueNote->setAccessibleName(tr("What is wrong, if you want to say"));
    m_issueNote->setPlaceholderText(tr("What is wrong (optional)"));
    m_issueNote->setMaxLength(1000);
    reviewBody->addWidget(m_issueNote);
    m_issueAdd = new QPushButton(Icons::get(QStringLiteral("flag")), tr("Mark issue"), body);
    m_issueAdd->setObjectName(QStringLiteral("addIssue"));
    Theme::smallButton(m_issueAdd);
    m_issueAdd->setIconSize(QSize(14, 14));
    reviewBody->addWidget(m_issueAdd, 0, Qt::AlignLeft);
    const auto mark = [this] { addIssue(m_issueCategory->currentData().toString(), m_issueSeverity->currentData().toString(), m_issueNote->text()); };
    connect(m_issueAdd, &QPushButton::clicked, this, mark);
    connect(m_issueNote, &QLineEdit::returnPressed, this, mark);
    m_qualityScore = new QLabel(body);
    m_qualityScore->setObjectName(QStringLiteral("qualityScore"));
    m_qualityScore->setTextFormat(Qt::RichText);
    m_qualityText = label(QString(), "help", body);
    m_qualityText->setObjectName(QStringLiteral("qualityText"));
    m_qualityText->setWordWrap(true);
    auto *saveReview = new QPushButton(tr("Save the project's review as CSV"), body);
    saveReview->setObjectName(QStringLiteral("saveReview"));
    saveReview->setProperty("stepLink", true);
    saveReview->setFlat(true);
    saveReview->setCursor(Qt::PointingHandCursor);
    connect(saveReview, &QPushButton::clicked, this, [this] {
        const QString name = m_project.value(QLatin1String("settings")).toObject().value(QLatin1String("name")).toString() + QStringLiteral(" review.csv");
        const QString to = QFileDialog::getSaveFileName(this, tr("Save the project's review as CSV"), QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation) + QLatin1Char('/') + name);
        if (!to.isEmpty())
            saveReviewTo(to);
    });
    reviewBody->addWidget(m_qualityScore);
    reviewBody->addWidget(m_qualityText);
    reviewBody->addWidget(saveReview, 0, Qt::AlignLeft);
    m_qualityScore->hide();
    m_qualityText->hide();

    // Comments on the open segment, and a field for a new one.
    QVBoxLayout *commentsBody = nullptr;
    v->addWidget(section(tr("Comments"), &m_commentsCount, body, &commentsBody));
    m_commentsList = new QVBoxLayout;
    m_commentsList->setSpacing(S2);
    commentsBody->addLayout(m_commentsList);
    m_commentText = new QPlainTextEdit(body);
    m_commentText->setObjectName(QStringLiteral("commentText"));
    m_commentText->setAccessibleName(tr("A comment on this segment"));
    m_commentText->setPlaceholderText(tr("A comment for those working on the project"));
    m_commentText->setTabChangesFocus(true);
    m_commentText->setFixedHeight(56);
    commentsBody->addWidget(m_commentText);
    m_commentAdd = new QPushButton(Icons::get(QStringLiteral("comment")), tr("Add comment"), body);
    m_commentAdd->setObjectName(QStringLiteral("addComment"));
    Theme::smallButton(m_commentAdd);
    m_commentAdd->setIconSize(QSize(14, 14));
    m_commentAdd->setEnabled(false);
    commentsBody->addWidget(m_commentAdd, 0, Qt::AlignLeft);
    connect(m_commentText, &QPlainTextEdit::textChanged, this, [this] { m_commentAdd->setEnabled(!m_commentText->toPlainText().trimmed().isEmpty() && !m_activeId.isEmpty()); });
    connect(m_commentAdd, &QPushButton::clicked, this, [this] { addComment(m_commentText->toPlainText()); });

    // Add a term: the intro and its buttons, or the form.
    QVBoxLayout *termBody = nullptr;
    v->addWidget(section(tr("Add a term"), nullptr, body, &termBody));
    m_captureIntro = new QWidget(body);
    auto *il = new QVBoxLayout(m_captureIntro);
    il->setContentsMargins(0, 0, 0, 0);
    il->setSpacing(S2);
    il->addWidget(label(tr("Select the term in the source and its translation in the target, then add it. The sentence is saved with it."),
                        "muted", m_captureIntro));
    auto *irowBox = new QWidget(m_captureIntro);
    auto *irow = new FlowLayout(irowBox, S2);
    QPushButton *addTerm = Theme::keyButton(tr("Add term"), QStringLiteral("Ctrl+E"), QStringLiteral("plus"), m_captureIntro);
    QPushButton *search = Theme::keyButton(tr("Search memory"), QStringLiteral("Ctrl+K"), QStringLiteral("search"), m_captureIntro);
    connect(addTerm, &QPushButton::clicked, this, &EditorPage::openCapture);
    connect(search, &QPushButton::clicked, this, [this] { openLookup(); });
    irow->addWidget(addTerm);
    irow->addWidget(search);
    il->addWidget(irowBox);
    termBody->addWidget(m_captureIntro);

    m_captureForm = new QWidget(body);
    auto *fl = new QVBoxLayout(m_captureForm);
    fl->setContentsMargins(0, 0, 0, 0);
    fl->setSpacing(S1);
    auto field = [this, fl](const QString &labelText, const QString &placeholder, const char *name) {
        auto *l = new QLabel(labelText, m_captureForm);
        l->setObjectName(QLatin1String(name) + QStringLiteral("Label"));
        auto *e = new QLineEdit(m_captureForm);
        e->setObjectName(QLatin1String(name));
        e->setPlaceholderText(placeholder);
        e->setAccessibleName(labelText);
        l->setBuddy(e);
        fl->addWidget(l);
        fl->addWidget(e);
        fl->addSpacing(S1);
        return e;
    };
    m_captureSource = field(QString(), tr("Select it in the source, or type"), "captureSource");
    m_captureTarget = field(QString(), tr("Select it in your translation, or type"), "captureTarget");
    m_captureNote = field(tr("Note (optional)"), QString(), "captureNote");
    m_captureHelp = label(QString(), "secondary", m_captureForm);
    fl->addWidget(m_captureHelp);
    auto *frow = new QHBoxLayout;
    frow->setSpacing(S2);
    m_captureAdd = new QPushButton(tr("Add term"), m_captureForm);
    Theme::primary(m_captureAdd);
    Theme::smallButton(m_captureAdd);
    auto *cancel = new QPushButton(tr("Cancel"), m_captureForm);
    Theme::smallButton(cancel);
    Theme::quietButton(cancel);
    frow->addWidget(m_captureAdd);
    frow->addWidget(cancel);
    frow->addStretch();
    fl->addLayout(frow);
    m_captureForm->hide();
    termBody->addWidget(m_captureForm);
    auto validate = [this] {
        m_captureAdd->setEnabled(!m_captureSource->text().trimmed().isEmpty() && !m_captureTarget->text().trimmed().isEmpty());
    };
    connect(m_captureSource, &QLineEdit::textChanged, this, validate);
    connect(m_captureTarget, &QLineEdit::textChanged, this, validate);
    auto closeCapture = [this] {
        m_captureForm->hide();
        m_captureIntro->show();
        if (m_target)
            m_target->setFocus();
    };
    connect(cancel, &QPushButton::clicked, this, closeCapture);
    auto *capEsc = new QShortcut(QKeySequence(Qt::Key_Escape), m_captureForm);
    capEsc->setContext(Qt::WidgetWithChildrenShortcut);
    connect(capEsc, &QShortcut::activated, this, closeCapture);
    for (QLineEdit *e : {m_captureSource, m_captureTarget, m_captureNote})
        connect(e, &QLineEdit::returnPressed, m_captureAdd, &QPushButton::click);
    connect(m_captureAdd, &QPushButton::clicked, this, [this, closeCapture] {
        const QString src = m_captureSource->text().trimmed();
        const QString tgt = m_captureTarget->text().trimmed();
        if (src.isEmpty() || tgt.isEmpty() || !m_conn || m_activeId.isEmpty())
            return;
        const QJsonArray sourceFields{
            QJsonObject{{QStringLiteral("field"), QStringLiteral("context")},
                        {QStringLiteral("value"), plainText(m_model->segment(activeRow()).value(QLatin1String("source")).toArray())}}};
        QJsonArray fields;
        if (const QString note = m_captureNote->text().trimmed(); !note.isEmpty())
            fields.append(QJsonObject{{QStringLiteral("field"), QStringLiteral("note")}, {QStringLiteral("value"), note}});
        const QJsonObject entry{
            {QStringLiteral("fields"), fields},
            {QStringLiteral("languages"),
             QJsonArray{QJsonObject{{QStringLiteral("lang"), m_sourceLang},
                                    {QStringLiteral("fields"), sourceFields},
                                    {QStringLiteral("terms"),
                                     QJsonArray{QJsonObject{{QStringLiteral("text"), src}, {QStringLiteral("status"), QStringLiteral("proposed")}}}}},
                        QJsonObject{{QStringLiteral("lang"), m_lang},
                                    {QStringLiteral("terms"),
                                     QJsonArray{QJsonObject{{QStringLiteral("text"), tgt}, {QStringLiteral("status"), QStringLiteral("proposed")}}}}}}}};
        onResult(m_conn->api()->post(projectPath() + QStringLiteral("/terms"), entry), this, [this, src, closeCapture](const ApiResult &r) {
            if (!r.ok()) {
                notify(tr("The term was not added: %1").arg(r.error.message));
                return;
            }
            notify(tr("Added “%1” to %2").arg(src, r.object().value(QLatin1String("termbase")).toString()));
            closeCapture();
            recognize();
            updateReview();
        });
    });

    // Copy source, next tag, confirm.
    auto *toolsBox = new QWidget(body);
    auto *tools = new FlowLayout(toolsBox, S2);
    QPushButton *copy = Theme::keyButton(tr("Copy source"), QStringLiteral("Ctrl+Ins"), QStringLiteral("copy"), body);
    m_nextTag = Theme::keyButton(tr("Next tag"), QStringLiteral("F9"), QStringLiteral("tag"), body);
    QPushButton *confirmButton = Theme::keyButton(tr("Confirm"), QStringLiteral("Ctrl+Enter"), QStringLiteral("check"), body);
    Theme::primary(confirmButton);
    // On the accent, the text, icon and key chip are white.
    confirmButton->setStyleSheet(QStringLiteral("QLabel { color: #ffffff; background: transparent; }"
                                                " QLabel[kbd=\"true\"] { border: 1px solid rgba(255,255,255,0.45); border-bottom-width: 2px;"
                                                " background: rgba(255,255,255,0.12); color: #ffffff; }"));
    for (QLabel *l : confirmButton->findChildren<QLabel *>()) {
        if (!l->pixmap().isNull())
            l->setPixmap(Icons::get(QStringLiteral("check"), QColor(Qt::white)).pixmap(14, 14));
    }
    connect(copy, &QPushButton::clicked, this, &EditorPage::copySource);
    connect(m_nextTag, &QPushButton::clicked, this, &EditorPage::insertNextTag);
    connect(confirmButton, &QPushButton::clicked, this, &EditorPage::confirm);
    tools->addWidget(copy);
    tools->addWidget(m_nextTag);
    tools->addWidget(confirmButton);
    v->addWidget(toolsBox);

    // Split and join: in documents and in plain XLIFF files.
    m_cutBox = new QWidget(body);
    auto *cutV = new QVBoxLayout(m_cutBox);
    cutV->setContentsMargins(0, 0, 0, 0);
    cutV->setSpacing(S2);
    auto *cutRow = new QWidget(m_cutBox);
    auto *cuts = new FlowLayout(cutRow, S2);
    m_split = Theme::keyButton(tr("Split"), QStringLiteral("Alt+Shift+S"), QStringLiteral("scissors"), body);
    m_join = Theme::keyButton(tr("Join with next"), QStringLiteral("Alt+Shift+J"), QStringLiteral("combine"), body);
    m_split->setObjectName(QStringLiteral("splitSegment"));
    m_join->setObjectName(QStringLiteral("joinSegment"));
    connect(m_split, &QPushButton::clicked, this, [this] { showSplitDialog(); });
    connect(m_join, &QPushButton::clicked, this, [this] { resegment(QStringLiteral("join")); });
    cuts->addWidget(m_split);
    cuts->addWidget(m_join);
    cutV->addWidget(cutRow);
    m_cutNote = label(QString(), "help", m_cutBox);
    m_cutNote->setObjectName(QStringLiteral("cutNote"));
    m_cutNote->setWordWrap(true);
    cutV->addWidget(m_cutNote);
    m_cutBox->setVisible(isDocument(m_file) || isBilingual(m_file));
    v->addWidget(m_cutBox);

    // Keyboard shortcuts, folded (.shortcuts).
    auto *keysToggle = new QPushButton(Icons::get(QStringLiteral("keyboard"), Theme::color("accentText")), tr("Keyboard shortcuts"), body);
    Theme::smallButton(keysToggle);
    Theme::quietButton(keysToggle);
    keysToggle->setCheckable(true);
    keysToggle->setIconSize(QSize(14, 14));
    v->addWidget(keysToggle, 0, Qt::AlignLeft);
    m_shortcuts = new QWidget(body);
    auto *grid = new QGridLayout(m_shortcuts);
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setHorizontalSpacing(S3);
    grid->setVerticalSpacing(S1);
    const std::pair<const char *, const char *> keys[] = {
        {"Ctrl+Enter", QT_TR_NOOP("Confirm, fill repetitions and go to the next open segment")},
        {"Alt+↓ / Alt+↑", QT_TR_NOOP("Next / previous segment")},
        {"Ctrl+1 to 9", QT_TR_NOOP("Insert result 1 to 9 (a memory match replaces the target, a term goes in at the caret)")},
        {"Ctrl+↓ / Ctrl+↑, Ctrl+Space", QT_TR_NOOP("Move through the results, insert the highlighted one")},
        {"Ctrl+Insert", QT_TR_NOOP("Copy source to target")},
        {"F9", QT_TR_NOOP("Insert the next missing tag")},
        {"Ctrl+E", QT_TR_NOOP("Add a term from the selections")},
        {"Ctrl+H", QT_TR_NOOP("Find and replace in the translations")},
        {"Alt+Shift+S, Alt+Shift+J", QT_TR_NOOP("Split the segment at any place, join it with the next one of its paragraph")},
        {"Ctrl+K or F3", QT_TR_NOOP("Search the memory for the selection")},
    };
    int r = 0;
    for (const auto &[k, what] : keys) {
        auto *kl = new QLabel(QString::fromUtf8(k), m_shortcuts);
        QFont f = kl->font();
        f.setWeight(QFont::DemiBold);
        f.setPixelSize(12);
        kl->setFont(f);
        grid->addWidget(kl, r, 0, Qt::AlignTop);
        grid->addWidget(label(tr(what), "secondary", m_shortcuts), r, 1);
        ++r;
    }
    grid->addWidget(label(tr("These are the shortcuts usual in translation tools."), "secondary", m_shortcuts), r, 0, 1, 2);
    grid->setColumnStretch(1, 1);
    m_shortcuts->hide();
    connect(keysToggle, &QPushButton::toggled, m_shortcuts, &QWidget::setVisible);
    v->addWidget(m_shortcuts);
    v->addStretch();
    m_panel->setWidget(body);
    Web::fitScroll(m_panel);
    return m_panel;
}

QString EditorPage::projectPath() const
{
    return QStringLiteral("/api/projects/%1").arg(m_id);
}

int EditorPage::activeRow() const
{
    return m_model->rowOf(m_activeId);
}

int EditorPage::shownCount() const
{
    int n = 0;
    for (int row = 0; row < m_model->rowCount(); ++row) {
        if (!m_grid->isRowHidden(row))
            ++n;
    }
    return n;
}

bool EditorPage::eventFilter(QObject *watched, QEvent *event)
{
    if (watched == m_target && event->type() == QEvent::FocusIn && activeRow() >= 0)
        m_grid->scrollTo(m_model->index(activeRow(), SegmentModel::Target));
    return QWidget::eventFilter(watched, event);
}

void EditorPage::resizeEvent(QResizeEvent *event)
{
    QWidget::resizeEvent(event);
    if (m_toast->isVisible())
        m_toast->move((width() - m_toast->width()) / 2, height() - m_toast->height() - S5);
}

void EditorPage::notify(const QString &text)
{
    m_lastMessage = text;
    m_toast->setText(text);
    m_toast->adjustSize();
    m_toast->move((width() - m_toast->width()) / 2, height() - m_toast->height() - S5);
    m_toast->raise();
    m_toast->show();
    m_toastTimer.start();
}

bool EditorPage::editable() const
{
    const int row = activeRow();
    return row >= 0 && m_target && !m_model->segment(row).value(QLatin1String("locked")).toBool();
}

bool EditorPage::dirty() const
{
    return editable() && !SegmentContent::equal(m_target->content(), m_loadedTarget);
}

QJsonArray EditorPage::memorySettings() const
{
    return settings().value(QLatin1String("memories")).toObject().value(m_lang).toArray();
}

void EditorPage::loadProject()
{
    if (!m_conn)
        return;
    onResult(m_conn->api()->get(projectPath()), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        m_project = r.object();
        loadServices();
        const QJsonObject s = settings();
        m_sourceLang = s.value(QLatin1String("sourceLang")).toString();
        m_title->setText(s.value(QLatin1String("name")).toString());
        m_title->setAccessibleName(m_title->text());
        setWindowTitle(m_title->text());
        m_sourceChip->setText(m_sourceLang);
        m_sourceChip->setToolTip(Languages::name(m_sourceLang));
        m_targetChip->setToolTip(Languages::name(m_lang));
        // Several files or languages: choose here, as in the web editor.
        QStringList files;
        QJsonObject fileInfo;
        for (const QJsonValue &v : s.value(QLatin1String("files")).toArray()) {
            const QJsonObject f = v.toObject();
            if (f.value(QLatin1String("role")).toString() != QLatin1String("translatable"))
                continue;
            files << f.value(QLatin1String("name")).toString();
            if (f.value(QLatin1String("name")).toString() == m_file)
                fileInfo = f;
        }
        {
            const QSignalBlocker block(m_fileChoice);
            m_fileChoice->clear();
            for (const QString &f : files)
                m_fileChoice->addItem(QFileInfo(f).fileName(), f);
            m_fileChoice->setCurrentIndex(files.indexOf(m_file));
        }
        m_fileChoice->setVisible(files.size() > 1);
        m_fileName->setVisible(files.size() <= 1);
        const QStringList targets = s.value(QLatin1String("targetLangs")).toVariant().toStringList();
        {
            const QSignalBlocker block(m_langChoice);
            m_langChoice->clear();
            for (const QString &t : targets)
                m_langChoice->addItem(Languages::name(t), t);
            m_langChoice->setCurrentIndex(targets.indexOf(m_lang));
        }
        m_langChoice->setVisible(targets.size() > 1);
        m_targetChip->setVisible(targets.size() <= 1);
        // Saved as the file came: a document, a bilingual file, or a
        // return package for a package's file.
        const bool document = isDocument(m_file) && !m_table;
        const bool xliff = !document && (isBilingual(m_file) || m_table);
        m_save->setVisible(document || xliff);
        if (!fileInfo.value(QLatin1String("package")).toString().isEmpty())
            m_save->setText(tr("Save return package"));
        else if (xliff && hasSuffix(m_file, {".rtf"}))
            m_save->setText(tr("Save RTF"));
        else if (xliff && hasSuffix(m_file, {".docx"}))
            m_save->setText(tr("Save DOCX"));
        else if (xliff)
            m_save->setText(tr("Save XLIFF"));
        else
            m_save->setText(tr("Save translated document"));
        const QJsonObject t = s.value(QLatin1String("terminology")).toObject();
        const QString tb = t.value(QLatin1String("mode")).toString() == QLatin1String("direct") ? t.value(QLatin1String("writeTo")).toString()
                                                                                                : t.value(QLatin1String("termbase")).toString();
        m_captureHelp->setText(tr("Saved as proposed to %1, with this sentence.").arg(tb.isEmpty() ? tr("the termbase") : tb));
        updateReview();
    });
}

void EditorPage::updateReview()
{
    // A layered project termbase's own entries are its changes to the
    // masters, waiting to be reviewed and merged.
    if (!m_conn)
        return;
    const QJsonObject t = settings().value(QLatin1String("terminology")).toObject();
    const QString tb = t.value(QLatin1String("termbase")).toString();
    if (t.value(QLatin1String("mode")).toString() != QLatin1String("layered") || tb.isEmpty()) {
        m_review->hide();
        return;
    }
    onResult(m_conn->api()->get(QStringLiteral("/api/termbases/%1/layered").arg(ApiClient::seg(tb))), this, [this, tb](const ApiResult &r) {
        int own = 0;
        if (r.ok()) {
            for (const QJsonValue &v : r.array()) {
                if (v.toObject().value(QLatin1String("termbase")).toString() == tb)
                    ++own;
            }
        }
        m_reviewTermbase = tb;
        m_review->setText(tr("Review and merge (%1)").arg(own));
        m_review->setVisible(own > 0);
    });
}

void EditorPage::load()
{
    if (!m_conn)
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/segments"), q), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        m_model->setSegments(r.array());
        // A Word document whose segments carry the state of a bilingual
        // file is a table for review (Trados, memoQ): it is saved as the
        // file it is, and has no preview of its own.
        if (!m_table && hasSuffix(m_file, {".docx"}) && m_model->rowCount() > 0
            && m_model->segment(0).value(QLatin1String("file")).isObject()) {
            m_table = true;
            m_preview->setChecked(false);
            m_preview->hide();
            if (m_save->text() == tr("Save translated document"))
                m_save->setText(tr("Save DOCX"));
        }
        applyFilter();
        updateProgress();
        reloadPresence();
        loadQuality();
        m_grid->resizeRowsToContents();
        int first = 0;
        for (int i = 0; i < m_model->rowCount(); ++i) {
            const QJsonObject s = m_model->segment(i);
            if (s.value(QLatin1String("status")).toString() != QLatin1String("confirmed") && !s.value(QLatin1String("locked")).toBool()) {
                first = i;
                break;
            }
        }
        if (m_model->rowCount() > 0)
            goToSegment(m_model->segment(first).value(QLatin1String("id")).toString());
        else
            notify(tr("No text to translate in this file."));
    });
}

void EditorPage::applyFilter()
{
    for (int row = 0; row < m_model->rowCount(); ++row)
        m_grid->setRowHidden(row, !m_filter.accepts(m_model, row));
    int withWarnings = 0;
    for (const QJsonObject &s : m_model->segments()) {
        if (!s.value(QLatin1String("warnings")).toArray().isEmpty())
            ++withWarnings;
    }
    m_warningsFilter->setText(withWarnings ? tr("Warnings (%1)").arg(withWarnings) : tr("Warnings"));
    int found = 0;
    for (int row = 0; m_filter.searching() && row < m_model->rowCount(); ++row)
        found += m_filter.holds(m_model, row) ? 1 : 0;
    m_found->setVisible(m_filter.searching());
    m_found->setText(found == 0 ? tr("Nothing found") : found == 1 ? tr("1 segment") : tr("%1 segments").arg(QLocale(QLocale::English).toString(found)));
}

void EditorPage::goToSegment(const QString &id)
{
    const int row = m_model->rowOf(id);
    if (row < 0)
        return;
    if (m_activeId != id)
        activate(row);
    const QModelIndex i = m_model->index(row, SegmentModel::Target);
    {
        const QSignalBlocker block(m_grid->selectionModel());
        m_grid->setCurrentIndex(i);
    }
    m_grid->scrollTo(i);
}

void EditorPage::saveActive()
{
    m_saveTimer.stop();
    if (!dirty())
        return;
    const QJsonArray target = m_target->content();
    const bool empty = plainText(target).trimmed().isEmpty() && SegmentContent::tags(target).isEmpty();
    save(m_activeId, target, empty ? QStringLiteral("new") : QStringLiteral("draft"), {}, m_origin);
}

QWidget *EditorPage::makeCellWidget(QWidget *parent, const QModelIndex &index)
{
    const QJsonObject s = m_model->segment(index.row());
    QFont font = parent->font();
    font.setPixelSize(15);
    if (index.column() == SegmentModel::Source) {
        auto *src = new QTextBrowser(parent);
        src->setObjectName(QStringLiteral("activeSource"));
        src->setAccessibleName(tr("Source of segment %1").arg(index.row() + 1));
        src->setFont(font);
        src->setFrameShape(QFrame::NoFrame);
        src->setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        src->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        src->document()->setDocumentMargin(0);
        src->document()->setDefaultStyleSheet(SegmentContent::css());
        QTextOption direction = src->document()->defaultTextOption();
        direction.setTextDirection(Languages::rtl(m_sourceLang) ? Qt::RightToLeft : Qt::LeftToRight);
        src->document()->setDefaultTextOption(direction);
        src->viewport()->setAutoFillBackground(false);
        src->setOpenLinks(false);
        src->setTextInteractionFlags(Qt::TextSelectableByMouse | Qt::TextSelectableByKeyboard);
        m_source = src;
        return src;
    }
    auto *box = new QWidget(parent);
    auto *l = new QVBoxLayout(box);
    l->setContentsMargins(0, 0, 0, 0);
    l->setSpacing(0);
    const int n = index.row() + 1;
    if (s.value(QLatin1String("locked")).toBool()) {
        // .seg-locked: the translation, and why it can't be changed.
        auto *view = new QLabel(box);
        view->setFocusPolicy(Qt::StrongFocus);
        view->setWordWrap(true);
        view->setFont(font);
        view->setContentsMargins(S3, S3, S3, S3);
        const QJsonArray target = s.value(QLatin1String("target")).toArray();
        view->setText(QStringLiteral("<style>%1</style>%2<br><span style=\"color:%3; font-size:12px\">%4</span>")
                          .arg(SegmentContent::css(),
                               target.isEmpty() ? QStringLiteral("<span class=\"missing\">%1</span>").arg(tr("Not translated")) : SegmentContent::toHtml(target),
                               Theme::color("text3").name(), tr("Locked in the file")));
        view->setAccessibleName(
            tr("Segment %1: %2. Locked in the file").arg(n).arg(plainText(target).isEmpty() ? tr("not translated") : plainText(target)));
        l->addWidget(view);
        l->addStretch();
        return box;
    }
    auto *editor = new TargetEditor(box);
    editor->setObjectName(QStringLiteral("targetEditor"));
    editor->setFont(font);
    editor->document()->setDocumentMargin(0);
    // Typed from the right in a right-to-left language, whatever the
    // direction of the window.
    QTextOption direction = editor->document()->defaultTextOption();
    direction.setTextDirection(Languages::rtl(m_lang) ? Qt::RightToLeft : Qt::LeftToRight);
    editor->document()->setDefaultTextOption(direction);
    editor->setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    editor->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    editor->setPlaceholderText(tr("Type the translation"));
    editor->setAccessibleName(tr("Translation of segment %1, %2").arg(n).arg(Languages::name(m_lang)));
    editor->setAccessibleDescription(tr("Control Enter confirms and moves on. Control and a number inserts a numbered result. "
                                        "Control E adds a term. Control K searches the memory. F9 inserts the next tag."));
    editor->installEventFilter(this);
    l->addWidget(editor, 1);
    if (const int max = s.value(QLatin1String("maxLength")).toInt(); max > 0) {
        auto *counter = new QLabel(box);
        Theme::role(counter, "progress");
        counter->setContentsMargins(S3, 0, S3, S1);
        l->addWidget(counter);
        m_counter = counter;
    }
    // A part of a split subtitle has the time of the whole, not its own.
    if (s.value(QLatin1String("timing")).isObject() && !s.value(QLatin1String("split")).isObject()) {
        auto *counter = new QLabel(box);
        counter->setObjectName(QStringLiteral("subtitleCounter"));
        Theme::role(counter, "progress");
        counter->setTextFormat(Qt::RichText);
        counter->setWordWrap(true);
        counter->setContentsMargins(S3, 0, S3, S1);
        l->addWidget(counter);
        m_subtitleCounter = counter;
    }
    m_target = editor;
    connect(editor, &TargetEditor::edited, this, [this] {
        if (m_activeId.isEmpty() || !m_target)
            return;
        m_model->setPending(m_activeId, m_target->content());
        if (m_counter) {
            const int max = m_model->segment(activeRow()).value(QLatin1String("maxLength")).toInt();
            const int len = int(plainText(m_target->content()).size());
            m_counter->setText(len > max ? tr("%1 of %2 characters, %3 too many").arg(len).arg(max).arg(len - max)
                                         : tr("%1 of %2 characters").arg(len).arg(max));
            m_counter->setStyleSheet(len > max ? QStringLiteral("color: %1").arg(Theme::color("danger").name()) : QString());
        }
        updateSubtitleCounter();
        resizeActiveRow();
        m_checkTimer.start();
        if (dirty())
            m_saveTimer.start();
    });
    connect(editor, &TargetEditor::addWord, this, &EditorPage::addWord);
    if (m_subtitleCounter)
        QTimer::singleShot(0, this, &EditorPage::updateSubtitleCounter);
    return box;
}

// The lines of a subtitle's translation: its text cut at the tags the
// source has as breaks, and at line breaks typed in.
static QStringList subtitleLines(const QJsonArray &source, const QJsonArray &target)
{
    QSet<QString> breaks;
    for (const QJsonValue &v : source) {
        const QJsonObject r = v.toObject();
        if (r.contains(QLatin1String("tag")) && r.value(QLatin1String("tag")).toObject().value(QLatin1String("type")).toString() == QLatin1String("break"))
            breaks.insert(r.value(QLatin1String("tag")).toObject().value(QLatin1String("id")).toString());
    }
    QStringList lines{QString()};
    for (const QJsonValue &v : target) {
        const QJsonObject r = v.toObject();
        if (r.contains(QLatin1String("tag"))) {
            const QJsonObject t = r.value(QLatin1String("tag")).toObject();
            if (t.value(QLatin1String("type")).toString() == QLatin1String("break")
                || (t.value(QLatin1String("kind")).toString() == QLatin1String("standalone") && breaks.contains(t.value(QLatin1String("id")).toString())))
                lines << QString();
            continue;
        }
        const QStringList parts = r.value(QLatin1String("text")).toString().split(QLatin1Char('\n'));
        lines.last() += parts.first();
        for (int i = 1; i < parts.size(); ++i)
            lines << parts.at(i);
    }
    QStringList out;
    for (const QString &l : std::as_const(lines)) {
        if (!l.trimmed().isEmpty())
            out << l.trimmed();
    }
    return out;
}

void EditorPage::updateSubtitleCounter()
{
    if (!m_subtitleCounter || !m_target || activeRow() < 0)
        return;
    const QJsonObject seg = m_model->segment(activeRow());
    const QJsonObject timing = seg.value(QLatin1String("timing")).toObject();
    const QJsonObject lim = settings().value(QLatin1String("qa")).toObject().value(QLatin1String("subtitles")).toObject();
    int maxLine = lim.value(QLatin1String("lineLength")).toInt(42);
    if (maxLine <= 0)
        maxLine = 42;
    if (const int own = timing.value(QLatin1String("lineLimit")).toInt(); own > 0 && own < maxLine)
        maxLine = own;
    int maxLines = lim.value(QLatin1String("lines")).toInt(2);
    if (maxLines <= 0)
        maxLines = 2;
    double maxCps = lim.value(QLatin1String("cps")).toDouble(17);
    if (maxCps <= 0)
        maxCps = 17;
    const QStringList lines = subtitleLines(seg.value(QLatin1String("source")).toArray(), m_target->content());
    int longest = 0, chars = 0;
    for (const QString &l : lines) {
        const int n = int(l.toUcs4().size());
        longest = qMax(longest, n);
        chars += n;
    }
    const int count = qMax(1, int(lines.size()));
    const double seconds = (timing.value(QLatin1String("end")).toDouble() - timing.value(QLatin1String("start")).toDouble()) / 1000;
    const QString danger = Theme::color("danger").name();
    const auto part = [&danger](const QString &text, bool over) {
        return over ? QStringLiteral("<span style=\"color:%1; font-weight:600\">%2</span>").arg(danger, text.toHtmlEscaped()) : text.toHtmlEscaped();
    };
    QStringList parts{part(tr("Longest line %1 of %2").arg(longest).arg(maxLine), longest > maxLine),
                      part((count == 1 ? tr("1 line of %1") : tr("%1 lines of %2").arg(count)).arg(maxLines), count > maxLines)};
    if (seconds > 0) {
        const double cps = chars / seconds;
        parts << part(tr("%1 of %2 characters per second in %3 s").arg(QLocale().toString(cps, 'f', 1), QLocale().toString(maxCps), QLocale().toString(seconds, 'f', 2)), cps > maxCps);
    }
    m_subtitleCounter->setText(parts.join(QStringLiteral(" · ")));
}

int EditorPage::activeHeight(int column, int width) const
{
    if (column == SegmentModel::Target && m_target) {
        // Measured on a copy: the editor's own document follows its
        // current width, which lags behind the column's.
        std::unique_ptr<QTextDocument> doc(m_target->document()->clone());
        SegmentContent::enableTags(doc.get());
        doc->setDefaultFont(m_target->font());
        doc->setTextWidth(qMax(10, width - 2 * S3));
        return int(doc->size().height()) + 2 * S3 + (m_counter ? 20 : 0) + (m_subtitleCounter ? qMax(20, m_subtitleCounter->heightForWidth(width)) : 0);
    }
    return 0;
}

void EditorPage::resizeActiveRow()
{
    const int row = activeRow();
    if (row < 0)
        return;
    // A row is as tall as its widgets want to be; they want only what
    // their text needs, as the web editor's grows with its text.
    QStyleOptionViewItem opt;
    opt.widget = m_grid;
    opt.font = m_grid->font();
    const int sourceHeight = m_delegate->sizeHint(opt, m_model->index(row, SegmentModel::Source)).height();
    if (m_target) {
        const int height = activeHeight(SegmentModel::Target, m_grid->horizontalHeader()->sectionSize(SegmentModel::Target));
        m_target->parentWidget()->setMaximumHeight(qMax(height, sourceHeight));
    } else if (auto *w = m_grid->indexWidget(m_model->index(row, SegmentModel::Target))) {
        w->setMaximumHeight(sourceHeight);
    }
    if (m_source)
        m_source->setMaximumHeight(qMax(20, sourceHeight - 24));
    m_grid->resizeRowToContents(row);
}

void EditorPage::activate(int row)
{
    saveActive();
    const QJsonObject s = m_model->segment(row);
    const QString id = s.value(QLatin1String("id")).toString();
    if (const int old = activeRow(); old >= 0 && old != row) {
        m_model->setMarks(m_activeId, {});
        if (!m_conflicts.contains(m_activeId))
            m_model->clearPending(m_activeId);
        m_target = nullptr;
        m_source = nullptr;
        m_counter = nullptr;
        m_subtitleCounter = nullptr;
        m_grid->closePersistentEditor(m_model->index(old, SegmentModel::Source));
        m_grid->closePersistentEditor(m_model->index(old, SegmentModel::Target));
        m_grid->resizeRowToContents(old);
    }
    m_activeId = id;
    m_model->setActive(id);
    m_loadedTarget = s.value(QLatin1String("target")).toArray();
    m_origin = s.value(QLatin1String("origin")).toObject();
    m_ignored = s.value(QLatin1String("ignored")).toArray();
    m_results = {};
    m_terms = {};
    m_issues = {};
    m_selected = 0;
    // The best match goes into an empty, untouched segment once its
    // matches are known (Trados' default).
    m_insertBest = Options::autoInsert() && s.value(QLatin1String("status")).toString() == QLatin1String("new") && !s.value(QLatin1String("locked")).toBool() &&
                   plainText(m_loadedTarget).trimmed().isEmpty();
    m_captureForm->hide();
    m_captureIntro->show();
    m_grid->openPersistentEditor(m_model->index(row, SegmentModel::Source));
    m_grid->openPersistentEditor(m_model->index(row, SegmentModel::Target));
    if (m_source)
        m_source->setHtml(SegmentContent::toHtml(s.value(QLatin1String("source")).toArray()));
    if (m_target) {
        m_target->setContent(m_loadedTarget);
        emit m_target->edited();
    }
    if (m_conflicts.contains(id))
        showConflict(id, m_conflicts.value(id));
    else
        m_conflict->hide();
    applyFilter();
    resizeActiveRow();
    if (m_target)
        m_target->setFocus();
    showResults();
    showIssues();
    m_commentText->clear();
    showComments();
    showReviewIssues();
    m_suggestion = {};
    m_serviceProblem.clear();
    showSuggestions();
    updatePreview();
    sendPresence();
    updateCut();
    lookup();
    recognize();
    runChecks();
    m_saveTimer.stop();
}

void EditorPage::lookup()
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const int serial = ++m_lookupSerial;
    const QJsonObject s = m_model->segment(activeRow());
    const QJsonObject body{{QStringLiteral("lang"), m_lang}, {QStringLiteral("source"), s.value(QLatin1String("source"))}, {QStringLiteral("limit"), 9}};
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/lookup"), body), this, [this, serial](const ApiResult &r) {
        if (serial != m_lookupSerial)
            return;
        QJsonArray results;
        if (r.ok()) {
            for (const QJsonValue &v : r.array()) {
                QJsonObject m = v.toObject();
                m.insert(QStringLiteral("kind"), QStringLiteral("match"));
                results.append(m);
            }
        }
        for (const QJsonValue &v : std::as_const(m_results)) {
            if (v.toObject().value(QLatin1String("kind")).toString() == QLatin1String("term"))
                results.append(v);
        }
        m_results = results;
        showResults();
        if (m_insertBest) {
            m_insertBest = false;
            const QJsonObject best = m_results.isEmpty() ? QJsonObject() : m_results.first().toObject();
            if (best.value(QLatin1String("kind")).toString() == QLatin1String("match") && editable() && m_target && plainText(m_target->content()).trimmed().isEmpty()) {
                insertResult(1);
                const int score = best.value(QLatin1String("score")).toInt();
                emit bestInserted(best.value(QLatin1String("context")).toBool() ? tr("Context match inserted") : tr("%1% match inserted").arg(score));
            }
        }
    });
}

void EditorPage::recognize()
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const QString id = m_activeId;
    const QJsonArray source = m_model->segment(activeRow()).value(QLatin1String("source")).toArray();
    const QJsonObject body{{QStringLiteral("lang"), m_lang}, {QStringLiteral("text"), plainText(source)}};
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/recognize"), body), this, [this, id, source](const ApiResult &r) {
        if (id != m_activeId || !r.ok())
            return;
        m_terms = r.array();
        QJsonArray results;
        for (const QJsonValue &v : std::as_const(m_results)) {
            if (v.toObject().value(QLatin1String("kind")).toString() == QLatin1String("match"))
                results.append(v);
        }
        QVector<SegmentContent::Mark> marks;
        for (const QJsonValue &v : std::as_const(m_terms)) {
            QJsonObject t = v.toObject();
            marks << SegmentContent::Mark{t.value(QLatin1String("start16")).toInt(), t.value(QLatin1String("end16")).toInt(), QStringLiteral("term")};
            t.insert(QStringLiteral("kind"), QStringLiteral("term"));
            results.append(t);
        }
        m_results = results;
        m_model->setMarks(id, marks);
        if (m_source)
            m_source->setHtml(SegmentContent::toHtml(source, marks));
        resizeActiveRow();
        showResults();
    });
}

void EditorPage::showResults()
{
    // What may complete a word being typed: the terms found for the segment,
    // then the longer words of its memory matches.
    {
        QStringList words;
        if (Options::autoComplete()) {
            static const QRegularExpression word(QStringLiteral("[\\p{L}\\p{N}][\\p{L}\\p{N}\\p{M}'\u2019-]{4,}"),
                                                 QRegularExpression::UseUnicodePropertiesOption);
            QStringList fromMatches;
            for (const QJsonValue &v : std::as_const(m_results)) {
                const QJsonObject r = v.toObject();
                if (r.contains(QLatin1String("unit"))) {
                    auto it = word.globalMatch(plainText(r.value(QLatin1String("unit")).toObject().value(QLatin1String("target")).toArray()));
                    while (it.hasNext())
                        fromMatches << it.next().captured(0);
                    continue;
                }
                for (const QJsonValue &t : r.value(QLatin1String("targets")).toArray())
                    if (t.toObject().value(QLatin1String("status")).toString() != QLatin1String("forbidden"))
                        words << t.toObject().value(QLatin1String("text")).toString();
            }
            words << fromMatches;
        }
        m_target->setCompletions(words);
    }
    clear(m_resultsList);
    QWidget *panel = m_panel->widget();
    m_resultsCount->setVisible(!m_results.isEmpty());
    m_resultsCount->setText(QString::number(m_results.size()));
    if (m_results.isEmpty()) {
        m_resultsList->addWidget(label(tr("No memory matches or terms for this segment."), "muted", panel));
        emit resultsShown();
        return;
    }
    const QString source = m_activeId.isEmpty() ? QString() : plainText(m_model->segment(activeRow()).value(QLatin1String("source")).toArray());
    m_selected = qMin(m_selected, int(m_results.size()) - 1);
    for (int i = 0; i < m_results.size(); ++i) {
        const QJsonObject r = m_results.at(i).toObject();
        auto *card = new ResultCard(panel);
        auto *g = new QHBoxLayout(card);
        g->setContentsMargins(S3, S2, S3, S2);
        g->setSpacing(S2);
        auto *key = new QLabel(QString::number(i + 1), card);
        Theme::role(key, "progress");
        key->setMinimumWidth(18);
        g->addWidget(key, 0, Qt::AlignTop);
        auto *bodyBox = new QVBoxLayout;
        bodyBox->setSpacing(2);
        QString spoken;
        if (r.value(QLatin1String("kind")).toString() == QLatin1String("match")) {
            const QJsonObject unit = r.value(QLatin1String("unit")).toObject();
            const int score = r.value(QLatin1String("score")).toInt();
            const bool context = r.value(QLatin1String("context")).toBool();
            QWidget *b = Bands::badge(Bands::scoreLabel(score, context), Bands::ofScore(score, context), card);
            // At the least as wide as 44 px: a wider font widens it.
            b->setMinimumWidth(44);
            b->setToolTip(Bands::scoreDescription(score, context));
            g->addWidget(b, 0, Qt::AlignTop);
            const QString matchSource = plainText(unit.value(QLatin1String("source")).toArray());
            bodyBox->addWidget(label(QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), SegmentContent::diffHtml(source, matchSource)),
                                     "secondary", card));
            auto *tgt = label(QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), SegmentContent::toHtml(unit.value(QLatin1String("target")).toArray())),
                              nullptr, card);
            QFont f = tgt->font();
            f.setWeight(QFont::Medium);
            tgt->setFont(f);
            bodyBox->addWidget(tgt);
            QStringList meta{r.value(QLatin1String("memory")).toString()};
            if (const QString by = unit.value(QLatin1String("changedBy")).toString(); !by.isEmpty())
                meta << by;
            if (const QString at = unit.value(QLatin1String("changedAt")).toString(); !at.isEmpty())
                meta << QLocale().toString(QDateTime::fromString(at, Qt::ISODate).toLocalTime().date(), QLocale::ShortFormat);
            bodyBox->addWidget(label(meta.join(QLatin1String(", ")), "progress", card));
            spoken = tr("%1: %2, %3. %4")
                         .arg(i + 1)
                         .arg(Bands::scoreDescription(score, context), plainText(unit.value(QLatin1String("target")).toArray()),
                              meta.join(QLatin1String(", ")));
        } else {
            auto *b = new QLabel(card);
            b->setFixedSize(44, 20);
            b->setAlignment(Qt::AlignCenter);
            b->setPixmap(Icons::get(QStringLiteral("tag"), Theme::color("accentText")).pixmap(13, 13));
            b->setStyleSheet(QStringLiteral("QLabel { background: %1; border-radius: 4px; }").arg(Theme::color("accentWeak").name()));
            b->setToolTip(tr("Term"));
            g->addWidget(b, 0, Qt::AlignTop);
            const QString term = r.value(QLatin1String("term")).toObject().value(QLatin1String("text")).toString();
            QStringList targets, plainTargets;
            for (const QJsonValue &t : r.value(QLatin1String("targets")).toArray()) {
                const QJsonObject tt = t.toObject();
                const QString status = tt.value(QLatin1String("status")).toString();
                const bool forbidden = status == QLatin1String("forbidden");
                QString item = tt.value(QLatin1String("text")).toString().toHtmlEscaped();
                if (forbidden)
                    item = QStringLiteral("<span style=\"color:%1\">%2</span>").arg(Theme::color("danger").name(), item);
                if (!status.isEmpty() && status != QLatin1String("approved"))
                    item += QStringLiteral(" <span style=\"background:%1; color:%2; font-size:11px; font-weight:600\">&nbsp;%3&nbsp;</span>")
                                .arg(Theme::color(forbidden ? "dangerBg" : "warnBg").name(), Theme::color(forbidden ? "danger" : "warn").name(),
                                     forbidden ? tr("Forbidden") : tr("Proposed"));
                targets << item;
                plainTargets << tt.value(QLatin1String("text")).toString();
            }
            bodyBox->addWidget(label(term.toHtmlEscaped(), "secondary", card));
            auto *tgt = label(targets.join(QLatin1String(", ")), nullptr, card);
            QFont f = tgt->font();
            f.setWeight(QFont::Medium);
            tgt->setFont(f);
            bodyBox->addWidget(tgt);
            bodyBox->addWidget(label(r.value(QLatin1String("source")).toString(), "progress", card));
            spoken = tr("%1: term %2: %3").arg(i + 1).arg(term, plainTargets.join(QLatin1String(", ")));
        }
        g->addLayout(bodyBox, 1);
        card->setAccessibleName(spoken);
        card->setToolTip(i < 9 ? QStringLiteral("Ctrl+%1").arg(i + 1) : QString());
        card->setSelected(i == m_selected);
        const int n = i + 1;
        card->activated = [this, n] { insertResult(n); };
        m_resultsList->addWidget(card);
    }
    emit resultsShown();
}

void EditorPage::insertResult(int n)
{
    if (!editable() || n < 1 || n > m_results.size())
        return;
    const QJsonObject r = m_results.at(n - 1).toObject();
    if (r.value(QLatin1String("kind")).toString() == QLatin1String("match")) {
        const QJsonArray target = r.value(QLatin1String("unit")).toObject().value(QLatin1String("target")).toArray();
        m_target->setContent(target);
        m_origin = QJsonObject{{QStringLiteral("kind"), QStringLiteral("tm")},
                               {QStringLiteral("memory"), r.value(QLatin1String("memory"))},
                               {QStringLiteral("score"), r.value(QLatin1String("score"))},
                               {QStringLiteral("context"), r.value(QLatin1String("context")).toBool()},
                               {QStringLiteral("inserted"), QString::fromUtf8(QJsonDocument(target).toJson(QJsonDocument::Compact))}};
        emit m_target->edited();
    } else {
        for (const QJsonValue &t : r.value(QLatin1String("targets")).toArray()) {
            const QJsonObject tt = t.toObject();
            if (tt.value(QLatin1String("status")).toString() == QLatin1String("forbidden"))
                continue;
            m_target->insertPlain(tt.value(QLatin1String("text")).toString());
            break;
        }
    }
    m_target->setFocus();
}

void EditorPage::copySource()
{
    if (!editable())
        return;
    m_target->setContent(m_model->segment(activeRow()).value(QLatin1String("source")).toArray());
    m_origin = QJsonObject{{QStringLiteral("kind"), QStringLiteral("copy")}};
    emit m_target->edited();
    m_target->setFocus();
}

void EditorPage::insertNextTag()
{
    if (!editable())
        return;
    QVector<QJsonObject> have = SegmentContent::tags(m_target->content());
    for (const QJsonObject &tag : SegmentContent::tags(m_model->segment(activeRow()).value(QLatin1String("source")).toArray())) {
        bool found = false;
        for (int i = 0; i < have.size(); ++i) {
            if (have[i].value(QLatin1String("id")) == tag.value(QLatin1String("id"))
                && have[i].value(QLatin1String("kind")) == tag.value(QLatin1String("kind"))) {
                have.removeAt(i);
                found = true;
                break;
            }
        }
        if (!found) {
            m_target->insertTagAtCursor(tag);
            m_target->setFocus();
            return;
        }
    }
    notify(tr("All tags are already in the target."));
}

void EditorPage::move(int step)
{
    const int row = activeRow();
    int next = row + step;
    while (next >= 0 && next < m_model->rowCount() && m_grid->isRowHidden(next))
        next += step;
    if (row < 0 || next < 0 || next >= m_model->rowCount())
        return;
    goToSegment(m_model->segment(next).value(QLatin1String("id")).toString());
}

void EditorPage::runChecks()
{
    if (!m_conn || m_activeId.isEmpty() || !m_target)
        return;
    const int serial = ++m_checkSerial;
    const QString id = m_activeId;
    QJsonObject targets;
    targets.insert(id, m_target->content());
    const QJsonObject body{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang},
                           {QStringLiteral("segments"), QJsonArray{id}}, {QStringLiteral("targets"), targets}};
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/qa"), body), this, [this, serial, id](const ApiResult &r) {
        if (serial != m_checkSerial || id != m_activeId || !r.ok())
            return;
        m_issues = r.array();
        showIssues();
        emit checked();
    });
    checkGrammar(id, plainText(m_target->content()));
}

// Asks the project's grammar service about a segment's translation, and
// calls then when it has answered or failed. A project without a service
// asks nobody.
void EditorPage::checkGrammar(const QString &id, const QString &text, std::function<void()> then)
{
    const QString service = grammarService();
    const int serial = ++m_grammarSerial;
    const auto done = [this, id, text, serial, then](const QJsonArray &found, const QString &error) {
        if (serial == m_grammarSerial) {
            m_grammar = found;
            m_grammarId = id;
            m_grammarText = text;
            m_grammarError = error;
            if (id == m_activeId)
                showIssues();
            emit grammarChecked();
        }
        if (then)
            then();
    };
    if (!m_conn || service.isEmpty() || text.trimmed().isEmpty()) {
        done({}, {});
        return;
    }
    const QJsonObject body{{QStringLiteral("lang"), m_lang}, {QStringLiteral("text"), text}};
    onResult(m_conn->api()->post(QStringLiteral("/api/providers/%1/check").arg(ApiClient::seg(service)), body), this, [done](const ApiResult &r) {
        if (!r.ok()) {
            done({}, r.error.message);
            return;
        }
        QJsonArray found;
        for (const QJsonValue &v : r.object().value(QLatin1String("matches")).toArray()) {
            const QJsonObject m = v.toObject();
            const QString what = m.value(QLatin1String("found")).toString();
            found.append(QJsonObject{{QStringLiteral("kind"), QStringLiteral("grammar")},
                                     {QStringLiteral("message"), tr("%1 (“%2”)").arg(m.value(QLatin1String("message")).toString(), what)},
                                     {QStringLiteral("found"), what},
                                     {QStringLiteral("replacements"), m.value(QLatin1String("replacements"))},
                                     {QStringLiteral("offset"), m.value(QLatin1String("offset"))},
                                     {QStringLiteral("length"), m.value(QLatin1String("length"))}});
        }
        done(found, {});
    });
}

QJsonArray EditorPage::grammarIssues() const
{
    if (grammarService().isEmpty() || !m_target || m_grammarId != m_activeId || m_grammarText != plainText(m_target->content()))
        return {};
    return m_grammar;
}

// The run of text a stretch of the plain text lies in, and where in it;
// -1 where the stretch has a tag in it.
static int runOfStretch(const QJsonArray &content, int offset, int length, int *within)
{
    int at = 0;
    for (int i = 0; i < content.size(); ++i) {
        const QJsonObject run = content.at(i).toObject();
        if (run.contains(QLatin1String("tag")))
            continue;
        const int size = int(run.value(QLatin1String("text")).toString().size());
        if (offset >= at && offset + length <= at + size) {
            *within = offset - at;
            return i;
        }
        at += size;
    }
    return -1;
}

bool EditorPage::replaceFound(int offset, int length, const QString &value)
{
    if (!m_target || !editable())
        return false;
    QJsonArray content = m_target->content();
    int within = 0;
    const int i = runOfStretch(content, offset, length, &within);
    if (i < 0)
        return false;
    QString text = content.at(i).toObject().value(QLatin1String("text")).toString();
    text.replace(within, length, value);
    if (text.isEmpty())
        content.removeAt(i);
    else
        content.replace(i, QJsonObject{{QStringLiteral("text"), text}});
    m_target->setContent(content);
    emit m_target->edited();
    m_target->setFocus();
    return true;
}

void EditorPage::showIssues()
{
    clear(m_issuesList);
    QWidget *panel = m_panel->widget();
    QSet<QString> ignored;
    for (const QJsonValue &v : std::as_const(m_ignored))
        ignored << v.toString();
    QVector<TargetEditor::Misspelling> words;
    int shown = 0;
    if (!grammarService().isEmpty() && !m_grammarError.isEmpty() && m_grammarId == m_activeId) {
        auto *failed = label(tr("Grammar is not checked: %1").arg(m_grammarError), "muted", panel);
        failed->setObjectName(QStringLiteral("grammarError"));
        m_issuesList->addWidget(failed);
    }
    QJsonArray all = m_issues;
    for (const QJsonValue &v : grammarIssues())
        all.append(v);
    for (const QJsonValue &v : std::as_const(all)) {
        const QJsonObject issue = v.toObject();
        const QString text = issue.value(QLatin1String("message")).toString();
        if (ignored.contains(text))
            continue;
        ++shown;
        const QString kind = issue.value(QLatin1String("kind")).toString();
        const QString found = issue.value(QLatin1String("found")).toString();
        if (kind == QLatin1String("spelling")) {
            QStringList suggestions;
            for (const QJsonValue &s : issue.value(QLatin1String("suggestions")).toArray())
                suggestions << s.toString();
            words << TargetEditor::Misspelling{found, suggestions};
        }
        // .issue: the warning sign, the text, and what can be done.
        auto *box = new QFrame(panel);
        box->setObjectName(QStringLiteral("issue"));
        const bool forbidden = kind == QLatin1String("term-forbidden");
        box->setProperty("forbidden", forbidden);
        auto *h = new QHBoxLayout(box);
        h->setContentsMargins(S3, S2, S3, S2);
        h->setSpacing(S2);
        auto *icon = new QLabel(box);
        icon->setPixmap(Icons::get(QStringLiteral("alert-triangle"), Theme::color(forbidden ? "danger" : "warn")).pixmap(14, 14));
        h->addWidget(icon, 0, Qt::AlignTop);
        // The text, and under it what could stand there instead.
        auto *words = new QVBoxLayout;
        words->setContentsMargins(0, 0, 0, 0);
        words->setSpacing(S1);
        words->addWidget(label(text, nullptr, box));
        h->addLayout(words, 1);
        if (kind == QLatin1String("spelling") && !found.isEmpty()) {
            auto *add = new QPushButton(tr("Add to the project's words"), box);
            add->setProperty("tiny", true);
            add->setAccessibleName(tr("Add %1 to the project's words").arg(found));
            connect(add, &QPushButton::clicked, this, [this, found] { addWord(found); });
            h->addWidget(add, 0, Qt::AlignTop);
        }
        if (kind == QLatin1String("grammar") && m_target) {
            const int offset = issue.value(QLatin1String("offset")).toInt();
            const int length = issue.value(QLatin1String("length")).toInt();
            int within = 0;
            const QJsonArray values = issue.value(QLatin1String("replacements")).toArray();
            for (int i = 0; i < values.size() && i < 3 && runOfStretch(m_target->content(), offset, length, &within) >= 0; ++i) {
                const QString value = values.at(i).toString();
                auto *put = new QPushButton(tr("Replace with “%1”").arg(value), box);
                put->setObjectName(QStringLiteral("replaceFound"));
                put->setProperty("tiny", true);
                put->setAccessibleName(tr("Replace %1 with %2").arg(found, value));
                connect(put, &QPushButton::clicked, this, [this, offset, length, value] { replaceFound(offset, length, value); });
                words->addWidget(put, 0, Qt::AlignLeft);
            }
        }
        auto *ignore = new QPushButton(tr("Ignore"), box);
        ignore->setProperty("tiny", true);
        ignore->setAccessibleName(tr("Ignore: %1").arg(text));
        connect(ignore, &QPushButton::clicked, this, [this, text] { ignoreIssue(text); });
        h->addWidget(ignore, 0, Qt::AlignTop);
        m_issuesList->addWidget(box);
    }
    if (m_target)
        m_target->setMisspellings(words);
    m_checksCount->setVisible(shown > 0);
    m_checksCount->setText(QString::number(shown));
    if (shown == 0) {
        const bool typed = m_target && !plainText(m_target->content()).trimmed().isEmpty();
        m_issuesList->addWidget(label(typed ? tr("No problems found.") : tr("Checks run as you type and again when you confirm."), "muted", panel));
    }
    if (!m_ignored.isEmpty())
        m_issuesList->addWidget(
            label(tr("%1 ignored for this segment.").arg(plural(int(m_ignored.size()), tr("warning"), tr("warnings"))), "secondary", panel));
}

void EditorPage::ignoreIssue(const QString &text)
{
    m_ignored.append(text);
    showIssues();
    // Kept with the segment, so the warning stays away.
    if (const int row = activeRow(); row >= 0) {
        const QJsonObject s = m_model->segment(row);
        save(m_activeId, m_target ? m_target->content() : s.value(QLatin1String("target")).toArray(), s.value(QLatin1String("status")).toString(),
             s.value(QLatin1String("warnings")).toArray(), m_origin);
    }
}

void EditorPage::save(const QString &id, const QJsonArray &target, const QString &status, const QJsonArray &warnings,
                      const QJsonObject &origin, std::function<void(bool)> done)
{
    if (!m_conn)
        return;
    const int row = m_model->rowOf(id);
    if (row < 0)
        return;
    const QJsonObject seg = m_model->segment(row);
    QJsonObject edit{{QStringLiteral("target"), target}, {QStringLiteral("status"), status}};
    if (!warnings.isEmpty())
        edit.insert(QStringLiteral("warnings"), warnings);
    const QJsonArray ignored = id == m_activeId ? m_ignored : seg.value(QLatin1String("ignored")).toArray();
    if (!ignored.isEmpty())
        edit.insert(QStringLiteral("ignored"), ignored);
    if (!origin.isEmpty())
        edit.insert(QStringLiteral("origin"), origin);
    if (id == m_activeId)
        m_loadedTarget = target;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), id}};
    onResult(m_conn->api()->put(projectPath() + QStringLiteral("/segments"), edit, seg.value(QLatin1String("version")).toInt(), q), this,
             [this, id, done](const ApiResult &r) {
                 if (!r.ok()) {
                     if (r.status == 409 || r.status == 412) {
                         const QUrlQuery fq{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
                         onResult(m_conn->api()->get(projectPath() + QStringLiteral("/segments"), fq), this, [this, id](const ApiResult &all) {
                             if (!all.ok())
                                 return;
                             for (const QJsonValue &v : all.array()) {
                                 if (v.toObject().value(QLatin1String("id")).toString() == id)
                                     showConflict(id, v.toObject());
                             }
                         });
                     } else {
                         notify(tr("The segment was not saved: %1").arg(r.error.message));
                     }
                     if (done)
                         done(false);
                     return;
                 }
                 m_model->update(r.object());
                 const bool same = id != m_activeId || !m_target
                     || SegmentContent::equal(m_target->content(), r.object().value(QLatin1String("target")).toArray());
                 if (same)
                     m_model->clearPending(id);
                 if (const int row = m_model->rowOf(id); row >= 0)
                     m_grid->resizeRowToContents(row);
                 updateProgress();
                 updatePreview();
                 applyFilter();
                 emit saved(id);
                 if (done)
                     done(true);
             });
}

void EditorPage::showConflict(const QString &id, const QJsonObject &theirs)
{
    m_conflicts.insert(id, theirs);
    m_conflictId = id;
    const QString by = theirs.value(QLatin1String("changedBy")).toString();
    const QString text = tr("%1 saved segment %2 while you worked on it. Their translation: “%3”.")
                             .arg(by.isEmpty() ? tr("Someone") : by)
                             .arg(m_model->rowOf(id) + 1)
                             .arg(plainText(theirs.value(QLatin1String("target")).toArray()));
    m_conflictText->setText(text);
    m_lastMessage = text;
    if (id != m_activeId)
        goToSegment(id);
    m_conflict->show();
    m_panel->ensureWidgetVisible(m_conflict);
}

void EditorPage::confirm()
{
    if (m_activeId.isEmpty() || !m_conn)
        return;
    const int row = activeRow();
    if (m_model->segment(row).value(QLatin1String("locked")).toBool()) {
        notify(tr("This segment is locked in the file and cannot be changed."));
        return;
    }
    if (!m_target)
        return;
    const QString id = m_activeId;
    const QJsonArray target = m_target->content();
    if (plainText(target).trimmed().isEmpty()) {
        notify(tr("The target is empty. Type a translation or insert a match first."));
        return;
    }
    m_saveTimer.stop();
    m_checkTimer.stop();
    ++m_checkSerial;
    QJsonObject targets;
    targets.insert(id, target);
    const QJsonObject body{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang},
                           {QStringLiteral("segments"), QJsonArray{id}}, {QStringLiteral("targets"), targets}};
    const QJsonObject origin = m_origin;
    // The grammar service reads the translation as it is confirmed, unless
    // it has read just that; what it finds is kept with the segment.
    const QString said = plainText(target);
    if (!grammarService().isEmpty() && !m_confirmAsked && !(m_grammarId == id && m_grammarText == said)) {
        if (m_confirmWaits)
            return;
        m_confirmWaits = true;
        checkGrammar(id, said, [this, id, said] {
            m_confirmWaits = false;
            // An answer or a failure: either way it has been asked, and
            // the segment is confirmed if it still says the same.
            m_confirmAsked = true;
            if (id == m_activeId && m_target && plainText(m_target->content()) == said)
                confirm();
            m_confirmAsked = false;
        });
        return;
    }
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/qa"), body), this, [this, id, target, origin, said](const ApiResult &r) {
        QSet<QString> ignored;
        for (const QJsonValue &v : std::as_const(m_ignored))
            ignored << v.toString();
        QJsonArray warnings;
        if (r.ok()) {
            if (id == m_activeId) {
                m_issues = r.array();
                showIssues();
            }
            QJsonArray found = r.array();
            if (m_grammarId == id && m_grammarText == said) {
                for (const QJsonValue &v : std::as_const(m_grammar))
                    found.append(v);
            }
            for (const QJsonValue &v : std::as_const(found)) {
                const QJsonObject issue = v.toObject();
                if (!ignored.contains(issue.value(QLatin1String("message")).toString()))
                    warnings.append(QJsonObject{{QStringLiteral("kind"), issue.value(QLatin1String("kind"))},
                                                {QStringLiteral("text"), issue.value(QLatin1String("message"))}});
            }
        }
        // Unconfirmed repetitions of the same source get the translation.
        const int row = m_model->rowOf(id);
        const QString key = sourceKey(m_model->segment(row).value(QLatin1String("source")).toArray());
        QStringList numbers;
        QList<int> repRows;
        for (int i = 0; i < m_model->rowCount(); ++i) {
            const QJsonObject s = m_model->segment(i);
            if (!Options::autoPropagate() || i == row || s.value(QLatin1String("status")).toString() == QLatin1String("confirmed") || s.value(QLatin1String("locked")).toBool())
                continue;
            if (sourceKey(s.value(QLatin1String("source")).toArray()) == key) {
                numbers << QString::number(i + 1);
                repRows << i;
            }
        }
        save(id, target, QStringLiteral("confirmed"), warnings, origin, [this, id, row, warnings, numbers, repRows, target](bool ok) {
            if (!ok)
                return;
            for (int i : repRows)
                save(m_model->segment(i).value(QLatin1String("id")).toString(), target, QStringLiteral("confirmed"), {},
                     QJsonObject{{QStringLiteral("kind"), QStringLiteral("repetition")}, {QStringLiteral("from"), id}});
            if (!numbers.isEmpty())
                notify(tr("Also filled %1 (segment %2)")
                           .arg(plural(int(numbers.size()), tr("repetition"), tr("repetitions")), numbers.join(QLatin1String(", "))));
            else
                m_lastMessage = warnings.isEmpty() ? tr("Confirmed")
                                                   : tr("Confirmed with %1").arg(plural(int(warnings.size()), tr("warning"), tr("warnings")));
            if (id != m_activeId)
                return;
            // The next open segment after it, else the first open one.
            auto open = [this, &repRows, row](int i) {
                const QJsonObject s = m_model->segment(i);
                return i != row && !repRows.contains(i) && s.value(QLatin1String("status")).toString() != QLatin1String("confirmed")
                    && !s.value(QLatin1String("locked")).toBool();
            };
            for (int pass = 0; pass < 2; ++pass) {
                const int from = pass == 0 ? row + 1 : 0;
                const int to = pass == 0 ? m_model->rowCount() : row;
                for (int i = from; i < to; ++i) {
                    if (open(i)) {
                        goToSegment(m_model->segment(i).value(QLatin1String("id")).toString());
                        return;
                    }
                }
            }
            notify(tr("All segments confirmed"));
        });
    });
}

void EditorPage::openCapture()
{
    if (m_activeId.isEmpty())
        return;
    m_lookupSection->hide();
    const QString src = m_source ? m_source->textCursor().selectedText().trimmed() : QString();
    QString tgt = m_target ? m_target->textCursor().selectedText() : QString();
    tgt.remove(QChar::ObjectReplacementCharacter);
    showCapture(src, tgt.trimmed());
}

void EditorPage::showCapture(const QString &source, const QString &target)
{
    if (auto *l = m_captureForm->findChild<QLabel *>(QStringLiteral("captureSourceLabel")))
        l->setText(Languages::name(m_sourceLang));
    if (auto *l = m_captureForm->findChild<QLabel *>(QStringLiteral("captureTargetLabel")))
        l->setText(Languages::name(m_lang));
    m_captureSource->setAccessibleName(Languages::name(m_sourceLang));
    m_captureTarget->setAccessibleName(Languages::name(m_lang));
    m_captureSource->setText(source);
    m_captureTarget->setText(target);
    m_captureNote->clear();
    m_captureAdd->setEnabled(!source.isEmpty() && !target.isEmpty());
    m_captureIntro->hide();
    m_captureForm->show();
    (source.isEmpty() ? m_captureSource : m_captureTarget)->setFocus();
    m_panel->ensureWidgetVisible(m_captureForm);
}

void EditorPage::openLookup(const QString &text)
{
    m_captureForm->hide();
    m_captureIntro->show();
    QString q = text;
    if (q.isEmpty() && m_target)
        q = m_target->textCursor().selectedText().remove(QChar::ObjectReplacementCharacter);
    if (q.isEmpty() && m_source)
        q = m_source->textCursor().selectedText();
    m_lookupSection->show();
    {
        const QSignalBlocker block(m_lookupField);
        m_lookupField->setText(q.trimmed());
    }
    m_lookupField->setFocus();
    m_lookupField->selectAll();
    searchMemory();
}

void EditorPage::searchMemory()
{
    clear(m_lookupList);
    const QString q = m_lookupField->text().trimmed();
    if (!m_conn || q.size() < 2)
        return;
    // Words in either language: the source side, then the target side.
    const int serial = ++m_searchSerial;
    auto hits = std::make_shared<QJsonArray>();
    auto pending = std::make_shared<int>(2);
    auto show = [this, serial, q, hits] {
        if (serial != m_searchSerial)
            return;
        clear(m_lookupList);
        QWidget *panel = m_panel->widget();
        if (hits->isEmpty()) {
            m_lookupList->addWidget(label(tr("Nothing in the memory contains “%1”.").arg(q), "muted", panel));
            return;
        }
        auto mark = [&q](const QString &s) {
            const int at = int(s.indexOf(q, 0, Qt::CaseInsensitive));
            if (at < 0)
                return s.toHtmlEscaped();
            return s.left(at).toHtmlEscaped() + QStringLiteral("<span class=\"mark\">") + s.mid(at, q.size()).toHtmlEscaped()
                + QStringLiteral("</span>") + s.mid(at + q.size()).toHtmlEscaped();
        };
        for (const QJsonValue &v : std::as_const(*hits)) {
            const QJsonObject hit = v.toObject();
            const QJsonObject unit = hit.value(QLatin1String("unit")).toObject();
            const QString source = plainText(unit.value(QLatin1String("source")).toArray());
            const QString target = plainText(unit.value(QLatin1String("target")).toArray());
            auto *item = new QWidget(panel);
            auto *iv = new QVBoxLayout(item);
            iv->setContentsMargins(0, 0, 0, S1);
            iv->setSpacing(2);
            iv->addWidget(label(QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), mark(source)), nullptr, item));
            auto *t = label(QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), mark(target)), nullptr, item);
            QFont f = t->font();
            f.setWeight(QFont::Medium);
            t->setFont(f);
            iv->addWidget(t);
            QStringList meta{hit.value(QLatin1String("memory")).toString()};
            if (const QString by = unit.value(QLatin1String("changedBy")).toString(); !by.isEmpty())
                meta << by;
            iv->addWidget(label(meta.join(QLatin1String(", ")), "progress", item));
            item->setAccessibleName(tr("%1, translated as %2").arg(source, target));
            m_lookupList->addWidget(item);
        }
    };
    for (bool inTarget : {false, true}) {
        const QJsonObject body{{QStringLiteral("lang"), m_lang}, {QStringLiteral("query"), q}, {QStringLiteral("inTarget"), inTarget}, {QStringLiteral("limit"), 20}};
        onResult(m_conn->api()->post(projectPath() + QStringLiteral("/concordance"), body), this, [hits, pending, show](const ApiResult &r) {
            if (r.ok()) {
                for (const QJsonValue &v : r.array()) {
                    const QJsonObject h = v.toObject();
                    bool dup = false;
                    for (const QJsonValue &x : std::as_const(*hits)) {
                        const QJsonObject o = x.toObject();
                        if (o.value(QLatin1String("memory")) == h.value(QLatin1String("memory"))
                            && o.value(QLatin1String("unit")).toObject().value(QLatin1String("id")) == h.value(QLatin1String("unit")).toObject().value(QLatin1String("id")))
                            dup = true;
                    }
                    if (!dup)
                        hits->append(h);
                }
            }
            if (--*pending == 0)
                show();
        });
    }
}

void EditorPage::refreshFromServer()
{
    if (!m_conn)
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/segments"), q), this, [this](const ApiResult &r) {
        if (!r.ok()) {
            if (r.status == 404)
                notify(tr("This file is no longer in the project."));
            return;
        }
        for (const QJsonValue &v : r.array()) {
            const QJsonObject theirs = v.toObject();
            const QString id = theirs.value(QLatin1String("id")).toString();
            const QJsonObject mine = m_model->segment(m_model->rowOf(id));
            if (theirs.value(QLatin1String("version")).toInt() <= mine.value(QLatin1String("version")).toInt())
                continue;
            if (id == m_activeId && dirty()) {
                showConflict(id, theirs);
                continue;
            }
            m_model->update(theirs);
            if (id == m_activeId && m_target) {
                m_loadedTarget = theirs.value(QLatin1String("target")).toArray();
                m_target->setContent(m_loadedTarget);
                m_model->clearPending(id);
            }
            if (const int row = m_model->rowOf(id); row >= 0)
                m_grid->resizeRowToContents(row);
        }
        updateProgress();
        updatePreview();
        applyFilter();
    });
}

void EditorPage::reloadPresence()
{
    if (!m_conn)
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/presence"), q), this, [this](const ApiResult &r) {
        if (!r.ok())
            return;
        const qint64 me = m_conn->user().value(QLatin1String("id")).toInteger();
        QHash<QString, QStringList> by;
        for (const QJsonValue &v : r.array()) {
            const QJsonObject p = v.toObject();
            if (p.value(QLatin1String("userId")).toInteger() != me)
                by[p.value(QLatin1String("segment")).toString()] << p.value(QLatin1String("userName")).toString();
        }
        m_model->setPresence(by);
        m_rowsTimer.start();
    });
}

void EditorPage::sendPresence()
{
    if (!m_conn || !m_conn->isReady() || m_activeId.isEmpty())
        return;
    m_conn->api()->put(projectPath() + QStringLiteral("/presence"),
                       QJsonObject{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), m_activeId}});
}

void EditorPage::updateProgress()
{
    int total = 0, done = 0;
    for (const QJsonObject &s : m_model->segments()) {
        const int w = wordCount(s.value(QLatin1String("source")).toArray());
        total += w;
        if (s.value(QLatin1String("status")).toString() == QLatin1String("confirmed"))
            done += w;
    }
    m_progressTrack->setRange(0, qMax(1, total));
    m_progressTrack->setValue(done);
    m_progressTrack->setAccessibleName(tr("%1 percent confirmed").arg(qRound(100.0 * done / qMax(1, total))));
    m_progressText->setText(tr("%1 of %2 confirmed").arg(wordsText(done), QLocale().toString(total)));
}

void EditorPage::showPreview(bool on)
{
    if (m_preview->isChecked() != on)
        m_preview->setChecked(on);
}

// The paragraphs of the open document, asked for once for a file.
void EditorPage::loadLayout()
{
    if (!m_conn || (m_layoutAsked && m_layoutFile == m_file))
        return;
    m_layoutAsked = true;
    m_layoutFile = m_file;
    m_layout = {};
    const QString file = m_file;
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/files/layout"), {{QStringLiteral("name"), file}}), this, [this, file](const ApiResult &r) {
        if (file != m_file)
            return;
        // Without the layout the preview shows the segments paragraph by
        // paragraph, as it does for a bilingual file.
        m_layout = r.ok() ? r.object().value(QLatin1String("blocks")).toArray() : QJsonArray();
        updatePreview();
    });
}

QString EditorPage::previewHtml() const
{
    return ::previewHtml(m_model->segments(), m_layout, m_activeId);
}

void EditorPage::updatePreview()
{
    if (!m_previewBox->isVisible())
        return;
    const int scroll = m_previewPane->verticalScrollBar()->value();
    m_previewPane->setHtml(previewHtml());
    m_previewPane->verticalScrollBar()->setValue(scroll);
}

void EditorPage::addWord(const QString &word)
{
    if (!m_conn || m_project.isEmpty())
        return;
    QJsonObject s = settings();
    QJsonObject qa = s.value(QLatin1String("qa")).toObject();
    QJsonArray words = qa.value(QLatin1String("words")).toArray();
    if (!words.contains(word))
        words.append(word);
    qa.insert(QStringLiteral("words"), words);
    s.insert(QStringLiteral("qa"), qa);
    onResult(m_conn->api()->put(projectPath() + QStringLiteral("/settings"), s, m_project.value(QLatin1String("version")).toInt()), this,
             [this, word](const ApiResult &r) {
                 if (!r.ok()) {
                     notify(tr("“%1” was not added: %2").arg(word, r.error.message));
                     return;
                 }
                 m_project = r.object();
                 notify(tr("“%1” is spelled right in this project from now on").arg(word));
                 runChecks();
             });
}

void EditorPage::loadServices()
{
    if (!m_conn)
        return;
    onResult(m_conn->api()->get(QStringLiteral("/api/providers")), this, [this](const ApiResult &r) {
        if (!r.ok())
            return;
        m_services.clear();
        for (const QJsonValue &v : r.array()) {
            if (v.toObject().value(QLatin1String("kind")).toString() != QLatin1String("check"))
                m_services << v.toObject().value(QLatin1String("name")).toString();
        }
        showSuggestions();
    });
}

// The translation services, each with its button, and under them what
// the one asked last answered.
void EditorPage::showSuggestions()
{
    if (!m_suggestList)
        return;
    clear(m_suggestList);
    QWidget *panel = m_panel->widget();
    if (m_services.isEmpty() || m_activeId.isEmpty() || !editable())
        return;
    for (const QString &name : std::as_const(m_services)) {
        auto *b = new QPushButton(Icons::get(QStringLiteral("languages")), m_asking == name ? tr("Asking %1…").arg(name) : tr("Suggest with %1").arg(name), panel);
        b->setObjectName(QStringLiteral("suggestWith:%1").arg(name));
        Theme::smallButton(b);
        b->setIconSize(QSize(14, 14));
        b->setEnabled(m_asking.isEmpty());
        connect(b, &QPushButton::clicked, this, [this, name] { askService(name); });
        m_suggestList->addWidget(b, 0, Qt::AlignLeft);
    }
    if (!m_serviceProblem.isEmpty()) {
        auto *wrong = label(m_serviceProblem, "error", panel);
        wrong->setObjectName(QStringLiteral("serviceProblem"));
        m_suggestList->addWidget(wrong);
    }
    if (m_suggestion.isEmpty())
        return;
    auto *box = new QFrame(panel);
    box->setProperty("role", QStringLiteral("box"));
    box->setObjectName(QStringLiteral("suggestion"));
    auto *bv = new QVBoxLayout(box);
    bv->setContentsMargins(S3, S2, S3, S2);
    bv->setSpacing(S1);
    auto *words = new QLabel(SegmentContent::plainText(m_suggestion.value(QLatin1String("target")).toArray()), box);
    words->setTextFormat(Qt::PlainText);
    words->setWordWrap(true);
    words->setObjectName(QStringLiteral("suggestionWords"));
    bv->addWidget(words);
    QString from = tr("From %1").arg(m_suggestion.value(QLatin1String("name")).toString());
    if (m_suggestion.value(QLatin1String("tagsLost")).toBool())
        from += tr(", without the source's tags: put them in by hand");
    bv->addWidget(label(from, "secondary", box));
    auto *insert = new QPushButton(tr("Insert"), box);
    insert->setObjectName(QStringLiteral("insertSuggestion"));
    Theme::smallButton(insert);
    connect(insert, &QPushButton::clicked, this, &EditorPage::insertSuggestion);
    bv->addWidget(insert, 0, Qt::AlignLeft);
    m_suggestList->addWidget(box);
}

void EditorPage::askService(const QString &name)
{
    if (!m_conn || m_activeId.isEmpty() || !m_asking.isEmpty())
        return;
    const QString id = m_activeId;
    const QJsonObject seg = m_model->segment(activeRow());
    QJsonObject body{{QStringLiteral("sourceLang"), m_sourceLang},
                     {QStringLiteral("targetLang"), m_lang},
                     {QStringLiteral("segments"), QJsonArray{QJsonObject{{QStringLiteral("id"), id}, {QStringLiteral("source"), seg.value(QLatin1String("source"))}}}}};
    m_asking = name;
    m_serviceProblem.clear();
    showSuggestions();
    onResult(m_conn->api()->post(QStringLiteral("/api/providers/%1/translate").arg(ApiClient::seg(name)), body), this, [this, id, name](const ApiResult &r) {
        m_asking.clear();
        if (id != m_activeId) {
            // The answer is for a segment that was left.
            showSuggestions();
            return;
        }
        const QJsonArray ts = r.object().value(QLatin1String("translations")).toArray();
        if (!r.ok() || ts.isEmpty()) {
            m_serviceProblem = r.ok() ? tr("%1 did not translate the segment.").arg(name) : r.error.message;
            m_suggestion = {};
        } else {
            m_suggestion = ts.first().toObject();
            m_suggestion.insert(QStringLiteral("name"), name);
        }
        showSuggestions();
        emit suggested(r.ok() && !ts.isEmpty());
    });
}

void EditorPage::insertSuggestion()
{
    if (!editable() || m_suggestion.isEmpty() || !m_target)
        return;
    m_target->setContent(m_suggestion.value(QLatin1String("target")).toArray());
    emit m_target->edited();
    m_target->setFocus();
}

QJsonArray EditorPage::comments() const
{
    return m_model->segment(activeRow()).value(QLatin1String("comments")).toArray();
}

void EditorPage::showComments()
{
    clear(m_commentsList);
    QWidget *panel = m_commentText->parentWidget();
    const QJsonArray list = m_activeId.isEmpty() ? QJsonArray() : comments();
    m_commentsCount->setVisible(!list.isEmpty());
    m_commentsCount->setText(QString::number(list.size()));
    if (list.isEmpty()) {
        auto *none = label(tr("No comments on this segment."), "muted", panel);
        none->setObjectName(QStringLiteral("noComments"));
        m_commentsList->addWidget(none);
        return;
    }
    // Their author deletes them; on this computer, or managing the project, anyone's.
    const bool all = m_conn && (m_conn->isLocal() || Roles::allows(m_project.value(QLatin1String("access")).toObject().value(QLatin1String("role")).toString(), QStringLiteral("manage")));
    const QString me = m_conn ? m_conn->login() : QString();
    for (const QJsonValue &v : list) {
        const QJsonObject c = v.toObject();
        const QString id = c.value(QLatin1String("id")).toString();
        const QString text = c.value(QLatin1String("text")).toString();
        auto *box = new QFrame(panel);
        box->setProperty("role", QStringLiteral("box"));
        box->setObjectName(QStringLiteral("comment"));
        auto *bv = new QVBoxLayout(box);
        bv->setContentsMargins(S3, S2, S3, S2);
        bv->setSpacing(S1);
        auto *words = new QLabel(text, box);
        words->setTextFormat(Qt::PlainText);
        words->setWordWrap(true);
        words->setObjectName(QStringLiteral("commentWords"));
        bv->addWidget(words);
        auto *meta = new QHBoxLayout;
        meta->setSpacing(S2);
        const QDateTime at = QDateTime::fromString(c.value(QLatin1String("at")).toString(), Qt::ISODate).toLocalTime();
        meta->addWidget(label(tr("%1, %2").arg(c.value(QLatin1String("by")).toString(), QLocale(QLocale::English, QLocale::UnitedKingdom).toString(at.date(), QStringLiteral("d MMM yyyy"))), "secondary", box), 1);
        if (all || c.value(QLatin1String("by")).toString() == me) {
            auto *del = new QPushButton(tr("Delete"), box);
            del->setObjectName(QStringLiteral("deleteComment:%1").arg(id));
            del->setAccessibleName(tr("Delete the comment “%1”").arg(text));
            Theme::smallButton(del);
            Theme::quietButton(del);
            connect(del, &QPushButton::clicked, this, [this, id] { deleteComment(id); });
            meta->addWidget(del);
        }
        bv->addLayout(meta);
        m_commentsList->addWidget(box);
    }
}

// The segment as the server has it now: its comments are taken, the
// translation being worked on stays.
void EditorPage::takeComments(const QJsonObject &segment)
{
    const QString id = segment.value(QLatin1String("id")).toString();
    const int row = m_model->rowOf(id);
    if (row < 0)
        return;
    QJsonObject mine = m_model->segment(row);
    const bool comments = mine.value(QLatin1String("comments")) != segment.value(QLatin1String("comments"));
    const bool issues = mine.value(QLatin1String("issues")) != segment.value(QLatin1String("issues"));
    if (!comments && !issues)
        return;
    for (const QLatin1String key : {QLatin1String("comments"), QLatin1String("issues")}) {
        if (segment.value(key).toArray().isEmpty())
            mine.remove(key);
        else
            mine.insert(key, segment.value(key));
    }
    m_model->update(mine);
    m_grid->resizeRowToContents(row);
    if (id == m_activeId && comments)
        showComments();
    if (id == m_activeId && issues)
        showReviewIssues();
    if (issues)
        loadQuality();
}

QJsonArray EditorPage::reviewIssues() const
{
    return m_model->segment(activeRow()).value(QLatin1String("issues")).toArray();
}

void EditorPage::showReviewIssues()
{
    clear(m_reviewList);
    QWidget *panel = m_issueNote->parentWidget();
    const QJsonArray list = m_activeId.isEmpty() ? QJsonArray() : reviewIssues();
    m_reviewCount->setVisible(!list.isEmpty());
    m_reviewCount->setText(QString::number(list.size()));
    m_issueAdd->setEnabled(!m_activeId.isEmpty());
    if (list.isEmpty()) {
        auto *none = label(tr("No issues marked on this segment."), "muted", panel);
        none->setObjectName(QStringLiteral("noIssues"));
        m_reviewList->addWidget(none);
        return;
    }
    // Who marked an issue deletes it; on this computer, or managing the project, anyone's.
    const bool all = m_conn && (m_conn->isLocal() || Roles::allows(m_project.value(QLatin1String("access")).toObject().value(QLatin1String("role")).toString(), QStringLiteral("manage")));
    const QString me = m_conn ? m_conn->login() : QString();
    const auto categoryName = [](const QString &id) {
        for (const auto &[c, name] : issueCategories())
            if (c == id)
                return name;
        return id;
    };
    for (const QJsonValue &v : list) {
        const QJsonObject i = v.toObject();
        const QString id = i.value(QLatin1String("id")).toString();
        QString severity = i.value(QLatin1String("severity")).toString();
        int points = 0;
        for (const auto &[sv, name, p] : issueSeverities()) {
            if (sv == severity) {
                severity = name.toLower();
                points = p;
            }
        }
        const QString category = categoryName(i.value(QLatin1String("category")).toString());
        const QString note = i.value(QLatin1String("note")).toString();
        auto *box = new QFrame(panel);
        box->setProperty("role", QStringLiteral("box"));
        box->setObjectName(QStringLiteral("reviewIssue"));
        auto *bv = new QVBoxLayout(box);
        bv->setContentsMargins(S3, S2, S3, S2);
        bv->setSpacing(S1);
        auto *words = new QLabel(QStringLiteral("<b>%1</b>, %2 (%3)%4").arg(category.toHtmlEscaped(), severity.toHtmlEscaped(), points == 1 ? tr("1 point") : tr("%1 points").arg(points),
                                                                           note.isEmpty() ? QString() : QStringLiteral(": ") + note.toHtmlEscaped()),
                                 box);
        words->setTextFormat(Qt::RichText);
        words->setWordWrap(true);
        words->setObjectName(QStringLiteral("issueWords"));
        bv->addWidget(words);
        auto *meta = new QHBoxLayout;
        meta->setSpacing(S2);
        const QDateTime at = QDateTime::fromString(i.value(QLatin1String("at")).toString(), Qt::ISODate).toLocalTime();
        meta->addWidget(label(tr("%1, %2").arg(i.value(QLatin1String("by")).toString(), QLocale(QLocale::English, QLocale::UnitedKingdom).toString(at.date(), QStringLiteral("d MMM yyyy"))), "secondary", box), 1);
        if (all || i.value(QLatin1String("by")).toString() == me) {
            auto *del = new QPushButton(tr("Delete"), box);
            del->setObjectName(QStringLiteral("deleteIssue:%1").arg(id));
            del->setAccessibleName(tr("Delete the issue %1, %2").arg(category, severity));
            Theme::smallButton(del);
            Theme::quietButton(del);
            connect(del, &QPushButton::clicked, this, [this, id] { deleteIssue(id); });
            meta->addWidget(del);
        }
        bv->addLayout(meta);
        m_reviewList->addWidget(box);
    }
}

void EditorPage::addIssue(const QString &category, const QString &severity, const QString &note)
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), m_activeId}};
    const QString id = m_activeId;
    QJsonObject body{{QStringLiteral("category"), category}, {QStringLiteral("severity"), severity}};
    if (!note.trimmed().isEmpty())
        body.insert(QStringLiteral("note"), note.trimmed());
    m_issueAdd->setEnabled(false);
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/issues"), body, q), this, [this, id](const ApiResult &r) {
        m_issueAdd->setEnabled(!m_activeId.isEmpty());
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        if (id == m_activeId)
            m_issueNote->clear();
        takeComments(r.object());
        emit reviewed(id);
    });
}

void EditorPage::deleteIssue(const QString &issueId)
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), m_activeId}, {QStringLiteral("issue"), issueId}};
    const QString id = m_activeId;
    onResult(m_conn->api()->del(projectPath() + QStringLiteral("/issues"), q), this, [this, id](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        takeComments(r.object());
        emit reviewed(id);
    });
}

// The file's score, as the server counts it.
void EditorPage::loadQuality()
{
    if (!m_conn)
        return;
    // Answers may come back in another order than they were asked for: only the last one asked counts.
    const int asked = ++m_qualityAsked;
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/review"), {{QStringLiteral("lang"), m_lang}}), this, [this, asked](const ApiResult &r) {
        if (!r.ok() || asked != m_qualityAsked)
            return;
        for (const QJsonValue &v : r.object().value(QLatin1String("files")).toArray()) {
            if (v.toObject().value(QLatin1String("file")).toString() == m_file)
                m_quality = v.toObject();
        }
        if (m_quality.isEmpty())
            return;
        const int words = m_quality.value(QLatin1String("words")).toInt();
        const int n = m_quality.value(QLatin1String("issues")).toInt();
        const int penalty = m_quality.value(QLatin1String("penalty")).toInt();
        const QString wordsText = words == 1 ? tr("1 word") : tr("%1 words").arg(words);
        m_qualityScore->setText(tr("Score of this file <b style=\"font-size:20px\">%1</b> <span style=\"color:%2\">of 100</span>")
                                    .arg(QString::number(m_quality.value(QLatin1String("score")).toDouble(), 'f', 1), Theme::color("text3").name()));
        QString text;
        if (n == 0) {
            text = tr("No issues marked in %1.").arg(wordsText);
        } else {
            QStringList parts;
            if (const int k = m_quality.value(QLatin1String("minor")).toInt())
                parts << tr("%1 minor").arg(k);
            if (const int k = m_quality.value(QLatin1String("major")).toInt())
                parts << tr("%1 major").arg(k);
            if (const int k = m_quality.value(QLatin1String("critical")).toInt())
                parts << tr("%1 critical").arg(k);
            text = tr("%1 (%2), %3 in %4.").arg(n == 1 ? tr("1 issue") : tr("%1 issues").arg(n), parts.join(QStringLiteral(", ")),
                                              penalty == 1 ? tr("1 penalty point") : tr("%1 penalty points").arg(penalty), wordsText);
        }
        m_qualityText->setText(text + QLatin1Char(' ') + tr("The score is 100 less the penalty points in percent of the words."));
        m_qualityScore->show();
        m_qualityText->show();
        emit scored();
    });
}

void EditorPage::saveReviewTo(const QString &path)
{
    if (!m_conn)
        return;
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/review"), {{QStringLiteral("format"), QStringLiteral("csv")}}), this, [this, path](const ApiResult &r) {
        if (!r.ok()) {
            notify(tr("Could not save the review: %1").arg(r.error.message));
            emit reviewSaved(false);
            return;
        }
        QSaveFile f(path);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size() || !f.commit()) {
            notify(tr("Could not save the review: %1").arg(f.errorString()));
            emit reviewSaved(false);
            return;
        }
        notify(tr("Saved %1").arg(QFileInfo(path).fileName()));
        emit reviewSaved(true);
    });
}

void EditorPage::refreshComments()
{
    if (!m_conn)
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}};
    onResult(m_conn->api()->get(projectPath() + QStringLiteral("/segments"), q), this, [this](const ApiResult &r) {
        if (!r.ok())
            return;
        for (const QJsonValue &v : r.array())
            takeComments(v.toObject());
        // A segment saved meanwhile may have brought them already: what
        // is shown for the open segment is what the list has now.
        showComments();
        showReviewIssues();
    });
}

void EditorPage::addComment(const QString &text)
{
    const QString said = text.trimmed();
    if (!m_conn || m_activeId.isEmpty() || said.isEmpty())
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), m_activeId}};
    const QString id = m_activeId;
    m_commentAdd->setEnabled(false);
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/comments"), QJsonObject{{QStringLiteral("text"), said}}, q), this, [this, id](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            m_commentAdd->setEnabled(!m_commentText->toPlainText().trimmed().isEmpty());
            return;
        }
        if (id == m_activeId)
            m_commentText->clear();
        takeComments(r.object());
        emit commented(id);
    });
}

void EditorPage::deleteComment(const QString &commentId)
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const QUrlQuery q{{QStringLiteral("file"), m_file}, {QStringLiteral("lang"), m_lang}, {QStringLiteral("segment"), m_activeId}, {QStringLiteral("comment"), commentId}};
    const QString id = m_activeId;
    onResult(m_conn->api()->del(projectPath() + QStringLiteral("/comments"), q), this, [this, id](const ApiResult &r) {
        if (!r.ok()) {
            notify(r.error.message);
            return;
        }
        takeComments(r.object());
        emit commented(id);
    });
}

namespace {

// What find and replace looks for; a place lies within the text between
// two tags, so the tags stay where they are. The same rules are in the
// web client (replace.ts).
QRegularExpression searchPattern(const QString &find, bool matchCase, bool wholeWords)
{
    QString body = QRegularExpression::escape(find);
    // A whole word has no letter or digit before or after it.
    if (wholeWords)
        body = QStringLiteral("(?<![\\p{L}\\p{N}])%1(?![\\p{L}\\p{N}])").arg(body);
    QRegularExpression::PatternOptions o = QRegularExpression::UseUnicodePropertiesOption;
    if (!matchCase)
        o |= QRegularExpression::CaseInsensitiveOption;
    return QRegularExpression(body, o);
}

// The content with every place replaced, and how many there were.
QJsonArray replaceIn(const QJsonArray &content, const QRegularExpression &re, const QString &with, int *count)
{
    QJsonArray out;
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (!run.contains(QLatin1String("text"))) {
            out.append(run);
            continue;
        }
        const QString text = run.value(QLatin1String("text")).toString();
        QString next;
        int at = 0;
        for (auto it = re.globalMatch(text); it.hasNext();) {
            const QRegularExpressionMatch m = it.next();
            if (m.capturedLength() == 0)
                continue;
            next += text.mid(at, m.capturedStart() - at);
            next += with;
            at = int(m.capturedEnd());
            ++*count;
        }
        next += text.mid(at);
        if (!next.isEmpty())
            out.append(QJsonObject{{QStringLiteral("text"), next}});
    }
    return out;
}

} // namespace

void EditorPage::replaceAll(const QString &find, const QString &with, bool matchCase, bool wholeWords)
{
    if (find.isEmpty() || !m_conn)
        return;
    const QRegularExpression re = searchPattern(find, matchCase, wholeWords);
    struct Change {
        QString id;
        QJsonArray target;
    };
    auto changes = std::make_shared<QList<Change>>();
    int places = 0;
    for (int row = 0; row < m_model->rowCount(); ++row) {
        if (m_model->index(row, 0).data(SegmentModel::LockedRole).toBool())
            continue;
        const QJsonObject s = m_model->segment(row);
        const QString id = s.value(QLatin1String("id")).toString();
        // The open segment is replaced as it stands in its editor.
        const QJsonArray now = id == m_activeId && m_target ? m_target->content() : s.value(QLatin1String("target")).toArray();
        int n = 0;
        const QJsonArray next = replaceIn(now, re, with, &n);
        if (n > 0) {
            places += n;
            changes->append({id, next});
        }
    }
    if (changes->isEmpty())
        return;
    const int total = int(changes->size());
    // One after the other, so a segment someone else changed stops nothing else.
    auto step = std::make_shared<std::function<void(int)>>();
    *step = [this, changes, step, places, total](int i) {
        if (i >= changes->size()) {
            notify(tr("Replaced %1 in %2")
                       .arg(places == 1 ? tr("1 place") : tr("%1 places").arg(QLocale(QLocale::English).toString(places)),
                            total == 1 ? tr("1 segment") : tr("%1 segments").arg(QLocale(QLocale::English).toString(total))));
            emit replaced(places, total);
            return;
        }
        const Change c = changes->at(i);
        if (c.id == m_activeId && m_target)
            m_target->setContent(c.target);
        // A segment that changes is a draft again; one emptied is new.
        const bool empty = SegmentContent::plainText(c.target).trimmed().isEmpty() && c.target.isEmpty();
        save(c.id, c.target, empty ? QStringLiteral("new") : QStringLiteral("draft"), {}, {}, [step, i](bool) { (*step)(i + 1); });
    };
    (*step)(0);
}

void EditorPage::updateCut()
{
    if (!m_cutBox)
        return;
    const int row = activeRow();
    const QJsonObject s = m_model->segment(row);
    const QJsonObject next = row >= 0 && row + 1 < m_model->rowCount() ? m_model->segment(row + 1) : QJsonObject();
    const auto locked = [this](int r) { return m_model->index(r, 0).data(SegmentModel::LockedRole).toBool(); };
    // Trados, memoQ, Phrase, Wordfast, TagEditor and Déjà Vu keep their own record of every segment of their files.
    const auto kept = [](const QJsonObject &seg) { return seg.value(QLatin1String("file")).toObject().value(QLatin1String("fixed")).toBool(); };
    const bool fixed = kept(s) || hasSuffix(m_file, {".sdlxliff", ".mqxliff", ".mxliff", ".txml", ".ttx", ".mqxlz", ".rtf"});
    const bool cuts = row >= 0 && !locked(row) && !fixed;
    const int joinedFrom = int(s.value(QLatin1String("joined")).toArray().size());
    const QString part = s.value(QLatin1String("split")).toObject().value(QLatin1String("unit")).toString();
    const QString nextPart = next.value(QLatin1String("split")).toObject().value(QLatin1String("unit")).toString();
    const QString block = s.value(QLatin1String("block")).toString();
    const bool joins = cuts && !next.isEmpty() && !locked(row + 1) && !kept(next)
        && ((!part.isEmpty() && part == nextPart) || (!block.isEmpty() && block == next.value(QLatin1String("block")).toString()));
    m_split->setEnabled(cuts);
    m_join->setEnabled(joins);
    m_cutNote->setText(fixed ? tr("The program this file comes from keeps its own record of every segment, so its segments stay as they are.")
                       : !part.isEmpty() && joinedFrom > 0 ? tr("A part of %1 segments of the file that were joined.").arg(joinedFrom)
                       : joinedFrom > 0 ? tr("Joined from %1 segments of the file. Split it where they met to have them apart again.").arg(joinedFrom)
                       : !part.isEmpty() ? tr("A part of a segment of the file that was split.")
                                         : QString());
    m_cutNote->setVisible(!m_cutNote->text().isEmpty());
}

void EditorPage::takeSegments(const QJsonArray &segments, int row)
{
    if (const int old = activeRow(); old >= 0) {
        m_model->setMarks(m_activeId, {});
        m_model->clearPending(m_activeId);
        m_target = nullptr;
        m_source = nullptr;
        m_counter = nullptr;
        m_subtitleCounter = nullptr;
        m_grid->closePersistentEditor(m_model->index(old, SegmentModel::Source));
        m_grid->closePersistentEditor(m_model->index(old, SegmentModel::Target));
    }
    m_saveTimer.stop();
    m_activeId.clear();
    m_model->setSegments(segments);
    applyFilter();
    updateProgress();
    m_grid->resizeRowsToContents();
    if (m_model->rowCount() > 0)
        goToSegment(m_model->segment(qBound(0, row, m_model->rowCount() - 1)).value(QLatin1String("id")).toString());
}

void EditorPage::resegment(const QString &how, const QJsonArray &before, std::function<void(const QString &)> failed)
{
    if (!m_conn || m_activeId.isEmpty())
        return;
    const QString id = m_activeId;
    const auto go = [this, how, before, failed, id] {
        const int row = m_model->rowOf(id);
        if (row < 0 || id != m_activeId)
            return;
        const QUrlQuery q{{QStringLiteral("file"), m_file},
                          {QStringLiteral("lang"), m_lang},
                          {QStringLiteral("segment"), id},
                          {QStringLiteral("version"), QString::number(m_model->segment(row).value(QLatin1String("version")).toInt())}};
        const QJsonValue body = how == QLatin1String("split") ? QJsonValue(QJsonObject{{QStringLiteral("before"), before}}) : QJsonValue();
        onResult(m_conn->api()->post(projectPath() + QStringLiteral("/segments/") + how, body, q), this, [this, how, failed, row](const ApiResult &r) {
            if (!r.ok()) {
                if (failed)
                    failed(r.error.message);
                else
                    notify(r.error.message);
                emit resegmented(how, false);
                return;
            }
            takeSegments(r.array(), row);
            notify(how == QLatin1String("split") ? tr("Split in two segments") : tr("Joined with the next segment"));
            emit resegmented(how, true);
        });
    };
    // What was typed is saved first, so it is what gets split or joined.
    if (dirty()) {
        m_saveTimer.stop();
        const QJsonArray target = m_target->content();
        const bool empty = plainText(target).trimmed().isEmpty() && SegmentContent::tags(target).isEmpty();
        save(id, target, empty ? QStringLiteral("new") : QStringLiteral("draft"), {}, m_origin, [go, this, how](bool ok) {
            if (ok)
                go();
            else
                emit resegmented(how, false);
        });
    } else {
        go();
    }
}

Web::Dialog *EditorPage::showSplitDialog()
{
    const int row = activeRow();
    if (row < 0 || !m_split || !m_split->isEnabled())
        return nullptr;
    const QJsonArray source = m_model->segment(row).value(QLatin1String("source")).toArray();
    auto *d = new Web::Dialog(tr("Split the segment"), window());
    d->setObjectName(QStringLiteral("splitDialog"));
    d->setAttribute(Qt::WA_DeleteOnClose);
    QString how = tr("Click where the second part begins, or go there with the arrow keys. The translation stays with the first part.");
    if (!m_model->segment(row).value(QLatin1String("joined")).toArray().isEmpty())
        how += QLatin1Char(' ') + tr("Split where two joined segments met, they stand apart again.");
    auto *help = Web::text(how, "help", d);
    help->setWordWrap(true);
    // The source with a caret in it, which nothing can be typed at.
    auto *box = new TargetEditor(d);
    box->setObjectName(QStringLiteral("splitSource"));
    box->setAccessibleName(tr("Source of the segment"));
    box->setAccessibleDescription(help->text());
    box->setContent(source);
    box->setReadOnly(true);
    box->setTextInteractionFlags(Qt::TextSelectableByMouse | Qt::TextSelectableByKeyboard);
    box->setProperty("lines", QStringLiteral("few"));
    box->moveCursor(QTextCursor::Start);
    auto *parts = new QWidget(d);
    auto *grid = new QGridLayout(parts);
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setHorizontalSpacing(S3);
    grid->setVerticalSpacing(6);
    auto *first = new QLabel(parts);
    auto *second = new QLabel(parts);
    first->setObjectName(QStringLiteral("splitFirst"));
    second->setObjectName(QStringLiteral("splitSecond"));
    for (QLabel *l : {first, second}) {
        l->setWordWrap(true);
        l->setTextFormat(Qt::RichText);
        l->setMinimumHeight(l->fontMetrics().lineSpacing() + 4);
        l->setAlignment(Qt::AlignLeft | Qt::AlignTop);
    }
    grid->addWidget(Web::text(tr("First part"), "fieldLabel", parts), 0, 0, Qt::AlignTop);
    grid->addWidget(first, 0, 1);
    grid->addWidget(Web::text(tr("Second part"), "fieldLabel", parts), 1, 0, Qt::AlignTop);
    grid->addWidget(second, 1, 1);
    grid->setColumnStretch(1, 1);
    d->body()->addWidget(help);
    d->body()->addWidget(box);
    d->body()->addWidget(parts);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, d);
    auto *go = Web::button(tr("Split"), Web::Primary | Web::Small, d);
    go->setObjectName(QStringLiteral("save"));
    d->buttons()->addWidget(cancel);
    d->buttons()->addWidget(go);
    connect(cancel, &QPushButton::clicked, d, &QDialog::reject);
    // The caret divides the source: what is before it and what is after.
    const auto side = [box](bool before) {
        QTextCursor c = box->textCursor();
        c.setPosition(c.position());
        c.movePosition(before ? QTextCursor::Start : QTextCursor::End, QTextCursor::KeepAnchor);
        QTextDocument part;
        SegmentContent::enableTags(&part);
        QTextCursor(&part).insertFragment(c.selection());
        return SegmentContent::fromDocument(&part);
    };
    const auto show = [side, first, second, go] {
        static const QRegularExpression text(QStringLiteral("[\\p{L}\\p{N}]"), QRegularExpression::UseUnicodePropertiesOption);
        const QJsonArray a = side(true), b = side(false);
        const bool ok = plainText(a).contains(text) && plainText(b).contains(text);
        const QString muted = QStringLiteral("<span style=\"color:%1\">%2</span>").arg(Theme::color("text3").name());
        first->setText(ok ? SegmentContent::toHtml(trimmedContent(a, true)) : muted.arg(tr("Place the cursor inside the text.")));
        second->setText(ok ? SegmentContent::toHtml(trimmedContent(b, false)) : muted.arg(tr("Both parts need text.")));
        go->setEnabled(ok);
    };
    connect(box, &QTextEdit::cursorPositionChanged, d, show);
    show();
    QPointer<Web::Dialog> dialog(d);
    connect(go, &QPushButton::clicked, this, [this, dialog, side, go] {
        if (!dialog)
            return;
        go->setEnabled(false);
        dialog->setError(QString());
        resegment(QStringLiteral("split"), side(true), [dialog, go](const QString &why) {
            if (!dialog)
                return;
            dialog->setError(why);
            go->setEnabled(true);
        });
    });
    connect(this, &EditorPage::resegmented, d, [dialog](const QString &how, bool ok) {
        if (dialog && ok && how == QLatin1String("split"))
            dialog->accept();
    });
    d->open();
    d->layout()->activate();
    d->resize(d->width(), qMax(d->height(), d->layout()->totalHeightForWidth(d->width())));
    box->setFocus();
    return d;
}

Web::Dialog *EditorPage::showReplaceDialog()
{
    if (!m_model || m_model->rowCount() == 0)
        return nullptr;
    auto *d = new Web::Dialog(tr("Find and replace"), window());
    d->setObjectName(QStringLiteral("replaceDialog"));
    d->setAttribute(Qt::WA_DeleteOnClose);
    auto *find = new QLineEdit(m_find ? m_find->text().trimmed() : QString(), d);
    find->setObjectName(QStringLiteral("replaceFind"));
    auto *with = new QLineEdit(d);
    with->setObjectName(QStringLiteral("replaceWith"));
    auto *matchCase = new QCheckBox(tr("Match case"), d);
    matchCase->setObjectName(QStringLiteral("matchCase"));
    auto *wholeWords = new QCheckBox(tr("Whole words only"), d);
    wholeWords->setObjectName(QStringLiteral("wholeWords"));
    auto *status = new QLabel(d);
    status->setObjectName(QStringLiteral("replaceStatus"));
    status->setWordWrap(true);
    auto *help = Web::text(tr("Formatting tags stay where they are. A confirmed segment that changes is a draft again, to be confirmed anew."), "help", d);
    help->setWordWrap(true);
    d->body()->addWidget(Web::field(tr("Find in the translations"), find, d));
    d->body()->addWidget(Web::field(tr("Replace with"), with, d));
    d->body()->addWidget(matchCase);
    d->body()->addWidget(wholeWords);
    d->body()->addWidget(status);
    d->body()->addWidget(help);
    auto *cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, d);
    auto *go = Web::button(tr("Replace all"), Web::Primary | Web::Small, d);
    go->setObjectName(QStringLiteral("save"));
    d->buttons()->addWidget(cancel);
    d->buttons()->addWidget(go);
    connect(cancel, &QPushButton::clicked, d, &QDialog::reject);
    const auto count = [this, find, with, matchCase, wholeWords, status, go] {
        const auto many = [](int n, const QString &one, const QString &more) { return n == 1 ? one : more.arg(QLocale(QLocale::English).toString(n)); };
        int places = 0, segments = 0, locked = 0;
        if (!find->text().isEmpty()) {
            const QRegularExpression re = searchPattern(find->text(), matchCase->isChecked(), wholeWords->isChecked());
            for (int row = 0; row < m_model->rowCount(); ++row) {
                const QJsonObject s = m_model->segment(row);
                const QString id = s.value(QLatin1String("id")).toString();
                int n = 0;
                replaceIn(id == m_activeId && m_target ? m_target->content() : s.value(QLatin1String("target")).toArray(), re, with->text(), &n);
                if (n == 0)
                    continue;
                if (m_model->index(row, 0).data(SegmentModel::LockedRole).toBool()) {
                    ++locked;
                } else {
                    places += n;
                    ++segments;
                }
            }
        }
        QString said = find->text().isEmpty() ? tr("Nothing to find yet.")
                       : places == 0          ? tr("Not found in the translations of this file.")
                                              : tr("%1 in %2.").arg(many(places, tr("1 place"), tr("%1 places")), many(segments, tr("1 segment"), tr("%1 segments")));
        if (locked > 0)
            said += QLatin1Char(' ') + (locked == 1 ? tr("1 locked segment has it too and stays as it is.") : tr("%1 locked segments have it too and stay as they are.").arg(QLocale(QLocale::English).toString(locked)));
        status->setText(said);
        go->setEnabled(places > 0);
    };
    connect(find, &QLineEdit::textChanged, d, count);
    connect(with, &QLineEdit::textChanged, d, count);
    connect(matchCase, &QCheckBox::toggled, d, count);
    connect(wholeWords, &QCheckBox::toggled, d, count);
    count();
    QPointer<Web::Dialog> dialog(d);
    connect(go, &QPushButton::clicked, this, [this, dialog, find, with, matchCase, wholeWords] {
        if (!dialog)
            return;
        replaceAll(find->text(), with->text(), matchCase->isChecked(), wholeWords->isChecked());
        dialog->accept();
    });
    d->open();
    find->setFocus();
    return d;
}

Web::Dialog *EditorPage::showMemoriesDialog()
{
    // As the web's "Translation memories" dialog.
    auto *dlg = new Web::Dialog(tr("Translation memories"), this);
    dlg->setObjectName(QStringLiteral("memoriesDialog"));
    dlg->setAttribute(Qt::WA_DeleteOnClose);
    auto *help = label(tr("Memories for %1, highest priority first. Matches are scored in the core, with each memory's penalty taken off. "
                          "<a href=\"project\">Change them in the project settings</a>.")
                           .arg(Languages::name(m_lang)),
                       "secondary", dlg);
    help->setTextFormat(Qt::RichText);
    connect(help, &QLabel::linkActivated, dlg, [this, dlg] {
        dlg->accept();
        saveActive();
        emit openProject();
    });
    dlg->body()->addWidget(help);
    const QJsonArray mems = memorySettings();
    const QString projectMemory = settings().value(QLatin1String("projectMemory")).toBool()
        ? QStringLiteral("%1 (%2)").arg(settings().value(QLatin1String("name")).toString(), m_lang)
        : QString();
    if (mems.isEmpty()) {
        dlg->body()->addWidget(label(tr("This language of the project uses no memory, so the editor shows no matches."), "muted", dlg));
    } else {
        auto *t = new Web::Table({tr("Memory"), tr("Used for"), tr("Units"), QString()}, dlg);
        t->setObjectName(QStringLiteral("projectMemories"));
        t->setAccessibleName(tr("Memories of this project"));
        t->setProperty("inDialog", true);
        t->setNumeric({2});
        t->setStretch(0);
        for (int i = 0; i < mems.size(); ++i) {
            const QJsonObject m = mems.at(i).toObject();
            const QString name = m.value(QLatin1String("memory")).toString();
            const bool isProject = name == projectMemory;
            QStringList uses;
            if (m.value(QLatin1String("lookup")).toBool()) {
                const int penalty = m.value(QLatin1String("penalty")).toInt();
                uses << (penalty ? tr("lookup (penalty %1)").arg(penalty) : tr("lookup"));
            }
            if (m.value(QLatin1String("concordance")).toBool())
                uses << tr("concordance");
            if (m.value(QLatin1String("update")).toBool())
                uses << tr("updated on confirm");
            auto *units = qobject_cast<QLabel *>(Web::cellText(QStringLiteral("…"), t, true));
            QPointer<QLabel> unitsLabel(units);
            auto fillUnits = [this, name, unitsLabel] {
                onResult(m_conn->api()->get(QStringLiteral("/api/memories/%1").arg(ApiClient::seg(name))), this, [unitsLabel](const ApiResult &r) {
                    if (unitsLabel)
                        unitsLabel->setText(r.ok() ? QLocale().toString(r.object().value(QLatin1String("units")).toInt()) : QStringLiteral("–"));
                });
            };
            fillUnits();
            QList<QWidget *> actions;
            if (isProject) {
                auto *imp = Web::button(tr("Import"), Web::Small, t, QStringLiteral("upload"));
                imp->setAccessibleName(tr("Import a memory file into %1").arg(name));
                imp->setToolTip(tr("A memory file: .tmx, .sdltm or .txt."));
                connect(imp, &QPushButton::clicked, this, [this, name, fillUnits, dlg] {
                    const QString path = QFileDialog::getOpenFileName(
                        dlg, tr("Import a memory file into %1").arg(name), QString(),
                        tr("Memory files (*.tmx *.sdltm *.txt);;.tmx (*.tmx);;.sdltm (*.sdltm);;.txt (*.txt);;All files (*)"));
                    if (path.isEmpty())
                        return;
                    QFile f(path);
                    const QString fileName = QFileInfo(path).fileName();
                    if (!f.open(QIODevice::ReadOnly)) {
                        notify(tr("Could not import %1: %2").arg(fileName, f.errorString()));
                        return;
                    }
                    onResult(m_conn->api()->upload("POST", QStringLiteral("/api/memories/%1/import").arg(ApiClient::seg(name)), f.readAll(),
                                                   QStringLiteral("application/octet-stream")),
                             this, [this, name, fileName, fillUnits](const ApiResult &r) {
                                 if (!r.ok()) {
                                     notify(tr("Could not import %1: %2").arg(fileName, r.error.message));
                                     return;
                                 }
                                 notify(MemoriesPage::importNotice(r.object(), name, fileName));
                                 fillUnits();
                             });
                });
                actions << imp;
            }
            // The memory out as TMX or as a .txt memory.
            const auto exportAs = [this, name, dlg](const QString &format) {
                QString base = name;
                base.replace(QRegularExpression(QStringLiteral("[^\\p{L}\\p{N}]+")), QStringLiteral("_"));
                const bool wordfast = format == QLatin1String("wordfast");
                const QString to = QFileDialog::getSaveFileName(
                    dlg, wordfast ? tr("Export %1 as a .txt memory").arg(name) : tr("Export TMX"),
                    QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation) + QLatin1Char('/') + base + (wordfast ? QStringLiteral(".txt") : QStringLiteral(".tmx")),
                    wordfast ? tr(".txt memories (*.txt)") : tr("TMX files (*.tmx)"));
                if (to.isEmpty())
                    return;
                QUrlQuery q;
                if (wordfast)
                    q.addQueryItem(QStringLiteral("format"), format);
                onResult(m_conn->api()->get(projectPath() + QStringLiteral("/memories/%1/export").arg(ApiClient::seg(name)), q), this, [this, to](const ApiResult &r) {
                    if (!r.ok()) {
                        notify(r.error.message);
                        return;
                    }
                    QSaveFile f(to);
                    if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size() || !f.commit()) {
                        notify(tr("Could not write %1.").arg(QDir::toNativeSeparators(to)));
                        return;
                    }
                    notify(tr("Saved %1").arg(QFileInfo(to).fileName()));
                });
            };
            auto *exp = Web::button(tr("Export TMX"), Web::Quiet | Web::Small, t, QStringLiteral("download"));
            connect(exp, &QPushButton::clicked, this, [exportAs] { exportAs(QStringLiteral("tmx")); });
            actions << exp;
            auto *wf = Web::button(tr("Export .txt"), Web::Quiet | Web::Small, t, QStringLiteral("download"));
            wf->setAccessibleName(tr("Export %1 as a .txt memory").arg(name));
            connect(wf, &QPushButton::clicked, this, [exportAs] { exportAs(QStringLiteral("wordfast")); });
            actions << wf;
            t->addRow({Web::cell(name, isProject ? tr("project memory") : QString(), t), Web::cellText(uses.join(QLatin1String(", ")), t), units,
                       Web::cellActions(actions, t)});
        }
        t->fit();
        dlg->body()->addWidget(t);
    }
    auto *close = Web::button(tr("Close"), Web::Primary | Web::Small, dlg);
    connect(close, &QPushButton::clicked, dlg, &QDialog::accept);
    dlg->buttons()->addWidget(close);
    dlg->open();
    return dlg;
}

Web::Dialog *EditorPage::showAnalysisDialog()
{
    if (!m_conn)
        return nullptr;
    auto *dlg = new Web::Dialog(tr("Analysis"), this);
    dlg->setObjectName(QStringLiteral("analysisDialog"));
    dlg->setAttribute(Qt::WA_DeleteOnClose);
    auto *help = label(tr("Analysing…"), "secondary", dlg);
    dlg->body()->addWidget(help);
    auto *holder = new QVBoxLayout;
    dlg->body()->addLayout(holder);
    auto *close = Web::button(tr("Close"), Web::Primary | Web::Small, dlg);
    connect(close, &QPushButton::clicked, dlg, &QDialog::accept);
    dlg->buttons()->addWidget(close);
    QPointer<Web::Dialog> alive(dlg);
    onResult(m_conn->api()->post(projectPath() + QStringLiteral("/analysis"), QJsonObject{{QStringLiteral("lang"), m_lang}}), dlg,
             [this, help, holder, alive](const ApiResult &r) {
                 if (!alive)
                     return;
                 if (!r.ok()) {
                     help->setText(r.error.message);
                     return;
                 }
                 QJsonObject report;
                 for (const QJsonValue &x : r.array()) {
                     if (x.toObject().value(QLatin1String("targetLang")).toString() == m_lang)
                         report = x.toObject();
                 }
                 QStringList memories;
                 for (const QJsonValue &m : report.value(QLatin1String("memories")).toArray())
                     memories << m.toString();
                 const int files = int(report.value(QLatin1String("files")).toArray().size());
                 const QString projectMemory = QStringLiteral("%1 (%2)").arg(settings().value(QLatin1String("name")).toString(), m_lang);
                 QString text = tr("%1 in %2 against %3, counted in this order: context matches first, then repetitions within a file and "
                                   "across files, then the best match. Below 50% counts as new.")
                                    .arg(plural(files, tr("file"), tr("files")), Languages::name(m_lang),
                                         memories.isEmpty() ? tr("no memory") : memories.join(tr(" and ")));
                 if (settings().value(QLatin1String("projectMemory")).toBool() && !memories.contains(projectMemory))
                     text += QLatin1Char(' ') + tr("The project memory is left out: it holds this project's own translations.");
                 help->setText(text);
                 holder->addWidget(Bands::analysisTable(report, alive));
                 emit analysisShown();
             });
    dlg->open();
    return dlg;
}

void EditorPage::saveDocument()
{
    if (!m_conn)
        return;
    saveActive();
    const QString label = m_save->text();
    const bool package = label == tr("Save return package");
    const QString path = package ? projectPath() + QStringLiteral("/return-package") : projectPath() + QStringLiteral("/files/translated");
    const QUrlQuery q = package ? QUrlQuery{{QStringLiteral("lang"), m_lang}} : QUrlQuery{{QStringLiteral("name"), m_file}, {QStringLiteral("lang"), m_lang}};
    m_save->setEnabled(false);
    m_save->setText(tr("Saving…"));
    onResult(m_conn->api()->get(path, q), this, [this, label](const ApiResult &r) {
        m_save->setEnabled(true);
        m_save->setText(label);
        if (!r.ok()) {
            notify(tr("Could not save the document: %1").arg(r.error.message));
            return;
        }
        const QString name = r.fileName().isEmpty() ? QFileInfo(m_file).fileName() : r.fileName();
        const QString to = QFileDialog::getSaveFileName(this, label, QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation) + QLatin1Char('/') + name);
        if (to.isEmpty())
            return;
        QSaveFile f(to);
        if (!f.open(QIODevice::WriteOnly) || f.write(r.body) != r.body.size() || !f.commit()) {
            notify(tr("Could not save the document: %1").arg(f.errorString()));
            return;
        }
        int open = 0;
        for (const QJsonObject &s : m_model->segments()) {
            if (s.value(QLatin1String("status")).toString() != QLatin1String("confirmed") && !s.value(QLatin1String("locked")).toBool())
                ++open;
        }
        QString text = tr("Saved %1.").arg(QFileInfo(to).fileName());
        if (open)
            text += QLatin1Char(' ') + (open == 1 ? tr("1 segment is not confirmed yet.") : tr("%1 segments are not confirmed yet.").arg(open));
        notify(text);
    });
}
