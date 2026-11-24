#include "segmentcontent.h"

#include "theme.h"

#include <QAbstractTextDocumentLayout>
#include <QApplication>
#include <QFontMetricsF>
#include <QJsonDocument>
#include <QPainter>
#include <QPainterPath>
#include <QPalette>
#include <QTextBlock>
#include <QTextCursor>
#include <QTextDocument>
#include <QTextFragment>

namespace SegmentContent {

QString plainText(const QJsonArray &content)
{
    QString out;
    for (const QJsonValue &v : content)
        out += v.toObject().value(QLatin1String("text")).toString();
    return out;
}

QString editorText(const QJsonArray &content)
{
    QString out;
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (run.contains(QLatin1String("tag")))
            out += QChar::ObjectReplacementCharacter;
        else
            out += run.value(QLatin1String("text")).toString();
    }
    return out;
}

QVector<QJsonObject> tags(const QJsonArray &content)
{
    QVector<QJsonObject> out;
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (run.contains(QLatin1String("tag")))
            out << run.value(QLatin1String("tag")).toObject();
    }
    return out;
}

QString tagLabel(const QJsonObject &tag)
{
    const QString id = tag.value(QLatin1String("id")).toString();
    const QString kind = tag.value(QLatin1String("kind")).toString();
    if (kind == QLatin1String("open"))
        return id + QChar(0x203A);
    if (kind == QLatin1String("close"))
        return QChar(0x2039) + id;
    return QChar(0x27E8) + id + QChar(0x27E9);
}

static QString tagHtml(const QJsonObject &tag)
{
    const QString desc = tag.value(QLatin1String("desc")).toString();
    return QStringLiteral("<span class=\"tag\" title=\"%1\">&nbsp;%2&nbsp;</span>")
        .arg(desc.toHtmlEscaped(), tagLabel(tag).toHtmlEscaped());
}

QString toHtml(const QJsonArray &content, const QVector<Mark> &marks)
{
    QString out;
    int pos = 0; // in the plain text
    auto markAt = [&marks](int p) -> const Mark * {
        for (const Mark &m : marks) {
            if (p >= m.start && p < m.end)
                return &m;
        }
        return nullptr;
    };
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (run.contains(QLatin1String("tag"))) {
            out += tagHtml(run.value(QLatin1String("tag")).toObject());
            continue;
        }
        const QString text = run.value(QLatin1String("text")).toString();
        int i = 0;
        while (i < text.size()) {
            const Mark *m = markAt(pos + i);
            int j = i + 1;
            while (j < text.size() && markAt(pos + j) == m)
                ++j;
            QString piece = text.mid(i, j - i).toHtmlEscaped();
            piece.replace(QLatin1Char('\n'), QLatin1String("<br>"));
            if (m)
                out += QStringLiteral("<span class=\"%1\">%2</span>").arg(m->cssClass, piece);
            else
                out += piece;
            i = j;
        }
        pos += text.size();
    }
    return out;
}

QString css()
{
    return QStringLiteral(".tag{background:%1;color:%2;font-size:11px;font-weight:600;}"
                          ".term{background:%3;text-decoration:underline;}"
                          ".mark{background:%4;}"
                          ".missing{color:%5;font-style:italic;}")
        .arg(Theme::color("tagBg").name(), Theme::color("server").name(), Theme::color("termHit").name(),
             Theme::color("mark").name(), Theme::color("text3").name());
}

// Words and the spaces between them, each its own token.
static QStringList tokens(const QString &s)
{
    QStringList out;
    QString cur;
    bool space = false;
    for (QChar c : s) {
        const bool sp = c.isSpace() || c.isPunct();
        if (!cur.isEmpty() && (sp || space)) {
            out << cur;
            cur.clear();
        }
        cur += c;
        space = sp;
    }
    if (!cur.isEmpty())
        out << cur;
    return out;
}

QString diffHtml(const QString &a, const QString &b)
{
    // The match's words, with those the segment doesn't have marked, as
    // the web editor shows a fuzzy match.
    const QStringList x = tokens(a);
    const QStringList y = tokens(b);
    const int n = x.size(), m = y.size();
    QVector<QVector<int>> l(n + 1, QVector<int>(m + 1, 0));
    for (int i = n - 1; i >= 0; --i) {
        for (int j = m - 1; j >= 0; --j)
            l[i][j] = x[i] == y[j] ? l[i + 1][j + 1] + 1 : qMax(l[i + 1][j], l[i][j + 1]);
    }
    QString out;
    int i = 0, j = 0;
    while (j < m) {
        if (i < n && x[i] == y[j]) {
            out += y[j].toHtmlEscaped();
            ++i;
            ++j;
        } else if (i == n || l[i][j + 1] >= l[i + 1][j]) {
            const bool space = y[j].trimmed().isEmpty();
            out += space ? y[j].toHtmlEscaped() : QStringLiteral("<span class=\"mark\">%1</span>").arg(y[j].toHtmlEscaped());
            ++j;
        } else {
            ++i;
        }
    }
    return out;
}

