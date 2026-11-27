pragma Singleton
import QtQuick

// The design system the web app uses (web/src/styles.css, DESIGN.md):
// cool neutral shell, white work surfaces, one blue accent, 6 px controls
// on 10 px panels. The same on every system; light or dark as the
// system is set.
QtObject {
    id: theme

    readonly property bool dark: Application.styleHints.colorScheme === Qt.ColorScheme.Dark

    // Colours
    readonly property color bg: dark ? "#14171c" : "#f3f5f8"
    readonly property color rail: dark ? "#1a1e24" : "#e8ecf1"
    readonly property color surface: dark ? "#1e232a" : "#ffffff"
    readonly property color surface2: dark ? "#232931" : "#f7f9fb"
    readonly property color border: dark ? "#2f3640" : "#d8dde4"
    readonly property color borderStrong: dark ? "#4a5463" : "#b9c1cc"
    readonly property color borderHover: dark ? "#5c6676" : "#98a2ae"
    readonly property color text: dark ? "#e6e9ee" : "#1a1f26"
    readonly property color text2: dark ? "#b6bec9" : "#4c5562"
    readonly property color text3: dark ? "#98a1ad" : "#66707c"
    readonly property color accent: dark ? "#6b9bff" : "#1f5bd6"
    readonly property color accentHover: dark ? "#86adff" : "#1a4db6"
    readonly property color accentPressed: dark ? "#9dbcff" : "#173f95"
    readonly property color accentWeak: dark ? "#1f2c47" : "#e8effd"
    readonly property color accentText: dark ? "#a9c4ff" : "#1a4aae"
    readonly property color onAccent: dark ? "#0c1220" : "#ffffff"
    readonly property color ok: dark ? "#5fd08f" : "#17703f"
    readonly property color okBg: dark ? "#1c3326" : "#e5f3eb"
    readonly property color warn: dark ? "#f0c060" : "#7f5300"
    readonly property color warnBg: dark ? "#3a2f17" : "#fff2d4"
    readonly property color danger: dark ? "#ff8a80" : "#aa2219"
    readonly property color dangerBg: dark ? "#3d1f1d" : "#fce8e6"
    readonly property color server: dark ? "#b7a9ff" : "#4a3aa8"
    readonly property color serverBg: dark ? "#2a2545" : "#eeebfb"
    readonly property color badge: dark ? "#2a3038" : "#eceff3"
    // The editor's grid: the lines between rows, the open row, a term found,
    // a word marked, and a tag.
    readonly property color rowLine: dark ? "#2a3038" : "#e9ecf0"
    readonly property color activeRow: dark ? "#1d2636" : "#f4f7fd"
    readonly property color termHit: dark ? "#1f2a40" : "#eef3fd"
    readonly property color termLine: dark ? "#4e6fb0" : "#8fb0ef"
    readonly property color mark: dark ? "#6b5a1f" : "#ffe3a3"
    readonly property color tagBg: dark ? "#2e2a45" : "#ebe7f7"
    readonly property color hover: dark ? Qt.rgba(1, 1, 1, 0.06) : Qt.rgba(26 / 255, 31 / 255, 38 / 255, 0.06)
    readonly property color rowHover: dark ? "#222830" : "#fafbfc"
    readonly property color selection: dark ? "#2c4a80" : "#c9d9fb"
    readonly property color shadow: dark ? Qt.rgba(0, 0, 0, 0.5) : Qt.rgba(20 / 255, 30 / 255, 45 / 255, 0.14)

    // Shapes and spacing
    readonly property int rControl: 6
    readonly property int rPanel: 10
    readonly property int s1: 4
    readonly property int s2: 8
    readonly property int s3: 12
    readonly property int s4: 16
    readonly property int s5: 24
    readonly property int s6: 32
    readonly property int controlHeight: 32
    readonly property int smallHeight: 28

    // Type: the system's interface font at the web's sizes.
    readonly property string font: Qt.application.font.family
    readonly property int body: 14
    readonly property int small: 13
    readonly property int tiny: 12
    readonly property int h1: 22
    readonly property int h2: 18
    readonly property int h3: 16
}
