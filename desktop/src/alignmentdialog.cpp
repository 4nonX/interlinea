#include "alignmentdialog.h"

#include "apiclient.h"
#include "languages.h"
#include "segmentcontent.h"
#include "theme.h"

#include <QCheckBox>
#include <QFile>
#include <QFileDialog>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonObject>
#include <QKeyEvent>
#include <QLabel>
#include <QLocale>
#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <QPainterPathStroker>
#include <QPointer>
#include <QPushButton>
#include <QRegularExpression>
#include <QScrollArea>
#include <QScrollBar>
#include <QStyle>
#include <QTimer>
#include <QUrlQuery>
#include <QVBoxLayout>

namespace {

// The score below which a pair needs a look, the width between the two
// texts, and how near a click must be to a line to mean it.
constexpr int S2 = 8, S3 = 12, S5 = 24;
const int LookBelow = 50;
const int Gutter = 96;
const qreal Near = 7;
// The point beside a segment where its lines begin: its radius, and how
// far its middle is from the segment's edge.
const qreal PortRadius = 5;
const qreal PortOut = 6;

void empty(QLayout *l)
{
    while (QLayoutItem *it = l->takeAt(0)) {
        if (QWidget *w = it->widget()) {
            w->hide();
            w->setParent(nullptr);
            w->deleteLater();
        } else if (QLayout *inner = it->layout()) {
            empty(inner);
        }
        delete it;
    }
}

QString plural(int n, const QString &one, const QString &many)
{
    return QStringLiteral("%1 %2").arg(QLocale(QLocale::English).toString(n), n == 1 ? one : many);
}

// Whether a character is of a script written without spaces, or is a space.
bool spaceless(const QString &ch)
{
    static const QRegularExpression re(QStringLiteral("^[\\p{Han}\\p{Hiragana}\\p{Katakana}\\p{Thai}\\p{Lao}\\p{Khmer}\\p{Myanmar}\\s]"),
                                       QRegularExpression::UseUnicodePropertiesOption);
    return re.match(ch).hasMatch();
}

QString firstChar(const QString &s)
{
    return s.isEmpty() ? QString() : (s.at(0).isHighSurrogate() && s.size() > 1 ? s.left(2) : s.left(1));
}

QString lastChar(const QString &s)
{
    return s.isEmpty() ? QString() : (s.at(s.size() - 1).isLowSurrogate() && s.size() > 1 ? s.right(2) : s.right(1));
}

QString keyOf(const QList<int> &a, const QList<int> &b)
{
    QStringList x, y;
    for (int i : a)
        x << QString::number(i);
    for (int i : b)
        y << QString::number(i);
    return x.join(QLatin1Char(',')) + QLatin1Char('|') + y.join(QLatin1Char(','));
}

QList<int> ints(const QJsonArray &a)
{
    QList<int> out;
    for (const QJsonValue &v : a)
        out << v.toInt();
    return out;
}

// The line as it is drawn: a curve that leaves the sentence and reaches
// the translation level, so lines to one segment stay apart and each end
// shows whose it is.
QPainterPath curve(const QLineF &l)
{
    const qreal mid = (l.x1() + l.x2()) / 2;
    QPainterPath path(l.p1());
    path.cubicTo(QPointF(mid, l.y1()), QPointF(mid, l.y2()), l.p2());
    return path;
}

} // namespace

AlignSegment::AlignSegment(int side, int index, const QString &text, QWidget *parent)
    : QFrame(parent)
    , m_side(side)
    , m_index(index)
    , m_what(side == 0 ? tr("Sentence") : tr("Translation"))
    , m_text(text)
{
    setProperty("alignSeg", true);
    setObjectName(QStringLiteral("seg:%1:%2").arg(side == 0 ? QStringLiteral("source") : QStringLiteral("target")).arg(index));
    setFocusPolicy(Qt::StrongFocus);
    setCursor(Qt::PointingHandCursor);
    auto *h = new QHBoxLayout(this);
    h->setContentsMargins(S3, S2, S3, S2);
    h->setSpacing(S2);
    auto *n = new QLabel(QString::number(index + 1), this);
    Theme::role(n, "cellSub");
    n->setMinimumWidth(24);
    n->setAlignment(Qt::AlignLeft | Qt::AlignTop);
    auto *t = new QLabel(text, this);
    t->setTextFormat(Qt::PlainText);
    t->setWordWrap(true);
    t->setAlignment(Qt::AlignLeft | Qt::AlignTop);
    h->addWidget(n, 0, Qt::AlignTop);
    h->addWidget(t, 1);
    // The labels leave the mouse to the box.
    n->setAttribute(Qt::WA_TransparentForMouseEvents);
    t->setAttribute(Qt::WA_TransparentForMouseEvents);
    setState(false, false);
}

