-- The demo warehouse. Small enough to read, varied enough that the row-scope
-- and filter behaviour is visible rather than theoretical.
CREATE TABLE customers (id TEXT PRIMARY KEY, name TEXT, city TEXT);
CREATE TABLE invoices (
  id TEXT PRIMARY KEY, customer_id TEXT, issued_at TEXT,
  currency TEXT, total REAL, status TEXT
);

INSERT INTO customers VALUES
  ('c-1','Aurora Freight','Rotterdam'),
  ('c-2','Baltic Cold Chain','Gdansk'),
  ('c-3','Cedar & Vine Foods','Bristol');

INSERT INTO invoices VALUES
  ('i-01','c-1','2026-05-04','EUR', 12400.00,'paid'),
  ('i-02','c-1','2026-06-11','EUR',  8250.50,'paid'),
  ('i-03','c-1','2026-07-02','EUR', 19800.00,'sent'),
  ('i-04','c-1','2026-07-26','EUR',  4120.75,'overdue'),
  ('i-05','c-1','2026-08-03','EUR', 15600.00,'sent'),
  ('i-06','c-2','2026-05-19','EUR', 31000.00,'paid'),
  ('i-07','c-2','2026-06-23','EUR',  9400.00,'overdue'),
  ('i-08','c-2','2026-07-14','EUR', 27350.25,'sent'),
  ('i-09','c-2','2026-08-08','EUR', 11200.00,'overdue'),
  ('i-10','c-3','2026-06-02','EUR',  5300.00,'paid'),
  ('i-11','c-3','2026-07-21','EUR',  7750.00,'sent'),
  ('i-12','c-3','2026-08-15','EUR',  2480.00,'draft');
