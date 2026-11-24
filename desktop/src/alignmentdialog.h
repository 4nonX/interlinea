#pragma once

#include "webui.h"

#include <QFrame>
#include <QHash>
#include <QJsonArray>
#include <QList>
#include <QPair>

class ApiClient;
class QCheckBox;
class QLabel;
class QPushButton;
class QScrollArea;

// A segment of one of the two texts of an alignment: its number and text
// in a box that is chosen with the mouse or the keyboard, and from which
// a line is pulled to a segment of the other text.
class AlignSegment : public QFrame {
    Q_OBJECT
public:
    AlignSegment(int side, int index, const QString &text, QWidget *parent);
    int side() const { return m_side; }
    int index() const { return m_index; }
    void setState(bool picked, bool alone);

signals:
    void chosen();
    // The mouse was let go over another place than this segment.
    void pulledTo(const QPoint &globalPos);

protected:
    void mousePressEvent(QMouseEvent *e) override;
    void mouseReleaseEvent(QMouseEvent *e) override;
    void keyPressEvent(QKeyEvent *e) override;

private:
    int m_side;
    int m_index;
    QString m_what;
    QString m_text;
};

// The two texts side by side, with the lines between them.
class AlignBoard : public QWidget {
    Q_OBJECT
public:
    explicit AlignBoard(QWidget *parent);
    QList<AlignSegment *> segments[2];
    // The lines, as (segment of the document, segment of the translation),
    // and those of pairs that need a look.
    QList<QPair<int, int>> lines;
    QList<QPair<int, int>> lookLines;
    // Where a line starts and ends on the board; it is drawn as a curve
    // that leaves and arrives level.
    QLineF place(const QPair<int, int> &line) const;
    // The line at a place on the board, or -1.
    int lineIndexAt(const QPointF &p) const { return lineAt(p); }

signals:
    void lineChosen(int source, int target);

protected:
    void paintEvent(QPaintEvent *e) override;
    void mousePressEvent(QMouseEvent *e) override;
    void mouseMoveEvent(QMouseEvent *e) override;
    void leaveEvent(QEvent *e) override;

private:
    int lineAt(const QPointF &p) const;
    int m_hover = -1;
};

// A document and its translation, done before, into units of a memory:
// the core pairs their sentences, and the pairs are checked here, as lines
// between the two texts, before they are added. The same dialog, with the
// same model, is in the web client (Alignment.tsx, align.ts).
class AlignmentDialog : public Web::Dialog {
    Q_OBJECT
public:
    enum Side { Source, Target };

    AlignmentDialog(ApiClient *api, const QString &memory, const QString &sourceLang, const QString &targetLang, QWidget *parent);

    // Chooses the file of a side, as its button does.
    void setFile(Side side, const QString &path);
    // Sends both files to be paired.
    void pair();

    int segmentCount(Side side) const { return int(m_segments[side].size()); }
    QString text(Side side, int index) const;
    // The lines, each "document segment|translation segment", sorted.
    QStringList lines() const;
    bool alone(Side side, int index) const;
    // What would be added: the units the lines make.
    int unitCount() const;
    QString status() const;
    AlignBoard *board() const { return m_board; }

    // A segment chosen, then one of the other text: the line between them
    // is drawn, or taken away if it is there.
    void choose(Side side, int index);
    // A line pulled from a segment to one of the other text is drawn.
    void pull(Side from, int index, int other);
    // A line chosen is taken away.
    void takeLine(int source, int target);
    // Brings the next segment without a line, or pair that needs a look, into view.
    void next();
    // Adds the units to the memory.
    void add();

signals:
    // The pairs are there to be checked.
    void paired();
    void added(int units);

private:
    struct Unit {
        QList<int> source;
        QList<int> target;
    };
    void render();
    void refresh();
    void fitHeight();
    QList<Unit> units() const;
    QList<Unit> lookUnits(const QList<Unit> &units) const;
    QList<int> lookPlaces() const;
    QJsonArray joined(Side side, const QList<int> &which) const;

    ApiClient *m_api;
    QString m_memory;
    QString m_sourceLang;
    QString m_targetLang;
    QString m_paths[2];
    bool m_busy = false;
    bool m_hasResult = false;
    QJsonArray m_segments[2];
    QList<QPair<int, int>> m_links;
    QHash<QString, int> m_scores;
    int m_pickedSide = -1;
    int m_picked = -1;
    bool m_overwrite = false;
    AlignBoard *m_board = nullptr;
    QScrollArea *m_scroll = nullptr;
    QLabel *m_status = nullptr;
    QPushButton *m_next = nullptr;
    QPushButton *m_other;
    QPushButton *m_cancel;
    QPushButton *m_go;
};
