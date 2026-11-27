#include "testcore.h"

#include "iconprovider.h"

#include <QCoreApplication>
#include <QDir>
#include <QKeyEvent>
#include <QQuickItem>
#include <QQuickWindow>
#include <QSignalSpy>
#include <QTest>
#include <QtQml/qqmlextensionplugin.h>

Q_IMPORT_QML_PLUGIN(InterlineaPlugin)

QString testCoreProgram()
{
    const QString forced = qEnvironmentVariable("INTERLINEA_CORE");
    if (!forced.isEmpty())
        return forced;
    return CoreLauncher::defaultProgram();
}

TestCore::TestCore()
    : launcher(testCoreProgram(), dir.path() + QStringLiteral("/data"), QStringLiteral("Tess Tester"))
{
}

bool TestCore::start(QString *why)
{
    QSignalSpy started(&launcher, &CoreLauncher::started);
    QSignalSpy failed(&launcher, &CoreLauncher::failed);
    launcher.start();
    for (int i = 0; i < 300 && started.isEmpty() && failed.isEmpty(); ++i)
        QTest::qWait(100);
    if (started.isEmpty()) {
        *why = failed.isEmpty() ? QStringLiteral("no answer") : failed.first().first().toString();
        return false;
    }
    return true;
}

ApiResult waitFor(ApiReply *reply)
{
    QSignalSpy spy(reply, &ApiReply::finished);
    if (!spy.wait(20000))
        return {};
    return spy.first().first().value<ApiResult>();
}

// Text that runs out of its item or out of the part that clips it: the
// rest is not seen, or lies over what is beside it.
static void checkItem(QQuickItem *item, const QRectF &clip, QStringList *found)
{
    if (!item->isVisible() || item->opacity() <= 0.0)
        return;
    QRectF within = clip;
    const QRectF box = item->mapRectToScene(QRectF(0, 0, item->width(), item->height()));
    if (item->inherits("QQuickText") || item->inherits("QQuickTextEdit") || item->inherits("QQuickTextInput")) {
        const QString text = item->property("text").toString();
        const bool wraps = item->property("wrapMode").toInt() != 0;
        const bool elides = item->property("elide").toInt() != 0;
        const qreal content = item->property("contentWidth").toReal();
        const bool plain = !text.trimmed().isEmpty() && !item->inherits("QQuickTextInput") && !text.contains(QLatin1Char('<'));
        if (plain && !wraps && !elides && content > item->width() + 1.5 && item->width() > 0)
            found->append(QStringLiteral("text wider than its place (%1 for %2): \"%3\"").arg(content).arg(item->width()).arg(text.left(80)));
        else if (plain && box.width() > 0 && (box.right() > clip.right() + 1.5 || box.left() < clip.left() - 1.5) && clip.width() > 0)
            found->append(QStringLiteral("text out of its part: \"%1\"").arg(text.left(80)));
        else if (QQuickItem *p = item->parentItem(); plain && p && p->inherits("QQuickLayout") && p->width() > 0) {
            const QRectF in = p->mapRectToScene(QRectF(0, 0, p->width(), p->height()));
            if (box.right() > in.right() + 1.5)
                found->append(QStringLiteral("text out of its row or column: \"%1\"").arg(text.left(80)));
        }
        return;
    }
    // Parts side by side in a row that lie over each other.
    if (item->inherits("QQuickRowLayout") || item->inherits("QQuickRow")) {
        QList<QRectF> boxes;
        for (QQuickItem *c : item->childItems())
            if (c->isVisible() && c->width() > 0 && c->height() > 0 && !c->inherits("QQuickRepeater"))
                boxes << c->mapRectToScene(QRectF(0, 0, c->width(), c->height()));
        for (int a = 0; a < boxes.size(); ++a)
            for (int b = a + 1; b < boxes.size(); ++b)
                if (boxes[a].intersected(boxes[b]).width() > 1.5 && boxes[a].intersected(boxes[b]).height() > 1.5)
                    found->append(QStringLiteral("parts over each other in a row of %1").arg(item->parentItem() ? QString::fromLatin1(item->parentItem()->metaObject()->className()) : QString()));
    }
    // Lists and scrolled parts clip their content; what is scrolled away is fine.
    if (item->clip())
        within = clip.intersected(box);
    const bool scrolls = item->inherits("QQuickFlickable");
    for (QQuickItem *c : item->childItems()) {
        if (scrolls)
            checkItem(c, within, found);
        else
            checkItem(c, within, found);
    }
}

QString layoutProblems(QQuickWindow *window)
{
    QTest::qWait(300);
    QStringList found;
    checkItem(window->contentItem(), QRectF(0, 0, window->width(), window->height()), &found);
    found.removeDuplicates();
    return found.join(QLatin1Char('\n'));
}

QmlApp::QmlApp(CoreLauncher *core)
    : connections(core)
    , backend(&connections)
{
    Backend::setInstance(&backend);
}

QmlApp::~QmlApp()
{
    Backend::setInstance(nullptr);
}

bool QmlApp::start(QString *why)
{
    engine.addImageProvider(QStringLiteral("icon"), new IconProvider);
    engine.loadFromModule("Interlinea", "Main");
    if (engine.rootObjects().isEmpty()) {
        *why = QStringLiteral("the window did not load");
        return false;
    }
    window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
    window->resize(1366, 860);
    connections.start();
    for (int i = 0; i < 300 && !(connections.local() && connections.local()->isReady()); ++i)
        QTest::qWait(100);
    if (!connections.local() || !connections.local()->isReady()) {
        *why = QStringLiteral("the core on this computer is not ready");
        return false;
    }
    QTest::qWait(200);
    return true;
}

