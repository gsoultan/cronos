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

-- Shipments exist so that billing-summary's `region` filter binds to a dataset
-- that is real. The report does not read it, which is the point: a filter that
-- names a dataset no block here uses is announced as not applying, and that is
-- the behaviour the viewer has to get right. Binding it to a name nobody had
-- made the demo unpublishable — the loader accepted it and the management API
-- refused it, which is two answers to the same question.
CREATE TABLE shipments (
  id TEXT PRIMARY KEY, customer_id TEXT, dispatched_at TEXT,
  region TEXT, weight_kg REAL, status TEXT
);

INSERT INTO shipments VALUES
  ('s-01','c-1','2026-07-03','Benelux',    1840.0,'delivered'),
  ('s-02','c-1','2026-07-28','Benelux',     920.5,'delivered'),
  ('s-03','c-2','2026-07-15','Baltic',     3100.0,'in transit'),
  ('s-04','c-2','2026-08-09','Baltic',     2450.0,'delivered'),
  ('s-05','c-3','2026-07-22','UK & IE',     640.0,'delivered'),
  ('s-06','c-3','2026-08-16','UK & IE',    1180.0,'in transit');

-- The parcel network, for the maps. Depots are real cities; everything else is
-- invented, and shaped to look like something a logistics team would draw: a
-- scatter of deliveries around each depot, a service zone around it, trunk
-- routes between them — two of them with a ferry crossing — and the transfers
-- that move parcels from one depot to the next.
CREATE TABLE depots (
  depot TEXT PRIMARY KEY, city TEXT, region TEXT, carrier TEXT,
  lat REAL, lon REAL, capacity REAL, staff REAL
);

INSERT INTO depots VALUES
  ('RTM','Rotterdam', 'Benelux','Northline',       51.9225,  4.4792, 5200, 140),
  ('AMS','Amsterdam', 'Benelux','Northline',       52.3676,  4.9041, 3800,  96),
  ('ANR','Antwerp',   'Benelux','Northline',       51.2194,  4.4025, 2900,  71),
  ('HAM','Hamburg',   'Baltic', 'Harbour Express', 53.5511,  9.9937, 4100, 108),
  ('GDN','Gdansk',    'Baltic', 'Harbour Express', 54.3520, 18.6466, 3100,  80),
  ('RIX','Riga',      'Baltic', 'Harbour Express', 56.9496, 24.1052, 2100,  52),
  ('TLL','Tallinn',   'Baltic', 'Harbour Express', 59.4370, 24.7536, 1500,  38),
  ('BRS','Bristol',   'UK & IE','Swift Parcel',    51.4545, -2.5879, 2600,  64),
  ('LON','London',    'UK & IE','Swift Parcel',    51.5074, -0.1278, 6100, 170),
  ('MAN','Manchester','UK & IE','Swift Parcel',    53.4808, -2.2426, 3300,  85),
  ('DUB','Dublin',    'UK & IE','Swift Parcel',    53.3498, -6.2603, 1900,  47);

-- Nine hundred deliveries, generated rather than listed: a list would be most
-- of this file, and a hash of the row number is as repeatable as one. Each
-- drop is placed around its depot on a normal scatter — Box-Muller over two
-- hashed numbers — and one that would land in the sea is turned to face the
-- other way, because a delivery in the North Sea is the first thing anybody
-- looking at a map notices.
CREATE TABLE drops (
  id TEXT PRIMARY KEY, depot TEXT, carrier TEXT, status TEXT,
  lat REAL, lon REAL, parcels REAL, dropped_on TEXT
);

