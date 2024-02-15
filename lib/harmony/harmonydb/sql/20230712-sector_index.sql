create table sector_location
(
    miner_id         bigint    not null,
    sector_num       bigint    not null,
    sector_filetype  int   not null,
    storage_id       varchar not null,
    is_primary       bool,
    read_ts          timestamp(6),
    read_refs        int,
    write_ts         timestamp(6),
    write_lock_owner varchar,
    constraint sectorlocation_pk
        primary key (miner_id, sector_num, sector_filetype, storage_id)
);

alter table sector_location
    alter column read_refs set not null;

alter table sector_location
    alter column read_refs set default 0;

create table storage_path
(
    "storage_id"  varchar not null
        constraint "storage_path_pkey"
            primary key,
    "urls"       varchar, -- comma separated list of urls
    "weight"     bigint,
    "max_storage" bigint,
    "can_seal"    bool,
    "can_store"   bool,
    "groups"     varchar, -- comma separated list of group names
    "allow_to"    varchar, -- comma separated list of allowed groups
    "allow_types" varchar,  -- comma separated list of allowed file types
    "deny_types"  varchar, -- comma separated list of denied file types

    "capacity" bigint,
    "available" bigint,
    "fs_available" bigint,
    "reserved" bigint,
    "used" bigint,
    "last_heartbeat" timestamp(6),
    "heartbeat_err"  varchar
);

