create table wdpost_tasks
(
    task_id                  int  not null
        constraint wdpost_tasks_pkey
            primary key,
    tskey                    bytea not null,
    current_epoch            bigint  not null,
    period_start             bigint  not null,
    index                    bigint  not null
        constraint wdpost_tasks_index_key
            unique,
    open                     bigint  not null,
    close                    bigint  not null,
    challenge                bigint  not null,
    fault_cutoff             bigint,
    wpost_period_deadlines   bigint,
    wpost_proving_period     bigint,
    wpost_challenge_window   bigint,
    wpost_challenge_lookback bigint,
    fault_declaration_cutoff bigint
);


