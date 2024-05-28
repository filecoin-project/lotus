-- Harmony counts failed tasks by task_id, without this index we'd do a full scan on the history table.
CREATE INDEX harmony_task_history_task_id_result_index
    ON harmony_task_history (task_id, result);

