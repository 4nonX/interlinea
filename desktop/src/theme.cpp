#include "theme.h"

#include <QApplication>
#include <QFont>
#include <QFontDatabase>
#include <QPalette>
#include <QStyle>
#include <QStyleFactory>
#include <QStyleHints>
#include <QWidget>

#ifdef Q_OS_WIN
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#endif

namespace {

bool g_active = false;
bool g_dark = false;

struct Tokens {
    const char *bg, *rail, *surface, *surface2, *border, *borderStrong, *text, *text2, *text3, *accent, *accentHover, *accentWeak, *accentText,
        *onAccent, *danger;
};

const Tokens light{"#f3f5f8", "#e8ecf1", "#ffffff", "#f7f9fb", "#d8dde4", "#b9c1cc", "#1a1f26", "#4c5562", "#66707c",
                   "#1f5bd6", "#1a4fbd", "#e8effd", "#1a4aae", "#ffffff", "#aa2219"};
const Tokens dark{"#14171c", "#1a1e24", "#1e232a", "#232931", "#2f3640", "#4a5463", "#e6e9ee", "#b6bec9", "#98a1ad",
                  "#6b9bff", "#86adff", "#1f2c47", "#a9c4ff", "#0c1220", "#ff8a80"};

bool highContrast()
{
#ifdef Q_OS_WIN
    HIGHCONTRASTW hc{};
    hc.cbSize = sizeof(hc);
    if (SystemParametersInfoW(SPI_GETHIGHCONTRAST, sizeof(hc), &hc, 0) && (hc.dwFlags & HCF_HIGHCONTRASTON))
        return true;
#endif
#if QT_VERSION >= QT_VERSION_CHECK(6, 10, 0)
    if (QGuiApplication::styleHints()->accessibility()->contrastPreference() == Qt::ContrastPreference::HighContrast)
        return true;
#endif
    return false;
}

bool systemDark()
{
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    const Qt::ColorScheme s = QGuiApplication::styleHints()->colorScheme();
    if (s != Qt::ColorScheme::Unknown)
        return s == Qt::ColorScheme::Dark;
#endif
    return QGuiApplication::palette().color(QPalette::Window).lightness() < 128;
}

QString sheet(const Tokens &t)
{
    QString s = QStringLiteral(R"(
* { outline: 0; }
QMainWindow, QDialog { background: @bg; }
QWidget { color: @text; }
QToolTip { background: @text; color: @surface; border: 0; border-radius: 6px; padding: 6px 8px; }

/* Navigation */
QListWidget#navigation { background: @rail; border: 0; padding: 8px 10px; font-size: 14px; }
QListWidget#navigation::item { border-radius: 6px; padding: 6px 8px; margin: 1px 0; color: @text2; }
QListWidget#navigation::item:hover { background: rgba(127,140,160,0.14); color: @text; }
QListWidget#navigation::item:selected { background: @accentWeak; color: @accentText; font-weight: 600; }
QListWidget#navigation::item:focus { border: 2px solid @accent; }
QWidget#side { background: @rail; border-right: 1px solid @border; }
QListWidget#navigation { border-right: 0; }
QLabel#appName { font-size: 16px; font-weight: 650; padding: 18px 18px 6px 18px; color: @text; background: transparent; }

/* Text */
QLabel[role="title"] { font-size: 22px; font-weight: 600; color: @text; padding-bottom: 2px; }
QLabel[role="secondary"] { color: @text2; }
QLabel#error, QLabel[role="error"] { color: @danger; }

/* Panels */
QGroupBox { background: @surface; border: 1px solid @border; border-radius: 10px; margin-top: 30px; padding: 14px 16px 16px 16px; font-weight: 600; }
QGroupBox::title { subcontrol-origin: margin; subcontrol-position: top left; left: 2px; top: 4px; padding: 0; color: @text; font-size: 14px; }
QGroupBox QLabel, QGroupBox QCheckBox, QGroupBox QRadioButton { font-weight: 400; }
QFrame#message { background: @accentWeak; border: 1px solid @border; border-radius: 10px; }

/* Buttons */
QPushButton { background: @surface; border: 1px solid @borderStrong; border-radius: 6px; padding: 6px 14px; min-height: 20px; color: @text; }
QPushButton:hover { background: @surface2; border-color: @text3; }
QPushButton:pressed { background: @border; }
QPushButton:focus { border: 2px solid @accent; padding: 5px 13px; }
QPushButton:disabled { color: @text3; border-color: @border; background: @surface2; }
QPushButton[primary="true"] { background: @accent; border: 1px solid @accent; color: @onAccent; font-weight: 600; }
QPushButton[primary="true"]:hover { background: @accentHover; border-color: @accentHover; }
QPushButton[primary="true"]:focus { border: 2px solid @text; }
QPushButton[primary="true"]:disabled { background: @border; border-color: @border; color: @text3; }
QPushButton:flat { border: 0; background: transparent; color: @accentText; }
QPushButton:flat:hover { text-decoration: underline; }
QToolButton { border: 0; border-radius: 6px; padding: 3px 8px; color: @text2; background: transparent; }
QToolButton:hover { background: rgba(127,140,160,0.16); color: @text; }

/* Fields */
QLineEdit, QPlainTextEdit, QTextEdit, QSpinBox, QDateTimeEdit, QComboBox {
    background: @surface; border: 1px solid @borderStrong; border-radius: 6px; padding: 5px 8px; min-height: 20px;
    selection-background-color: @accent; selection-color: @onAccent; }
QLineEdit:hover, QPlainTextEdit:hover, QSpinBox:hover, QDateTimeEdit:hover, QComboBox:hover { border-color: @text3; }
QLineEdit:focus, QPlainTextEdit:focus, QTextEdit:focus, QSpinBox:focus, QDateTimeEdit:focus, QComboBox:focus { border: 2px solid @accent; padding: 4px 7px; }
QLineEdit:disabled, QComboBox:disabled, QSpinBox:disabled, QDateTimeEdit:disabled, QPlainTextEdit:disabled { background: @surface2; color: @text3; border-color: @border; }
QComboBox::drop-down, QDateTimeEdit::drop-down { border: 0; width: 22px; }
QComboBox QAbstractItemView { background: @surface; border: 1px solid @border; border-radius: 6px; padding: 4px; selection-background-color: @accentWeak; selection-color: @accentText; }
QCheckBox, QRadioButton { spacing: 8px; }

/* Lists and tables */
QTreeView, QTableView, QListView { background: @surface; border: 1px solid @border; border-radius: 10px; alternate-background-color: @surface2;
    selection-background-color: @accentWeak; selection-color: @accentText; gridline-color: @border; }
QTreeView::item, QTableView::item, QListView::item { padding: 5px 6px; border: 0; }
QTreeView::item:hover, QTableView::item:hover, QListView::item:hover { background: @surface2; }
QTreeView::item:selected, QTableView::item:selected, QListView::item:selected { background: @accentWeak; color: @accentText; }
QTreeView:focus, QTableView:focus, QListView:focus { border: 2px solid @accent; }
QHeaderView { background: transparent; border: 0; }
QHeaderView::section { background: @surface2; color: @text2; border: 0; border-bottom: 1px solid @border; padding: 6px 8px; font-weight: 600; font-size: 12px; }
QTableCornerButton::section { background: @surface2; border: 0; }

/* Tabs */
QTabWidget::pane { border: 0; border-top: 1px solid @border; top: -1px; }
QTabBar { qproperty-drawBase: 0; }
QTabBar::tab { background: transparent; border: 0; border-bottom: 2px solid transparent; padding: 8px 14px; margin-right: 4px; color: @text2; }
QTabBar::tab:hover { color: @text; }
QTabBar::tab:selected { color: @accentText; border-bottom: 2px solid @accent; font-weight: 600; }
QTabBar::tab:focus { color: @accentText; }

/* Scroll bars: thin, out of the way */
QScrollBar:vertical { background: transparent; width: 10px; margin: 2px; }
QScrollBar:horizontal { background: transparent; height: 10px; margin: 2px; }
QScrollBar::handle { background: rgba(127,140,160,0.45); border-radius: 3px; min-height: 30px; min-width: 30px; }
QScrollBar::handle:hover { background: rgba(127,140,160,0.75); }
QScrollBar::add-line, QScrollBar::sub-line, QScrollBar::add-page, QScrollBar::sub-page { background: none; border: 0; width: 0; height: 0; }

/* Status bar */
QStatusBar { background: @bg; border-top: 1px solid @border; color: @text2; }
QStatusBar::item { border: 0; }
QMenu { background: @surface; border: 1px solid @border; border-radius: 8px; padding: 4px; }
QMenu::item { padding: 6px 16px; border-radius: 6px; }
QMenu::item:selected { background: @accentWeak; color: @accentText; }
)");
    const std::pair<const char *, const char *> vars[] = {
        {"@borderStrong", t.borderStrong}, {"@accentHover", t.accentHover}, {"@accentWeak", t.accentWeak}, {"@accentText", t.accentText},
        {"@surface2", t.surface2},         {"@onAccent", t.onAccent},       {"@surface", t.surface},       {"@border", t.border},
        {"@accent", t.accent},             {"@danger", t.danger},           {"@text2", t.text2},           {"@text3", t.text3},
        {"@text", t.text},                 {"@rail", t.rail},               {"@bg", t.bg}};
    for (const auto &[k, v] : vars)
        s.replace(QLatin1String(k), QLatin1String(v));
    return s;
}

QPalette palette(const Tokens &t)
{
    QPalette p;
    const auto c = [](const char *hex) { return QColor(QLatin1String(hex)); };
    p.setColor(QPalette::Window, c(t.bg));
    p.setColor(QPalette::WindowText, c(t.text));
    p.setColor(QPalette::Base, c(t.surface));
    p.setColor(QPalette::AlternateBase, c(t.surface2));
    p.setColor(QPalette::Text, c(t.text));
    p.setColor(QPalette::PlaceholderText, c(t.text3));
    p.setColor(QPalette::Button, c(t.surface));
    p.setColor(QPalette::ButtonText, c(t.text));
    p.setColor(QPalette::Highlight, c(t.accent));
    p.setColor(QPalette::HighlightedText, c(t.onAccent));
    p.setColor(QPalette::Link, c(t.accentText));
    p.setColor(QPalette::ToolTipBase, c(t.text));
    p.setColor(QPalette::ToolTipText, c(t.surface));
    p.setColor(QPalette::Mid, c(t.border));
    p.setColor(QPalette::Dark, c(t.borderStrong));
    for (QPalette::ColorRole r : {QPalette::WindowText, QPalette::Text, QPalette::ButtonText})
        p.setColor(QPalette::Disabled, r, c(t.text3));
    return p;
}

void setFont(QApplication &app)
{
    QFont f = QGuiApplication::font();
#ifdef Q_OS_WIN
    // Windows 11's UI font, where it is installed.
    if (QFontDatabase::families().contains(QStringLiteral("Segoe UI Variable Text")))
        f.setFamily(QStringLiteral("Segoe UI Variable Text"));
#endif
    // 14 px body text, as the web app.
    f.setPixelSize(14);
    app.setFont(f);
}

void polish(QApplication &app)
{
    const Tokens &t = g_dark ? dark : light;
    app.setPalette(palette(t));
    app.setStyleSheet(sheet(t));
}

} // namespace

