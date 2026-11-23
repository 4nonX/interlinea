#include "settingspage.h"

#include "apiclient.h"
#include "connectionmanager.h"
#include "secretstore.h"
#include "signindialog.h"

#include <QDesktopServices>
#include <QDir>
#include <QFormLayout>
#include <QGroupBox>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QLineEdit>
#include <QMessageBox>
#include <QPushButton>
#include <QSettings>
#include <QTreeWidget>
#include <QUrl>
#include <QVBoxLayout>

static QString stateText(const Connection *c)
{
    switch (c->state()) {
    case Connection::State::Connecting:
        return c->isLocal() ? SettingsPage::tr("Starting") : SettingsPage::tr("Connecting");
    case Connection::State::Ready:
        return SettingsPage::tr("Ready");
    case Connection::State::SignedOut:
        return SettingsPage::tr("Signed out");
    case Connection::State::Offline:
        return SettingsPage::tr("Can't be reached");
    case Connection::State::Failed:
        return SettingsPage::tr("Not working");
    }
    return {};
}

SettingsPage::SettingsPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent), m_manager(manager)
{
    setObjectName(QStringLiteral("settings"));
    auto *layout = new QVBoxLayout(this);
    auto *heading = new QLabel(tr("Settings"), this);
    QFont f = heading->font();
    f.setPointSizeF(f.pointSizeF() * 1.5);
    f.setWeight(QFont::DemiBold);
    heading->setFont(f);
    heading->setAccessibleName(tr("Settings"));
    layout->addWidget(heading);

    // Where work is kept.
    auto *where = new QGroupBox(tr("Where your work is kept"), this);
    auto *wl = new QVBoxLayout(where);
    auto *intro = new QLabel(tr("Work on this computer needs no server. Team servers keep a team's projects, memories and termbases."), where);
    intro->setWordWrap(true);
    wl->addWidget(intro);
    m_list = new QTreeWidget(where);
    m_list->setObjectName(QStringLiteral("connections"));
    m_list->setAccessibleName(tr("Places your work is kept"));
    m_list->setColumnCount(3);
    m_list->setHeaderLabels({tr("Place"), tr("Signed in as"), tr("State")});
    m_list->setRootIsDecorated(false);
    m_list->setUniformRowHeights(true);
    m_list->header()->setSectionResizeMode(0, QHeaderView::Stretch);
    m_list->setMinimumHeight(120);
    wl->addWidget(m_list);
    m_details = new QLabel(where);
    m_details->setWordWrap(true);
    m_details->setTextInteractionFlags(Qt::TextSelectableByMouse | Qt::TextSelectableByKeyboard);
    m_details->setAccessibleName(tr("Details"));
    wl->addWidget(m_details);
    auto *buttons = new QHBoxLayout;
    m_add = new QPushButton(tr("&Add team server…"), where);
    m_signIn = new QPushButton(tr("Sign &in…"), where);
    m_signOut = new QPushButton(tr("Sign &out"), where);
    m_remove = new QPushButton(tr("&Remove"), where);
    m_openData = new QPushButton(tr("Open data &folder"), where);
    m_restart = new QPushButton(tr("Start the core &again"), where);
    for (QPushButton *b : {m_add, m_signIn, m_signOut, m_remove, m_openData, m_restart})
        buttons->addWidget(b);
    buttons->addStretch();
    wl->addLayout(buttons);
    m_secrets = new QLabel(where);
    m_secrets->setWordWrap(true);
    wl->addWidget(m_secrets);
    layout->addWidget(where);

    // The person.
    auto *you = new QGroupBox(tr("You"), this);
    auto *yl = new QFormLayout(you);
    m_name = new QLineEdit(you);
    m_name->setAccessibleName(tr("Your name"));
    auto *nl = new QLabel(tr("Your &name:"), you);
    nl->setBuddy(m_name);
    auto *nameRow = new QHBoxLayout;
    nameRow->addWidget(m_name);
    m_saveName = new QPushButton(tr("Sa&ve name"), you);
    nameRow->addWidget(m_saveName);
    yl->addRow(nl, nameRow);
    m_nameStatus = new QLabel(you);
    yl->addRow(QString(), m_nameStatus);
    auto *nameHelp = new QLabel(tr("Your work on this computer is signed with it, in histories and when you send work to a team."), you);
    nameHelp->setWordWrap(true);
    yl->addRow(QString(), nameHelp);
    layout->addWidget(you);

    // The app.
    auto *about = new QGroupBox(tr("About Interlinea"), this);
    auto *al = new QVBoxLayout(about);
    auto *version = new QLabel(tr("Interlinea %1. Open source: a translation workspace with its editor, terminology, memories and projects.")
                                   .arg(QStringLiteral(INTERLINEA_VERSION)),
                               about);
    version->setWordWrap(true);
    al->addWidget(version);
    layout->addWidget(about);
    layout->addStretch();

    connect(m_list, &QTreeWidget::currentItemChanged, this, &SettingsPage::updateDetails);
    connect(m_add, &QPushButton::clicked, this, &SettingsPage::addServer);
    connect(m_signIn, &QPushButton::clicked, this, &SettingsPage::signInAgain);
    connect(m_signOut, &QPushButton::clicked, this, &SettingsPage::signOut);
    connect(m_remove, &QPushButton::clicked, this, &SettingsPage::removeServer);
    connect(m_openData, &QPushButton::clicked, this, [this] {
        QDesktopServices::openUrl(QUrl::fromLocalFile(m_manager->core()->dataDir()));
    });
    connect(m_restart, &QPushButton::clicked, this, [this] { m_manager->core()->start(); });
    connect(m_saveName, &QPushButton::clicked, this, &SettingsPage::saveName);
    connect(m_name, &QLineEdit::returnPressed, this, &SettingsPage::saveName);
    connect(manager, &ConnectionManager::added, this, &SettingsPage::refresh);
    connect(manager, &ConnectionManager::removed, this, &SettingsPage::refresh);
    connect(manager, &ConnectionManager::changed, this, &SettingsPage::refresh);
    refresh();
}

