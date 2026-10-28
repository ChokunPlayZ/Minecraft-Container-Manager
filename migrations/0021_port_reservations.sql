-- Reservations are maintained by triggers in the same transaction as the
-- server configuration, including callers that write settings directly.
CREATE TABLE server_port_reservations (
 host_port INTEGER NOT NULL PRIMARY KEY CHECK(host_port BETWEEN 1 AND 65535),
 server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 protocol TEXT NOT NULL DEFAULT 'tcp'
);
INSERT INTO server_port_reservations(host_port,server_id,protocol)
 SELECT host_port,id,'tcp' FROM servers WHERE host_port>0;
INSERT INTO server_port_reservations(host_port,server_id,protocol)
 SELECT CAST(json_extract(p.value,'$.host_port') AS INTEGER),s.id,COALESCE(json_extract(p.value,'$.protocol'),'tcp')
 FROM servers s,json_each(s.extra_ports) p WHERE json_extract(p.value,'$.host_port')>0;
CREATE TRIGGER server_ports_insert AFTER INSERT ON servers BEGIN
 INSERT INTO server_port_reservations(host_port,server_id) SELECT NEW.host_port,NEW.id WHERE NEW.host_port>0;
 INSERT INTO server_port_reservations(host_port,server_id,protocol)
 SELECT CAST(json_extract(value,'$.host_port') AS INTEGER),NEW.id,COALESCE(json_extract(value,'$.protocol'),'tcp')
 FROM json_each(NEW.extra_ports) WHERE json_extract(value,'$.host_port')>0;
END;
CREATE TRIGGER server_ports_update AFTER UPDATE OF host_port,extra_ports ON servers BEGIN
 DELETE FROM server_port_reservations WHERE server_id=NEW.id;
 INSERT INTO server_port_reservations(host_port,server_id) SELECT NEW.host_port,NEW.id WHERE NEW.host_port>0;
 INSERT INTO server_port_reservations(host_port,server_id,protocol)
 SELECT CAST(json_extract(value,'$.host_port') AS INTEGER),NEW.id,COALESCE(json_extract(value,'$.protocol'),'tcp')
 FROM json_each(NEW.extra_ports) WHERE json_extract(value,'$.host_port')>0;
END;