INSERT INTO drops
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 900),
k(k) AS (VALUES (1), (2), (3), (4), (5), (6)),
h AS (SELECT i, k, abs(sin(i * 12.9898 + k * 78.233)) * 43758.5453 AS x FROM n, k),
u AS (
  SELECT i,
    max(CASE k WHEN 1 THEN x - floor(x) END) AS pick,
    max(CASE k WHEN 2 THEN x - floor(x) END) AS near,
    max(CASE k WHEN 3 THEN x - floor(x) END) AS turn,
    max(CASE k WHEN 4 THEN x - floor(x) END) AS fleet,
    max(CASE k WHEN 5 THEN x - floor(x) END) AS fate,
    max(CASE k WHEN 6 THEN x - floor(x) END) AS size
  FROM h GROUP BY i
),
-- How each depot's deliveries are spread: its share of them, how far they
-- reach (degrees of latitude, one standard deviation), which way the sea is,
-- and how often a drop there fails.
spread(depot, upto, reach, sea, fails) AS (VALUES
  ('RTM', 0.16, 0.07, 285, 0.05), ('AMS', 0.27, 0.06, 45, 0.04),
  ('ANR', 0.35, 0.06, NULL, 0.06), ('HAM', 0.46, 0.08, NULL, 0.05),
  ('GDN', 0.55, 0.07, 20, 0.09), ('RIX', 0.61, 0.07, 340, 0.12),
  ('TLL', 0.65, 0.05, 0, 0.10), ('BRS', 0.72, 0.06, 290, 0.05),
  ('LON', 0.88, 0.10, NULL, 0.07), ('MAN', 0.95, 0.07, NULL, 0.06),
  ('DUB', 1.01, 0.06, 95, 0.11)
),
placed AS (
  SELECT u.*, s.*, d.carrier AS own, d.lat AS dlat, d.lon AS dlon,
    s.reach * min(sqrt(-2 * ln(0.0001 + 0.9998 * near)), 2.6) AS r,
    CASE WHEN s.sea IS NOT NULL AND cos(2 * pi() * turn - radians(s.sea)) > 0.15
      THEN 2 * pi() * turn + pi() ELSE 2 * pi() * turn END AS bearing
  FROM u
  JOIN spread s ON s.upto = (SELECT min(upto) FROM spread WHERE upto > u.pick)
  JOIN depots d ON d.depot = s.depot
)
SELECT
  printf('d-%04d', i),
  depot,
  CASE
    WHEN fleet < 0.65 THEN own
    WHEN fleet < 0.825 THEN CASE own WHEN 'Northline' THEN 'Harbour Express' ELSE 'Northline' END
    ELSE CASE own WHEN 'Swift Parcel' THEN 'Harbour Express' ELSE 'Swift Parcel' END
  END,
  CASE WHEN fate < fails THEN 'failed' WHEN fate < fails + 0.12 THEN 'in transit' ELSE 'delivered' END,
  round(dlat + r * cos(bearing), 5),
  round(dlon + r * sin(bearing) / cos(radians(dlat)), 5),
  1 + floor(size * size * 14),
  date('2026-08-01', '+' || ((i * 7) % 31) || ' days')
FROM placed;

-- The area each depot delivers to, as GeoJSON — what ST_AsGeoJSON returns in
-- PostGIS and in DuckDB's spatial extension, stored as text here because
-- SQLite has neither.
CREATE TABLE zones (zone TEXT PRIMARY KEY, depot TEXT, shape TEXT, parcels REAL, on_time REAL);

