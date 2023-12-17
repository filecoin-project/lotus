-- NOTE: task_ids can be the same between different task types and between different sectors
--  e.g. SN-supraseal doing 128 sdr/TreeC/TreeR with the same task_id

create table sectors_sdr_pipeline (
    sp_id bigint not null,
    sector_number bigint not null,

    -- at request time
    create_time timestamp not null,
    reg_seal_proof int not null,

    -- sdr
    ticket_epoch bigint,
    ticket_value bytea,

    task_id_sdr bigint,
    after_sdr bool not null default false,

    -- tree D
    tree_d_cid text, -- commd from treeD compute

    task_id_tree_d bigint,
    after_tree_d bool not null default false,

    -- tree C
    task_id_tree_c bigint,
    after_tree_c bool not null default false,

    -- tree R
    tree_r_cid text, -- commr from treeR compute

    task_id_tree_r bigint,
    after_tree_r bool not null default false,

    -- precommit message sending
    precommit_msg_cid text,

    task_id_precommit_msg bigint,
    after_precommit_msg bool not null default false,

    -- precommit message wait
    seed_epoch bigint,
    precommit_msg_tsk bytea,

    task_id_precommit_msg_wait bigint,
    after_precommit_msg_success bool not null default false,

    -- seed
    seed_value bytea,

    -- Commit (PoRep snark)
    task_id_porep bigint,
    porep_proof bytea,

    -- Commit message sending
    commit_msg_cid text,

    task_id_commit_msg bigint,
    after_commit_msg bool not null default false,

    -- Commit message wait
    commit_msg_tsk bytea,

    task_id_commit_msg_wait bigint,
    after_commit_msg_success bool not null default false,

    -- Failure handling
    failed bool not null default false,
    failed_at timestamp,
    failed_reason varchar(20),
    failed_reason_msg text,

    -- constraints
    primary key (sp_id, sector_number)
);

create table sectors_sdr_initial_pieces (
    sp_id bigint not null,
    sector_number bigint not null,

    piece_index bigint not null,
    piece_cid text not null,
    piece_size bigint not null,

    primary key (sp_id, sector_number, piece_index)
);
