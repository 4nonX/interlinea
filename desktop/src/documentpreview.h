#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QList>
#include <QString>

// The preview of a translated document: its paragraphs with their styles,
// tables, notes, header and footer, each segment in its translation, or in
// the source text, grey and underlined, while it has none. Each segment is
// a link "seg:<id>". Built like the browser client's.
QString previewHtml(const QList<QJsonObject> &segments, const QJsonArray &layout, const QString &activeId);
