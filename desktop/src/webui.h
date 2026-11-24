#pragma once

#include <QDialog>
#include <QFrame>
#include <QScrollArea>
#include <QJsonArray>
#include <QLabel>
#include <QStringList>
#include <QTableWidget>
#include <QTimer>

#include <functional>

class QAbstractButton;
class QBoxLayout;
class QLabel;
class QPushButton;
class QScrollArea;
class QVBoxLayout;

// The web app's page parts (styles.css, components/ui.tsx), so every page
// of the desktop app is laid out and spaced as the same page on the web.
namespace Web {

// A page as .main > .page: scrolling, 24 px from the top and 32 px from the
// sides, at most maxWidth wide (1120, or 760 for .page-narrow), with 24 px
// between its parts. Add the parts to body.
struct Page {
    QScrollArea *scroll = nullptr;
    QVBoxLayout *body = nullptr;
};
Page page(QWidget *parent, int maxWidth = 1120);

// .page-header: the title (22 px, 650), its description, and actions on the
// right.
QWidget *header(const QString &title, const QString &description, const QList<QWidget *> &actions, QWidget *parent);
// .section: a 16 px title and the parts below it, 12 px apart.
QWidget *section(const QString &title, QWidget *parent, QVBoxLayout **body);
// .panel: a white box with a 10 px radius, 24 px in, and its 18 px title.
QFrame *panel(const QString &title, QWidget *parent, QVBoxLayout **body);
QLabel *text(const QString &text, const char *role, QWidget *parent);

// Buttons as .btn, .btn-primary, .btn-quiet, .btn-small.
enum ButtonKind { Normal = 0, Primary = 1, Quiet = 2, Small = 4 };
QPushButton *button(const QString &text, int kind, QWidget *parent, const QString &icon = QString());

// A language code chip with its name as the tooltip and for screen readers.
QLabel *langChip(const QString &code, QWidget *parent);
// "de-DE → en-GB en-US".
QWidget *langPair(const QString &source, const QStringList &targets, QWidget *parent);
// .badge-server or .badge (local), with its icon.
QWidget *locationBadge(bool server, QWidget *parent, const QString &host = QString());
// .tag and .status chips: kind "accent", "conflict", "changed", "new",
// "approved", "proposed", "forbidden", or "" for grey.
QLabel *chip(const QString &text, const QString &kind, QWidget *parent);

// A table as .table-wrap .table: grey header row, 10 by 12 px cells with a
// line under each, as tall as its rows (the page scrolls, not the table).
class Table : public QTableWidget {
    Q_OBJECT
public:
    Table(const QStringList &headers, QWidget *parent);
    // Right-aligned number columns (.num).
    void setNumeric(const QList<int> &columns);
    void setStretch(int column);
    // Adds a row of widgets (or plain text for a null widget); returns it.
    int addRow(const QList<QWidget *> &cells);
    void clearRows();
    // Fits its height to the rows.
    void fit();
    QSize sizeHint() const override;
    QSize minimumSizeHint() const override;

protected:
    void resizeEvent(QResizeEvent *e) override;
    void showEvent(QShowEvent *e) override;

private:
    void fitRows();
    QSize inset() const;

    QList<int> m_numeric;
    bool m_refit = false;
};

// A cell's title and grey line under it (.cell-title, .cell-sub). With
// onClick the title is a link, as .cell-link.
QWidget *cell(const QString &title, const QString &sub, QWidget *parent, std::function<void()> onClick = {});
QWidget *cellText(const QString &text, QWidget *parent, bool numeric = false, bool muted = false);
// Buttons in a cell, in a row (.cell-action .row).
QWidget *cellActions(const QList<QWidget *> &buttons, QWidget *parent);

// .choice: a radio button or check box with its title and description,
// framed, blue when chosen.
QFrame *choice(QAbstractButton *input, const QString &description, QWidget *parent);

// .field: a label (with "optional" after it), the control, and help.
QWidget *field(const QString &label, QWidget *control, QWidget *parent, bool optional = false, const QString &help = QString());

// .row: widgets side by side, 8 px apart, kept to the left.
QWidget *row(const QList<QWidget *> &widgets, QWidget *parent, int spacing = 8);
// A control under its small grey label (.field-label), as in .role-add.
QWidget *labelled(const QString &label, QWidget *control, QWidget *parent);

// The web's ConfirmButton: a quiet button that asks before it acts. Pressed,
// it shows the question in red with the confirming button and Cancel.
class ConfirmButton : public QWidget {
    Q_OBJECT
public:
    ConfirmButton(const QString &label, const QString &question, const QString &action, QWidget *parent, bool small = true,
                  const QString &icon = QString());
    void setQuestion(const QString &question);
    // Greyed out, as a disabled button.
    void setAvailable(bool available);
    // Whether the step takes something away: then its button is red.
    // Otherwise it is the page's main button.
    void setDanger(bool danger);
    bool asking() const;
    QPushButton *button() const { return m_button; }
    QPushButton *yes() const { return m_yes; }

signals:
    void confirmed();

private:
    void ask(bool asking);

