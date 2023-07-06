CREATE TABLE itest_scratch (
    id INTEGER SERIAL PRIMARY KEY,
	content TEXT,
    some_int INTEGER,
	update_time TIMESTAMP DEFAULT current_timestamp
)