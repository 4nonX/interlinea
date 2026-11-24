#pragma once

#include <QJsonObject>
#include <QWidget>

class ConnectionManager;
class EditorPage;
class ProjectPage;
class ProjectsModel;
class QLayout;
class QStackedWidget;
class QVBoxLayout;

// The projects of every place work is kept, and one project opened.
class ProjectsPage : public QWidget {
    Q_OBJECT
public:
    explicit ProjectsPage(ConnectionManager *manager, QWidget *parent = nullptr);

    ProjectsModel *model() const { return m_model; }
    ProjectPage *openProject() const { return m_project; }
    EditorPage *openEditor() const { return m_editor; }
    void edit(const QString &file, const QString &lang);
    void open(const QString &connectionId, qint64 projectId);
    void newProject(const QStringList &files = {});
    void showList();

signals:
    // Go to another page of the window ("terminology", …), at an item.
    void navigate(const QString &page, const QString &item);

private:
    void updateState();
    static QStringList expand(const QStringList &paths);
    static void clearLayout(QLayout *layout);
    void openInEditor(const QString &connectionId, qint64 projectId, const QJsonObject &settings);

    ConnectionManager *m_manager;
    ProjectsModel *m_model;
    QStackedWidget *m_stack;
    QVBoxLayout *m_sections;
    ProjectPage *m_project = nullptr;
    EditorPage *m_editor = nullptr;
    void closeEditor();
};
