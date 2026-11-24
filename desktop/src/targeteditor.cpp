#include "targeteditor.h"

#include "segmentcontent.h"

#include <QContextMenuEvent>
#include <QJsonDocument>
#include <QKeyEvent>
#include <QListWidget>
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
        // Completions follow typing, not text that is put in from elsewhere.
        if (hasFocus())
            suggest();
        else
            closeOffer();
    });
}

QStringList TargetEditor::completionsFor(const QString &prefix, const QStringList &candidates, int limit)
{
    const QString p = prefix.toLower();
    if (p.toUcs4().size() < 2)
        return {};
    QStringList out, seen;
    for (const QString &c : candidates) {
        const QString k = c.toLower();
        if (k == p || !k.startsWith(p) || seen.contains(k))
            continue;
        seen << k;
        out << c;
    }
    std::stable_sort(out.begin(), out.end(), [](const QString &a, const QString &b) {
        return a.size() != b.size() ? a.size() < b.size() : a.localeAwareCompare(b) < 0;
    });
    return out.mid(0, limit);
}

void TargetEditor::setCompletions(const QStringList &words)
{
    m_completions = words;
    if (m_offer && m_offer->isVisible())
        suggest();
}

QStringList TargetEditor::offered() const
{
    QStringList out;
    if (m_offer && m_offer->isVisible())
        for (int i = 0; i < m_offer->count(); ++i)
            out << m_offer->item(i)->text();
    return out;
}

int TargetEditor::offeredIndex() const
{
    return m_offer && m_offer->isVisible() ? m_offer->currentRow() : -1;
}

void TargetEditor::closeOffer()
{
    if (m_offer)
        m_offer->hide();
    m_prefix.clear();
}

// What is offered for the word the caret is at the end of.
void TargetEditor::suggest()
{
    const QTextCursor c = textCursor();
    if (isReadOnly() || m_completions.isEmpty() || c.hasSelection()) {
        closeOffer();
        return;
    }
    const QString block = c.block().text();
    const int at = c.positionInBlock();
    static const QRegularExpression start(QStringLiteral("[\\p{L}\\p{N}][\\p{L}\\p{N}\\p{M}'\u2019-]*$"),
                                          QRegularExpression::UseUnicodePropertiesOption);
    static const QRegularExpression goesOn(QStringLiteral("^[\\p{L}\\p{N}\\p{M}]"), QRegularExpression::UseUnicodePropertiesOption);
    const auto m = start.match(block.left(at));
    if (!m.hasMatch() || goesOn.match(block.mid(at)).hasMatch()) {
        closeOffer();
        return;
    }
    const QStringList items = completionsFor(m.captured(0), m_completions);
    if (items.isEmpty()) {
        closeOffer();
        return;
    }
    if (!m_offer) {
        // A list over the page that never takes the caret from the translation.
        m_offer = new QListWidget(window());
        m_offer->setObjectName(QStringLiteral("completions"));
        m_offer->setAccessibleName(tr("Completions"));
        m_offer->setFocusPolicy(Qt::NoFocus);
        m_offer->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        m_offer->setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
        m_offer->setSelectionMode(QAbstractItemView::SingleSelection);
        m_offer->setCursor(Qt::PointingHandCursor);
        connect(m_offer, &QListWidget::itemClicked, this, [this](QListWidgetItem *item) {
            accept(item->text());
            setFocus();
        });
    }
    if (m_offer->parentWidget() != window())
        m_offer->setParent(window());
    m_prefix = m.captured(0);
    m_offer->clear();
    m_offer->addItems(items);
    m_offer->setCurrentRow(0);
    int width = 160;
    for (const QString &item : items)
        width = qMax(width, m_offer->fontMetrics().horizontalAdvance(item) + 34);
    width = qMin(width, 360);
    const int height = m_offer->sizeHintForRow(0) * int(items.size()) + 10;
    const QWidget *w = window();
    QPoint pos = viewport()->mapTo(w, cursorRect().bottomLeft()) + QPoint(0, 4);
    pos.setX(qMax(4, qMin(pos.x(), w->width() - width - 4)));
    if (pos.y() + height > w->height() - 4)
        pos.setY(viewport()->mapTo(w, cursorRect().topLeft()).y() - height - 4);
    m_offer->setGeometry(pos.x(), pos.y(), width, height);
    m_offer->show();
    m_offer->raise();
}

// The rest of the chosen word goes in after what was typed, which stays as typed.
void TargetEditor::accept(const QString &word)
{
    // As many characters as were typed are left out, a pair of surrogates counting as one.
    qsizetype skip = 0;
    for (qsizetype typed = m_prefix.toUcs4().size(); typed > 0 && skip < word.size(); --typed)
        skip += word.at(skip).isHighSurrogate() && skip + 1 < word.size() ? 2 : 1;
    const QString rest = word.mid(skip);
    closeOffer();
    m_setting = true;
    insertPlain(rest);
    m_setting = false;
    highlight();
    emit edited();
}

bool TargetEditor::event(QEvent *event)
{
    if (m_offer && m_offer->isVisible()) {
        if (event->type() == QEvent::ShortcutOverride) {
            // While completions are offered these keys are the list's, not a shortcut's.
            const auto *k = static_cast<QKeyEvent *>(event);
            if (!(k->modifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier)) &&
                (k->key() == Qt::Key_Escape || k->key() == Qt::Key_Return || k->key() == Qt::Key_Enter || k->key() == Qt::Key_Tab ||
                 k->key() == Qt::Key_Up || k->key() == Qt::Key_Down)) {
                event->accept();
                return true;
            }
        }
        if (event->type() == QEvent::KeyPress) {
            // Tab would move on to the next field before the editor sees it.
            auto *k = static_cast<QKeyEvent *>(event);
            if (k->key() == Qt::Key_Tab && !(k->modifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier | Qt::ShiftModifier))) {
                accept(m_offer->currentItem() ? m_offer->currentItem()->text() : QString());
                return true;
            }
        }
    }
    return QTextEdit::event(event);
}

void TargetEditor::keyPressEvent(QKeyEvent *event)
{
    if (m_offer && m_offer->isVisible() && !(event->modifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier))) {
        const int n = m_offer->count();
        switch (event->key()) {
        case Qt::Key_Down:
            m_offer->setCurrentRow((m_offer->currentRow() + 1) % n);
            return;
        case Qt::Key_Up:
            m_offer->setCurrentRow((m_offer->currentRow() + n - 1) % n);
            return;
        case Qt::Key_Return:
        case Qt::Key_Enter:
            if (!(event->modifiers() & Qt::ShiftModifier)) {
                accept(m_offer->currentItem() ? m_offer->currentItem()->text() : QString());
                return;
            }
            break;
        case Qt::Key_Escape:
            closeOffer();
            return;
        case Qt::Key_Left:
        case Qt::Key_Right:
        case Qt::Key_Home:
        case Qt::Key_End:
            closeOffer();
            break;
        default:
            break;
        }
    }
    QTextEdit::keyPressEvent(event);
}

void TargetEditor::focusOutEvent(QFocusEvent *event)
{
    closeOffer();
    QTextEdit::focusOutEvent(event);
}

void TargetEditor::mousePressEvent(QMouseEvent *event)
{
    closeOffer();
    QTextEdit::mousePressEvent(event);
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
