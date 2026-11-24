#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QTextEdit>

// Where a translation is typed. Tags are objects in the text: they move
// and delete as one character and can't be broken by typing.
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

signals:
    void edited();
    // Add the word to the project's words, so it counts as right.
    void addWord(const QString &word);

protected:
    void insertFromMimeData(const QMimeData *source) override;
    bool canInsertFromMimeData(const QMimeData *source) const override;
    QMimeData *createMimeDataFromSelection() const override;
    void contextMenuEvent(QContextMenuEvent *event) override;

private:
    void highlight();

    QVector<Misspelling> m_misspellings;
    bool m_setting = false;
};