void AlignSegment::setState(bool picked, bool alone)
{
    setProperty("picked", picked);
    setProperty("alone", alone);
    setAccessibleName(QStringLiteral("%1 %2%3: %4").arg(m_what).arg(m_index + 1).arg(alone ? tr(", no pair") : QString(), m_text));
    setAccessibleDescription(picked ? tr("Chosen") : QString());
    style()->unpolish(this);
    style()->polish(this);
    update();
}

void AlignSegment::mousePressEvent(QMouseEvent *e)
{
    if (e->button() == Qt::LeftButton) {
        setFocus(Qt::MouseFocusReason);
        e->accept();
        return;
    }
    QFrame::mousePressEvent(e);
}

void AlignSegment::mouseReleaseEvent(QMouseEvent *e)
{
    if (e->button() != Qt::LeftButton) {
        QFrame::mouseReleaseEvent(e);
        return;
    }
    e->accept();
    if (rect().contains(e->position().toPoint()))
        emit chosen();
    else
        emit pulledTo(e->globalPosition().toPoint());
}

void AlignSegment::keyPressEvent(QKeyEvent *e)
{
    if (e->key() == Qt::Key_Space || e->key() == Qt::Key_Return || e->key() == Qt::Key_Enter) {
        e->accept();
        emit chosen();
        return;
    }
    QFrame::keyPressEvent(e);
}

// The point beside the segment, which the board draws, lights up with it.
void AlignSegment::enterEvent(QEnterEvent *e)
{
    QFrame::enterEvent(e);
    if (parentWidget())
        parentWidget()->update();
}

void AlignSegment::leaveEvent(QEvent *e)
{
    QFrame::leaveEvent(e);
    if (parentWidget())
        parentWidget()->update();
}

AlignBoard::AlignBoard(QWidget *parent)
    : QWidget(parent)
{
    setObjectName(QStringLiteral("alignBoard"));
    setMouseTracking(true);
    setAttribute(Qt::WA_StyledBackground, true);
}

QLineF AlignBoard::place(const QPair<int, int> &line) const
{
    const AlignSegment *a = segments[0].value(line.first);
    const AlignSegment *b = segments[1].value(line.second);
    if (!a || !b)
        return {};
    const QRect ra = a->geometry();
    const QRect rb = b->geometry();
    return QLineF(ra.right() + 1, ra.center().y(), rb.left(), rb.center().y());
}

QPointF AlignBoard::port(int side, int index) const
{
    const AlignSegment *seg = segments[side].value(index);
    if (!seg)
        return {};
    const QRect r = seg->geometry();
    return side == 0 ? QPointF(r.right() + 1 + PortOut, r.center().y()) : QPointF(r.left() - PortOut, r.center().y());
}

AlignSegment *AlignBoard::portAt(const QPointF &p) const
{
    for (int s = 0; s < 2; ++s) {
        for (AlignSegment *seg : segments[s]) {
            // A point is small: a little around it counts as well.
            if (QLineF(port(s, seg->index()), p).length() <= PortRadius + 3)
                return seg;
        }
    }
    return nullptr;
}

AlignSegment *AlignBoard::segmentAt(const QPoint &p) const
{
    if (AlignSegment *seg = portAt(p))
        return seg;
    QWidget *w = childAt(p);
    while (w && !qobject_cast<AlignSegment *>(w))
        w = w->parentWidget();
    return qobject_cast<AlignSegment *>(w);
}

int AlignBoard::lineAt(const QPointF &p) const
{
    // Of the lines near the place, the one whose curve is nearest: the
    // narrowest band around a curve that still holds the place.
    int best = -1;
    qreal bestWidth = 2 * Near + 1;
    for (int i = 0; i < lines.size(); ++i) {
        const QLineF l = place(lines.at(i));
        if (l.isNull() || p.x() < l.x1() - Near || p.x() > l.x2() + Near)
            continue;
        const QPainterPath path = curve(l);
        for (qreal w = 2; w <= 2 * Near && w < bestWidth; w += 2) {
            QPainterPathStroker band;
            band.setWidth(w);
            if (band.createStroke(path).contains(p)) {
                bestWidth = w;
                best = i;
                break;
            }
        }
    }
    return best;
}

