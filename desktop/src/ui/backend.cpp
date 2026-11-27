#include "backend.h"

#include "apiclient.h"
#include "connectionmanager.h"
#include "documentpreview.h"
#include "eventfeed.h"
#include "interfacelanguage.h"
#include "languages.h"
#include "options.h"
#include "roles.h"
#include "secretstore.h"
#include "segmentcontent.h"
#include "words.h"

#include <QCoreApplication>
#include <QJSEngine>
#include <QJSValue>
#include <QClipboard>
#include <QDateTime>
#include <QDesktopServices>
#include <QGuiApplication>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QKeySequence>
#include <QRegularExpression>
#include <QSaveFile>
#include <QTemporaryFile>
#include <QSettings>
#include <QUrl>
#include <QUrlQuery>

namespace {

QVariantMap resultMap(const ApiResult &r)
{
    QVariantMap error;
    if (r.error.isError()) {
        error.insert(QStringLiteral("code"), r.error.code);
        error.insert(QStringLiteral("message"), r.error.message);
        QVariantList problems;
        for (const Problem &p : r.error.problems)
            problems.append(QVariantMap{{QStringLiteral("field"), p.field}, {QStringLiteral("message"), p.message}});
        error.insert(QStringLiteral("problems"), problems);
    }
    QVariant data;
    if (r.json.isObject())
        data = r.json.object().toVariantMap();
    else if (r.json.isArray())
        data = r.json.array().toVariantList();
    else if (!r.body.isEmpty())
        data = QString::fromUtf8(r.body);
    return {{QStringLiteral("ok"), !r.error.isError()},
            {QStringLiteral("status"), r.status},
            {QStringLiteral("data"), data},
            {QStringLiteral("error"), error},
            {QStringLiteral("version"), QString::fromLatin1(r.headers.value("etag")).remove(QLatin1Char('"'))},
            {QStringLiteral("fileName"), r.fileName()},
            {QStringLiteral("headers"), [&r] {
                 QVariantMap h;
                 for (auto it = r.headers.cbegin(); it != r.headers.cend(); ++it)
                     h.insert(QString::fromLatin1(it.key()), QString::fromUtf8(it.value()));
                 return h;
             }()}};
}

// What QML hands over as a JS value (an object built up in a function
// can come as one) is made plain data, all the way down, so it turns into
// JSON.
QVariant plain(const QVariant &v)
{
    if (v.metaType() == QMetaType::fromType<QJSValue>())
        return plain(v.value<QJSValue>().toVariant());
    if (v.typeId() == QMetaType::QVariantMap) {
        QVariantMap m = v.toMap();
        for (auto it = m.begin(); it != m.end(); ++it)
            it.value() = plain(it.value());
        return m;
    }
    if (v.typeId() == QMetaType::QVariantList) {
        QVariantList l = v.toList();
        for (QVariant &x : l)
            x = plain(x);
        return l;
    }
    return v;
}

QUrlQuery toQuery(const QVariantMap &m)
{
    QUrlQuery q;
    // Values are escaped here: QUrlQuery would take a "&" or "+" in one
    // for the start of another item.
    for (auto it = m.cbegin(); it != m.cend(); ++it) {
        const QVariant v = plain(it.value());
        const QString text = v.typeId() == QMetaType::QVariantMap || v.typeId() == QMetaType::QVariantList
                                 ? QString::fromUtf8(QJsonDocument::fromVariant(v).toJson(QJsonDocument::Compact))
                                 : v.toString();
        q.addQueryItem(it.key(), QString::fromUtf8(QUrl::toPercentEncoding(text)));
    }
    return q;
}

} // namespace

ApiCall::ApiCall(ApiReply *reply, QObject *parent, const QString &file, bool keepAside)
    : QObject(parent), m_reply(reply), m_file(file), m_keepAside(keepAside)
{
    connect(reply, &ApiReply::finished, this, [this](const ApiResult &r) {
        QVariantMap result = resultMap(r);
        if (!r.error.isError() && (!m_file.isEmpty() || m_keepAside)) {
            QString target = m_file;
            if (m_keepAside) {
                QTemporaryFile temp(QDir::tempPath() + QStringLiteral("/interlinea-XXXXXX"));
                temp.setAutoRemove(false);
                if (temp.open())
                    target = temp.fileName();
            }
            QSaveFile out(target);
            const bool saved = !target.isEmpty() && out.open(QIODevice::WriteOnly) && out.write(r.body) == r.body.size() && out.commit();
            result.insert(QStringLiteral("saved"), saved);
            result.insert(QStringLiteral("saveError"), saved ? QString() : out.errorString());
            if (m_keepAside)
                result.insert(QStringLiteral("temp"), saved ? target : QString());
        }
        emit done(result);
        deleteLater();
    });
    connect(reply, &ApiReply::progress, this, [this](qint64 d, qint64 t) { emit progress(double(d), double(t)); });
}

