#include "documentpreview.h"

#include "segmentcontent.h"

#include <QCoreApplication>
#include <QHash>
#include <QMap>

namespace {

// The runs of a segment with the formatting their tags stand for, as the
// preview shows them: no tags, but bold, italic and the like. notes
// counts the footnote marks of the page.
QString formatted(const QJsonArray &runs, int *notes)
{
    struct Frame {
        QString type;
        QString id;
        QString html;
    };
    QList<Frame> stack{Frame{}};
    const auto wrap = [](const Frame &f) {
        if (f.type == QLatin1String("bold"))
            return QStringLiteral("<b>%1</b>").arg(f.html);
        if (f.type == QLatin1String("italic"))
            return QStringLiteral("<i>%1</i>").arg(f.html);
        if (f.type == QLatin1String("underline"))
            return QStringLiteral("<u>%1</u>").arg(f.html);
        if (f.type == QLatin1String("superscript"))
            return QStringLiteral("<sup>%1</sup>").arg(f.html);
        if (f.type == QLatin1String("subscript"))
            return QStringLiteral("<sub>%1</sub>").arg(f.html);
        if (f.type == QLatin1String("link"))
            return QStringLiteral("<span style=\"color:#0563c1; text-decoration:underline\">%1</span>").arg(f.html);
        return f.html;
    };
    for (const QJsonValue &v : runs) {
        const QJsonObject run = v.toObject();
        if (!run.contains(QLatin1String("tag"))) {
            QString text = run.value(QLatin1String("text")).toString().toHtmlEscaped();
            text.replace(QLatin1Char('\n'), QLatin1String("<br>"));
            stack.last().html += text;
            continue;
        }
        const QJsonObject tag = run.value(QLatin1String("tag")).toObject();
        const QString kind = tag.value(QLatin1String("kind")).toString();
        const QString type = tag.value(QLatin1String("type")).toString();
        const QString id = tag.value(QLatin1String("id")).toString();
        if (kind == QLatin1String("open")) {
            stack.append(Frame{type, id, QString()});
        } else if (kind == QLatin1String("close")) {
            int at = -1;
            for (int i = int(stack.size()) - 1; i > 0; --i) {
                if (stack.at(i).id == id) {
                    at = i;
                    break;
                }
            }
            if (at <= 0)
                continue;
            while (stack.size() > at) {
                const Frame f = stack.takeLast();
                stack.last().html += wrap(f);
            }
        } else if (type == QLatin1String("footnote")) {
            stack.last().html += QStringLiteral("<sup>%1</sup>").arg(++*notes);
        } else if (type == QLatin1String("lb")) {
            stack.last().html += QStringLiteral("<br>");
        } else if (type == QLatin1String("tab")) {
            stack.last().html += QStringLiteral("&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;");
        } else if (type == QLatin1String("field")) {
            stack.last().html += QStringLiteral("<span style=\"background:#e8e8e8; color:#555555\">&nbsp;#&nbsp;</span>");
        } else if (type == QLatin1String("image")) {
            stack.last().html += QStringLiteral("<span style=\"color:#777777; font-size:10px\">[%1]</span>").arg(QCoreApplication::translate("EditorPage", "image"));
        }
    }
    while (stack.size() > 1) {
        const Frame f = stack.takeLast();
        stack.last().html += wrap(f);
    }
    return stack.first().html;
}

} // namespace

