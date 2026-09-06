# cronos — product definition

Owner: Gembit Soultan Shirazi · Status: draft for review · 2026-08-10

---

## 1. The problem

Every B2B product eventually has to show customers their own data. That request
arrives as three different jobs, and the market serves them with three different
product categories:

| The job | Who serves it today | What it costs |
| :--- | :--- | :--- |
| "Show my customers their data **inside my product**" | Embedded analytics vendors | Per external viewer — $12.50/user (Metabase) to $50/user (ThoughtSpot) |
| "Send 5,000 customers a **monthly PDF statement**" | Legacy report servers — Jasper, Crystal, SSRS | Crystal is EOL; Jasper CE was discontinued Jan 2024; commercial Jasper is six figures |
| "Let a business user **build a report** without a ticket" | Self-service BI — Metabase, Superset | Free, but does neither of the above |

No product does all three. So teams buy two tools and glue them together, or they
build it themselves — a query engine, a PDF renderer, a scheduler and a permission
model — and are still finishing it eighteen months later.

The discontinuation of JasperReports Server Community Edition on 25 January 2024
removed the only free option for the second job and stranded a large installed base
with an unpatched server. That population has no modern destination.

## 2. Vision

**The report server for products.** Install it once, embed it in your application,
and never pay per customer.

cronos is a report engine and report repository: define reports over SQL databases,
object storage and CSV, render them as interactive views, paginated PDFs or
spreadsheets, and schedule them to thousands of recipients — all from one definition,
under one flat license.

## 3. Positioning statement

> For **B2B software teams and enterprise BI owners** who need to deliver reports to
> their customers, cronos is a **self-hosted report server** that produces both
> interactive embedded analytics and pixel-perfect scheduled documents from a single
> definition. Unlike **embedded analytics vendors**, it does not charge per viewer;
> unlike **dashboard BI tools**, it produces real documents; unlike **legacy report
> servers**, it is maintained, embeddable and free to run.

## 4. Who it is for

### Priya — Platform Engineer, vertical B2B SaaS (primary buyer)

40-person company, 800 customers, Postgres and an S3 event lake.

- **Job:** "Our customers keep asking for reports. I have built three one-off
  endpoints and I am not building a fourth."
- **Pain:** quoted $40k/yr because pricing scales with her customer count — the exact
  number she is trying to grow. Building it herself means owning a query engine, a PDF
  renderer and a scheduler forever.
- **Trigger:** a contract renewal requires scheduled PDF statements.
- **Success:** customer-facing reports shipped in one sprint, then never touched again.
- **Why she stays:** her per-customer cost does not move when she doubles customers.

### Marek — BI Lead, mid-market enterprise (secondary buyer, Jasper refugee)

2,000-person logistics firm, JasperReports Server CE since 2016, ~400 definitions.

- **Job:** "My report server has had no security patches since January 2024 and the
  vendor wants six figures."
- **Pain:** 400 `.jrxml` files, and business users who navigate by folder tree and
  will revolt if it changes.
- **Trigger:** an audit flags the unpatched server.
- **Success:** reports keep running, folder structure survives, nobody retrains.
- **Why he stays:** switching cost, once migrated, runs the other way.

### Dewi — Finance Analyst (author; not a buyer, but the daily user)

- **Job:** "Change the statement layout without filing a ticket."
- **Pain:** every layout tweak is a two-week engineering task.
- **Success:** edits a report, previews it against real data, publishes it herself.
- **Why she matters:** she is why the deal renews. Priya buys it; Dewi decides
  whether it gets used.

### Sam — Priya's customer (the report consumer)

- **Job:** "See my invoices, filter by date, download the PDF."
- Never logs into cronos. Has never heard of it. **That is the product working.**

## 5. Jobs to be done

1. When a customer asks to see their data, I want to put a governed report inside my
   product, so I can answer without building a reporting stack.
2. When a billing period closes, I want to send every customer their own document, so
   nobody assembles 5,000 PDFs by hand.
3. When a business user needs a new report, I want them to build it themselves, so
   engineering is not the bottleneck.
4. When an auditor asks what a customer received in March, I want to show the exact
   definition and data that produced it, so the answer is evidence rather than memory.
5. When my customer count doubles, I want my reporting cost to stay flat, so success
   is not penalised.

Job 4 is under-served by every competitor and is the strongest enterprise wedge.

## 6. What it is not

Scope discipline matters more than feature count. cronos is **not**:

