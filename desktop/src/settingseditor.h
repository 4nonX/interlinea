#pragma once

#include "apierror.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QTabWidget>

#include <functional>

class Connection;
class LanguageCombo;
class QCheckBox;
class QComboBox;
class QDateTimeEdit;
class QLabel;
class QLineEdit;
class QListWidget;
class QPlainTextEdit;
class QRadioButton;
class QSpinBox;
class QTableWidget;

// A project's settings: general details and languages, memories,
// terminology, quality checks and preparation. Used to set up a new
// project and to change one.
class SettingsEditor : public QTabWidget {
    Q_OBJECT
public:
    explicit SettingsEditor(QWidget *parent = nullptr);

    // What the settings can refer to on the connection: its memories,
    // termbases, dictionaries and checks.
    void setCatalog(const QJsonObject &catalog, const QJsonArray &dictionaries, const QJsonArray &qaKinds);
    // A new project chooses its source language, its own memories and its
    // project termbase; an existing one has them.
    void setSettings(const QJsonObject &settings, bool newProject);
    QJsonObject settings() const;
    void setReadOnly(bool readOnly);
    // Shows what the server refused, by field, on the tab it is on.
    void showProblems(const QList<Problem> &problems);

    // Loads the catalog of a connection, then calls done (with an error
    // message, empty when all went well).
    void load(Connection *c, std::function<void(const QString &error)> done);

signals:
    void changed();

private:
    QWidget *generalTab();
    QWidget *memoriesTab();
    QWidget *terminologyTab();
    QWidget *checksTab();
    QWidget *preparationTab();
    QStringList targets() const;
    void refreshTargets();
    void refreshMemories();
    void refreshTerminology();
    void refreshDictionaries();
    void addTarget();
    void addMemory();
    QJsonArray memoriesFor(const QString &lang) const;
    void storeMemories();

    QJsonObject m_settings;
    QJsonObject m_catalog;
    QJsonArray m_dicts;
    QJsonArray m_kinds;
    bool m_new = true;
    // Memory settings per target language, as edited.
    QJsonObject m_memories;

    // General.
    QLineEdit *m_name;
    QLineEdit *m_client;
    QPlainTextEdit *m_description;
    QCheckBox *m_hasDue;
    QDateTimeEdit *m_due;
    LanguageCombo *m_source;
    QListWidget *m_targets;
    LanguageCombo *m_newTarget;
    // Memories.
    QComboBox *m_memLang;
    QTableWidget *m_memTable;
    QComboBox *m_addMemory;
    QCheckBox *m_projectMemory;
    // Terminology.
    QRadioButton *m_own;
    QRadioButton *m_layered;
    QRadioButton *m_direct;
    QListWidget *m_masters;
    QComboBox *m_writeTo;
    QCheckBox *m_autoUpdate;
    QLabel *m_termbaseNote;
    // Checks.
    QListWidget *m_checks;
    QCheckBox *m_uneditedFuzzy;
    QTableWidget *m_dictTable;
    QPlainTextEdit *m_words;
    QTableWidget *m_rules;
    std::function<void(const QJsonObject &)> m_ruleRow;
    // Preparation.
    QCheckBox *m_pretranslate;
    QSpinBox *m_minScore;
    QCheckBox *m_confirmExact;
    QCheckBox *m_confirmContext;
    QComboBox *m_context;
    // Problems, per tab.
    QList<QLabel *> m_problemLabels;
};
