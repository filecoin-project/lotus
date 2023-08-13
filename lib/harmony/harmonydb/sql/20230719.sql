/* For HarmonyTask base implementation. */
CREATE TABLE harmony_task (
    id SERIAL PRIMARY KEY NOT NULL,
    initiated_by INTEGER,     
    update_time TIMESTAMP NOT NULL DEFAULT current_timestamp,
    posted_time TIMESTAMP NOT NULL,
    owner_id INTEGER, 
    added_by INTEGER NOT NULL,
    previous_task INTEGER,
    name varchar(8) NOT NULL
);
COMMENT ON COLUMN harmony_task.initiated_by IS 'The task ID whose completion occasioned this task.';
COMMENT ON COLUMN harmony_task.owner_id IS 'The foreign key to harmony_machines.';
COMMENT ON COLUMN harmony_task.name IS 'The name of the task type.';
COMMENT ON COLUMN harmony_task.owner_id IS 'may be null if between owners or not yet taken';

CREATE TABLE harmony_machines (
    id SERIAL PRIMARY KEY NOT NULL,
    last_contact TIMESTAMP NOT NULL DEFAULT current_timestamp,
    host_and_port varchar(300) NOT NULL, 
    cpu INTEGER NOT NULL, 
    ram BIGINT NOT NULL, 
    gpu FLOAT NOT NULL, 
    gpuram BIGINT NOT NULL
);

CREATE TABLE harmony_task_history (
    id SERIAL PRIMARY KEY NOT NULL,  
    task_id INTEGER NOT NULL, 
    name VARCHAR(8) NOT NULL,
    posted TIMESTAMP NOT NULL, 
    work_start TIMESTAMP NOT NULL, 
    work_end TIMESTAMP NOT NULL, 
    result BOOLEAN NOT NULL, 
    err varchar
);
COMMENT ON COLUMN harmony_task_history.result IS 'Use to detemine if this was a successful run.';

CREATE TABLE harmony_task_follow (
    id SERIAL PRIMARY KEY NOT NULL,  
    owner_id INTEGER NOT NULL,
    to_type VARCHAR(8) NOT NULL,
    from_type VARCHAR(8) NOT NULL
);

CREATE TABLE harmony_task_impl (
    id SERIAL PRIMARY KEY NOT NULL,  
    owner_id INTEGER NOT NULL,
    name VARCHAR(8) NOT NULL
);