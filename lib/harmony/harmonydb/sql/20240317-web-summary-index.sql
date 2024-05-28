/* Used for webui clusterMachineSummary */
-- NOTE: This index is changed in 20240420-web-task-indexes.sql
CREATE INDEX harmony_task_history_work_index
	ON harmony_task_history (completed_by_host_and_port ASC, name ASC, result ASC, work_end DESC);

/* Used for webui actorSummary sp wins */
CREATE INDEX mining_tasks_won_sp_id_base_compute_time_index
    ON mining_tasks (won ASC, sp_id ASC, base_compute_time DESC);
