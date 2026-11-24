#include "words.h"

#include <QCoreApplication>
#include <QSettings>

#include <cstring>

namespace Words {

bool git()
{
    return QSettings().value(QStringLiteral("words/git"), false).toBool();
}

void setGit(bool git)
{
    QSettings().setValue(QStringLiteral("words/git"), git);
}

QString get(const char *key)
{
    struct Word {
        const char *key;
        const char *plain;
        const char *git;
    };
    static const Word words[] = {
        {"projectChanges", QT_TRANSLATE_NOOP("Words", "Project changes"), QT_TRANSLATE_NOOP("Words", "Branch diff")},
        {"reviewMerge", QT_TRANSLATE_NOOP("Words", "Review and merge"), QT_TRANSLATE_NOOP("Words", "Diff and merge")},
        {"mergeInto", QT_TRANSLATE_NOOP("Words", "Merge into"), QT_TRANSLATE_NOOP("Words", "Merge into")},
        {"merged", QT_TRANSLATE_NOOP("Words", "Merged"), QT_TRANSLATE_NOOP("Words", "Merged")},
        {"updateFromMaster", QT_TRANSLATE_NOOP("Words", "Update from master"), QT_TRANSLATE_NOOP("Words", "Pull from master")},
        {"updated", QT_TRANSLATE_NOOP("Words", "Project updated from master"), QT_TRANSLATE_NOOP("Words", "Pulled from master")},
        {"history", QT_TRANSLATE_NOOP("Words", "History"), QT_TRANSLATE_NOOP("Words", "Log")},
        {"basedOn", QT_TRANSLATE_NOOP("Words", "Based on"), QT_TRANSLATE_NOOP("Words", "Branched from")},
        {"projectTermbase", QT_TRANSLATE_NOOP("Words", "Project termbase"), QT_TRANSLATE_NOOP("Words", "Branch")},
        {"projectTermbases", QT_TRANSLATE_NOOP("Words", "Project termbases"), QT_TRANSLATE_NOOP("Words", "Branches")},
        {"useProject", QT_TRANSLATE_NOOP("Words", "Use project version"), QT_TRANSLATE_NOOP("Words", "Use ours")},
        {"keepMaster", QT_TRANSLATE_NOOP("Words", "Keep master version"), QT_TRANSLATE_NOOP("Words", "Use theirs")},
        {"keepBoth", QT_TRANSLATE_NOOP("Words", "Keep both"), QT_TRANSLATE_NOOP("Words", "Union")},
        {"skip", QT_TRANSLATE_NOOP("Words", "Leave out"), QT_TRANSLATE_NOOP("Words", "Drop")},
        {"accept", QT_TRANSLATE_NOOP("Words", "Include"), QT_TRANSLATE_NOOP("Words", "Pick")},
        {"newEntry", QT_TRANSLATE_NOOP("Words", "New entry"), QT_TRANSLATE_NOOP("Words", "Added")},
        {"changedEntry", QT_TRANSLATE_NOOP("Words", "Changed"), QT_TRANSLATE_NOOP("Words", "Modified")},
        {"conflict", QT_TRANSLATE_NOOP("Words", "Conflict"), QT_TRANSLATE_NOOP("Words", "Conflict")},
        {"change", QT_TRANSLATE_NOOP("Words", "change"), QT_TRANSLATE_NOOP("Words", "commit")},
        {"changes", QT_TRANSLATE_NOOP("Words", "changes"), QT_TRANSLATE_NOOP("Words", "commits")},
    };
    const bool g = git();
    for (const Word &w : words) {
        if (std::strcmp(w.key, key) == 0)
            return QCoreApplication::translate("Words", g ? w.git : w.plain);
    }
    return QString::fromLatin1(key);
}

} // namespace Words
