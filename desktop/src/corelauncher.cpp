#include "corelauncher.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QJsonDocument>
#include <QStandardPaths>


CoreLauncher::CoreLauncher(const QString &program, const QString &dataDir, const QString &personName, QObject *parent)
    : QObject(parent), m_program(program), m_dataDir(dataDir), m_name(personName)
{
    m_timeout.setSingleShot(true);
    m_timeout.setInterval(30 * 1000);
    connect(&m_timeout, &QTimer::timeout, this, [this] {
        fail(tr("The core did not start within 30 seconds."));
        if (m_proc)
            m_proc->kill();
    });
}

CoreLauncher::~CoreLauncher()
{
    stop();
}

QString CoreLauncher::defaultProgram()
{
    const QString forced = qEnvironmentVariable("INTERLINEA_CORE");
    if (!forced.isEmpty())
        return forced;
#ifdef Q_OS_WIN
    const QString name = QStringLiteral("interlinea-core.exe");
#else
    const QString name = QStringLiteral("interlinea-core");
#endif
    return QDir(QCoreApplication::applicationDirPath()).filePath(name);
}

QString CoreLauncher::defaultDataDir()
{
    const QString forced = qEnvironmentVariable("INTERLINEA_DATA");
    if (!forced.isEmpty())
        return forced;
    return QStandardPaths::writableLocation(QStandardPaths::AppDataLocation) + QStringLiteral("/data");
}

void CoreLauncher::start()
{
    if (m_state == State::Starting || m_state == State::Running)
        return;
    m_restarts.clear();
    m_stopping = false;
    launch();
}

void CoreLauncher::launch()
{
    if (!QFileInfo(m_program).isExecutable()) {
        fail(tr("The core (%1) is missing. Reinstall Interlinea.").arg(QDir::toNativeSeparators(m_program)));
        return;
    }
    QDir().mkpath(m_dataDir);
    m_out.clear();
    m_error.clear();
    m_proc = new QProcess(this);
    m_proc->setProgram(m_program);
    m_proc->setArguments({QStringLiteral("-data"), m_dataDir, QStringLiteral("-name"), m_name});
    m_proc->setProcessChannelMode(QProcess::SeparateChannels);
    connect(m_proc, &QProcess::readyReadStandardOutput, this, &CoreLauncher::onStdout);
    connect(m_proc, &QProcess::readyReadStandardError, this, &CoreLauncher::onStderr);
    connect(m_proc, &QProcess::finished, this, &CoreLauncher::onFinished);
    connect(m_proc, &QProcess::errorOccurred, this, &CoreLauncher::onError);
    setState(State::Starting);
    m_timeout.start();
    m_proc->start();
}

void CoreLauncher::stop()
{
    m_stopping = true;
    m_timeout.stop();
    if (!m_proc)
        return;
    QProcess *p = m_proc;
    m_proc = nullptr;
    p->disconnect(this);
    if (p->state() != QProcess::NotRunning) {
        // Closing its input is how the core is asked to stop.
        p->closeWriteChannel();
        if (!p->waitForFinished(8000)) {
            p->kill();
            p->waitForFinished(2000);
        }
    }
    p->deleteLater();
    setState(State::Stopped);
}

void CoreLauncher::onStdout()
{
    if (!m_proc)
        return;
    m_out += m_proc->readAllStandardOutput();
    if (m_state != State::Starting)
        return;
    const qsizetype nl = m_out.indexOf('\n');
    if (nl < 0)
        return;
    QJsonParseError pe;
    const QJsonObject o = QJsonDocument::fromJson(m_out.left(nl), &pe).object();
    if (pe.error != QJsonParseError::NoError || !o.contains(QLatin1String("url"))) {
        fail(tr("The core's first answer could not be read."));
        m_proc->kill();
        return;
    }
    m_timeout.stop();
    m_handshake = {QUrl(o.value(QLatin1String("url")).toString()), o.value(QLatin1String("token")).toString(),
                   o.value(QLatin1String("version")).toString(), o.value(QLatin1String("user")).toObject()};
    setState(State::Running);
    emit started(m_handshake);
}

void CoreLauncher::onStderr()
{
    if (!m_proc)
        return;
    const QStringList lines = QString::fromUtf8(m_proc->readAllStandardError()).split(QLatin1Char('\n'), Qt::SkipEmptyParts);
    m_log += lines;
    while (m_log.size() > 200)
        m_log.removeFirst();
}

void CoreLauncher::onFinished(int code, QProcess::ExitStatus status)
{
    QProcess *p = m_proc;
    m_proc = nullptr;
    if (p)
        p->deleteLater();
    m_timeout.stop();
    if (m_stopping)
        return;
    const QString why = status == QProcess::CrashExit
        ? tr("The core stopped unexpectedly.")
        : tr("The core ended with code %1.").arg(code);
    const QString last = m_log.isEmpty() ? QString() : m_log.constLast();
    if (m_state == State::Failed)
        return;
    // Started again unless it keeps failing.
    const QDateTime now = QDateTime::currentDateTimeUtc();
    while (!m_restarts.isEmpty() && m_restarts.first().secsTo(now) > 60)
        m_restarts.removeFirst();
    if (m_state == State::Running && m_restarts.size() < 3) {
        m_restarts.append(now);
        emit restarting(why);
        launch();
        return;
    }
    fail(last.isEmpty() ? why : why + QLatin1Char(' ') + last);
}

void CoreLauncher::onError(QProcess::ProcessError error)
{
    if (error == QProcess::FailedToStart)
        fail(tr("The core could not be started: %1").arg(m_proc ? m_proc->errorString() : QString()));
}

void CoreLauncher::fail(const QString &why)
{
    if (m_state == State::Failed)
        return;
    m_error = why;
    setState(State::Failed);
    emit failed(why);
}

void CoreLauncher::setState(State s)
{
    if (m_state == s)
        return;
    m_state = s;
    emit stateChanged(s);
}
