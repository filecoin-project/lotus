ALTER TABLE sectors_sdr_initial_pieces
    ADD COLUMN created_at TIMESTAMP;

CREATE OR REPLACE FUNCTION insert_sector_market_piece(
    v_sp_id bigint,
    v_sector_number bigint,
    v_piece_index bigint,
    v_piece_cid text,
    v_piece_size bigint,
    v_data_url text,
    v_data_headers jsonb,
    v_data_raw_size bigint,
    v_data_delete_on_finalize boolean,
    v_f05_publish_cid text,
    v_f05_deal_id bigint,
    v_f05_deal_proposal jsonb,
    v_f05_deal_start_epoch bigint,
    v_f05_deal_end_epoch bigint
    ) RETURNS void AS $$
        BEGIN
        INSERT INTO sectors_sdr_initial_pieces (
            sp_id,
            sector_number,
            piece_index,
            created_at,
            piece_cid,
            piece_size,
            data_url,
            data_headers,
            data_raw_size,
            data_delete_on_finalize,
            f05_publish_cid,
            f05_deal_id,
            f05_deal_proposal,
            f05_deal_start_epoch,
            f05_deal_end_epoch
        ) VALUES (
                     v_sp_id,
                     v_sector_number,
                     v_piece_index,
                     NOW(),
                     v_piece_cid,
                     v_piece_size,
                     v_data_url,
                     v_data_headers,
                     v_data_raw_size,
                     v_data_delete_on_finalize,
                     v_f05_publish_cid,
                     v_f05_deal_id,
                     v_f05_deal_proposal,
                     v_f05_deal_start_epoch,
                     v_f05_deal_end_epoch
                 )
            ON CONFLICT (sp_id, sector_number, piece_index) DO NOTHING;

        IF NOT FOUND THEN
                RAISE EXCEPTION 'Conflict detected for piece_index %', v_piece_index;
        END IF;
        END;
        $$ LANGUAGE plpgsql;



CREATE OR REPLACE FUNCTION insert_sector_ddo_piece(
    v_sp_id bigint,
    v_sector_number bigint,
    v_piece_index bigint,
    v_piece_cid text,
    v_piece_size bigint,
    v_data_url text,
    v_data_headers jsonb,
    v_data_raw_size bigint,
    v_data_delete_on_finalize boolean,
    v_direct_start_epoch bigint,
    v_direct_end_epoch bigint,
    v_direct_piece_activation_manifest jsonb
    ) RETURNS void AS $$
    BEGIN
    INSERT INTO sectors_sdr_initial_pieces (
        sp_id,
        sector_number,
        piece_index,
        created_at,
        piece_cid,
        piece_size,
        data_url,
        data_headers,
        data_raw_size,
        data_delete_on_finalize,
        direct_start_epoch,
        direct_end_epoch,
        direct_piece_activation_manifest
    ) VALUES (
                 v_sp_id,
                 v_sector_number,
                 v_piece_index,
                 NOW(),
                 v_piece_cid,
                 v_piece_size,
                 v_data_url,
                 v_data_headers,
                 v_data_raw_size,
                 v_data_delete_on_finalize,
                 v_direct_start_epoch,
                 v_direct_end_epoch,
                 v_direct_piece_activation_manifest
             )
        ON CONFLICT (sp_id, sector_number, piece_index) DO NOTHING;

    IF NOT FOUND THEN
            RAISE EXCEPTION 'Conflict detected for piece_index %', v_piece_index;
    END IF;
    END;
    $$ LANGUAGE plpgsql;
