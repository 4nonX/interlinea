#include "webui.h"

#include "icons.h"
#include "languages.h"
#include "theme.h"

#include <QAbstractButton>
#include <QDragEnterEvent>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QMimeData>
#include <QPushButton>
#include <QScrollArea>
#include <QStyle>
#include <QVBoxLayout>

namespace Web {

namespace {
constexpr int S1 = 4, S2 = 8, S3 = 12, S5 = 24, S6 = 32;

void repolish(QWidget *w)
{
    w->style()->unpolish(w);
    w->style()->polish(w);
}
} // namespace

Page page(QWidget *parent, int maxWidth)
{
    Page p;
    p.scroll = new QScrollArea(parent);
    p.scroll->setObjectName(QStringLiteral("page"));
    p.scroll->setWidgetResizable(true);
    p.scroll->setFrameShape(QFrame::NoFrame);
    p.scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    auto *outer = new QWidget(p.scroll);
    outer->setObjectName(QStringLiteral("pageOuter"));
    auto *h = new QHBoxLayout(outer);
    h->setContentsMargins(S6, S5, S6, S6);
    h->setSpacing(0);
    auto *column = new QWidget(outer);
    column->setMaximumWidth(maxWidth);
    p.body = new QVBoxLayout(column);
    p.body->setContentsMargins(0, 0, 0, 0);
    p.body->setSpacing(S5);
    h->addStretch(0);
    h->addWidget(column, 1);
    h->addStretch(0);
    p.scroll->setWidget(outer);
    return p;
}

QLabel *text(const QString &t, const char *role, QWidget *parent)
{
    auto *l = new QLabel(t, parent);
    if (role)
        Theme::role(l, role);
    l->setWordWrap(true);
    return l;
}

QWidget *header(const QString &title, const QString &description, const QList<QWidget *> &actions, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(16);
    auto *left = new QVBoxLayout;
    left->setSpacing(S1);
    auto *t = new QLabel(title, w);
    t->setObjectName(QStringLiteral("pageTitle"));
    Theme::role(t, "pageTitle");
    t->setAccessibleName(title);
    t->setFocusPolicy(Qt::TabFocus);
    left->addWidget(t);
    if (!description.isEmpty())
        left->addWidget(text(description, "secondary", w));
    h->addLayout(left, 1);
    if (!actions.isEmpty()) {
        auto *a = new QHBoxLayout;
        a->setSpacing(S2);
        for (QWidget *b : actions)
            a->addWidget(b);
        h->addLayout(a);
        h->setAlignment(a, Qt::AlignTop);
    }
    return w;
}

QWidget *section(const QString &title, QWidget *parent, QVBoxLayout **body)
{
    auto *w = new QWidget(parent);
    w->setAccessibleName(title);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S3);
    if (!title.isEmpty())
        v->addWidget(text(title, "sectionTitle16", w));
    *body = v;
    return w;
}

QFrame *panel(const QString &title, QWidget *parent, QVBoxLayout **body)
{
    auto *f = new QFrame(parent);
    f->setObjectName(QStringLiteral("panel"));
    f->setAccessibleName(title);
    auto *v = new QVBoxLayout(f);
    v->setContentsMargins(S5, S5, S5, S5);
    v->setSpacing(S5);
    if (!title.isEmpty())
        v->addWidget(text(title, "panelTitle", f));
    *body = v;
    return f;
}

QPushButton *button(const QString &t, int kind, QWidget *parent, const QString &icon)
{
    auto *b = new QPushButton(t, parent);
    if (kind & Primary)
        Theme::primary(b);
    if (kind & Quiet)
        Theme::quietButton(b);
    if (kind & Small)
        Theme::smallButton(b);
    if (!icon.isEmpty()) {
        b->setIcon(Icons::get(icon, (kind & Quiet) ? Theme::color("accentText") : (kind & Primary) ? QColor(Qt::white) : QColor()));
        b->setIconSize(QSize(14, 14));
    }
    return b;
}

QLabel *langChip(const QString &code, QWidget *parent)
{
    auto *l = new QLabel(code, parent);
    l->setProperty("chip", QStringLiteral("lang"));
    l->setFixedHeight(18);
    l->setToolTip(Languages::name(code));
    l->setAccessibleName(Languages::name(code));
    return l;
}

QWidget *langPair(const QString &source, const QStringList &targets, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(S1);
    h->addWidget(langChip(source, w));
    auto *arrow = new QLabel(QStringLiteral("→"), w);
    Theme::role(arrow, "muted");
    arrow->setAccessibleName(QObject::tr("to"));
    h->addWidget(arrow);
    for (const QString &t : targets)
        h->addWidget(langChip(t, w));
    h->addStretch();
    QStringList names;
    for (const QString &t : targets)
        names << Languages::name(t);
    w->setAccessibleName(QObject::tr("%1 to %2").arg(Languages::name(source), names.join(QLatin1String(", "))));
    return w;
}

