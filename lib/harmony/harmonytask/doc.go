/*
Package harmonytask implements a pure (no task logic), distributed
task manager. This clean interface allows a task implementer to completely
avoid being concerned with task scheduling and management.
It's based on the idea of tasks as small units of work broken from other
work by hardware, parallelizabilty, reliability, or any other reason.
Workers will be Greedy: vaccuuming up their favorite jobs from a list.
Once 1 task is accepted, harmonydb tries to get other task runner
machines to accept work (round robin) before trying again to accept.
*
Mental Model:

	Things that block tasks:
		- task not registered for any running server
		- max was specified and reached
		- resource exhaustion
		- CanAccept() interface (per-task implmentation) does not accept it.
	Ways tasks start: (slowest first)
		- DB Read every 1 minute
		- Bump via HTTP if registered in DB
		- Task was added (to db) by this process
	Ways tasks get added:
	    - Async Listener task (for chain, etc)
		- Followers: Tasks get added because another task completed
	When Follower collectors run:
	    - If both sides are process-local, then
		- Otherwise, at the listen interval during db scrape
	How duplicate tasks are avoided:
	    - that's up to the task definition, but probably a unique key

*
To use:
1.Implement TaskInterface for a new task.
2 Have New() receive this & all other ACTIVE implementations.
*
*
As we are not expecting DBAs in this database, it's important to know
what grows uncontrolled. The only harmony_* table is _task_history
(somewhat quickly) and harmony_machines (slowly). These will need a
clean-up for after the task data could never be acted upon.
but the design **requires** extraInfo tables to grow until the task's
info could not possibly be used by a following task, including slow
release rollout. This would normally be in the order of months old.
*
Other possible enhancements include more collaborative coordination
to assign a task to machines closer to the data.

__Database_Behavior__
harmony_task is the list of work that has not been completed.

	AddTaskFunc manages the additions, but is designed to have its
	transactions failed-out on overlap with a similar task already written.
	It's up to the TaskInterface implementer to discover this overlap via
	some other table it uses (since overlap can mean very different things).

harmony_task_history

	This holds transactions that completed or saw too many retries. It also
	serves as input for subsequent (follower) tasks to kick off. This is not
	done machine-internally because a follower may not be on the same machine
	as the previous task.

harmony_task_machines

	Managed by lib/harmony/resources, this is a reference to machines registered
	via the resources. This registration does not obligate the machine to
	anything, but serves as a discovery mechanism. Paths are hostnames + ports
	which are presumed to support http, but this assumption is only used by
	the task system.

harmony_task_follow / harmony_task_impl

	These tables are used to fast-path notifications to other machines instead
	of waiting for polling. _impl helps round-robin work pick-up. _follow helps
	discover the machines that are interested in creating tasks following the
	task that just completed.
*/
package harmonytask
