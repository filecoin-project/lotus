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
    deadline           bigint  not null,
    partitions         bytea not null,
    proof_type         bigint,
    proof_bytes        bytea,
    chain_commit_epoch bigint,
    chain_commit_rand  bytea
);



