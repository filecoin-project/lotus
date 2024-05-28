create table mining_tasks
(
    task_id bigint not null
        constraint mining_tasks_pk
            primary key,
    sp_id   bigint not null,
    epoch   bigint not null,
    base_compute_time timestamp not null,

    won bool not null default false,
    mined_cid text,
    mined_header jsonb,
    mined_at timestamp,

    submitted_at timestamp,

    constraint mining_tasks_sp_epoch
        unique (sp_id, epoch)
);

create table mining_base_block
(
    id        bigserial not null
        constraint mining_base_block_pk
            primary key,
    task_id   bigint    not null
        constraint mining_base_block_mining_tasks_task_id_fk
            references mining_tasks
            on delete cascade,
    sp_id     bigint,
    block_cid text      not null,

    no_win    bool      not null default false,

    constraint mining_base_block_pk2
        unique (sp_id, task_id, block_cid)
);

CREATE UNIQUE INDEX mining_base_block_cid_k ON mining_base_block (sp_id, block_cid) WHERE no_win = false;