QWidget *locationBadge(bool server, QWidget *parent, const QString &host)
{
    auto *w = new QFrame(parent);
    const QColor fg = server ? Theme::color("server") : Theme::color("text2");
    const QColor bg = server ? Theme::color("serverBg") : (Theme::dark() ? QColor(0x2a, 0x30, 0x38) : QColor(0xec, 0xef, 0xf3));
    w->setObjectName(QStringLiteral("badge"));
    w->setStyleSheet(QStringLiteral("QFrame#badge { background: %1; border-radius: 10px; } QLabel { color: %2; font-size: 12px; font-weight: 600; background: transparent; }")
                         .arg(bg.name(), fg.name()));
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(7, 0, 7, 0);
    h->setSpacing(S1);
    auto *icon = new QLabel(w);
    icon->setPixmap(Icons::get(server ? QStringLiteral("cloud") : QStringLiteral("hard-drive"), fg).pixmap(13, 13));
    h->addWidget(icon);
    h->addWidget(new QLabel(server ? QObject::tr("Server") : QObject::tr("Local"), w));
    w->setFixedHeight(20);
    w->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    w->setToolTip(server ? (host.isEmpty() ? QObject::tr("On server") : QObject::tr("On server %1").arg(host)) : QObject::tr("On this computer"));
    w->setAccessibleName(server ? QObject::tr("Server") : QObject::tr("Local"));
    return w;
}

QLabel *chip(const QString &t, const QString &kind, QWidget *parent)
{
    auto *l = new QLabel(t, parent);
    QColor bg(Theme::dark() ? QColor(0x2a, 0x30, 0x38) : QColor(0xec, 0xef, 0xf3)), fg = Theme::color("text2");
    if (kind == QLatin1String("accent")) {
        bg = Theme::color("accentWeak");
        fg = Theme::color("accentText");
    } else if (kind == QLatin1String("conflict") || kind == QLatin1String("forbidden")) {
        bg = Theme::color("dangerBg");
        fg = Theme::color("danger");
    } else if (kind == QLatin1String("changed") || kind == QLatin1String("proposed")) {
        bg = Theme::color("warnBg");
        fg = Theme::color("warn");
    } else if (kind == QLatin1String("new") || kind == QLatin1String("approved")) {
        bg = Theme::color("okBg");
        fg = Theme::color("ok");
    }
    l->setStyleSheet(QStringLiteral("QLabel { background: %1; color: %2; border-radius: 10px; padding: 1px 7px; font-size: 12px; font-weight: 600; }")
                         .arg(bg.name(), fg.name()));
    l->setFixedHeight(20);
    l->setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    return l;
}

Table::Table(const QStringList &headers, QWidget *parent)
    : QTableWidget(0, int(headers.size()), parent)
{
    setObjectName(QStringLiteral("webTable"));
    setHorizontalHeaderLabels(headers);
    verticalHeader()->hide();
    setEditTriggers(QAbstractItemView::NoEditTriggers);
    setSelectionMode(QAbstractItemView::NoSelection);
    setFocusPolicy(Qt::NoFocus);
    setShowGrid(false);
    setWordWrap(true);
    setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    horizontalHeader()->setSectionResizeMode(QHeaderView::ResizeToContents);
    horizontalHeader()->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
    horizontalHeader()->setHighlightSections(false);
    horizontalHeader()->setSectionsClickable(false);
    setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
}

void Table::setNumeric(const QList<int> &columns)
{
    m_numeric = columns;
    for (int c : columns) {
        if (QTableWidgetItem *h = horizontalHeaderItem(c))
            h->setTextAlignment(Qt::AlignRight | Qt::AlignVCenter);
    }
}

void Table::setStretch(int column)
{
    horizontalHeader()->setSectionResizeMode(column, QHeaderView::Stretch);
}

int Table::addRow(const QList<QWidget *> &cells)
{
    const int row = rowCount();
    insertRow(row);
    for (int c = 0; c < cells.size() && c < columnCount(); ++c) {
        QWidget *w = cells.at(c);
        if (!w)
            continue;
        w->setParent(this);
        auto *wrap = new QWidget(this);
        auto *l = new QHBoxLayout(wrap);
        l->setContentsMargins(S3, 10, S3, 10);
        l->addWidget(w, 1, m_numeric.contains(c) ? Qt::AlignRight | Qt::AlignTop : Qt::AlignLeft | Qt::AlignTop);
        setCellWidget(row, c, wrap);
        // The text for screen readers, without sizing the column.
        auto *item = new QTableWidgetItem;
        item->setData(Qt::AccessibleTextRole, w->accessibleName());
        setItem(row, c, item);
    }
    return row;
}

void Table::clearRows()
{
    setRowCount(0);
    fit();
}

void Table::fit()
{
    for (int c = 0; c < columnCount(); ++c) {
        if (horizontalHeader()->sectionResizeMode(c) != QHeaderView::Stretch)
            resizeColumnToContents(c);
    }
    resizeRowsToContents();
    updateGeometry();
}

void Table::resizeEvent(QResizeEvent *e)
{
    QTableWidget::resizeEvent(e);
    // Wrapped text is as tall as the width it gets.
    QMetaObject::invokeMethod(this, [this] {
        resizeRowsToContents();
        updateGeometry();
    }, Qt::QueuedConnection);
}

