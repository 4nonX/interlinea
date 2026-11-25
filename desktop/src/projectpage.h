#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QPointer>
#include <QTimer>
#include <QUrlQuery>
#include <QWidget>

class Connection;
class ProjectForm;
class QLabel;
class QPushButton;
class QVBoxLayout;

namespace Web {
class Toast;
}

// One project, laid out as the web's project page: its details, files,
// languages, memories, terminology, checks and analysis and
// pre-translation in panels down the page, saved together; on a team
// server the work given out in it, who is working now and who may use it;
// then saving it as a template and deleting it.
class ProjectPage : public QWidget {
    Q_OBJECT
public:
    ProjectPage(Connection *connection, qint64 projectId, QWidget *parent = nullptr);

    qint64 projectId() const { return m_id; }
    Connection *connection() const { return m_conn; }
    QJsonObject project() const { return m_project; }
    ProjectForm *form() const { return m_form; }
    bool unsaved() const { return m_dirty; }
    QString lastNotice() const;

signals:
    void back();
    // The project was deleted.
    void gone();

    // Translate a file into a language; both empty for the first file.
    void openEditor(const QString &file, const QString &lang);

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    void leave();
    bool m_gone = false;
    void reload();
    void showProject();
    void fillFiles();
    void fillLanguages();
    void fillProjectMemories();
    void fillAnalysis();
    void fillPretranslate();
    void fillWork();
    void fillNow();
    void fillTemplate();
    void refreshBar();
    bool manages() const;
    void say(const QString &message);
    void addFiles(const QStringList &paths);
    void removeFile(const QString &name);
    void download(const QString &path, const QUrlQuery &query, const QString &what);
    void save();
    void putSettings(QJsonObject settings);
    void discard();
    void pretranslate();
    void runAnalysis();
    void saveAnalysis();
    void saveTemplate(const QString &name);
    void deleteProject();
    QJsonObject settings() const;

    QPointer<Connection> m_conn;
    qint64 m_id;
    QJsonObject m_project;
    QJsonObject m_access;
    QJsonArray m_progress;
    QJsonArray m_analysis;
    QString m_analysisLang;
    bool m_analysing = false;
    QJsonArray m_assignments;
    QJsonArray m_presence;
    QJsonArray m_people;
    QJsonArray m_templates;
    QTimer m_reload;
    bool m_dirty = false;
    bool m_saving = false;
    bool m_loaded = false;
    QString m_busy;
    QString m_saveError;
    QString m_workError;

    ProjectForm *m_form;
    Web::Toast *m_toast;
    QLabel *m_title;
    QLabel *m_where;
    QPushButton *m_open;
    QVBoxLayout *m_files = nullptr;
    QVBoxLayout *m_languages = nullptr;
    QVBoxLayout *m_projectMemories = nullptr;
    QVBoxLayout *m_analysisBody = nullptr;
    QVBoxLayout *m_pretranslate = nullptr;
    QVBoxLayout *m_work = nullptr;
    QVBoxLayout *m_now = nullptr;
    QVBoxLayout *m_template = nullptr;
    QVBoxLayout *m_access_ = nullptr;
    QWidget *m_workPanel = nullptr;
    QWidget *m_nowPanel = nullptr;
    QWidget *m_accessPanel = nullptr;
    QWidget *m_templatePanel = nullptr;
    QWidget *m_deletePanel = nullptr;
    QLabel *m_deleteHelp = nullptr;
    QWidget *m_bar;
    QLabel *m_barError;
    QPushButton *m_discard;
    QPushButton *m_save;
};