// The preview: the document's paragraphs with their styles, tables,
// notes, header and footer, each segment in its translation, or in the
// source text, grey and underlined, while it has none. Built like the
// browser client's.
QString previewHtml(const QList<QJsonObject> &all, const QJsonArray &layout, const QString &activeId)
{
    int notes = 0;
    QHash<QString, QList<QJsonObject>> byBlock;
    for (const QJsonObject &s : all)
        byBlock[s.value(QLatin1String("block")).toString()].append(s);
    const auto segments = [&notes, &activeId](const QList<QJsonObject> &segs) {
        QString out;
        for (int i = 0; i < segs.size(); ++i) {
            const QJsonObject &s = segs.at(i);
            const QJsonArray target = s.value(QLatin1String("target")).toArray();
            const bool translated = !SegmentContent::plainText(target).trimmed().isEmpty();
            const QString id = s.value(QLatin1String("id")).toString();
            QString style = QStringLiteral("text-decoration:none; color:%1;").arg(translated ? QStringLiteral("#111111") : QStringLiteral("#6b7280"));
            if (id == activeId)
                style += QStringLiteral(" background:#dbe6fb;");
            QString text = formatted(translated ? target : s.value(QLatin1String("source")).toArray(), &notes);
            if (!translated)
                text = QStringLiteral("<u>%1</u>").arg(text);
            if (i > 0)
                out += QLatin1Char(' ');
            out += QStringLiteral("<a href=\"seg:%1\" title=\"%2\" style=\"%3\">%4</a>")
                       .arg(id.toHtmlEscaped(), translated ? QString() : QCoreApplication::translate("EditorPage", "Not translated yet: the source text is used").toHtmlEscaped(), style, text);
        }
        return out;
    };
    const auto marks = [&notes](const QJsonArray &types, const QString &note) {
        QString out;
        for (const QJsonValue &t : types) {
            if (t.toString() == QLatin1String("footnote"))
                out += QStringLiteral("<sup>%1</sup>").arg(++notes);
            else if (t.toString() == QLatin1String("note-mark"))
                out += QStringLiteral("<sup>%1</sup>").arg(note.toHtmlEscaped());
            else if (t.toString() == QLatin1String("field"))
                out += QStringLiteral("<span style=\"background:#e8e8e8; color:#555555\">&nbsp;#&nbsp;</span>");
        }
        return out;
    };
    const auto paragraph = [&](const QJsonObject &b, const QString &base) {
        const QString inner = marks(b.value(QLatin1String("lead")).toArray(), b.value(QLatin1String("note")).toString()) +
                              segments(byBlock.value(b.value(QLatin1String("id")).toString())) +
                              marks(b.value(QLatin1String("trail")).toArray(), b.value(QLatin1String("note")).toString());
        QString style = base;
        const int heading = b.value(QLatin1String("heading")).toInt();
        if (heading > 0 && b.value(QLatin1String("style")).toString() == QLatin1String("Title"))
            style += QStringLiteral("font-size:18px; font-weight:700; text-align:center; margin-bottom:14px;");
        else if (heading == 1)
            style += QStringLiteral("font-size:14px; font-weight:700; margin-top:14px;");
        else if (heading > 1)
            style += QStringLiteral("font-size:13px; font-weight:700; margin-top:10px;");
        const QString align = b.value(QLatin1String("align")).toString();
        if (align == QLatin1String("center"))
            style += QStringLiteral("text-align:center;");
        else if (align == QLatin1String("right") || align == QLatin1String("end"))
            style += QStringLiteral("text-align:right;");
        else if (align == QLatin1String("both"))
            style += QStringLiteral("text-align:justify;");
        if (b.value(QLatin1String("list")).toBool())
            style += QStringLiteral("margin-left:18px;");
        return QStringLiteral("<p style=\"%1\">%2</p>").arg(style, inner);
    };
    QHash<QString, QList<QJsonObject>> parts;
    for (const QJsonValue &v : layout)
        parts[v.toObject().value(QLatin1String("part")).toString()].append(v.toObject());

    QString html = QStringLiteral("<html><body style=\"font-family:Calibri,Carlito,'Segoe UI',sans-serif; font-size:13px; color:#111111;\">");
    if (layout.isEmpty()) {
        // No layout (a bilingual file, or none yet): paragraph by paragraph.
        QString block;
        QList<QJsonObject> run;
        const auto flush = [&] {
            if (!run.isEmpty())
                html += QStringLiteral("<p style=\"margin:0 0 9px 0;\">%1</p>").arg(segments(run));
            run.clear();
        };
        for (const QJsonObject &s : all) {
            const QString b = s.value(QLatin1String("block")).toString();
            if (b != block || b.isEmpty())
                flush();
            block = b;
            run.append(s);
        }
        flush();
        return html + QStringLiteral("</body></html>");
    }
    for (const QJsonObject &b : parts.value(QStringLiteral("header")))
        html += paragraph(b, QStringLiteral("margin:0 0 4px 0; color:#666666; font-size:10px; text-align:right;"));
    // The body, cells that follow each other as tables.
    const QList<QJsonObject> body = parts.value(QStringLiteral("body"));
    for (int i = 0; i < body.size();) {
        const QJsonObject b = body.at(i);
        if (!b.contains(QLatin1String("cell"))) {
            html += paragraph(b, QStringLiteral("margin:0 0 9px 0;"));
            ++i;
            continue;
        }
        const int table = b.value(QLatin1String("cell")).toObject().value(QLatin1String("table")).toInt();
        QMap<int, QMap<int, QString>> rows;
        int cols = 0;
        for (; i < body.size() && body.at(i).contains(QLatin1String("cell")) && body.at(i).value(QLatin1String("cell")).toObject().value(QLatin1String("table")).toInt() == table; ++i) {
            const QJsonObject cell = body.at(i).value(QLatin1String("cell")).toObject();
            const int col = cell.value(QLatin1String("col")).toInt();
            rows[cell.value(QLatin1String("row")).toInt()][col] += paragraph(body.at(i), QStringLiteral("margin:0;"));
            cols = qMax(cols, col + 1);
        }
        html += QStringLiteral("<table width=\"100%\" cellspacing=\"0\" cellpadding=\"4\" border=\"1\" style=\"border-collapse:collapse; border-color:#333333; margin-bottom:9px;\">");
        for (auto row = rows.cbegin(); row != rows.cend(); ++row) {
            html += QStringLiteral("<tr>");
            for (int c = 0; c < cols; ++c)
                html += QStringLiteral("<td valign=\"top\">%1</td>").arg(row.value().value(c));
            html += QStringLiteral("</tr>");
        }
        html += QStringLiteral("</table>");
    }
    const QList<QJsonObject> footnotes = parts.value(QStringLiteral("footnote")) + parts.value(QStringLiteral("endnote"));
    if (!footnotes.isEmpty()) {
        html += QStringLiteral("<hr width=\"40%\" align=\"left\">");
        for (const QJsonObject &b : footnotes)
            html += paragraph(b, QStringLiteral("margin:0 0 2px 0; font-size:10px;"));
    }
    for (const QJsonObject &b : parts.value(QStringLiteral("footer")))
        html += paragraph(b, QStringLiteral("margin:4px 0 0 0; font-size:10px; text-align:center;"));
    // What is not on the page itself, each kind under its name.
    const std::pair<const char *, QString> extras[] = {
        {"chart", QCoreApplication::translate("EditorPage", "Charts")},
        {"diagram", QCoreApplication::translate("EditorPage", "SmartArt")},
        {"comment", QCoreApplication::translate("EditorPage", "Comments")},
        {"notes", QCoreApplication::translate("EditorPage", "Speaker notes")},
        {"meta", QCoreApplication::translate("EditorPage", "Title and description of the page")},
        {"attribute", QCoreApplication::translate("EditorPage", "Texts of pictures, links and form fields")},
    };
    for (const auto &[part, title] : extras) {
        const QList<QJsonObject> blocks = parts.value(QLatin1String(part));
        if (blocks.isEmpty())
            continue;
        html += QStringLiteral("<hr><p style=\"margin:8px 0 4px 0; color:#6b7280; font-size:11px; font-weight:600;\">%1</p>").arg(title.toHtmlEscaped());
        for (const QJsonObject &b : blocks)
            html += paragraph(b, QStringLiteral("margin:0 0 4px 0; font-size:12px;"));
    }
    return html + QStringLiteral("</body></html>");
}

