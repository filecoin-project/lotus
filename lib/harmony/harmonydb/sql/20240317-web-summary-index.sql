/* Used for webui clusterMachineSummary */
CREATE INDEX harmony_task_history_work_index
	ON harmony_task_history (completed_by_host_and_port ASC, name ASC, result ASC, work_end DESC);

/* Used for webui actorSummary sp wins */
create index mining_tasks_won_sp_id_base_compute_time_index
    on mining_tasks (won asc, sp_id asc, base_compute_time desc);
