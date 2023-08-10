create table SectorLocation
(
    "miner_id"        bigint,
    "sector_num"   bigint,
    "sector_filetype" int,
    "storage_id"      varchar,
    "is_primary"      bool,
    constraint SectorLocation_pk
        primary key ("miner_id", "sector_num", "sector_filetype", "storage_id")

    -- TODO: Maybe add index on above PK fields
);


create table StorageLocation
(
    "storage_id"  varchar not null
        constraint "StorageLocation_pkey"
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

