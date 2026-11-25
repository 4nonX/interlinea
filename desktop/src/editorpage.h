#pragma once

#include "segmentmodel.h"
#include "targeteditor.h"

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QLabel>
#include <QTextBrowser>
#include <QPointer>
#include <QTimer>
#include <QWidget>

#include <functional>

class Connection;
namespace Web {
class Dialog;
}
class QButtonGroup;
class QComboBox;
class QFrame;
class QLabel;
class QLineEdit;
class QPlainTextEdit;
class QProgressBar;
class QPushButton;
class QScrollArea;
class QTableView;
class QVBoxLayout;

// Translating one file of a project into one language, laid out as the web
// editor: a bar with the project, file, progress, filter and actions; the
// segments, typed into in place; and the results panel beside them.
class EditorPage : public QWidget {
    Q_OBJECT
public:
    EditorPage(Connection *connection, qint64 projectId, const QString &file, const QString &lang,
               QWidget *parent = nullptr);
    ~EditorPage() override;

    QString file() const { return m_file; }
    // The preview as it is shown: the HTML of the page, and the
    // paragraphs it is built from.
    QString previewHtml() const;
    QJsonArray layout() const { return m_layout; }
    void showPreview(bool on);
    QString lang() const { return m_lang; }
    SegmentModel *model() const { return m_model; }
    QTableView *grid() const { return m_grid; }
    // The active segment's translation, while there is one.
    TargetEditor *target() const { return m_target; }
    int activeRow() const;
    QString activeId() const { return m_activeId; }
    int shownCount() const;
    QJsonArray results() const { return m_results; }
    QJsonArray issues() const { return m_issues; }
    // What the project's grammar service found in the open segment's
    // translation as it is now: issues of the kind "grammar", each with
    // its place (offset, length) and replacements. Empty until it has
    // read the translation.
    QJsonArray grammarIssues() const;
    // The grammar service the project names, or nothing.
    QString grammarService() const { return m_project.value(QLatin1String("settings")).toObject().value(QLatin1String("qa")).toObject().value(QLatin1String("grammar")).toString(); }
    // Why the grammar service did not check, if it did not.
    QString grammarError() const { return m_grammarError; }
    // Puts a replacement where the grammar service found something;
    // false where the place has a tag in it.
    bool replaceFound(int offset, int length, const QString &value);
    // The last thing said to the person (a notice or a conflict).
    QString message() const { return m_lastMessage; }

    // The editor's actions, as the web editor's keys run them.
    void confirm();
    void copySource();
    void insertResult(int n);
    void insertNextTag();
    void move(int step);
    void openCapture();
    void openLookup(const QString &text = QString());
    void goToSegment(const QString &id);
    // Saves the active segment if it changed. Done before leaving.
    void saveActive();

    // Comments on the open segment: a remark for the others working on
    // the project. Its translation and version stay as they are.
    void addComment(const QString &text);
    void deleteComment(const QString &commentId);
    QJsonArray comments() const;
    // Scored review: an issue a reviewer marks on the open segment
    // (category and severity as the server names them), and its removal.
    void addIssue(const QString &category, const QString &severity, const QString &note);
    void deleteIssue(const QString &issueId);
    QJsonArray reviewIssues() const;
    // The score of the open file in its language, as the server gives it;
    // empty until it is known.
    QJsonObject quality() const { return m_quality; }
    // Writes every issue of the project as a CSV table to a file.
    void saveReviewTo(const QString &path);

    // Asks a translation service for a suggestion for the open segment,
    // which is sent to the service; the answer is shown to be inserted.
    void askService(const QString &name);
    // Reads the list of services again.
    void reloadServices() { loadServices(); }
    void insertSuggestion();
    QStringList services() const { return m_services; }
    QJsonObject suggestion() const { return m_suggestion; }

    // Find and replace in the translations of the open file.
    Web::Dialog *showReplaceDialog();
    // Asks where the open segment is split; the caret in its source is the place.
    Web::Dialog *showSplitDialog();
    // Splits the open segment where before, the start of its source,
    // ends ("split"), or joins it with the one after it ("join"). failed
    // gets why not; without it that is shown as a message.
    void resegment(const QString &how, const QJsonArray &before = {}, std::function<void(const QString &)> failed = {});
    // Replaces every place of the text; says what it did when it is done.
    void replaceAll(const QString &find, const QString &with, bool matchCase, bool wholeWords);

signals:
    // The analysis window shows its report.
    void analysisShown();
    void back();
    void openProject();
    void openReview(const QString &termbase);
    // Another file or language of the project.
    void openFile(const QString &file, const QString &lang);
    void saved(const QString &id);
    // A translation service answered, or could not.
    void suggested(bool ok);
    // The comments of a segment changed here.
    void commented(const QString &id);
    // The issues of a segment changed here, and the file's score is known again.
    void reviewed(const QString &id);
    void scored();
    // The review was written to a file, or could not be.
    void reviewSaved(bool ok);
    // A segment was split or two were joined ("split", "join"), or it
    // could not be done.
    void resegmented(const QString &how, bool ok);
    // Find and replace is done: how many places in how many segments.
    void replaced(int places, int segments);
    // The best memory match was put into the segment that just opened;
    // what says which, for those who do not see it.
    void bestInserted(const QString &what);
    void checked();
    // The grammar service has answered, or failed to.
    void grammarChecked();
    void resultsShown();

protected:
    bool eventFilter(QObject *watched, QEvent *event) override;
    void resizeEvent(QResizeEvent *event) override;

private:
    QWidget *buildBar();
    QWidget *buildPanel();
    void loadProject();
    void load();
    void applyFilter();
    void activate(int row);
    QWidget *makeCellWidget(QWidget *parent, const QModelIndex &index);
    int activeHeight(int column, int width) const;
    void resizeActiveRow();
    void lookup();
    void recognize();
    void runChecks();
    void showResults();
    void showIssues();
    void showComments();
    void showSuggestions();
    void loadServices();
    void takeComments(const QJsonObject &segment);
    void refreshComments();
    void showReviewIssues();
    void loadQuality();
    void showCapture(const QString &source, const QString &target);
    void searchMemory();
    void save(const QString &id, const QJsonArray &target, const QString &status, const QJsonArray &warnings,
              const QJsonObject &origin, std::function<void(bool)> done = {});
    void refreshFromServer();
    void showConflict(const QString &id, const QJsonObject &theirs);
    void reloadPresence();
    void sendPresence();
    void updateProgress();
    void updatePreview();
    void updateReview();
    void notify(const QString &text);
    void addWord(const QString &word);
    void ignoreIssue(const QString &text);
    Web::Dialog *showMemoriesDialog();
    Web::Dialog *showAnalysisDialog();
    void saveDocument();
    bool editable() const;
    bool dirty() const;
    QString projectPath() const;
    QJsonObject settings() const { return m_project.value(QLatin1String("settings")).toObject(); }
    QJsonArray memorySettings() const;

