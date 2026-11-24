#include "segmentmodel.h"

#include "bands.h"
#include "icons.h"
#include "theme.h"

#include <QAbstractTextDocumentLayout>
#include <QApplication>
#include <QHeaderView>
#include <QPainter>
#include <QTableView>
#include <QJsonArray>
#include <QTextDocument>

SegmentModel::SegmentModel(QObject *parent)
    : QAbstractTableModel(parent)
{
}

int SegmentModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : m_segments.size();
}

int SegmentModel::columnCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : Columns;
}

QString SegmentModel::statusName(const QString &status)
{
    if (status == QLatin1String("confirmed"))
        return tr("Confirmed");
    if (status == QLatin1String("draft"))
        return tr("Draft");
    return tr("Not translated");
}

QVariant SegmentModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() >= m_segments.size())
        return {};
    const QJsonObject &s = m_segments[index.row()];
    const QString id = s.value(QLatin1String("id")).toString();
    const QJsonArray target = m_pending.contains(id) ? m_pending.value(id) : s.value(QLatin1String("target")).toArray();
    const QString status = s.value(QLatin1String("status")).toString();
    const bool locked = s.value(QLatin1String("locked")).toBool();
    const int warnings = s.value(QLatin1String("warnings")).toArray().size();
    switch (role) {
    case IdRole:
        return id;
    case StatusRole:
        return status;
    case LockedRole:
        return locked;
    case WarningsRole:
        return warnings;
    case SegmentRole:
        return s;
    case ActiveRole:
        return !m_active.isEmpty() && id == m_active;
    case PresenceRole:
        return m_presence.value(id);
    case HtmlRole:
        if (index.column() == Source)
            return SegmentContent::toHtml(s.value(QLatin1String("source")).toArray(), m_marks.value(id));
        if (index.column() == Target)
            return SegmentContent::toHtml(target);
        return {};
    case Qt::DisplayRole:
        switch (index.column()) {
        case Number:
            return index.row() + 1;
        case Source:
            return SegmentContent::plainText(s.value(QLatin1String("source")).toArray());
        case Target:
            return SegmentContent::plainText(target);
        case State: {
            QStringList parts{statusName(status)};
            if (locked)
                parts << tr("Locked");
            if (warnings)
                parts << tr("%n warning(s)", nullptr, warnings);
            if (m_pending.contains(id))
                parts << tr("Not saved");
            const QStringList who = m_presence.value(id);
            if (!who.isEmpty())
                parts << tr("Open by %1").arg(who.join(QLatin1String(", ")));
            return parts.join(QLatin1String(", "));
        }
        }
        return {};
    case Qt::AccessibleTextRole:
        if (index.column() == Target && target.isEmpty())
            return tr("Not translated");
        return data(index, Qt::DisplayRole);
    case Qt::ToolTipRole:
        if (index.column() == State) {
            QStringList lines;
            for (const QJsonValue &w : s.value(QLatin1String("warnings")).toArray())
                lines << w.toObject().value(QLatin1String("text")).toString();
            const QString by = s.value(QLatin1String("changedBy")).toString();
            if (!by.isEmpty())
                lines << tr("Last saved by %1").arg(by);
            return lines.join(QLatin1Char('\n'));
        }
        return {};
    case Qt::TextAlignmentRole:
        if (index.column() == Number)
            return int(Qt::AlignRight | Qt::AlignTop);
        return int(Qt::AlignLeft | Qt::AlignTop);
    }
    return {};
}

QVariant SegmentModel::headerData(int section, Qt::Orientation orientation, int role) const
{
    if (orientation != Qt::Horizontal || role != Qt::DisplayRole)
        return {};
    switch (section) {
    case Number:
        return tr("#");
    case Source:
        return tr("Source");
    case Target:
        return tr("Translation");
    case State:
        return tr("State");
    }
    return {};
}

void SegmentModel::setSegments(const QJsonArray &segments)
{
    beginResetModel();
    m_segments.clear();
    m_rows.clear();
    for (const QJsonValue &v : segments) {
        const QJsonObject s = v.toObject();
        m_rows.insert(s.value(QLatin1String("id")).toString(), m_segments.size());
        m_segments << s;
    }
    endResetModel();
}

bool SegmentModel::update(const QJsonObject &segment)
{
    const int row = rowOf(segment.value(QLatin1String("id")).toString());
    if (row < 0)
        return false;
    m_segments[row] = segment;
    changed(row);
    return true;
}

void SegmentModel::setMarks(const QString &id, const QVector<SegmentContent::Mark> &marks)
{
    m_marks.insert(id, marks);
    if (const int row = rowOf(id); row >= 0)
        changed(row);
}

