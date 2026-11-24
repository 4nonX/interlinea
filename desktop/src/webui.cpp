#include "webui.h"

#include "icons.h"
#include "languages.h"
#include "theme.h"

#include <QAbstractButton>
#include <QDragEnterEvent>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QKeyEvent>
#include <QLabel>
#include <QMouseEvent>
#include <QMimeData>
#include <QPushButton>
#include <QScrollArea>
#include <QStyle>
#include <QTimer>
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
        b->setIconSize(QSize(16, 16));
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
        // A button or a choice is never narrower than what it says.
        QList<QWidget *> controls = w->findChildren<QWidget *>();
        controls.prepend(w);
        for (QWidget *c : controls) {
            if (qobject_cast<QAbstractButton *>(c) || c->inherits("QComboBox")) {
                c->ensurePolished();
                c->setMinimumWidth(c->sizeHint().width() + 4);
            }
        }
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
    // What the cells hold is styled before it is measured.
    for (int r = 0; r < rowCount(); ++r) {
        for (int c = 0; c < columnCount(); ++c) {
            if (QWidget *w = cellWidget(r, c)) {
                w->ensurePolished();
                for (QWidget *child : w->findChildren<QWidget *>())
                    child->ensurePolished();
                if (w->layout())
                    w->layout()->invalidate();
            }
        }
    }
    for (int c = 0; c < columnCount(); ++c) {
        if (horizontalHeader()->sectionResizeMode(c) != QHeaderView::Stretch)
            resizeColumnToContents(c);
    }
    resizeRowsToContents();
    updateGeometry();
    // Cells are styled for good only once they are shown, and may then be
    // larger than they said: measure once more after that.
    if (!m_refit) {
        m_refit = true;
        QTimer::singleShot(0, this, [this] {
            fit();
            m_refit = false;
        });
    }
}

