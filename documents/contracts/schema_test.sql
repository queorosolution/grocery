-- Security and constraint tests for contracts/schema.sql.
-- Run on a FRESH database right after schema.sql, as the owner role:
--   psql -d test -f contracts/schema.sql && psql -d test -f contracts/schema_test.sql
-- Expected last line: passed = 30, failed = 0.
\set ON_ERROR_STOP 1
-- Seed as owner (bypasses RLS)
INSERT INTO users (id, google_sub, email, name) VALUES
 ('00000000-0000-0000-0000-0000000000a1','g-a1','custa@x.lk','Cust A'),
 ('00000000-0000-0000-0000-0000000000b1','g-b1','custb@x.lk','Cust B'),
 ('00000000-0000-0000-0000-0000000000a2','g-a2','ownera@x.lk','Owner A'),
 ('00000000-0000-0000-0000-0000000000d1','g-d1','drv@x.lk','Driver A');
INSERT INTO shops (id, slug, name, lat, lng, status) VALUES
 ('00000000-0000-0000-0000-00000000aaaa','krishna-super','Krishna Super',7.29,80.63,'ACTIVE'),
 ('00000000-0000-0000-0000-00000000bbbb','other-shop','Other Shop',7.30,80.64,'ACTIVE');
INSERT INTO memberships (user_id, scope, shop_id, role) VALUES
 ('00000000-0000-0000-0000-0000000000a2','SHOP','00000000-0000-0000-0000-00000000aaaa','SHOP_OWNER');
INSERT INTO categories (id, slug, name) VALUES ('00000000-0000-0000-0000-0000000000c1','rice','Rice');
INSERT INTO products (id, barcode, name, brand, keywords, category_id) VALUES
 ('00000000-0000-0000-0000-0000000000f1','4791234567890','Samba Rice 5kg','Araliya','haal sahal','00000000-0000-0000-0000-0000000000c1');
INSERT INTO shop_products (id, shop_id, product_id, mrp_cents, shop_price_cents, online_price_cents) VALUES
 ('00000000-0000-0000-0000-0000000005a1','00000000-0000-0000-0000-00000000aaaa','00000000-0000-0000-0000-0000000000f1',150000,140000,143500),
 ('00000000-0000-0000-0000-0000000005b1','00000000-0000-0000-0000-00000000bbbb','00000000-0000-0000-0000-0000000000f1',150000,139000,142000);
INSERT INTO slots (id, shop_id, starts_at, ends_at, cutoff_at, capacity) VALUES
 ('00000000-0000-0000-0000-0000000051a1','00000000-0000-0000-0000-00000000aaaa', now()+interval '5 hour', now()+interval '7 hour', now()+interval '3 hour', 1),
 ('00000000-0000-0000-0000-0000000051b1','00000000-0000-0000-0000-00000000bbbb', now()+interval '5 hour', now()+interval '7 hour', now()+interval '3 hour', 5);
INSERT INTO orders (id, code, shop_id, customer_id, status, delivery_type, address, lat, lng, distance_m, weight_g, slot_id,
  items_cents, delivery_fee_cents, total_cents, payment_provider, driver_id) VALUES
 ('00000000-0000-0000-0000-0000000000e1','KS-AAAA01','00000000-0000-0000-0000-00000000aaaa','00000000-0000-0000-0000-0000000000a1','SUBMITTED','SLOT','{}',7.29,80.63,1200,5000,'00000000-0000-0000-0000-0000000051a1',143500,15000,158500,'PAYHERE','00000000-0000-0000-0000-0000000000d1'),
 ('00000000-0000-0000-0000-0000000000e2','OS-BBBB01','00000000-0000-0000-0000-00000000bbbb','00000000-0000-0000-0000-0000000000b1','SUBMITTED','SLOT','{}',7.30,80.64,1200,5000,'00000000-0000-0000-0000-0000000051b1',142000,15000,157000,'PAYHERE',NULL);
INSERT INTO order_items (order_id, shop_id, shop_product_id, product_id, name, unit, weight_g, qty, shop_price_cents, online_price_cents, line_cents) VALUES
 ('00000000-0000-0000-0000-0000000000e1','00000000-0000-0000-0000-00000000aaaa','00000000-0000-0000-0000-0000000005a1','00000000-0000-0000-0000-0000000000f1','Samba Rice 5kg','PCS',5000,1,140000,143500,143500),
 ('00000000-0000-0000-0000-0000000000e2','00000000-0000-0000-0000-00000000bbbb','00000000-0000-0000-0000-0000000005b1','00000000-0000-0000-0000-0000000000f1','Samba Rice 5kg','PCS',5000,1,139000,142000,142000);
INSERT INTO ledger_entries (shop_id, order_id, type, party, amount_cents) VALUES
 ('00000000-0000-0000-0000-00000000aaaa','00000000-0000-0000-0000-0000000000e1','SALE','SHOP',140000),
 ('00000000-0000-0000-0000-00000000bbbb','00000000-0000-0000-0000-0000000000e2','SALE','SHOP',139000);
