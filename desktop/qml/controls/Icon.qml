import QtQuick
import Interlinea

// One of the app's line icons, in a colour, sharp at any scale.
Image {
    id: icon
    property string name
    property color color: Theme.text
    property int size: 16
    width: size
    height: size
    visible: name !== ""
    source: name === "" ? "" : "image://icon/" + name + "?color=" + encodeURIComponent(color.toString())
    sourceSize: Qt.size(Math.ceil(size * Screen.devicePixelRatio), Math.ceil(size * Screen.devicePixelRatio))
    smooth: true
    asynchronous: false
    Accessible.ignored: true
}
