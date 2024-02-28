create table parked_pieces (
    id serial primary key,
    created_at timestamp default current_timestamp,

    piece_cid text not null,
    piece_padded_size bigint not null,
    piece_raw_size text not null,
    
    complete boolean not null default false
);

create table parked_piece_refs (
    ref_id serial primary key,
    piece_id int not null,

    foreign key (piece_id) references parked_pieces(id) on delete cascade
);

create table park_piece_tasks (
    task_id bigint not null
      constraint park_piece_tasks_pk
          primary key,

    piece_ref_id int not null,

    data_url text not null,
    data_headers jsonb not null default '{}',
    data_raw_size bigint not null,
    data_delete_on_finalize bool not null
);
