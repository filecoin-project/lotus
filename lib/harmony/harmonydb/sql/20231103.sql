create table message_sends
(
    from_key     text               not null,
    nonce        bigint             not null,
    to_addr      text               not null,
    signed_data  bytea              not null,
    signed_json  jsonb              not null,
    signed_cid   text               not null,
    send_time    date default CURRENT_TIMESTAMP,
    send_reason  text,
    send_success bool default false not null,
    constraint message_sends_pk
        primary key (from_key, nonce)
);

comment on column message_sends.from_key is 'text f[1/3/4]... address';
comment on column message_sends.nonce is 'assigned message nonce';
comment on column message_sends.to_addr is 'text f[0/1/2/3/4]... address';
comment on column message_sends.signed_data is 'signed message data';
comment on column message_sends.signed_cid is 'signed message cid';
comment on column message_sends.send_reason is 'optional description of send reason';
comment on column message_sends.send_success is 'whether this message was broadcasted to the network already';
