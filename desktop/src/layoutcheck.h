#pragma once

#include <QStringList>

#include <functional>

class QWidget;

// Looks over a window for what a person would see as broken: text cut
// off or lying over other text, a button narrower than its label, a box
// for text too low for one line, parts lying over each other or reaching
// out of what holds them. The tests run it over every window they show,
// at several scalings, and fail on what it finds, and on a layout that
// keeps switching between sizes without settling; with
// INTERLINEA_LAYOUT_CHECK set, the app writes what it finds to the log.
namespace LayoutCheck {

// What is wrong in a window now, one line each, with where it is.
QStringList problems(QWidget *window);

// Looks over every window a few times a second; what stays wrong from one
// look to the next is reported, once, to report.
void watch(std::function<void(const QString &)> report);

// Marks a widget that is meant to lie over others (a notice, a popup).
void markOverlay(QWidget *w);

} // namespace LayoutCheck
