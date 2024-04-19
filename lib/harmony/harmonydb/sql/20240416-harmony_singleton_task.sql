create table harmony_task_singletons (
    task_name varchar(255) not null,
    task_id bigint,
    last_run_time timestamp,

    primary key (task_name),
    foreign key (task_id) references harmony_task (id) on delete set null
);
