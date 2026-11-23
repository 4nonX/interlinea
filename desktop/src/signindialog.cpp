#include "signindialog.h"

#include <QDialogButtonBox>
#include <QFormLayout>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QStackedWidget>
#include <QVBoxLayout>

static QLabel *errorLabel(QWidget *parent)
{
    auto *l = new QLabel(parent);
    l->setWordWrap(true);
    l->setTextFormat(Qt::PlainText);
    l->setObjectName(QStringLiteral("error"));
    QPalette p = l->palette();
    p.setColor(QPalette::WindowText, QColor(0xaa, 0x22, 0x19));
    l->setPalette(p);
    l->setVisible(false);
    return l;
}

SignInDialog::SignInDialog(ConnectionManager *manager, const QUrl &server, QWidget *parent)
    : QDialog(parent), m_manager(manager), m_stack(new QStackedWidget(this)), m_status(new QLabel(this))
{
    setWindowTitle(tr("Sign in to a team server"));
    setMinimumWidth(460);
    auto *layout = new QVBoxLayout(this);
    layout->addWidget(m_stack);
    m_status->setWordWrap(true);
    m_status->setAccessibleName(tr("Status"));
    layout->addWidget(m_status);
    auto *buttons = new QDialogButtonBox(QDialogButtonBox::Cancel, this);
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::reject);
    layout->addWidget(buttons);

    buildAddressPage();
    buildSignInPage();
    buildSetupPage();
    m_stack->setCurrentIndex(AddressPage);
    if (server.isValid()) {
        m_address->setText(server.toString());
        lookUp();
    }
}

SignInDialog::~SignInDialog()
{
    m_manager->cancelProviderSignIn();
}

void SignInDialog::buildAddressPage()
{
    auto *page = new QWidget(m_stack);
    auto *form = new QFormLayout(page);
    auto *intro = new QLabel(tr("A team server keeps projects, memories and termbases for a team. Ask its administrator for its address."), page);
    intro->setWordWrap(true);
    form->addRow(intro);
    m_address = new QLineEdit(page);
    m_address->setPlaceholderText(QStringLiteral("interlinea.example.com"));
    m_address->setInputMethodHints(Qt::ImhUrlCharactersOnly);
    m_address->setAccessibleName(tr("Server address"));
    auto *label = new QLabel(tr("Server &address:"), page);
    label->setBuddy(m_address);
    form->addRow(label, m_address);
    m_addressError = errorLabel(page);
    form->addRow(m_addressError);
    m_next = new QPushButton(tr("&Continue"), page);
    m_next->setDefault(true);
    form->addRow(QString(), m_next);
    connect(m_next, &QPushButton::clicked, this, &SignInDialog::lookUp);
    connect(m_address, &QLineEdit::returnPressed, this, &SignInDialog::lookUp);
    m_stack->addWidget(page);
}

void SignInDialog::buildSignInPage()
{
    auto *page = new QWidget(m_stack);
    auto *form = new QFormLayout(page);
    m_serverLabel = new QLabel(page);
    m_serverLabel->setWordWrap(true);
    form->addRow(m_serverLabel);
    m_provider = new QPushButton(page);
    form->addRow(m_provider);
    connect(m_provider, &QPushButton::clicked, this, &SignInDialog::signInWithProvider);
    m_username = new QLineEdit(page);
    m_username->setAccessibleName(tr("Username"));
    auto *ul = new QLabel(tr("&Username:"), page);
    ul->setBuddy(m_username);
    form->addRow(ul, m_username);
    m_password = new QLineEdit(page);
    m_password->setEchoMode(QLineEdit::Password);
    m_password->setAccessibleName(tr("Password"));
    auto *pl = new QLabel(tr("&Password:"), page);
    pl->setBuddy(m_password);
    form->addRow(pl, m_password);
    m_signInError = errorLabel(page);
    form->addRow(m_signInError);
    m_signIn = new QPushButton(tr("&Sign in"), page);
    form->addRow(QString(), m_signIn);
    connect(m_signIn, &QPushButton::clicked, this, &SignInDialog::signIn);
    connect(m_password, &QLineEdit::returnPressed, this, &SignInDialog::signIn);
    auto *back = new QPushButton(tr("Other &server"), page);
    back->setFlat(true);
    connect(back, &QPushButton::clicked, this, [this] { m_stack->setCurrentIndex(AddressPage); m_address->setFocus(); });
    form->addRow(QString(), back);
    m_stack->addWidget(page);
}

