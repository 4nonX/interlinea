#pragma once

class QApplication;
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
}
