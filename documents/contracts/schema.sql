-- =====================================================================
-- Grocery platform — Postgres schema, version 1
-- Target: Postgres 17 (Neon). Run as the database owner role.
-- Rules:
--   * Money is BIGINT cents. No floats.
--   * Enum values are TEXT + CHECK and MUST match contracts/enums.json.
--   * Timestamps are TIMESTAMPTZ (UTC). Local logic uses Asia/Colombo.
--   * The application connects as role app_rw. RLS applies to app_rw.
--   * Request context is set per transaction with set_config(..., true):
--       app.user_id   uuid of the signed-in user (or empty)
--       app.shop_id   uuid of the shop in a console request (or empty)
--       app.privileged 'on' for super admin and system jobs
-- =====================================================================

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- App role. In Neon, create the LOGIN role in the console first;
-- this block only makes local runs work.
DO $$
BEGIN
  CREATE ROLE app_rw NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE SCHEMA IF NOT EXISTS app;

CREATE OR REPLACE FUNCTION app.uid() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT nullif(current_setting('app.user_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION app.shop() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT nullif(current_setting('app.shop_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION app.privileged() RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT coalesce(current_setting('app.privileged', true), '') = 'on'
$$;

CREATE OR REPLACE FUNCTION app.touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION app.block_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'table % is append-only', TG_TABLE_NAME;
END $$;

-- ---------------------------------------------------------------------
-- Users and access
-- ---------------------------------------------------------------------

CREATE TABLE users (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  google_sub        text NOT NULL UNIQUE,
  email             text NOT NULL UNIQUE CHECK (email = lower(email) AND char_length(email) <= 254),
  name              text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  phone             text CHECK (phone ~ '^\+94[0-9]{9}$'),
  phone_verified_at timestamptz,
  status            text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','BLOCKED','DELETED')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE shops (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug             text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND char_length(slug) BETWEEN 3 AND 40),
  name             text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 80),
  phone            text CHECK (phone ~ '^\+94[0-9]{9}$'),
  address_text     text CHECK (char_length(address_text) <= 200),
  lat              numeric(9,6) NOT NULL CHECK (lat BETWEEN 5.8 AND 10.0),
  lng              numeric(9,6) NOT NULL CHECK (lng BETWEEN 79.4 AND 82.0),
  logo_url         text CHECK (char_length(logo_url) <= 300),
  theme_color      text NOT NULL DEFAULT '#222222' CHECK (theme_color ~ '^#[0-9a-fA-F]{6}$'),
  status           text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','SUSPENDED')),
  accepting_orders boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER shops_touch BEFORE UPDATE ON shops FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE memberships (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           uuid NOT NULL REFERENCES users(id),
  scope             text NOT NULL CHECK (scope IN ('PLATFORM','SHOP')),
  shop_id           uuid REFERENCES shops(id),
  role              text NOT NULL CHECK (role IN ('SUPER_ADMIN','SHOP_OWNER','SHOP_STAFF','SHOP_DRIVER','PLATFORM_DRIVER')),
  can_edit_products boolean NOT NULL DEFAULT false,
  status            text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','REMOVED')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (scope = 'PLATFORM' AND shop_id IS NULL AND role IN ('SUPER_ADMIN','PLATFORM_DRIVER'))
    OR
    (scope = 'SHOP' AND shop_id IS NOT NULL AND role IN ('SHOP_OWNER','SHOP_STAFF','SHOP_DRIVER'))
  )
);
CREATE UNIQUE INDEX memberships_one_role ON memberships
  (user_id, coalesce(shop_id, '00000000-0000-0000-0000-000000000000'::uuid), role);
CREATE INDEX memberships_shop ON memberships (shop_id, role) WHERE status = 'ACTIVE';

CREATE TABLE invites (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email       text NOT NULL CHECK (email = lower(email)),
  shop_id     uuid REFERENCES shops(id),
  role        text NOT NULL CHECK (role IN ('SHOP_OWNER','SHOP_STAFF','SHOP_DRIVER','PLATFORM_DRIVER')),
  can_edit_products boolean NOT NULL DEFAULT false,
  token_hash  bytea NOT NULL UNIQUE,
  invited_by  uuid NOT NULL REFERENCES users(id),
  expires_at  timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((role = 'PLATFORM_DRIVER') = (shop_id IS NULL))
);

CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id),
  family_id    uuid NOT NULL,
  refresh_hash bytea NOT NULL UNIQUE,
  totp_ok      boolean NOT NULL DEFAULT false,
  expires_at   timestamptz NOT NULL,
  rotated_at   timestamptz,
  revoked_at   timestamptz,
  user_agent   text CHECK (char_length(user_agent) <= 200),
  ip_hash      bytea,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_family ON sessions (family_id);

CREATE TABLE admin_totp (
  user_id        uuid PRIMARY KEY REFERENCES users(id),
  secret_enc     bytea NOT NULL,
  enabled_at     timestamptz,
  last_used_step bigint NOT NULL DEFAULT 0
);

