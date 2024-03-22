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
        primary key (send_task_id, from_key)
);

comment on column message_sends.from_key is 'text f[1/3/4]... address';
comment on column message_sends.to_addr is 'text f[0/1/2/3/4]... address';
comment on column message_sends.send_reason is 'optional description of send reason';
comment on column message_sends.send_task_id is 'harmony task id of the send task';

comment on column message_sends.unsigned_data is 'unsigned message data';
comment on column message_sends.unsigned_cid is 'unsigned message cid';

comment on column message_sends.nonce is 'assigned message nonce, set while the send task is executing';
comment on column message_sends.signed_data is 'signed message data, set while the send task is executing';
comment on column message_sends.signed_cid is 'signed message cid, set while the send task is executing';

comment on column message_sends.send_time is 'time when the send task was executed, set after pushing the message to the network';
comment on column message_sends.send_success is 'whether this message was broadcasted to the network already, null if not yet attempted, true if successful, false if failed';
comment on column message_sends.send_error is 'error message if send_success is false';

create unique index message_sends_success_index
    on message_sends (from_key, nonce)
    where send_success is not false;

comment on index message_sends_success_index is
'message_sends_success_index enforces sender/nonce uniqueness, it is a conditional index that only indexes rows where send_success is not false. This allows us to have multiple rows with the same sender/nonce, as long as only one of them was successfully broadcasted (true) to the network or is in the process of being broadcasted (null).';

create table message_send_locks
(
    from_key text   not null,
    task_id  bigint not null,
    claimed_at timestamp not null,

    constraint message_send_locks_pk
        primary key (from_key)
);
