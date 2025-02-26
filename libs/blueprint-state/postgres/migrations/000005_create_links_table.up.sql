CREATE TABLE IF NOT EXISTS links (
    id uuid PRIMARY KEY,
    "status" smallint NOT NULL,
    precise_status smallint NOT NULL,
    last_status_update_timestamp timestamp,
    last_deployed_timestamp timestamp,
    last_deploy_attempt_timestamp timestamp,
    intermediary_resources_state jsonb NOT NULL,
    data jsonb NOT NULL,
    failure_reasons jsonb NOT NULL,
    durations jsonb
);
