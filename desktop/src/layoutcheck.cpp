#include "layoutcheck.h"

#include <QAbstractButton>
#include <QAbstractScrollArea>
#include <QApplication>
#include <QComboBox>
#include <QElapsedTimer>
#include <QHash>
#include <QLabel>
#include <QLayout>
#include <QLineEdit>
#include <QPlainTextEdit>
#include <QPointer>
#include <QPushButton>
#include <QResizeEvent>
#include <QScrollArea>
#include <QSet>
#include <QStackedLayout>
#include <QStackedWidget>
#include <QTabBar>
#include <QTextEdit>
#include <QTimer>

#include <functional>

namespace LayoutCheck {

namespace {

QString where(const QWidget *w)
{
    QStringList path;
    for (const QWidget *p = w; p && path.size() < 6; p = p->parentWidget()) {
        const QString n = !p->objectName().isEmpty() ? p->objectName() : QString::fromLatin1(p->metaObject()->className());
        if (!n.startsWith(QLatin1String("qt_")) && n != QLatin1String("QWidget"))
            path.prepend(n);
    }
    return path.join(QLatin1Char('/'));
}

QString shown(const QString &text)
{
    QString t = text.left(50);
    t.replace(QLatin1Char('\n'), QLatin1Char(' '));
    return QStringLiteral("\"%1\"").arg(t);
}

bool overlay(const QWidget *w)
{
    for (; w; w = w->parentWidget()) {
        if (w->property("layoutOverlay").toBool())
            return true;
    }
    return false;
}

// Inside a scrolling area's moving content, or a list's own drawing,
// parts may reach past what is visible: that is what scrolling is for.
bool scrolled(const QWidget *w)
{
    for (const QWidget *p = w->parentWidget(); p; p = p->parentWidget()) {
        if (qobject_cast<const QAbstractScrollArea *>(p))
            return true;
    }
    return false;
}

void checkLabel(QLabel *l, QStringList &out)
{
    if (l->text().isEmpty() || l->width() < 2 || l->pixmap(Qt::ReturnByValue).width() > 0)
        return;
    const int need = l->wordWrap() ? l->heightForWidth(l->width()) : l->sizeHint().height();
    if (need > l->height() + 2)
        out << QStringLiteral("text cut off at the bottom by %1px: %2 in %3 (%4x%5)").arg(need - l->height()).arg(shown(l->text()), where(l)).arg(l->width()).arg(l->height());
    else if (!l->wordWrap() && l->textFormat() != Qt::RichText && l->sizeHint().width() > l->width() + 2)
        out << QStringLiteral("text cut off at the side by %1px: %2 in %3").arg(l->sizeHint().width() - l->width()).arg(shown(l->text()), where(l));
}

void checkButton(QAbstractButton *b, QStringList &out)
{
    if (b->text().isEmpty() || qobject_cast<QTabBar *>(b->parentWidget()))
        return;
    const QSize hint = b->sizeHint();
    if (hint.width() > b->width() + 2)
        out << QStringLiteral("button narrower than its text by %1px: %2 in %3").arg(hint.width() - b->width()).arg(shown(b->text()), where(b));
    if (b->height() + 2 < qMin(hint.height(), b->fontMetrics().height() + 2))
        out << QStringLiteral("button lower than its text: %1 in %2").arg(shown(b->text()), where(b));
}

void checkTextBox(QWidget *w, QStringList &out)
{
    // A box of several lines shows one line at the least in its viewport;
    // one of a single line its text and a little frame.
    const int text = w->fontMetrics().height();
    if (auto *area = qobject_cast<QAbstractScrollArea *>(w)) {
        if (area->viewport()->height() + 1 < text)
            out << QStringLiteral("box too low for a line of text (%1px of %2px) in %3").arg(area->viewport()->height()).arg(text).arg(where(w));
        return;
    }
    if (w->height() < text + 2)
        out << QStringLiteral("box too low for its text (%1px of %2px) in %3").arg(w->height()).arg(text + 2).arg(where(w));
}

// Children laid out side by side or under each other must not cover each
// other, nor reach out of their parent.
void checkChildren(QWidget *parent, QStringList &out)
{
    if (qobject_cast<QStackedWidget *>(parent) || qobject_cast<QStackedLayout *>(parent->layout()))
        return;
    QList<QWidget *> kids;
    for (QObject *o : parent->children()) {
        auto *w = qobject_cast<QWidget *>(o);
        if (!w || !w->isVisible() || w->isWindow() || overlay(w) || w->objectName().startsWith(QLatin1String("qt_")))
            continue;
        // Only parts a layout places: a widget placed by hand chooses where.
        if (!parent->layout() || parent->layout()->indexOf(w) < 0)
            continue;
        kids << w;
    }
    const QRect inside = parent->rect();
    for (int i = 0; i < kids.size(); ++i) {
        const QRect a = kids[i]->geometry();
        if (a.width() <= 0 || a.height() <= 0)
            continue;
        if (!scrolled(parent) && (a.right() > inside.right() + 2 || a.bottom() > inside.bottom() + 2 || a.left() < -2 || a.top() < -2))
            out << QStringLiteral("part reaches out of what holds it: %1 in %2 (%3,%4 %5x%6 in %7x%8)")
                       .arg(where(kids[i]), where(parent))
                       .arg(a.x())
                       .arg(a.y())
                       .arg(a.width())
                       .arg(a.height())
                       .arg(inside.width())
                       .arg(inside.height());
        for (int k = i + 1; k < kids.size(); ++k) {
            const QRect both = a.intersected(kids[k]->geometry());
            if (both.width() > 2 && both.height() > 2)
                out << QStringLiteral("parts lie over each other: %1 and %2").arg(where(kids[i]), where(kids[k]));
        }
    }
}

// Watches every widget's size for a layout that never settles: a widget
// that keeps going back and forth between the same few sizes, without
// anyone doing anything, is laid out by parts that undo each other (a
// scrollbar shown and hidden again, a height set and taken back).
class Settling : public QObject {
public:
    explicit Settling(std::function<void(const QString &)> report)
        : QObject(qApp)
        , m_report(std::move(report))
    {
        m_clock.start();
        qApp->installEventFilter(this);
    }

protected:
    bool eventFilter(QObject *o, QEvent *e) override
    {
        switch (e->type()) {
        case QEvent::KeyPress:
        case QEvent::KeyRelease:
        case QEvent::MouseButtonPress:
        case QEvent::MouseButtonRelease:
        case QEvent::MouseButtonDblClick:
        case QEvent::Wheel:
        case QEvent::DragMove:
        case QEvent::Drop:
            // What follows a person's typing or clicking is their doing:
            // a layout is judged once they have stopped.
            m_lastInput = m_clock.elapsed();
            m_tracks.clear();
            return false;
        default:
            break;
        }
        if (e->type() != QEvent::Resize || !o->isWidgetType())
            return false;
        if (m_clock.elapsed() - m_lastInput < Quiet)
            return false;
        auto *w = static_cast<QWidget *>(o);
        if (!w->isVisible() || overlay(w) || w->objectName().startsWith(QLatin1String("qt_")))
            return false;
        const qint64 now = m_clock.elapsed();
        Track &t = m_tracks[w];
        if (t.widget != w) {
            t = Track();
            t.widget = w;
        }
        t.sizes.append({now, static_cast<QResizeEvent *>(e)->size()});
        while (!t.sizes.isEmpty() && now - t.sizes.first().first > Window)
            t.sizes.removeFirst();
        if (t.sizes.size() >= Changes && !t.reported) {
            // Few sizes, each come back to again and again.
            QList<QSize> seen;
            int returns = 0;
            for (int i = 0; i < t.sizes.size(); ++i) {
                const QSize s = t.sizes[i].second;
                if (seen.contains(s) && (i == 0 || t.sizes[i - 1].second != s))
                    ++returns;
                if (!seen.contains(s))
                    seen << s;
            }
            if (seen.size() <= 3 && returns >= Changes / 2) {
                t.reported = true;
                QStringList sizes;
                for (const QSize &s : std::as_const(seen))
                    sizes << QStringLiteral("%1x%2").arg(s.width()).arg(s.height());
                const auto *l = qobject_cast<const QLabel *>(w);
                const QString what = l && !l->text().isEmpty() ? QStringLiteral("%1 in %2").arg(shown(l->text()), where(w)) : where(w);
                m_report(QStringLiteral("layout never settles: %1 keeps switching between %2").arg(what, sizes.join(QLatin1String(" and "))));
            }
        }
        // Widgets gone or quiet for a while are forgotten.
        if (++m_seen % 512 == 0) {
            for (auto it = m_tracks.begin(); it != m_tracks.end();) {
                if (!it->widget || it->sizes.isEmpty() || now - it->sizes.last().first > Window)
                    it = m_tracks.erase(it);
                else
                    ++it;
            }
        }
        return false;
    }

private:
    // Changes of size within a second that count as unsettled: far more than
    // a person or a test makes, as many as parts undoing each other make.
    static constexpr qint64 Window = 1000;
    static constexpr int Changes = 30;
    // How long after the last input a layout may still be following it.
    static constexpr qint64 Quiet = 300;
    struct Track {
        QPointer<QWidget> widget;
        QList<QPair<qint64, QSize>> sizes;
        bool reported = false;
    };
    std::function<void(const QString &)> m_report;
    QHash<const QObject *, Track> m_tracks;
    QElapsedTimer m_clock;
    quint64 m_seen = 0;
    qint64 m_lastInput = -Quiet;
};

} // namespace

QStringList problems(QWidget *window)
{
    QStringList out;
    if (!window || !window->isVisible())
        return out;
    QList<QWidget *> all = window->findChildren<QWidget *>();
    all.prepend(window);
    for (QWidget *w : all) {
        if (!w->isVisible() || overlay(w) || w->objectName().startsWith(QLatin1String("qt_")))
            continue;
        // A widget wholly scrolled out of sight is not looked at.
        if (scrolled(w) && w->visibleRegion().isEmpty())
            continue;
        if (auto *l = qobject_cast<QLabel *>(w))
            checkLabel(l, out);
        else if (auto *b = qobject_cast<QAbstractButton *>(w))
            checkButton(b, out);
        else if (qobject_cast<QPlainTextEdit *>(w) || qobject_cast<QTextEdit *>(w) || qobject_cast<QLineEdit *>(w) || qobject_cast<QComboBox *>(w))
            checkTextBox(w, out);
        checkChildren(w, out);
    }
    out.removeDuplicates();
    return out;
}

void markOverlay(QWidget *w)
{
    w->setProperty("layoutOverlay", true);
}

void watch(std::function<void(const QString &)> report)
{
    static bool on = false;
    if (on)
        return;
    on = true;
    // A layout that keeps changing is told at once, once.
    new Settling(report);
    auto *reported = new QSet<QString>;
    auto *held = new QHash<QString, int>;
    auto *timer = new QTimer(qApp);
    timer->setInterval(150);
    QObject::connect(timer, &QTimer::timeout, qApp, [report, reported, held] {
        QStringList found;
        for (QWidget *top : QApplication::topLevelWidgets()) {
            if (top->isVisible() && !top->property("layoutOverlay").toBool())
                found << problems(top);
        }
        // A window being laid out is not judged: only what stays wrong
        // over three looks counts.
        for (auto it = held->begin(); it != held->end();) {
            if (found.contains(it.key()))
                ++it;
            else
                it = held->erase(it);
        }
        for (const QString &line : std::as_const(found)) {
            if (++(*held)[line] < 3 || reported->contains(line))
                continue;
            reported->insert(line);
            report(line);
        }
    });
    timer->start();
}

} // namespace LayoutCheck
