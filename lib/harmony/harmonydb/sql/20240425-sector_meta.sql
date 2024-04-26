CREATE TABLE sectors_meta (
    sp_id BIGINT NOT NULL,
    sector_num BIGINT NOT NULL,

    reg_seal_proof INT NOT NULL,
    ticket_epoch BIGINT NOT NULL,
    ticket_value BYTEA NOT NULL,

    orig_sealed_cid TEXT NOT NULL,
    orig_unsealed_cid TEXT NOT NULL,

    cur_sealed_cid TEXT NOT NULL,
    cur_unsealed_cid TEXT NOT NULL,

    msg_cid_precommit TEXT,
    msg_cid_commit TEXT,
    msg_cid_update TEXT, -- snapdeal update

    seed_epoch BIGINT NOT NULL,
    seed_value BYTEA NOT NULL,

    PRIMARY KEY (sp_id, sector_num)
);

CREATE TABLE sector_meta_pieces (
    sp_id BIGINT NOT NULL,
    sector_num BIGINT NOT NULL,
    piece_num BIGINT NOT NULL,

    piece_cid TEXT NOT NULL,
    piece_size BIGINT NOT NULL, -- padded size

    requested_keep_data BOOLEAN NOT NULL,
    raw_data_size BIGINT, -- null = piece_size.unpadded()

    start_epoch BIGINT,
    orig_end_epoch BIGINT,

    f05_deal_id BIGINT,
    ddo_pam jsonb,

    PRIMARY KEY (sp_id, sector_num, piece_num),
    FOREIGN KEY (sp_id, sector_num) REFERENCES sectors_meta(sp_id, sector_num) ON DELETE CASCADE
);
