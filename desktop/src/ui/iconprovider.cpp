#include "iconprovider.h"

#include "icons.h"

#include <QPainter>
#include <QUrlQuery>

IconProvider::IconProvider()
    : QQuickImageProvider(QQuickImageProvider::Image)
{
}

QImage IconProvider::requestImage(const QString &id, QSize *size, const QSize &requestedSize)
{
    const QString name = id.section(QLatin1Char('?'), 0, 0);
    const QUrlQuery q(id.section(QLatin1Char('?'), 1));
    const QColor color(q.queryItemValue(QStringLiteral("color")));
    const QSize s = requestedSize.isValid() && !requestedSize.isEmpty() ? requestedSize : QSize(16, 16);
    QImage img(s, QImage::Format_ARGB32_Premultiplied);
    img.fill(Qt::transparent);
    QPainter p(&img);
    p.setRenderHint(QPainter::Antialiasing);
    // Drawn on a grid of 24 units, as the web's icons, with their lines.
    Icons::paint(p, QRectF(0, 0, s.width(), s.height()), name, color.isValid() ? color : QColor(Qt::black), 1.8);
    p.end();
    if (size)
        *size = s;
    return img;
}
