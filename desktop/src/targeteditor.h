#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QTextEdit>

// Where a translation is typed. Tags are objects in the text: they move
// and delete as one character and can't be broken by typing.
class QListWidget;

class TargetEditor : public QTextEdit {
    Q_OBJECT
public:
    explicit TargetEditor(QWidget *parent = nullptr);

    QJsonArray content() const;
    void setContent(const QJsonArray &content);
    // Words the spell checker doesn't know, with its suggestions.
    struct Misspelling {
        QString word;
        QStringList suggestions;
    };
    void setMisspellings(const QVector<Misspelling> &words);
    void insertTagAtCursor(const QJsonObject &tag);
    void insertPlain(const QString &text);
    // Words and terms that may complete the word being typed: the terms
    // found for the segment and the words of its memory matches.
    void setCompletions(const QStringList &words);
    // What is offered at the caret now, and which of it is chosen.
    QStringList offered() const;
    int offeredIndex() const;
    // What completes a word started with prefix: the candidates that start
    // with it, whatever its case, the shortest first.
    static QStringList completionsFor(const QString &prefix, const QStringList &candidates, int limit = 5);

signals:
    void edited();
    // Add the word to the project's words, so it counts as right.
    void addWord(const QString &word);

protected:
    void insertFromMimeData(const QMimeData *source) override;
    bool canInsertFromMimeData(const QMimeData *source) const override;
    QMimeData *createMimeDataFromSelection() const override;
    void contextMenuEvent(QContextMenuEvent *event) override;
    bool event(QEvent *event) override;
    void keyPressEvent(QKeyEvent *event) override;
    void focusOutEvent(QFocusEvent *event) override;
    void mousePressEvent(QMouseEvent *event) override;

private:
    void highlight();
    void suggest();
    void accept(const QString &word);
    void closeOffer();

    QStringList m_completions;
    QListWidget *m_offer = nullptr;
    QString m_prefix;

    QVector<Misspelling> m_misspellings;
    bool m_setting = false;
};
