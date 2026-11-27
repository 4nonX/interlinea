#pragma once

#include <QObject>
#include <QPointer>
#include <QVariant>
#include <QtQml/qqmlregistration.h>

#include <QQuickTextDocument>
class QTextDocument;

// A segment's content in a QML TextEdit: text with its tags as objects
// that draw as small labels ("1›", "‹1") and move and delete as one
// character, as in the web editor. Set document to the TextEdit's
// textDocument.
class SegmentDocument : public QObject {
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QQuickTextDocument *document READ document WRITE setDocument NOTIFY documentChanged)
    Q_PROPERTY(QVariantList content READ content WRITE setContent NOTIFY contentChanged)
    Q_PROPERTY(QString plainText READ plainText NOTIFY contentChanged)
    // Words the spelling check found, underlined in red where they stand.
    Q_PROPERTY(QStringList misspelled READ misspelled WRITE setMisspelled NOTIFY misspelledChanged)
public:
    explicit SegmentDocument(QObject *parent = nullptr);

    QQuickTextDocument *document() const { return m_quick; }
    void setDocument(QQuickTextDocument *d);
    QVariantList content() const;
    void setContent(const QVariantList &content);
    QString plainText() const;

    QStringList misspelled() const { return m_misspelled; }
    void setMisspelled(const QStringList &words);

    // Puts a tag where the caret is.
    Q_INVOKABLE void insertTag(int position, const QVariantMap &tag);
    // Puts text in place of what lies between start and end; where the
    // caret goes after it.
    Q_INVOKABLE int replace(int start, int end, const QString &text);
    // The content between two places, tags and all.
    Q_INVOKABLE QVariantList contentBetween(int start, int end) const;
    // The text between two places, tags left out.
    Q_INVOKABLE QString textBetween(int start, int end) const;
    // The word a place is in: { start, end, word }.
    Q_INVOKABLE QVariantMap wordAt(int position) const;
    // Copies what lies between two places, keeping its tags for a paste
    // into a segment; paste() puts the clipboard there (its tags with it,
    // or its text) and gives where the caret goes.
    Q_INVOKABLE void copy(int start, int end) const;
    Q_INVOKABLE int paste(int start, int end);
    // The tags of a content, in order.
    Q_INVOKABLE static QVariantList tags(const QVariantList &content);
    Q_INVOKABLE static QString tagLabel(const QVariantMap &tag);
    Q_INVOKABLE static QString textOf(const QVariantList &content);

signals:
    void documentChanged();
    void contentChanged();
    void misspelledChanged();

private:
    QTextDocument *doc() const;
    QPointer<QQuickTextDocument> m_quick;
    QVariantList m_pending;
    bool m_hasPending = false;
    bool m_setting = false;
    QStringList m_misspelled;
    QPointer<QObject> m_highlighter;
};