namespace Theme {

void apply(QApplication &app)
{
    g_active = qEnvironmentVariable("INTERLINEA_NATIVE_STYLE") != QLatin1String("1") && !highContrast();
    if (!g_active)
        return;
    // Fusion draws the same on every system, so the sheet looks the same.
    app.setStyle(QStyleFactory::create(QStringLiteral("Fusion")));
    setFont(app);
    g_dark = systemDark();
    polish(app);
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    QObject::connect(QGuiApplication::styleHints(), &QStyleHints::colorSchemeChanged, &app, [&app](Qt::ColorScheme s) {
        if (s == Qt::ColorScheme::Unknown)
            return;
        g_dark = s == Qt::ColorScheme::Dark;
        polish(app);
    });
#endif
}

bool active()
{
    return g_active;
}

bool dark()
{
    return g_dark;
}

void primary(QWidget *button)
{
    button->setProperty("primary", true);
}

void title(QWidget *label)
{
    label->setProperty("role", QStringLiteral("title"));
    label->setAccessibleName(label->property("text").toString());
    if (!g_active) {
        QFont f = label->font();
        f.setPointSizeF(f.pointSizeF() * 1.5);
        f.setWeight(QFont::DemiBold);
        label->setFont(f);
    }
}

void secondary(QWidget *label)
{
    label->setProperty("role", QStringLiteral("secondary"));
}

} // namespace Theme
