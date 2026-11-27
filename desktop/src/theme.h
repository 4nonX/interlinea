#pragma once

#include <QObject>

class QApplication;
class QColor;
class QPushButton;
class QString;
class QWidget;

// Interlinea's look, from the design system the web app shares
// (DESIGN.md): cool neutrals, one blue accent, 6 px controls on 10 px
// panels, the platform's UI font. It is light or dark as the settings
// choose, or as the system is set, and steps aside for a high-contrast
// setting, where the platform's own colours serve people better.
namespace Theme {
// Tells when the look changed from light to dark or back, for what drew
// itself in the colours it had.
class Notifier : public QObject {
    Q_OBJECT
signals:
    void changed();
};
Notifier *notifier();
// Sets the look of the whole application.
void apply(QApplication &app);
// Keeps the choice of light or dark ("light", "dark", or empty for the
// system's) and shows it at once.
void setAppearance(QApplication &app, const QString &appearance);
// Whether the theme is in use (not in high contrast, nor turned off with
// INTERLINEA_NATIVE_STYLE=1).
bool active();
bool dark();
// Marks a button as the page's or dialog's main action.
void primary(QWidget *button);
// Marks a label as a page title.
void title(QWidget *label);
// Marks a label as secondary text (hints, details).
void secondary(QWidget *label);
// Sets a widget's role for the style sheet ("h1", "meta", "muted",
// "sectionTitle", "count", "progress").
void role(QWidget *w, const char *role);
// The web app's .btn-small and .btn-quiet.
void smallButton(QWidget *button);
void quietButton(QWidget *button);
// A colour of the design system by its web name ("ok", "warnBg",
// "accent", …), in the current light or dark variant.
QColor color(const char *name);
// The name of a colour of the design system as it is now, or nothing: an
// icon drawn in it is drawn in that colour's other variant when the look
// changes.
QByteArray nameOf(const QColor &c);
// A small button with an icon, its text and its keys in a chip, as the
// web editor's tool buttons ("Copy source  Ctrl+Ins").
QPushButton *keyButton(const QString &text, const QString &keys, const QString &icon, QWidget *parent);
}
