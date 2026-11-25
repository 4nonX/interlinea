#include "layoutcheck.h"

#include <QAbstractButton>
#include <QAbstractScrollArea>
#include <QApplication>
#include <QComboBox>
#include <QHash>
#include <QLabel>
#include <QLayout>
#include <QLineEdit>
#include <QPlainTextEdit>
#include <QPushButton>
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
        out << QStringLiteral("text cut off at the bottom by %1px: %2 in %3").arg(need - l->height()).arg(shown(l->text()), where(l));
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
            out << QStringLiteral("part reaches out of what holds it: %1 in %2").arg(where(kids[i]), where(parent));
        for (int k = i + 1; k < kids.size(); ++k) {
            const QRect both = a.intersected(kids[k]->geometry());
            if (both.width() > 2 && both.height() > 2)
                out << QStringLiteral("parts lie over each other: %1 and %2").arg(where(kids[i]), where(kids[k]));
        }
    }
}

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
