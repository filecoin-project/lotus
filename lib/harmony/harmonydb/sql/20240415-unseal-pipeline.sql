create table sectors_unseal_pipeline (
    sp_id bigint not null,
    sector_number bigint not null,

    create_time timestamp not null default current_timestamp,

    task_id_unseal_sdr bigint, -- builds unseal cache
    after_unseal_sdr bool not null default false,

    task_id_decode_sector bigint, -- makes the "unsealed" copy (runs either next to unseal cache OR in long-term storage)
    after_decode_sector bool not null default false,

    task_id_move_storage bigint, -- optional, moves the unsealed sector to storage
    after_move_storage bool not null default false,

    -- note: those foreign keys are a part of the retry mechanism. If a task
    -- fails due to retry limit, it will drop the assigned task_id, and the
    -- poller will reassign the task to a new node if it deems the task is
    -- still valid to be retried.
    foreign key (task_id_unseal_sdr) references harmony_task (id) on delete set null,
    foreign key (task_id_decode_sector) references harmony_task (id) on delete set null,
    foreign key (task_id_move_storage) references harmony_task (id) on delete set null,

    primary key (sp_id, sector_number)
);
