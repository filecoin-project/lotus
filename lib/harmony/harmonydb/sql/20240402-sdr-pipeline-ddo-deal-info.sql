ALTER TABLE sectors_sdr_initial_pieces
    ADD COLUMN direct_start_epoch BIGINT;

ALTER TABLE sectors_sdr_initial_pieces
    ADD COLUMN direct_end_epoch BIGINT;

ALTER TABLE sectors_sdr_initial_pieces
    ADD COLUMN direct_piece_activation_manifest JSONB;
