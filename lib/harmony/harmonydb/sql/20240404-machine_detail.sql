CREATE TABLE harmony_machine_details (
  id SERIAL PRIMARY KEY,
	tasks TEXT,
  layers TEXT,
  startup_time TIMESTAMP,
  miners TEXT,
  machine_id INTEGER,
  FOREIGN KEY (machine_id) REFERENCES harmony_machines(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX machine_details_machine_id ON harmony_machine_details(machine_id);

