#pragma once

#include <functional>

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QList>
#include <QPointer>
#include <QSet>
#include <QTimer>
#include <QWidget>

#include "webui.h"

class Connection;
class ConnectionManager;
class LanguageCombo;
class QCheckBox;
class QLabel;
class QLineEdit;
class QPushButton;
class QVBoxLayout;

// Translation memories, laid out as the web app's Memories page: those on
// each team server first, then the main and project memories on this
// computer, with memory files in (TMX, Trados Studio, Wordfast) and TMX
// out, search, new memories and who may use them.
class AlignmentDialog;

class MemoriesPage : public QWidget {
    Q_OBJECT
public:
    explicit MemoriesPage(ConnectionManager *manager, QWidget *parent = nullptr);
    ~MemoriesPage() override;

    // Reads everything again.
    void reload();
    // The memory opened on a team server, for its search and access.
    QString openMemory(const QString &connectionId) const;
    // Opens a memory of a team server; an empty name closes the open one.
    void open(const QString &connectionId, const QString &name);
    // The units the last search of a place's opened memory found.
    QJsonArray found(const QString &connectionId) const;
    // Searches the opened memory of a place.
    void search(const QString &connectionId, const QString &text, bool inTranslations = false);
    // The dialogs for one unit: a new one, or one found to be changed.
    Web::Dialog *addUnitDialog(const QString &connectionId);
    // A document and its translation, aligned into the open memory.
    AlignmentDialog *alignDialog(const QString &connectionId);
    // The same from the page: into a memory of that place chosen in the
    // dialog, or one made for the documents.
    AlignmentDialog *alignFromPage(const QString &connectionId);
    Web::Dialog *editUnitDialog(const QString &connectionId, const QJsonObject &unit);
    // Reads a memory file (TMX, Trados Studio or Wordfast) into a memory,
    // and writes a memory to a TMX file: what the buttons do once a file
    // is chosen.
    void importFile(const QString &connectionId, const QString &memory, const QString &path);
    void exportFile(const QString &connectionId, const QString &memory, const QString &path);
    // The memory file a new memory is filled from; its name and languages,
    // where it gives them, fill the form's that are still empty.
    void setNewMemoryFile(const QString &path);
    void notify(const QString &text);
    QString message() const;

signals:
    // Every place's memories were read and are shown.
    void loaded();
    // A search of the units was answered and its results are shown.
    void searched();

protected:
    void resizeEvent(QResizeEvent *e) override;

private:
    // The memories of one place, and the parts of the page that show them.
    struct Part {
        QPointer<Connection> conn;
        QWidget *root = nullptr;
        QLabel *problem = nullptr;
        // Local: the main memories and the project memories. Server: the one list.
        QVBoxLayout *list = nullptr;
        QWidget *ownSection = nullptr;
        QVBoxLayout *ownList = nullptr;
        QVBoxLayout *detail = nullptr;
        QVBoxLayout *formHolder = nullptr;
        QString open;
        QString openRole;
        // The units found by the last search of the opened memory, and how
        // to search again after one of them changed.
        // What the lists were drawn from, to leave them when nothing changed.
        QByteArray drawn;
        QJsonArray found;
        std::function<void()> searchAgain;
        QPointer<Web::ConfirmButton> openDelete;
        QJsonArray memories;
        QJsonArray projects;
        QJsonArray templates;
        bool read = false;
        int pending = 0;
        // Which reading the answers belong to, and whether to read again
        // once they are in.
        int reading = 0;
        bool again = false;
    };

    void syncParts();
    Part *part(const QString &connectionId) const;
    void load(Part *p);
    void finished(Part *p);
    void showPart(Part *p);
    void showLocal(Part *p);
    void showServer(Part *p);
    QWidget *localTable(Part *p, const QList<QJsonObject> &memories, bool main);
    void showDetail(Part *p);
    void localForm(Part *p, bool open);
    void serverForm(Part *p, bool open);
    QJsonObject memory(const Part *p, const QString &name) const;
    // The project a memory belongs to (its own memory), if any.
    QString ownerProject(const Part *p, const QString &memory) const;
    QStringList usedBy(const Part *p, const QString &memory) const;
    static QString units(qint64 n);
    static QString date(const QString &iso);
    // What an import says it did, as the browser client words it.
    static QString importNotice(const QJsonObject &st, const QString &memory, const QString &file);
    static void clear(QVBoxLayout *layout);

    ConnectionManager *m_manager;
    QVBoxLayout *m_sections;
    QList<Part *> m_parts;
    QSet<QString> m_listening;
    QTimer m_reload;
    int m_reading = 0;
    Web::Toast *m_toast;
    QString m_newFile;
    QPointer<QLabel> m_newFileLabel;
    QString m_newFormConnection;
    QPointer<QLineEdit> m_newName;
    QPointer<LanguageCombo> m_newSource, m_newTarget;
};
