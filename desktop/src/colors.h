#pragma once

#include <QColor>

// The colours of the design system the apps share (DESIGN.md), in the
// light or dark variant the system is set to: for what the C++ side draws
// or writes as HTML (tags, marks in segments).
namespace Theme {
bool dark();
// A colour by its web name ("ok", "warnBg", "accent", ...).
QColor color(const char *name);
}
