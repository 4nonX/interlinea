#pragma once

#include <QString>

// The two wordings of the branching vocabulary, as the web app's Settings
// offer them: plain ("Review and merge", "Based on") or as in version
// control ("Diff and merge", "Branched from").
namespace Words {
bool git();
void setGit(bool git);
// The word for a key: projectChanges, reviewMerge, mergeInto, merged,
// updateFromMaster, updated, history, basedOn, projectTermbase, useProject,
// keepMaster, keepBoth, skip, accept, newEntry, changedEntry, conflict,
// change, changes.
QString get(const char *key);
}
