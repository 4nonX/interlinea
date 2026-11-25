#pragma once

#include "corelauncher.h"

#include <QString>

class QWidget;
#include <QTemporaryDir>

// The core the tests run against: INTERLINEA_CORE, or the one built next to
// the tests.
QString testCoreProgram();

// What is broken in a window's layout once it has settled (text cut off,
// parts over each other), for a test to say there is nothing.
QString layoutProblems(QWidget *window);

// A started core with a fresh data folder.
struct TestCore {
    QTemporaryDir dir;
    CoreLauncher launcher;
    TestCore();
    // Waits until the core runs; false (and a message) when it doesn't.
    bool start(QString *why);
};
