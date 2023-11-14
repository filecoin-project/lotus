create table wdpost_partition_tasks
(
    task_id              bigint not null
        constraint wdpost_partition_tasks_pk
            primary key,
    sp_id                bigint not null,
    proving_period_start bigint not null,
    deadline_index       bigint not null,
    partition_index      bigint not null,
    constraint wdpost_partition_tasks_identity_key
        unique (sp_id, proving_period_start, deadline_index, partition_index)
);

comment on column wdpost_partition_tasks.task_id is 'harmonytask task ID';
comment on column wdpost_partition_tasks.sp_id is 'storage provider ID';
comment on column wdpost_partition_tasks.proving_period_start is 'proving period start';
comment on column wdpost_partition_tasks.deadline_index is 'deadline index within the proving period';
comment on column wdpost_partition_tasks.partition_index is 'partition index within the deadline';

create table wdpost_proofs
(
    sp_id                bigint not null,
    proving_period_start bigint not null,
    deadline             bigint not null,
    partition            bigint not null,
    submit_at_epoch      bigint not null,
    submit_by_epoch      bigint not null,
    proof_params         bytea,

    submit_task_id       bigint,
    message_cid          text,

    constraint wdpost_proofs_identity_key
        unique (sp_id, proving_period_start, deadline, partition)
);

create table wdpost_recovery_tasks
(
    task_id              bigint not null
        constraint wdpost_recovery_tasks_pk
            primary key,
    sp_id                bigint not null,
    proving_period_start bigint not null,
    deadline_index       bigint not null,
    partition_index      bigint not null,
    constraint wdpost_recovery_tasks_identity_key
        unique (sp_id, proving_period_start, deadline_index, partition_index)
);