#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QUrlQuery>
#include <QPointer>
#include <QTimer>
#include <QWidget>

class Connection;
class LanguageCombo;
class QComboBox;
class QLabel;
class QPushButton;
class QTabWidget;
class QTableWidget;
class QTreeWidget;
class SettingsEditor;

// One project: its files and how far they are, its settings, its
// analysis, and on a team server the people who work on it.
class ProjectPage : public QWidget {
    Q_OBJECT
public:
    ProjectPage(Connection *connection, qint64 projectId, QWidget *parent = nullptr);

    qint64 projectId() const { return m_id; }
    Connection *connection() const { return m_conn; }
    QJsonObject project() const { return m_project; }

signals:
    void back();
    // The project was deleted.
    void gone();

private:
    QWidget *filesTab();
    QWidget *settingsTab();
    QWidget *analysisTab();
    QWidget *peopleTab();
    void reload();
    void showProject();
    void showProgress(const QJsonArray &progress);
    bool manages() const;
    void say(const QString &message);
    QString currentFile() const;
    QString currentLang() const;
    void saveTranslated();
    void saveReturnPackage();
    void addFiles();
    void removeFile();
    void pretranslate();
    void saveSettings();
    void saveTemplate();
    void runAnalysis();
    void saveAnalysis();
    void reloadPeople();
    void addGrant();
    void removeGrant();
    void addAssignment();
    void toggleDone();
    void removeAssignment();
    void deleteProject();
    void download(const QString &path, const QUrlQuery &query, const QString &what);

    QPointer<Connection> m_conn;
    qint64 m_id;
    QJsonObject m_project;
    QJsonObject m_access;
    QJsonArray m_analysis;
    QJsonArray m_people;
    QJsonArray m_assignments;
    QTimer m_reload;
    bool m_settingsDirty = false;

    QLabel *m_title;
    QLabel *m_meta;
    QLabel *m_message;
    QPushButton *m_delete;
    QTabWidget *m_tabs;
    // Files.
    QTreeWidget *m_files;
    QPushButton *m_saveTranslated;
    QPushButton *m_returnPackage;
    QPushButton *m_addFiles;
    QPushButton *m_removeFile;
    QPushButton *m_pretranslate;
    // Settings.
    SettingsEditor *m_editor;
    QPushButton *m_saveSettings;
    QPushButton *m_saveTemplate;
    // Analysis.
    QComboBox *m_analysisLang;
    QTableWidget *m_analysisTable;
    QPushButton *m_saveAnalysis;
    // People.
    QWidget *m_peopleTab = nullptr;
    QTableWidget *m_grants;
    QTableWidget *m_work;
};