void SegmentModel::setActive(const QString &id)
{
    const QString was = m_active;
    m_active = id;
    for (const QString &x : {was, id}) {
        if (const int row = rowOf(x); row >= 0)
            changed(row);
    }
}

void SegmentModel::setPresence(const QHash<QString, QStringList> &bySegment)
{
    QSet<QString> touched;
    for (auto it = m_presence.cbegin(); it != m_presence.cend(); ++it)
        touched << it.key();
    for (auto it = bySegment.cbegin(); it != bySegment.cend(); ++it)
        touched << it.key();
    m_presence = bySegment;
    for (const QString &id : touched) {
        if (const int row = rowOf(id); row >= 0)
            changed(row);
    }
}

void SegmentModel::setPending(const QString &id, const QJsonArray &target)
{
    m_pending.insert(id, target);
    if (const int row = rowOf(id); row >= 0)
        changed(row);
}

void SegmentModel::clearPending(const QString &id)
{
    if (m_pending.remove(id)) {
        if (const int row = rowOf(id); row >= 0)
            changed(row);
    }
}

void SegmentModel::changed(int row)
{
    emit dataChanged(index(row, 0), index(row, Columns - 1));
}

bool SegmentFilter::accepts(const SegmentModel *model, int row) const
{
    const QModelIndex i = model->index(row, 0);
    if (m_show == Show::All || i.data(SegmentModel::ActiveRole).toBool())
        return true;
    if (m_show == Show::ToDo)
        return i.data(SegmentModel::StatusRole).toString() != QLatin1String("confirmed");
    return i.data(SegmentModel::WarningsRole).toInt() > 0;
}

namespace {

constexpr int Pad = 12;

QFont segmentFont(const QFont &base)
{
    // .seg: 15 px, as the web editor's segments.
    QFont f = base;
    f.setPixelSize(15);
    return f;
}

void prepare(QTextDocument &doc, const QFont &font, const QString &html, int width)
{
    doc.setDefaultFont(segmentFont(font));
    doc.setDefaultStyleSheet(SegmentContent::css());
    doc.setDocumentMargin(0);
    doc.setHtml(html);
    doc.setTextWidth(qMax(10, width - 2 * Pad));
}

QString initials(const QString &name)
{
    QString out;
    for (const QString &part : name.split(QLatin1Char(' '), Qt::SkipEmptyParts)) {
        out += part.at(0).toUpper();
        if (out.size() == 2)
            break;
    }
    return out;
}

} // namespace

