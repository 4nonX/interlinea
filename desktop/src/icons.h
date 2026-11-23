#pragma once

#include <QIcon>

// The app's line icons, drawn rather than shipped: they follow the
// palette (and so light, dark and high contrast) and stay sharp at any
// scale.
namespace Icons {
// "projects", "editor", "memories", "terminology", "review", "accounts",
// "settings".
QIcon get(const QString &name);
}