    QPointer<Connection> m_conn;
    qint64 m_id;
    QString m_file;
    // The paragraphs of the open document, for its preview, and the file
    // they were asked for.
    QJsonArray m_layout;
    QString m_layoutFile;
    bool m_layoutAsked = false;
    void loadLayout();
    QString m_lang;
    QJsonObject m_project;
    QString m_sourceLang;

    SegmentModel *m_model;
    SegmentFilter m_filter;
    QLineEdit *m_find = nullptr;
    QLabel *m_found = nullptr;
    QString m_activeId;
    QJsonArray m_loadedTarget;
    QJsonObject m_origin;
    bool m_insertBest = false;
    QJsonArray m_ignored;
    QJsonArray m_results;
    QJsonArray m_terms;
    QJsonArray m_issues;
    // The grammar service's findings, and the segment and text they are for.
    QJsonArray m_grammar;
    QString m_grammarId;
    QString m_grammarText;
    QString m_grammarError;
    int m_grammarSerial = 0;
    // A confirmation waits for the grammar service, or comes back from it.
    bool m_confirmWaits = false;
    bool m_confirmAsked = false;
    void checkGrammar(const QString &id, const QString &text, std::function<void()> then = {});
    int m_selected = 0;
    QHash<QString, QJsonObject> m_conflicts;
    QString m_conflictId;
    int m_lookupSerial = 0;
    int m_checkSerial = 0;
    int m_searchSerial = 0;
    QString m_lastMessage;

    QTimer m_checkTimer;
    QTimer m_saveTimer;
    QTimer m_presenceTimer;
    QTimer m_refreshTimer;
    QTimer m_searchTimer;
    QTimer m_rowsTimer;
    QTimer m_toastTimer;

    // Bar.
    QLabel *m_title;
    QComboBox *m_fileChoice;
    QLabel *m_fileName;
    QLabel *m_sourceChip;
    QComboBox *m_langChoice;
    QLabel *m_targetChip;
    QProgressBar *m_progressTrack;
    QLabel *m_progressText;
    QButtonGroup *m_show;
    QPushButton *m_warningsFilter;
    QPushButton *m_review;
    QPushButton *m_preview;
    QPushButton *m_save;
    QString m_reviewTermbase;

    // Segments, and the widgets on the active one.
    QTableView *m_grid;
    SegmentDelegate *m_delegate;
    QPointer<QTextBrowser> m_source;
    QPointer<TargetEditor> m_target;
    QPointer<QLabel> m_counter;
    QTextBrowser *m_previewPane;
    QWidget *m_previewBox;

    // Results panel.
    QScrollArea *m_panel;
    QFrame *m_conflict;
    QLabel *m_conflictText;
    QLabel *m_resultsCount;
    QVBoxLayout *m_resultsList;
    QLabel *m_checksCount;
    QVBoxLayout *m_issuesList;
    QVBoxLayout *m_suggestList = nullptr;
    QStringList m_services;
    QString m_asking;
    QString m_serviceProblem;
    QJsonObject m_suggestion;
    QLabel *m_commentsCount = nullptr;
    QVBoxLayout *m_commentsList = nullptr;
    QPlainTextEdit *m_commentText = nullptr;
    QPushButton *m_commentAdd = nullptr;
    QLabel *m_reviewCount = nullptr;
    QVBoxLayout *m_reviewList = nullptr;
    QComboBox *m_issueCategory = nullptr;
    QComboBox *m_issueSeverity = nullptr;
    QLineEdit *m_issueNote = nullptr;
    QPushButton *m_issueAdd = nullptr;
    QLabel *m_qualityScore = nullptr;
    QLabel *m_qualityText = nullptr;
    QJsonObject m_quality;
    int m_qualityAsked = 0;
    QWidget *m_lookupSection;
    QLineEdit *m_lookupField;
    QVBoxLayout *m_lookupList;
    QWidget *m_captureIntro;
    QWidget *m_captureForm;
    QLineEdit *m_captureSource;
    QLineEdit *m_captureTarget;
    QLineEdit *m_captureNote;
    QLabel *m_captureHelp;
    QPushButton *m_captureAdd;
    QPushButton *m_nextTag;
    QWidget *m_cutBox = nullptr;
    QPushButton *m_split = nullptr;
    QPushButton *m_join = nullptr;
    QLabel *m_cutNote = nullptr;
    // The file's segments as they are now take the place of the list; the
    // segment at the row is the open one.
    void takeSegments(const QJsonArray &segments, int row);
    void updateCut();
    QWidget *m_shortcuts;
    QLabel *m_toast;
};
