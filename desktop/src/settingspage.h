#pragma once

#include <QJsonArray>
#include <QHash>
#include <QWidget>

class Connection;
class ConnectionManager;
class QLabel;
class QLineEdit;
class QVBoxLayout;

namespace Web {
class Toast;
}

// Settings, laid out as the web's: the person's name, the wording of the
// terminology, the editor, project templates, spelling dictionaries, the
// team servers, and where the work on this computer is kept.
class SettingsPage : public QWidget {
    Q_OBJECT
public:
    explicit SettingsPage(ConnectionManager *manager, QWidget *parent = nullptr);

    QString lastNotice() const;

    // Adds a translation service at a place, and makes an access token
    // for a program there, as the forms do.
    // Shows the licence and the copyright notice that come with the app.
    void showLicence();
    // kind is "translate" for a translation service and "check" for a
    // grammar service.
    void saveProvider(const QString &connectionId, const QString &name, const QString &url, const QString &headerName, const QString &headerValue,
                      const QString &kind = QStringLiteral("translate"));
    // Saves a service with all it is set up with: name, kind, service
    // ("own" or one built in), url, headerName, headerValue (the key),
    // model and region.
    void saveProvider(const QString &connectionId, const QJsonObject &fields);
    void makeToken(const QString &connectionId, const QString &name, int days);
    QString madeToken() const { return m_madeToken; }
    // Reads what the panels show again.
    void reloadNow() { reload(); }

protected:
    void resizeEvent(QResizeEvent *e) override;
    void showEvent(QShowEvent *e) override;

private:
    // A dictionary chosen as its two files, whose language is asked for
    // when the files do not say it.
    struct Loose {
        QString connectionId;
        QString name;
        QByteArray aff;
        QByteArray dic;
        QString lang;
    };
    void refresh();
    void reload();
    void fillTemplates();
    void fillDictionaries();
    void fillProviders();
    void fillServices(QVBoxLayout *into, const QString &kind);
    QWidget *serviceForm(const QString &connectionId, const QString &kind, QWidget *parent);
    QString serviceWhere(const QJsonObject &service) const;
    void fillPrograms();
    void fillServers();
    void fillSaved();
    void saveName();
    void addDictionaries(Connection *c);
    void sendLoose(int index);
    void say(const QString &message);
    QList<Connection *> ready() const;

    ConnectionManager *m_manager;
    Web::Toast *m_toast;
    QLineEdit *m_name;
    QLabel *m_nameStatus;
    QVBoxLayout *m_templates;
    QVBoxLayout *m_dictionaries;
    QVBoxLayout *m_servers;
    QVBoxLayout *m_providers = nullptr;
    QVBoxLayout *m_checkers = nullptr;
    QVBoxLayout *m_programs = nullptr;
    QVBoxLayout *m_saved;
    // Per connection: its templates and dictionaries, and whether the
    // person may change them there.
    QHash<QString, QJsonArray> m_templateList;
    QHash<QString, QJsonArray> m_dictionaryList;
    QHash<QString, QJsonArray> m_providerList;
    QHash<QString, QJsonArray> m_tokenList;
    // The form for a new service is open at this place; the token just
    // made, shown once, and where.
    QString m_providerFormAt;
    QString m_providerFormKind;
    // The services built in, as the core lists them, and the one the
    // form for a translation service is set up for.
    QJsonArray m_services;
    QString m_providerService;
    QString m_madeToken;
    QString m_madeTokenName;
    QString m_madeTokenAt;
    QList<Loose> m_loose;
    QStringList m_dictionaryProblems;
    QString m_states;
};