void SettingsPage::refresh()
{
    const QString keep = selected() ? selected()->id() : QStringLiteral("local");
    m_list->clear();
    for (Connection *c : m_manager->all()) {
        auto *item = new QTreeWidgetItem(m_list, {c->displayName(), c->isReady() ? c->userName() : QString(), stateText(c)});
        item->setData(0, Qt::UserRole, c->id());
        if (c->id() == keep)
            m_list->setCurrentItem(item);
    }
    if (!m_list->currentItem() && m_list->topLevelItemCount() > 0)
        m_list->setCurrentItem(m_list->topLevelItem(0));
    Connection *local = m_manager->local();
    if (!m_name->hasFocus() && local->isReady())
        m_name->setText(local->user().value(QLatin1String("name")).toString());
    m_name->setEnabled(local->isReady());
    m_saveName->setEnabled(local->isReady());
    updateDetails();
}

Connection *SettingsPage::selected() const
{
    QTreeWidgetItem *item = m_list->currentItem();
    return item ? m_manager->find(item->data(0, Qt::UserRole).toString()) : nullptr;
}

void SettingsPage::updateDetails()
{
    Connection *c = selected();
    const bool team = c && !c->isLocal();
    m_signIn->setVisible(team);
    m_signOut->setVisible(team);
    m_remove->setVisible(team);
    m_openData->setVisible(c && c->isLocal());
    m_restart->setVisible(c && c->isLocal() && c->state() == Connection::State::Failed);
    m_signIn->setEnabled(team && c->state() != Connection::State::Ready);
    m_signOut->setEnabled(team && c->state() != Connection::State::SignedOut);
    if (!c) {
        m_details->clear();
        return;
    }
    QStringList lines;
    if (c->isLocal()) {
        lines << tr("Your work on this computer is kept in %1.").arg(QDir::toNativeSeparators(m_manager->core()->dataDir()));
        if (!c->serverVersion().isEmpty())
            lines << tr("Core %1.").arg(c->serverVersion());
    } else {
        lines << c->url().toString();
        if (!c->serverVersion().isEmpty())
            lines << tr("Interlinea %1 on the server.").arg(c->serverVersion());
        if (c->isReady())
            lines << (c->isAdmin() ? tr("You are an administrator there.") : tr("Signed in as %1.").arg(c->userName()));
    }
    if (!c->message().isEmpty())
        lines << c->message();
    m_details->setText(lines.join(QLatin1Char('\n')));
    m_secrets->setText(m_manager->teams().isEmpty()
                           ? QString()
                           : (SecretStore::secure() ? tr("Sign-ins to team servers are kept in %1.").arg(SecretStore::where())
                                                    : tr("Sign-ins to team servers are kept in %1, because the keychain can't be used: %2")
                                                          .arg(SecretStore::where(), SecretStore::fallbackReason())));
}

void SettingsPage::addServer()
{
    SignInDialog dlg(m_manager, QUrl(), this);
    dlg.exec();
}

void SettingsPage::signInAgain()
{
    Connection *c = selected();
    if (!c || c->isLocal())
        return;
    SignInDialog dlg(m_manager, c->url(), this);
    dlg.exec();
}

void SettingsPage::signOut()
{
    if (Connection *c = selected())
        m_manager->signOut(c);
}

void SettingsPage::removeServer()
{
    Connection *c = selected();
    if (!c || c->isLocal())
        return;
    const auto answer = QMessageBox::question(this, tr("Remove the team server?"),
                                              tr("Interlinea signs out of %1 and forgets it. Your work on the server stays there.").arg(c->displayName()),
                                              QMessageBox::Yes | QMessageBox::Cancel, QMessageBox::Cancel);
    if (answer == QMessageBox::Yes)
        m_manager->remove(c);
}

void SettingsPage::saveName()
{
    Connection *local = m_manager->local();
    const QString name = m_name->text().trimmed();
    if (!local->isReady() || name.isEmpty())
        return;
    const QJsonObject user = local->user();
    const qint64 id = user.value(QLatin1String("id")).toInteger();
    m_nameStatus->setText(tr("Saving…"));
    onResult(local->api()->patch(QStringLiteral("/api/users/%1").arg(id), QJsonObject{{QStringLiteral("name"), name}},
                                 user.value(QLatin1String("version")).toInt()),
             this, [this, local](const ApiResult &r) {
                 if (!r.ok()) {
                     m_nameStatus->setText(r.error.message);
                     return;
                 }
                 local->setUser(r.object());
                 // The core is started with it next time.
                 QSettings().setValue(QStringLiteral("you/name"), r.object().value(QLatin1String("name")).toString());
                 m_nameStatus->setText(tr("Saved."));
             });
}
