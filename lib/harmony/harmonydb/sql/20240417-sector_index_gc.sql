create table sector_path_url_liveness (
    storage_id text,
    url text,

    last_checked timestamp not null,
    last_live timestamp,
    last_dead timestamp,
    last_dead_reason text,

    primary key (storage_id, url),

    foreign key (storage_id) references storage_path (storage_id) on delete cascade
)
