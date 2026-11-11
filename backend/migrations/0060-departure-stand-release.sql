CREATE TABLE departure_stand_releases (
    strip_id INTEGER PRIMARY KEY REFERENCES strips(id) ON DELETE CASCADE
);
