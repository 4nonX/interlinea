#pragma once

#include <QObject>
#include <QStringList>

class QLocalServer;

// Keeps one Interlinea running per person: a second start hands its
// arguments (files to open) to the first and ends.
class SingleInstance : public QObject {
    Q_OBJECT
public:
    explicit SingleInstance(const QString &key, QObject *parent = nullptr);
    ~SingleInstance() override;

    // True when this is the first instance, which then listens for later
    // ones. False when another instance was found and told about args.
    bool claim(const QStringList &args);

signals:
    // A later start asked for the window, with its arguments.
    void activated(const QStringList &args);

private:
    QString m_key;
    QLocalServer *m_server = nullptr;
};
