-- Convert timestamps in harmony_machines table
ALTER TABLE harmony_machines
ALTER COLUMN last_contact TYPE TIMESTAMPTZ
    USING last_contact AT TIME ZONE 'UTC';

-- Convert timestamps in harmony_task table
ALTER TABLE harmony_task
ALTER COLUMN update_time TYPE TIMESTAMPTZ
    USING update_time AT TIME ZONE 'UTC';

ALTER TABLE harmony_task
ALTER COLUMN posted_time TYPE TIMESTAMPTZ
    USING posted_time AT TIME ZONE 'UTC';

-- Convert timestamps in harmony_task_history table
ALTER TABLE harmony_task_history
ALTER COLUMN posted TYPE TIMESTAMPTZ
    USING posted AT TIME ZONE 'UTC';

ALTER TABLE harmony_task_history
ALTER COLUMN work_start TYPE TIMESTAMPTZ
    USING work_start AT TIME ZONE 'UTC';

ALTER TABLE harmony_task_history
ALTER COLUMN work_end TYPE TIMESTAMPTZ
    USING work_end AT TIME ZONE 'UTC';

-- Convert timestamps in sector_location table
ALTER TABLE sector_location
ALTER COLUMN read_ts TYPE TIMESTAMPTZ
    USING read_ts AT TIME ZONE 'UTC';

ALTER TABLE sector_location
ALTER COLUMN write_ts TYPE TIMESTAMPTZ
    USING write_ts AT TIME ZONE 'UTC';

-- Convert timestamps in storage_path table
ALTER TABLE storage_path
ALTER COLUMN last_heartbeat TYPE TIMESTAMPTZ
    USING last_heartbeat AT TIME ZONE 'UTC';

-- Convert timestamps in mining_tasks table
ALTER TABLE mining_tasks
ALTER COLUMN base_compute_time TYPE TIMESTAMPTZ
    USING base_compute_time AT TIME ZONE 'UTC';

ALTER TABLE mining_tasks
ALTER COLUMN mined_at TYPE TIMESTAMPTZ
    USING mined_at AT TIME ZONE 'UTC';

ALTER TABLE mining_tasks
ALTER COLUMN submitted_at TYPE TIMESTAMPTZ
    USING submitted_at AT TIME ZONE 'UTC';

-- Convert timestamps in itest_scratch table
ALTER TABLE itest_scratch
ALTER COLUMN update_time TYPE TIMESTAMPTZ
    USING update_time AT TIME ZONE 'UTC';

-- Convert timestamps in message_sends table
ALTER TABLE message_sends
ALTER COLUMN send_time TYPE TIMESTAMPTZ
    USING send_time AT TIME ZONE 'UTC';

-- Convert timestamps in message_send_locks table
ALTER TABLE message_send_locks
ALTER COLUMN claimed_at TYPE TIMESTAMPTZ
    USING claimed_at AT TIME ZONE 'UTC';

-- Convert timestamps in harmony_task_singletons table
ALTER TABLE harmony_task_singletons
ALTER COLUMN last_run_time TYPE TIMESTAMPTZ
    USING last_run_time AT TIME ZONE 'UTC';

-- Convert timestamps in sector_path_url_liveness table
ALTER TABLE sector_path_url_liveness
ALTER COLUMN last_checked TYPE TIMESTAMPTZ
    USING last_checked AT TIME ZONE 'UTC';

ALTER TABLE sector_path_url_liveness
ALTER COLUMN last_live TYPE TIMESTAMPTZ
    USING last_live AT TIME ZONE 'UTC';

ALTER TABLE sector_path_url_liveness
ALTER COLUMN last_dead TYPE TIMESTAMPTZ
    USING last_dead AT TIME ZONE 'UTC';

-- Convert timestamps in harmony_machine_details table
ALTER TABLE harmony_machine_details
ALTER COLUMN startup_time TYPE TIMESTAMPTZ
    USING startup_time AT TIME ZONE 'UTC';

-- Convert timestamps in open_sector_pieces table
ALTER TABLE open_sector_pieces
ALTER COLUMN created_at TYPE TIMESTAMPTZ
    USING created_at AT TIME ZONE 'UTC';

-- Convert timestamps in parked_pieces table
ALTER TABLE parked_pieces
ALTER COLUMN created_at TYPE TIMESTAMPTZ
    USING created_at AT TIME ZONE 'UTC';

-- Convert timestamps in sectors_sdr_pipeline table
ALTER TABLE sectors_sdr_pipeline
ALTER COLUMN create_time TYPE TIMESTAMPTZ
    USING create_time AT TIME ZONE 'UTC';

ALTER TABLE sectors_sdr_pipeline
ALTER COLUMN failed_at TYPE TIMESTAMPTZ
    USING failed_at AT TIME ZONE 'UTC';

-- Convert timestamps in sectors_sdr_initial_pieces table
ALTER TABLE sectors_sdr_initial_pieces
ALTER COLUMN created_at TYPE TIMESTAMPTZ
    USING created_at AT TIME ZONE 'UTC';