INSERT INTO zones VALUES
  ('Rotterdam zone','RTM','{"type":"Polygon","coordinates":[[[4.4792,52.1378],[4.419,52.085],[4.3953,52.03],[4.3893,51.992],[4.3769,51.9728],[4.3509,51.9606],[4.3206,51.9448],[4.2987,51.9225],[4.2917,51.8961],[4.2958,51.868],[4.3035,51.8361],[4.3176,51.7975],[4.35,51.7571],[4.4066,51.7264],[4.4792,51.7189],[4.5605,51.7029],[4.6619,51.6885],[4.7649,51.7016],[4.8254,51.7522],[4.8229,51.8204],[4.7894,51.8788],[4.775,51.9225],[4.7913,51.9664],[4.8051,52.0193],[4.7843,52.0726],[4.7294,52.116],[4.6568,52.15],[4.5704,52.169],[4.4792,52.1378]]]}',
   41200, 96.1),
  ('Amsterdam zone','AMS','{"type":"Polygon","coordinates":[[[4.9041,52.4978],[4.8474,52.5194],[4.7786,52.5267],[4.7168,52.511],[4.6861,52.4738],[4.6621,52.4388],[4.604,52.4094],[4.5321,52.3676],[4.4993,52.3112],[4.5341,52.2588],[4.6139,52.2263],[4.6975,52.2094],[4.7683,52.1954],[4.8351,52.1829],[4.9041,52.1794],[4.9725,52.1846],[5.0447,52.1893],[5.1317,52.1933],[5.2017,52.2227],[5.2045,52.2793],[5.1524,52.333],[5.0815,52.3676],[5.0303,52.3852],[5.0077,52.3981],[4.9991,52.4139],[4.9894,52.4329],[4.9716,52.4531],[4.944,52.4744],[4.9041,52.4978]]]}',
   30800, 95.4),
  ('Antwerp zone','ANR','{"type":"Polygon","coordinates":[[[4.4025,51.4419],[4.3245,51.4336],[4.2515,51.4158],[4.1913,51.3853],[4.1598,51.3406],[4.1582,51.2931],[4.1584,51.2543],[4.1301,51.2194],[4.0822,51.1736],[4.0605,51.1162],[4.0976,51.0671],[4.1793,51.0441],[4.2665,51.0425],[4.3386,51.0442],[4.4025,51.0419],[4.4666,51.0436],[4.5286,51.0554],[4.5895,51.0725],[4.6591,51.0912],[4.7336,51.1195],[4.781,51.1653],[4.7687,51.2194],[4.7048,51.2626],[4.6359,51.2898],[4.5969,51.3165],[4.5775,51.3568],[4.5436,51.4029],[4.4808,51.4341],[4.4025,51.4419]]]}',
   22400, 93.8),
  ('Hamburg zone','HAM','{"type":"Polygon","coordinates":[[[9.9937,53.8863],[9.8709,53.8707],[9.7768,53.8186],[9.7269,53.7498],[9.6892,53.6954],[9.6121,53.6603],[9.4941,53.6188],[9.401,53.5511],[9.3982,53.4704],[9.4827,53.4049],[9.5973,53.3633],[9.7003,53.3325],[9.7942,53.3049],[9.8928,53.2884],[9.9937,53.2863],[10.0956,53.2857],[10.215,53.2781],[10.3566,53.2807],[10.4788,53.3213],[10.522,53.3999],[10.4787,53.4853],[10.4109,53.5511],[10.3828,53.6039],[10.3926,53.6652],[10.3869,53.7374],[10.33,53.8016],[10.2324,53.8455],[10.1175,53.8733],[9.9937,53.8863]]]}',
   33900, 94.9),
  ('Gdansk zone','GDN','{"type":"Polygon","coordinates":[[[18.6466,54.4732],[18.5932,54.4884],[18.5263,54.4976],[18.4441,54.5],[18.3543,54.4878],[18.2833,54.454],[18.2764,54.4012],[18.2932,54.352],[18.2718,54.3021],[18.2303,54.2352],[18.2299,54.1583],[18.3058,54.1029],[18.43,54.0899],[18.5502,54.1059],[18.6466,54.1224],[18.7336,54.1297],[18.8216,54.1402],[18.9037,54.1641],[18.98,54.197],[19.0632,54.2351],[19.1078,54.2907],[19.0712,54.352],[18.9752,54.3957],[18.8622,54.4125],[18.7789,54.4135],[18.7354,54.4169],[18.7119,54.4311],[18.6859,54.4524],[18.6466,54.4732]]]}',
   24100, 91.2),
  ('Riga zone','RIX','{"type":"Polygon","coordinates":[[[24.1052,57.0737],[24.0586,57.061],[24.0157,57.0509],[23.9758,57.0381],[23.9429,57.0202],[23.9095,57.001],[23.8461,56.9819],[23.7306,56.9496],[23.5969,56.8863],[23.5594,56.8063],[23.6547,56.7537],[23.787,56.732],[23.9059,56.7239],[24.0066,56.714],[24.1052,56.7068],[24.2043,56.7127],[24.3013,56.7275],[24.4088,56.742],[24.5348,56.7628],[24.6454,56.8077],[24.68,56.878],[24.62,56.9496],[24.5197,57.0012],[24.4302,57.0349],[24.365,57.0626],[24.3076,57.088],[24.2358,57.0975],[24.1635,57.089],[24.1052,57.0737]]]}',
   15300, 88.7),
  ('Tallinn zone','TLL','{"type":"Polygon","coordinates":[[[24.7536,59.5288],[24.7165,59.5197],[24.6851,59.5093],[24.6471,59.5049],[24.5759,59.5091],[24.4579,59.5094],[24.3249,59.4868],[24.2371,59.437],[24.2875,59.3829],[24.3628,59.3413],[24.4336,59.3073],[24.5051,59.2785],[24.5862,59.2603],[24.6702,59.2512],[24.7536,59.2386],[24.8523,59.2172],[24.974,59.2043],[25.0864,59.2248],[25.1405,59.2801],[25.1288,59.3451],[25.0999,59.3968],[25.1024,59.437],[25.079,59.4748],[25.0378,59.5066],[24.9723,59.5257],[24.9035,59.5326],[24.8454,59.534],[24.7968,59.5332],[24.7536,59.5288]]]}',
   9800, 90.3),
  ('Bristol zone','BRS','{"type":"Polygon","coordinates":[[[-2.5879,51.6488],[-2.6457,51.6122],[-2.6807,51.5746],[-2.6991,51.5414],[-2.7037,51.512],[-2.6986,51.4877],[-2.6946,51.4697],[-2.7065,51.4545],[-2.7426,51.4325],[-2.7904,51.3937],[-2.8171,51.3406],[-2.7971,51.291],[-2.7382,51.26],[-2.6604,51.2566],[-2.5879,51.2611],[-2.5178,51.2631],[-2.4484,51.274],[-2.3838,51.295],[-2.3167,51.3197],[-2.2407,51.3503],[-2.1768,51.396],[-2.166,51.4545],[-2.2223,51.5065],[-2.3089,51.5382],[-2.3746,51.5605],[-2.4083,51.5948],[-2.4418,51.6436],[-2.5098,51.6677],[-2.5879,51.6488]]]}',
   19700, 95.0),
  ('London zone','LON','{"type":"Polygon","coordinates":[[[-0.1278,51.8849],[-0.2619,51.8732],[-0.3716,51.8224],[-0.4317,51.7446],[-0.4626,51.6736],[-0.5225,51.6257],[-0.6355,51.5795],[-0.7471,51.5074],[-0.7756,51.4154],[-0.6996,51.336],[-0.5714,51.2872],[-0.4491,51.2566],[-0.3434,51.2287],[-0.2375,51.2083],[-0.1278,51.2049],[-0.0188,51.2102],[0.1025,51.2098],[0.2494,51.213],[0.3915,51.2496],[0.4618,51.3307],[0.4297,51.4282],[0.3454,51.5074],[0.2895,51.5667],[0.2847,51.6311],[0.2828,51.7112],[0.232,51.7882],[0.1306,51.8414],[0.0056,51.8712],[-0.1278,51.8849]]]}',
   52600, 92.6),
  ('Manchester zone','MAN','{"type":"Polygon","coordinates":[[[-2.2426,53.7536],[-2.3487,53.7575],[-2.4453,53.7313],[-2.5255,53.6919],[-2.5893,53.6453],[-2.6226,53.5897],[-2.6135,53.5312],[-2.5838,53.4808],[-2.5761,53.4355],[-2.6005,53.3782],[-2.6133,53.3049],[-2.5653,53.24],[-2.4587,53.2138],[-2.3398,53.2275],[-2.2426,53.2536],[-2.1618,53.27],[-2.0808,53.2808],[-2.0016,53.301],[-1.9324,53.3336],[-1.8656,53.3728],[-1.7943,53.4199],[-1.7436,53.4808],[-1.757,53.5468],[-1.8435,53.5952],[-1.9564,53.6166],[-2.0414,53.6309],[-2.0941,53.6643],[-2.1528,53.7149],[-2.2426,53.7536]]]}',
   26900, 94.1),
  ('Dublin zone','DUB','{"type":"Polygon","coordinates":[[[-6.2603,53.5965],[-6.347,53.5766],[-6.4074,53.5321],[-6.4425,53.4861],[-6.486,53.4573],[-6.5647,53.4373],[-6.6559,53.4037],[-6.7046,53.3498],[-6.6825,53.2923],[-6.6137,53.2482],[-6.5391,53.2171],[-6.473,53.1906],[-6.4056,53.1697],[-6.3329,53.1599],[-6.2603,53.1669],[-6.1977,53.1861],[-6.1423,53.2035],[-6.1014,53.2309],[-6.0908,53.2691],[-6.1092,53.3064],[-6.1357,53.3328],[-6.1483,53.3498],[-6.1388,53.3664],[-6.1152,53.3915],[-6.0965,53.4278],[-6.098,53.4713],[-6.1241,53.5187],[-6.1766,53.5686],[-6.2603,53.5965]]]}',
   12400, 89.5);

