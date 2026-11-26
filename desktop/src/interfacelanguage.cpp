#include "interfacelanguage.h"

#include <QCoreApplication>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLibraryInfo>
#include <QLocale>
#include <QSettings>

namespace {
const QString key = QStringLiteral("interface/language");
QString shown;
} // namespace

QStringList InterfaceLanguage::codes()
{
    return {QStringLiteral("en"), QStringLiteral("de"), QStringLiteral("ko")};
}

QString InterfaceLanguage::nativeName(const QString &code)
{
    if (code == QLatin1String("de"))
        return QStringLiteral("Deutsch");
    if (code == QLatin1String("ko"))
        return QStringLiteral("한국어");
    return QStringLiteral("English");
}

QString InterfaceLanguage::chosen()
{
    const QString v = QSettings().value(key).toString();
    return codes().contains(v) ? v : QString();
}

void InterfaceLanguage::choose(const QString &code)
{
    if (codes().contains(code))
        QSettings().setValue(key, code);
    else
        QSettings().remove(key);
}

QString InterfaceLanguage::systemLanguage()
{
    for (const QString &l : QLocale::system().uiLanguages()) {
        const QString base = l.section(QLatin1Char('-'), 0, 0).section(QLatin1Char('_'), 0, 0).toLower();
        if (codes().contains(base))
            return base;
    }
    return QStringLiteral("en");
}

QString InterfaceLanguage::current()
{
    if (shown.isEmpty()) {
        // INTERLINEA_INTERFACE_LANGUAGE sets it for a run (the tests).
        const QString env = qEnvironmentVariable("INTERLINEA_INTERFACE_LANGUAGE");
        shown = codes().contains(env) ? env : !chosen().isEmpty() ? chosen() : systemLanguage();
    }
    return shown;
}

QLocale InterfaceLanguage::locale()
{
    const QString l = current();
    return l == QLatin1String("de") ? QLocale(QLocale::German, QLocale::Germany)
         : l == QLatin1String("ko") ? QLocale(QLocale::Korean, QLocale::SouthKorea)
                                    : QLocale(QLocale::English, QLocale::UnitedKingdom);
}

void InterfaceLanguage::install(QCoreApplication &app)
{
    const QString l = current();
    if (l == QLatin1String("en"))
        return;
    QLocale::setDefault(locale());
    auto *own = new CatalogTranslator;
    if (own->loadCatalog(QStringLiteral(":/i18n/%1.json").arg(l)))
        app.installTranslator(own);
    else
        delete own;
    // Qt's own texts (the buttons of its dialogs), where Qt has them.
    auto *qt = new QTranslator(&app);
    if (qt->load(QLocale(l), QStringLiteral("qtbase"), QStringLiteral("_"), QLibraryInfo::path(QLibraryInfo::TranslationsPath)))
        app.installTranslator(qt);
    else
        delete qt;
}

bool CatalogTranslator::loadCatalog(const QString &path)
{
    QFile f(path);
    if (!f.open(QIODevice::ReadOnly))
        return false;
    const QJsonObject o = QJsonDocument::fromJson(f.readAll()).object();
    m_strings.clear();
    m_strings.reserve(o.size());
    for (auto it = o.begin(); it != o.end(); ++it) {
        const QString v = it.value().toString();
        if (!v.isEmpty())
            m_strings.insert(it.key(), v);
    }
    return !m_strings.isEmpty();
}

QString CatalogTranslator::translate(const char *, const char *sourceText, const char *, int) const
{
    if (!sourceText)
        return QString();
    // A string not translated yet is shown in English: a null string tells
    // Qt to use the source text.
    return m_strings.value(QString::fromUtf8(sourceText));
}