- a data warehouse, ETL tool or transformation layer — it reads, it does not model
- a general dashboarding tool — Superset and Metabase win that, for free
- real-time streaming analytics — batch and on-demand only
- ML, forecasting or predictive analytics
- a spreadsheet replacement

And explicitly out of v1: write-back, alerting and thresholds, natural-language query,
cross-dataset joins in a report, mobile native apps.

## 7. Product pillars

| Pillar | What it means | Why a competitor cannot copy it cheaply |
| :--- | :--- | :--- |
| **One definition, three outputs** | Interactive, paginated PDF and spreadsheet from one report — a dashboard is simply a report whose only output is interactive | Dashboard tools have no typesetting engine; legacy tools have no embed story. Both would need to rebuild half their product |
| **Governed by construction** | RLS lives on the dataset and is applied on every read path, including exports and scheduled bursts | Retrofitting unconditional RLS into a tool that has a "run as owner" mode is a breaking change |
| **Flat cost** | Free for internal use; commercial license priced per deployment, never per viewer | Vendors whose revenue model is per-viewer cannot follow without cutting revenue |
| **Definitions as files** | YAML, versioned, diffable, git-syncable, content-addressed | Legacy formats are proprietary; SaaS BI tools keep definitions in their database |

## 8. Epics

| # | Epic | Delivers | Persona |
| :-- | :--- | :--- | :--- |
| E1 | **Connect** | Register SQL, object-store and CSV sources with secrets, timeouts and row caps | Priya |
| E2 | **Model** | Governed datasets: typed fields, validated params, row-level security | Priya |
| E3 | **Author** | Build reports and layouts, preview against live data, publish | Dewi |
| E4 | **Render** | Interactive views, Typst PDFs, XLSX/CSV exports | All |
| E5 | **Deliver** | Cron schedules, bursting, email/S3/SFTP/webhook, retries | Marek |
| E6 | **Embed** | JWT-scoped embed API and web component for customer apps | Priya |
| E7 | **Govern** | Repository, folders, versions, permissions, tenants, audit | Marek |
| E8 | **Operate** | Run history, failure visibility, metrics, reproducibility | Priya |

### Selected stories with acceptance criteria

**E2 — governed datasets**

> As a platform engineer, I want row-level security defined once on a dataset, so no
> report can leak another tenant's data.

- A dataset's RLS predicate applies to preview, embed, export and scheduled burst
- No configuration, flag or API parameter disables it
- A report authored by tenant A, run by tenant B, returns only tenant B's rows
- A parameter value cannot alter query structure; templates resolving to SQL
  fragments are rejected at validation, not at run time

**E5 — bursting**

> As a BI lead, I want one schedule to produce one document per customer, so month-end
> is not a manual process.

- A schedule bursts over a dataset, binding each row to report parameters
- Fan-out is bounded by an explicit concurrency limit with backpressure
- Each row's run applies the dataset's RLS; a burst cannot widen access
- A partial failure delivers the successes and reports the failures individually
- Retries are per recipient, not per run

**E6 — embedding**

> As a platform engineer, I want to embed a report with a signed token, so my
> customers see only their own data without cronos knowing my user model.

- A signed token carries tenant, principal and parameter constraints
- Constraints in the token cannot be overridden by the client
- The embed bundle is ≤40 KB gzipped and carries no builder code — `packages/embed`, 3.0 KB, enforced in CI
- Theming via CSS custom properties; no cronos branding in the commercial tier

**E7 — reproducibility** *(Job 4 — the enterprise wedge)*

> As an auditor, I want to see exactly what a customer received and what produced it.

- Every run records the definition version hash, parameters and principal
- A delivered document traces to the exact definition version that rendered it
- Definition history shows author, timestamp and diff
- Audit events survive independently of the run log (EE)

## 9. Release plan

Sequenced so each release validates one persona's job, and the riskiest technical
assumption is tested first.

