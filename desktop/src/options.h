#pragma once

#include <QString>

// The person's choices that the web app's Settings page offers, kept with
// the app's settings on this computer.
namespace Options {
// Insert the best memory match when an empty segment opens.
bool autoInsert();
void setAutoInsert(bool on);
// Offer the terms and memory words that complete a word being typed.
bool autoComplete();
void setAutoComplete(bool on);
// Fill repetitions when a segment is confirmed.
bool autoPropagate();
void setAutoPropagate(bool on);
// How new projects count context matches: "previous" (the segment before
// also matches, as Trados) or "both" (before and after, as memoQ).
QString contextRule();
void setContextRule(const QString &rule);
}
