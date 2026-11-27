import QtQuick
import QtQuick.Layouts
import QtQuick.Templates as T
import QtQuick.Dialogs
import QtCore
import Interlinea

// The window: the menu bar, the navigation down the side as in the web
// app, and the page.
T.ApplicationWindow {
    id: window
    width: 1280
    height: 800
    minimumWidth: 800
    minimumHeight: 520
    visible: true
    title: "Interlinea"
    color: Theme.bg
    font.family: Theme.font
    font.pixelSize: Theme.body

    property string page: "projects"
    // Where the window was and how large, for the next start.
    Settings {
        category: "window"
        property alias x: window.x
        property alias y: window.y
        property alias width: window.width
        property alias height: window.height
    }
    readonly property var local: Backend.local
    readonly property bool admin: Backend.connections.some(c => !c.local && c.ready && c.admin)

    // The rail shows where a page belongs: New project, a project and
    // its editor are under Projects.
    readonly property string section: ["new", "project", "editor"].indexOf(page) >= 0 ? "projects" : page
    function show(id) {
        page = id
    }
    function newProject(files) {
        newProjectPage.start(files)
        show("new")
    }
    function openProject(cid, id) {
        projectPage.open(cid, id)
        show("project")
    }
    function openEditor(cid, id, file, lang) {
        editorPage.open(cid, id, file, lang)
        show("editor")
    }
    // Documents opened with Interlinea start a project; a later start
    // brings the window up.
    Connections {
        target: Backend
        function onFilesOpened(files) {
            if (files.length) window.newProject(files)
            if (window.visibility === Window.Minimized) window.showNormal()
            window.raise()
            window.requestActivate()
        }
    }
    // A project termbase's changes, to review and merge, in Terminology.
    function openMerge(cid, termbase) {
        terminology.open(cid, termbase)
        review.visible = false
        show("terminology")
    }

    menuBar: MenuBar {
        Menu {
            objectName: "fileMenu"
            title: qsTr("File")
            MenuItem { action: newProjectAction }
            MenuItem { action: openAction }
            MenuSeparator {}
            MenuItem { action: quitAction }
        }
        // Edit works on the field being typed in.
        Menu {
            objectName: "editMenu"
            title: qsTr("Edit")
            MenuItem { action: undoAction }
            MenuItem { action: redoAction }
            MenuSeparator {}
            MenuItem { action: cutAction }
            MenuItem { action: copyAction }
            MenuItem { action: pasteAction }
            MenuSeparator {}
            MenuItem { action: selectAllAction }
        }
        Menu {
            title: qsTr("View")
            Repeater {
                model: nav.model
                MenuItem {
                    required property var modelData
                    text: modelData.label
                    checkable: true
                    checked: window.section === modelData.id
                    onTriggered: window.show(modelData.id)
                }
            }
        }
        Menu {
            title: qsTr("Help")
            MenuItem { text: qsTr("Documentation"); onTriggered: Qt.openUrlExternally("https://github.com/4nonX/interlinea#readme") }
            MenuItem { text: qsTr("About Interlinea"); onTriggered: window.show("settings") }
        }
    }

    T.Action {
        id: newProjectAction
        text: qsTr("New project")
        shortcut: StandardKey.New
        onTriggered: window.newProject([])
    }
    T.Action {
        id: openAction
        text: qsTr("Open documents…")
        shortcut: StandardKey.Open
        onTriggered: openDialog.open()
    }
    FileDialog {
        id: openDialog
        title: qsTr("Open documents to translate")
        fileMode: FileDialog.OpenFiles
        currentFolder: StandardPaths.writableLocation(StandardPaths.DocumentsLocation)
        onAccepted: window.newProject(selectedFiles)
    }
    // The field with the keyboard, when it can do what is asked.
    readonly property var editing: activeFocusItem && typeof activeFocusItem.cut === "function" ? activeFocusItem : null
    T.Action { id: undoAction; readonly property string keys: Backend.keyText(StandardKey.Undo); text: qsTr("Undo"); enabled: !!window.editing && window.editing.canUndo; onTriggered: window.editing.undo() }
    T.Action { id: redoAction; readonly property string keys: Backend.keyText(StandardKey.Redo); text: qsTr("Redo"); enabled: !!window.editing && window.editing.canRedo; onTriggered: window.editing.redo() }
    T.Action { id: cutAction; readonly property string keys: Backend.keyText(StandardKey.Cut); text: qsTr("Cut"); enabled: !!window.editing && !window.editing.readOnly && window.editing.selectedText !== ""; onTriggered: window.editing.cut() }
    T.Action { id: copyAction; readonly property string keys: Backend.keyText(StandardKey.Copy); text: qsTr("Copy"); enabled: !!window.editing && window.editing.selectedText !== ""; onTriggered: window.editing.copy() }
    T.Action { id: pasteAction; readonly property string keys: Backend.keyText(StandardKey.Paste); text: qsTr("Paste"); enabled: !!window.editing && window.editing.canPaste; onTriggered: window.editing.paste() }
    T.Action { id: selectAllAction; readonly property string keys: Backend.keyText(StandardKey.SelectAll); text: qsTr("Select all"); enabled: !!window.editing; onTriggered: window.editing.selectAll() }
    T.Action {
        id: quitAction
        text: qsTr("Quit")
        shortcut: StandardKey.Quit
        onTriggered: Qt.quit()
    }

    Toast { id: toast; objectName: "toast" }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        // The web's .rail: 216 px, the brand over the pages.
        Rectangle {
            Layout.fillHeight: true
            Layout.preferredWidth: 216
            color: Theme.rail
            Accessible.role: Accessible.PageTabList
            Accessible.name: qsTr("Main")
            Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Theme.border }
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: Theme.s3
                anchors.topMargin: Theme.s4
                anchors.bottomMargin: Theme.s4
                spacing: Theme.s4
                Row {
                    Layout.leftMargin: Theme.s2
                    spacing: Theme.s2
                    BrandMark { anchors.verticalCenter: parent.verticalCenter }
                    Text {
                        text: "Interlinea"
                        color: Theme.text
                        font.family: Theme.font
                        font.pixelSize: 16
                        font.weight: 650
                        anchors.verticalCenter: parent.verticalCenter
                    }
                }
                Column {
                    id: nav
                    Layout.fillWidth: true
                    spacing: 2
                    readonly property var model: [
                        { id: "projects", label: qsTr("Projects"), icon: "projects", keys: "Ctrl+1" },
                        { id: "memories", label: qsTr("Memories"), icon: "memories", keys: "Ctrl+2" },
                        { id: "terminology", label: qsTr("Terminology"), icon: "terminology", keys: "Ctrl+3" }
                    ].concat(window.admin ? [{ id: "accounts", label: qsTr("Accounts"), icon: "accounts", keys: "Ctrl+4" }] : [])
                     .concat([{ id: "settings", label: qsTr("Settings"), icon: "settings", keys: "Ctrl+," }])
                    Repeater {
                        model: nav.model
                        RailItem {
                            required property var modelData
                            width: nav.width
                            text: modelData.label
                            iconName: modelData.icon
                            keys: modelData.keys
                            current: window.section === modelData.id
                            onClicked: window.show(modelData.id)
                            Shortcut { sequence: modelData.keys; onActivated: window.show(modelData.id) }
                        }
                    }
                }
                Item { Layout.fillHeight: true }
                Row {
                    Layout.leftMargin: Theme.s2
                    visible: Backend.gitWords
                    spacing: Theme.s1
                    Icon { name: "git-branch"; color: Theme.text2; size: 14; anchors.verticalCenter: parent.verticalCenter }
                    Txt { text: qsTr("Git terms on"); color: Theme.text2; font.pixelSize: Theme.tiny; font.weight: 500 }
                }
            }
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0
        // Something that stops work (the core not running) is said at the top.
        Note {
            objectName: "coreProblem"
            visible: !!window.local && window.local.state === 4
            Layout.fillWidth: true
            Layout.margins: Theme.s3
            kind: "warn"
            text: window.local ? Fmt.fill(qsTr("Work on this computer can't be opened: {reason}"), { reason: window.local.message }) : ""
            Accessible.name: qsTr("Message")
            Button { small: true; text: qsTr("Try again"); onClicked: Backend.restartCore() }
        }
        StackLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            currentIndex: ["projects", "memories", "terminology", "accounts", "settings", "new", "project", "editor"].indexOf(window.page)
            ProjectsPage {
                id: projectsPage
                onNewProject: files => window.newProject(files)
                onOpenProject: (c, id) => window.openProject(c, id)
                onOpenMerge: (c, id, tb) => window.openMerge(c, tb)
                onOpenEditor: (c, id) => window.openEditor(c, id, "", "")
            }
            MemoriesPage { objectName: "memoriesPage"; onNotice: text => toast.say(text) }
            // Terminology, with the review of a merge request in its place
            // while one is open.
            Item {
                objectName: "terminologyHost"
                function start(connectionId, request) { review.open(connectionId, Number(request)); review.visible = true }
                TerminologyPage {
                    id: terminology
                    anchors.fill: parent
                    visible: !review.visible
                    onNotice: text => toast.say(text)
                    onOpenReview: (c, tb, id) => { review.open(c, id); review.visible = true }
                }
                ReviewPage {
                    id: review
                    objectName: "review"
                    anchors.fill: parent
                    visible: false
                    onNotice: text => toast.say(text)
                    onBack: (c, tb) => { review.visible = false; if (tb) terminology.open(c, tb) }
                }
            }
            AccountsPage { onNotice: text => toast.say(text); onOpenSettings: window.page = "settings" }
            SettingsPage { onNotice: text => toast.say(text) }
            NewProjectPage {
                id: newProjectPage
                objectName: "newProjectPage"
                onNotice: text => toast.say(text)
                onCancelled: window.show("projects")
                onCreated: (c, id) => window.openProject(c, id)
            }
            ProjectPage {
                id: projectPage
                objectName: "projectPage"
                onNotice: text => toast.say(text)
                onBack: window.show("projects")
                onGone: window.show("projects")
                onOpenEditor: (c, id, file, lang) => window.openEditor(c, id, file, lang)
            }
            EditorPage {
                id: editorPage
                objectName: "editorPage"
                onNotice: text => toast.say(text)
                onBack: window.show("projects")
                onOpenProject: (c, id) => window.openProject(c, id)
                onOpenReview: (c, tb) => window.openMerge(c, tb)
            }
        }
        }
    }
}
