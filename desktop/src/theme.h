#pragma once

class QApplication;
class QColor;
class QPushButton;
class QString;
class QWidget;

// Interlinea's look, from the design system the web app shares
// (DESIGN.md): cool neutrals, one blue accent, 6 px controls on 10 px
// panels, the platform's UI font. It follows the system's light or dark
// setting, and steps aside for a high-contrast setting, where the
// platform's own colours serve people better.
namespace Theme {
// Sets the look of the whole application.
void apply(QApplication &app);
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
// A small button with an icon, its text and its keys in a chip, as the
// web editor's tool buttons ("Copy source  Ctrl+Ins").
QPushButton *keyButton(const QString &text, const QString &keys, const QString &icon, QWidget *parent);
}
