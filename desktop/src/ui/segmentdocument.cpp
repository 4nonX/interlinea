#include "segmentdocument.h"

#include "segmentcontent.h"

#include <QClipboard>
#include <QGuiApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMimeData>
#include <QRegularExpression>
#include <QSyntaxHighlighter>
#include <QTextBlock>
#include <QTextDocumentFragment>
#include <QQuickTextDocument>
#include <QTextCursor>
#include <QTextDocument>

namespace {

const char *ContentMime = "application/x-interlinea-content";

// Underlines the words a spelling check found.
class Misspellings : public QSyntaxHighlighter {
public:
    using QSyntaxHighlighter::QSyntaxHighlighter;
    QStringList words;

protected:
    void highlightBlock(const QString &text) override
    {
        QTextCharFormat f;
        f.setUnderlineStyle(QTextCharFormat::WaveUnderline);
        f.setUnderlineColor(QColor(0xd1, 0x24, 0x2f));
        for (const QString &w : std::as_const(words)) {
            if (w.isEmpty())
                continue;
            const QRegularExpression re(QStringLiteral("(?<![\\p{L}\\p{N}])%1(?![\\p{L}\\p{N}])").arg(QRegularExpression::escape(w)),
                                        QRegularExpression::UseUnicodePropertiesOption);
            for (auto it = re.globalMatch(text); it.hasNext();) {
                const auto m = it.next();
                setFormat(int(m.capturedStart()), int(m.capturedLength()), f);
            }
        }
    }
};

} // namespace

SegmentDocument::SegmentDocument(QObject *parent)
    : QObject(parent)
{
}

QTextDocument *SegmentDocument::doc() const
{
    return m_quick ? m_quick->textDocument() : nullptr;
}

void SegmentDocument::setDocument(QQuickTextDocument *d)
{
    if (m_quick == d)
        return;
    m_quick = d;
    if (QTextDocument *t = doc()) {
        SegmentContent::enableTags(t);
        auto *h = new Misspellings(t);
        h->words = m_misspelled;
        m_highlighter = h;
        connect(t, &QTextDocument::contentsChanged, this, [this] {
            if (!m_setting)
                emit contentChanged();
        });
        if (m_hasPending) {
            m_hasPending = false;
            setContent(m_pending);
        }
    }
    emit documentChanged();
}

QVariantList SegmentDocument::content() const
{
    if (const QTextDocument *t = doc())
        return SegmentContent::fromDocument(t).toVariantList();
    return m_pending;
}

void SegmentDocument::setContent(const QVariantList &content)
{
    QTextDocument *t = doc();
    if (!t) {
        m_pending = content;
        m_hasPending = true;
        emit contentChanged();
        return;
    }
    if (SegmentContent::equal(SegmentContent::fromDocument(t), QJsonArray::fromVariantList(content)))
        return;
    m_setting = true;
    SegmentContent::setDocument(t, QJsonArray::fromVariantList(content));
    m_setting = false;
    emit contentChanged();
}

QString SegmentDocument::plainText() const
{
    return SegmentContent::plainText(QJsonArray::fromVariantList(content()));
}

void SegmentDocument::insertTag(int position, const QVariantMap &tag)
{
    QTextDocument *t = doc();
    if (!t)
        return;
    QTextCursor c(t);
    c.setPosition(qBound(0, position, t->characterCount() - 1));
    SegmentContent::insertTag(c, QJsonObject::fromVariantMap(tag));
}

QVariantList SegmentDocument::tags(const QVariantList &content)
{
    QVariantList out;
    for (const QJsonObject &t : SegmentContent::tags(QJsonArray::fromVariantList(content)))
        out.append(t.toVariantMap());
    return out;
}

QString SegmentDocument::tagLabel(const QVariantMap &tag)
{
    return SegmentContent::tagLabel(QJsonObject::fromVariantMap(tag));
}

