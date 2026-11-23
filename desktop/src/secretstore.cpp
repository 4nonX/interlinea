// libsecret's headers use "signals" as a name, which Qt defines as a macro.
#ifdef INTERLINEA_LIBSECRET
#pragma push_macro("signals")
#undef signals
#include <libsecret/secret.h>
#pragma pop_macro("signals")
#endif

#include "secretstore.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>
#include <QStandardPaths>

#ifdef Q_OS_WIN
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#include <wincred.h>
#endif
#ifdef Q_OS_MACOS
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#endif

namespace {

const char *const service = "org.interlinea.desktop";

bool g_secure = true;
QString g_reason;

QString tr(const char *s)
{
    return QCoreApplication::translate("SecretStore", s);
}

// The file store: a JSON object, readable by its owner only.
QString filePath()
{
    const QString forced = qEnvironmentVariable("INTERLINEA_SECRETS_FILE");
    if (!forced.isEmpty())
        return forced;
    return QStandardPaths::writableLocation(QStandardPaths::AppConfigLocation) + QStringLiteral("/sign-ins.json");
}

QJsonObject readFile()
{
    QFile f(filePath());
    if (!f.open(QIODevice::ReadOnly))
        return {};
    return QJsonDocument::fromJson(f.readAll()).object();
}

bool writeFile(const QJsonObject &o, QString *error)
{
    const QString path = filePath();
    QDir().mkpath(QFileInfo(path).absolutePath());
    QSaveFile f(path);
    if (!f.open(QIODevice::WriteOnly)) {
        if (error)
            *error = f.errorString();
        return false;
    }
    f.setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner);
    f.write(QJsonDocument(o).toJson(QJsonDocument::Compact));
    if (!f.commit()) {
        if (error)
            *error = f.errorString();
        return false;
    }
    QFile::setPermissions(path, QFileDevice::ReadOwner | QFileDevice::WriteOwner);
    return true;
}

bool fileForced()
{
    return !qEnvironmentVariable("INTERLINEA_SECRETS_FILE").isEmpty();
}

#ifdef INTERLINEA_LIBSECRET
const SecretSchema *schema()
{
    static const SecretSchema s = {
        service, SECRET_SCHEMA_NONE,
        {{"key", SECRET_SCHEMA_ATTRIBUTE_STRING}, {nullptr, SECRET_SCHEMA_ATTRIBUTE_STRING}},
        0, nullptr, nullptr, nullptr, nullptr, nullptr, nullptr, nullptr};
    return &s;
}
#endif

#ifdef Q_OS_WIN
std::wstring target(const QString &key)
{
    return (QStringLiteral("Interlinea/") + key).toStdWString();
}
#endif