void AlignBoard::paintEvent(QPaintEvent *e)
{
    QWidget::paintEvent(e);
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing);
    for (int i = 0; i < lines.size(); ++i) {
        const QLineF l = place(lines.at(i));
        if (l.isNull() || !e->rect().intersects(QRectF(l.p1(), l.p2()).normalized().adjusted(-3, -3, 3, 3).toRect()))
            continue;
        // Paired for the translator: dotted. Drawn or confirmed: solid.
        QPen pen(Theme::color("text2"), 1.5);
        if (!mineLines.contains(lines.at(i))) {
            const bool look = lookLines.contains(lines.at(i));
            pen = QPen(Theme::color(look ? "warn" : "text2"), look ? 2.5 : 2);
            pen.setCapStyle(Qt::RoundCap);
            pen.setDashPattern({0.05, 5 / pen.widthF()});
        }
        if (i == m_hover)
            pen = QPen(Theme::color("danger"), 2.5);
        p.setPen(pen);
        p.setBrush(Qt::NoBrush);
        p.drawPath(curve(l));
    }
    // The points, over the ends of the lines.
    for (int side = 0; side < 2; ++side) {
        for (const AlignSegment *seg : segments[side]) {
            const QPointF c = port(side, seg->index());
            if (!e->rect().adjusted(-8, -8, 8, 8).contains(c.toPoint()))
                continue;
            const bool lit = seg->picked() || seg == m_hoverPort || seg->underMouse();
            const QColor ink = lit ? Theme::color("accent") : seg->alone() ? Theme::color("warn") : Theme::color("text2");
            p.setPen(QPen(ink, 2));
            p.setBrush(seg->alone() && !lit ? Theme::color("surface") : ink);
            p.drawEllipse(c, PortRadius - 1, PortRadius - 1);
        }
    }
}

void AlignBoard::mousePressEvent(QMouseEvent *e)
{
    if (e->button() != Qt::LeftButton) {
        QWidget::mousePressEvent(e);
        return;
    }
    // A point comes before the line that ends in it.
    if (AlignSegment *seg = portAt(e->position())) {
        m_pressed = seg;
        seg->setFocus(Qt::MouseFocusReason);
        e->accept();
        return;
    }
    const int i = lineAt(e->position());
    if (i < 0) {
        QWidget::mousePressEvent(e);
        return;
    }
    e->accept();
    const QPair<int, int> l = lines.at(i);
    m_hover = -1;
    emit lineChosen(l.first, l.second);
}

void AlignBoard::mouseReleaseEvent(QMouseEvent *e)
{
    AlignSegment *from = m_pressed;
    m_pressed = nullptr;
    if (e->button() != Qt::LeftButton || !from) {
        QWidget::mouseReleaseEvent(e);
        return;
    }
    e->accept();
    AlignSegment *to = segmentAt(e->position().toPoint());
    if (to == from)
        emit portChosen(from->side(), from->index());
    else if (to && to->side() != from->side())
        emit portPulled(from->side(), from->index(), to->side(), to->index());
}

void AlignBoard::mouseMoveEvent(QMouseEvent *e)
{
    AlignSegment *seg = portAt(e->position());
    const int i = seg ? -1 : lineAt(e->position());
    if (i != m_hover || seg != m_hoverPort) {
        m_hover = i;
        m_hoverPort = seg;
        setCursor(seg ? Qt::CrossCursor : i >= 0 ? Qt::PointingHandCursor : Qt::ArrowCursor);
        setToolTip(seg ? tr("Pull from here to the segment that belongs to it") : i >= 0 ? tr("Take this line away") : QString());
        update();
    }
    QWidget::mouseMoveEvent(e);
}

void AlignBoard::leaveEvent(QEvent *e)
{
    if (m_hover >= 0 || m_hoverPort) {
        m_hover = -1;
        m_hoverPort = nullptr;
        update();
    }
    QWidget::leaveEvent(e);
}

AlignmentDialog::AlignmentDialog(ApiClient *api, const QString &memory, const QString &sourceLang, const QString &targetLang, QWidget *parent)
    : Web::Dialog(tr("Align documents into %1").arg(memory), parent, true)
    , m_api(api)
    , m_memory(memory)
    , m_sourceLang(sourceLang)
    , m_targetLang(targetLang)
{
    setObjectName(QStringLiteral("alignDocuments"));
    m_other = Web::button(tr("Other documents"), Web::Quiet | Web::Small, this);
    m_other->setObjectName(QStringLiteral("otherDocuments"));
    m_cancel = Web::button(tr("Cancel"), Web::Quiet | Web::Small, this);
    m_go = Web::button(tr("Pair the sentences"), Web::Primary | Web::Small, this);
    m_go->setObjectName(QStringLiteral("save"));
    buttons()->addWidget(m_other);
    buttons()->addWidget(m_cancel);
    buttons()->addWidget(m_go);
    connect(m_cancel, &QPushButton::clicked, this, &QDialog::reject);
    connect(m_other, &QPushButton::clicked, this, [this] {
        m_hasResult = false;
        setError(QString());
        render();
    });
    connect(m_go, &QPushButton::clicked, this, [this] {
        if (m_hasResult)
            add();
        else
            pair();
    });
    render();
}