void SegmentDelegate::paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const
{
    const bool active = index.data(SegmentModel::ActiveRole).toBool();
    const QRect r = option.rect;
    painter->save();
    painter->fillRect(r, active ? Theme::color("activeRow") : Theme::color("surface"));
    // Row lines, and the line between source and translation.
    painter->setPen(Theme::color("rowLine"));
    painter->drawLine(r.bottomLeft(), r.bottomRight());
    if (index.column() == SegmentModel::Source)
        painter->drawLine(r.topRight(), r.bottomRight());
    const QJsonObject s = index.data(SegmentModel::SegmentRole).toJsonObject();
    switch (index.column()) {
    case SegmentModel::Number: {
        if (active)
            painter->fillRect(QRect(r.left(), r.top(), 3, r.height()), Theme::color("accent"));
        QFont f = option.font;
        f.setPixelSize(12);
        if (active)
            f.setWeight(QFont::DemiBold);
        painter->setFont(f);
        painter->setPen(active ? Theme::color("accentText") : Theme::color("text3"));
        const QRect numRect(r.left(), r.top() + 15, r.width() - Pad, 18);
        painter->drawText(numRect, Qt::AlignRight | Qt::AlignTop, QString::number(index.row() + 1));
        // Who else is here, as the web's .seg-presence.
        int y = r.top() + 36;
        for (const QString &name : index.data(SegmentModel::PresenceRole).toStringList()) {
            const QRect badge(r.right() - Pad - 20, y, 20, 20);
            painter->setRenderHint(QPainter::Antialiasing);
            painter->setBrush(Theme::color("serverBg"));
            painter->setPen(Qt::NoPen);
            painter->drawEllipse(badge);
            QFont small = option.font;
            small.setPixelSize(9);
            small.setWeight(QFont::DemiBold);
            painter->setFont(small);
            painter->setPen(Theme::color("server"));
            painter->drawText(badge, Qt::AlignCenter, initials(name));
            y += 24;
        }
        break;
    }
    case SegmentModel::Source:
    case SegmentModel::Target: {
        QString html = index.data(SegmentModel::HtmlRole).toString();
        if (index.column() == SegmentModel::Target && html.isEmpty())
            html = QStringLiteral("<span class=\"missing\">%1</span>").arg(tr("Not translated"));
        QTextDocument doc;
        prepare(doc, option.font, html, r.width());
        painter->translate(r.left() + Pad, r.top() + Pad);
        QAbstractTextDocumentLayout::PaintContext ctx;
        ctx.palette = option.palette;
        ctx.palette.setColor(QPalette::Text, Theme::color("text"));
        doc.documentLayout()->draw(painter, ctx);
        break;
    }
    case SegmentModel::State: {
        // .seg-status: the status icon, then lock, warnings and the
        // match score, one under the other.
        const QString status = s.value(QLatin1String("status")).toString();
        int y = r.top() + 15;
        const int cx = r.center().x();
        QString icon = QStringLiteral("circle-dashed");
        QColor c = Theme::color("text3");
        if (status == QLatin1String("confirmed")) {
            icon = QStringLiteral("check");
            c = Theme::color("ok");
        } else if (status == QLatin1String("draft")) {
            icon = QStringLiteral("pencil-line");
            c = Theme::color("warn");
        }
        Icons::paint(*painter, QRectF(cx - 8, y, 16, 16), icon, c, status == QLatin1String("confirmed") ? 2.2 : 1.8);
        y += 18;
        if (s.value(QLatin1String("locked")).toBool()) {
            Icons::paint(*painter, QRectF(cx - 6.5, y, 13, 13), QStringLiteral("lock"), Theme::color("text3"));
            y += 15;
        }
        QFont small = option.font;
        small.setPixelSize(11);
        small.setWeight(QFont::DemiBold);
        painter->setFont(small);
        if (const int w = s.value(QLatin1String("warnings")).toArray().size(); w > 0) {
            Icons::paint(*painter, QRectF(cx - 10, y, 13, 13), QStringLiteral("alert-triangle"), Theme::color("warn"));
            painter->setPen(Theme::color("warn"));
            painter->drawText(QRect(cx + 4, y, 16, 13), Qt::AlignLeft | Qt::AlignVCenter, QString::number(w));
            y += 15;
        }
        const QJsonObject origin = s.value(QLatin1String("origin")).toObject();
        if (origin.value(QLatin1String("kind")).toString() == QLatin1String("tm")) {
            const int score = origin.value(QLatin1String("score")).toInt();
            const bool context = origin.value(QLatin1String("context")).toBool();
            const Bands::Colors bc = Bands::colors(Bands::ofScore(score, context));
            const QRect badge(cx - 18, y + 2, 36, 18);
            painter->setRenderHint(QPainter::Antialiasing);
            painter->setPen(Qt::NoPen);
            painter->setBrush(bc.background);
            painter->drawRoundedRect(badge, 4, 4);
            painter->setPen(bc.text);
            painter->drawText(badge, Qt::AlignCenter, Bands::scoreLabel(score, context));
        } else if (origin.value(QLatin1String("kind")).toString() == QLatin1String("repetition")) {
            Icons::paint(*painter, QRectF(cx - 6.5, y + 2, 13, 13), QStringLiteral("repeat"), Theme::color("text3"));
        }
        break;
    }
    }
    painter->restore();
}

QSize SegmentDelegate::sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const
{
    int width = 300;
    if (auto *view = qobject_cast<const QTableView *>(option.widget))
        width = view->horizontalHeader()->sectionSize(index.column());
    if (index.column() == SegmentModel::Number || index.column() == SegmentModel::State) {
        const int people = index.data(SegmentModel::PresenceRole).toStringList().size();
        return QSize(width, 15 + 18 + 15 + people * 24);
    }
    QString html = index.data(SegmentModel::HtmlRole).toString();
    if (html.isEmpty())
        html = QStringLiteral("x");
    QTextDocument doc;
    prepare(doc, option.font, html, width);
    int h = int(doc.size().height()) + 2 * Pad;
    if (index.data(SegmentModel::ActiveRole).toBool() && activeHeight)
        h = qMax(h, activeHeight(index.column(), width));
    return QSize(width, h);
}

QWidget *SegmentDelegate::createEditor(QWidget *parent, const QStyleOptionViewItem &, const QModelIndex &index) const
{
    return factory ? factory(parent, index) : nullptr;
}

void SegmentDelegate::updateEditorGeometry(QWidget *editor, const QStyleOptionViewItem &option, const QModelIndex &index) const
{
    // The source's widget sits in the cell's padding; the translation's
    // fills the cell, as the web's .target-editor.
    QRect r = option.rect.adjusted(0, 0, 0, -1);
    if (index.column() == SegmentModel::Source)
        r = r.adjusted(Pad, Pad, -Pad - 1, -Pad);
    editor->setGeometry(r);
}

bool SegmentDelegate::eventFilter(QObject *, QEvent *)
{
    // The editor's own keys stay with it: Enter, Tab and Escape are not
    // the view's to take.
    return false;
}
