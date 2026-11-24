#pragma once

#include <QColor>
#include <QJsonObject>
#include <QString>

class QTableWidget;
class QWidget;

// Match scores and analysis bands as the web app shows them
// (editor/analysis.ts, components/AnalysisTable.tsx).
namespace Bands {

// "cm", "100", "95", "85", "75", "50" or "new".
QString ofScore(int score, bool context = false);
// "CM" or "87%".
QString scoreLabel(int score, bool context = false);
// "Exact match (100%)", "Fuzzy match, 87%", …
QString scoreDescription(int score, bool context = false);
// The badge colours of a band (also the analysis bands "rep", "locked").
struct Colors {
    QColor background;
    QColor text;
    QColor border;
};
Colors colors(const QString &band);
// A small badge label, as the web's .score.
QWidget *badge(const QString &text, const QString &band, QWidget *parent);

// The analysis table of one report (Match, Segments, Words, Share, and a
// Total row), as components/AnalysisTable.tsx.
QTableWidget *analysisTable(const QJsonObject &report, QWidget *parent);

} // namespace Bands
