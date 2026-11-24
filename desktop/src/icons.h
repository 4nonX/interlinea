#pragma once

#include <QColor>
#include <QIcon>

class QPainter;
class QRectF;

// The app's line icons, drawn rather than shipped: they follow the
// palette (and so light, dark and high contrast) and stay sharp at any
// scale.
namespace Icons {
// "projects", "editor", "memories", "terminology", "review", "accounts",
// "settings", and the web editor's: "arrow-left", "bar-chart", "eye",
// "download", "upload", "git-merge", "check", "pencil-line",
// "circle-dashed", "lock", "alert-triangle", "tag", "plus", "x", "search",
// "copy", "keyboard", "repeat". Drawn in the text colour unless a colour
// is given.
QIcon get(const QString &name, const QColor &color = QColor());
// Draws an icon straight onto a painter (for delegates).
void paint(QPainter &p, const QRectF &rect, const QString &name, const QColor &color, double width = 1.6);
}
