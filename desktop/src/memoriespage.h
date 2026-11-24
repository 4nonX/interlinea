#pragma once

#include <QHash>
#include <QJsonArray>
#include <QPointer>
#include <QTimer>
#include <QWidget>

class Connection;
class ConnectionManager;
class QVBoxLayout;

// Translation memories, laid out as the web app's Memories page: the main
// and project memories on this computer, and those on each team server,
// with TMX in and out, search, new memories and who may use them.
class MemoriesPage : public QWidget {
    Q_OBJECT
public:
    explicit MemoriesPage(ConnectionManager *manager, QWidget *parent = nullptr);

    // Reads everything again (for tests and after a change).
    void reload();
    // The memory opened on a team server, for its search and access.
    QString openMemory(const QString &connectionId) const { return m_open.value(connectionId); }
    void open(const QString &connectionId, const QString &name);
    void notify(const QString &text);
    QString message() const { return m_lastMessage; }

signals:
    void loaded();

private:
    void build();
    QWidget *localSection(Connection *c, const QJsonArray &memories, const QJsonArray &projects, const QJsonArray &templates);
    QWidget *serverSection(Connection *c, const QJsonArray &memories);
    QWidget *memoryTable(Connection *c, const QJsonArray &memories, const QJsonArray &projects, const QJsonArray &templates, bool main);
    QWidget *newMemoryForm(Connection *c, bool server);
    QWidget *serverDetail(Connection *c, const QJsonObject &memory);
    void importTmx(Connection *c, const QString &name);
    void exportTmx(Connection *c, const QString &name);

    ConnectionManager *m_manager;
    QVBoxLayout *m_body;
    QVBoxLayout *m_sections;
    QHash<QString, QString> m_open;
    QHash<QString, QJsonArray> m_memories;
    QHash<QString, QJsonArray> m_projects;
    QHash<QString, QJsonArray> m_templates;
    QTimer m_reload;
    QString m_lastMessage;
    class QLabel *m_toast;
    QTimer m_toastTimer;
};