CREATE TABLE otp_codes (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id),
  phone      text NOT NULL CHECK (phone ~ '^\+94[0-9]{9}$'),
  purpose    text NOT NULL CHECK (purpose IN ('PHONE_VERIFY')),
  code_hash  bytea NOT NULL,
  attempts   int NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_codes_phone ON otp_codes (phone, created_at DESC);
CREATE INDEX otp_codes_user ON otp_codes (user_id, created_at DESC);

-- Fixed-window counters for sensitive actions only (OTP, coupon tries, code tries).
CREATE TABLE rate_counters (
  key          text NOT NULL CHECK (char_length(key) <= 120),
  window_start timestamptz NOT NULL,
  count        int NOT NULL DEFAULT 0,
  PRIMARY KEY (key, window_start)
);

CREATE TABLE addresses (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id),
  label      text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 30),
  line1      text NOT NULL CHECK (char_length(line1) BETWEEN 3 AND 160),
  landmark   text CHECK (char_length(landmark) <= 120),
  lat        numeric(9,6) NOT NULL CHECK (lat BETWEEN 5.8 AND 10.0),
  lng        numeric(9,6) NOT NULL CHECK (lng BETWEEN 79.4 AND 82.0),
  is_default boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX addresses_user ON addresses (user_id);
CREATE UNIQUE INDEX addresses_one_default ON addresses (user_id) WHERE is_default;

-- ---------------------------------------------------------------------
-- Shop settings, payment accounts, hours, slots, fees
-- ---------------------------------------------------------------------

