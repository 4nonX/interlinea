#include "theme.h"

#include "icons.h"

#include <QRegularExpression>
#include <QApplication>

#include <algorithm>
#include <vector>
#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
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
    // The web app's status and editor colours (styles.css).
    const char *ok, *okBg, *warn, *warnBg, *dangerBg, *server, *serverBg, *rowLine, *activeRow, *termHit, *termLine, *mark, *tagBg,
        *count, *track;
};

const Tokens light{"#f3f5f8", "#e8ecf1", "#ffffff", "#f7f9fb", "#d8dde4", "#b9c1cc", "#1a1f26", "#4c5562", "#66707c",
                   "#1f5bd6", "#1a4db6", "#e8effd", "#1a4aae", "#ffffff", "#aa2219",
                   "#17703f", "#e5f3eb", "#7f5300", "#fff2d4", "#fce8e6", "#4a3aa8", "#eeebfb", "#e9ecf0", "#f4f7fd",
                   "#eef3fd", "#8fb0ef", "#ffe3a3", "#ebe7f7", "#e1e6ed", "#e3e7ed"};
const Tokens dark{"#14171c", "#1a1e24", "#1e232a", "#232931", "#2f3640", "#4a5463", "#e6e9ee", "#b6bec9", "#98a1ad",
                  "#6b9bff", "#86adff", "#1f2c47", "#a9c4ff", "#0c1220", "#ff8a80",
                  "#5fd08f", "#1c3326", "#f0c060", "#3a2f17", "#3d1f1d", "#b7a9ff", "#2a2545", "#2a3038", "#1d2636",
                  "#1f2a40", "#4e6fb0", "#6b5a1f", "#2e2a45", "#2f3640", "#2f3640"};

const Tokens &tokens()
{
    return g_dark ? dark : light;
}

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

/* Navigation (.rail) */
QWidget#side { background: @rail; border-right: 1px solid @border; }
QListWidget#navigation { background: transparent; border: 0; padding: 0; font-size: 14px; outline: 0; }
QListWidget#navigation::item { border-radius: 6px; padding: 7px 8px; margin: 0 0 2px 0; color: @text2; font-weight: 500; }
QListWidget#navigation::item:hover { background: rgba(26,31,38,0.06); color: @text; }
QListWidget#navigation::item:selected { background: @surface; color: @text; }
QListWidget#navigation::item:focus { border: 2px solid @accent; }
QLabel#appName { font-size: 16px; font-weight: 650; color: @text; background: transparent; }
QLabel#brandMark { background: @accent; border-radius: 7px; }

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
QPushButton:disabled { color: dim(@text); border-color: dim(@borderStrong); background: dim(@surface); }
QPushButton[primary="true"] { background: @accent; border: 1px solid @accent; color: @onAccent; font-weight: 600; }
QPushButton[primary="true"]:hover { background: @accentHover; border-color: @accentHover; }
QPushButton[primary="true"]:focus { border: 2px solid @text; }
QPushButton[primary="true"]:disabled { background: dim(@accent); border-color: transparent; color: @onAccent; }
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

