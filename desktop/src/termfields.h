#pragma once

#include <QJsonObject>
#include <QList>
#include <QPlainTextEdit>
#include <QStringList>
#include <QWidget>

#include <functional>

#include "webui.h"

class QAbstractButton;
class QComboBox;
class QLabel;
class QLineEdit;

// The fields of a termbase as the web app makes and edits them
// (TermbaseFields.tsx): the kinds of value a field takes, where it belongs,
// the ready-made structures a termbase starts from, the table of fields of
// a termbase being made, and the form that adds one field.
namespace TermFields {

struct Kind {
    QString id;
    QString name;
};
// Text, a list of values, a number, a date, yes or no.
QList<Kind> kinds();
QString kindName(const QString &id);

struct Level {
    // As the core keeps it: entry, language or term.
    QString id;
    QString name;
    QString hint;
};
QList<Level> levels();
QString levelName(const QString &id);

// A field as it is being made.
struct Draft {
    QString name;
    QString level = QStringLiteral("entry");
    QString kind = QStringLiteral("text");
    QStringList values;
};
QJsonObject toJson(const Draft &f);
Draft fromJson(const QJsonObject &f);
// "Gender (term, a list of values: masculine, feminine)".
QString describe(const Draft &f);
// What keeps a field from being saved, or nothing.
QString problem(const Draft &f, const QStringList &others);

struct Structure {
    QString id;
    QString name;
    QString text;
    QList<Draft> fields;
};
// From a plain glossary to terminology with grammar.
QList<Structure> structures();

// The kinds of value, to choose one; the item data is the kind's id.
QComboBox *kindCombo(const QString &kind, QWidget *parent);

// A radio button or check box with a text that wraps, bold up to the first
// full stop when strong is set (as <strong>A master.</strong> on the web).
// Clicking the text clicks the button.
QWidget *checkText(QAbstractButton *button, const QString &text, QWidget *parent, bool strong = false);

// The values of a list, one a line. changed() is sent as they are typed;
// leaving the box tidies them (no empty lines, none twice).
class ValuesEdit : public QPlainTextEdit {
    Q_OBJECT
public:
    ValuesEdit(const QStringList &values, QWidget *parent);
    QStringList values() const;
    void setValues(const QStringList &values);

signals:
    void changed(const QStringList &values);
    // Left with these values.
    void left(const QStringList &values);

protected:
    void focusOutEvent(QFocusEvent *e) override;

private:
    void fit();
};

// The fields of a termbase being made: each can be renamed, moved to
// another level, given another kind or taken out, and more added.
class FieldTable : public QWidget {
    Q_OBJECT
public:
    explicit FieldTable(QWidget *parent);
    void setFields(const QList<Draft> &fields);
    QList<Draft> fields() const { return m_fields; }
    // Names a field may not have: those it gets from elsewhere.
    void setTaken(const QStringList &names);
    // The first thing that keeps the fields from being saved.
    QString firstProblem() const;

signals:
    void changed();

private:
    void render();
    QString problemOf(int i) const;

    QList<Draft> m_fields;
    QStringList m_taken;
    QList<QLabel *> m_problems;
    QVBoxLayout *m_layout;
    bool m_focusLast = false;
};

// Adds one field to a termbase that has some already: its name, where it
// belongs and what it takes. Closed, it is a button.
class AddFieldForm : public Web::Switcher {
    Q_OBJECT
public:
    AddFieldForm(const QString &termbase, const QStringList &taken, QWidget *parent);

signals:
    // The field to add, as the core keeps it.
    void added(const QJsonObject &field);

private:
    void reset();
    void check();

    QString m_termbase;
    QStringList m_taken;
    Draft m_draft;
    QLineEdit *m_name;
    QLabel *m_hint;
    QList<QAbstractButton *> m_levels;
    QComboBox *m_kind;
    ValuesEdit *m_values;
    QWidget *m_valuesBox;
    QPushButton *m_add;
};

// Languages chosen from the list of all languages, as check boxes, with a
// search: nobody has to know which languages there are or how they are
// written.
class LanguageChecklist : public QWidget {
    Q_OBJECT
public:
    LanguageChecklist(const QString &label, QWidget *parent);
    QStringList chosen() const { return m_chosen; }
    void setChosen(const QStringList &tags);
    QLineEdit *search() const { return m_search; }
    // The languages the search shows.
    QStringList shown() const;

signals:
    void changed(const QStringList &chosen);

private:
    void showChips();
    void filter();

    QStringList m_chosen;
    QWidget *m_chips;
    QLineEdit *m_search;
    QLabel *m_none;
    QList<std::pair<QString, QWidget *>> m_items;
};

} // namespace TermFields
