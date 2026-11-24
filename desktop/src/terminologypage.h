#pragma once

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QList>
#include <QPointer>
#include <QSet>
#include <QTimer>
#include <QWidget>

#include <functional>

#include "webui.h"

class Connection;
class ConnectionManager;
class EntryListModel;
class ExportDialog;
class ImportDialog;
class QComboBox;
class QLabel;
class QLineEdit;
class QListView;
class QStackedWidget;
class QVBoxLayout;
class TermbaseSettingsDialog;

// Terminology, laid out as the web app's page: the termbases on the left
// (masters, project termbases, those on each team server), the entries of
// the one opened in the middle with search, duplicates, import and export,
// and the chosen entry on the right with its fields, terms and history.
// Everything is read from and saved to the core of the place the termbase
// lives in.
class TerminologyPage : public QWidget {
    Q_OBJECT
public:
    explicit TerminologyPage(ConnectionManager *manager, QWidget *parent = nullptr);

    // Reads the lists and the opened termbase again.
    void reload();
    // Opens a termbase of a place, and an entry of it.
    void open(const QString &connectionId, const QString &termbase, const QString &entryId = QString());
    QString connectionId() const { return m_conn; }
    QString termbase() const { return m_name; }
    QString entryId() const { return m_selected; }
    // The opened termbase as the core describes it (definition, kind, role,
    // upstreams).
    QJsonObject detail() const { return m_detail; }
    // Whether the role on the opened termbase includes a permission
    // (read, suggest, edit, approve, manage).
    bool may(const char *permission) const;
    QStringList languages() const;
    QJsonArray fields() const;
    QList<QJsonObject> entries() const { return m_entries; }
    QList<QJsonObject> shown() const;
    QJsonObject entry(const QString &id) const;
    void selectEntry(const QString &id);
    void setQuery(const QString &text);
    void setStatus(const QString &status);
    // 0 for the entries, 1 for the duplicates, 2 for the deleted entries,
    // 3 for the master's entries under a project termbase.
    void setView(int view);
    int view() const { return m_view; }
    // The master entries a project termbase shows, and those left out.
    QList<QJsonObject> masterEntries() const { return m_masterEntries; }
    QJsonArray removals() const { return m_removals; }
    QJsonArray deleted() const { return m_deleted; }
    void selectMasterEntry(const QString &master, const QString &id);
    // The opened termbase is read and shown, and nothing of it is being
    // read again.
    bool ready() const { return m_opened && !m_loading; }
    // Changes waiting to be saved or being saved.
    bool busy() const { return m_saving || !m_queue.isEmpty(); }

    ImportDialog *importDialog(bool intoNew);
    ExportDialog *exportDialog();
    TermbaseSettingsDialog *settingsDialog();

    void notify(const QString &text);
    QString message() const;

signals:
    // The lists and the opened termbase are read and shown.
    void loaded();
    // Every change made is saved.
    void saved();
    // The duplicates were looked up and are listed.
    void duplicatesShown();
    // The entry's history was read and is listed.
    void historyShown();
    // The deleted entries, or the master's entries, are read and listed.
    void deletedShown();
    void masterShown();
    // A merge request of a team server is to be shown.
    void openReview(const QString &connectionId, int mergeRequest);

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    struct Change {
        QString entry;
        QString summary;
        std::function<QJsonObject(QJsonObject)> apply;
        bool rebuild;
    };

    Connection *conn() const;
    QString path(const QString &rest = QString()) const;
    bool isTeam() const;
    bool isProject() const;

    void listen();
    void loadLists();
    void showLists();
    void pickDefault();
    void loadTermbase();
    void loadEntries(int offset, int reading);
    void showTermbase();
    void showHead();
    void showReviewNote();
    void showEntries();
    void showDuplicates();
    void showDeleted();
    void showMaster();
    void showMasterDetail();
    void showDetail();
    void showHistory(QVBoxLayout *into);
    void showCreateForm(bool open);
    // What the page shows of the opened termbase, to tell whether reading
    // it again brought anything new.
    QByteArray signature() const;
    void readEntry(const QJsonObject &entry, QVBoxLayout *body, QWidget *inner);
    void showNewEntry(bool open);

