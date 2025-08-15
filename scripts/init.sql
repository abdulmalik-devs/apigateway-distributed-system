CREATE TABLE IF NOT EXISTS gateway_requests (
  id SERIAL PRIMARY KEY,
  service TEXT,
  method TEXT,
  path TEXT,
  status INT,
  duration_ms INT,
  user_id TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
  id SERIAL PRIMARY KEY,
  key TEXT UNIQUE NOT NULL,
  owner TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
