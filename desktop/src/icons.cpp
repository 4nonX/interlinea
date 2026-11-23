#include "icons.h"

#include <QApplication>
#include <QIconEngine>
#include <QPainter>
#include <QPainterPath>
#include <QPalette>

namespace {

// Draws one icon on a 24 by 24 grid, 1.6 units wide strokes.
void draw(QPainter &p, const QString &name)
{
    QPainterPath path;
    if (name == QLatin1String("projects")) {
        path.addRoundedRect(QRectF(4, 6, 16, 13), 2, 2);
        path.moveTo(4, 9);
        path.lineTo(20, 9);
        path.moveTo(8, 6);
        path.lineTo(9.5, 3.5);
        path.lineTo(14.5, 3.5);
        path.lineTo(16, 6);
    } else if (name == QLatin1String("editor")) {
        path.addRoundedRect(QRectF(3.5, 4, 17, 16), 2, 2);
        for (double y : {8.5, 12.0, 15.5}) {
            path.moveTo(7, y);
            path.lineTo(y == 15.5 ? 13 : 17, y);
        }
    } else if (name == QLatin1String("memories")) {
        path.addEllipse(QRectF(5, 3.5, 14, 4.5));
        path.moveTo(5, 5.75);
        path.lineTo(5, 18);
        path.moveTo(19, 5.75);
        path.lineTo(19, 18);
        path.arcMoveTo(QRectF(5, 15.75, 14, 4.5), 180);
        path.arcTo(QRectF(5, 15.75, 14, 4.5), 180, 180);
        path.arcMoveTo(QRectF(5, 9.75, 14, 4.5), 180);
        path.arcTo(QRectF(5, 9.75, 14, 4.5), 180, 180);
    } else if (name == QLatin1String("terminology")) {
        path.moveTo(5, 4);
        path.lineTo(16, 4);
        path.lineTo(19, 7);
        path.lineTo(19, 20);
        path.lineTo(5, 20);
        path.closeSubpath();
        path.moveTo(8.5, 10);
        path.lineTo(15.5, 10);
        path.moveTo(8.5, 13.5);
        path.lineTo(15.5, 13.5);
        path.moveTo(8.5, 17);
        path.lineTo(12.5, 17);
    } else if (name == QLatin1String("review")) {
        path.addRoundedRect(QRectF(4, 4, 16, 16), 3, 3);
        path.moveTo(8, 12.5);
        path.lineTo(11, 15.5);
        path.lineTo(16.5, 9);
    } else if (name == QLatin1String("accounts")) {
        path.addEllipse(QPointF(12, 8.5), 3.6, 3.6);
        path.moveTo(5, 20);
        path.cubicTo(5.5, 15, 8.5, 13.5, 12, 13.5);
        path.cubicTo(15.5, 13.5, 18.5, 15, 19, 20);
    } else { // settings: three sliders
        for (int i = 0; i < 3; ++i) {
            const double y = 6 + i * 6;
            const double knob = i == 1 ? 15 : 9;
            path.moveTo(4, y);
            path.lineTo(knob - 2.2, y);
            path.moveTo(knob + 2.2, y);
            path.lineTo(20, y);
            path.addEllipse(QPointF(knob, y), 2.2, 2.2);
        }
    }
    p.drawPath(path);
}

class LineIcon : public QIconEngine {
public:
    explicit LineIcon(const QString &name) : m_name(name) {}
    QIconEngine *clone() const override { return new LineIcon(m_name); }
    void paint(QPainter *p, const QRect &rect, QIcon::Mode mode, QIcon::State) override
    {
        const QPalette pal = QApplication::palette();
        QColor c = mode == QIcon::Selected ? pal.color(QPalette::Link) : pal.color(QPalette::WindowText);
        if (mode == QIcon::Disabled)
            c = pal.color(QPalette::Disabled, QPalette::WindowText);
        p->save();
        p->setRenderHint(QPainter::Antialiasing);
        p->translate(rect.topLeft());
        p->scale(rect.width() / 24.0, rect.height() / 24.0);
        QPen pen(c, 1.6);
        pen.setCapStyle(Qt::RoundCap);
        pen.setJoinStyle(Qt::RoundJoin);
        p->setPen(pen);
        p->setBrush(Qt::NoBrush);
        draw(*p, m_name);
        p->restore();
    }
    QPixmap pixmap(const QSize &size, QIcon::Mode mode, QIcon::State state) override
    {
        QPixmap pm(size);
        pm.fill(Qt::transparent);
        QPainter p(&pm);
        paint(&p, QRect(QPoint(), size), mode, state);
        return pm;
    }

private:
    QString m_name;
};

} // namespace

namespace Icons {
QIcon get(const QString &name)
{
    return QIcon(new LineIcon(name));
}
}