-- Trunk routes between depots, through the towns a lorry would pass. The
-- crossings to Ireland and to the Netherlands are ferries, so those routes
-- are MultiLineStrings: a road, a sea leg, and a road.
CREATE TABLE routes (route TEXT PRIMARY KEY, region TEXT, path TEXT, trucks REAL, parcels REAL);

INSERT INTO routes VALUES
  ('Rotterdam – Hamburg','Benelux','{"type":"LineString","coordinates":[[4.4792,51.9225],[5.1214,52.0907],[5.9699,52.2112],[8.0472,52.2799],[8.8017,53.0793],[9.9937,53.5511]]}', 64, 38400),
  ('Rotterdam – Antwerp','Benelux','{"type":"LineString","coordinates":[[4.4792,51.9225],[4.6901,51.8133],[4.776,51.5719],[4.4025,51.2194]]}', 52, 30500),
  ('Rotterdam – Amsterdam','Benelux','{"type":"LineString","coordinates":[[4.4792,51.9225],[4.3007,52.0705],[4.497,52.1601],[4.9041,52.3676]]}', 47, 27900),
  ('Hamburg – Gdansk','Baltic','{"type":"LineString","coordinates":[[9.9937,53.5511],[10.6866,53.8655],[12.0991,54.0924],[14.5528,53.4285],[16.1714,54.1944],[17.0385,54.4641],[18.6466,54.352]]}', 41, 22100),
  ('Gdansk – Riga','Baltic','{"type":"LineString","coordinates":[[18.6466,54.352],[19.4044,54.1561],[23.9036,54.8985],[23.3144,55.9349],[24.1052,56.9496]]}', 23, 11800),
  ('Riga – Tallinn','Baltic','{"type":"LineString","coordinates":[[24.1052,56.9496],[24.497,58.3859],[24.7536,59.437]]}', 14, 6900),
  ('London – Bristol','UK & IE','{"type":"LineString","coordinates":[[-0.1278,51.5074],[-0.9781,51.4543],[-1.7797,51.5558],[-2.5879,51.4545]]}', 33, 17600),
  ('London – Manchester','UK & IE','{"type":"LineString","coordinates":[[-0.1278,51.5074],[-0.7594,52.0406],[-1.8904,52.4862],[-2.1794,53.0027],[-2.2426,53.4808]]}', 38, 21200),
  ('London – Rotterdam','UK & IE','{"type":"MultiLineString","coordinates":[[[-0.1278,51.5074],[0.4691,51.7356],[1.2855,51.9446]],[[1.2855,51.9446],[4.13,51.978]],[[4.13,51.978],[4.4792,51.9225]]]}', 29, 16800),
  ('Bristol – Dublin','UK & IE','{"type":"MultiLineString","coordinates":[[[-2.5879,51.4545],[-3.1791,51.4816],[-3.9436,51.6214],[-4.9795,51.9938]],[[-4.9795,51.9938],[-6.3435,52.2546]],[[-6.3435,52.2546],[-6.4633,52.3369],[-6.2603,53.3498]]]}', 12, 5400),
  ('Manchester – Dublin','UK & IE','{"type":"MultiLineString","coordinates":[[[-2.2426,53.4808],[-2.891,53.1934],[-4.1293,53.2274],[-4.6327,53.3095]],[[-4.6327,53.3095],[-6.2603,53.3498]]]}', 9, 4100);

