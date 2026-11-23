#pragma once

#include "corelauncher.h"

#include <QString>
#include <QTemporaryDir>

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
