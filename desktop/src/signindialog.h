#pragma once

#include "connectionmanager.h"

#include <QDialog>

class QLabel;
class QLineEdit;
class QPushButton;
class QStackedWidget;

// Signs in to a team server: its address first, then a password or the
// server's identity provider, or, on a new server, making its first
// administrator with the setup token.
class SignInDialog : public QDialog {
    Q_OBJECT
public:
    // A known server (address filled in) or a new one.
    SignInDialog(ConnectionManager *manager, const QUrl &server = {}, QWidget *parent = nullptr);
    ~SignInDialog() override;

    Connection *connection() const { return m_connection; }

private:
    enum Page { AddressPage, SignInPage, SetupPage };
    void buildAddressPage();
    void buildSignInPage();
    void buildSetupPage();
    void lookUp();
    void signIn();
    void signInWithProvider();
    void setUp();
    void finish(Connection *c, const ApiError &e, QLabel *errorLabel);
    void setBusy(bool busy, const QString &what = {});
    void showError(QLabel *label, const QString &message);

    ConnectionManager *m_manager;
    ServerInfo m_info;
    Connection *m_connection = nullptr;

    QStackedWidget *m_stack;
    QLabel *m_status;
    // Address page.
    QLineEdit *m_address;
    QLabel *m_addressError;
    QPushButton *m_next;
    // Sign-in page.
    QLabel *m_serverLabel;
    QLineEdit *m_username;
    QLineEdit *m_password;
    QLabel *m_signInError;
    QPushButton *m_signIn;
    QPushButton *m_provider;
    // Setup page.
    QLineEdit *m_setupToken;
    QLineEdit *m_adminUser;
    QLineEdit *m_adminName;
    QLineEdit *m_adminEmail;
    QLineEdit *m_adminPassword;
    QLabel *m_setupError;
    QPushButton *m_setUp;
};
