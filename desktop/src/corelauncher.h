#pragma once

#include <QDateTime>
#include <QJsonObject>
#include <QObject>
#include <QProcess>
#include <QStringList>
#include <QTimer>
#include <QUrl>

// Starts and watches interlinea-core, the app's own server for the work on
// this computer. The core prints where it listens and a token on its first
// line, and stops when the app closes its standard input, so it ends with
// the app even if the app is killed. A core that stops unexpectedly is
// started again, up to three times a minute.
class CoreLauncher : public QObject {
    Q_OBJECT
public:
    struct Handshake {
        QUrl url;
        QString token;
        QString version;
        QJsonObject user;
    };
    enum class State { Stopped, Starting, Running, Failed };
    Q_ENUM(State)

    CoreLauncher(const QString &program, const QString &dataDir, const QString &personName, QObject *parent = nullptr);
    ~CoreLauncher() override;

    // Next to the app (INTERLINEA_CORE overrides it).
    static QString defaultProgram();
    // The app's data folder (INTERLINEA_DATA overrides it).
    static QString defaultDataDir();

    void start();
    // Stops the core and waits for it, at most a few seconds.
    void stop();

    State state() const { return m_state; }
    Handshake handshake() const { return m_handshake; }
    QString program() const { return m_program; }
    QString dataDir() const { return m_dataDir; }
    QString errorText() const { return m_error; }
    // The core's last lines of output, for a report.
    QStringList log() const { return m_log; }

signals:
    void started(const CoreLauncher::Handshake &handshake);
    // The core stopped unexpectedly and is being started again.
    void restarting(const QString &why);
    void failed(const QString &why);
    void stateChanged(CoreLauncher::State state);

private:
    void launch();
    void onStdout();
    void onStderr();
    void onFinished(int code, QProcess::ExitStatus status);
    void onError(QProcess::ProcessError error);
    void fail(const QString &why);
    void setState(State s);

    QString m_program;
    QString m_dataDir;
    QString m_name;
    QProcess *m_proc = nullptr;
    QTimer m_timeout;
    QByteArray m_out;
    QStringList m_log;
    QList<QDateTime> m_restarts;
    Handshake m_handshake;
    State m_state = State::Stopped;
    QString m_error;
    bool m_stopping = false;
};
