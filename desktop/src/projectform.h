#pragma once

#include "apierror.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QObject>
#include <QPointer>
#include <QStringList>

#include <functional>

#include <QWidget>

class Connection;
class LanguageCombo;
class QDateTimeEdit;
class QLabel;
class QPushButton;
class QVBoxLayout;

// A date and time that may be left out, as the web's date field: none
// until one is set, and taken away again with the button beside it.
class DueEdit : public QWidget {
    Q_OBJECT
public:
    explicit DueEdit(const QString &name, QWidget *parent = nullptr);
    // The time in UTC as ISO 8601, empty for none.
    QString iso() const;
    void setIso(const QString &iso);

signals:
    void changed();

private:
    void show(bool set);

    QPushButton *m_set;
    QDateTimeEdit *m_edit;
    QPushButton *m_clear;
    bool m_has = false;
};

// The sections of a project's setup, shared by the New project page and
// the project's own page (the web's ProjectSections), so a choice made at
// setup is changed later in the same place and the same words. The form
// holds the settings being edited; each section shows a part of them and
// writes what the person changes back.
class ProjectForm : public QObject {
    Q_OBJECT
public:
    explicit ProjectForm(QObject *parent = nullptr);

    // Loads what the settings can refer to on a connection (its memories,
    // termbases, dictionaries and checks), then calls done with an error
    // message, empty when all went well.
    void load(Connection *c, std::function<void(const QString &error)> done);
    void setCatalog(const QJsonObject &catalog, const QJsonArray &dictionaries, const QJsonArray &qaKinds);
    QJsonObject catalog() const { return m_catalog; }

    // A new project chooses its own memories and where new terms go; an
    // existing one has them.
    void setSettings(const QJsonObject &settings, bool newProject);
    QJsonObject settings() const;
    bool isNew() const { return m_new; }
    // Changes one of the settings from outside a section and shows it.
    void set(const QString &key, const QJsonValue &value);
    void setReadOnly(bool readOnly);

    // Target languages offered as check boxes besides the chosen ones, and
    // those bilingual files need, which can't be unticked.
    void setSuggestedTargets(const QStringList &targets);
    // The language picked under the target languages and not added yet,
    // when it could be added; empty otherwise.
    QString pendingTarget() const;
    // Adds that language, as its Add button does.
    void addPendingTarget();
    void setFixedTargets(const QStringList &targets);

    // Shows what the server refused, by field, in the section it is in.
    // Returns the messages no section shown has a place for.
    QStringList showProblems(const QList<Problem> &problems);
    QStringList problemsAt(const QString &field) const;

    // The sections. Each is made once, in parent.
    QWidget *details(QWidget *parent);
    QWidget *targets(QWidget *parent);
    QWidget *memories(QWidget *parent);
    QWidget *terminology(QWidget *parent);
    QWidget *checks(QWidget *parent);
    QWidget *prepare(QWidget *parent);
    // The project's own rules for where sentences end.
    QWidget *segmentation(QWidget *parent);

    // What pre-translation would do with these settings, shown in the
    // preparation section.
    void setPrepareSummary(const QString &summary);

    // A template of a project's settings: everything but its name, files,
    // dates and project termbase, and without its own project memories.
    static QJsonObject templateOf(const QString &name, const QJsonObject &settings);

    // How many of the checks are on.
    int checksOn() const;
    int checksOf() const;

    // Memories made in the memories section and not on the connection yet,
    // as the core's tm.Info. A new project sends them with its request;
    // importNewMemories then fills those made from a TMX file. For an
    // existing project createNewMemories does both.
    QJsonArray newMemories() const;
    void importNewMemories(Connection *c, std::function<void(const QString &error)> done);
    void createNewMemories(Connection *c, std::function<void(const QString &error)> done);

signals:
    void changed();
    // The language picked to be added changed.
    void pendingChanged();

private:
    struct NewMemory {
        QString name;
        QString sourceLang;
        QString targetLang;
        QString tmxPath;
    };
    QStringList targetList() const;
    QString source() const;
    void touch();
    void fillDetails();
    void fillTargets();
    void addTarget(const QString &lang);
    void fillMemories();
    void fillTerminology();
    void fillChecks();
    void fillPrepare();
    void fillSegmentation();
    QLabel *problemLabel(const QString &field, QWidget *parent);
    void dropStaleMemories();
    void pickDictionaries();
    QList<QJsonObject> dictionariesFor(const QString &lang) const;
    QJsonObject memoryInfo(const QString &name) const;
    QString host() const;

    QPointer<Connection> m_conn;
    QJsonObject m_s;
    QJsonObject m_catalog;
    QJsonArray m_dicts;
    // The grammar services of the place, by name.
    QStringList m_grammar;
    QJsonArray m_kinds;
    bool m_new = true;
    // The checks were changed by hand: dictionaries are no longer picked.
    bool m_checksTouched = false;
    bool m_readOnly = false;
    QStringList m_suggested;
    QStringList m_fixed;
    QList<Problem> m_problems;
    QList<NewMemory> m_created;
    QString m_summary;
    // The language whose "New memory" form is open, if any.
    QString m_creatingFor;

    QPointer<QWidget> m_details;
    QPointer<QWidget> m_targets;
    QPointer<LanguageCombo> m_adding;
    QPointer<QWidget> m_memories;
    QPointer<QWidget> m_terminology;
    QPointer<QWidget> m_checks;
    QPointer<QWidget> m_prepare;
    QPointer<QWidget> m_segmentation;
    QList<QPair<QString, QPointer<QLabel>>> m_problemLabels;
};