void ApiCall::abort()
{
    if (m_reply)
        m_reply->abort();
}

namespace {
Backend *g_backend = nullptr;
}

Backend *Backend::create(QQmlEngine *, QJSEngine *)
{
    // QML must not delete the app's backend.
    QJSEngine::setObjectOwnership(g_backend, QJSEngine::CppOwnership);
    return g_backend;
}

void Backend::setInstance(Backend *b)
{
    g_backend = b;
}

Backend::Backend(ConnectionManager *manager, QObject *parent)
    : QObject(parent), m_manager(manager)
{
    for (Connection *c : manager->all())
        watch(c);
    connect(manager, &ConnectionManager::added, this, [this](Connection *c) {
        watch(c);
        emit connectionsChanged();
    });
    connect(manager, &ConnectionManager::removed, this, &Backend::connectionsChanged);
    // The servers kept from the last time come in together, once read.
    connect(manager, &ConnectionManager::loaded, this, [this] {
        for (Connection *c : m_manager->all())
            watch(c);
        emit connectionsChanged();
    });
}

void Backend::watch(Connection *c)
{
    if (m_watched.contains(c))
        return;
    m_watched.insert(c);
    connect(c, &QObject::destroyed, this, [this, c] { m_watched.remove(c); });
    const QString id = c->id();
    connect(c->feed(), &EventFeed::event, this, [this, id](const QString &type, const QJsonObject &data) { emit event(id, type, data.toVariantMap()); });
    connect(c->feed(), &EventFeed::reset, this, [this, id] { emit reset(id); });
}

QList<QObject *> Backend::connections() const
{
    QList<QObject *> out;
    for (Connection *c : m_manager->all())
        out << c;
    return out;
}

QObject *Backend::local() const
{
    return m_manager->local();
}

QString Backend::version() const
{
    return QCoreApplication::applicationVersion();
}

QString Backend::language() const
{
    return InterfaceLanguage::current();
}

QObject *Backend::connection(const QString &id) const
{
    return m_manager->find(id);
}

ApiCall *Backend::request(const QString &connectionId, const QString &method, const QString &path, const QVariant &body, const QVariantMap &query,
                          int version)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        c = m_manager->local();
    ApiClient *api = c->api();
    const QJsonValue json = QJsonValue::fromVariant(plain(body));
    const std::optional<int> v = version >= 0 ? std::optional<int>(version) : std::nullopt;
    const QString m = method.toUpper();
    ApiReply *reply = nullptr;
    if (m == QLatin1String("GET"))
        reply = api->get(path, toQuery(query));
    else if (m == QLatin1String("POST"))
        reply = api->post(path, json, toQuery(query));
    else if (m == QLatin1String("PUT"))
        reply = api->put(path, json, v, toQuery(query));
    else if (m == QLatin1String("PATCH"))
        reply = api->patch(path, json, v);
    else
        reply = api->del(path, toQuery(query), v);
    return new ApiCall(reply, this);
}

ApiCall *Backend::upload(const QString &connectionId, const QString &method, const QString &path, const QString &file, const QString &contentType,
                         const QVariantMap &query)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        c = m_manager->local();
    QFile f(localPath(file));
    QByteArray data;
    if (f.open(QIODevice::ReadOnly))
        data = f.readAll();
    return new ApiCall(c->api()->upload(method.toUpper().toLatin1(), path, data, contentType, toQuery(query)), this);
}

ApiCall *Backend::download(const QString &connectionId, const QString &path, const QString &file, const QVariantMap &query)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        c = m_manager->local();
    return new ApiCall(c->api()->get(path, toQuery(query)), this, localPath(file));
}

