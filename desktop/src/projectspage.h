#pragma once

#include <QWidget>

class ConnectionManager;
class EditorPage;
class ProjectPage;
class ProjectsModel;
class QLabel;
class QLineEdit;
class QPushButton;
class QSortFilterProxyModel;
class QStackedWidget;
class QTableView;

// The projects of every place work is kept, and one project opened.
class ProjectsPage : public QWidget {
    Q_OBJECT
public:
    explicit ProjectsPage(ConnectionManager *manager, QWidget *parent = nullptr);

    ProjectsModel *model() const { return m_model; }
    QTableView *table() const { return m_table; }
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

    ConnectionManager *m_manager;
    ProjectsModel *m_model;
    QSortFilterProxyModel *m_proxy;
    QStackedWidget *m_stack;
    QTableView *m_table;
    QLineEdit *m_filter;
    QPushButton *m_open;
    QLabel *m_empty;
    ProjectPage *m_project = nullptr;
    EditorPage *m_editor = nullptr;
    void closeEditor();
};