    QWidget *fieldRow(const QJsonObject &field, const QString &value, std::function<void(const QString &)> save, bool small, QWidget *parent);
    QWidget *termItem(const QString &lang, int index, QWidget *parent);
    QWidget *addTerm(const QString &lang, QWidget *parent);
    QWidget *addField(QWidget *parent);
    QWidget *languageBlock(const QString &lang, QWidget *parent);

    void change(const QString &entryId, const QString &summary, std::function<QJsonObject(QJsonObject)> apply, bool rebuild = true);
    void sendNext();
    void replaceEntry(const QJsonObject &stored);
    void addLanguageToDefinition(const QString &lang);
    void putDefinition(const QJsonObject &definition, const QString &done);
    void failed(const QString &what, const QString &why);

    static void clear(QLayout *layout);

    ConnectionManager *m_manager;
    QSet<QString> m_listening;
    QTimer m_reload;
    int m_reading = 0;
    int m_pendingLists = 0;

    // What each place has: connection ID to its termbases.
    QHash<QString, QJsonArray> m_lists;
    QString m_conn;
    QString m_name;
    QString m_wantEntry;
    QJsonObject m_detail;
    QList<QJsonObject> m_entries;
    QList<QJsonObject> m_pendingEntries;
    QHash<QString, QString> m_bases;
    QList<QJsonObject> m_masterEntries;
    QJsonArray m_removals;
    QJsonArray m_deleted;
    QString m_masterSelected;
    QJsonArray m_requests;
    QString m_selected;
    QString m_query;
    QString m_status;
    int m_view = 0;
    int m_detailTab = 0;
    QString m_dupLang;
    QSet<QString> m_dismissed;
    bool m_opened = false;
    QString m_problem;

    QList<Change> m_queue;
    bool m_saving = false;

    QVBoxLayout *m_listBody;
    QVBoxLayout *m_createHolder;
    QVBoxLayout *m_middle;
    QVBoxLayout *m_headHolder;
    QVBoxLayout *m_noteHolder;
    Web::Tabs *m_tabs;
    QStackedWidget *m_views;
    QLineEdit *m_search;
    QComboBox *m_statusBox;
    QVBoxLayout *m_newEntryHolder;
    QStackedWidget *m_listStack;
    QListView *m_list;
    EntryListModel *m_model;
    QVBoxLayout *m_emptyHolder;
    QVBoxLayout *m_dupBody;
    QVBoxLayout *m_deletedBody;
    QVBoxLayout *m_masterBody;
    QVBoxLayout *m_detailHolder;
    Web::Toast *m_toast;
    QString m_newMasterName;
    QStringList m_newMasterLangs;
    int m_newMasterPlace = 0;
    // The form makes a project termbase, over these masters.
    bool m_newProject = false;
    bool m_createOpen = false;
    bool m_loading = false;
    QByteArray m_drawn;
    QByteArray m_listsDrawn;
    QStringList m_newUpstreams;
};

class ReviewPage;
class QStackedWidget;

// Terminology as the window shows it: the termbases, and over them the
// merge request opened from one, as the web app's review page takes the
// place of its terminology page.
class TerminologySection : public QWidget {
    Q_OBJECT
public:
    explicit TerminologySection(ConnectionManager *manager, QWidget *parent = nullptr);
    TerminologyPage *termbases() const { return m_termbases; }
    ReviewPage *review() const { return m_review; }
    bool reviewing() const;
    // Opens a termbase of a place.
    void open(const QString &connectionId, const QString &termbase);
    void openReview(const QString &connectionId, int mergeRequest);

private:
    ConnectionManager *m_manager;
    QStackedWidget *m_stack;
    TerminologyPage *m_termbases;
    ReviewPage *m_review;
};
