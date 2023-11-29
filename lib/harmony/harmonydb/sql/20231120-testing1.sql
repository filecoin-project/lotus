CREATE TABLE harmony_test (
    task_id bigint         
        constraint harmony_test_pk
            primary key,
    options text,
    result text
);
ALTER TABLE wdpost_proofs ADD COLUMN test_task_id bigint;