void AlignmentDialog::setFile(Side side, const QString &path)
{
    m_paths[side] = path;
    if (!m_hasResult)
        render();
}

void AlignmentDialog::pair()
{
    request({});
}

void AlignmentDialog::pairAgain()
{
    QJsonArray fixed;
    for (const Unit &u : fixedUnits()) {
        QJsonArray a, b;
        for (int i : u.source)
            a.append(i);
        for (int i : u.target)
            b.append(i);
        fixed.append(QJsonObject{{QStringLiteral("source"), a}, {QStringLiteral("target"), b}});
    }
    if (m_hasResult && !fixed.isEmpty())
        request(fixed);
}

// Sends both files to be paired, around the pairs that are fixed.
void AlignmentDialog::request(const QJsonArray &fixed)
{
    if (m_busy || m_paths[Source].isEmpty() || m_paths[Target].isEmpty())
        return;
    QJsonObject body;
    const QString langs[2] = {m_sourceLang, m_targetLang};
    const char *fields[2] = {"source", "target"};
    for (int s = 0; s < 2; ++s) {
        QFile f(m_paths[s]);
        const QString name = QFileInfo(m_paths[s]).fileName();
        if (!f.open(QIODevice::ReadOnly)) {
            setError(tr("%1 could not be read: %2.").arg(name, f.errorString()));
            return;
        }
        body.insert(QLatin1String(fields[s]),
                    QJsonObject{{QStringLiteral("name"), name}, {QStringLiteral("lang"), langs[s]}, {QStringLiteral("data"), QString::fromLatin1(f.readAll().toBase64())}});
    }
    if (!fixed.isEmpty())
        body.insert(QStringLiteral("fixed"), fixed);
    m_busy = true;
    setError(QString());
    if (m_hasResult)
        refresh();
    else
        render();
    QPointer<AlignmentDialog> self(this);
    onResult(m_api->post(QStringLiteral("/api/align"), body), this, [self](const ApiResult &r) {
        if (!self)
            return;
        self->m_busy = false;
        if (!r.ok()) {
            self->setError(r.error.message);
            if (self->m_hasResult)
                self->refresh();
            else
                self->render();
            return;
        }
        const QJsonObject o = r.object();
        self->m_segments[Source] = o.value(QLatin1String("source")).toArray();
        self->m_segments[Target] = o.value(QLatin1String("target")).toArray();
        self->m_links.clear();
        self->m_mine.clear();
        self->m_scores.clear();
        for (const QJsonValue &v : o.value(QLatin1String("pairs")).toArray()) {
            const QJsonObject p = v.toObject();
            const QList<int> a = ints(p.value(QLatin1String("sourceSegments")).toArray());
            const QList<int> b = ints(p.value(QLatin1String("targetSegments")).toArray());
            const bool fixedPair = p.value(QLatin1String("fixed")).toBool();
            if (!fixedPair)
                self->m_scores.insert(keyOf(a, b), p.value(QLatin1String("score")).toInt());
            if (a.isEmpty() || b.isEmpty())
                continue;
            const auto link = [&self, fixedPair](int s, int t) {
                if (!self->m_links.contains({s, t}))
                    self->m_links.append({s, t});
                if (fixedPair && !self->m_mine.contains({s, t}))
                    self->m_mine.append({s, t});
            };
            // Every segment of a pair hangs together: each sentence on the
            // first translation, each translation on the last sentence.
            for (int s : a)
                link(s, b.first());
            for (int t : b)
                link(a.last(), t);
        }
        self->m_pickedSide = -1;
        self->m_picked = -1;
        self->m_hasResult = true;
        self->render();
        emit self->paired();
    });
}

QJsonArray AlignmentDialog::joined(Side side, const QList<int> &which) const
{
    QJsonArray out;
    QString before;
    for (int i : which) {
        const QJsonArray next = m_segments[side].at(i).toArray();
        const QString after = SegmentContent::plainText(next);
        if (!before.isEmpty() && !after.isEmpty() && !spaceless(lastChar(before)) && !spaceless(firstChar(after))) {
            out.append(QJsonObject{{QStringLiteral("text"), QStringLiteral(" ")}});
            before += QLatin1Char(' ');
        }
        for (const QJsonValue &run : next)
            out.append(run);
        before += after;
    }
    return out;
}

QString AlignmentDialog::text(Side side, int index) const
{
    return SegmentContent::plainText(m_segments[side].at(index).toArray());
}

