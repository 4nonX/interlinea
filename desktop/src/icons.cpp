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
    } else if (name == QLatin1String("arrow-up") || name == QLatin1String("arrow-down")) {
        const bool up = name == QLatin1String("arrow-up");
        path.moveTo(12, up ? 19 : 5);
        path.lineTo(12, up ? 5 : 19);
        path.moveTo(6, up ? 11 : 13);
        path.lineTo(12, up ? 5 : 19);
        path.lineTo(18, up ? 11 : 13);
    } else if (name == QLatin1String("arrow-left")) {
        path.moveTo(19, 12);
        path.lineTo(5, 12);
        path.moveTo(11, 6);
        path.lineTo(5, 12);
        path.lineTo(11, 18);
    } else if (name == QLatin1String("bar-chart")) {
        for (const auto &[x, top] : {std::pair{6.0, 13.0}, {12.0, 5.0}, {18.0, 9.0}}) {
            path.moveTo(x, 20);
            path.lineTo(x, top);
        }
    } else if (name == QLatin1String("eye")) {
        path.moveTo(2.5, 12);
        path.cubicTo(5, 6.5, 8.5, 5, 12, 5);
        path.cubicTo(15.5, 5, 19, 6.5, 21.5, 12);
        path.cubicTo(19, 17.5, 15.5, 19, 12, 19);
        path.cubicTo(8.5, 19, 5, 17.5, 2.5, 12);
        path.addEllipse(QPointF(12, 12), 3, 3);
    } else if (name == QLatin1String("download") || name == QLatin1String("upload")) {
        const bool down = name == QLatin1String("download");
        path.moveTo(4, 15);
        path.lineTo(4, 19);
        path.lineTo(20, 19);
        path.lineTo(20, 15);
        path.moveTo(12, 4);
        path.lineTo(12, 15);
        path.moveTo(7, down ? 10 : 9);
        path.lineTo(12, down ? 15 : 4);
        path.lineTo(17, down ? 10 : 9);
    } else if (name == QLatin1String("git-merge")) {
        path.addEllipse(QPointF(18, 18), 2.5, 2.5);
        path.addEllipse(QPointF(6, 6), 2.5, 2.5);
        path.moveTo(6, 8.5);
        path.lineTo(6, 21);
        path.moveTo(6, 9);
        path.cubicTo(6, 14, 10, 18, 15.5, 18);
    } else if (name == QLatin1String("check")) {
        path.moveTo(5, 12.5);
        path.lineTo(10, 17.5);
        path.lineTo(19.5, 7);
    } else if (name == QLatin1String("pencil-line")) {
        path.moveTo(13, 21);
        path.lineTo(21, 21);
        path.moveTo(16.5, 3.5);
        path.lineTo(20.5, 7.5);
        path.lineTo(8, 20);
        path.lineTo(3.5, 21);
        path.lineTo(4.5, 16.5);
        path.closeSubpath();
    } else if (name == QLatin1String("circle-dashed")) {
        for (int i = 0; i < 6; ++i) {
            path.arcMoveTo(QRectF(3.5, 3.5, 17, 17), i * 60 + 10);
            path.arcTo(QRectF(3.5, 3.5, 17, 17), i * 60 + 10, 32);
        }
    } else if (name == QLatin1String("lock")) {
        path.addRoundedRect(QRectF(5, 11, 14, 10), 2, 2);
        path.moveTo(8, 11);
        path.lineTo(8, 7.5);
        path.arcTo(QRectF(8, 3.5, 8, 8), 180, -180);
        path.lineTo(16, 11);
    } else if (name == QLatin1String("alert-triangle")) {
        path.moveTo(12, 3.5);
        path.lineTo(21.5, 20);
        path.lineTo(2.5, 20);
        path.closeSubpath();
        path.moveTo(12, 9.5);
        path.lineTo(12, 13.5);
        path.moveTo(12, 16.8);
        path.lineTo(12, 17);
    } else if (name == QLatin1String("tag")) {
        path.moveTo(3, 3);
        path.lineTo(12, 3);
        path.lineTo(21, 12);
        path.lineTo(12, 21);
        path.lineTo(3, 12);
        path.closeSubpath();
        path.addEllipse(QPointF(7.5, 7.5), 1.2, 1.2);
    } else if (name == QLatin1String("plus") || name == QLatin1String("x")) {
        if (name == QLatin1String("plus")) {
            path.moveTo(12, 5);
            path.lineTo(12, 19);
            path.moveTo(5, 12);
            path.lineTo(19, 12);
        } else {
            path.moveTo(6, 6);
            path.lineTo(18, 18);
            path.moveTo(18, 6);
            path.lineTo(6, 18);
        }
    } else if (name == QLatin1String("search")) {
        path.addEllipse(QPointF(11, 11), 6.5, 6.5);
        path.moveTo(16, 16);
        path.lineTo(20.5, 20.5);
    } else if (name == QLatin1String("copy")) {
        path.addRoundedRect(QRectF(8, 8, 13, 13), 2, 2);
        path.moveTo(4, 16);
        path.lineTo(4, 5);
        path.quadTo(4, 3, 6, 3);
        path.lineTo(16, 3);
    } else if (name == QLatin1String("keyboard")) {
        path.addRoundedRect(QRectF(2.5, 5.5, 19, 13), 2, 2);
        for (double x : {6.5, 10.0, 13.5, 17.0})
            path.addRect(QRectF(x, 9, 0.6, 0.6));
        path.moveTo(7, 15);
        path.lineTo(17, 15);
    } else if (name == QLatin1String("repeat")) {
        path.moveTo(4, 11);
        path.lineTo(4, 9);
        path.quadTo(4, 7, 6, 7);
        path.lineTo(19, 7);
        path.moveTo(16, 4);
        path.lineTo(19, 7);
        path.lineTo(16, 10);
        path.moveTo(20, 13);
        path.lineTo(20, 15);
        path.quadTo(20, 17, 18, 17);
        path.lineTo(5, 17);
        path.moveTo(8, 20);
        path.lineTo(5, 17);
        path.lineTo(8, 14);
    } else if (name == QLatin1String("cloud")) {
        path.moveTo(7, 19);
        path.cubicTo(3.5, 19, 2.5, 15, 5, 13.5);
        path.cubicTo(5, 9, 10, 7, 12.5, 9.5);
        path.cubicTo(14, 6.5, 19.5, 7, 19.5, 11.5);
        path.cubicTo(22.5, 12.5, 22, 19, 18, 19);
        path.closeSubpath();
    } else if (name == QLatin1String("hard-drive")) {
        path.addRoundedRect(QRectF(3, 13, 18, 7), 2, 2);
        path.moveTo(3, 13);
        path.lineTo(6.5, 5);
        path.lineTo(17.5, 5);
        path.lineTo(21, 13);
        path.moveTo(7, 16.5);
        path.lineTo(7.2, 16.5);
        path.moveTo(10.5, 16.5);
        path.lineTo(10.7, 16.5);
    } else if (name == QLatin1String("file-text")) {
        path.moveTo(14, 3);
        path.lineTo(6, 3);
        path.lineTo(6, 21);
        path.lineTo(18, 21);
        path.lineTo(18, 7);
        path.closeSubpath();
        path.moveTo(14, 3);
        path.lineTo(14, 7);
        path.lineTo(18, 7);
        path.moveTo(9, 12);
        path.lineTo(15, 12);
        path.moveTo(9, 16);
        path.lineTo(15, 16);
    } else if (name == QLatin1String("folder-open")) {
        path.moveTo(3, 19);
        path.lineTo(3, 6);
        path.lineTo(9, 6);
        path.lineTo(11, 8);
        path.lineTo(18, 8);
        path.lineTo(18, 11);
        path.moveTo(3, 19);
        path.lineTo(6, 11);
        path.lineTo(21, 11);
        path.lineTo(18, 19);
        path.closeSubpath();
    } else if (name == QLatin1String("trash")) {
        path.moveTo(4, 7);
        path.lineTo(20, 7);
        path.moveTo(9, 7);
        path.lineTo(9, 4);
        path.lineTo(15, 4);
        path.lineTo(15, 7);
        path.moveTo(6, 7);
        path.lineTo(7, 20);
        path.lineTo(17, 20);
        path.lineTo(18, 7);
    } else if (name == QLatin1String("pencil")) {
        path.moveTo(16.5, 3.5);
        path.lineTo(20.5, 7.5);
        path.lineTo(8, 20);
        path.lineTo(3.5, 21);
        path.lineTo(4.5, 16.5);
        path.closeSubpath();
    } else if (name == QLatin1String("clock")) {
        path.addEllipse(QPointF(12, 12), 9, 9);
        path.moveTo(12, 7);
        path.lineTo(12, 12);
        path.lineTo(15.5, 14);
    } else if (name == QLatin1String("user-plus")) {
        path.addEllipse(QPointF(9, 8), 4, 4);
        path.moveTo(2, 21);
        path.cubicTo(2, 16, 5, 14, 9, 14);
        path.cubicTo(13, 14, 16, 16, 16, 21);
        path.moveTo(19, 8);
        path.lineTo(19, 14);
        path.moveTo(16, 11);
        path.lineTo(22, 11);
    } else if (name == QLatin1String("link")) {
        path.moveTo(9, 17);
        path.lineTo(7, 17);
        path.cubicTo(0.5, 17, 0.5, 7, 7, 7);
        path.lineTo(9, 7);
        path.moveTo(15, 7);
        path.lineTo(17, 7);
        path.cubicTo(23.5, 7, 23.5, 17, 17, 17);
        path.lineTo(15, 17);
        path.moveTo(8, 12);
        path.lineTo(16, 12);
    } else if (name == QLatin1String("refresh")) {
        path.arcMoveTo(QRectF(4, 4, 16, 16), 30);
        path.arcTo(QRectF(4, 4, 16, 16), 30, 300);
        path.moveTo(20, 4);
        path.lineTo(20, 9);
        path.lineTo(15, 9);
    } else if (name == QLatin1String("git-branch")) {
        path.addEllipse(QPointF(6, 5.5), 2.5, 2.5);
        path.addEllipse(QPointF(6, 18.5), 2.5, 2.5);
        path.addEllipse(QPointF(18, 7), 2.5, 2.5);
        path.moveTo(6, 8);
        path.lineTo(6, 16);
        path.moveTo(18, 9.5);
        path.cubicTo(18, 13, 12, 12, 6, 16);
    } else if (name == QLatin1String("git-pull-request")) {
        path.addEllipse(QPointF(6, 6), 2.5, 2.5);
        path.addEllipse(QPointF(6, 18), 2.5, 2.5);
        path.addEllipse(QPointF(18, 18), 2.5, 2.5);
        path.moveTo(6, 8.5);
        path.lineTo(6, 15.5);
        path.moveTo(18, 15.5);
        path.lineTo(18, 9);
        path.cubicTo(18, 7, 17, 6, 15, 6);
        path.lineTo(12, 6);
        path.moveTo(14, 3.5);
        path.lineTo(11.5, 6);
        path.lineTo(14, 8.5);
    } else if (name == QLatin1String("sliders")) {
        path.moveTo(4, 7);
        path.lineTo(12, 7);
        path.moveTo(17, 7);
        path.lineTo(20, 7);
        path.addEllipse(QPointF(14.5, 7), 2.5, 2.5);
        path.moveTo(4, 17);
        path.lineTo(7, 17);
        path.moveTo(12, 17);
        path.lineTo(20, 17);
        path.addEllipse(QPointF(9.5, 17), 2.5, 2.5);
    } else if (name == QLatin1String("replace")) {
        // Two boxes and the way from the one to the other.
        path.addRoundedRect(QRectF(3, 3, 7, 7), 1.5, 1.5);
        path.addRoundedRect(QRectF(14, 14, 7, 7), 1.5, 1.5);
        path.moveTo(14, 5);
        path.lineTo(17.5, 5);
        path.lineTo(17.5, 10.5);
        path.moveTo(15, 8);
        path.lineTo(17.5, 10.5);
        path.lineTo(20, 8);
        path.moveTo(10, 19);
        path.lineTo(6.5, 19);
        path.lineTo(6.5, 13.5);
        path.moveTo(4, 16);
        path.lineTo(6.5, 13.5);
        path.lineTo(9, 16);
    } else if (name == QLatin1String("scissors")) {
        // Two rings and the blades that cross.
        path.addEllipse(QPointF(6, 6), 3, 3);
        path.addEllipse(QPointF(6, 18), 3, 3);
        path.moveTo(8.12, 8.12);
        path.lineTo(12, 12);
        path.moveTo(20, 4);
        path.lineTo(8.12, 15.88);
        path.moveTo(14.8, 14.8);
        path.lineTo(20, 20);
    } else if (name == QLatin1String("combine")) {
        // Two boxes brought into one.
        path.addRoundedRect(QRectF(3, 3, 8, 8), 1.5, 1.5);
        path.addRoundedRect(QRectF(13, 13, 8, 8), 1.5, 1.5);
        path.moveTo(14, 7);
        path.lineTo(17, 7);
        path.lineTo(17, 10);
        path.moveTo(10, 17);
        path.lineTo(7, 17);
        path.lineTo(7, 14);
    } else if (name == QLatin1String("comment")) {
        // A speech bubble with its tail at the bottom left.
        path.moveTo(21, 15);
        path.cubicTo(21, 16.1, 20.1, 17, 19, 17);
        path.lineTo(7, 17);
        path.lineTo(3, 21);
        path.lineTo(3, 5);
        path.cubicTo(3, 3.9, 3.9, 3, 5, 3);
        path.lineTo(19, 3);
        path.cubicTo(20.1, 3, 21, 3.9, 21, 5);
        path.closeSubpath();
    } else if (name == QLatin1String("flag")) {
        // A flag on its pole.
        path.moveTo(4, 22);
        path.lineTo(4, 3);
        path.moveTo(4, 4);
        path.cubicTo(8, 1.5, 12, 6.5, 20, 4);
        path.lineTo(20, 14);
        path.cubicTo(12, 16.5, 8, 11.5, 4, 14);
    } else if (name == QLatin1String("languages")) {
        // A letter of one script and one of another, as translation is drawn.
        path.moveTo(4, 6);
        path.lineTo(13, 6);
        path.moveTo(8.5, 4);
        path.lineTo(8.5, 6);
        path.moveTo(11, 6);
        path.cubicTo(10.5, 10, 8, 13, 4, 15);
        path.moveTo(6, 9);
        path.cubicTo(7, 11.5, 9.5, 13.5, 12, 14.5);
        path.moveTo(13, 20);
        path.lineTo(17, 11);
        path.lineTo(21, 20);
        path.moveTo(14.4, 17);
        path.lineTo(19.6, 17);
    } else if (name == QLatin1String("columns")) {
        path.addRoundedRect(QRectF(3, 3, 18, 18), 2, 2);
        path.moveTo(12, 3);
        path.lineTo(12, 21);
    } else if (name == QLatin1String("file-up")) {
        path.moveTo(14, 3);
        path.lineTo(6, 3);
        path.lineTo(6, 21);
        path.lineTo(18, 21);
        path.lineTo(18, 7);
        path.lineTo(14, 3);
        path.lineTo(14, 7);
        path.lineTo(18, 7);
        path.moveTo(12, 18);
        path.lineTo(12, 11);
        path.moveTo(9, 14);
        path.lineTo(12, 11);
        path.lineTo(15, 14);
    } else if (name == QLatin1String("history")) {
        path.arcMoveTo(QRectF(3.5, 3.5, 17, 17), 160);
        path.arcTo(QRectF(3.5, 3.5, 17, 17), 160, -340);
        path.moveTo(4, 6.5);
        path.lineTo(4.3, 10.5);
        path.lineTo(8.2, 9.8);
        path.moveTo(12, 8);
        path.lineTo(12, 12);
        path.lineTo(15, 14);
    } else if (name == QLatin1String("info")) {
        path.addEllipse(QPointF(12, 12), 8.5, 8.5);
        path.moveTo(12, 11);
        path.lineTo(12, 16);
        path.moveTo(12, 8);
        path.lineTo(12, 8.2);
    } else if (name == QLatin1String("chevron-down")) {
        path.moveTo(6, 9);
        path.lineTo(12, 15);
        path.lineTo(18, 9);
    } else if (name == QLatin1String("chevron-up")) {
        path.moveTo(6, 15);
        path.lineTo(12, 9);
        path.lineTo(18, 15);
    } else if (name == QLatin1String("chevron-right")) {
        path.moveTo(9, 6);
        path.lineTo(15, 12);
        path.lineTo(9, 18);
    } else if (name == QLatin1String("users")) {
        path.addEllipse(QPointF(9, 8.5), 3.3, 3.3);
        path.moveTo(3, 19.5);
        path.cubicTo(3.5, 15, 6, 13.5, 9, 13.5);
        path.cubicTo(12, 13.5, 14.5, 15, 15, 19.5);
        path.addEllipse(QPointF(16.5, 8), 2.6, 2.6);
        path.moveTo(16.5, 12.5);
        path.cubicTo(19, 12.5, 20.5, 14, 21, 17.5);
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
    LineIcon(const QString &name, const QColor &color) : m_name(name), m_color(color) {}
    QIconEngine *clone() const override { return new LineIcon(m_name, m_color); }
    void paint(QPainter *p, const QRect &rect, QIcon::Mode mode, QIcon::State) override
    {
        const QPalette pal = QApplication::palette();
        QColor c = mode == QIcon::Selected ? pal.color(QPalette::Link) : pal.color(QPalette::WindowText);
        if (m_color.isValid())
            c = m_color;
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
    QColor m_color;
};

} // namespace

namespace Icons {
QIcon get(const QString &name, const QColor &color)
{
    return QIcon(new LineIcon(name, color));
}

void paint(QPainter &p, const QRectF &rect, const QString &name, const QColor &color, double width)
{
    p.save();
    p.setRenderHint(QPainter::Antialiasing);
    p.translate(rect.topLeft());
    p.scale(rect.width() / 24.0, rect.height() / 24.0);
    QPen pen(color, width);
    pen.setCapStyle(Qt::RoundCap);
    pen.setJoinStyle(Qt::RoundJoin);
    p.setPen(pen);
    p.setBrush(Qt::NoBrush);
    draw(p, name);
    p.restore();
}
}