static QObject *findItem(QQuickItem *in, const QString &name)
{
    for (QQuickItem *c : in->childItems()) {
        if (c->objectName() == name)
            return c;
        if (QObject *o = findItem(c, name))
            return o;
    }
    return nullptr;
}

QObject *QmlApp::find(const QString &name) const
{
    if (!window)
        return nullptr;
    if (QObject *o = window->findChild<QObject *>(name))
        return o;
    return findItem(window->contentItem(), name);
}

QObject *QmlApp::find(QObject *within, const QString &name) const
{
    if (!within)
        return nullptr;
    if (QObject *o = within->findChild<QObject *>(name))
        return o;
    if (auto *item = qobject_cast<QQuickItem *>(within))
        return findItem(item, name);
    return nullptr;
}

QVariant QmlApp::call(QObject *target, const char *method, const QVariantList &a) const
{
    if (!target)
        return {};
    QVariant out;
    QVariantList args = a;
    const auto arg = [&args](int i) { return Q_ARG(QVariant, args[i]); };
    bool ok = false;
    switch (args.size()) {
    case 0:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out));
        break;
    case 1:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0));
        break;
    case 2:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0), arg(1));
        break;
    case 3:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0), arg(1), arg(2));
        break;
    case 4:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0), arg(1), arg(2), arg(3));
        break;
    case 5:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0), arg(1), arg(2), arg(3), arg(4));
        break;
    default:
        ok = QMetaObject::invokeMethod(target, method, Q_RETURN_ARG(QVariant, out), arg(0), arg(1), arg(2), arg(3), arg(4), arg(5));
        break;
    }
    // A method of C++ that gives nothing back.
    if (!ok) {
        switch (args.size()) {
        case 0:
            ok = QMetaObject::invokeMethod(target, method);
            break;
        case 1:
            ok = QMetaObject::invokeMethod(target, method, arg(0));
            break;
        case 2:
            ok = QMetaObject::invokeMethod(target, method, arg(0), arg(1));
            break;
        default:
            break;
        }
    }
    if (!ok)
        qWarning("QmlApp::call: %s has no %s with %lld arguments", qPrintable(target->objectName()), method, static_cast<long long>(args.size()));
    // A value of JavaScript comes back as a QJSValue: as plain data.
    if (out.metaType() == QMetaType::fromType<QJSValue>())
        out = out.value<QJSValue>().toVariant();
    return out;
}

QVariant QmlApp::call(const QString &name, const char *method, const QVariantList &args) const
{
    return call(find(name), method, args);
}

QVariant QmlApp::get(const QString &name, const char *property) const
{
    QObject *o = find(name);
    QVariant v = o ? o->property(property) : QVariant();
    if (v.metaType() == QMetaType::fromType<QJSValue>())
        v = v.value<QJSValue>().toVariant();
    return v;
}

void QmlApp::show(const QString &page) const
{
    window->setProperty("page", page);
    QTest::qWait(50);
}

void QmlApp::shot(const QString &name) const
{
    const QString dir = qEnvironmentVariable("INTERLINEA_SCREENSHOTS");
    if (dir.isEmpty() || !window)
        return;
    QTest::qWait(150);
    window->grabWindow().save(QStringLiteral("%1/%2.png").arg(dir, name));
}

bool QmlApp::click(QObject *button) const
{
    if (!button || !button->property("enabled").toBool() || !button->property("visible").toBool())
        return false;
    // As a click does: a checkable button changes, then says it was clicked.
    if (button->property("checkable").toBool())
        button->setProperty("checked", !button->property("checked").toBool());
    // A signal invoked so reports failure even when it was sent.
    QMetaObject::invokeMethod(button, "clicked");
    return true;
}

bool QmlApp::clickable(const QString &name) const
{
    QObject *b = find(name);
    return b && b->property("enabled").toBool() && b->property("visible").toBool();
}

bool QmlApp::click(const QString &name) const
{
    return click(find(name));
}

bool QmlApp::type(QObject *field, const QString &text, bool clear) const
{
    auto *item = qobject_cast<QQuickItem *>(field);
    if (!item)
        return false;
    item->forceActiveFocus();
    QTest::qWait(20);
    if (clear)
        QMetaObject::invokeMethod(item, "selectAll");
    if (clear && text.isEmpty()) {
        QTest::keyClick(window, Qt::Key_Delete);
        return true;
    }
    // Each character as a key pressed and let go, with its text.
    for (const QChar ch : text) {
        QKeyEvent press(QEvent::KeyPress, Qt::Key_unknown, Qt::NoModifier, QString(ch));
        QKeyEvent release(QEvent::KeyRelease, Qt::Key_unknown, Qt::NoModifier, QString(ch));
        QCoreApplication::sendEvent(window, &press);
        QCoreApplication::sendEvent(window, &release);
    }
    QTest::qWait(20);
    return true;
}

bool QmlApp::type(const QString &name, const QString &text, bool clear) const
{
    return type(find(name), text, clear);
}

QString QmlApp::notice() const
{
    return get(QStringLiteral("toast"), "last").toString();
}

QStringList QmlApp::notices() const
{
    return get(QStringLiteral("toast"), "said").toStringList();
}