QStringList AlignmentDialog::lines() const
{
    QList<QPair<int, int>> sorted = m_links;
    std::sort(sorted.begin(), sorted.end());
    QStringList out;
    for (const auto &l : sorted)
        out << QStringLiteral("%1|%2").arg(l.first).arg(l.second);
    return out;
}

QStringList AlignmentDialog::mineLines() const
{
    QList<QPair<int, int>> sorted = m_mine;
    std::sort(sorted.begin(), sorted.end());
    QStringList out;
    for (const auto &l : sorted)
        out << QStringLiteral("%1|%2").arg(l.first).arg(l.second);
    return out;
}

// Makes the lines of a segment's unit the translator's own.
void AlignmentDialog::own(Side side, int index)
{
    for (const Unit &u : units()) {
        if (!(side == Source ? u.source : u.target).contains(index))
            continue;
        for (const auto &l : m_links) {
            if (u.source.contains(l.first) && !m_mine.contains(l))
                m_mine.append(l);
        }
    }
}

// A line added is its maker's, and so is what it joins.
void AlignmentDialog::draw(const QPair<int, int> &line)
{
    if (!m_links.contains(line))
        m_links.append(line);
    own(Source, line.first);
}

// The units whose lines are all the translator's own: what stays when the
// rest is paired again.
QList<AlignmentDialog::Unit> AlignmentDialog::fixedUnits() const
{
    QList<Unit> out;
    for (const Unit &u : units()) {
        bool all = true;
        for (const auto &l : m_links) {
            if (u.source.contains(l.first) && !m_mine.contains(l))
                all = false;
        }
        if (all)
            out << u;
    }
    return out;
}

void AlignmentDialog::confirmPicked()
{
    if (!m_hasResult || m_picked < 0)
        return;
    own(Side(m_pickedSide), m_picked);
    m_picked = -1;
    m_pickedSide = -1;
    refresh();
}

bool AlignmentDialog::alone(Side side, int index) const
{
    for (const auto &l : m_links) {
        if ((side == Source ? l.first : l.second) == index)
            return false;
    }
    return true;
}

// The units the lines make, in the order of the document: segments joined
// by lines, directly or through others, are one.
QList<AlignmentDialog::Unit> AlignmentDialog::units() const
{
    const int ns = int(m_segments[Source].size());
    QList<int> parent(ns + m_segments[Target].size());
    for (int i = 0; i < parent.size(); ++i)
        parent[i] = i;
    const auto find = [&parent](int x) {
        while (parent[x] != x) {
            parent[x] = parent[parent[x]];
            x = parent[x];
        }
        return x;
    };
    for (const auto &l : m_links) {
        const int a = find(l.first);
        const int b = find(ns + l.second);
        if (a != b)
            parent[a] = b;
    }
    QHash<int, int> at;
    QList<Unit> out;
    for (const auto &l : m_links) {
        const int root = find(l.first);
        if (!at.contains(root)) {
            at.insert(root, int(out.size()));
            out.append(Unit());
        }
        Unit &u = out[at.value(root)];
        if (!u.source.contains(l.first))
            u.source << l.first;
        if (!u.target.contains(l.second))
            u.target << l.second;
    }
    for (Unit &u : out) {
        std::sort(u.source.begin(), u.source.end());
        std::sort(u.target.begin(), u.target.end());
    }
    std::sort(out.begin(), out.end(), [](const Unit &a, const Unit &b) { return a.source.first() < b.source.first(); });
    return out;
}

QList<AlignmentDialog::Unit> AlignmentDialog::lookUnits(const QList<Unit> &units) const
{
    // A pair that was checked needs no look any more.
    QList<int> checked;
    for (const Unit &f : fixedUnits())
        checked << f.source.first();
    QList<Unit> out;
    for (const Unit &u : units) {
        const auto it = m_scores.constFind(keyOf(u.source, u.target));
        if (it != m_scores.constEnd() && *it < LookBelow && !checked.contains(u.source.first()))
            out << u;
    }
    return out;
}

int AlignmentDialog::unitCount() const
{
    return int(units().size());
}

