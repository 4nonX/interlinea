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
    QVBoxLayout *m_saved;
    // Per connection: its templates and dictionaries, and whether the
    // person may change them there.
    QHash<QString, QJsonArray> m_templateList;
    QHash<QString, QJsonArray> m_dictionaryList;
    QList<Loose> m_loose;
    QStringList m_dictionaryProblems;
    QString m_states;
};