-- Parcels moved from one depot to another in a month. Both directions of the
-- busiest pairs, which is what the flow layer's bowed arcs are for: the two
-- would otherwise be drawn on the same line.
CREATE TABLE lanes (
  lane TEXT PRIMARY KEY, region TEXT, carrier TEXT,
  from_lat REAL, from_lon REAL, to_lat REAL, to_lon REAL, parcels REAL
);

INSERT INTO lanes VALUES
  ('Rotterdam → Hamburg', 'Benelux','Northline',       51.9225,  4.4792, 53.5511,  9.9937, 1840),
  ('Hamburg → Rotterdam', 'Baltic', 'Northline',       53.5511,  9.9937, 51.9225,  4.4792,  960),
  ('Antwerp → Rotterdam', 'Benelux','Northline',       51.2194,  4.4025, 51.9225,  4.4792, 1370),
  ('Amsterdam → Hamburg', 'Benelux','Northline',       52.3676,  4.9041, 53.5511,  9.9937,  640),
  ('Hamburg → Gdansk',    'Baltic', 'Harbour Express', 53.5511,  9.9937, 54.3520, 18.6466, 1220),
  ('Gdansk → Riga',       'Baltic', 'Harbour Express', 54.3520, 18.6466, 56.9496, 24.1052,  690),
  ('Riga → Tallinn',      'Baltic', 'Harbour Express', 56.9496, 24.1052, 59.4370, 24.7536,  380),
  ('Rotterdam → London',  'Benelux','Swift Parcel',    51.9225,  4.4792, 51.5074, -0.1278, 2080),
  ('London → Rotterdam',  'UK & IE','Swift Parcel',    51.5074, -0.1278, 51.9225,  4.4792, 1290),
  ('London → Manchester', 'UK & IE','Swift Parcel',    51.5074, -0.1278, 53.4808, -2.2426,  910),
  ('London → Bristol',    'UK & IE','Northline',       51.5074, -0.1278, 51.4545, -2.5879,  780),
  ('Bristol → Dublin',    'UK & IE','Swift Parcel',    51.4545, -2.5879, 53.3498, -6.2603,  440),
  ('Manchester → Dublin', 'UK & IE','Harbour Express', 53.4808, -2.2426, 53.3498, -6.2603,  310);