ApiCall *Backend::fetch(const QString &connectionId, const QString &path, const QVariantMap &query)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        c = m_manager->local();
    return new ApiCall(c->api()->get(path, toQuery(query)), this, QString(), true);
}

QString Backend::writeText(const QVariant &to, const QString &text) const
{
    QSaveFile f(localPath(to));
    if (!f.open(QIODevice::WriteOnly) || f.write(text.toUtf8()) < 0 || !f.commit())
        return f.errorString();
    return QString();
}

QString Backend::keep(const QString &temp, const QVariant &to) const
{
    const QString target = localPath(to);
    QFile from(temp);
    if (!from.exists())
        return tr("The file is no longer there.");
    if (QFile::exists(target) && !QFile::remove(target))
        return tr("%1 can't be replaced.").arg(QDir::toNativeSeparators(target));
    if (from.rename(target))
        return QString();
    // Across drives a copy does it.
    if (from.copy(target)) {
        from.remove();
        return QString();
    }
    return from.errorString();
}

ApiCall *Backend::postToFile(const QString &connectionId, const QString &path, const QVariant &body, const QString &file)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        c = m_manager->local();
    ApiReply *reply = c->api()->post(path, QJsonValue::fromVariant(plain(body)));
    return new ApiCall(reply, this, localPath(file));
}

QString Backend::seg(const QString &s) const
{
    return ApiClient::seg(s);
}

QString Backend::fileSafe(const QString &name) const
{
    QString file = name;
    file.replace(QRegularExpression(QStringLiteral("[^\\p{L}\\p{N}]+")), QStringLiteral("_"));
    return file;
}

QString Backend::localPath(const QVariant &url) const
{
    const QUrl u = url.toUrl();
    if (u.isLocalFile())
        return u.toLocalFile();
    return url.toString();
}

QStringList Backend::expand(const QVariantList &urls) const
{
    QStringList out;
    for (const QVariant &v : urls) {
        const QString p = localPath(v);
        if (!QFileInfo(p).isDir()) {
            out << p;
            continue;
        }
        QDirIterator it(p, QDir::Files | QDir::NoDotAndDotDot, QDirIterator::Subdirectories);
        while (it.hasNext())
            out << it.next();
    }
    return out;
}

QString Backend::fileName(const QString &path) const
{
    return QFileInfo(localPath(path)).fileName();
}

QVariantList Backend::filesOf(const QVariantList &urls) const
{
    QVariantList out;
    const auto add = [&out](const QString &path, const QString &name) {
        QFile f(path);
        QString problem;
        if (!f.open(QIODevice::ReadOnly))
            problem = tr("The file can't be read: %1").arg(f.errorString());
        out << QVariantMap{{QStringLiteral("path"), path}, {QStringLiteral("name"), name}, {QStringLiteral("size"), f.size()}, {QStringLiteral("problem"), problem}};
    };
    for (const QVariant &v : urls) {
        const QString p = localPath(v);
        const QFileInfo info(p);
        if (info.isDir()) {
            const QDir base(info.absolutePath());
            QStringList inside;
            QDirIterator it(p, QDir::Files | QDir::NoDotAndDotDot, QDirIterator::Subdirectories);
            while (it.hasNext())
                inside << it.next();
            inside.sort();
            for (const QString &f : std::as_const(inside))
                add(f, base.relativeFilePath(f));
        } else if (info.exists()) {
            add(p, info.fileName());
        }
    }
    return out;
}

QString Backend::segmentHtml(const QVariantList &content, const QVariantList &marks) const
{
    QVector<SegmentContent::Mark> m;
    for (const QVariant &v : marks) {
        const QVariantMap x = v.toMap();
        m << SegmentContent::Mark{x.value(QStringLiteral("start")).toInt(), x.value(QStringLiteral("end")).toInt(), x.value(QStringLiteral("cssClass")).toString()};
    }
    return QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), SegmentContent::toHtml(QJsonArray::fromVariantList(plain(content).toList()), m));
}

QString Backend::diffHtml(const QString &a, const QString &b) const
{
    return QStringLiteral("<style>%1</style>%2").arg(SegmentContent::css(), SegmentContent::diffHtml(a, b));
}

QString Backend::segmentCss() const
{
    return SegmentContent::css();
}