QString AlignmentDialog::status() const
{
    const QList<Unit> us = units();
    const int look = int(lookUnits(us).size());
    int lone = 0;
    for (int s = 0; s < 2; ++s) {
        for (int i = 0; i < m_segments[s].size(); ++i)
            lone += alone(Side(s), i) ? 1 : 0;
    }
    QString out = tr("%1 from %2 and %3.")
                      .arg(plural(int(us.size()), tr("unit"), tr("units")), QLocale(QLocale::English).toString(m_segments[Source].size()),
                           plural(int(m_segments[Target].size()), tr("segment"), tr("segments")));
    if (const int checked = checkedCount(); checked > 0)
        out += QLatin1Char(' ') + tr("%1 checked.").arg(plural(checked, tr("unit is"), tr("units are")));
    if (lone > 0)
        out += QLatin1Char(' ') + tr("%1 no pair and %2 left out.").arg(plural(lone, tr("segment has"), tr("segments have")), lone == 1 ? tr("is") : tr("are"));
    if (look > 0)
        out += QLatin1Char(' ') + tr("%1 a look: the lengths or numbers do not fit.").arg(plural(look, tr("pair needs"), tr("pairs need")));
    if (lone == 0 && look == 0)
        out += QLatin1Char(' ') + tr("All of them fit.");
    return out;
}

void AlignmentDialog::choose(Side side, int index)
{
    if (!m_hasResult || index < 0 || index >= m_segments[side].size())
        return;
    if (m_picked >= 0 && m_pickedSide != side) {
        const QPair<int, int> l = side == Target ? qMakePair(m_picked, index) : qMakePair(index, m_picked);
        if (m_links.contains(l)) {
            m_links.removeAll(l);
            m_mine.removeAll(l);
        } else {
            draw(l);
        }
        m_picked = -1;
        m_pickedSide = -1;
    } else if (m_picked == index && m_pickedSide == side) {
        m_picked = -1;
        m_pickedSide = -1;
    } else {
        m_pickedSide = side;
        m_picked = index;
    }
    refresh();
}

void AlignmentDialog::pull(Side from, int index, int other)
{
    if (!m_hasResult)
        return;
    const QPair<int, int> l = from == Source ? qMakePair(index, other) : qMakePair(other, index);
    if (l.first < 0 || l.first >= m_segments[Source].size() || l.second < 0 || l.second >= m_segments[Target].size())
        return;
    draw(l);
    m_picked = -1;
    m_pickedSide = -1;
    refresh();
}

void AlignmentDialog::takeLine(int source, int target)
{
    m_links.removeAll({source, target});
    m_mine.removeAll({source, target});
    refresh();
}

// Where on the board there is something to look at, from the top.
QList<int> AlignmentDialog::lookPlaces() const
{
    QList<int> ys;
    if (!m_board)
        return ys;
    for (int s = 0; s < 2; ++s) {
        for (int i = 0; i < m_board->segments[s].size(); ++i) {
            if (alone(Side(s), i))
                ys << m_board->segments[s].at(i)->geometry().center().y();
        }
    }
    for (const Unit &u : lookUnits(units())) {
        if (AlignSegment *seg = m_board->segments[Source].value(u.source.first()))
            ys << seg->geometry().center().y();
    }
    std::sort(ys.begin(), ys.end());
    return ys;
}

void AlignmentDialog::next()
{
    if (!m_scroll)
        return;
    const QList<int> ys = lookPlaces();
    if (ys.isEmpty())
        return;
    QScrollBar *bar = m_scroll->verticalScrollBar();
    const int half = m_scroll->viewport()->height() / 2;
    const int from = bar->value() + half;
    int y = ys.first();
    for (int v : ys) {
        if (v > from + 1) {
            y = v;
            break;
        }
    }
    bar->setValue(qMax(0, y - half));
}

void AlignmentDialog::add()
{
    const QList<Unit> us = units();
    if (m_busy || !m_hasResult || us.isEmpty())
        return;
    QJsonArray body;
    for (const Unit &u : us)
        body.append(QJsonObject{{QStringLiteral("source"), joined(Source, u.source)}, {QStringLiteral("target"), joined(Target, u.target)}});
    QUrlQuery query;
    if (m_overwrite)
        query.addQueryItem(QStringLiteral("policy"), QStringLiteral("overwrite"));
    m_busy = true;
    setError(QString());
    m_go->setEnabled(false);
    const int n = int(body.size());
    QPointer<AlignmentDialog> self(this);
    onResult(m_api->post(QStringLiteral("/api/memories/%1/units/batch").arg(ApiClient::seg(m_memory)), QJsonObject{{QStringLiteral("units"), body}}, query), this,
             [self, n](const ApiResult &r) {
                 if (!self)
                     return;
                 self->m_busy = false;
                 if (!r.ok()) {
                     self->setError(r.error.message);
                     self->refresh();
                     return;
                 }
                 emit self->added(n);
                 self->accept();
             });
}

