-- Schema ownership is explicit. Credentials below are synthetic local-lab values.
CREATE ROLE core5g_udm LOGIN PASSWORD 'local-lab-udm';
CREATE ROLE core5g_amf LOGIN PASSWORD 'local-lab-amf';
CREATE ROLE core5g_smf LOGIN PASSWORD 'local-lab-smf';
CREATE ROLE core5g_upf LOGIN PASSWORD 'local-lab-upf';
CREATE SCHEMA udm AUTHORIZATION core5g_udm;
CREATE SCHEMA amf AUTHORIZATION core5g_amf;
CREATE SCHEMA smf AUTHORIZATION core5g_smf;
CREATE SCHEMA upf AUTHORIZATION core5g_upf;

CREATE TABLE udm.subscribers (
 id TEXT PRIMARY KEY, data JSONB NOT NULL,
 supi VARCHAR(32) GENERATED ALWAYS AS (data->>'supi') STORED UNIQUE NOT NULL,
 status VARCHAR(24) GENERATED ALWAYS AS (data->>'status') STORED NOT NULL CHECK(status IN ('ENABLED','DISABLED')),
 auth_profile VARCHAR(80) GENERATED ALWAYS AS (data->>'auth_profile') STORED NOT NULL,
 CHECK(id=supi)
);
CREATE TABLE amf.equipments (
 id TEXT PRIMARY KEY, data JSONB NOT NULL,
 ue_id UUID GENERATED ALWAYS AS ((data->>'ue_id')::uuid) STORED UNIQUE NOT NULL,
 supi VARCHAR(32) GENERATED ALWAYS AS (data->>'supi') STORED NOT NULL,
 state VARCHAR(24) GENERATED ALWAYS AS (data->>'state') STORED NOT NULL CHECK(state IN ('DISCONNECTED','REGISTERING','REGISTERED','SESSION_ACTIVE')),
 CHECK(id=ue_id::text)
);
CREATE TABLE amf.registrations (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE amf.session_events (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE smf.sessions (
 id TEXT PRIMARY KEY, data JSONB NOT NULL,
 session_id UUID GENERATED ALWAYS AS ((data->>'session_id')::uuid) STORED UNIQUE NOT NULL,
 request_id UUID GENERATED ALWAYS AS ((data->>'request_id')::uuid) STORED UNIQUE NOT NULL,
 ue_id UUID GENERATED ALWAYS AS ((data->>'ue_id')::uuid) STORED NOT NULL,
 ip_address INET GENERATED ALWAYS AS (NULLIF(data->>'ip_address','')::inet) STORED,
 state VARCHAR(24) GENERATED ALWAYS AS (data->>'state') STORED NOT NULL CHECK(state IN ('CREATING','ACTIVE','RELEASING','RELEASED','FAILED')),
 CHECK(id=session_id::text)
);
CREATE UNIQUE INDEX uq_active_ip ON smf.sessions(ip_address) WHERE state IN ('CREATING','ACTIVE','RELEASING') AND ip_address IS NOT NULL;
CREATE INDEX session_ue ON smf.sessions(ue_id);
CREATE TABLE upf.routes (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE upf.settings (id TEXT PRIMARY KEY, data JSONB NOT NULL);
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA udm TO core5g_udm;
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA amf TO core5g_amf;
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA smf TO core5g_smf;
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA upf TO core5g_upf;
