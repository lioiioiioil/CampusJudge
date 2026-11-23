CREATE TABLE campusjudge.metadata (
    singleton boolean PRIMARY KEY CHECK (singleton),
    schema_version integer NOT NULL,
    state_version integer NOT NULL,
    demo boolean NOT NULL
);
INSERT INTO campusjudge.metadata VALUES (true, 1, 1, false);

CREATE TABLE campusjudge.groups (
    id text PRIMARY KEY, position integer NOT NULL,
    name text NOT NULL, invite_code text UNIQUE DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE campusjudge.users (
    id text PRIMARY KEY, position integer NOT NULL,
    name text NOT NULL, username text NOT NULL UNIQUE DEFERRABLE INITIALLY DEFERRED,
    role text NOT NULL, group_id text REFERENCES campusjudge.groups(id) DEFERRABLE INITIALLY DEFERRED,
    password_hash text NOT NULL
);
CREATE TABLE campusjudge.topics (
    id text PRIMARY KEY, position integer NOT NULL,
    title text NOT NULL, description text NOT NULL, lesson_url text
);
CREATE TABLE campusjudge.problems (
    id text PRIMARY KEY, position integer NOT NULL,
    title text NOT NULL, statement text NOT NULL,
    input_format text NOT NULL, output_format text NOT NULL,
    time_limit_ms integer NOT NULL, memory_limit_mb integer NOT NULL,
    samples jsonb, tests jsonb,
    test_revisions_set boolean NOT NULL
);
CREATE TABLE campusjudge.test_revisions (
    problem_id text REFERENCES campusjudge.problems(id) DEFERRABLE INITIALLY DEFERRED,
    number integer NOT NULL CHECK (number > 0), position integer NOT NULL,
    samples jsonb, tests jsonb, created_at timestamptz NOT NULL,
    PRIMARY KEY (problem_id, number)
);
CREATE TABLE campusjudge.works (
    id text PRIMARY KEY, position integer NOT NULL,
    title text NOT NULL, description text NOT NULL, kind text NOT NULL,
    topic_id text REFERENCES campusjudge.topics(id) DEFERRABLE INITIALLY DEFERRED,
    status text NOT NULL, starts_at timestamptz, ends_at timestamptz,
    created_at timestamptz NOT NULL,
    group_ids_set boolean NOT NULL, problem_ids_set boolean NOT NULL,
    problem_revisions jsonb
);
CREATE TABLE campusjudge.work_groups (
    work_id text REFERENCES campusjudge.works(id) DEFERRABLE INITIALLY DEFERRED,
    position integer NOT NULL,
    group_id text NOT NULL REFERENCES campusjudge.groups(id) DEFERRABLE INITIALLY DEFERRED,
    PRIMARY KEY (work_id, position), UNIQUE (work_id, group_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE campusjudge.work_problems (
    work_id text REFERENCES campusjudge.works(id) DEFERRABLE INITIALLY DEFERRED,
    position integer NOT NULL,
    problem_id text NOT NULL REFERENCES campusjudge.problems(id) DEFERRABLE INITIALLY DEFERRED,
    PRIMARY KEY (work_id, position), UNIQUE (work_id, problem_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE campusjudge.submissions (
    id text PRIMARY KEY, position integer NOT NULL,
    work_id text NOT NULL REFERENCES campusjudge.works(id) DEFERRABLE INITIALLY DEFERRED,
    problem_id text NOT NULL REFERENCES campusjudge.problems(id) DEFERRABLE INITIALLY DEFERRED,
    user_id text NOT NULL REFERENCES campusjudge.users(id) DEFERRABLE INITIALLY DEFERRED,
    language text NOT NULL, source text NOT NULL,
    verdict text NOT NULL, feedback text NOT NULL,
    created_at timestamptz NOT NULL, judged_at timestamptz,
    reviewer_id text REFERENCES campusjudge.users(id) DEFERRABLE INITIALLY DEFERRED,
    judging jsonb, test_revision integer NOT NULL, past_judgments jsonb
);
CREATE INDEX submissions_work_user ON campusjudge.submissions(work_id, user_id);
CREATE INDEX submissions_pending ON campusjudge.submissions ((judging->>'status'))
    WHERE judging->>'status' IN ('queued', 'running');
CREATE TABLE campusjudge.sessions (
    token_hash text PRIMARY KEY, position integer NOT NULL,
    user_id text NOT NULL REFERENCES campusjudge.users(id) DEFERRABLE INITIALLY DEFERRED,
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user ON campusjudge.sessions(user_id);