// What the lines changed: the state of the segments, the count and the lines.
void AlignmentDialog::refresh()
{
    if (!m_hasResult || !m_board)
        return;
    for (int s = 0; s < 2; ++s) {
        for (int i = 0; i < m_board->segments[s].size(); ++i)
            m_board->segments[s].at(i)->setState(m_pickedSide == s && m_picked == i, alone(Side(s), i));
    }
    const QList<Unit> us = units();
    m_board->lines = m_links;
    m_board->mineLines = m_mine;
    m_board->lookLines.clear();
    for (const Unit &u : lookUnits(us)) {
        for (const auto &l : m_links) {
            if (u.source.contains(l.first))
                m_board->lookLines << l;
        }
    }
    m_board->update();
    m_status->setText(status());
    m_next->setEnabled(!lookPlaces().isEmpty());
    bool unchecked = false;
    for (const auto &l : m_links) {
        if (m_picked >= 0 && (m_pickedSide == Source ? l.first : l.second) == m_picked && !m_mine.contains(l))
            unchecked = true;
    }
    m_confirm->setEnabled(unchecked);
    const int checked = checkedCount();
    bool anyAlone = false;
    for (int s = 0; s < 2 && !anyAlone; ++s) {
        for (int i = 0; i < m_segments[s].size() && !anyAlone; ++i)
            anyAlone = alone(Side(s), i);
    }
    m_again->setText(m_busy ? tr("Pairing…") : tr("Pair the rest again"));
    m_again->setEnabled(!m_busy && checked > 0 && !(checked == us.size() && !anyAlone));
    m_go->setText(tr("Add %1 to %2").arg(plural(int(us.size()), tr("unit"), tr("units")), m_memory));
    m_go->setEnabled(!us.isEmpty() && !m_busy);
}

