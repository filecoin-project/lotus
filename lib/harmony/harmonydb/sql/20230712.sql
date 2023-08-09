create table SectorLocation
(
    "miner_id"        int8,
    "sector_num"   int8,
    "sector_filetype" int,
    "storage_id"      varchar,
    "is_primary"      bool,
    constraint SectorLocation_pk
        primary key ("miner_id", "sector_num", "sector_filetype", "storage_id")
);


create table StorageLocation
(
    "storage_id"  varchar not null
        constraint "StorageLocation_pkey"
            primary key,
    "urls"       varchar,
    "weight"     int8,
    "max_storage" int8,
    "can_seal"    bool,
    "can_store"   bool,
    "groups"     varchar,
    "allow_to"    varchar,
    "allow_types" varchar,
    "deny_types"  varchar,

    "capacity" int8,
    "available" int8,
    "fs_available" int8,
    "reserved" int8,
--     "MaxStorage" int8,
    "used" int8,
    "last_heartbeat" timestamp(6),
    "heartbeat_err"  varchar
);

