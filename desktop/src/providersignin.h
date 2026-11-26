#pragma once

#include <QObject>
#include <QPointer>
#include <QTimer>
#include <QUrl>

class QTcpServer;
class QTcpSocket;

// Receives the result of a sign-in through a team server's identity
// provider. The provider and then the server send the browser to an
// address on this computer; the result is in the address's fragment,
// which browsers don't send, so the page served there hands it over with a
// second request.
class ProviderSignIn : public QObject {
    Q_OBJECT
public:
    explicit ProviderSignIn(QObject *parent = nullptr);
    ~ProviderSignIn() override;

    // Starts listening; returnTo() is then the address to give the server.
    bool listen(QString *error = nullptr);
    QUrl returnTo() const;
    void close();

signals:
    // A one-time code to exchange for a sign-in (#interlinea-code=).
    void code(const QString &code);
    // A one-time code that links a provider sign-in to the account once
    // the account confirms it (#interlinea-link=).
    void linkCode(const QString &code);
    void failed(const QString &message);

private:
    void onConnection();
    void answer(QTcpSocket *s);

    QTcpServer *m_server = nullptr;
    QTimer m_timeout;
    bool m_done = false;
};
