#pragma once

#include <QObject>
#include <QSet>
#include <QPointer>
#include <QVariant>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;

class ApiReply;
class Connection;
class ConnectionManager;

// One request to a connection, for QML: done(result) comes once, with
// { ok, status, data, error: { code, message, problems: [{field, message}] } },
// and the call deletes itself after it.
class ApiCall : public QObject {
    Q_OBJECT
    QML_ANONYMOUS
public:
    // With a file, a good answer's bytes are written to it before done():
    // the result says { saved, saveError }. With keepAside, they go to a
    // file of their own in the temporary folder, as { temp }.
    explicit ApiCall(ApiReply *reply, QObject *parent = nullptr, const QString &file = QString(), bool keepAside = false);
    Q_INVOKABLE void abort();

signals:
    void done(const QVariantMap &result);
    void progress(double done, double total);

private:
    QPointer<ApiReply> m_reply;
    QString m_file;
    bool m_keepAside = false;
};

// What the interface needs of the app's C++ side: the connections (this
// computer and the team servers), requests to them and their live feeds,
// files on this computer, and the interface language.
class Backend : public QObject {
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    Q_PROPERTY(QList<QObject *> connections READ connections NOTIFY connectionsChanged)
    Q_PROPERTY(QObject *local READ local CONSTANT)
    Q_PROPERTY(QString version READ version CONSTANT)
    Q_PROPERTY(QString language READ language CONSTANT)
    Q_PROPERTY(bool gitWords READ gitWords WRITE setGitWords NOTIFY gitWordsChanged)
    // Light or dark: "light", "dark", or empty to follow the system.
    Q_PROPERTY(QString appearance READ appearance WRITE setAppearance NOTIFY appearanceChanged)
public:
    explicit Backend(ConnectionManager *manager, QObject *parent = nullptr);
    // The one the app made, for QML.
    static Backend *create(QQmlEngine *, QJSEngine *);
    static void setInstance(Backend *b);

    QList<QObject *> connections() const;
    QObject *local() const;
    QString version() const;
    QString language() const;
    bool gitWords() const;
    QString appearance() const;
    void setAppearance(const QString &appearance);
    // Shows the kept choice of light or dark; called once at start.
    static void applyAppearance();
    void setGitWords(bool git);
    ConnectionManager *manager() const { return m_manager; }

    Q_INVOKABLE QObject *connection(const QString &id) const;
    // method is GET, POST, PUT, PATCH or DELETE; body any JSON value;
    // version, where 0 or more, goes as If-Match.
    Q_INVOKABLE ApiCall *request(const QString &connectionId, const QString &method, const QString &path, const QVariant &body = {},
                                 const QVariantMap &query = {}, int version = -1);
    // Sends a file of this computer as the body.
    Q_INVOKABLE ApiCall *upload(const QString &connectionId, const QString &method, const QString &path, const QString &file,
                                const QString &contentType = QStringLiteral("application/octet-stream"), const QVariantMap &query = {});
    // Writes the answer's bytes to a file of this computer.
    Q_INVOKABLE ApiCall *download(const QString &connectionId, const QString &path, const QString &file, const QVariantMap &query = {});
    // Posts body and writes the answer's bytes to a file of this computer;
    // the result's headers has the answer's headers by lower-case name.
    // A file from a connection, kept aside until the person says where it
    // goes (keep()): done() has { temp, fileName } besides the result.
    Q_INVOKABLE ApiCall *fetch(const QString &connectionId, const QString &path, const QVariantMap &query = {});
    // Moves a file fetched to where it is kept; the problem, or "".
    Q_INVOKABLE QString keep(const QString &temp, const QVariant &to) const;
    // Writes text (UTF-8) to a file; the problem, or "".
    Q_INVOKABLE QString writeText(const QVariant &to, const QString &text) const;
    Q_INVOKABLE ApiCall *postToFile(const QString &connectionId, const QString &path, const QVariant &body, const QString &file);
    // A path segment escaped as the API wants it.
    Q_INVOKABLE QString seg(const QString &s) const;
    // A name as a file's name: letters and digits, the rest "_".
    Q_INVOKABLE QString fileSafe(const QString &name) const;

    // Files and folders as QML hands them over (file: URLs or paths).
    Q_INVOKABLE QString localPath(const QVariant &url) const;
    // A folder's files, with those of its subfolders; files stay as they are.
    Q_INVOKABLE QStringList expand(const QVariantList &urls) const;
    Q_INVOKABLE QString fileName(const QString &path) const;

