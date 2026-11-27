#include "colors.h"

#include "options.h"

#include <QGuiApplication>
#include <QPalette>
#include <QStyleHints>

namespace {

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
const Tokens darkTokens{"#14171c", "#1a1e24", "#1e232a", "#232931", "#2f3640", "#4a5463", "#e6e9ee", "#b6bec9", "#98a1ad",
                  "#6b9bff", "#86adff", "#1f2c47", "#a9c4ff", "#0c1220", "#ff8a80",
                  "#5fd08f", "#1c3326", "#f0c060", "#3a2f17", "#3d1f1d", "#b7a9ff", "#2a2545", "#2a3038", "#1d2636",
                  "#1f2a40", "#4e6fb0", "#6b5a1f", "#2e2a45", "#2f3640", "#2f3640"};

} // namespace

namespace Theme {

bool dark()
{
    // The choice made in Settings, then the system's setting.
    const QString chosen = Options::appearance();
    if (!chosen.isEmpty())
        return chosen == QLatin1String("dark");
    const Qt::ColorScheme s = QGuiApplication::styleHints()->colorScheme();
    if (s != Qt::ColorScheme::Unknown)
        return s == Qt::ColorScheme::Dark;
    return QGuiApplication::palette().color(QPalette::Window).lightness() < 128;
}

QColor color(const char *name)
{
    const Tokens &t = dark() ? darkTokens : light;
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

} // namespace Theme
