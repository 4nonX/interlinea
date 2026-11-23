#include "projectspage.h"

#include "connectionmanager.h"
#include "newprojectdialog.h"
#include "projectpage.h"
#include "progressdelegate.h"
#include "projectsmodel.h"

#include "theme.h"

#include <QHBoxLayout>
#include <QHeaderView>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QSortFilterProxyModel>
#include <QStackedWidget>
#include <QTableView>
#include <QVBoxLayout>

ProjectsPage::ProjectsPage(ConnectionManager *manager, QWidget *parent)
    : QWidget(parent), m_manager(manager), m_model(new ProjectsModel(manager, this)), m_proxy(new QSortFilterProxyModel(this)),
      m_stack(new QStackedWidget(this))
{
    setObjectName(QStringLiteral("projects"));
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->addWidget(m_stack);

    auto *list = new QWidget(m_stack);
    auto *l = new QVBoxLayout(list);
    l->setContentsMargins(28, 22, 28, 20);
    l->setSpacing(12);
    auto *head = new QHBoxLayout;
    auto *title = new QLabel(tr("Projects"), list);
    Theme::title(title);
    head->addWidget(title, 1);
    m_filter = new QLineEdit(list);
    m_filter->setPlaceholderText(tr("Find a project"));
    m_filter->setAccessibleName(tr("Find a project"));
    m_filter->setClearButtonEnabled(true);
    m_filter->setMinimumWidth(240);
    head->addWidget(m_filter);
    auto *add = new QPushButton(tr("&New project…"), list);
    Theme::primary(add);
    m_open = new QPushButton(tr("&Open"), list);
    head->addWidget(add);
    head->addWidget(m_open);
    l->addLayout(head);
    m_empty = new QLabel(list);
    m_empty->setWordWrap(true);
    Theme::secondary(m_empty);
    l->addWidget(m_empty);
    m_proxy->setSourceModel(m_model);
    m_proxy->setSortRole(ProjectsModel::SortRole);
    m_proxy->setFilterKeyColumn(-1);
    m_proxy->setFilterCaseSensitivity(Qt::CaseInsensitive);
    m_table = new QTableView(list);
    m_table->setObjectName(QStringLiteral("projectList"));
    m_table->setAccessibleName(tr("Projects"));
    m_table->setModel(m_proxy);
    m_table->setSortingEnabled(true);
    m_table->sortByColumn(ProjectsModel::Due, Qt::AscendingOrder);
    m_table->setSelectionBehavior(QAbstractItemView::SelectRows);
    m_table->setSelectionMode(QAbstractItemView::SingleSelection);
    m_table->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_table->verticalHeader()->setVisible(false);
    m_table->verticalHeader()->setDefaultSectionSize(36);
    m_table->setShowGrid(false);
    m_table->setFrameShape(QFrame::NoFrame);
    m_table->horizontalHeader()->setSectionResizeMode(QHeaderView::ResizeToContents);
    m_table->horizontalHeader()->setSectionResizeMode(ProjectsModel::Name, QHeaderView::Stretch);
    m_table->horizontalHeader()->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
    m_table->horizontalHeader()->setHighlightSections(false);
    m_table->setTabKeyNavigation(false);
    m_table->setItemDelegateForColumn(ProjectsModel::Progress, new ProgressDelegate(m_table));
    l->addWidget(m_table, 1);
    m_stack->addWidget(list);

    connect(m_filter, &QLineEdit::textChanged, m_proxy, &QSortFilterProxyModel::setFilterFixedString);
    connect(add, &QPushButton::clicked, this, [this] { newProject(); });
    const auto openCurrent = [this] {
        const QModelIndex i = m_table->currentIndex();
        if (i.isValid())
            open(i.data(ProjectsModel::ConnectionRole).toString(), i.data(ProjectsModel::IdRole).toLongLong());
    };
    connect(m_open, &QPushButton::clicked, this, openCurrent);
    connect(m_table, &QTableView::activated, this, openCurrent);
    connect(m_model, &ProjectsModel::loaded, this, &ProjectsPage::updateState);
    connect(m_table->selectionModel(), &QItemSelectionModel::currentChanged, this, &ProjectsPage::updateState);
    updateState();
}

void ProjectsPage::updateState()
{
    m_open->setEnabled(m_table->currentIndex().isValid());
    QStringList notes;
    for (Connection *c : m_manager->all()) {
        const QString e = m_model->error(c->id());
        if (!e.isEmpty())
            notes << tr("%1: %2").arg(c->displayName(), e);
    }
    if (m_model->rowCount() == 0 && notes.isEmpty())
        notes << tr("No projects yet. Make one from the documents to translate with New project.");
    m_empty->setText(notes.join(QLatin1Char('\n')));
    m_empty->setVisible(!notes.isEmpty());
}

void ProjectsPage::open(const QString &connectionId, qint64 projectId)
{
    Connection *c = m_manager->find(connectionId);
    if (!c)
        return;
    if (m_project) {
        m_stack->removeWidget(m_project);
        m_project->deleteLater();
    }
    m_project = new ProjectPage(c, projectId, m_stack);
    m_stack->addWidget(m_project);
    m_stack->setCurrentWidget(m_project);
    connect(m_project, &ProjectPage::back, this, &ProjectsPage::showList);
    connect(m_project, &ProjectPage::gone, this, [this] {
        showList();
        if (Connection *conn = m_project ? m_project->connection() : nullptr)
            m_model->reload(conn);
    });
    m_project->setFocus();
}

void ProjectsPage::showList()
{
    m_stack->setCurrentIndex(0);
    m_table->setFocus();
}

void ProjectsPage::newProject(const QStringList &files)
{
    NewProjectDialog dlg(m_manager, this);
    if (!files.isEmpty())
        dlg.addFiles(files);
    if (dlg.exec() != QDialog::Accepted)
        return;
    if (Connection *c = m_manager->find(dlg.connectionId()))
        m_model->reload(c);
    open(dlg.connectionId(), dlg.projectId());
}
