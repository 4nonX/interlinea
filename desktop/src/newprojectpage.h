#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QTimer>
#include <QWidget>

class Connection;
class ConnectionManager;
class LanguageCombo;
class ProjectForm;
class QCheckBox;
class QLabel;
class QLineEdit;
class QPushButton;
class QVBoxLayout;

namespace Web {
class Stepper;
class Toast;
}

// Makes a project in five steps, as the web's New project page: where to
// start from, the files and languages, the memories, the terminology, and
// a review with the match analysis. The files are read by the core or the
// server the project is made on.
class NewProjectPage : public QWidget {
    Q_OBJECT
public:
    explicit NewProjectPage(ConnectionManager *manager, QWidget *parent = nullptr);

    // Documents to start with (dropped on the window, say).
    void addFiles(const QStringList &paths);
    // Keeps the project where this connection is, if it is ready.
    void setConnection(const QString &connectionId);

    int step() const { return m_step; }
    ProjectForm *form() const { return m_form; }
    Connection *connection() const;

signals:
    void cancelled();
    // The project was made.
    void created(const QString &connectionId, qint64 projectId);

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    struct Doc {
        QString path;
        QString name;
        QByteArray data;
        QJsonObject inspection;
        // translatable, reference or package
        QString role;
        bool read = false;
    };
    void setStep(int step);
    void showStep();
    void fillStart();
    void fillFiles();
    void fillSource();
    void fillReview();
    void fillAnalysis();
    void refreshContinue();
    void reloadWhere();
    void applyStart();
    void inspect(int index);
    void filesChanged();
    void chooseFiles();
    void chooseFolder();
    QStringList problems() const;
    QJsonObject request() const;
    void preview();
    void create();
    bool canTranslate(const Doc &d) const;
    QStringList bilingualSources() const;
    QStringList fixedTargets() const;

    ConnectionManager *m_manager;
    ProjectForm *m_form;
    QList<Doc> m_docs;
    QString m_connectionId;
    QJsonObject m_defaults;
    QJsonArray m_templates;
    QJsonArray m_projects;
    // blank, template or project, and which.
    QString m_startKind = QStringLiteral("blank");
    QString m_startId;
    int m_step = 0;
    // What the person set themselves is not replaced by a guess.
    bool m_sourceTouched = false;
    bool m_nameTouched = false;
    QString m_autoName;
    // The places a project can be kept, as last shown.
    QString m_places;
    bool m_loading = false;
    bool m_creating = false;
    bool m_checksOpen = false;
    bool m_asTemplate = false;
    QString m_templateName;
    // The review: what the project would be, per target language.
    QJsonArray m_preview;
    QString m_previewError;
    bool m_previewing = false;
    int m_previewSeq = 0;
    QString m_reviewLang;
    QTimer m_previewTimer;
    QString m_createError;

    Web::Stepper *m_stepper;
    Web::Toast *m_toast;
    QWidget *m_panels[5] = {};
    QVBoxLayout *m_startBody = nullptr;
    QVBoxLayout *m_filesBody = nullptr;
    QVBoxLayout *m_sourceBody = nullptr;
    QVBoxLayout *m_reviewBody = nullptr;
    QVBoxLayout *m_analysisBody = nullptr;
    QLabel *m_filesError = nullptr;
    QPushButton *m_continue[4] = {};
    QPushButton *m_create = nullptr;
};