-- Two hundred thousand van positions: every van on the road in August,
-- reporting where it was every few minutes. The map of these is the one no
-- browser can hold one by one — see "A million places" in
-- docs/report-format.md. Placed like the drops, by a hash of the row number
-- and a spread around each depot, but further out: a van drives between drops,
-- and the busiest roads are the ones near the depot it loads at.
CREATE TABLE pings (
  id INTEGER PRIMARY KEY, depot TEXT, carrier TEXT, lat REAL, lon REAL, speed REAL
);

INSERT INTO pings
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 200000),
f AS (
  SELECT i,
    abs(sin(i * 12.9898) * 43758.5453) AS a,
    abs(sin(i * 78.233 + 1.7) * 24634.6345) AS b
  FROM n
),
-- Each depot's share of the vans, as a CASE rather than a lookup: a subquery
-- per row made this twelve seconds of every development boot.
v AS (
  SELECT i, a - floor(a) AS pick, b - floor(b) AS near,
    a * 97.13 - floor(a * 97.13) AS turn, b * 89.71 - floor(b * 89.71) AS pace
  FROM f
),
w AS (
  SELECT i, near, turn, pace,
    CASE WHEN pick < 0.17 THEN 'RTM' WHEN pick < 0.31 THEN 'AMS' WHEN pick < 0.40 THEN 'ANR'
         WHEN pick < 0.52 THEN 'HAM' WHEN pick < 0.59 THEN 'GDN' WHEN pick < 0.64 THEN 'RIX'
         WHEN pick < 0.67 THEN 'TLL' WHEN pick < 0.74 THEN 'BRS' WHEN pick < 0.89 THEN 'LON'
         WHEN pick < 0.95 THEN 'MAN' ELSE 'DUB' END AS depot,
    -- How far its vans range: degrees of latitude, one standard deviation.
    CASE WHEN pick < 0.17 THEN 0.30 WHEN pick < 0.31 THEN 0.28 WHEN pick < 0.40 THEN 0.25
         WHEN pick < 0.52 THEN 0.35 WHEN pick < 0.64 THEN 0.30 WHEN pick < 0.67 THEN 0.22
         WHEN pick < 0.74 THEN 0.28 WHEN pick < 0.89 THEN 0.40 WHEN pick < 0.95 THEN 0.30
         ELSE 0.25 END AS reach
  FROM v
),
placed AS (
  SELECT w.i, w.depot, w.pace, dp.carrier, dp.lat AS dlat, dp.lon AS dlon,
    w.reach * min(sqrt(-2 * ln(0.0001 + 0.9998 * w.near)), 3.0) AS r,
    2 * pi() * w.turn AS bearing
  FROM w JOIN depots dp ON dp.depot = w.depot
)
SELECT i, depot, carrier,
  round(dlat + r * cos(bearing), 5),
  round(dlon + r * sin(bearing) / cos(radians(dlat)), 5),
  -- Slow near the depot and in town, faster out on the road.
  round(12 + 70 * pace * min(r / 0.4, 1), 1)
FROM placed;
