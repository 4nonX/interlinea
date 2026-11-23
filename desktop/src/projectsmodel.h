#pragma once

#include <QAbstractTableModel>
#include <QHash>
#include <QJsonObject>
#include <QTimer>

class Connection;
class ConnectionManager;

// The projects of every connection signed in, kept current by their live
// feeds.
class ProjectsModel : public QAbstractTableModel {
    Q_OBJECT
public:
    enum Column { Name, Client, Languages, Due, Progress, Where, ColumnCount };
    enum Role { ConnectionRole = Qt::UserRole + 1, IdRole, ProjectRole, AccessRole, SortRole };

    explicit ProjectsModel(ConnectionManager *manager, QObject *parent = nullptr);

    int rowCount(const QModelIndex &parent = {}) const override;
    int columnCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role = Qt::DisplayRole) const override;
    QVariant headerData(int section, Qt::Orientation orientation, int role = Qt::DisplayRole) const override;

    // Reads a connection's projects again.
    void reload(Connection *c);
    // The error of the last reading of a connection, if any.
    QString error(const QString &connectionId) const { return m_errors.value(connectionId); }

signals:
    void loaded();

private:
    struct Row {
        QString connection;
        QJsonObject project; // the summary, with segments and confirmed
        QJsonObject access;
    };
    void watch(Connection *c);
    void replace(const QString &connectionId, const QList<Row> &rows);

    ConnectionManager *m_manager;
    QList<Row> m_rows;
    QHash<QString, QString> m_errors;
    QHash<QString, QTimer *> m_pending;
};
