-- Tables of the tenancy context (ADR-053): the aggregate in the
-- plural, tenant_id leading the key, a typed column only for what a query
-- filters by and the rest of the state in snapshot jsonb.
CREATE TABLE IF NOT EXISTS stores (
  tenant_id text   NOT NULL,
  store_id  text   NOT NULL,
  version   bigint NOT NULL DEFAULT 0,
  -- Typed because FindStoresByStatus filters by it.
  status    text   NOT NULL,
  snapshot  jsonb  NOT NULL,
  -- Derived from the snapshot because the unique constraint needs a column (ADR-066).
  name      text   GENERATED ALWAYS AS (snapshot->>'name') STORED NOT NULL,
  CONSTRAINT stores_pkey PRIMARY KEY (tenant_id, store_id),
  CONSTRAINT stores_status_check CHECK (status IN ('active', 'suspended')),
  CONSTRAINT stores_tenant_id_name_key UNIQUE (tenant_id, name)
);

-- The index behind FindStoresByStatus leads with the tenant for the same reason
-- the PK does: without it, a search by status scans every tenant's rows.
CREATE INDEX IF NOT EXISTS stores_tenant_id_status_idx ON stores (tenant_id, status);

-- The cross-tenant probes of the kernel Table and Relation (IDN-12) look the
-- value up without the tenant, which an index led by tenant_id cannot serve.
CREATE INDEX IF NOT EXISTS stores_store_id_idx ON stores (store_id);
CREATE INDEX IF NOT EXISTS stores_status_idx ON stores (status);