void AlignmentDialog::render()
{
    empty(body());
    m_board = nullptr;
    m_scroll = nullptr;
    m_status = nullptr;
    m_next = nullptr;
    m_confirm = nullptr;
    m_again = nullptr;
    m_other->setVisible(m_hasResult);
    if (!m_hasResult) {
        auto *help = Web::text(tr("Choose a document and its translation. Their sentences are paired, and you check the pairs before they go into the memory. "
                                  "The two files need not be of the same format: DOCX, PPTX, XLSX, HTML, IDML or text."),
                               "help", this);
        help->setWordWrap(true);
        body()->addWidget(help);
        const QString labels[2] = {tr("Document in %1").arg(Languages::name(m_sourceLang)), tr("Its translation in %1").arg(Languages::name(m_targetLang))};
        const char *names[2] = {"chooseSource", "chooseTarget"};
        for (int s = 0; s < 2; ++s) {
            auto *row = new QWidget(this);
            auto *h = new QHBoxLayout(row);
            h->setContentsMargins(0, 0, 0, 0);
            h->setSpacing(12);
            auto *choose = Web::button(tr("Choose File"), Web::Small, row);
            choose->setObjectName(QLatin1String(names[s]));
            choose->setAccessibleName(labels[s]);
            auto *name = Web::text(m_paths[s].isEmpty() ? tr("No file chosen") : QFileInfo(m_paths[s]).fileName(), m_paths[s].isEmpty() ? "muted" : "", row);
            name->setObjectName(QStringLiteral("%1Name").arg(QLatin1String(names[s])));
            h->addWidget(choose);
            h->addWidget(name, 1);
            body()->addWidget(Web::field(labels[s], row, this));
            const Side side = Side(s);
            const QString title = labels[s];
            connect(choose, &QPushButton::clicked, this, [this, side, title] {
                const QString path = QFileDialog::getOpenFileName(this, title, QString(),
                                                                  tr("Documents (*.docx *.docm *.pptx *.pptm *.ppsx *.xlsx *.xlsm *.html *.htm *.xhtml *.idml *.txt *.md);;All files (*)"));
                if (!path.isEmpty())
                    setFile(side, path);
            });
        }
        m_go->setText(m_busy ? tr("Pairing…") : tr("Pair the sentences"));
        m_go->setEnabled(!m_busy && !m_paths[Source].isEmpty() && !m_paths[Target].isEmpty());
        fitHeight();
        return;
    }

    m_status = Web::text(QString(), "", this);
    m_status->setObjectName(QStringLiteral("alignStatus"));
    m_status->setWordWrap(true);
    body()->addWidget(m_status);
    auto *help = Web::text(tr("A line joins a sentence and its translation, from the point on the one to the point on the other; a ring instead of a point means "
                              "nothing is joined there. To draw a line, choose a sentence and then its translation, or pull from one point to the other; the same "
                              "takes a line away, as does choosing the line. Sentences joined by lines become one unit. A dotted line was paired for you; one you draw or "
                              "confirm is solid, and stays when the rest is paired again around it."),
                           "help", this);
    help->setWordWrap(true);
    body()->addWidget(help);

    auto *head = new QWidget(this);
    auto *hl = new QHBoxLayout(head);
    hl->setContentsMargins(0, 0, 0, 0);
    m_next = Web::button(tr("Next to look at"), Web::Quiet | Web::Small, head);
    m_next->setObjectName(QStringLiteral("nextToLook"));
    m_confirm = Web::button(tr("Confirm"), Web::Quiet | Web::Small, head);
    m_confirm->setObjectName(QStringLiteral("confirmLines"));
    m_again = Web::button(tr("Pair the rest again"), Web::Quiet | Web::Small, head);
    m_again->setObjectName(QStringLiteral("pairAgain"));
    hl->setSpacing(S2);
    hl->addWidget(Web::langChip(m_sourceLang, head));
    hl->addStretch();
    hl->addWidget(m_confirm);
    hl->addWidget(m_again);
    hl->addWidget(m_next);
    hl->addStretch();
    connect(m_confirm, &QPushButton::clicked, this, &AlignmentDialog::confirmPicked);
    connect(m_again, &QPushButton::clicked, this, &AlignmentDialog::pairAgain);
    hl->addWidget(Web::langChip(m_targetLang, head));
    connect(m_next, &QPushButton::clicked, this, &AlignmentDialog::next);
    body()->addWidget(head);

    m_scroll = new QScrollArea(this);
    m_scroll->setObjectName(QStringLiteral("alignScroll"));
    m_scroll->setAccessibleName(tr("The two texts and the lines between them"));
    m_scroll->setWidgetResizable(true);
    m_scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    m_board = new AlignBoard(m_scroll);
    auto *grid = new QHBoxLayout(m_board);
    grid->setContentsMargins(S3, S3, S3, S3);
    grid->setSpacing(0);
    for (int s = 0; s < 2; ++s) {
        auto *col = new QVBoxLayout;
        col->setContentsMargins(0, 0, 0, 0);
        col->setSpacing(S2);
        for (int i = 0; i < m_segments[s].size(); ++i) {
            auto *seg = new AlignSegment(s, i, text(Side(s), i), m_board);
            m_board->segments[s] << seg;
            col->addWidget(seg);
            const Side side = Side(s);
            connect(seg, &AlignSegment::chosen, this, [this, side, i] { choose(side, i); });
            connect(seg, &AlignSegment::pulledTo, this, [this, side, i](const QPoint &global) {
                if (!m_board)
                    return;
                if (AlignSegment *to = m_board->segmentAt(m_board->mapFromGlobal(global)); to && to->side() != int(side))
                    pull(side, i, to->index());
            });
        }
        col->addStretch();
        grid->addLayout(col, 1);
        if (s == 0)
            grid->addSpacing(Gutter);
    }
    connect(m_board, &AlignBoard::lineChosen, this, &AlignmentDialog::takeLine);
    connect(m_board, &AlignBoard::portChosen, this, [this](int side, int index) { choose(Side(side), index); });
    connect(m_board, &AlignBoard::portPulled, this, [this](int side, int index, int, int other) { pull(Side(side), index, other); });
    m_scroll->setWidget(m_board);
    m_scroll->setMinimumHeight(240);
    m_scroll->setMaximumHeight(520);
    body()->addWidget(m_scroll);

    auto *both = new QCheckBox(tr("Replace other translations of these sentences"), this);
    both->setObjectName(QStringLiteral("unitOverwrite"));
    both->setChecked(m_overwrite);
    connect(both, &QCheckBox::toggled, this, [this](bool on) { m_overwrite = on; });
    body()->addWidget(both);
    refresh();
    fitHeight();
}

// The dialog keeps its width and is as high as what it holds.
void AlignmentDialog::fitHeight()
{
    const auto fit = [this] {
        // What it holds is styled before it is measured.
        for (QWidget *w : findChildren<QWidget *>())
            w->ensurePolished();
        const int width = qMax(this->width(), 920);
        if (m_board && m_scroll) {
            const int inner = width - 2 * S5 - 2 - style()->pixelMetric(QStyle::PM_ScrollBarExtent);
            m_board->layout()->invalidate();
            const int want = m_board->layout()->hasHeightForWidth() ? m_board->layout()->totalHeightForWidth(inner) : m_board->sizeHint().height();
            m_scroll->setFixedHeight(qBound(120, want + 2, 520));
        }
        layout()->invalidate();
        layout()->activate();
        resize(width, layout()->hasHeightForWidth() ? layout()->totalHeightForWidth(width) : layout()->totalSizeHint().height());
    };
    fit();
    // Shown, the parts may be larger than they said before: once more then.
    QPointer<AlignmentDialog> self(this);
    QTimer::singleShot(0, this, [self, fit] {
        if (self)
            fit();
    });
}
