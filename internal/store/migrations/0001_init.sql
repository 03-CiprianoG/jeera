-- +goose Up

CREATE TABLE projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key_prefix  TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    repo_path   TEXT    NOT NULL,
    defaults    TEXT    NOT NULL DEFAULT '{}',
    seq_counter INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL
);

CREATE TABLE statuses (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    category   TEXT    NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_statuses_project ON statuses(project_id);

CREATE TABLE sprints (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    goal       TEXT    NOT NULL DEFAULT '',
    state      TEXT    NOT NULL DEFAULT 'future',
    start_at   TEXT,
    end_at     TEXT
);
CREATE INDEX idx_sprints_project ON sprints(project_id);

CREATE TABLE tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    color      TEXT    NOT NULL DEFAULT '',
    UNIQUE(project_id, name)
);

CREATE TABLE issues (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id        INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq               INTEGER NOT NULL,
    type              TEXT    NOT NULL,
    title             TEXT    NOT NULL,
    description       TEXT    NOT NULL DEFAULT '',
    status_id         INTEGER NOT NULL REFERENCES statuses(id),
    priority          TEXT    NOT NULL DEFAULT 'medium',
    story_points      INTEGER,
    assignee_provider TEXT    NOT NULL DEFAULT '',
    assignee_model    TEXT    NOT NULL DEFAULT '',
    assignee_effort   TEXT    NOT NULL DEFAULT '',
    epic_id           INTEGER REFERENCES issues(id) ON DELETE SET NULL,
    sprint_id         INTEGER REFERENCES sprints(id) ON DELETE SET NULL,
    parent_id         INTEGER REFERENCES issues(id) ON DELETE SET NULL,
    rank              TEXT    NOT NULL DEFAULT '',
    worktree_on       INTEGER,
    settings          TEXT    NOT NULL DEFAULT '{}',
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL,
    UNIQUE(project_id, seq)
);
CREATE INDEX idx_issues_project ON issues(project_id);
CREATE INDEX idx_issues_status  ON issues(status_id);
CREATE INDEX idx_issues_sprint  ON issues(sprint_id);
CREATE INDEX idx_issues_epic    ON issues(epic_id);

CREATE TABLE issue_tags (
    issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    tag_id   INTEGER NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
    PRIMARY KEY (issue_id, tag_id)
);

CREATE TABLE issue_links (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_id  INTEGER NOT NULL REFERENCES issues(id)   ON DELETE CASCADE,
    target_id  INTEGER NOT NULL REFERENCES issues(id)   ON DELETE CASCADE,
    type       TEXT    NOT NULL,
    UNIQUE(source_id, target_id, type)
);
CREATE INDEX idx_links_source ON issue_links(source_id);
CREATE INDEX idx_links_target ON issue_links(target_id);

CREATE TABLE comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author     TEXT    NOT NULL DEFAULT 'human',
    body       TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_comments_issue ON comments(issue_id);

CREATE TABLE attachments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    filename   TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    mime       TEXT    NOT NULL DEFAULT '',
    size       INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_attachments_issue ON attachments(issue_id);

CREATE TABLE runs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id        INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL DEFAULT 1,
    parent_run_id   INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    provider        TEXT    NOT NULL,
    model           TEXT    NOT NULL DEFAULT '',
    effort          TEXT    NOT NULL DEFAULT '',
    session_id      TEXT    NOT NULL DEFAULT '',
    worktree_path   TEXT    NOT NULL DEFAULT '',
    branch          TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'queued',
    permission_mode TEXT    NOT NULL DEFAULT '',
    started_at      TEXT,
    ended_at        TEXT,
    exit_code       INTEGER,
    log_path        TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL
);
CREATE INDEX idx_runs_issue ON runs(issue_id);

CREATE TABLE schedules (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id      INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    cron_spec     TEXT    NOT NULL,
    with_children INTEGER NOT NULL DEFAULT 0,
    enabled       INTEGER NOT NULL DEFAULT 1,
    job_uuid      TEXT    NOT NULL DEFAULT '',
    next_run      TEXT
);
CREATE INDEX idx_schedules_issue ON schedules(issue_id);

CREATE TABLE settings (
    scope    TEXT    NOT NULL,
    scope_id INTEGER NOT NULL DEFAULT 0,
    key      TEXT    NOT NULL,
    value    TEXT    NOT NULL,
    PRIMARY KEY (scope, scope_id, key)
);

-- +goose Down
DROP TABLE settings;
DROP TABLE schedules;
DROP TABLE runs;
DROP TABLE attachments;
DROP TABLE comments;
DROP TABLE issue_links;
DROP TABLE issue_tags;
DROP TABLE issues;
DROP TABLE tags;
DROP TABLE sprints;
DROP TABLE statuses;
DROP TABLE projects;