INSERT INTO connectors (shop_id, secret_ref, host_display) VALUES
 ('00000000-0000-0000-0000-00000000aaaa','/gp/dev/connectors/x','postgres://ro:•••@ep-x.neon.tech/ks');

CREATE TEMP TABLE results (name text, ok boolean);
GRANT ALL ON results TO app_rw;

-- 1. Customer A sees only own order and its items
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000a1',true);
INSERT INTO results SELECT 'customer sees own orders only', (SELECT count(*) FROM orders) = 1 AND (SELECT code FROM orders) = 'KS-AAAA01';
INSERT INTO results SELECT 'customer sees own order items only', (SELECT count(*) FROM order_items) = 1;
INSERT INTO results SELECT 'customer cannot see ledger', (SELECT count(*) FROM ledger_entries) = 0;
INSERT INTO results SELECT 'customer cannot see connectors', (SELECT count(*) FROM connectors) = 0;
WITH u AS (UPDATE orders SET status = 'CANCELLED' WHERE code = 'OS-BBBB01' RETURNING 1)
INSERT INTO results SELECT 'customer cannot update other order', (SELECT count(*) FROM u) = 0;
COMMIT;

-- 2. Customer A cannot insert an order for another customer
DO $$
BEGIN
  SET LOCAL ROLE app_rw;
  PERFORM set_config('app.user_id','00000000-0000-0000-0000-0000000000a1',true);
  INSERT INTO orders (code, shop_id, customer_id, status, delivery_type, address, lat, lng, distance_m, weight_g, slot_id, items_cents, delivery_fee_cents, total_cents, payment_provider)
  VALUES ('XX-FAKE01','00000000-0000-0000-0000-00000000bbbb','00000000-0000-0000-0000-0000000000b1','SUBMITTED','SLOT','{}',7.3,80.6,0,0,'00000000-0000-0000-0000-0000000051b1',1,0,1,'PAYHERE');
  RESET ROLE;
  INSERT INTO results VALUES ('customer cannot insert order for someone else', false);
EXCEPTION WHEN insufficient_privilege OR others THEN
  RESET ROLE;
  INSERT INTO results VALUES ('customer cannot insert order for someone else', SQLSTATE = '42501');
END $$;

-- 3. Shop A scope sees only shop A data
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000a2',true);
SELECT set_config('app.shop_id','00000000-0000-0000-0000-00000000aaaa',true);
INSERT INTO results SELECT 'shop sees own orders only', (SELECT count(*) FROM orders) = 1 AND (SELECT shop_id FROM orders) = '00000000-0000-0000-0000-00000000aaaa';
INSERT INTO results SELECT 'shop sees own ledger only', (SELECT count(*) FROM ledger_entries) = 1;
INSERT INTO results SELECT 'shop cannot see connector secrets', (SELECT count(*) FROM connectors) = 0;
WITH u AS (UPDATE orders SET status = 'REJECTED' WHERE code = 'OS-BBBB01' RETURNING 1)
INSERT INTO results SELECT 'shop cannot update other shop order', (SELECT count(*) FROM u) = 0;
COMMIT;

-- 4. Driver sees only assigned order
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000d1',true);
INSERT INTO results SELECT 'driver sees only assigned order', (SELECT count(*) FROM orders) = 1;
COMMIT;

-- 5. No context at all sees nothing protected
BEGIN;
SET LOCAL ROLE app_rw;
INSERT INTO results SELECT 'anonymous sees no orders', (SELECT count(*) FROM orders) = 0;
INSERT INTO results SELECT 'anonymous can read public catalog', (SELECT count(*) FROM shop_products) = 2;
COMMIT;

-- 6. Privileged sees all
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.privileged','on',true);
INSERT INTO results SELECT 'privileged sees all orders', (SELECT count(*) FROM orders) = 2;
COMMIT;

-- 7. Ledger is append-only for app_rw
DO $$
BEGIN
  SET LOCAL ROLE app_rw;
  PERFORM set_config('app.privileged','on',true);
  UPDATE ledger_entries SET amount_cents = 1;
  RESET ROLE;
  INSERT INTO results VALUES ('ledger update blocked', false);
EXCEPTION WHEN others THEN
  RESET ROLE;
  INSERT INTO results VALUES ('ledger update blocked', true);
END $$;
DO $$
BEGIN
  UPDATE ledger_entries SET amount_cents = 1;  -- even the owner is blocked by trigger
  INSERT INTO results VALUES ('ledger update blocked for owner too', false);
EXCEPTION WHEN others THEN
  INSERT INTO results VALUES ('ledger update blocked for owner too', true);
END $$;

-- 8. Totals must add up
DO $$
BEGIN
  UPDATE orders SET total_cents = 1 WHERE code = 'KS-AAAA01';
  INSERT INTO results VALUES ('order total check enforced', false);
EXCEPTION WHEN check_violation THEN
  INSERT INTO results VALUES ('order total check enforced', true);
END $$;

