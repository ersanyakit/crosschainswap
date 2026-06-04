CREATE TABLE IF NOT EXISTS pools (
    id VARCHAR(255) PRIMARY KEY,
    chain_key VARCHAR(255) NOT NULL,
    venue_key VARCHAR(255) NOT NULL,
    kind VARCHAR(50) NOT NULL,
    token0 VARCHAR(255) NOT NULL,
    token1 VARCHAR(255) NOT NULL,
    reserve0 NUMERIC(78, 0) NOT NULL,
    reserve1 NUMERIC(78, 0) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pools_venue_key ON pools(venue_key);
CREATE INDEX IF NOT EXISTS idx_pools_chain_key ON pools(chain_key);
CREATE INDEX IF NOT EXISTS idx_pools_token0 ON pools(token0);
CREATE INDEX IF NOT EXISTS idx_pools_token1 ON pools(token1);