QString SegmentDocument::textOf(const QVariantList &content)
{
    return SegmentContent::plainText(QJsonArray::fromVariantList(content));
}

void SegmentDocument::setMisspelled(const QStringList &words)
{
    if (m_misspelled == words)
        return;
    m_misspelled = words;
    if (auto *h = static_cast<Misspellings *>(m_highlighter.data())) {
        h->words = words;
        // The underline is a format of its own: the text is the same.
        m_setting = true;
        h->rehighlight();
        m_setting = false;
    }
    emit misspelledChanged();
}

int SegmentDocument::replace(int start, int end, const QString &text)
{
    QTextDocument *t = doc();
    if (!t)
        return start;
    const int last = t->characterCount() - 1;
    QTextCursor c(t);
    c.setPosition(qBound(0, start, last));
    c.setPosition(qBound(0, end, last), QTextCursor::KeepAnchor);
    c.insertText(text, QTextCharFormat());
    return c.position();
}

QVariantList SegmentDocument::contentBetween(int start, int end) const
{
    const QTextDocument *t = doc();
    if (!t)
        return {};
    const int last = t->characterCount() - 1;
    QTextCursor c(const_cast<QTextDocument *>(t));
    c.setPosition(qBound(0, qMin(start, end), last));
    c.setPosition(qBound(0, qMax(start, end), last), QTextCursor::KeepAnchor);
    QTextDocument part;
    SegmentContent::enableTags(&part);
    QTextCursor(&part).insertFragment(c.selection());
    return SegmentContent::fromDocument(&part).toVariantList();
}

QString SegmentDocument::textBetween(int start, int end) const
{
    return SegmentContent::plainText(QJsonArray::fromVariantList(contentBetween(start, end)));
}

QVariantMap SegmentDocument::wordAt(int position) const
{
    const QTextDocument *t = doc();
    if (!t)
        return {};
    QTextCursor c(const_cast<QTextDocument *>(t));
    c.setPosition(qBound(0, position, t->characterCount() - 1));
    c.select(QTextCursor::WordUnderCursor);
    return {{QStringLiteral("start"), c.selectionStart()}, {QStringLiteral("end"), c.selectionEnd()}, {QStringLiteral("word"), c.selectedText()}};
}

void SegmentDocument::copy(int start, int end) const
{
    const QJsonArray content = QJsonArray::fromVariantList(contentBetween(start, end));
    auto *mime = new QMimeData;
    mime->setData(QLatin1String(ContentMime), QJsonDocument(content).toJson(QJsonDocument::Compact));
    mime->setText(SegmentContent::plainText(content));
    QGuiApplication::clipboard()->setMimeData(mime);
}

int SegmentDocument::paste(int start, int end)
{
    QTextDocument *t = doc();
    const QMimeData *mime = QGuiApplication::clipboard()->mimeData();
    if (!t || !mime)
        return start;
    const int last = t->characterCount() - 1;
    QTextCursor c(t);
    c.setPosition(qBound(0, qMin(start, end), last));
    c.setPosition(qBound(0, qMax(start, end), last), QTextCursor::KeepAnchor);
    c.beginEditBlock();
    c.removeSelectedText();
    if (mime->hasFormat(QLatin1String(ContentMime))) {
        // Text and tags copied from a segment keep their tags.
        for (const QJsonValue &v : QJsonDocument::fromJson(mime->data(QLatin1String(ContentMime))).array()) {
            const QJsonObject run = v.toObject();
            if (run.contains(QLatin1String("tag")))
                SegmentContent::insertTag(c, run.value(QLatin1String("tag")).toObject());
            else
                c.insertText(run.value(QLatin1String("text")).toString(), QTextCharFormat());
        }
    } else {
        // Tags can't be pasted as characters.
        QString text = mime->text();
        text.remove(QChar::ObjectReplacementCharacter);
        c.insertText(text, QTextCharFormat());
    }
    c.endEditBlock();
    return c.position();
}