-- 9. Atomic slot booking: capacity 1
WITH b1 AS (UPDATE slots SET booked = booked + 1 WHERE id = '00000000-0000-0000-0000-0000000051a1' AND status='OPEN' AND booked < capacity AND now() < cutoff_at RETURNING 1)
INSERT INTO results SELECT 'first booking succeeds', (SELECT count(*) FROM b1) = 1;
WITH b2 AS (UPDATE slots SET booked = booked + 1 WHERE id = '00000000-0000-0000-0000-0000000051a1' AND status='OPEN' AND booked < capacity AND now() < cutoff_at RETURNING 1)
INSERT INTO results SELECT 'second booking refused when full', (SELECT count(*) FROM b2) = 0;

-- 10. Online price cannot exceed MRP
DO $$
BEGIN
  UPDATE shop_products SET online_price_cents = 160000 WHERE id = '00000000-0000-0000-0000-0000000005a1';
  INSERT INTO results VALUES ('online price above MRP blocked', false);
EXCEPTION WHEN check_violation THEN
  INSERT INTO results VALUES ('online price above MRP blocked', true);
END $$;

-- 11. Trigram search tolerates a typo
BEGIN;
SET LOCAL pg_trgm.word_similarity_threshold = 0.4;
INSERT INTO results SELECT 'typo search finds product', (SELECT count(*) FROM products WHERE 'sambha rise' <% search_doc) = 1;
INSERT INTO results SELECT 'unrelated word finds nothing', (SELECT count(*) FROM products WHERE 'sugar' <% search_doc) = 0;
COMMIT;

-- 12. Coupon atomic reservation
INSERT INTO coupons (id, code, type, value, starts_at, ends_at, usage_limit, funded_by, created_by)
VALUES ('00000000-0000-0000-0000-00000000c0c0','WELCOME10','PERCENT',1000, now()-interval '1 day', now()+interval '1 day', 1,'PLATFORM','00000000-0000-0000-0000-0000000000a2');
WITH r1 AS (UPDATE coupons SET used_count = used_count + 1 WHERE code='WELCOME10' AND status='ACTIVE' AND (usage_limit IS NULL OR used_count < usage_limit) RETURNING 1)
INSERT INTO results SELECT 'coupon first reserve ok', (SELECT count(*) FROM r1) = 1;
WITH r2 AS (UPDATE coupons SET used_count = used_count + 1 WHERE code='WELCOME10' AND status='ACTIVE' AND (usage_limit IS NULL OR used_count < usage_limit) RETURNING 1)
INSERT INTO results SELECT 'coupon over limit refused', (SELECT count(*) FROM r2) = 0;

-- 13. Lowercase coupon code rejected
DO $$
BEGIN
  INSERT INTO coupons (code, type, value, starts_at, ends_at, funded_by, created_by)
  VALUES ('welcome', 'FIXED', 100, now(), now()+interval '1 day', 'SHOP', '00000000-0000-0000-0000-0000000000a2');
  INSERT INTO results VALUES ('coupon code must be uppercase', false);
EXCEPTION WHEN check_violation THEN
  INSERT INTO results VALUES ('coupon code must be uppercase', true);
END $$;

-- 14. Only one RUNNING connector run at a time
INSERT INTO connector_runs (connector_id, trigger) SELECT id, 'SCHEDULE' FROM connectors;
DO $$
BEGIN
  INSERT INTO connector_runs (connector_id, trigger) SELECT id, 'MANUAL' FROM connectors;
  INSERT INTO results VALUES ('connector runs never overlap', false);
EXCEPTION WHEN unique_violation THEN
  INSERT INTO results VALUES ('connector runs never overlap', true);
END $$;

-- 15. Shop staff can insert audit rows but cannot read them
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000a2',true);
SELECT set_config('app.shop_id','00000000-0000-0000-0000-00000000aaaa',true);
INSERT INTO audit_log (actor_id, actor_role, action, entity) VALUES ('00000000-0000-0000-0000-0000000000a2','SHOP_OWNER','product.price','shop_product');
INSERT INTO results SELECT 'shop cannot read audit log', (SELECT count(*) FROM audit_log) = 0;
COMMIT;


-- 16. users table: customer sees only self; shop sees its customers and team; anonymous sees nobody
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000a1',true);
INSERT INTO results SELECT 'customer sees only own user row', (SELECT count(*) FROM users) = 1;
COMMIT;
BEGIN;
SET LOCAL ROLE app_rw;
SELECT set_config('app.user_id','00000000-0000-0000-0000-0000000000a2',true);
SELECT set_config('app.shop_id','00000000-0000-0000-0000-00000000aaaa',true);
INSERT INTO results SELECT 'shop sees own customers and team only',
  (SELECT count(*) FROM users) = 2
  AND NOT EXISTS (SELECT 1 FROM users WHERE email = 'custb@x.lk');
COMMIT;
BEGIN;
SET LOCAL ROLE app_rw;
INSERT INTO results SELECT 'anonymous sees no users', (SELECT count(*) FROM users) = 0;
COMMIT;

SELECT name, CASE WHEN ok THEN 'PASS' ELSE 'FAIL' END AS result FROM results;
SELECT count(*) FILTER (WHERE ok) AS passed, count(*) FILTER (WHERE NOT ok) AS failed FROM results;
