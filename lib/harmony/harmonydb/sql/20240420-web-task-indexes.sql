DROP INDEX harmony_task_history_work_index;

/*
 This structure improves clusterMachineSummary query better than the old version,
 while at the same time also being usable by clusterTaskHistorySummary (which wasn't
 the case with the old index).
 */
create index harmony_task_history_work_index
    on harmony_task_history (work_end desc, completed_by_host_and_port asc, name asc, result asc);
