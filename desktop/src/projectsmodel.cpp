#include "projectsmodel.h"

#include "apiclient.h"
#include "connectionmanager.h"
#include "eventfeed.h"
#include "languages.h"
#include "progressdelegate.h"

#include <QDateTime>
#include <QJsonArray>
#include <QLocale>

ProjectsModel::ProjectsModel(ConnectionManager *manager, QObject *parent)
    : QAbstractTableModel(parent), m_manager(manager)
{
    for (Connection *c : manager->all())
        watch(c);
    connect(manager, &ConnectionManager::added, this, &ProjectsModel::watch);
    connect(manager, &ConnectionManager::removed, this, [this](const QString &id) { replace(id, {}); });
}

void ProjectsModel::watch(Connection *c)
{
    // Changes come in bursts (a pre-translation); one reading follows.
    auto *timer = new QTimer(this);
    timer->setSingleShot(true);
    timer->setInterval(400);
    m_pending.insert(c->id(), timer);
    connect(timer, &QTimer::timeout, this, [this, c] { reload(c); });
    connect(c, &Connection::stateChanged, this, [this, c, timer] {
        if (c->isReady())
            timer->start();
        else
            replace(c->id(), {});
    });
    connect(c->feed(), &EventFeed::event, this, [timer](const QString &type) {
        if (type == QLatin1String("project") || type == QLatin1String("account"))
            timer->start();
    });
    connect(c->feed(), &EventFeed::reset, timer, qOverload<>(&QTimer::start));
    connect(c, &QObject::destroyed, this, [this, id = c->id()] { m_pending.remove(id); });
    if (c->isReady())
        timer->start();
}

void ProjectsModel::reload(Connection *c)
{
    if (!c->isReady())
        return;
    onResult(c->api()->get(QStringLiteral("/api/projects")), this, [this, id = c->id()](const ApiResult &r) {
        if (!r.ok()) {
            m_errors.insert(id, r.error.message);
            emit loaded();
            return;
        }
        m_errors.remove(id);
        QList<Row> rows;
        for (const QJsonValue &v : r.array()) {
            const QJsonObject o = v.toObject();
            rows.append({id, o, o.value(QLatin1String("access")).toObject()});
        }
        replace(id, rows);
    });
}

void ProjectsModel::replace(const QString &connectionId, const QList<Row> &rows)
{
    beginResetModel();
    m_rows.removeIf([&](const Row &r) { return r.connection == connectionId; });
    m_rows += rows;
    endResetModel();
    emit loaded();
}

int ProjectsModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : int(m_rows.size());
}

int ProjectsModel::columnCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : ColumnCount;
}

QVariant ProjectsModel::headerData(int section, Qt::Orientation orientation, int role) const
{
    if (orientation != Qt::Horizontal || role != Qt::DisplayRole)
        return {};
    switch (section) {
    case Name:
        return tr("Project");
    case Client:
        return tr("Client");
    case Languages:
        return tr("Languages");
    case Due:
        return tr("Due");
    case Progress:
        return tr("Confirmed");
    case Where:
        return tr("Kept on");
    }
    return {};
}

QVariant ProjectsModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() >= m_rows.size())
        return {};
    const Row &row = m_rows.at(index.row());
    const QJsonObject s = row.project.value(QLatin1String("settings")).toObject();
    switch (role) {
    case ConnectionRole:
        return row.connection;
    case IdRole:
        return row.project.value(QLatin1String("id")).toInteger();
    case ProjectRole:
        return row.project;
    case AccessRole:
        return row.access;
    default:
        break;
    }
    const int segments = row.project.value(QLatin1String("segments")).toInt();
    const int confirmed = row.project.value(QLatin1String("confirmed")).toInt();
    const QDateTime due = QDateTime::fromString(s.value(QLatin1String("due")).toString(), Qt::ISODate);
    if (role == SortRole) {
        switch (index.column()) {
        case Due:
            return due.isValid() ? due : QDateTime(QDate(9999, 1, 1), QTime());
        case Progress:
            return segments ? double(confirmed) / segments : 0.0;
        default:
            return data(index, Qt::DisplayRole).toString().toLower();
        }
    }
    if (role == Qt::DisplayRole || role == Qt::ToolTipRole) {
        switch (index.column()) {
        case Name:
            return s.value(QLatin1String("name")).toString();
        case Client:
            return s.value(QLatin1String("client")).toString();
        case Languages: {
            QStringList targets;
            for (const QJsonValue &t : s.value(QLatin1String("targetLangs")).toArray())
                targets << t.toString();
            const QString src = s.value(QLatin1String("sourceLang")).toString();
            if (role == Qt::ToolTipRole) {
                QStringList names;
                for (const QString &t : targets)
                    names << Languages::name(t);
                return tr("From %1 into %2").arg(Languages::name(src), QLocale().createSeparatedList(names));
            }
            return QStringLiteral("%1 → %2").arg(src, targets.join(QStringLiteral(", ")));
        }
        case Due:
            return due.isValid() ? Languages::when(due) : QString();
        case Progress:
            return segments ? QStringLiteral("%1 %").arg(qRound(100.0 * confirmed / segments)) : QString();
        case Where: {
            const Connection *c = m_manager->find(row.connection);
            return c ? c->displayName() : QString();
        }
        }
    }
    if (role == ProgressDelegate::ShareRole && index.column() == Progress)
        return segments ? double(confirmed) / segments : QVariant();
    if (role == Qt::AccessibleDescriptionRole && index.column() == Progress)
        return tr("%1 of %2 segments confirmed").arg(confirmed).arg(segments);
    return {};
}
