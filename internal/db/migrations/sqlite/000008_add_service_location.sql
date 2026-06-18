ALTER TABLE clusters ADD COLUMN service_location TEXT NOT NULL DEFAULT 'native';
ALTER TABLE nodes ADD COLUMN service_location TEXT NOT NULL DEFAULT 'native';
ALTER TABLE nodes ADD COLUMN docker_available INTEGER NOT NULL DEFAULT 0;
