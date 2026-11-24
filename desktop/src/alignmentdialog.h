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
    bool picked() const { return property("picked").toBool(); }
    bool alone() const { return property("alone").toBool(); }

signals:
    void chosen();
    // The mouse was let go over another place than this segment.
    void pulledTo(const QPoint &globalPos);

protected:
    void mousePressEvent(QMouseEvent *e) override;
    void mouseReleaseEvent(QMouseEvent *e) override;
    void keyPressEvent(QKeyEvent *e) override;
    void enterEvent(QEnterEvent *e) override;
    void leaveEvent(QEvent *e) override;

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
    // The lines someone drew or confirmed, drawn solid; the others, paired
    // for them, are dotted.
    QList<QPair<int, int>> mineLines;
    // Where a line starts and ends on the board; it is drawn as a curve
    // that leaves and arrives level.
    QLineF place(const QPair<int, int> &line) const;
    // The line at a place on the board, or -1.
    int lineIndexAt(const QPointF &p) const { return lineAt(p); }

    // Where a segment's lines begin: a point in the gap beside it, drawn
    // as a ring while nothing is joined there.
    QPointF port(int side, int index) const;
    // The segment whose point or box is at a place, or null.
    AlignSegment *segmentAt(const QPoint &p) const;

signals:
    void lineChosen(int source, int target);
    // A point was pressed and let go on itself, or over another segment.
    void portChosen(int side, int index);
    void portPulled(int side, int index, int otherSide, int otherIndex);

protected:
    void paintEvent(QPaintEvent *e) override;
    void mousePressEvent(QMouseEvent *e) override;
    void mouseReleaseEvent(QMouseEvent *e) override;
    void mouseMoveEvent(QMouseEvent *e) override;
    void leaveEvent(QEvent *e) override;

private:
    int lineAt(const QPointF &p) const;
    AlignSegment *portAt(const QPointF &p) const;
    int m_hover = -1;
    AlignSegment *m_hoverPort = nullptr;
    AlignSegment *m_pressed = nullptr;
};

// A document and its translation, done before, into units of a memory:
// the core pairs their sentences, and the pairs are checked here, as lines
// between the two texts, before they are added. The same dialog, with the
// same model, is in the web client (Alignment.tsx, align.ts).
class AlignmentDialog : public Web::Dialog {
    Q_OBJECT
public:
    enum Side { Source, Target };

    // Into one memory.
    AlignmentDialog(ApiClient *api, const QString &memory, const QString &sourceLang, const QString &targetLang, QWidget *parent);
    // Into a memory chosen here, of those given ({name, sourceLang,
    // targetLang}), or one made for the documents; taken are the names
    // that are there already.
    AlignmentDialog(ApiClient *api, const QJsonArray &memories, const QStringList &taken, QWidget *parent);

    // The memory the units go into: one of those given, or none for a new one.
    void chooseMemory(const QString &name);
    void setNewMemory(const QString &name, const QString &sourceLang, const QString &targetLang);
    QString memory() const;
    // Why the new memory can't be made as it is, or nothing.
    QString destinationProblem() const;

    // Chooses the file of a side, as its button does.
    void setFile(Side side, const QString &path);
    // Sends both files to be paired.
    void pair();
    // Makes the lines of the chosen segment's unit the translator's own.
    void confirmPicked();
    void confirmAll();
    // Goes back to the lines before the last change, and forth again.
    void undo();
    void redo();
    // Closing, or choosing other documents, with work on the lines asks
    // first: the question is shown, and answered with stay() or leave().
    QString leaveQuestion() const;
    void stay();
    void leave();
    void reject() override;
    bool canUndo() const { return !m_past.isEmpty(); }
    bool canRedo() const { return !m_future.isEmpty(); }
    // Pairs everything again around the units that are the translator's own.
    void pairAgain();
    // The lines that are the translator's own, as lines() gives them.
    QStringList mineLines() const;
    int checkedCount() const { return int(fixedUnits().size()); }

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
    void added(int units, const QString &memory);

private:
    struct Unit {
        QList<int> source;
        QList<int> target;
    };
    // The lines and what they were paired from, to go back to.
    struct Snap {
        QList<QPair<int, int>> links;
        QList<QPair<int, int>> mine;
        QHash<QString, int> scores;
    };
    void init();
    void askToLeave(bool close);
    void otherDocuments();
    void remember();
    void restore(const Snap &snap);
    void sendUnits();
    void updateStart();
    void render();
    void refresh();
    void request(const QJsonArray &fixed);
    void draw(const QPair<int, int> &line);
    void own(Side side, int index);
    QList<Unit> fixedUnits() const;
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
    // Chosen here: the memories to choose from, the names taken, the one
    // chosen (none for a new one), the new one's name, and whether it is made.
    bool m_choice = false;
    QJsonArray m_memories;
    QStringList m_taken;
    QString m_newName;
    bool m_new = false;
    bool m_made = false;
    QList<Snap> m_past;
    QList<Snap> m_future;
    QPushButton *m_undo = nullptr;
    QPushButton *m_redo = nullptr;
    QPushButton *m_confirmAll = nullptr;
    QLabel *m_startProblem = nullptr;
    // 0: not asked; 1: asked about closing; 2: about other documents.
    int m_leaving = 0;
    QWidget *m_leaveRow = nullptr;
    QLabel *m_leaveText = nullptr;
    QPushButton *m_leaveGo = nullptr;
    bool m_busy = false;
    bool m_hasResult = false;
    QJsonArray m_segments[2];
    QList<QPair<int, int>> m_links;
    QList<QPair<int, int>> m_mine;
    QHash<QString, int> m_scores;
    int m_pickedSide = -1;
    int m_picked = -1;
    bool m_overwrite = false;
    AlignBoard *m_board = nullptr;
    QScrollArea *m_scroll = nullptr;
    QLabel *m_status = nullptr;
    QPushButton *m_next = nullptr;
    QPushButton *m_confirm = nullptr;
    QPushButton *m_again = nullptr;
    QPushButton *m_other;
    QPushButton *m_cancel;
    QPushButton *m_go;
};
