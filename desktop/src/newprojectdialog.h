#pragma once

#include <QDialog>
#include <QJsonArray>
#include <QJsonObject>

class Connection;
class ConnectionManager;
class LanguageCombo;
class QComboBox;
class QLabel;
class QPushButton;
class QStackedWidget;
class QTreeWidget;
class SettingsEditor;

// Makes a project from documents: they are read and their language
// guessed, then the project is set up (languages, memories, terminology,
// checks) and made, with its own memories and termbase.
class NewProjectDialog : public QDialog {
    Q_OBJECT
public:
    NewProjectDialog(ConnectionManager *manager, QWidget *parent = nullptr);

    // Documents to start with (dropped on the window, say).
    void addFiles(const QStringList &paths);

    // After accept(): where the project is and its number.
    QString connectionId() const { return m_connectionId; }
    qint64 projectId() const { return m_projectId; }

private:
    struct Doc {
        QString path;
        QString name;
        QByteArray data;
        QJsonObject inspection;
        QString role; // translatable, reference or package
    };
    Connection *connection() const;
    void refreshWhere();
    void chooseFiles();
    void chooseFolder();
    void inspect(int index);
    void refreshDocs();
    void guessLanguages();
    void next();
    void back();
    void create();
    void setBusy(bool busy, const QString &what = {});

    ConnectionManager *m_manager;
    QList<Doc> m_docs;
    QJsonObject m_defaults;
    QJsonArray m_templates;
    QString m_connectionId;
    qint64 m_projectId = 0;

    QStackedWidget *m_stack;
    QComboBox *m_where;
    QTreeWidget *m_files;
    LanguageCombo *m_source;
    QLabel *m_detected;
    QComboBox *m_template;
    QLabel *m_filesError;
    SettingsEditor *m_editor;
    QLabel *m_status;
    QPushButton *m_back;
    QPushButton *m_next;
};
