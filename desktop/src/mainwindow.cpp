#include "mainwindow.h"

#include "connectionmanager.h"
#include "navigationrail.h"

#include <QAction>
#include <QCloseEvent>
#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
#include <QSettings>
#include <QShortcut>
#include <QStackedWidget>
#include <QStatusBar>
#include <QToolButton>
#include <QVBoxLayout>

static QString statusText(const Connection *c)
{
    switch (c->state()) {
    case Connection::State::Connecting:
        return MainWindow::tr("%1: connecting").arg(c->displayName());
    case Connection::State::Ready:
        return c->isLocal() ? c->displayName() : MainWindow::tr("%1: %2").arg(c->displayName(), c->userName());
    case Connection::State::SignedOut:
        return MainWindow::tr("%1: signed out").arg(c->displayName());
    case Connection::State::Offline:
        return MainWindow::tr("%1: can't be reached").arg(c->displayName());
    case Connection::State::Failed:
        return MainWindow::tr("%1: not working").arg(c->displayName());
    }
    return {};
}

MainWindow::MainWindow(ConnectionManager *manager, QWidget *parent)
    : QMainWindow(parent), m_manager(manager), m_rail(new NavigationRail(this)), m_stack(new QStackedWidget(this))
{
    setWindowTitle(QStringLiteral("Interlinea"));
    setObjectName(QStringLiteral("main"));
    auto *central = new QWidget(this);
    auto *outer = new QHBoxLayout(central);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->setSpacing(0);
    outer->addWidget(m_rail);
    auto *right = new QVBoxLayout;
    right->setContentsMargins(0, 0, 0, 0);

    // Something that stops work (the core not running) is said at the top.
    m_message = new QFrame(central);
    m_message->setFrameShape(QFrame::StyledPanel);
    m_message->setAccessibleName(tr("Message"));
    auto *ml = new QHBoxLayout(m_message);
    m_messageText = new QLabel(m_message);
    m_messageText->setWordWrap(true);
    m_messageText->setTextInteractionFlags(Qt::TextSelectableByMouse);
    m_messageAction = new QPushButton(tr("Try &again"), m_message);
    ml->addWidget(m_messageText, 1);
    ml->addWidget(m_messageAction);
    m_message->setVisible(false);
    connect(m_messageAction, &QPushButton::clicked, this, [this] { m_manager->core()->start(); });
    right->addWidget(m_message);
    right->addWidget(m_stack, 1);
    outer->addLayout(right, 1);
    setCentralWidget(central);

    connect(m_rail, &NavigationRail::pageChosen, this, &MainWindow::showPage);
    connect(manager, &ConnectionManager::changed, this, [this] { updateStatus(); updateMessage(); });
    connect(manager, &ConnectionManager::added, this, &MainWindow::updateStatus);
    connect(manager, &ConnectionManager::removed, this, &MainWindow::updateStatus);
    updateStatus();
    updateMessage();

    QSettings s;
    if (!restoreGeometry(s.value(QStringLiteral("window/geometry")).toByteArray()))
        resize(1280, 800);
}

void MainWindow::addPage(const QString &id, const QString &label, const QIcon &icon, QWidget *page, const QKeySequence &shortcut)
{
    m_pages.insert(id, page);
    m_stack->addWidget(page);
    m_rail->addPage(id, label, icon, shortcut.toString(QKeySequence::NativeText));
    if (!shortcut.isEmpty()) {
        auto *sc = new QShortcut(shortcut, this);
        connect(sc, &QShortcut::activated, this, [this, id] { showPage(id); });
    }
    if (m_pages.size() == 1)
        showPage(id);
}

void MainWindow::showPage(const QString &id)
{
    QWidget *page = m_pages.value(id);
    if (!page)
        return;
    m_stack->setCurrentWidget(page);
    m_rail->setCurrentPage(id);
    // Focus goes into the page, as the web app moves it to the heading.
    if (QWidget *w = page->nextInFocusChain(); w && page->isAncestorOf(w) && isVisible())
        w->setFocus(Qt::OtherFocusReason);
}

QString MainWindow::currentPage() const
{
    return m_pages.key(m_stack->currentWidget());
}

void MainWindow::updateStatus()
{
    QHash<QString, QToolButton *> keep;
    for (Connection *c : m_manager->all()) {
        QToolButton *b = m_status.take(c->id());
        if (!b) {
            b = new QToolButton(this);
            b->setAutoRaise(true);
            statusBar()->addPermanentWidget(b);
            connect(b, &QToolButton::clicked, this, [this] { showPage(QStringLiteral("settings")); });
        }
        b->setText(statusText(c));
        b->setToolTip(c->message().isEmpty() ? statusText(c) : c->message());
        b->setAccessibleName(statusText(c));
        keep.insert(c->id(), b);
    }
    for (QToolButton *b : std::as_const(m_status))
        b->deleteLater();
    m_status = keep;
}

void MainWindow::updateMessage()
{
    Connection *local = m_manager->local();
    const bool failed = local->state() == Connection::State::Failed;
    m_message->setVisible(failed);
    if (failed)
        m_messageText->setText(tr("Work on this computer can't be opened: %1").arg(local->message()));
}

void MainWindow::closeEvent(QCloseEvent *event)
{
    QSettings().setValue(QStringLiteral("window/geometry"), saveGeometry());
    QMainWindow::closeEvent(event);
}
