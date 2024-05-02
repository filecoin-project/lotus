ALTER TABLE sectors_sdr_pipeline
    ADD COLUMN sched_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE parked_pieces
    ADD COLUMN sched_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE parked_pieces
    ADD COLUMN sched_cleanup_count BIGINT NOT NULL DEFAULT 0;

