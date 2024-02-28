create table parked_pieces (
    id bigserial primary key,
    created_at timestamp default current_timestamp,

    piece_cid text not null,
    piece_padded_size bigint not null,
    piece_raw_size text not null,

    complete boolean not null default false
);

/*
 * This table is used to keep track of the references to the parked pieces
 * so that we can delete them when they are no longer needed.
 *
 * All references into the parked_pieces table should be done through this table.
 */
create table parked_piece_refs (
    ref_id bigserial primary key,
    piece_id bigint not null,

    foreign key (piece_id) references parked_pieces(id) on delete cascade
);

create table park_piece_tasks (
    task_id bigint not null
      constraint park_piece_tasks_pk
          primary key,

    piece_ref_id bigint not null,

    data_url text not null,
    data_headers jsonb not null default '{}',
    data_raw_size bigint not null,
    data_delete_on_finalize bool not null
);