QSize Table::sizeHint() const
{
    int h = horizontalHeader()->isVisible() ? horizontalHeader()->sizeHint().height() : 0;
    for (int r = 0; r < rowCount(); ++r)
        h += rowHeight(r);
    return QSize(QTableWidget::sizeHint().width(), h + 2 * frameWidth() + 2);
}

QSize Table::minimumSizeHint() const
{
    return QSize(0, sizeHint().height());
}

QWidget *cell(const QString &title, const QString &sub, QWidget *parent, std::function<void()> onClick)
{
    auto *w = new QWidget(parent);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(2);
    QLabel *t = nullptr;
    if (onClick) {
        t = new QLabel(QStringLiteral("<a href=\"#\" style=\"color:%1; text-decoration:none\">%2</a>").arg(Theme::color("text").name(), title.toHtmlEscaped()), w);
        t->setTextInteractionFlags(Qt::LinksAccessibleByMouse | Qt::LinksAccessibleByKeyboard);
        t->setFocusPolicy(Qt::StrongFocus);
        QObject::connect(t, &QLabel::linkActivated, w, [onClick] { onClick(); });
        t->setCursor(Qt::PointingHandCursor);
    } else {
        t = new QLabel(title.toHtmlEscaped(), w);
    }
    t->setWordWrap(true);
    QFont f = t->font();
    f.setWeight(QFont::DemiBold);
    t->setFont(f);
    v->addWidget(t);
    if (!sub.isEmpty())
        v->addWidget(text(sub, "cellSub", w));
    w->setAccessibleName(sub.isEmpty() ? title : QStringLiteral("%1, %2").arg(title, sub));
    return w;
}

QWidget *cellText(const QString &t, QWidget *parent, bool numeric, bool muted)
{
    auto *l = new QLabel(t, parent);
    l->setWordWrap(!numeric);
    if (muted)
        Theme::role(l, "muted");
    if (numeric)
        l->setAlignment(Qt::AlignRight | Qt::AlignTop);
    l->setAccessibleName(t);
    return l;
}

QWidget *cellActions(const QList<QWidget *> &buttons, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S1);
    for (QWidget *b : buttons)
        v->addWidget(b, 0, Qt::AlignLeft);
    return w;
}

QFrame *choice(QAbstractButton *input, const QString &description, QWidget *parent)
{
    auto *f = new QFrame(parent);
    f->setObjectName(QStringLiteral("choice"));
    auto *h = new QVBoxLayout(f);
    h->setContentsMargins(S3, S3, S3, S3);
    h->setSpacing(2);
    input->setParent(f);
    QFont bold = input->font();
    bold.setWeight(QFont::DemiBold);
    input->setFont(bold);
    h->addWidget(input);
    if (!description.isEmpty()) {
        auto *d = text(description, "secondary", f);
        d->setContentsMargins(26, 0, 0, 0);
        h->addWidget(d);
        input->setAccessibleDescription(description);
    }
    auto update = [f, input] {
        f->setProperty("checked", input->isChecked());
        repolish(f);
    };
    QObject::connect(input, &QAbstractButton::toggled, f, update);
    update();
    return f;
}

QWidget *field(const QString &label, QWidget *control, QWidget *parent, bool optional, const QString &help)
{
    auto *w = new QWidget(parent);
    auto *v = new QVBoxLayout(w);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(S2);
    auto *l = new QLabel(optional ? QStringLiteral("<b>%1</b> <span style=\"color:%2\">%3</span>").arg(label.toHtmlEscaped(), Theme::color("text3").name(), QObject::tr("optional"))
                                  : QStringLiteral("<b>%1</b>").arg(label.toHtmlEscaped()),
                         w);
    l->setBuddy(control);
    control->setParent(w);
    if (control->accessibleName().isEmpty())
        control->setAccessibleName(label);
    v->addWidget(l);
    v->addWidget(control);
    if (!help.isEmpty())
        v->addWidget(text(help, "help", w));
    return w;
}

DropZone::DropZone(QWidget *parent)
    : QFrame(parent)
{
    setObjectName(QStringLiteral("dropzone"));
    setAcceptDrops(true);
}

void DropZone::dragEnterEvent(QDragEnterEvent *e)
{
    if (e->mimeData()->hasUrls()) {
        e->acceptProposedAction();
        setProperty("dragging", true);
        repolish(this);
    }
}

void DropZone::dragLeaveEvent(QDragLeaveEvent *)
{
    setProperty("dragging", false);
    repolish(this);
}

void DropZone::dropEvent(QDropEvent *e)
{
    setProperty("dragging", false);
    repolish(this);
    QStringList paths;
    for (const QUrl &u : e->mimeData()->urls()) {
        if (u.isLocalFile())
            paths << u.toLocalFile();
    }
    if (!paths.isEmpty()) {
        e->acceptProposedAction();
        emit dropped(paths);
    }
}

} // namespace Web
