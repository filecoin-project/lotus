CREATE TABLE itest_scratch (
    id SERIAL PRIMARY KEY,
	content TEXT,
    some_int INTEGER,
    second_int INTEGER,
	update_time TIMESTAMP DEFAULT current_timestamp
)