bool Backend::sameContent(const QVariantList &a, const QVariantList &b) const
{
    return SegmentContent::equal(QJsonArray::fromVariantList(plain(a).toList()), QJsonArray::fromVariantList(plain(b).toList()));
}

QString Backend::contentKey(const QVariantList &content) const
{
    return QString::fromUtf8(QJsonDocument(SegmentContent::normalized(QJsonArray::fromVariantList(plain(content).toList()))).toJson(QJsonDocument::Compact));
}

int Backend::wordCount(const QVariantList &content) const
{
    static const QRegularExpression word(QStringLiteral("[\\p{L}\\p{N}]+(?:['’.,-][\\p{L}\\p{N}]+)*"));
    int n = 0;
    for (auto it = word.globalMatch(SegmentContent::plainText(QJsonArray::fromVariantList(plain(content).toList()))); it.hasNext(); it.next())
        ++n;
    return n;
}

QString Backend::foldForSearch(const QString &text) const
{
    return text.normalized(QString::NormalizationForm_KC).toCaseFolded();
}

QString Backend::base64File(const QString &path) const
{
    QFile f(localPath(path));
    return f.open(QIODevice::ReadOnly) ? QString::fromLatin1(f.readAll().toBase64()) : QString();
}

QVariantMap Backend::readBase64(const QVariant &path) const
{
    QFile f(localPath(path));
    if (!f.open(QIODevice::ReadOnly))
        return {{QStringLiteral("error"), f.errorString()}};
    return {{QStringLiteral("data"), QString::fromLatin1(f.readAll().toBase64())}};
}

QString Backend::baseUrl(const QString &connectionId) const
{
    Connection *c = m_manager->find(connectionId);
    QString base = c ? c->url().toString() : QString();
    while (base.endsWith(QLatin1Char('/')))
        base.chop(1);
    return base;
}

QVariant Backend::setting(const QString &key, const QVariant &fallback) const
{
    return QSettings().value(key, fallback);
}

void Backend::setSetting(const QString &key, const QVariant &value)
{
    QSettings().setValue(key, value);
}

QStringList Backend::interfaceLanguages() const
{
    return InterfaceLanguage::codes();
}

QString Backend::interfaceLanguageName(const QString &code) const
{
    return InterfaceLanguage::nativeName(code);
}

QString Backend::chosenInterfaceLanguage() const
{
    return InterfaceLanguage::chosen();
}

void Backend::chooseInterfaceLanguage(const QString &code)
{
    InterfaceLanguage::choose(code);
}

QString Backend::systemLanguage() const
{
    return InterfaceLanguage::systemLanguage();
}

QString Backend::languageName(const QString &code) const
{
    return Languages::name(code);
}

QVariantList Backend::commonLanguages() const
{
    QVariantList out;
    for (const QString &code : Languages::common())
        out.append(QVariantMap{{QStringLiteral("code"), code}, {QStringLiteral("label"), Languages::label(code)}});
    return out;
}

QString Backend::normalizeLanguage(const QString &typed) const
{
    return Languages::normalize(typed);
}

bool Backend::rtl(const QString &tag) const
{
    return Languages::rtl(tag);
}

QString Backend::date(const QString &iso) const
{
    // As the web app writes a date (day, short month, year) in each
    // interface language.
    const QDateTime t = QDateTime::fromString(iso, Qt::ISODate);
    if (!t.isValid())
        return {};
    const QDate d = t.toLocalTime().date();
    const QString lang = InterfaceLanguage::current();
    if (lang == QLatin1String("ko"))
        return QStringLiteral("%1년 %2월 %3일").arg(d.year()).arg(d.month()).arg(d.day());
    const QLocale l = InterfaceLanguage::locale();
    return l.toString(d, lang == QLatin1String("de") ? QStringLiteral("d. MMM yyyy") : QStringLiteral("d MMM yyyy"));
}

QString Backend::time(const QString &iso) const
{
    // Day, short month, hours and minutes.
    const QDateTime t = QDateTime::fromString(iso, Qt::ISODate);
    if (!t.isValid())
        return {};
    const QDateTime l = t.toLocalTime();
    const QString lang = InterfaceLanguage::current();
    if (lang == QLatin1String("ko"))
        return QStringLiteral("%1월 %2일 %3").arg(l.date().month()).arg(l.date().day()).arg(l.time().toString(QStringLiteral("HH:mm")));
    return InterfaceLanguage::locale().toString(l, lang == QLatin1String("de") ? QStringLiteral("d. MMM, HH:mm") : QStringLiteral("d MMM, HH:mm"));
}

