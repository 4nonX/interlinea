#pragma once

#include <QQuickImageProvider>

// The app's line icons for QML, drawn at the size asked for:
// "image://icon/<name>?color=<#rrggbb>".
class IconProvider : public QQuickImageProvider {
public:
    IconProvider();
    QImage requestImage(const QString &id, QSize *size, const QSize &requestedSize) override;
};