    QPushButton *m_button;
    QLabel *m_text;
    QPushButton *m_yes;
    QPushButton *m_cancel;
};

// .toast: a notice over the bottom of a page, gone after a while. Keep it
// in place with place() from the page's resizeEvent.
class Toast : public QLabel {
    Q_OBJECT
public:
    explicit Toast(QWidget *page);
    void say(const QString &text);
    void place();
    QString last() const { return m_last; }

private:
    QString m_last;
    QTimer *m_timer;
};

// A box that is clicked as a link is (.tb-link, .entry-row, .dup-entry):
// focusable, Enter and Space press it, and "current" marks the one shown.
class Clickable : public QFrame {
    Q_OBJECT
public:
    Clickable(const char *role, QWidget *parent);
    void setCurrent(bool current);
    bool isCurrent() const { return property("current").toBool(); }
signals:
    void clicked();

protected:
    void mouseReleaseEvent(QMouseEvent *e) override;
    void keyPressEvent(QKeyEvent *e) override;
};

// .tabs: tabs over a line, the chosen one underlined in blue. small is
// .tabs-small.
class Tabs : public QFrame {
    Q_OBJECT
public:
    Tabs(const QString &label, QWidget *parent, bool small = false);
    QPushButton *add(const QString &text, const QString &icon = QString());
    void setText(int index, const QString &text);
    void setEnabledAt(int index, bool enabled);
    void setVisibleAt(int index, bool visible);
    int current() const { return m_current; }
    void setCurrent(int index);
    QPushButton *tab(int index) const { return m_tabs.value(index); }
signals:
    void changed(int index);

private:
    QBoxLayout *m_row;
    QList<QPushButton *> m_tabs;
    int m_current = 0;
    bool m_small;
};

// .segmented: a few choices side by side, one of them chosen.
class Segmented : public QFrame {
    Q_OBJECT
public:
    Segmented(const QString &label, const QStringList &choices, QWidget *parent);
    int current() const { return m_current; }
    void setCurrent(int index);
    QPushButton *segment(int index) const { return m_segments.value(index); }
signals:
    void changed(int index);

private:
    QList<QPushButton *> m_segments;
    int m_current = 0;
};

// .stepper: the numbered steps of a wizard.
class Stepper : public QWidget {
    Q_OBJECT
public:
    Stepper(QWidget *parent);
    void setSteps(const QStringList &names, int current);
    QStringList steps() const { return m_names; }
    int current() const { return m_current; }

private:
    QBoxLayout *m_row;
    QStringList m_names;
    int m_current = 0;
};

// Two states of one part, such as a button and the form it opens: only
// the one shown takes room.
class Switcher : public QWidget {
    Q_OBJECT
public:
    explicit Switcher(QWidget *parent);
    // Adds a state; the first added is shown.
    void add(QWidget *state);
    int current() const { return m_current; }
    void setCurrent(int index);
signals:
    void changed(int index);

private:
    QList<QWidget *> m_states;
    int m_current = 0;
};

// .detect: a note with an icon, kind "warn", "ok" or "info".
QFrame *note(const QString &text, const QString &kind, QWidget *parent);

// .dialog: a window over the page with its title (17 px, 600) and 24 px
// around its parts, which are 12 px apart; 640 wide, or 920 (.dialog-wide).
// Its buttons sit in a row at the bottom right.
class Dialog : public QDialog {
    Q_OBJECT
public:
    Dialog(const QString &title, QWidget *parent, bool wide = false);
    QVBoxLayout *body() const { return m_body; }
    void setTitle(const QString &title);
    QString title() const;
    // The row of buttons at the bottom; add them left to right.
    QBoxLayout *buttons();
    // A red line over the buttons, hidden while empty.
    void setError(const QString &text);
    QString error() const;

private:
    QLabel *m_title;
    QVBoxLayout *m_body;
    QVBoxLayout *m_outer;
    QLabel *m_error;
    QBoxLayout *m_buttons = nullptr;
};

// .dropzone: a dashed box that takes dropped files and folders.
class DropZone : public QFrame {
    Q_OBJECT
public:
    explicit DropZone(QWidget *parent);
signals:
    void dropped(const QStringList &paths);

protected:
    void dragEnterEvent(QDragEnterEvent *e) override;
    void dragLeaveEvent(QDragLeaveEvent *e) override;
    void dropEvent(QDropEvent *e) override;
};

} // namespace Web
