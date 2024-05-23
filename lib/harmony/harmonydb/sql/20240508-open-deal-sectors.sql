ALTER TABLE sectors_sdr_initial_pieces
    ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT current_timestamp;

create table open_sector_pieces (
    sp_id bigint not null,
    sector_number bigint not null,

    piece_index bigint not null,
    piece_cid text not null,
    piece_size bigint not null, -- padded size

    -- data source
    data_url text not null,
    data_headers jsonb not null default '{}',
    data_raw_size bigint not null,
    data_delete_on_finalize bool not null,

    -- deal info
    f05_publish_cid text,
    f05_deal_id bigint,
    f05_deal_proposal jsonb,
    f05_deal_start_epoch bigint,
    f05_deal_end_epoch bigint,

    -- ddo deal info
    -- added in 20240402-sdr-pipeline-ddo-deal-info.sql
    direct_start_epoch bigint,
    direct_end_epoch bigint,
    direct_piece_activation_manifest jsonb,

    -- created_at added in 20240508-open-deal-sectors.sql
    created_at timestamp NOT NULL DEFAULT current_timestamp,

    -- sectors_sdr_initial_pieces table is a copy of this
    -- all alters should happen on both tables except constraints

    primary key (sp_id, sector_number, piece_index)
);


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
INSERT INTO open_sector_pieces (
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
         ) ON CONFLICT (sp_id, sector_number, piece_index) DO NOTHING;
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
INSERT INTO open_sector_pieces (
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
         ) ON CONFLICT (sp_id, sector_number, piece_index) DO NOTHING;
IF NOT FOUND THEN
        RAISE EXCEPTION 'Conflict detected for piece_index %', v_piece_index;
END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION transfer_and_delete_open_piece(v_sp_id bigint, v_sector_number bigint)
RETURNS void AS $$
BEGIN
    -- Copy data from open_sector_pieces to sectors_sdr_initial_pieces
INSERT INTO sectors_sdr_initial_pieces (
    sp_id,
    sector_number,
    piece_index,
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
    f05_deal_end_epoch,
    direct_start_epoch,
    direct_end_epoch,
    direct_piece_activation_manifest,
    created_at
)
SELECT
    sp_id,
    sector_number,
    piece_index,
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
    f05_deal_end_epoch,
    direct_start_epoch,
    direct_end_epoch,
    direct_piece_activation_manifest,
    created_at
FROM
    open_sector_pieces
WHERE
    sp_id = v_sp_id AND
    sector_number = v_sector_number;

-- Check for successful insertion, then delete the corresponding row from open_sector_pieces
IF FOUND THEN
DELETE FROM open_sector_pieces
WHERE sp_id = v_sp_id AND sector_number = v_sector_number;
ELSE
        RAISE EXCEPTION 'No data found to transfer for sp_id % and sector_number %', v_sp_id, v_sector_number;
END IF;
END;
$$ LANGUAGE plpgsql;
