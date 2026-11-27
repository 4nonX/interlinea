#pragma once

#include "apiclient.h"
#include "backend.h"
#include "connectionmanager.h"
#include "corelauncher.h"

#include <QQmlApplicationEngine>
#include <QString>
#include <QTemporaryDir>
#include <QVariant>

class QQuickWindow;

// The core the tests run against: INTERLINEA_CORE, or the one built next to
// the tests.
QString testCoreProgram();

// A started core with a fresh data folder.
struct TestCore {
    QTemporaryDir dir;
    CoreLauncher launcher;
    TestCore();
    // Waits until the core runs; false (and a message) when it doesn't.
    bool start(QString *why);
};

// Waits for an answer of the core or a server.
ApiResult waitFor(ApiReply *reply);

// What is broken in a window's layout once it has settled (text cut off
// or running out of the part it is in), for a test to say there is
// nothing.
QString layoutProblems(QQuickWindow *window);

// The whole app as a person sees it: its window from the QML interface,
// on the connections of a core. Parts of the window are found by their
// objectName, and worked with as their buttons and fields do, by calling
// what those call.
class QmlApp {
public:
    explicit QmlApp(CoreLauncher *core);
    ~QmlApp();
    // Loads the window and waits until the core on this computer is ready.
    bool start(QString *why);

    ConnectionManager connections;
    Backend backend;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    ApiClient *api() const { return connections.local()->api(); }
    // A part of the window by its objectName, in the object tree or the
    // items views made; null when there is none.
    QObject *find(const QString &name) const;
    // A part within another.
    QObject *find(QObject *within, const QString &name) const;
    // Calls a function of a part with up to six arguments; what it gives back.
    QVariant call(QObject *target, const char *method, const QVariantList &args = {}) const;
    QVariant call(const QString &name, const char *method, const QVariantList &args = {}) const;
    QVariant get(const QString &name, const char *property) const;
    // Clicks a button, as the mouse or the space key does; false when
    // there is none or it can't be clicked.
    bool click(const QString &name) const;
    // Whether a button is there to be clicked.
    bool clickable(const QString &name) const;
    bool click(QObject *button) const;
    // Types into a field, key by key, as a person does: what it does
    // while typing (onTextEdited) happens. clear empties it first.
    bool type(QObject *field, const QString &text, bool clear = true) const;
    bool type(const QString &name, const QString &text, bool clear = true) const;
    // The last notice the window showed.
    QString notice() const;
    // Shows a page of the window ("projects", "editor", ...).
    void show(const QString &page) const;
    // Saves a picture of the window under INTERLINEA_SCREENSHOTS, if set.
    void shot(const QString &name) const;
};
