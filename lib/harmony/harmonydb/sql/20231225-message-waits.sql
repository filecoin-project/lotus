create table message_waits (
    signed_message_cid text primary key,
    waiter_machine_id int references harmony_machines (id) on delete set null,

    executed_tsk_cid text,
    executed_tsk_epoch bigint,
    executed_msg_cid text,
    executed_msg_data jsonb,

    executed_rcpt_exitcode bigint,
    executed_rcpt_return bytea,
    executed_rcpt_gas_used bigint
)
