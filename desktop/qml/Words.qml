pragma Singleton
import QtQuick
import Interlinea

// The branching vocabulary in the wording chosen in Settings: plain
// ("Review and merge") or as in version control ("Diff and merge").
QtObject {
    function get(key) {
        Backend.gitWords // the texts follow the choice
        return Backend.word(key)
    }
}
