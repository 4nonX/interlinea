import QtQuick
import Interlinea

// Text in the interface font. role sets the look of a part of a page:
// "h1" (page title), "h2" (panel title), "h3" (section title), "muted"
// (secondary), "help", "sub" (grey line under a title), "error",
// "label" (a field's label), "strong".
Text {
    property string role: ""
    color: role === "error" ? Theme.danger
         : role === "muted" || role === "help" ? Theme.text2
         : role === "sub" ? Theme.text3
         : Theme.text
    font.family: Theme.font
    font.pixelSize: role === "h1" ? Theme.h1 : role === "h2" ? Theme.h2 : role === "h3" ? Theme.h3
                  : role === "help" || role === "sub" || role === "small" ? Theme.small : Theme.body
    font.weight: role === "h1" ? 650 : role === "h2" || role === "h3" || role === "label" || role === "strong" ? 600 : 400
    font.letterSpacing: role === "h1" ? -0.2 : 0
    lineHeight: 1.12
    wrapMode: Text.Wrap
    textFormat: Text.PlainText
    Accessible.role: Accessible.StaticText
    Accessible.name: text
}
