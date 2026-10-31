-- Tables of the iam context (ADR-053): the aggregate in the
-- plural, tenant_id leading the key, a typed column only for what a query
-- filters by and the rest of the state in snapshot jsonb.
CREATE TABLE IF NOT EXISTS roles (
  tenant_id text   NOT NULL,
  role_id   text   NOT NULL,
  version   bigint NOT NULL DEFAULT 0,
  -- Typed because FindRolesByStatus filters by it.
  status    text   NOT NULL,
  snapshot  jsonb  NOT NULL,
  -- Derived from the snapshot because the unique constraint needs a column (ADR-066).
  name      text   GENERATED ALWAYS AS (snapshot->>'name') STORED NOT NULL,
  CONSTRAINT roles_pkey PRIMARY KEY (tenant_id, role_id),
  CONSTRAINT roles_status_check CHECK (status IN ('active', 'retired')),
  CONSTRAINT roles_tenant_id_name_key UNIQUE (tenant_id, name)
);

-- The index behind FindRolesByStatus leads with the tenant for the same reason
-- the PK does: without it, a search by status scans every tenant's rows.
CREATE INDEX IF NOT EXISTS roles_tenant_id_status_idx ON roles (tenant_id, status);

-- The cross-tenant probes of the kernel Table and Relation (IDN-12) look the
-- value up without the tenant, which an index led by tenant_id cannot serve.
CREATE INDEX IF NOT EXISTS roles_role_id_idx ON roles (role_id);
CREATE INDEX IF NOT EXISTS roles_status_idx ON roles (status);

CREATE TABLE IF NOT EXISTS invitations (
  tenant_id     text   NOT NULL,
  invitation_id text   NOT NULL,
  version       bigint NOT NULL DEFAULT 0,
  -- Typed because FindInvitationsByStatus filters by it.
  status        text   NOT NULL,
  snapshot      jsonb  NOT NULL,
  CONSTRAINT invitations_pkey PRIMARY KEY (tenant_id, invitation_id),
  CONSTRAINT invitations_status_check CHECK (status IN ('pending', 'provisioned', 'failed'))
);

CREATE INDEX IF NOT EXISTS invitations_tenant_id_status_idx ON invitations (tenant_id, status);
CREATE INDEX IF NOT EXISTS invitations_invitation_id_idx ON invitations (invitation_id);
CREATE INDEX IF NOT EXISTS invitations_status_idx ON invitations (status);

CREATE TABLE IF NOT EXISTS memberships (
  tenant_id     text    NOT NULL,
  membership_id text    NOT NULL,
  version       bigint  NOT NULL DEFAULT 0,
  -- Typed because FindMembershipsByStatus filters by it.
  status        text    NOT NULL,
  snapshot      jsonb   NOT NULL,
  -- true only for the active owner and NULL otherwise, so the constraint holds
  -- one owner per store and leaves every other membership out (ADR-066).
  owner         boolean GENERATED ALWAYS AS (CASE WHEN status = 'active' AND snapshot->'roles' @> '["owner"]'::jsonb THEN true END) STORED,
  CONSTRAINT memberships_pkey PRIMARY KEY (tenant_id, membership_id),
  CONSTRAINT memberships_status_check CHECK (status IN ('active', 'removed')),
  CONSTRAINT memberships_tenant_id_owner_key UNIQUE (tenant_id, owner)
);

CREATE INDEX IF NOT EXISTS memberships_tenant_id_status_idx ON memberships (tenant_id, status);
CREATE INDEX IF NOT EXISTS memberships_membership_id_idx ON memberships (membership_id);
CREATE INDEX IF NOT EXISTS memberships_status_idx ON memberships (status);

CREATE TABLE IF NOT EXISTS buyers (
  tenant_id text   NOT NULL,
  buyer_id  text   NOT NULL,
  version   bigint NOT NULL DEFAULT 0,
  -- Typed because FindBuyersByStatus filters by it.
  status    text   NOT NULL,
  snapshot  jsonb  NOT NULL,
  CONSTRAINT buyers_pkey PRIMARY KEY (tenant_id, buyer_id),
  CONSTRAINT buyers_status_check CHECK (status IN ('active', 'blocked'))
);

CREATE INDEX IF NOT EXISTS buyers_tenant_id_status_idx ON buyers (tenant_id, status);
CREATE INDEX IF NOT EXISTS buyers_buyer_id_idx ON buyers (buyer_id);
CREATE INDEX IF NOT EXISTS buyers_status_idx ON buyers (status);
