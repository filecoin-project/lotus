create table wdpost_tasks
(
    task_id                  int not null,
    tskey                    varchar,
    current_epoch            bigint,
    period_start             bigint,
    index                    bigint,
    open                     bigint,
    close                    bigint,
    challenge                bigint,
    fault_cutoff             bigint,
    wpost_period_deadlines   bigint,
    wpost_proving_period     bigint,
    wpost_challenge_window   bigint,
    wpost_challenge_lookback bigint,
    fault_declaration_cutoff bigint
);

