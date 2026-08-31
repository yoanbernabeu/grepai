CREATE TABLE users (id INT PRIMARY KEY, name TEXT);
CREATE VIEW active_users AS SELECT * FROM users WHERE active = TRUE;
CREATE FUNCTION add(x INT, y INT) RETURNS INT AS $$ SELECT x + y $$ LANGUAGE SQL;
CREATE INDEX users_name_idx ON users(name);
