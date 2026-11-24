#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QList>
#include <QPointer>
#include <QStringList>

#include "webui.h"

class Connection;
class ConnectionManager;
class QButtonGroup;
class QCheckBox;
class QComboBox;
class QLineEdit;
class QPushButton;
class QVBoxLayout;

// The formats a termbase is written in, as the web app lists them.
struct TermFormat {
    QString id;
    QString label;
    QString ext;
    QString note;
};
QList<TermFormat> termExportFormats();
// "MultiTerm XML" for "multiterm".
QString termFormatLabel(const QString &id);
// The file dialog's filter for termbase files to import.
QString termImportFilter();

// Export a termbase, as the web app's dialog: the format, and all entries
// or those the search shows.
class ExportDialog : public Web::Dialog {
    Q_OBJECT
public:
    ExportDialog(Connection *conn, const QString &termbase, int total, const QStringList &shown, QWidget *parent);
    QString format() const;
    void setFormat(const QString &id);
    bool shownOnly() const { return m_shownOnly; }
    void setShownOnly(bool only);
    // The file name offered: the termbase's name with the format's ending.
    QString fileName() const;
    QPushButton *exportButton() const { return m_export; }
    // Writes the export to a file; what the button does once one is chosen.
    void exportTo(const QString &path);

signals:
    void exported(const QString &message);

private:
    void update();

    QPointer<Connection> m_conn;
    QString m_termbase;
    int m_total;
    QStringList m_shown;
    bool m_shownOnly = false;
    QButtonGroup *m_formats;
    QButtonGroup *m_scope = nullptr;
    QPushButton *m_export;
};

// A termbase's settings, as the web app's dialog: its name, languages and
// fields, and deleting it.
class TermbaseSettingsDialog : public Web::Dialog {
    Q_OBJECT
public:
    TermbaseSettingsDialog(Connection *conn, const QJsonObject &detail, const QList<QJsonObject> &entries, QWidget *parent);
    QString termbase() const { return m_name; }
    // Something is being read or saved.
    bool busy() const { return m_pending > 0; }

signals:
    // A notice for the page.
    void said(const QString &text);
    // The termbase changed and is to be read again under this name; an
    // empty name says it was deleted.
    void changed(const QString &name);
    // What the dialog shows is up to date.
    void shown();

private:
    void refresh();
    void render();
    void rename();
    void putDefinition(const QJsonObject &definition, const QString &done);
    void putRules(const QJsonObject &rules, const QString &done);
    void reviewSection(QWidget *host);
    void basedOnSection(QWidget *host);
    int termsIn(const QString &lang) const;
    int valuesOf(const QString &field) const;
    void request(class ApiReply *reply, std::function<void(const struct ApiResult &)> then);

    QPointer<Connection> m_conn;
    QString m_name;
    QJsonObject m_detail;
    QList<QJsonObject> m_entries;
    QStringList m_usedBy;
    // The masters of the place, for what a project termbase is based on.
    QStringList m_masters;
    // How merge requests into a master are reviewed, and whom to trust.
    QJsonObject m_rules;
    QJsonArray m_people;
    QString m_trustAuthor;
    QString m_trustLang;
    class AccessPanel *m_access = nullptr;
    int m_pending = 0;
    QLineEdit *m_nameEdit;
    QVBoxLayout *m_content;
};

// The import wizard, as the web app's: choose a file, say what the columns
// of a spreadsheet hold, map its languages, and see what the import does
// before it is done.
class ImportDialog : public Web::Dialog {
    Q_OBJECT
public:
    enum Step { File, Columns, LanguagesStep, Review };

    // With a termbase, the import goes into it unless a new one is chosen;
    // without, it makes a new master.
    ImportDialog(ConnectionManager *manager, const QString &connectionId, const QString &termbase, QWidget *parent);

    Step step() const { return m_step; }
    Web::Stepper *stepper() const { return m_stepper; }
    void setIntoNew(bool intoNew);
    bool intoNew() const { return m_intoNew; }
    void setNewName(const QString &name);
    // Which place a new termbase is made in: an index into the ready
    // connections, this computer first.
    void setPlace(int index);
    // Reads the file; what the button does once one is chosen.
    void chooseFile(const QString &path);
    QJsonObject mapping() const { return m_mapping; }
    void setMapping(const QJsonObject &mapping);
    // The file's languages and where each goes ("" to leave it out).
    QStringList fileLanguages() const { return m_fileLangs; }
    QString mappedTo(const QString &fileLang) const { return m_langMap.value(fileLang).toString(); }
    void mapLanguage(const QString &fileLang, const QString &to);
    void setFallbackStatus(const QString &status);
    void setPolicy(const QString &match, const QString &lang, const QString &onMatch);
    // The report of the dry run shown in the review step.
    QJsonObject plan() const { return m_plan; }
    QPushButton *nextButton() const { return m_next; }
    QPushButton *backButton() const { return m_back; }
    void next();
    void back();
    // Something is being read.
    bool busy() const { return m_pending > 0; }

signals:
    // The import is done: where it went and what to tell.
    void imported(const QString &connectionId, const QString &termbase, const QString &summary);
    // The step shown is up to date.
    void ready();

private:
    Connection *place() const;
    QList<Connection *> places() const;
    QStringList destinationLanguages() const;
    void render();
    void showFile();
    void showColumns();
    void showLanguages();
    void showReview();
    void inspect(bool withMapping, std::function<void(const QJsonObject &)> then);
    void readLanguages();
    void loadPlan();
    void doImport();
    QUrlQuery options(bool withLanguages) const;
    void request(class ApiReply *reply, std::function<void(const struct ApiResult &)> then);

    ConnectionManager *m_manager;
    QString m_connId;
    QString m_termbase;
    QJsonObject m_destination;
    int m_destinationEntries = 0;
    bool m_intoNew;
    QString m_newName;
    int m_place = 0;
    Step m_step = File;
    QString m_fileName;
    QByteArray m_data;
    QJsonObject m_inspection;
    QJsonObject m_mapping;
    QJsonObject m_read;
    QStringList m_fileLangs;
    QJsonObject m_langMap;
    QString m_fallback = QStringLiteral("approved");
    QString m_match = QStringLiteral("term");
    QString m_matchLang;
    QString m_onMatch = QStringLiteral("merge");
    QJsonObject m_plan;
    int m_pending = 0;
    int m_asked = 0;
    Web::Stepper *m_stepper;
    QVBoxLayout *m_content;
    QPushButton *m_back;
    QPushButton *m_next;
};
