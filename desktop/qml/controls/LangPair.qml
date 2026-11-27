import QtQuick
import Interlinea

// "de-DE → en-GB en-US": the source language and the targets.
Flow {
    id: pair
    property string source
    property var targets: []
    spacing: 4
    Accessible.role: Accessible.StaticText
    Accessible.name: qsTr("%1 to %2").arg(Backend.languageName(source)).arg(targets.map(t => Backend.languageName(t)).join(", "))
    LangChip { code: pair.source }
    Icon { name: "arrow-right"; color: Theme.text3; size: 14; y: 2 }
    Repeater {
        model: pair.targets
        LangChip { required property string modelData; code: modelData }
    }
}