    // Segments (SegmentContent): content as HTML to show, with its CSS and
    // marks [{ start, end, cssClass }] over the plain text; the words of b
    // that a lacks marked; whether two contents are the same; a key that is
    // the same for the same text and tags; the number of words; text folded
    // as a search compares it.
    Q_INVOKABLE QString segmentHtml(const QVariantList &content, const QVariantList &marks = {}) const;
    Q_INVOKABLE QString diffHtml(const QString &a, const QString &b) const;
    Q_INVOKABLE QString segmentCss() const;
    Q_INVOKABLE bool sameContent(const QVariantList &a, const QVariantList &b) const;
    Q_INVOKABLE QString contentKey(const QVariantList &content) const;
    Q_INVOKABLE int wordCount(const QVariantList &content) const;
    // Regular expressions with Unicode classes (\\p{L} and the like), which
    // the QML engine's own lack: every match of pattern in text.
    Q_INVOKABLE QStringList matchAll(const QString &pattern, const QString &text) const;
    // A segment's content with find replaced in its text runs, the tags
    // left where they are: { content, count }.
    Q_INVOKABLE QVariantMap replaceInContent(const QVariantList &content, const QString &find, const QString &with, bool matchCase, bool wholeWords) const;
    // The preview of the translated document (documentpreview.h).
    Q_INVOKABLE QString previewHtml(const QVariantList &segments, const QVariantList &layout, const QString &activeId) const;
    Q_INVOKABLE QString foldForSearch(const QString &text) const;
    // Files for a project: each file as { path, name, size, problem }, a
    // folder as its files, named with the folder ("Folder/sub/file.docx").
    Q_INVOKABLE QVariantList filesOf(const QVariantList &urls) const;
    // A file's bytes in base64, for requests that carry files in JSON.
    Q_INVOKABLE QString base64File(const QString &path) const;
    // A file in base64 as data, or why it could not be read as error.
    Q_INVOKABLE QVariantMap readBase64(const QVariant &path) const;
    // A connection's address, without a slash at the end.
    Q_INVOKABLE QString baseUrl(const QString &connectionId) const;

    // Settings of this computer.
    Q_INVOKABLE QVariant setting(const QString &key, const QVariant &fallback = {}) const;
    Q_INVOKABLE void setSetting(const QString &key, const QVariant &value);

    // The interface language: codes, names and the one chosen.
    Q_INVOKABLE QStringList interfaceLanguages() const;
    Q_INVOKABLE QString interfaceLanguageName(const QString &code) const;
    Q_INVOKABLE QString chosenInterfaceLanguage() const;
    Q_INVOKABLE void chooseInterfaceLanguage(const QString &code);
    Q_INVOKABLE QString systemLanguage() const;
    // A language tag's name in the interface language ("German (Germany)").
    Q_INVOKABLE QString languageName(const QString &code) const;
    // The languages offered by name, as { code, label } ("German
    // (Germany), de-DE"); and a tag as typed made canonical, or empty.
    Q_INVOKABLE QVariantList commonLanguages() const;
    Q_INVOKABLE QString normalizeLanguage(const QString &typed) const;
    Q_INVOKABLE bool rtl(const QString &tag) const;
    // A date and time as the interface writes them.
    Q_INVOKABLE QString date(const QString &iso) const;
    Q_INVOKABLE QString time(const QString &iso) const;
    Q_INVOKABLE QString number(double n, int decimals = 0) const;
    Q_INVOKABLE QString listOf(const QStringList &items) const;

    // The wording of the branching vocabulary (see words.h).
    Q_INVOKABLE QString word(const QString &key) const;
    // Whether a role allows a permission, and a role in words (roles.h).
    Q_INVOKABLE bool roleAllows(const QString &role, const QString &need) const;
    Q_INVOKABLE QString roleText(const QString &name) const;
    Q_INVOKABLE QStringList permissions() const;

    // The editor's options and the context rule of new projects (options.h).
    Q_INVOKABLE QVariant option(const QString &name) const;
    Q_INVOKABLE void setOption(const QString &name, const QVariant &value);

    // Where sign-ins and work are kept on this computer.
    Q_INVOKABLE QString keychainNote() const;
    Q_INVOKABLE QString dataDir() const;
    Q_INVOKABLE void openFolder(const QString &path) const;
    // The licence and the copyright notice as they come with the app.
    Q_INVOKABLE QString licence() const;
    Q_INVOKABLE void copy(const QString &text) const;

    // Team servers: signing out, forgetting one, and the local account's
    // record after it changed.
    Q_INVOKABLE void signOut(const QString &connectionId);
    Q_INVOKABLE void removeServer(const QString &connectionId);
    Q_INVOKABLE void setUser(const QString &connectionId, const QVariantMap &user);
    Q_INVOKABLE QString serverUrl(const QString &typed) const;
    // Asks a server what it is: answered by probed(url, { version,
    // provider, setup } or { error }).
    Q_INVOKABLE void probe(const QString &url);
    // Signs in; answered by signedIn(url, connectionId, error).
    Q_INVOKABLE void signInWithPassword(const QString &url, const QString &username, const QString &password);
    Q_INVOKABLE void signInWithProvider(const QString &url);
    Q_INVOKABLE void cancelProviderSignIn();
    Q_INVOKABLE void setUpServer(const QString &url, const QString &token, const QString &username, const QString &name, const QString &email,
                                 const QString &password);

    // Starts the core again after it failed.
    Q_INVOKABLE void restartCore();

    // How the system writes a standard key (QKeySequence::StandardKey).
    Q_INVOKABLE QString keyText(int standardKey) const;

    // Files Interlinea was asked to open: at start, by a later start, or
    // by the system (filesOpened).
    void openFiles(const QStringList &files) { emit filesOpened(files); }

signals:
    void connectionsChanged();
    void gitWordsChanged();
    void appearanceChanged();
    void probed(const QString &url, const QVariantMap &info);
    void signedIn(const QString &url, const QString &connectionId, const QVariantMap &error);
    // A connection's live feed: type is "project", "memory", "termbase",
    // "account" or "presence".
    void event(const QString &connectionId, const QString &type, const QVariantMap &data);
    // Too much changed while a feed was away: read everything again.
    void reset(const QString &connectionId);
    // Documents to start a project with; empty when only the window was asked for.
    void filesOpened(const QStringList &files);

private:
    void watch(Connection *c);
    QSet<Connection *> m_watched;

    ConnectionManager *m_manager;
};