QString Backend::listOf(const QStringList &items) const
{
    // "A, B and C" in the interface language.
    if (items.size() < 2)
        return items.join(QString());
    const QString lang = InterfaceLanguage::current();
    const QString last = lang == QLatin1String("de") ? QStringLiteral(" und ") : lang == QLatin1String("ko") ? QStringLiteral(" 및 ") : QStringLiteral(" and ");
    return items.mid(0, items.size() - 1).join(QStringLiteral(", ")) + last + items.last();
}

QString Backend::number(double n, int decimals) const
{
    return InterfaceLanguage::locale().toString(n, 'f', decimals);
}

bool Backend::gitWords() const
{
    return Words::git();
}

void Backend::setGitWords(bool git)
{
    if (git == Words::git())
        return;
    Words::setGit(git);
    emit gitWordsChanged();
}

QString Backend::word(const QString &key) const
{
    return Words::get(key.toLatin1().constData());
}

bool Backend::roleAllows(const QString &role, const QString &need) const
{
    return Roles::allows(role, need);
}

QString Backend::roleText(const QString &name) const
{
    return Roles::text(name);
}

QStringList Backend::permissions() const
{
    return Roles::permissions();
}

QVariant Backend::option(const QString &name) const
{
    if (name == QLatin1String("autoInsert"))
        return Options::autoInsert();
    if (name == QLatin1String("autoComplete"))
        return Options::autoComplete();
    if (name == QLatin1String("autoPropagate"))
        return Options::autoPropagate();
    if (name == QLatin1String("contextRule"))
        return Options::contextRule();
    return {};
}

void Backend::setOption(const QString &name, const QVariant &value)
{
    if (name == QLatin1String("autoInsert"))
        Options::setAutoInsert(value.toBool());
    else if (name == QLatin1String("autoComplete"))
        Options::setAutoComplete(value.toBool());
    else if (name == QLatin1String("autoPropagate"))
        Options::setAutoPropagate(value.toBool());
    else if (name == QLatin1String("contextRule"))
        Options::setContextRule(value.toString());
}

QString Backend::keychainNote() const
{
    return SecretStore::secure() ? tr("Sign-ins to team servers are kept in %1.").arg(SecretStore::where())
                                 : tr("Sign-ins to team servers are kept in %1, because the keychain can't be used: %2").arg(SecretStore::where(), SecretStore::fallbackReason());
}

QString Backend::dataDir() const
{
    return QDir::toNativeSeparators(m_manager->core()->dataDir());
}

void Backend::openFolder(const QString &path) const
{
    QDesktopServices::openUrl(QUrl::fromLocalFile(path));
}

QString Backend::licence() const
{
    QString all;
    QFile notice(QStringLiteral(":/legal/NOTICE"));
    QFile licence(QStringLiteral(":/legal/LICENSE"));
    if (notice.open(QIODevice::ReadOnly))
        all += QString::fromUtf8(notice.readAll()) + QStringLiteral("\n\n");
    if (licence.open(QIODevice::ReadOnly))
        all += QString::fromUtf8(licence.readAll());
    return all;
}

void Backend::copy(const QString &text) const
{
    QGuiApplication::clipboard()->setText(text);
}

void Backend::signOut(const QString &connectionId)
{
    if (Connection *c = m_manager->find(connectionId))
        m_manager->signOut(c);
}

void Backend::removeServer(const QString &connectionId)
{
    if (Connection *c = m_manager->find(connectionId); c && !c->isLocal())
        m_manager->remove(c);
}

void Backend::setUser(const QString &connectionId, const QVariantMap &user)
{
    if (Connection *c = m_manager->find(connectionId)) {
        c->setUser(QJsonObject::fromVariantMap(user));
        if (c->isLocal())
            QSettings().setValue(QStringLiteral("you/name"), user.value(QStringLiteral("name")).toString());
    }
}

QString Backend::serverUrl(const QString &typed) const
{
    return ConnectionManager::serverUrl(typed).toString();
}