| Release | Theme | Scope | Validates |
| :--- | :--- | :--- | :--- |
| **v0.1** | Engine | YAML definitions, param binding, RLS, table + chart output. *Done, over `database/sql`. Federation and CSV/XLSX landed too — a join across Postgres and SQLite is asserted in CI.* | Can a governed query run fast over three source types? |
| **v0.2** | Documents | Typst paginated PDF: grouping, page breaks, subtotals, headers/footers | The moat. Hardest technical risk, taken early |
| **v0.3** | Delivery | Scheduler, bursting, email + S3, retries, run history | Marek's job |
| **v0.4** | Embed | Repository API, embed tokens, tenancy, embed web component. *Done. The store is authoritative; the directory is the bootstrap.* | **Priya's job. First sellable release.** |
| **v0.5** | Author | Portal PWA, dataset browser, report builder, live preview. *Done, and tagged — see CHANGELOG.md.* | Dewi's job — self-service |
| **v1.0** | Migrate | `.jrxml` importer, HA scheduler, SSO + audit (EE), documentation. *Released 2026-09-06 — see CHANGELOG.md. The importer covers the common 80% and reports the rest per file — docs/migrating-from-jasper.md.* | Marek's switching cost |

Realistic elapsed time to v1.0 for a small team: **9–12 months**. v0.4 is the first
release worth charging for; everything before it is validation.

## 10. Pricing and packaging

The anti-per-viewer story is the product's commercial identity. Packaging must not
undermine it.

| | **Community** | **Business** | **Enterprise** |
| :--- | :--- | :--- | :--- |
| License | BSL 1.1 | Commercial | Commercial |
| Cost | Free | Flat annual per deployment | Custom |
| Internal use | Unlimited | Unlimited | Unlimited |
| Users / viewers / rows | Unlimited | Unlimited | Unlimited |
| Embedding, white-label | ✗ (license) | ✓ | ✓ |
| Engine, repository, PDF, scheduling, bursting, multi-tenancy | ✓ | ✓ | ✓ |
| SSO/SAML/SCIM, durable audit | ✗ | ✗ | ✓ |
| Support | Community | Business hours | SLA |

**Never priced on:** viewers, end users, rows, queries, dashboards, refreshes.

Anchoring: Reveal is a flat $30–50k/yr; DataBrain is $999–1,995/mo; Metabase reaches
$10k+/yr at 800 external users. Business tier priced meaningfully below all three is
both credible and the entire pitch.

## 11. Success metrics

| Stage | Metric | Target |
| :--- | :--- | :--- |
| Activation | `docker run` → first rendered report | < 15 minutes |
| **Aha** | First scheduled document delivered to a real recipient | Within week 1 of install |
| Adoption | Published definitions per install at day 30 | ≥ 10 |
| Stickiness | Scheduled runs per week per install | ≥ 20 |
| Commercial | Installs reaching the embed path → licensing conversation | ≥ 5% |
| Retention | Installs still running schedules at day 90 | ≥ 60% |

Scheduled runs per week is the leading indicator. A team with live schedules has put
cronos in the path of a recurring business obligation, and that is very hard to remove.

## 12. Risks

| Risk | Impact | Mitigation | Owner |
| :--- | :--- | :--- | :--- |
| ~~Typst layout cannot express real statement layouts~~ **retired** | Kills the differentiator | Prototyped and under test: grouping, page breaks, repeated headings, per-group subtotals and per-recipient page numbering all hold — `docs/rendering.md` | eng |
| BSL suppresses OSS adoption | Loses the distribution channel | Community tier is genuinely complete; measure installs, not stars | product |
| Builder UI is where similar projects die | No self-service, no Dewi, no renewal | Ship the file format and API first; the builder writes YAML it cannot corrupt | product |
| ~~Jasper migration is harder than it looks~~ **retired** | v1.0 slips, Marek does not switch | Scoped to the common 80% and said plainly: the importer grades every file blocked/review/note and refuses the two constructs it would otherwise import wrong — docs/migrating-from-jasper.md | eng |
| Two personas pull the roadmap apart | Neither is served well | Priya is primary. When they conflict, Priya wins | product |
| A competitor drops per-viewer pricing | Erodes the wedge | Governance and reproducibility (Job 4) are the second moat | product |

## 13. Open questions

1. **Vertical focus.** Priya is more findable inside one vertical. Fintech, logistics
   and healthcare all have contractual document reporting. Picking one sharpens the
   first ten design partners.
2. **Cloud offering.** Self-host only keeps the story simple and margins clean; a
   hosted tier shortens time-to-value but adds infrastructure the team must run.
3. **`.jrxml` import against a real estate.** It is built and tested against the
   shapes a report takes, but no four-hundred-file estate has been through it.
   The number to learn from the first one is what fraction comes back *blocked*:
   the design bets that subreports are the common blocker, and that is a guess
   until somebody runs it.
4. **Design partners.** Nothing here is proven. Three Priyas and one Marek, committed
   before v0.4, are worth more than any amount of further specification.
