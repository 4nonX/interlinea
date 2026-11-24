#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QPointer>
#include <QTimer>
#include <QWidget>

#include "webui.h"

class Connection;
class QProgressBar;
class QVBoxLayout;

// A merge request, laid out as the web app's review page: a project
// termbase's changes to a master, decided one by one or many at once by the
// master's reviewers.
class ReviewPage : public QWidget {
    Q_OBJECT
public:
    explicit ReviewPage(QWidget *parent = nullptr);

    // Shows a merge request of a place.
    void open(Connection *conn, int id);
    int request() const { return m_id; }
    QJsonObject mergeRequest() const { return m_request; }
    QJsonObject progress() const { return m_progress; }
    QJsonArray items() const { return m_items; }
    // 0 for those waiting, 1 for those decided, 2 for all.
    int view() const { return m_view; }
    void setView(int view);
    void reload();
    void notify(const QString &text);
    QString message() const;

signals:
    // The request is read and shown.
    void loaded();
    // Back to the termbase the request came from.
    void back(const QString &connectionId, const QString &termbase);

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    void show();
    QWidget *item(const QJsonObject &it, QWidget *parent);
    void decide(const QJsonObject &filter, const QString &state, const QJsonObject &edited, const QString &what);

    QPointer<Connection> m_conn;
    int m_id = 0;
    int m_view = 0;
    int m_reading = 0;
    QJsonObject m_request;
    QJsonObject m_progress;
    QJsonArray m_items;
    int m_total = 0;
    QString m_problem;
    bool m_read = false;
    QTimer m_reload;
    QVBoxLayout *m_body;
    Web::Toast *m_toast;
};
