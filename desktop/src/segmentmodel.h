#pragma once

#include "segmentcontent.h"

#include <QAbstractTableModel>
#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QStyledItemDelegate>

#include <functional>

// A file's segments in one target language, as the editor's grid shows
// them: number, source, target, and the state (status, warnings, who else
// is there).
class SegmentModel : public QAbstractTableModel {
    Q_OBJECT
public:
    enum Column { Number, Source, Target, State, Columns };
    enum Role {
        IdRole = Qt::UserRole + 1,
        HtmlRole,
        StatusRole,
        LockedRole,
        WarningsRole,
        SegmentRole,
        ActiveRole,
        PresenceRole,
    };

    explicit SegmentModel(QObject *parent = nullptr);

    int rowCount(const QModelIndex &parent = {}) const override;
    int columnCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QVariant headerData(int section, Qt::Orientation orientation, int role) const override;

    void setSegments(const QJsonArray &segments);
    // Replaces one segment with a newer copy; false when it isn't there.
    bool update(const QJsonObject &segment);
    QJsonObject segment(int row) const { return m_segments.value(row); }
    int rowOf(const QString &id) const { return m_rows.value(id, -1); }
    QVector<QJsonObject> segments() const { return m_segments; }
    // Underlines in a segment's source (terms found).
    void setMarks(const QString &id, const QVector<SegmentContent::Mark> &marks);
    // The segment being translated: its row is drawn as the web's
    // .seg.is-active.
    void setActive(const QString &id);
    QString active() const { return m_active; }
    // Who else has which segment open.
    void setPresence(const QHash<QString, QStringList> &bySegment);
    // A translation being typed, shown in the grid until it is saved.
    void setPending(const QString &id, const QJsonArray &target);
    void clearPending(const QString &id);

    static QString statusName(const QString &status);

private:
    void changed(int row);

    QVector<QJsonObject> m_segments;
    QHash<QString, int> m_rows;
    QHash<QString, QVector<SegmentContent::Mark>> m_marks;
    QHash<QString, QStringList> m_presence;
    QHash<QString, QJsonArray> m_pending;
    QString m_active;
};

// Which segments the editor shows: all, those still to do, or those with
// warnings. The active segment always shows.
class SegmentFilter {
public:
    enum class Show { All, ToDo, Warnings };
    void setShow(Show show) { m_show = show; }
    Show show() const { return m_show; }
    // Text the segments shown must hold, in the source or the translation.
    void setText(const QString &text);
    bool searching() const { return !m_text.isEmpty(); }
    // Whether the segment holds the text sought.
    bool holds(const SegmentModel *model, int row) const;
    bool accepts(const SegmentModel *model, int row) const;

private:
    Show m_show = Show::All;
    QString m_text;
};

// Draws a segment row as the web editor's (.seg): the number, source and
// translation with their tags and marks, and the status icons. The active
// row's source and translation are widgets the editor opens on it; the
// factory makes them.
class SegmentDelegate : public QStyledItemDelegate {
    Q_OBJECT
public:
    using QStyledItemDelegate::QStyledItemDelegate;
    std::function<QWidget *(QWidget *parent, const QModelIndex &index)> factory;
    // Whether a column's text is in a language written from right to left.
    std::function<bool(int column)> rightToLeft;
    // The height the active row's widgets need, by column.
    std::function<int(int column, int width)> activeHeight;
    void paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const override;
    QSize sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const override;
    QWidget *createEditor(QWidget *parent, const QStyleOptionViewItem &option, const QModelIndex &index) const override;
    void setEditorData(QWidget *, const QModelIndex &) const override {}
    void setModelData(QWidget *, QAbstractItemModel *, const QModelIndex &) const override {}
    void updateEditorGeometry(QWidget *editor, const QStyleOptionViewItem &option, const QModelIndex &) const override;
    bool eventFilter(QObject *object, QEvent *event) override;
};
