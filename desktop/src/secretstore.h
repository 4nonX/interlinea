#pragma once

#include <QString>

#include <optional>

// Keeps secrets (sign-in tokens) in the system's keychain: the Windows
// Credential Manager, the macOS Keychain, or the Secret Service on Linux.
// Where none is available (a Linux desktop without one), they go into a
// file only the person can read, and secure() says so.
//
// INTERLINEA_SECRETS_FILE names that file and makes it the store, for tests.
class SecretStore {
public:
    static bool write(const QString &key, const QString &secret, QString *error = nullptr);
    static std::optional<QString> read(const QString &key, QString *error = nullptr);
    static bool remove(const QString &key, QString *error = nullptr);

    // Whether the last operation used the keychain.
    static bool secure();
    // Where secrets are kept, for people ("the Windows Credential Manager").
    static QString where();
    // Why the keychain could not be used, if it couldn't.
    static QString fallbackReason();
};