QJsonArray normalized(const QJsonArray &content)
{
    QJsonArray out;
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (run.contains(QLatin1String("tag"))) {
            out.append(QJsonObject{{QStringLiteral("tag"), run.value(QLatin1String("tag"))}});
            continue;
        }
        const QString text = run.value(QLatin1String("text")).toString();
        if (text.isEmpty())
            continue;
        if (!out.isEmpty() && out.last().toObject().contains(QLatin1String("text"))) {
            const QString prev = out.last().toObject().value(QLatin1String("text")).toString();
            out[out.size() - 1] = QJsonObject{{QStringLiteral("text"), prev + text}};
        } else {
            out.append(QJsonObject{{QStringLiteral("text"), text}});
        }
    }
    return out;
}

bool equal(const QJsonArray &a, const QJsonArray &b)
{
    return normalized(a) == normalized(b);
}

QSizeF TagObject::intrinsicSize(QTextDocument *doc, int, const QTextFormat &format)
{
    const QJsonObject tag = QJsonDocument::fromJson(format.property(TagJsonProperty).toByteArray()).object();
    QFont f = doc->defaultFont();
    f.setPixelSize(11);
    f.setWeight(QFont::DemiBold);
    const QFontMetricsF fm(f);
    return QSizeF(fm.horizontalAdvance(tagLabel(tag)) + 12, QFontMetricsF(doc->defaultFont()).height());
}

void TagObject::drawObject(QPainter *painter, const QRectF &rect, QTextDocument *doc, int, const QTextFormat &format)
{
    const QJsonObject tag = QJsonDocument::fromJson(format.property(TagJsonProperty).toByteArray()).object();
    painter->save();
    painter->setRenderHint(QPainter::Antialiasing);
    // As the web's .tag-chip: opening tags round to the right, closing
    // ones to the left.
    const QRectF r = rect.adjusted(1, 3, -1, -3);
    const QString kind = tag.value(QLatin1String("kind")).toString();
    const double small = 4, big = r.height() / 2;
    const double tl = kind == QLatin1String("close") ? big : small;
    const double tr = kind == QLatin1String("open") ? big : small;
    QPainterPath path;
    path.moveTo(r.left() + tl, r.top());
    path.lineTo(r.right() - tr, r.top());
    path.arcTo(QRectF(r.right() - 2 * tr, r.top(), 2 * tr, 2 * tr), 90, -90);
    path.lineTo(r.right(), r.bottom() - tr);
    path.arcTo(QRectF(r.right() - 2 * tr, r.bottom() - 2 * tr, 2 * tr, 2 * tr), 0, -90);
    path.lineTo(r.left() + tl, r.bottom());
    path.arcTo(QRectF(r.left(), r.bottom() - 2 * tl, 2 * tl, 2 * tl), 270, -90);
    path.lineTo(r.left(), r.top() + tl);
    path.arcTo(QRectF(r.left(), r.top(), 2 * tl, 2 * tl), 180, -90);
    painter->fillPath(path, Theme::color("tagBg"));
    QFont f = doc->defaultFont();
    f.setPixelSize(11);
    f.setWeight(QFont::DemiBold);
    painter->setFont(f);
    painter->setPen(Theme::color("server"));
    painter->drawText(rect, Qt::AlignCenter, tagLabel(tag));
    painter->restore();
}

void enableTags(QTextDocument *doc)
{
    if (doc->property("interlineaTags").toBool())
        return;
    auto *handler = new TagObject(doc);
    doc->documentLayout()->registerHandler(TagObjectType, handler);
    doc->setProperty("interlineaTags", true);
}

void insertTag(QTextCursor &cursor, const QJsonObject &tag)
{
    QTextCharFormat f;
    f.setObjectType(TagObjectType);
    f.setProperty(TagJsonProperty, QJsonDocument(tag).toJson(QJsonDocument::Compact));
    f.setToolTip(tag.value(QLatin1String("desc")).toString());
    cursor.insertText(QString(QChar::ObjectReplacementCharacter), f);
    // Typing after a tag is plain text.
    cursor.setCharFormat(QTextCharFormat());
}

void setDocument(QTextDocument *doc, const QJsonArray &content)
{
    enableTags(doc);
    doc->clear();
    QTextCursor c(doc);
    for (const QJsonValue &v : content) {
        const QJsonObject run = v.toObject();
        if (run.contains(QLatin1String("tag")))
            insertTag(c, run.value(QLatin1String("tag")).toObject());
        else
            c.insertText(run.value(QLatin1String("text")).toString(), QTextCharFormat());
    }
}

QJsonArray fromDocument(const QTextDocument *doc)
{
    QJsonArray out;
    QString text;
    auto flush = [&] {
        if (!text.isEmpty())
            out.append(QJsonObject{{QStringLiteral("text"), text}});
        text.clear();
    };
    bool firstBlock = true;
    for (QTextBlock b = doc->begin(); b.isValid(); b = b.next()) {
        if (!firstBlock)
            text += QLatin1Char('\n');
        firstBlock = false;
        for (auto it = b.begin(); !it.atEnd(); ++it) {
            const QTextFragment frag = it.fragment();
            const QTextCharFormat f = frag.charFormat();
            if (f.objectType() == TagObjectType) {
                // One fragment may hold several tags with equal formats.
                const QJsonObject tag =
                    QJsonDocument::fromJson(f.property(TagJsonProperty).toByteArray()).object();
                for (int i = 0; i < frag.length(); ++i) {
                    flush();
                    out.append(QJsonObject{{QStringLiteral("tag"), tag}});
                }
            } else {
                text += frag.text();
            }
        }
    }
    flush();
    return out;
}

} // namespace SegmentContent
