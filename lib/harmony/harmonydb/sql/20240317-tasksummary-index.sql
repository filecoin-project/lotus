/* Used for webui clusterMachineSummary */
CREATE INDEX harmony_task_history_work_index
	ON harmony_task_history (completed_by_host_and_port ASC, name ASC, result ASC, work_end DESC);
