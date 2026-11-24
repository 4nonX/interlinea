#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QString>
#include <QTextFormat>
#include <QTextObjectInterface>
#include <QVector>

class QTextDocument;
class QTextCursor;

// A segment's content as the API sends it: runs of text and tags
// ([{"text": "…"}, {"tag": {"id": "1", "kind": "open", …}}]).
namespace SegmentContent {

// The text of the runs, tags left out.
QString plainText(const QJsonArray &content);
// The text with each tag as one U+FFFC, as an editor holds it.
QString editorText(const QJsonArray &content);
// The tags in order.
QVector<QJsonObject> tags(const QJsonArray &content);
// How a tag shows: "1›" opens, "‹1" closes, "⟨1⟩" stands alone.
QString tagLabel(const QJsonObject &tag);
// HTML for showing the content read-only. marks are character ranges of
// the plain text (UTF-16) to underline, such as found terms.
struct Mark {
    int start = 0;
    int end = 0;
    QString cssClass;
};
QString toHtml(const QJsonArray &content, const QVector<Mark> &marks = {});
// The CSS the HTML expects, in the theme's colours.
QString css();
// The words of b, those a lacks marked (a match's source against the
// segment's).
QString diffHtml(const QString &a, const QString &b);
bool equal(const QJsonArray &a, const QJsonArray &b);
// Keeps the text and tags with the same id in order, merging neighbours.
QJsonArray normalized(const QJsonArray &content);

// Tags in a QTextDocument: an inline object that draws as a small label
// and keeps the tag's JSON.
constexpr int TagObjectType = QTextFormat::UserObject + 1;
constexpr int TagJsonProperty = QTextFormat::UserProperty + 1;

class TagObject : public QObject, public QTextObjectInterface {
    Q_OBJECT
    Q_INTERFACES(QTextObjectInterface)
public:
    using QObject::QObject;
    QSizeF intrinsicSize(QTextDocument *doc, int posInDocument, const QTextFormat &format) override;
    void drawObject(QPainter *painter, const QRectF &rect, QTextDocument *doc, int posInDocument,
                    const QTextFormat &format) override;
};

// Registers the tag object with a document's layout (once per document).
void enableTags(QTextDocument *doc);
void insertTag(QTextCursor &cursor, const QJsonObject &tag);
// Replaces a document's text with content.
void setDocument(QTextDocument *doc, const QJsonArray &content);
// The content a document holds.
QJsonArray fromDocument(const QTextDocument *doc);

} // namespace SegmentContent
