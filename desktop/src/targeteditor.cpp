#include "targeteditor.h"

#include "segmentcontent.h"

#include <QContextMenuEvent>
#include <QJsonDocument>
#include <QMenu>
#include <QMimeData>
#include <QRegularExpression>
#include <QTextBlock>
#include <QTextDocumentFragment>

static const char *TagsMime = "application/x-interlinea-content";

TargetEditor::TargetEditor(QWidget *parent)
    : QTextEdit(parent)
{
    setAcceptRichText(false);
    setTabChangesFocus(true);
    SegmentContent::enableTags(document());
    connect(document(), &QTextDocument::contentsChanged, this, [this] {
        if (m_setting)
            return;
        highlight();
        emit edited();
    });
}

QJsonArray TargetEditor::content() const
{
    return SegmentContent::fromDocument(document());
}

void TargetEditor::setContent(const QJsonArray &content)
{
    m_setting = true;
    SegmentContent::setDocument(document(), content);
    m_misspellings.clear();
    setExtraSelections({});
    m_setting = false;
    moveCursor(QTextCursor::End);
}

void TargetEditor::insertTagAtCursor(const QJsonObject &tag)
{
    QTextCursor c = textCursor();
    SegmentContent::insertTag(c, tag);
    setTextCursor(c);
}

void TargetEditor::insertPlain(const QString &text)
{
    QTextCursor c = textCursor();
    c.insertText(text, QTextCharFormat());
    setTextCursor(c);
}

bool TargetEditor::canInsertFromMimeData(const QMimeData *source) const
{
    return source->hasFormat(QLatin1String(TagsMime)) || source->hasText();
}

void TargetEditor::insertFromMimeData(const QMimeData *source)
{
    QTextCursor c = textCursor();
    if (source->hasFormat(QLatin1String(TagsMime))) {
        // Text and tags copied from a segment keep their tags.
        const QJsonArray content = QJsonDocument::fromJson(source->data(QLatin1String(TagsMime))).array();
        for (const QJsonValue &v : content) {
            const QJsonObject run = v.toObject();
            if (run.contains(QLatin1String("tag")))
                SegmentContent::insertTag(c, run.value(QLatin1String("tag")).toObject());
            else
                c.insertText(run.value(QLatin1String("text")).toString(), QTextCharFormat());
        }
    } else {
        QString text = source->text();
        // Tags can't be pasted as characters.
        text.remove(QChar::ObjectReplacementCharacter);
        c.insertText(text, QTextCharFormat());
    }
    setTextCursor(c);
}

QMimeData *TargetEditor::createMimeDataFromSelection() const
{
    auto *mime = new QMimeData;
    const QTextCursor sel = textCursor();
    QTextDocument part;
    SegmentContent::enableTags(&part);
    QTextCursor c(&part);
    c.insertFragment(sel.selection());
    const QJsonArray content = SegmentContent::fromDocument(&part);
    mime->setData(QLatin1String(TagsMime), QJsonDocument(content).toJson(QJsonDocument::Compact));
    mime->setText(SegmentContent::plainText(content));
    return mime;
}

void TargetEditor::setMisspellings(const QVector<Misspelling> &words)
{
    m_misspellings = words;
    highlight();
}

void TargetEditor::highlight()
{
    QList<QTextEdit::ExtraSelection> out;
    const QString text = toPlainText();
    for (const Misspelling &m : std::as_const(m_misspellings)) {
        if (m.word.isEmpty())
            continue;
        const QRegularExpression re(QStringLiteral("\\b%1\\b").arg(QRegularExpression::escape(m.word)),
                                    QRegularExpression::UseUnicodePropertiesOption);
        auto it = re.globalMatch(text);
        while (it.hasNext()) {
            const auto match = it.next();
            QTextEdit::ExtraSelection s;
            s.cursor = QTextCursor(document());
            s.cursor.setPosition(match.capturedStart());
            s.cursor.setPosition(match.capturedEnd(), QTextCursor::KeepAnchor);
            s.format.setUnderlineStyle(QTextCharFormat::SpellCheckUnderline);
            s.format.setUnderlineColor(Qt::red);
            s.format.setToolTip(tr("Not in the dictionary"));
            out << s;
        }
    }
    setExtraSelections(out);
}

void TargetEditor::contextMenuEvent(QContextMenuEvent *event)
{
    QMenu *menu = createStandardContextMenu(event->pos());
    QTextCursor at = cursorForPosition(event->pos());
    at.select(QTextCursor::WordUnderCursor);
    const QString word = at.selectedText();
    for (const Misspelling &m : std::as_const(m_misspellings)) {
        if (m.word != word)
            continue;
        QAction *first = menu->actions().value(0);
        for (const QString &s : m.suggestions.mid(0, 6)) {
            auto *a = new QAction(s, menu);
            connect(a, &QAction::triggered, this, [this, at, s]() mutable {
                at.insertText(s);
            });
            menu->insertAction(first, a);
        }
        if (m.suggestions.isEmpty()) {
            auto *none = new QAction(tr("No suggestions"), menu);
            none->setEnabled(false);
            menu->insertAction(first, none);
        }
        auto *add = new QAction(tr("Add “%1” to the project's words").arg(word), menu);
        connect(add, &QAction::triggered, this, [this, word] { emit addWord(word); });
        menu->insertAction(first, add);
        menu->insertSeparator(first);
        break;
    }
    menu->exec(event->globalPos());
    delete menu;
}