CREATE TABLE payment_accounts (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type  text NOT NULL CHECK (owner_type IN ('PLATFORM','SHOP')),
  shop_id     uuid REFERENCES shops(id),
  provider    text NOT NULL CHECK (provider IN ('PAYHERE','MANUAL')),
  mode        text NOT NULL DEFAULT 'SANDBOX' CHECK (mode IN ('SANDBOX','LIVE')),
  merchant_id text CHECK (char_length(merchant_id) <= 40),
  secret_ref  text CHECK (char_length(secret_ref) <= 200),
  public_info jsonb NOT NULL DEFAULT '{}'::jsonb,
  status      text NOT NULL DEFAULT 'NOT_CONNECTED' CHECK (status IN ('NOT_CONNECTED','ACTIVE','DISABLED')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((owner_type = 'PLATFORM') = (shop_id IS NULL)),
  CHECK (provider <> 'PAYHERE' OR status <> 'ACTIVE' OR (merchant_id IS NOT NULL AND secret_ref IS NOT NULL))
);
CREATE TRIGGER payment_accounts_touch BEFORE UPDATE ON payment_accounts FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE shop_settings (
  shop_id                   uuid PRIMARY KEY REFERENCES shops(id),
  delivery_type             text NOT NULL DEFAULT 'SLOT' CHECK (delivery_type IN ('SLOT','COURIER')),
  payment_provider          text NOT NULL DEFAULT 'PAYHERE' CHECK (payment_provider IN ('PAYHERE','MANUAL')),
  payment_account_id        uuid REFERENCES payment_accounts(id),
  driver_source             text NOT NULL DEFAULT 'SHOP' CHECK (driver_source IN ('PLATFORM','SHOP','BOTH')),
  notify_channels           text[] NOT NULL DEFAULT ARRAY['SMS']::text[]
                              CHECK (notify_channels <@ ARRAY['SMS','WHATSAPP','WA_LINK']::text[]),
  markup_bp                 int NOT NULL DEFAULT 250 CHECK (markup_bp BETWEEN 0 AND 2000),
  min_platform_margin_cents bigint NOT NULL DEFAULT 0 CHECK (min_platform_margin_cents >= 0),
  min_order_cents           bigint NOT NULL DEFAULT 0 CHECK (min_order_cents >= 0),
  max_lines                 int NOT NULL DEFAULT 60 CHECK (max_lines BETWEEN 1 AND 200),
  review_timeout_min        int NOT NULL DEFAULT 15 CHECK (review_timeout_min BETWEEN 5 AND 120),
  revision_timeout_min      int NOT NULL DEFAULT 15 CHECK (revision_timeout_min BETWEEN 5 AND 120),
  payment_timeout_min       int NOT NULL DEFAULT 30 CHECK (payment_timeout_min BETWEEN 10 AND 240),
  cart_tenant_id            text CHECK (char_length(cart_tenant_id) <= 64),
  cart_public_key           text CHECK (char_length(cart_public_key) <= 120),
  updated_at                timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER shop_settings_touch BEFORE UPDATE ON shop_settings FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

-- ISO weekday: 1 = Monday ... 7 = Sunday. No row = closed that day. Times are Asia/Colombo.
CREATE TABLE shop_hours (
  shop_id uuid NOT NULL REFERENCES shops(id),
  weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
  opens   time NOT NULL,
  closes  time NOT NULL,
  PRIMARY KEY (shop_id, weekday),
  CHECK (closes > opens)
);

CREATE TABLE delivery_fee_rules (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id          uuid REFERENCES shops(id),  -- NULL = platform default
  base_cents       bigint NOT NULL DEFAULT 0 CHECK (base_cents >= 0),
  per_km_cents     bigint NOT NULL DEFAULT 0 CHECK (per_km_cents >= 0),
  free_km          numeric(5,2) NOT NULL DEFAULT 0 CHECK (free_km >= 0),
  per_kg_cents     bigint NOT NULL DEFAULT 0 CHECK (per_kg_cents >= 0),
  free_kg          numeric(6,2) NOT NULL DEFAULT 0 CHECK (free_kg >= 0),
  free_above_cents bigint CHECK (free_above_cents > 0),
  min_cents        bigint NOT NULL DEFAULT 0 CHECK (min_cents >= 0),
  max_cents        bigint CHECK (max_cents >= 0),
  max_km           numeric(5,2) NOT NULL DEFAULT 8 CHECK (max_km > 0 AND max_km <= 50),
  road_factor      numeric(3,2) NOT NULL DEFAULT 1.30 CHECK (road_factor BETWEEN 1.00 AND 2.00),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (max_cents IS NULL OR max_cents >= min_cents)
);
CREATE UNIQUE INDEX delivery_fee_rules_one_per_shop ON delivery_fee_rules
  (coalesce(shop_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE TRIGGER delivery_fee_rules_touch BEFORE UPDATE ON delivery_fee_rules FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE slots (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id),
  starts_at  timestamptz NOT NULL,
  ends_at    timestamptz NOT NULL,
  cutoff_at  timestamptz NOT NULL,
  capacity   int NOT NULL CHECK (capacity BETWEEN 0 AND 500),
  booked     int NOT NULL DEFAULT 0 CHECK (booked >= 0),
  status     text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, starts_at),
  CHECK (ends_at > starts_at),
  CHECK (cutoff_at <= starts_at)
);
CREATE INDEX slots_shop_time ON slots (shop_id, starts_at);

-- ---------------------------------------------------------------------
-- Catalog
-- ---------------------------------------------------------------------

CREATE TABLE categories (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id        uuid REFERENCES categories(id),
  slug             text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
  sort             int NOT NULL DEFAULT 0,
  default_weight_g int NOT NULL DEFAULT 500 CHECK (default_weight_g BETWEEN 1 AND 100000)
);

CREATE TABLE products (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  barcode     text UNIQUE CHECK (barcode ~ '^[0-9]{6,14}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 120),
  brand       text CHECK (char_length(brand) <= 60),
  size_label  text CHECK (char_length(size_label) <= 30),
  unit        text NOT NULL DEFAULT 'PCS' CHECK (unit IN ('PCS','KG','G','L','ML')),
  weight_g    int CHECK (weight_g BETWEEN 1 AND 100000),
  category_id uuid REFERENCES categories(id),
  image_url   text CHECK (char_length(image_url) <= 300),
  keywords    text NOT NULL DEFAULT '' CHECK (char_length(keywords) <= 300),
  status      text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','HIDDEN')),
  search_doc  text GENERATED ALWAYS AS (lower(name || ' ' || coalesce(brand, '') || ' ' || keywords)) STORED,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_search ON products USING gin (search_doc gin_trgm_ops);
CREATE INDEX products_category ON products (category_id);
CREATE TRIGGER products_touch BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

-- Optional per-category markup per shop (overrides shop_settings.markup_bp).
CREATE TABLE category_markups (
  shop_id     uuid NOT NULL REFERENCES shops(id),
  category_id uuid NOT NULL REFERENCES categories(id),
  markup_bp   int NOT NULL CHECK (markup_bp BETWEEN 0 AND 2000),
  PRIMARY KEY (shop_id, category_id)
);

CREATE TABLE shop_products (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id            uuid NOT NULL REFERENCES shops(id),
  product_id         uuid NOT NULL REFERENCES products(id),
  external_id        text CHECK (char_length(external_id) <= 100),
  mrp_cents          bigint CHECK (mrp_cents > 0),
  shop_price_cents   bigint NOT NULL CHECK (shop_price_cents > 0),
  online_price_cents bigint NOT NULL CHECK (online_price_cents > 0),
  stock_qty          numeric(12,3),
  is_available       boolean NOT NULL DEFAULT true,
  max_per_order      int NOT NULL DEFAULT 20 CHECK (max_per_order BETWEEN 1 AND 999),
  managed_by         text NOT NULL DEFAULT 'MANUAL' CHECK (managed_by IN ('MANUAL','CONNECTOR')),
  source_hash        bytea,
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, product_id),
  UNIQUE (shop_id, external_id),
  CHECK (mrp_cents IS NULL OR online_price_cents <= mrp_cents),
  CHECK (online_price_cents >= shop_price_cents)
);
CREATE INDEX shop_products_shop ON shop_products (shop_id) WHERE is_available;
CREATE INDEX shop_products_product_price ON shop_products (product_id, online_price_cents) WHERE is_available;
CREATE TRIGGER shop_products_touch BEFORE UPDATE ON shop_products FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE price_changes (
  id                  bigserial PRIMARY KEY,
  shop_product_id     uuid NOT NULL REFERENCES shop_products(id),
  old_shop_price      bigint,
  new_shop_price      bigint NOT NULL,
  old_online_price    bigint,
  new_online_price    bigint NOT NULL,
  source              text NOT NULL CHECK (source IN ('MANUAL','CONNECTOR','MARKUP')),
  changed_by          uuid REFERENCES users(id),
  at                  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX price_changes_sp ON price_changes (shop_product_id, at DESC);
CREATE TRIGGER price_changes_append_only BEFORE UPDATE OR DELETE ON price_changes FOR EACH ROW EXECUTE FUNCTION app.block_change();

-- ---------------------------------------------------------------------
-- Shop database connector
-- ---------------------------------------------------------------------

CREATE TABLE connectors (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id              uuid NOT NULL UNIQUE REFERENCES shops(id),
  kind                 text NOT NULL DEFAULT 'NEON_POSTGRES' CHECK (kind IN ('NEON_POSTGRES')),
  secret_ref           text NOT NULL CHECK (char_length(secret_ref) <= 200),
  host_display         text NOT NULL CHECK (char_length(host_display) <= 200),
  source_table         text CHECK (source_table ~ '^[a-z_][a-z0-9_]{0,62}(\.[a-z_][a-z0-9_]{0,62})?$'),
  mapping              jsonb NOT NULL DEFAULT '{}'::jsonb,
  money_unit           text NOT NULL DEFAULT 'RUPEES' CHECK (money_unit IN ('RUPEES','CENTS')),
  change_column        text CHECK (change_column ~ '^[a-z_][a-z0-9_]{0,62}$'),
  interval_min         int NOT NULL DEFAULT 60 CHECK (interval_min BETWEEN 10 AND 1440),
  auto_create_products boolean NOT NULL DEFAULT false,
  trigger_secret_ref   text CHECK (char_length(trigger_secret_ref) <= 200),
  status               text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','PAUSED','FAILING')),
  last_cursor          timestamptz,
  last_run_at          timestamptz,
  last_success_at      timestamptz,
  consecutive_failures int NOT NULL DEFAULT 0,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER connectors_touch BEFORE UPDATE ON connectors FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE connector_runs (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connector_id   uuid NOT NULL REFERENCES connectors(id),
  trigger        text NOT NULL CHECK (trigger IN ('SCHEDULE','MANUAL','WEBHOOK')),
  status         text NOT NULL DEFAULT 'RUNNING' CHECK (status IN ('RUNNING','SUCCESS','FAILED','SKIPPED')),
  rows_read      int NOT NULL DEFAULT 0,
  rows_changed   int NOT NULL DEFAULT 0,
  rows_unmatched int NOT NULL DEFAULT 0,
  rows_held      int NOT NULL DEFAULT 0,
  error          text CHECK (char_length(error) <= 500),
  started_at     timestamptz NOT NULL DEFAULT now(),
  finished_at    timestamptz
);
-- Runs never overlap: at most one RUNNING row per connector.
CREATE UNIQUE INDEX connector_runs_one_running ON connector_runs (connector_id) WHERE status = 'RUNNING';
CREATE INDEX connector_runs_recent ON connector_runs (connector_id, started_at DESC);

CREATE TABLE sync_issues (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connector_id uuid NOT NULL REFERENCES connectors(id),
  shop_id      uuid NOT NULL REFERENCES shops(id),
  kind         text NOT NULL CHECK (kind IN ('UNMATCHED','PRICE_HOLD')),
  external_id  text NOT NULL CHECK (char_length(external_id) <= 100),
  payload      jsonb NOT NULL,
  status       text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','RESOLVED','IGNORED')),
  resolved_by  uuid REFERENCES users(id),
  resolved_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sync_issues_one_open ON sync_issues (connector_id, kind, external_id) WHERE status = 'OPEN';

-- ---------------------------------------------------------------------
-- Coupons
-- ---------------------------------------------------------------------

CREATE TABLE coupons (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code               text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9]{4,20}$'),
  shop_id            uuid REFERENCES shops(id),  -- NULL = all shops
  type               text NOT NULL CHECK (type IN ('PERCENT','FIXED')),
  value              bigint NOT NULL CHECK (value > 0),  -- PERCENT: basis points, FIXED: cents
  max_discount_cents bigint CHECK (max_discount_cents > 0),
  min_order_cents    bigint NOT NULL DEFAULT 0 CHECK (min_order_cents >= 0),
  starts_at          timestamptz NOT NULL,
  ends_at            timestamptz NOT NULL,
  usage_limit        int CHECK (usage_limit > 0),
  per_user_limit     int NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
  used_count         int NOT NULL DEFAULT 0 CHECK (used_count >= 0),
  funded_by          text NOT NULL CHECK (funded_by IN ('PLATFORM','SHOP')),
  allow_loss         boolean NOT NULL DEFAULT false,
  status             text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','PAUSED')),
  created_by         uuid NOT NULL REFERENCES users(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at),
  CHECK (type <> 'PERCENT' OR value <= 10000),
  CHECK (usage_limit IS NULL OR used_count <= usage_limit)
);

-- ---------------------------------------------------------------------
-- Orders
-- ---------------------------------------------------------------------

CREATE TABLE orders (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code                   text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9-]{6,16}$'),
  shop_id                uuid NOT NULL REFERENCES shops(id),
  customer_id            uuid NOT NULL REFERENCES users(id),
  status                 text NOT NULL CHECK (status IN (
                           'SUBMITTED','REVISED','AWAITING_PAYMENT','PAID','PICKING','PACKED',
                           'OUT_FOR_DELIVERY','DELIVERY_FAILED','DELIVERED',
                           'REJECTED','EXPIRED','CANCELLED','CANCELLED_REFUND')),
  version                int NOT NULL DEFAULT 1,
  revision_no            int NOT NULL DEFAULT 0,
  delivery_type          text NOT NULL CHECK (delivery_type IN ('SLOT','COURIER')),
  address                jsonb NOT NULL,          -- snapshot: label, line1, landmark, phone
  lat                    numeric(9,6) NOT NULL,
  lng                    numeric(9,6) NOT NULL,
  distance_m             int NOT NULL CHECK (distance_m >= 0),
  weight_g               int NOT NULL CHECK (weight_g >= 0),
  slot_id                uuid REFERENCES slots(id),
  note                   text CHECK (char_length(note) <= 300),
  cart_ref               text CHECK (char_length(cart_ref) <= 64),
  items_cents            bigint NOT NULL CHECK (items_cents >= 0),
  delivery_fee_cents     bigint NOT NULL CHECK (delivery_fee_cents >= 0),
  service_fee_cents      bigint NOT NULL DEFAULT 0 CHECK (service_fee_cents >= 0),
  discount_cents         bigint NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  total_cents            bigint NOT NULL CHECK (total_cents >= 0),
  platform_margin_cents  bigint NOT NULL DEFAULT 0 CHECK (platform_margin_cents >= 0),
  coupon_id              uuid REFERENCES coupons(id),
  discount_funded_by     text CHECK (discount_funded_by IN ('PLATFORM','SHOP')),
  payment_provider       text NOT NULL CHECK (payment_provider IN ('PAYHERE','MANUAL')),
  driver_id              uuid REFERENCES users(id),
  delivery_code_enc      bytea,
  delivery_code_attempts int NOT NULL DEFAULT 0 CHECK (delivery_code_attempts BETWEEN 0 AND 10),
  status_reason          text CHECK (char_length(status_reason) <= 300),
  frozen_at              timestamptz,
  submitted_at           timestamptz NOT NULL DEFAULT now(),
  decided_at             timestamptz,
  paid_at                timestamptz,
  picking_at             timestamptz,
  packed_at              timestamptz,
  delivered_at           timestamptz,
  closed_at              timestamptz,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),
  CHECK (total_cents = items_cents + delivery_fee_cents + service_fee_cents - discount_cents),
  CHECK (discount_cents <= items_cents),
  CHECK ((coupon_id IS NULL) = (discount_funded_by IS NULL)),
  CHECK (delivery_type <> 'SLOT' OR slot_id IS NOT NULL)
);
CREATE INDEX orders_shop_status ON orders (shop_id, status, updated_at DESC);
CREATE INDEX orders_customer ON orders (customer_id, created_at DESC);
CREATE INDEX orders_driver ON orders (driver_id, status) WHERE driver_id IS NOT NULL;
CREATE INDEX orders_slot ON orders (slot_id);
CREATE TRIGGER orders_touch BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE order_items (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id           uuid NOT NULL REFERENCES orders(id),
  shop_id            uuid NOT NULL REFERENCES shops(id),
  shop_product_id    uuid NOT NULL REFERENCES shop_products(id),
  product_id         uuid NOT NULL REFERENCES products(id),
  name               text NOT NULL CHECK (char_length(name) <= 160),
  unit               text NOT NULL CHECK (unit IN ('PCS','KG','G','L','ML')),
  weight_g           int NOT NULL CHECK (weight_g >= 0),
  qty                numeric(10,3) NOT NULL CHECK (qty > 0 AND qty <= 999),
  mrp_cents          bigint,
  shop_price_cents   bigint NOT NULL CHECK (shop_price_cents > 0),
  online_price_cents bigint NOT NULL CHECK (online_price_cents > 0),
  line_cents         bigint NOT NULL CHECK (line_cents >= 0),
  sub_pref           text NOT NULL DEFAULT 'REMOVE' CHECK (sub_pref IN ('REMOVE','SIMILAR','CALL_ME')),
  state              text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','REMOVED','SUBSTITUTE','MISSING')),
  replaces_item_id   uuid REFERENCES order_items(id),
  revision_no        int NOT NULL DEFAULT 0,
  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_items_order ON order_items (order_id);

CREATE TABLE order_events (
  id          bigserial PRIMARY KEY,
  order_id    uuid NOT NULL REFERENCES orders(id),
  shop_id     uuid NOT NULL REFERENCES shops(id),
  from_status text,
  to_status   text NOT NULL,
  actor_id    uuid REFERENCES users(id),
  actor_role  text NOT NULL CHECK (actor_role IN ('SUPER_ADMIN','SHOP_OWNER','SHOP_STAFF','SHOP_DRIVER','PLATFORM_DRIVER','CUSTOMER','SYSTEM')),
  reason      text CHECK (char_length(reason) <= 300),
  at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_events_order ON order_events (order_id, at);
CREATE TRIGGER order_events_append_only BEFORE UPDATE OR DELETE ON order_events FOR EACH ROW EXECUTE FUNCTION app.block_change();

CREATE TABLE coupon_redemptions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  coupon_id    uuid NOT NULL REFERENCES coupons(id),
  order_id     uuid NOT NULL UNIQUE REFERENCES orders(id),
  user_id      uuid NOT NULL REFERENCES users(id),
  amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
  reserved_at  timestamptz NOT NULL DEFAULT now(),
  confirmed_at timestamptz
);
CREATE INDEX coupon_redemptions_user ON coupon_redemptions (coupon_id, user_id);

CREATE TABLE payments (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     uuid NOT NULL REFERENCES orders(id),
  shop_id      uuid NOT NULL REFERENCES shops(id),
  provider     text NOT NULL CHECK (provider IN ('PAYHERE','MANUAL')),
  account_id   uuid REFERENCES payment_accounts(id),
  amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
  currency     text NOT NULL DEFAULT 'LKR' CHECK (currency = 'LKR'),
  status       text NOT NULL DEFAULT 'INITIATED' CHECK (status IN ('INITIATED','SUCCESS','FAILED','CANCELLED','CHARGEBACK')),
  provider_ref text CHECK (char_length(provider_ref) <= 100),
  method       text CHECK (char_length(method) <= 30),
  raw          jsonb NOT NULL DEFAULT '{}'::jsonb,  -- sanitised: never card data
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX payments_provider_ref ON payments (provider, provider_ref) WHERE provider_ref IS NOT NULL;
CREATE INDEX payments_order ON payments (order_id);
CREATE TRIGGER payments_touch BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE refunds (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id      uuid NOT NULL REFERENCES orders(id),
  shop_id       uuid NOT NULL REFERENCES shops(id),
  payment_id    uuid REFERENCES payments(id),
  type          text NOT NULL CHECK (type IN ('FULL','PARTIAL','LATE_PAYMENT')),
  order_item_id uuid REFERENCES order_items(id),
  amount_cents  bigint NOT NULL CHECK (amount_cents > 0),
  reason        text NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 300),
  status        text NOT NULL DEFAULT 'REQUESTED' CHECK (status IN ('REQUESTED','APPROVED','REJECTED','PAID_OUT')),
  method        text NOT NULL DEFAULT 'MANUAL' CHECK (method IN ('MANUAL','PROVIDER')),
  reference     text CHECK (char_length(reference) <= 100),
  requested_by  uuid REFERENCES users(id),
  decided_by    uuid REFERENCES users(id),
  decided_at    timestamptz,
  paid_out_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (type <> 'PARTIAL' OR order_item_id IS NOT NULL)
);
CREATE INDEX refunds_status ON refunds (status, created_at);
CREATE INDEX refunds_order ON refunds (order_id);

CREATE TABLE support_tickets (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id    uuid NOT NULL REFERENCES orders(id),
  shop_id     uuid NOT NULL REFERENCES shops(id),
  user_id     uuid NOT NULL REFERENCES users(id),
  reason      text NOT NULL CHECK (reason IN ('MISSING_ITEM','DAMAGED','WRONG_ITEM','LATE','OTHER')),
  note        text CHECK (char_length(note) <= 500),
  status      text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','RESOLVED')),
  resolution  text CHECK (char_length(resolution) <= 500),
  created_at  timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz
);
CREATE INDEX support_tickets_open ON support_tickets (status, created_at);

-- ---------------------------------------------------------------------
-- Money: ledger and payouts (append-only ledger)
-- ---------------------------------------------------------------------

CREATE TABLE payouts (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id),
  period_from date NOT NULL,
  period_to   date NOT NULL,
  amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
  status      text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PAID')),
  reference   text CHECK (char_length(reference) <= 100),
  created_by  uuid NOT NULL REFERENCES users(id),
  paid_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (period_to >= period_from),
  CHECK (status <> 'PAID' OR (reference IS NOT NULL AND paid_at IS NOT NULL))
);

CREATE TABLE ledger_entries (
  id           bigserial PRIMARY KEY,
  shop_id      uuid NOT NULL REFERENCES shops(id),
  order_id     uuid REFERENCES orders(id),
  refund_id    uuid REFERENCES refunds(id),
  payout_id    uuid REFERENCES payouts(id),
  type         text NOT NULL CHECK (type IN ('SALE','PLATFORM_MARGIN','SERVICE_FEE','DELIVERY_FEE','DISCOUNT','GATEWAY_FEE','REFUND','PAYOUT','ADJUSTMENT')),
  party        text NOT NULL CHECK (party IN ('PLATFORM','SHOP')),
  amount_cents bigint NOT NULL,  -- signed
  note         text CHECK (char_length(note) <= 200),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_shop_time ON ledger_entries (shop_id, created_at);
CREATE INDEX ledger_order ON ledger_entries (order_id);
CREATE TRIGGER ledger_append_only BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION app.block_change();

-- ---------------------------------------------------------------------
-- Customer history, notifications, stats, audit, infra
-- ---------------------------------------------------------------------

CREATE TABLE customer_products (
  user_id        uuid NOT NULL REFERENCES users(id),
  shop_id        uuid NOT NULL REFERENCES shops(id),
  product_id     uuid NOT NULL REFERENCES products(id),
  times          int NOT NULL DEFAULT 1 CHECK (times > 0),
  last_bought_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, shop_id, product_id)
);
CREATE INDEX customer_products_recent ON customer_products (user_id, shop_id, last_bought_at DESC);

CREATE TABLE notifications (
  id           bigserial PRIMARY KEY,
  shop_id      uuid REFERENCES shops(id),
  user_id      uuid REFERENCES users(id),
  event_id     text CHECK (char_length(event_id) <= 40),
  channel      text NOT NULL CHECK (channel IN ('SMS','WHATSAPP','WA_LINK')),
  template     text NOT NULL CHECK (char_length(template) <= 60),
  to_masked    text NOT NULL CHECK (char_length(to_masked) <= 20),
  status       text NOT NULL CHECK (status IN ('SENT','FAILED','SKIPPED','LINK')),
  provider_ref text CHECK (char_length(provider_ref) <= 100),
  cost_units   int NOT NULL DEFAULT 0,
  error        text CHECK (char_length(error) <= 300),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_day ON notifications (created_at);
CREATE INDEX notifications_shop ON notifications (shop_id, created_at);

CREATE TABLE shop_daily_stats (
  shop_id               uuid NOT NULL REFERENCES shops(id),
  day                   date NOT NULL,  -- Asia/Colombo date
  orders_submitted      int NOT NULL DEFAULT 0,
  orders_paid           int NOT NULL DEFAULT 0,
  orders_delivered      int NOT NULL DEFAULT 0,
  orders_rejected       int NOT NULL DEFAULT 0,
  orders_expired        int NOT NULL DEFAULT 0,
  orders_cancelled      int NOT NULL DEFAULT 0,
  gmv_cents             bigint NOT NULL DEFAULT 0,
  shop_items_cents      bigint NOT NULL DEFAULT 0,
  platform_income_cents bigint NOT NULL DEFAULT 0,
  discount_cents        bigint NOT NULL DEFAULT 0,
  review_seconds_sum    bigint NOT NULL DEFAULT 0,
  review_count          int NOT NULL DEFAULT 0,
  pick_seconds_sum      bigint NOT NULL DEFAULT 0,
  pick_count            int NOT NULL DEFAULT 0,
  PRIMARY KEY (shop_id, day)
);

CREATE TABLE audit_log (
  id         bigserial PRIMARY KEY,
  actor_id   uuid REFERENCES users(id),
  actor_role text NOT NULL,
  action     text NOT NULL CHECK (char_length(action) <= 60),
  entity     text NOT NULL CHECK (char_length(entity) <= 40),
  entity_id  text CHECK (char_length(entity_id) <= 64),
  shop_id    uuid REFERENCES shops(id),
  before     jsonb,
  after      jsonb,
  ip_hash    bytea,
  at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_entity ON audit_log (entity, entity_id, at DESC);
CREATE INDEX audit_log_time ON audit_log (at DESC);
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log FOR EACH ROW EXECUTE FUNCTION app.block_change();

CREATE TABLE outbox (
  id         text PRIMARY KEY CHECK (id ~ '^evt_[0-9A-Z]{26}$'),
  type       text NOT NULL CHECK (char_length(type) <= 40),
  shop_id    uuid REFERENCES shops(id),
  entity_id  text CHECK (char_length(entity_id) <= 64),
  payload    jsonb NOT NULL DEFAULT '{}'::jsonb,
  attempts   int NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  sent_at    timestamptz
);
CREATE INDEX outbox_unsent ON outbox (created_at) WHERE sent_at IS NULL;

CREATE TABLE processed_events (
  event_id text NOT NULL,
  consumer text NOT NULL,
  at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, consumer)
);

CREATE TABLE idempotency_keys (
  user_id     uuid NOT NULL REFERENCES users(id),
  route       text NOT NULL CHECK (char_length(route) <= 80),
  key         text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{16,64}$'),
  body_hash   bytea NOT NULL,
  status_code int,
  response    jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, route, key)
);

-- ---------------------------------------------------------------------
-- Row-level security (safety net — the app also scopes every query)
-- ---------------------------------------------------------------------

ALTER TABLE users             ENABLE ROW LEVEL SECURITY;
ALTER TABLE addresses         ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships       ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders            ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items       ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_events      ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments          ENABLE ROW LEVEL SECURITY;
ALTER TABLE refunds           ENABLE ROW LEVEL SECURITY;
ALTER TABLE support_tickets   ENABLE ROW LEVEL SECURITY;
ALTER TABLE coupon_redemptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_products ENABLE ROW LEVEL SECURITY;
ALTER TABLE ledger_entries    ENABLE ROW LEVEL SECURITY;
ALTER TABLE payouts           ENABLE ROW LEVEL SECURITY;
ALTER TABLE shop_daily_stats  ENABLE ROW LEVEL SECURITY;
ALTER TABLE connectors        ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_runs    ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_issues       ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_accounts  ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions          ENABLE ROW LEVEL SECURITY;
ALTER TABLE admin_totp        ENABLE ROW LEVEL SECURITY;
ALTER TABLE otp_codes         ENABLE ROW LEVEL SECURITY;
ALTER TABLE invites           ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications     ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log         ENABLE ROW LEVEL SECURITY;

-- A user row is visible to: the user, a shop for its own customers (through visible orders)
-- and its own team, an assigned driver (through visible orders), and privileged scope.
CREATE POLICY p_users ON users
  USING (app.privileged() OR id = app.uid()
         OR EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = users.id)
         OR EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND m.shop_id = app.shop()))
  WITH CHECK (app.privileged() OR id = app.uid());

CREATE POLICY p_addresses ON addresses
  USING (app.privileged() OR user_id = app.uid())
  WITH CHECK (app.privileged() OR user_id = app.uid());

CREATE POLICY p_memberships ON memberships
  USING (app.privileged() OR user_id = app.uid() OR shop_id = app.shop())
  WITH CHECK (app.privileged() OR shop_id = app.shop());

CREATE POLICY p_orders ON orders
  USING (app.privileged() OR customer_id = app.uid() OR shop_id = app.shop() OR driver_id = app.uid())
  WITH CHECK (app.privileged() OR customer_id = app.uid() OR shop_id = app.shop() OR driver_id = app.uid());

-- Child tables follow the parent order's policy (the subquery is itself filtered by RLS).
CREATE POLICY p_order_items ON order_items
  USING (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = order_items.order_id))
  WITH CHECK (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = order_items.order_id));