namespace {
QVariantMap errorMap(const ApiError &e)
{
    if (!e.isError())
        return {};
    QVariantList problems;
    for (const Problem &p : e.problems)
        problems.append(QVariantMap{{QStringLiteral("field"), p.field}, {QStringLiteral("message"), p.message}});
    return {{QStringLiteral("code"), e.code}, {QStringLiteral("message"), e.message}, {QStringLiteral("problems"), problems}};
}
} // namespace

void Backend::probe(const QString &url)
{
    m_manager->probe(QUrl(url), [this, url](const ServerInfo &info, const ApiError &e) {
        if (e.isError()) {
            emit probed(url, {{QStringLiteral("error"), errorMap(e)}});
            return;
        }
        emit probed(url, {{QStringLiteral("url"), info.url.toString()},
                          {QStringLiteral("version"), info.version},
                          {QStringLiteral("provider"), info.provider},
                          {QStringLiteral("setup"), info.setup}});
    });
}

void Backend::signInWithPassword(const QString &url, const QString &username, const QString &password)
{
    m_manager->signInWithPassword(QUrl(url), username, password,
                                  [this, url](Connection *c, const ApiError &e) { emit signedIn(url, c ? c->id() : QString(), errorMap(e)); });
}

void Backend::signInWithProvider(const QString &url)
{
    m_manager->signInWithProvider(QUrl(url), [this, url](Connection *c, const ApiError &e) { emit signedIn(url, c ? c->id() : QString(), errorMap(e)); });
}

void Backend::cancelProviderSignIn()
{
    m_manager->cancelProviderSignIn();
}

void Backend::setUpServer(const QString &url, const QString &token, const QString &username, const QString &name, const QString &email, const QString &password)
{
    m_manager->setUpServer(QUrl(url), token, username, name, email, password,
                           [this, url](Connection *c, const ApiError &e) { emit signedIn(url, c ? c->id() : QString(), errorMap(e)); });
}

void Backend::restartCore()
{
    m_manager->core()->start();
}

QString Backend::previewHtml(const QVariantList &segments, const QVariantList &layout, const QString &activeId) const
{
    QList<QJsonObject> all;
    for (const QVariant &s : plain(segments).toList())
        all.append(QJsonObject::fromVariantMap(s.toMap()));
    return ::previewHtml(all, QJsonArray::fromVariantList(plain(layout).toList()), activeId);
}

QStringList Backend::matchAll(const QString &pattern, const QString &text) const
{
    QStringList out;
    const QRegularExpression re(pattern, QRegularExpression::UseUnicodePropertiesOption);
    for (auto it = re.globalMatch(text); it.hasNext();)
        out << it.next().captured(0);
    return out;
}

QVariantMap Backend::replaceInContent(const QVariantList &content, const QString &find, const QString &with, bool matchCase, bool wholeWords) const
{
    QString body = QRegularExpression::escape(find);
    // A whole word has no letter or digit before or after it.
    if (wholeWords)
        body = QStringLiteral("(?<![\\p{L}\\p{N}])%1(?![\\p{L}\\p{N}])").arg(body);
    QRegularExpression::PatternOptions o = QRegularExpression::UseUnicodePropertiesOption;
    if (!matchCase)
        o |= QRegularExpression::CaseInsensitiveOption;
    const QRegularExpression re(body, o);
    QVariantList out;
    int count = 0;
    for (const QVariant &v : plain(content).toList()) {
        const QVariantMap run = v.toMap();
        if (!run.contains(QStringLiteral("text"))) {
            out.append(run);
            continue;
        }
        const QString text = run.value(QStringLiteral("text")).toString();
        QString next;
        int at = 0;
        if (!find.isEmpty()) {
            for (auto it = re.globalMatch(text); it.hasNext();) {
                const QRegularExpressionMatch m = it.next();
                if (m.capturedLength() == 0)
                    continue;
                next += text.mid(at, m.capturedStart() - at);
                next += with;
                at = int(m.capturedEnd());
                ++count;
            }
        }
        next += text.mid(at);
        if (!next.isEmpty())
            out.append(QVariantMap{{QStringLiteral("text"), next}});
    }
    return {{QStringLiteral("content"), out}, {QStringLiteral("count"), count}};
}

QString Backend::keyText(int standardKey) const
{
    return QKeySequence(QKeySequence::StandardKey(standardKey)).toString(QKeySequence::NativeText);
}