#ifdef Q_OS_MACOS
CFMutableDictionaryRef query(const QString &key)
{
    CFMutableDictionaryRef q = CFDictionaryCreateMutable(nullptr, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFStringRef svc = CFStringCreateWithCString(nullptr, service, kCFStringEncodingUTF8);
    CFStringRef acct = key.toCFString();
    CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
    CFDictionarySetValue(q, kSecAttrService, svc);
    CFDictionarySetValue(q, kSecAttrAccount, acct);
    CFRelease(svc);
    CFRelease(acct);
    return q;
}

QString statusText(OSStatus st)
{
    CFStringRef s = SecCopyErrorMessageString(st, nullptr);
    if (!s)
        return QString::number(st);
    const QString out = QString::fromCFString(s);
    CFRelease(s);
    return out;
}
#endif

// keychain* return false when the keychain can't be used at all; then the
// file store is used and the reason kept.
bool keychainWrite(const QString &key, const QString &secret, QString *error)
{
#if defined(Q_OS_WIN)
    const std::wstring t = target(key);
    const QByteArray blob = secret.toUtf8();
    CREDENTIALW c = {};
    c.Type = CRED_TYPE_GENERIC;
    c.TargetName = const_cast<LPWSTR>(t.c_str());
    c.CredentialBlobSize = DWORD(blob.size());
    c.CredentialBlob = reinterpret_cast<LPBYTE>(const_cast<char *>(blob.constData()));
    c.Persist = CRED_PERSIST_LOCAL_MACHINE;
    std::wstring user = L"Interlinea";
    c.UserName = const_cast<LPWSTR>(user.c_str());
    if (!CredWriteW(&c, 0)) {
        if (error)
            *error = tr("The Credential Manager refused the sign-in (error %1).").arg(GetLastError());
        return true;
    }
    return true;
#elif defined(Q_OS_MACOS)
    CFMutableDictionaryRef q = query(key);
    SecItemDelete(q);
    const QByteArray b = secret.toUtf8();
    CFDataRef data = CFDataCreate(nullptr, reinterpret_cast<const UInt8 *>(b.constData()), b.size());
    CFDictionarySetValue(q, kSecValueData, data);
    const OSStatus st = SecItemAdd(q, nullptr);
    CFRelease(data);
    CFRelease(q);
    if (st != errSecSuccess && error)
        *error = tr("The Keychain refused the sign-in: %1").arg(statusText(st));
    return true;
#elif defined(INTERLINEA_LIBSECRET)
    Q_UNUSED(error)
    GError *err = nullptr;
    const QByteArray label = QStringLiteral("Interlinea sign-in (%1)").arg(key).toUtf8();
    secret_password_store_sync(schema(), SECRET_COLLECTION_DEFAULT, label.constData(), secret.toUtf8().constData(), nullptr, &err,
                               "key", key.toUtf8().constData(), nullptr);
    if (err) {
        g_reason = QString::fromUtf8(err->message);
        g_error_free(err);
        return false;
    }
    return true;
#else
    Q_UNUSED(key)
    Q_UNUSED(secret)
    Q_UNUSED(error)
    g_reason = tr("This build has no keychain support.");
    return false;
#endif
}

bool keychainRead(const QString &key, std::optional<QString> *out, QString *error)
{
#if defined(Q_OS_WIN)
    PCREDENTIALW c = nullptr;
    if (!CredReadW(target(key).c_str(), CRED_TYPE_GENERIC, 0, &c)) {
        const DWORD e = GetLastError();
        if (e != ERROR_NOT_FOUND && error)
            *error = tr("The Credential Manager could not be read (error %1).").arg(e);
        *out = std::nullopt;
        return true;
    }
    *out = QString::fromUtf8(reinterpret_cast<const char *>(c->CredentialBlob), int(c->CredentialBlobSize));
    CredFree(c);
    return true;
#elif defined(Q_OS_MACOS)
    CFMutableDictionaryRef q = query(key);
    CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
    CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
    CFTypeRef data = nullptr;
    const OSStatus st = SecItemCopyMatching(q, &data);
    CFRelease(q);
    if (st == errSecItemNotFound) {
        *out = std::nullopt;
        return true;
    }
    if (st != errSecSuccess) {
        if (error)
            *error = tr("The Keychain could not be read: %1").arg(statusText(st));
        *out = std::nullopt;
        return true;
    }
    CFDataRef d = static_cast<CFDataRef>(data);
    *out = QString::fromUtf8(reinterpret_cast<const char *>(CFDataGetBytePtr(d)), int(CFDataGetLength(d)));
    CFRelease(data);
    return true;
#elif defined(INTERLINEA_LIBSECRET)
    GError *err = nullptr;
    gchar *pw = secret_password_lookup_sync(schema(), nullptr, &err, "key", key.toUtf8().constData(), nullptr);
    if (err) {
        g_reason = QString::fromUtf8(err->message);
        g_error_free(err);
        return false;
    }
    *out = pw ? std::optional<QString>(QString::fromUtf8(pw)) : std::nullopt;
    if (pw)
        secret_password_free(pw);
    Q_UNUSED(error)
    return true;
#else
    Q_UNUSED(key)
    Q_UNUSED(out)
    Q_UNUSED(error)
    g_reason = tr("This build has no keychain support.");
    return false;
#endif
}

bool keychainRemove(const QString &key, QString *error)
{
#if defined(Q_OS_WIN)
    if (!CredDeleteW(target(key).c_str(), CRED_TYPE_GENERIC, 0)) {
        const DWORD e = GetLastError();
        if (e != ERROR_NOT_FOUND && error)
            *error = tr("The Credential Manager refused (error %1).").arg(e);
    }
    return true;
#elif defined(Q_OS_MACOS)
    CFMutableDictionaryRef q = query(key);
    const OSStatus st = SecItemDelete(q);
    CFRelease(q);
    if (st != errSecSuccess && st != errSecItemNotFound && error)
        *error = tr("The Keychain refused: %1").arg(statusText(st));
    return true;
#elif defined(INTERLINEA_LIBSECRET)
    GError *err = nullptr;
    secret_password_clear_sync(schema(), nullptr, &err, "key", key.toUtf8().constData(), nullptr);
    if (err) {
        g_reason = QString::fromUtf8(err->message);
        g_error_free(err);
        return false;
    }
    Q_UNUSED(error)
    return true;
#else
    Q_UNUSED(key)
    Q_UNUSED(error)
    g_reason = tr("This build has no keychain support.");
    return false;
#endif
}

} // namespace

bool SecretStore::write(const QString &key, const QString &secret, QString *error)
{
    QString e;
    if (!fileForced() && keychainWrite(key, secret, &e)) {
        g_secure = true;
        // A secret kept in the file before the keychain worked goes.
        QJsonObject o = readFile();
        if (o.contains(key)) {
            o.remove(key);
            writeFile(o, nullptr);
        }
        if (error)
            *error = e;
        return e.isEmpty();
    }
    g_secure = false;
    QJsonObject o = readFile();
    o.insert(key, secret);
    return writeFile(o, error);
}

std::optional<QString> SecretStore::read(const QString &key, QString *error)
{
    std::optional<QString> out;
    QString e;
    if (!fileForced() && keychainRead(key, &out, &e)) {
        g_secure = true;
        if (error)
            *error = e;
        if (out)
            return out;
        // Kept in the file while the keychain was away.
        const QJsonObject o = readFile();
        if (o.contains(key))
            return o.value(key).toString();
        return std::nullopt;
    }
    g_secure = false;
    const QJsonObject o = readFile();
    if (!o.contains(key))
        return std::nullopt;
    return o.value(key).toString();
}

bool SecretStore::remove(const QString &key, QString *error)
{
    bool ok = true;
    if (!fileForced()) {
        QString e;
        if (keychainRemove(key, &e)) {
            g_secure = true;
            if (!e.isEmpty()) {
                ok = false;
                if (error)
                    *error = e;
            }
        } else {
            g_secure = false;
        }
    } else {
        g_secure = false;
    }
    QJsonObject o = readFile();
    if (o.contains(key)) {
        o.remove(key);
        ok = writeFile(o, error) && ok;
    }
    return ok;
}

bool SecretStore::secure()
{
    return g_secure && !fileForced();
}

QString SecretStore::where()
{
    if (!secure())
        return tr("a file only you can read (%1)").arg(QDir::toNativeSeparators(filePath()));
#if defined(Q_OS_WIN)
    return tr("the Windows Credential Manager");
#elif defined(Q_OS_MACOS)
    return tr("the macOS Keychain");
#else
    return tr("the desktop's keyring (Secret Service)");
#endif
}

QString SecretStore::fallbackReason()
{
    return g_reason;
}