CREATE POLICY p_order_events ON order_events
  USING (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = order_events.order_id))
  WITH CHECK (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = order_events.order_id));

CREATE POLICY p_payments ON payments
  USING (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = payments.order_id))
  WITH CHECK (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = payments.order_id AND o.customer_id = app.uid()));

CREATE POLICY p_refunds ON refunds
  USING (app.privileged() OR EXISTS (SELECT 1 FROM orders o WHERE o.id = refunds.order_id))
  WITH CHECK (app.privileged() OR shop_id = app.shop());

CREATE POLICY p_support_tickets ON support_tickets
  USING (app.privileged() OR user_id = app.uid() OR shop_id = app.shop())
  WITH CHECK (app.privileged() OR user_id = app.uid());

CREATE POLICY p_coupon_redemptions ON coupon_redemptions
  USING (app.privileged() OR user_id = app.uid())
  WITH CHECK (app.privileged() OR user_id = app.uid());

CREATE POLICY p_customer_products ON customer_products
  USING (app.privileged() OR user_id = app.uid())
  WITH CHECK (app.privileged());

CREATE POLICY p_ledger ON ledger_entries
  USING (app.privileged() OR shop_id = app.shop())
  WITH CHECK (app.privileged());

CREATE POLICY p_payouts ON payouts
  USING (app.privileged() OR shop_id = app.shop())
  WITH CHECK (app.privileged());

CREATE POLICY p_stats ON shop_daily_stats
  USING (app.privileged() OR shop_id = app.shop())
  WITH CHECK (app.privileged());

-- Admin / system only.
CREATE POLICY p_connectors      ON connectors       USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_connector_runs  ON connector_runs   USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_sync_issues     ON sync_issues      USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_payment_accounts ON payment_accounts USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_sessions        ON sessions         USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_admin_totp      ON admin_totp       USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_otp_codes       ON otp_codes        USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_invites         ON invites          USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_notifications   ON notifications    USING (app.privileged()) WITH CHECK (app.privileged());
CREATE POLICY p_audit_log       ON audit_log        USING (app.privileged()) WITH CHECK (true);

-- ---------------------------------------------------------------------
-- Grants for the application role
-- ---------------------------------------------------------------------

GRANT USAGE ON SCHEMA public, app TO app_rw;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA app TO app_rw;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_rw;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_rw;

-- Append-only tables: the app may only read and insert.
REVOKE UPDATE, DELETE ON ledger_entries, audit_log, order_events, price_changes FROM app_rw;
