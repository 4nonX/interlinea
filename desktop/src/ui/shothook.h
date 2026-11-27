#pragma once

class QQuickWindow;
class Backend;

// For checking the layout without a screen: with INTERLINEA_SHOT=<file>
// the window is saved as a picture once it has settled
// (INTERLINEA_SHOT_AFTER milliseconds, 4000 by default) and the app quits.
//
//   INTERLINEA_SHOT_SIGNIN  <address>,<username>,<password>: signs in to a
//                           team server first
//   INTERLINEA_SHOT_PAGE    the page to show
//   INTERLINEA_SHOT_SIZE    <width>x<height> of the window
//   INTERLINEA_SHOT_OPEN    the objectName of the object to act on, one for
//                           each call separated by "|" (the last for the rest)
//   INTERLINEA_SHOT_CALL    its methods to call, separated by "|" (start
//                           if none is given; open when a call fails)
//   INTERLINEA_SHOT_ARGS    their arguments: groups separated by "|", the
//                           arguments of one call by ","
//
// The calls follow 2.5 seconds after the start, 1.5 seconds apart.
void installShotHook(QQuickWindow *window, Backend *backend);
