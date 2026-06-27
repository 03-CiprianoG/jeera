-- +goose Up
-- A SCRUM board scopes to "the" active sprint, so a project may have at most one
-- active sprint at a time. The original TUI state-cycle never enforced this, so
-- first collapse any pre-existing multiple-active sprints to a single active per
-- project — keep the newest by id, complete the rest — before building the unique
-- index, which would otherwise fail on such a database.
UPDATE sprints SET state = 'completed'
 WHERE state = 'active'
   AND id NOT IN (SELECT MAX(id) FROM sprints WHERE state = 'active' GROUP BY project_id);

CREATE UNIQUE INDEX idx_sprints_one_active ON sprints(project_id) WHERE state = 'active';

-- +goose Down
DROP INDEX idx_sprints_one_active;
