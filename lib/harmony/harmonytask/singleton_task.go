package harmonytask

import (
	"errors"
	"time"

	"github.com/yugabyte/pgx/v5"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/passcall"
)

func SingletonTaskAdder(minInterval time.Duration, task TaskInterface) func(AddTaskFunc) error {
	return passcall.Every(minInterval, func(add AddTaskFunc) error {
		taskName := task.TypeDetails().Name

		add(func(taskID TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
			var existingTaskID *int64
			var lastRunTime time.Time

			// Query to check the existing task entry
			err = tx.QueryRow(`SELECT task_id, last_run_time FROM harmony_task_singletons WHERE task_name = $1`, taskName).Scan(&existingTaskID, &lastRunTime)
			if err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					return false, err // return error if query failed and it's not because of missing row
				}
			}

			now := time.Now().UTC()
			// Determine if the task should run based on the absence of a record or outdated last_run_time
			shouldRun := err == pgx.ErrNoRows || (existingTaskID == nil && lastRunTime.Add(minInterval).Before(now))
			if !shouldRun {
				return false, nil
			}

			// Conditionally insert or update the task entry
			n, err := tx.Exec(`
                INSERT INTO harmony_task_singletons (task_name, task_id, last_run_time)
                VALUES ($1, $2, $3)
                ON CONFLICT (task_name) DO UPDATE
                SET task_id = COALESCE(harmony_task_singletons.task_id, $2),
                    last_run_time = $3
                WHERE harmony_task_singletons.task_id IS NULL
            `, taskName, taskID, now)
			if err != nil {
				return false, err
			}
			return n > 0, nil
		})
		return nil
	})
}