void SignInDialog::buildSetupPage()
{
    auto *page = new QWidget(m_stack);
    auto *form = new QFormLayout(page);
    auto *intro = new QLabel(tr("This server has no accounts yet. Make its first administrator with the setup token from the server's log."), page);
    intro->setWordWrap(true);
    form->addRow(intro);
    const auto field = [&](QLineEdit *&edit, const QString &label, const QString &name, bool secret = false) {
        edit = new QLineEdit(page);
        edit->setAccessibleName(name);
        if (secret)
            edit->setEchoMode(QLineEdit::Password);
        auto *l = new QLabel(label, page);
        l->setBuddy(edit);
        form->addRow(l, edit);
    };
    field(m_setupToken, tr("Setup &token:"), tr("Setup token"), true);
    field(m_adminUser, tr("&Username:"), tr("Username"));
    field(m_adminName, tr("&Name:"), tr("Name"));
    field(m_adminEmail, tr("&Email:"), tr("Email"));
    field(m_adminPassword, tr("&Password:"), tr("Password"), true);
    m_adminPassword->setToolTip(tr("12 characters or more."));
    m_setupError = errorLabel(page);
    form->addRow(m_setupError);
    m_setUp = new QPushButton(tr("&Make the administrator"), page);
    form->addRow(QString(), m_setUp);
    connect(m_setUp, &QPushButton::clicked, this, &SignInDialog::setUp);
    m_stack->addWidget(page);
}

void SignInDialog::showError(QLabel *label, const QString &message)
{
    label->setText(message);
    label->setVisible(!message.isEmpty());
    if (!message.isEmpty())
        m_status->setText(message); // read by screen readers with the status
}

void SignInDialog::setBusy(bool busy, const QString &what)
{
    m_next->setEnabled(!busy);
    m_signIn->setEnabled(!busy);
    m_provider->setEnabled(!busy);
    m_setUp->setEnabled(!busy);
    m_status->setText(what);
}

void SignInDialog::lookUp()
{
    showError(m_addressError, {});
    const QUrl url = ConnectionManager::serverUrl(m_address->text());
    if (!url.isValid()) {
        showError(m_addressError, tr("Type the server's address, such as interlinea.example.com."));
        m_address->setFocus();
        return;
    }
    setBusy(true, tr("Looking up %1…").arg(url.host()));
    m_manager->probe(url, [this, url](const ServerInfo &info, const ApiError &e) {
        setBusy(false);
        if (e.isError()) {
            showError(m_addressError, e.message);
            m_address->setFocus();
            return;
        }
        m_info = info;
        if (info.setup) {
            m_stack->setCurrentIndex(SetupPage);
            m_setupToken->setFocus();
            return;
        }
        m_serverLabel->setText(tr("Sign in to %1 (Interlinea %2).").arg(url.host(), info.version));
        m_provider->setVisible(!info.provider.isEmpty());
        m_provider->setText(tr("Sign in with %1").arg(info.provider));
        m_stack->setCurrentIndex(SignInPage);
        if (!info.provider.isEmpty())
            m_provider->setFocus();
        else
            m_username->setFocus();
    });
}

void SignInDialog::finish(Connection *c, const ApiError &e, QLabel *label)
{
    setBusy(false);
    if (!c) {
        showError(label, e.problems.isEmpty() ? e.message : e.problems.first().message);
        return;
    }
    m_connection = c;
    if (e.isError()) {
        // Signed in, with something to say (the sign-in not kept).
        showError(label, e.message);
    }
    accept();
}

void SignInDialog::signIn()
{
    showError(m_signInError, {});
    if (m_username->text().trimmed().isEmpty() || m_password->text().isEmpty()) {
        showError(m_signInError, tr("Type your username and password."));
        (m_username->text().trimmed().isEmpty() ? m_username : m_password)->setFocus();
        return;
    }
    setBusy(true, tr("Signing in…"));
    m_manager->signInWithPassword(m_info.url, m_username->text().trimmed(), m_password->text(), [this](Connection *c, const ApiError &e) {
        m_password->clear();
        finish(c, e, m_signInError);
    });
}

void SignInDialog::signInWithProvider()
{
    showError(m_signInError, {});
    setBusy(true, tr("Finish signing in in your browser."));
    m_manager->signInWithProvider(m_info.url, [this](Connection *c, const ApiError &e) { finish(c, e, m_signInError); });
}

void SignInDialog::setUp()
{
    showError(m_setupError, {});
    setBusy(true, tr("Making the administrator…"));
    m_manager->setUpServer(m_info.url, m_setupToken->text().trimmed(), m_adminUser->text().trimmed(), m_adminName->text().trimmed(),
                           m_adminEmail->text().trimmed(), m_adminPassword->text(), [this](Connection *c, const ApiError &e) {
                               m_adminPassword->clear();
                               finish(c, e, m_setupError);
                           });
}
