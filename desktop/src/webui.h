#pragma once

#include <QFrame>
#include <QScrollArea>
#include <QJsonArray>
#include <QStringList>
#include <QTableWidget>

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

private:
    QList<int> m_numeric;
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
