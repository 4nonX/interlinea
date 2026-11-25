#pragma once

#include <QHash>
#include <QJsonArray>
#include <QPointer>
#include <QStringList>

#include "termfields.h"
#include "webui.h"

class Connection;
class ConnectionManager;
class QLineEdit;
class QPushButton;
class QVBoxLayout;
struct ApiResult;

// Makes a termbase in steps, as the web app's New termbase dialog. A master
// is a termbase of its own: its languages, and its fields, started from a
// ready-made structure or from another termbase's. A project termbase is
// based on masters, as a branch is on what it branches from: it has their
// languages and fields, may add fields of its own, and sends its new and
// changed entries to them for review, with the fields they use.
class NewTermbaseDialog : public Web::Dialog {
    Q_OBJECT
public:
    enum Kind { Master, Project };
    enum Step { Start, LanguagesStep, Fields, Review };

    // lists: the termbases of each place, by connection.
    NewTermbaseDialog(ConnectionManager *manager, const QHash<QString, QJsonArray> &lists, Kind kind, QWidget *parent);

    Step step() const { return m_steps.value(m_at, Start); }
    QList<Step> steps() const { return m_steps; }
    Web::Stepper *stepper() const { return m_stepper; }
    Kind kind() const { return m_kind; }
    void setKind(Kind kind);
    void setName(const QString &name);
    // An index into the ready places, this computer first.
    void setPlace(int index);
    void setBasedOn(const QStringList &masters);
    void setLanguages(const QStringList &tags);
    // "terms", "notes", "terminology", or "tb:" and a termbase's name.
    void chooseStart(const QString &id);
    QString start() const { return m_start; }
    QList<TermFields::Draft> fields() const { return m_fields; }
    void setFields(const QList<TermFields::Draft> &fields);
    QStringList inheritedLanguages() const { return m_inheritedLanguages; }
    QList<TermFields::Draft> inheritedFields() const { return m_inheritedFields; }
    // What keeps the step from being left, or nothing.
    QString blocking() const;
    QPushButton *nextButton() const { return m_next; }
    QPushButton *backButton() const { return m_back; }
    void next();
    void back();
    // Something is being read or made.
    bool busy() const { return m_pending > 0; }

signals:
    // The termbase was made: where, its name, and what to tell.
    void created(const QString &connectionId, const QString &name, const QString &message);
    // What the dialog shows is up to date.
    void ready();

private:
    QList<Connection *> places() const;
    Connection *place() const;
    QList<QJsonObject> masters() const;
    bool taken(const QString &name) const;
    void render();
    void showStart();
    void showLanguages();
    void showFields();
    void showReview();
    void updateButtons();
    void fitHeight();
    void loadInherited();
    void create();
    void request(class ApiReply *reply, std::function<void(const ApiResult &)> then);

    ConnectionManager *m_manager;
    QHash<QString, QJsonArray> m_lists;
    Kind m_kind;
    QList<Step> m_steps;
    int m_at = 0;
    QString m_name;
    int m_place = 0;
    QStringList m_based;
    QStringList m_langs;
    QString m_start = QStringLiteral("terms");
    QList<TermFields::Draft> m_fields;
    QStringList m_inheritedLanguages;
    QList<TermFields::Draft> m_inheritedFields;
    int m_pending = 0;
    int m_asked = 0;
    Web::Stepper *m_stepper;
    QVBoxLayout *m_content;
    QLabel *m_hint;
    QPushButton *m_back;
    QPushButton *m_next;
    QPointer<TermFields::FieldTable> m_table;
    class QScrollArea *m_scroll;
};