/* As the web app's .btn-small and .btn-quiet */
QPushButton[small="true"] { padding: 5px 10px; min-height: 16px; font-size: 13px; font-weight: 500; }
QPushButton[small="true"]:focus { padding: 4px 9px; }
QPushButton[quiet="true"] { border-color: transparent; background: transparent; color: @accentText; }
QPushButton[quiet="true"]:hover { background: @accentWeak; border-color: transparent; }
QPushButton[quiet="true"]:focus { border: 2px solid @accent; }
QPushButton[danger="true"] { border-color: @danger; background: @danger; color: #ffffff; }
QPushButton[danger="true"]:hover { background: #8f1c15; border-color: #8f1c15; }
QPushButton[quiet="true"]:disabled { color: dim(@accentText); background: transparent; border-color: transparent; }
QPushButton[danger="true"]:disabled { background: dim(@danger); border-color: transparent; color: #ffffff; }
QLabel[role="fieldLabel"] { color: @text2; font-size: 13px; font-weight: 600; }
QLabel[role="error"], QLabel[role="confirmText"] { color: @danger; font-size: 13px; font-weight: 500; }
QLabel[role="confirmPlain"] { color: @text; font-size: 13px; font-weight: 500; }
QPushButton[tiny="true"] { padding: 2px 8px; min-height: 16px; font-size: 12px; border-color: transparent; background: transparent; color: inherit; }
QPushButton[tiny="true"]:hover { background: rgba(127,140,160,0.16); }

/* Labels as the web's chips */
QLabel[kbd="true"] { border: 1px solid @border; border-bottom: 2px solid @border; border-radius: 4px; background: @surface2; color: @text3; font-size: 11px; padding: 0 4px; }
QLabel[chip="lang"] { border: 1px solid @border; border-radius: 4px; color: @text2; font-size: 11px; font-weight: 600; padding: 0 5px; }
QLabel[role="muted"] { color: @text3; }
QLabel[role="h1"] { font-size: 16px; font-weight: 600; }
QLabel[role="meta"] { color: @text3; font-size: 13px; }
QLabel[role="sectionTitle"] { color: @text2; font-size: 13px; font-weight: 600; }
QLabel[role="count"] { background: @count; color: @text2; border-radius: 8px; font-size: 11px; padding: 0 6px; }
QLabel[role="progress"] { color: @text3; font-size: 12px; }

/* Pages (.main, .page, .page-header, .section, .panel) */
QScrollArea#page, QWidget#pageOuter { background: @bg; border: 0; }
QLabel[role="pageTitle"] { font-size: 22px; font-weight: 650; color: @text; }
QLabel[role="sectionTitle16"] { font-size: 16px; font-weight: 600; color: @text; }
QLabel[role="panelTitle"] { font-size: 18px; font-weight: 600; color: @text; }
QLabel[role="cellSub"] { color: @text3; font-size: 13px; }
QLabel[role="help"] { color: @text2; font-size: 13px; }
QFrame#panel { background: @surface; border: 1px solid @border; border-radius: 10px; }
QFrame#choice { background: @surface; border: 1px solid @border; border-radius: 6px; }
QFrame#choice:hover { border-color: @borderStrong; }
QFrame#choice[checked="true"] { border-color: @accent; background: @choiceBg; }
QFrame#choice QRadioButton, QFrame#choice QCheckBox, QFrame#choice QLabel { background: transparent; }
QFrame#dropzone { background: @surface; border: 2px dashed @borderStrong; border-radius: 10px; }
QFrame#dropzone[dragging="true"] { border-color: @accent; background: @accentWeak; }
QTableWidget#webTable { background: @surface; border: 1px solid @border; border-radius: 10px; }
QTableWidget#webTable:focus { border: 1px solid @border; }
QTableWidget#webTable::item { padding: 0; border: 0; border-bottom: 1px solid @border; }
QTableWidget#webTable::item:hover { background: transparent; }
QTableWidget#webTable QHeaderView::section { background: @surface2; color: @text2; font-size: 13px; font-weight: 600; padding: 10px 12px; border: 0; border-bottom: 1px solid @border; }

/* .select-small */
QComboBox[small="true"] { padding: 1px 6px; min-height: 16px; font-size: 13px; }
QComboBox[small="true"]:focus { padding: 0 5px; }

/* The segmented control (.segmented) */
QFrame#segmented { border: 1px solid @border; border-radius: 8px; background: @surface2; }
QFrame#segmented QPushButton { border: 0; border-radius: 6px; background: transparent; padding: 3px 10px; min-height: 16px; font-size: 13px; font-weight: 500; color: @text2; }
QFrame#segmented QPushButton:checked { background: @surface; color: @text; }
QFrame#segmented QPushButton:focus { border: 2px solid @accent; padding: 1px 8px; }

/* Terminology (.workspace, .pane, .tb-link, .entry-row, .tabs, .term) */
QFrame[role="pane"] { background: @surface; border: 1px solid @border; border-radius: 10px; }
QFrame[role="paneHead"], QFrame[role="paneBar"], QFrame[role="tabs"] { background: transparent; border: 0; border-bottom: 1px solid @border; }
QFrame[role="paneForm"] { background: @surface2; border: 0; border-bottom: 1px solid @border; }
QFrame[role="formTop"] { background: transparent; border: 0; border-top: 1px solid @border; }
QLabel[role="paneTitle"] { font-size: 16px; font-weight: 600; color: @text; }
QLabel[role="groupTitle"] { color: @text3; font-size: 13px; font-weight: 600; }
QLabel[role="tbName"] { font-weight: 600; }
QLabel[role="tbCount"] { color: @text3; font-size: 12px; }
QLabel[role="missing"] { color: @text3; font-style: italic; }
QLabel[role="langTitle"] { font-size: 14px; font-weight: 600; }
QLabel[role="termText"] { font-size: 16px; font-weight: 600; }
QLabel[role="entryTerm"] { font-weight: 600; }
QLabel[role="small2"] { color: @text2; font-size: 13px; }
QLabel[role="historySummary"] { font-weight: 600; }
QLabel[role="emptyTitle"] { font-weight: 600; color: @text; }
QLabel[role="dialogTitle"] { font-size: 17px; font-weight: 600; color: @text; }
QLabel[role="settingsTitle"] { font-size: 14px; font-weight: 600; }
QLabel[role="dangerTitle"] { font-size: 14px; font-weight: 600; color: @danger; }
QLabel[role="planCount"] { font-size: 18px; font-weight: 600; }
QLabel[role="planLabel"] { color: @text2; font-size: 12px; }
QDialog[role="dialog"] { background: @surface; }
QFrame[role="tbLink"], QFrame[role="dupEntry"] { border: 0; border-radius: 6px; background: transparent; }
QFrame[role="tbLink"]:hover, QFrame[role="dupEntry"]:hover { background: @surface2; }
QFrame[role="tbLink"][current="true"] { background: @accentWeak; }
QFrame[role="tbLink"]:focus, QFrame[role="dupEntry"]:focus { border: 2px solid @accent; }
QFrame[role="tbLink"] QLabel, QFrame[role="dupEntry"] QLabel { background: transparent; }
QFrame[role="term"] { background: @surface2; border: 0; border-radius: 6px; }
QFrame[role="term"] QLabel { background: transparent; }
QFrame[role="box"] { background: @surface; border: 1px solid @border; border-radius: 6px; }
QFrame[role="line"] { background: @border; border: 0; max-height: 1px; min-height: 1px; }
QFrame[role="historyItem"] { border: 0; border-left: 2px solid @border; background: transparent; }
QFrame[role="quote"] { background: @surface2; border: 0; border-radius: 6px; }
QFrame[note="warn"] { background: @warnBg; border: 0; border-radius: 6px; }
QFrame[note="ok"] { background: @okBg; border: 0; border-radius: 6px; }
QFrame[note="info"] { background: @surface2; border: 0; border-radius: 6px; }
QPushButton[tab] { border: 0; border-bottom: 2px solid transparent; border-radius: 0; background: transparent; color: @text2; font-weight: 600; padding: 10px 8px 8px 8px; }
QPushButton[tab="small"] { font-size: 13px; }
QPushButton[tab]:hover { background: transparent; color: @text; }
QPushButton[tab]:checked { border-bottom: 2px solid @accent; color: @text; }
QPushButton[tab]:focus { border: 0; border-bottom: 2px solid @accent; padding: 10px 8px 8px 8px; color: @accentText; }
QPushButton[tab]:disabled { color: dim(@text2); background: transparent; }
QPushButton[chipButton="true"] { padding: 2px 8px; min-height: 14px; border: 1px solid @border; border-radius: 10px; background: @surface; color: @text2; font-size: 12px; }
QPushButton[chipButton="true"]:hover { border-color: @borderStrong; }
QPushButton[iconButton="true"] { border: 0; border-radius: 4px; background: transparent; padding: 0; min-height: 22px; max-height: 22px; min-width: 22px; max-width: 22px; }
QPushButton[iconButton="true"]:hover { background: rgba(127,140,160,0.18); }
QPushButton[iconButton="true"]:focus { border: 2px solid @accent; padding: 0; }
QPushButton[link="true"] { border: 0; background: transparent; color: @accentText; padding: 0; min-height: 0; text-align: left; font-size: 13px; }
QPushButton[link="true"]:hover { text-decoration: underline; background: transparent; }
QPushButton[link="true"]:focus { border: 0; padding: 0; text-decoration: underline; }
QLabel[stepNum] { border: 1px solid @borderStrong; border-radius: 11px; font-size: 12px; color: @text3; }
QLabel[stepNum="current"] { background: @accent; border-color: @accent; color: #ffffff; }
QLabel[stepNum="done"] { border-color: transparent; color: @ok; }
QLabel[stepName] { color: @text3; font-size: 13px; font-weight: 500; }
QLabel[stepName="current"] { color: @text; font-weight: 600; }
QLineEdit[small="true"] { padding: 2px 8px; min-height: 18px; font-size: 13px; }
QLineEdit[small="true"]:focus { padding: 1px 7px; }
QLineEdit[invalid="true"] { border-color: @danger; }
QListView#entryList { border: 0; border-radius: 0; background: transparent; }
QListView#entryList:focus { border: 0; }
QListView#entryList::item { padding: 0; border: 0; }
QScrollArea[role="paneScroll"], QWidget[role="paneBody"] { background: transparent; border: 0; }
QTableWidget#webTable[inDialog="true"] { border-radius: 6px; }

/* The editor */
QFrame#editorBar { background: @surface; border: 0; border-bottom: 1px solid @border; }
QProgressBar#track { background: @track; border: 0; border-radius: 3px; max-height: 6px; min-height: 6px; }
QProgressBar#track::chunk { background: @ok; border-radius: 3px; }
QTableView#segments { border: 0; border-radius: 0; background: @surface; }
QTableView#segments:focus { border: 0; }
QScrollArea#results, QWidget#resultsBody { background: @bg; border: 0; }
QScrollArea#results { border-left: 1px solid @border; }
QFrame#result { text-align: left; border: 1px solid @border; border-radius: 6px; background: @surface; padding: 8px 12px; }
QFrame#result:hover { border-color: @borderStrong; }
QFrame#result:focus, QFrame#result[selected="true"] { border: 2px solid @accent; padding: 7px 11px; }
QFrame#issue { background: @warnBg; border-radius: 6px; }
QFrame#issue QLabel { color: @warn; }
QFrame#issue[forbidden="true"] { background: @dangerBg; }
QFrame#issue[forbidden="true"] QLabel { color: @danger; }
QTextEdit#targetEditor { background: @surface; border: 2px solid @accent; border-radius: 0; padding: 10px; }
QTextBrowser#activeSource { background: transparent; border: 0; padding: 0; }
)");
    const std::pair<const char *, const char *> vars[] = {
        {"@borderStrong", t.borderStrong}, {"@accentHover", t.accentHover}, {"@accentWeak", t.accentWeak}, {"@accentText", t.accentText},
        {"@surface2", t.surface2},         {"@onAccent", t.onAccent},       {"@surface", t.surface},       {"@border", t.border},
        {"@accent", t.accent},             {"@danger", t.danger},           {"@text2", t.text2},           {"@text3", t.text3},
        {"@dangerBg", t.dangerBg},         {"@warnBg", t.warnBg},           {"@okBg", t.okBg},             {"@count", t.count},
        {"@track", t.track},               {"@choiceBg", g_dark ? "#1d2636" : "#f7faff"},               {"@warn", t.warn},               {"@ok", t.ok},
        {"@text", t.text},                 {"@rail", t.rail},               {"@bg", t.bg}};
    // Longer names first, so "@dangerBg" is not read as "@danger" + "Bg".
    std::vector<std::pair<const char *, const char *>> sorted(std::begin(vars), std::end(vars));
    std::sort(sorted.begin(), sorted.end(), [](const auto &a, const auto &b) { return qstrlen(a.first) > qstrlen(b.first); });
    for (const auto &[k, v] : sorted)
        s.replace(QLatin1String(k), QLatin1String(v));
    // dim(colour): the colour at the web's opacity of what is disabled.
    static const QRegularExpression dim(QStringLiteral("dim\\((#[0-9a-fA-F]{6})\\)"));
    for (auto m = dim.match(s); m.hasMatch(); m = dim.match(s)) {
        const QColor c(m.captured(1));
        s.replace(m.capturedStart(), m.capturedLength(), QStringLiteral("rgba(%1, %2, %3, 0.55)").arg(c.red()).arg(c.green()).arg(c.blue()));
    }
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
    // INTERLINEA_THEME=light or dark sets it; otherwise the system does.
    const QString forced = qEnvironmentVariable("INTERLINEA_THEME");
    g_dark = forced.isEmpty() ? systemDark() : forced == QLatin1String("dark");
    polish(app);
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    QObject::connect(QGuiApplication::styleHints(), &QStyleHints::colorSchemeChanged, &app, [&app](Qt::ColorScheme s) {
        if (s == Qt::ColorScheme::Unknown || !qEnvironmentVariable("INTERLINEA_THEME").isEmpty())
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

void role(QWidget *w, const char *role)
{
    w->setProperty("role", QLatin1String(role));
}

void smallButton(QWidget *button)
{
    button->setProperty("small", true);
}

void quietButton(QWidget *button)
{
    button->setProperty("quiet", true);
}

QColor color(const char *name)
{
    const Tokens &t = tokens();
    const std::pair<const char *, const char *> all[] = {
        {"bg", t.bg},           {"surface", t.surface},     {"surface2", t.surface2},   {"border", t.border},
        {"borderStrong", t.borderStrong}, {"text", t.text}, {"text2", t.text2},         {"text3", t.text3},
        {"accent", t.accent},   {"accentWeak", t.accentWeak}, {"accentText", t.accentText}, {"danger", t.danger},
        {"dangerBg", t.dangerBg}, {"ok", t.ok},             {"okBg", t.okBg},           {"warn", t.warn},
        {"warnBg", t.warnBg},   {"server", t.server},       {"serverBg", t.serverBg},   {"rowLine", t.rowLine},
        {"activeRow", t.activeRow}, {"termHit", t.termHit}, {"termLine", t.termLine},   {"mark", t.mark},
        {"tagBg", t.tagBg},     {"count", t.count}};
    for (const auto &[k, v] : all) {
        if (qstrcmp(k, name) == 0)
            return QColor(QLatin1String(v));
    }
    return QColor(QLatin1String(t.text));
}

namespace {
// A button sized by what it holds: icon, text and key chip.
class KeyButton : public QPushButton {
public:
    using QPushButton::QPushButton;
    QSize sizeHint() const override { return layout() ? layout()->sizeHint().expandedTo(QSize(0, 28)) : QPushButton::sizeHint(); }
    QSize minimumSizeHint() const override { return sizeHint(); }
};
}

QPushButton *keyButton(const QString &text, const QString &keys, const QString &icon, QWidget *parent)
{
    auto *b = new KeyButton(parent);
    smallButton(b);
    b->setAccessibleName(text);
    b->setToolTip(keys);
    auto *l = new QHBoxLayout(b);
    l->setContentsMargins(9, 4, 5, 4);
    l->setSpacing(5);
    if (!icon.isEmpty()) {
        auto *i = new QLabel(b);
        i->setPixmap(Icons::get(icon).pixmap(14, 14));
        i->setAttribute(Qt::WA_TransparentForMouseEvents);
        l->addWidget(i);
    }
    auto *t = new QLabel(text, b);
    t->setAttribute(Qt::WA_TransparentForMouseEvents);
    t->setStyleSheet(QStringLiteral("font-size: 13px; font-weight: 500; background: transparent;"));
    l->addWidget(t);
    auto *k = new QLabel(keys, b);
    k->setProperty("kbd", true);
    k->setAttribute(Qt::WA_TransparentForMouseEvents);
    l->addWidget(k);
    return b;
}

} // namespace Theme