void Table::showEvent(QShowEvent *e)
{
    QTableWidget::showEvent(e);
    fit();
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
    // Both lines keep their full height in a table row, which is measured
    // before the style has set the fonts.
    QFont small = f;
    small.setPixelSize(13);
    w->setMinimumHeight(QFontMetrics(f).lineSpacing() + 2 + (sub.isEmpty() ? 0 : 2 + QFontMetrics(small).lineSpacing() + 2));
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

QWidget *row(const QList<QWidget *> &widgets, QWidget *parent, int spacing)
{
    auto *w = new QWidget(parent);
    auto *h = new QHBoxLayout(w);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(spacing);
    for (QWidget *x : widgets)
        h->addWidget(x, 0, Qt::AlignVCenter);
    h->addStretch();
    return w;
}

QWidget *labelled(const QString &label, QWidget *control, QWidget *parent)
{
    auto *w = new QWidget(parent);
    auto *l = new QVBoxLayout(w);
    l->setContentsMargins(0, 0, 0, 0);
    l->setSpacing(S1);
    auto *t = text(label, "fieldLabel", w);
    t->setBuddy(control);
    if (control->accessibleName().isEmpty())
        control->setAccessibleName(label);
    l->addWidget(t);
    l->addWidget(control);
    return w;
}

ConfirmButton::ConfirmButton(const QString &label, const QString &question, const QString &action, QWidget *parent, bool small, const QString &icon)
    : QWidget(parent)
{
    auto *h = new QHBoxLayout(this);
    h->setContentsMargins(0, 0, 0, 0);
    h->setSpacing(S1);
    m_button = Web::button(label, small ? Quiet | Small : Quiet, this, icon);
    m_text = new QLabel(question, this);
    Theme::role(m_text, "confirmText");
    m_yes = Web::button(action, Small, this);
    m_yes->setProperty("danger", true);
    m_cancel = Web::button(QObject::tr("Cancel"), Quiet | Small, this);
    h->addWidget(m_button);
    h->addWidget(m_text);
    h->addWidget(m_yes);
    h->addWidget(m_cancel);
    h->addStretch();
    setQuestion(question);
    ask(false);
    connect(m_button, &QPushButton::clicked, this, [this] { ask(true); });
    connect(m_cancel, &QPushButton::clicked, this, [this] {
        ask(false);
        m_button->setFocus();
    });
    connect(m_yes, &QPushButton::clicked, this, [this] {
        ask(false);
        emit confirmed();
    });
}

void ConfirmButton::setQuestion(const QString &question)
{
    m_text->setText(question);
    m_yes->setAccessibleDescription(question);
}

void ConfirmButton::setAvailable(bool available)
{
    if (!available)
        ask(false);
    m_button->setEnabled(available);
}

bool ConfirmButton::asking() const
{
    return !m_yes->isHidden();
}

void ConfirmButton::ask(bool asking)
{
    m_button->setVisible(!asking);
    m_text->setVisible(asking);
    m_yes->setVisible(asking);
    m_cancel->setVisible(asking);
    if (asking)
        m_yes->setFocus();
    // A table cell holding it is as wide as what it shows now.
    for (QWidget *p = parentWidget(); p; p = p->parentWidget()) {
        if (auto *t = qobject_cast<Table *>(p)) {
            QMetaObject::invokeMethod(t, [t] { t->fit(); }, Qt::QueuedConnection);
            break;
        }
    }
}

Toast::Toast(QWidget *page)
    : QLabel(page)
    , m_timer(new QTimer(this))
{
    setAccessibleName(QObject::tr("Notice"));
    setStyleSheet(QStringLiteral("QLabel { background: #1f2630; color: #ffffff; border-radius: 8px; padding: 10px 16px; font-weight: 500; }"));
    setWordWrap(true);
    setMaximumWidth(560);
    hide();
    m_timer->setSingleShot(true);
    m_timer->setInterval(4000);
    connect(m_timer, &QTimer::timeout, this, &QWidget::hide);
}

void Toast::say(const QString &text)
{
    m_last = text;
    setText(text);
    // One line while it fits; wrapped at the widest it may be.
    const int oneLine = fontMetrics().horizontalAdvance(text) + 36;
    setWordWrap(oneLine > maximumWidth());
    if (wordWrap())
        resize(maximumWidth(), heightForWidth(maximumWidth()));
    else
        adjustSize();
    place();
    raise();
    show();
    m_timer->start();
}

void Toast::place()
{
    if (QWidget *p = parentWidget())
        move((p->width() - width()) / 2, p->height() - height() - S5);
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

Clickable::Clickable(const char *role, QWidget *parent)
    : QFrame(parent)
{
    Theme::role(this, role);
    setFocusPolicy(Qt::StrongFocus);
    setCursor(Qt::PointingHandCursor);
    setAttribute(Qt::WA_Hover);
    setProperty("current", false);
}

void Clickable::setCurrent(bool current)
{
    if (isCurrent() == current)
        return;
    setProperty("current", current);
    style()->unpolish(this);
    style()->polish(this);
}

void Clickable::mouseReleaseEvent(QMouseEvent *e)
{
    if (e->button() == Qt::LeftButton && rect().contains(e->pos()))
        emit clicked();
    QFrame::mouseReleaseEvent(e);
}

void Clickable::keyPressEvent(QKeyEvent *e)
{
    if (e->key() == Qt::Key_Return || e->key() == Qt::Key_Enter || e->key() == Qt::Key_Space) {
        emit clicked();
        return;
    }
    QFrame::keyPressEvent(e);
}

Tabs::Tabs(const QString &label, QWidget *parent, bool small)
    : QFrame(parent)
    , m_small(small)
{
    Theme::role(this, "tabs");
    setAccessibleName(label);
    m_row = new QHBoxLayout(this);
    m_row->setContentsMargins(S3, 0, S3, 0);
    m_row->setSpacing(S1);
    m_row->addStretch();
}

QPushButton *Tabs::add(const QString &text, const QString &icon)
{
    auto *b = new QPushButton(text, this);
    b->setProperty("tab", m_small ? QStringLiteral("small") : QStringLiteral("normal"));
    b->setCheckable(true);
    b->setAutoExclusive(false);
    b->setCursor(Qt::PointingHandCursor);
    if (!icon.isEmpty()) {
        b->setIcon(Icons::get(icon, Theme::color("text2")));
        b->setIconSize(QSize(14, 14));
    }
    const int index = m_tabs.size();
    m_tabs << b;
    b->setChecked(index == m_current);
    m_row->insertWidget(index, b);
    connect(b, &QPushButton::clicked, this, [this, index] {
        const bool was = m_current == index;
        setCurrent(index);
        if (!was)
            emit changed(index);
    });
    return b;
}

void Tabs::setText(int index, const QString &text)
{
    if (QPushButton *b = m_tabs.value(index))
        b->setText(text);
}

void Tabs::setEnabledAt(int index, bool enabled)
{
    if (QPushButton *b = m_tabs.value(index))
        b->setEnabled(enabled);
}

void Tabs::setCurrent(int index)
{
    m_current = index;
    for (int i = 0; i < m_tabs.size(); i++)
        m_tabs[i]->setChecked(i == index);
}

Segmented::Segmented(const QString &label, const QStringList &choices, QWidget *parent)
    : QFrame(parent)
{
    setObjectName(QStringLiteral("segmented"));
    setAccessibleName(label);
    setSizePolicy(QSizePolicy::Maximum, QSizePolicy::Fixed);
    auto *h = new QHBoxLayout(this);
    h->setContentsMargins(2, 2, 2, 2);
    h->setSpacing(0);
    for (int i = 0; i < choices.size(); i++) {
        auto *b = new QPushButton(choices[i], this);
        b->setProperty("segment", true);
        b->setCheckable(true);
        b->setChecked(i == 0);
        b->setCursor(Qt::PointingHandCursor);
        m_segments << b;
        h->addWidget(b);
        connect(b, &QPushButton::clicked, this, [this, i] {
            const bool was = m_current == i;
            setCurrent(i);
            if (!was)
                emit changed(i);
        });
    }
}

void Segmented::setCurrent(int index)
{
    m_current = index;
    for (int i = 0; i < m_segments.size(); i++)
        m_segments[i]->setChecked(i == index);
}

Stepper::Stepper(QWidget *parent)
    : QWidget(parent)
{
    setAccessibleName(tr("Steps"));
    m_row = new QHBoxLayout(this);
    m_row->setContentsMargins(0, 0, 0, 0);
    m_row->setSpacing(16);
}

void Stepper::setSteps(const QStringList &names, int current)
{
    m_names = names;
    m_current = current;
    while (QLayoutItem *item = m_row->takeAt(0)) {
        if (item->widget())
            item->widget()->deleteLater();
        delete item;
    }
    for (int i = 0; i < names.size(); i++) {
        auto *step = new QWidget(this);
        auto *h = new QHBoxLayout(step);
        h->setContentsMargins(0, 0, 0, 0);
        h->setSpacing(S2);
        auto *num = new QLabel(QString::number(i + 1), step);
        num->setAlignment(Qt::AlignCenter);
        num->setFixedSize(22, 22);
        const QString state = i == current ? QStringLiteral("current") : i < current ? QStringLiteral("done") : QStringLiteral("todo");
        num->setProperty("stepNum", state);
        auto *name = new QLabel(names[i], step);
        name->setProperty("stepName", state);
        h->addWidget(num);
        h->addWidget(name);
        step->setAccessibleName(i == current ? tr("Step %1, %2, current").arg(i + 1).arg(names[i]) : tr("Step %1, %2").arg(i + 1).arg(names[i]));
        m_row->addWidget(step);
    }
    m_row->addStretch();
}

Switcher::Switcher(QWidget *parent)
    : QWidget(parent)
{
    auto *v = new QVBoxLayout(this);
    v->setContentsMargins(0, 0, 0, 0);
    v->setSpacing(0);
}

void Switcher::add(QWidget *state)
{
    state->setParent(this);
    layout()->addWidget(state);
    state->setVisible(m_states.isEmpty());
    m_states << state;
}

void Switcher::setCurrent(int index)
{
    if (index == m_current || index < 0 || index >= m_states.size())
        return;
    m_current = index;
    for (int i = 0; i < m_states.size(); i++)
        m_states[i]->setVisible(i == index);
    emit changed(index);
}

QFrame *note(const QString &text, const QString &kind, QWidget *parent)
{
    auto *f = new QFrame(parent);
    f->setProperty("note", kind);
    auto *h = new QHBoxLayout(f);
    h->setContentsMargins(S3, S2, S3, S2);
    h->setSpacing(S2);
    QColor fg = Theme::color("text2");
    if (kind == QLatin1String("warn"))
        fg = Theme::dark() ? Theme::color("warn") : QColor(0x6b, 0x45, 0x00);
    else if (kind == QLatin1String("ok"))
        fg = Theme::dark() ? Theme::color("ok") : QColor(0x14, 0x5f, 0x36);
    auto *icon = new QLabel(f);
    icon->setPixmap(Icons::get(kind == QLatin1String("warn") ? QStringLiteral("alert-triangle") : kind == QLatin1String("ok") ? QStringLiteral("check") : QStringLiteral("info"), fg).pixmap(14, 14));
    icon->setAlignment(Qt::AlignTop);
    h->addWidget(icon, 0, Qt::AlignTop);
    auto *l = new QLabel(text, f);
    l->setWordWrap(true);
    l->setStyleSheet(QStringLiteral("color: %1; font-size: 13px; background: transparent;").arg(fg.name()));
    h->addWidget(l, 1);
    f->setAccessibleName(text);
    return f;
}

Dialog::Dialog(const QString &title, QWidget *parent, bool wide)
    : QDialog(parent)
{
    setProperty("role", QStringLiteral("dialog"));
    setWindowTitle(title);
    setModal(true);
    const int width = wide ? 920 : 640;
    setMinimumWidth(qMin(width, 480));
    resize(width, 10);
    m_outer = new QVBoxLayout(this);
    m_outer->setContentsMargins(S5, S5, S5, S5);
    m_outer->setSpacing(S3);
    m_title = new QLabel(title, this);
    m_title->setProperty("role", QStringLiteral("dialogTitle"));
    m_title->setWordWrap(true);
    m_outer->addWidget(m_title);
    m_body = new QVBoxLayout;
    m_body->setSpacing(S3);
    m_outer->addLayout(m_body);
    m_error = text(QString(), "error", this);
    m_error->setWordWrap(true);
    m_error->hide();
    m_outer->addWidget(m_error);
}

void Dialog::setTitle(const QString &title)
{
    setWindowTitle(title);
    m_title->setText(title);
}

QString Dialog::title() const
{
    return m_title->text();
}

QBoxLayout *Dialog::buttons()
{
    if (!m_buttons) {
        m_buttons = new QHBoxLayout;
        m_buttons->setSpacing(S2);
        m_buttons->addStretch();
        m_outer->addLayout(m_buttons);
    }
    return m_buttons;
}

void Dialog::setError(const QString &text)
{
    m_error->setText(text);
    m_error->setVisible(!text.isEmpty());
    if (!text.isEmpty())
        m_error->setAccessibleDescription(tr("Problem"));
}

QString Dialog::error() const
{
    return m_error->isHidden() ? QString() : m_error->text();
}

} // namespace Web
