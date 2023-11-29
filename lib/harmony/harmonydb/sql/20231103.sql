create table message_sends
(
    from_key     text   not null,
    to_addr      text   not null,
    send_reason  text   not null,
    send_task_id bigint not null,

    unsigned_data bytea not null,
    unsigned_cid  text  not null,

    nonce        bigint,
    signed_data  bytea,
    signed_json  jsonb,
    signed_cid   text,

    send_time    timestamp default null,
    send_success boolean   default null,
    send_error   text,

    constraint message_sends_pk
        primary key (from_key, nonce)
);

create table message_send_locks
(
    from_key text   not null,
    task_id  bigint not null,
    claimed_at timestamp not null,

    constraint message_send_locks_pk
        primary key (from_key)
);

comment on column message_sends.from_key is 'text f[1/3/4]... address';
comment on column message_sends.nonce is 'assigned message nonce';
comment on column message_sends.to_addr is 'text f[0/1/2/3/4]... address';
comment on column message_sends.signed_data is 'signed message data';
comment on column message_sends.signed_cid is 'signed message cid';
comment on column message_sends.send_reason is 'optional description of send reason';
comment on column message_sends.send_success is 'whether this message was broadcasted to the network already';
