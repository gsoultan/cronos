# Tenancy

cronos scopes at three levels. They are separate mechanisms with separate
failure modes, and collapsing any two of them is the most expensive mistake
available in this codebase.

```
Organization ──1:N── Project ──1:N── DataSource · Dataset · Report · Schedule
     │                   │
     └── M:N ── User ── M:N ──┘
        org_members    project_members
```

| Level | Answers | Enforced by | Failure mode |
| :--- | :--- | :--- | :--- |
| **Organization** | Whose account is this? | Membership | Wrong bill, wrong admin |
| **Project** | Which resources exist? | Resource ownership | Cross-project data leak |
| **Row scope** | Which rows of them? | RLS predicate | Cross-customer data leak |

## What each level is for

**Organization** is the account: billing, members, projects, settings. A user
belongs to many organizations and switches between them.

**Project** is the isolation boundary and the unit of work. Datasources,
datasets, reports and schedules belong to exactly one project. A user belongs to
many projects, across many organizations.

**Row scope** is not a level of the hierarchy — it is a predicate inside a
project, and it is where an ISV's own customers live. A company embedding
cronos for 800 customers creates **one** project and 800 row scopes, never 800
organizations. Organizations model your buyer's internal structure; row scope
models their customers.

Folders are presentation only. They organise reports inside a project and carry
no permissions; two reports in the same folder are not thereby related, and a
folder never crosses a project.

## Roles

Org and project roles are separate grants. Org roles administer the account;
project roles govern content.

| Org role | Can |
| :--- | :--- |
| `owner` | Everything, including billing and deleting the organization |
| `admin` | Manage members and projects |
| `member` | Belong to the organization; see only projects they are a member of |

| Project role | Can |
| :--- | :--- |
| `admin` | Manage project members, groups, grants, datasources and settings. Not subject to grants — see Who may open which report |
| `editor` | Create and edit datasets, reports and schedules |
| `viewer` | Run and view — the reports they were granted, through the rows they were confined to; export if the report allows it |

Org `owner` and `admin` may enter any project in their organization without a
project membership — the alternative is an administrator who cannot fix a broken
report, and every product that tries it grows a back door instead.

An org `member` with no project membership sees an empty project list. That is
correct, not a bug.

## The active context is explicit, never inferred

A user with access to nine projects still acts in exactly one per request. The
active organization and project are resolved **once, at the edge**, and carried
through unchanged.

- **Management API** — scope is in the path:
  `/v1/orgs/{org}/projects/{project}/reports/{name}`. It cannot be forgotten, it
  appears in every log line, and audit gets it for free.
- **Embed API** — scope comes from the signed token only. The client cannot
  choose or widen it.
- **Never from a default.** No "the user's last project", no "their only
  project". Inferring the active context is how a request ends up reading the
  right report against the wrong project.

## The three checks, in order

Every read runs all three. They are different mechanisms and none substitutes
for another:

1. **Membership** — may this principal enter this project at all? Rejected at
   the edge, before any resource is named.
2. **Resource ownership** — does the named dataset belong to *this* project?
   Structural: a dataset carries exactly one `project_id`, and resolution is
   scoped by it. A report cannot reference another project's dataset. Not
   configurable, not a predicate — a resource from another project is not
   found, rather than found-and-denied.
3. **Row-level security** — which rows within it? A predicate, conjoined with
   everything else.

The effective predicate is still only ever a conjunction:

```
project ownership ∧ RLS ∧ token constraints ∧ report params ∧ user filter
```

A filter narrows. It never widens.

## Scope fails closed

Row scope applies to **end customers** — holders of an embed token — and to
**a project viewer somebody confined**. See *Confining a member* below.

Everybody else is exempt: an editor, an admin, an org administrator, or a
schedule running as its owner. They are protected by membership and by the
project owning its resources, which this document already calls sufficient, and
applying it to them would mean an author cannot preview their own report —
every figure on the page an em dash.

The exemption comes from a signed audience and never from an absent claim, and
that distinction is the whole of it. "This token says it belongs to a project
member" is a statement somebody made and signed; "this token has no scope" is a
statement nobody made, and reading the second as permission is how one missing
claim becomes a full-table disclosure. A principal nobody marked is treated as
an end customer, so forgetting costs a blank report rather than everybody's
data.

## Who may open which report

A project role answers "may this person read reports here", which is one
question short. A finance department has people who should read the receivables
summary and people who should not, and the only way to express that used to be
a second project — which splits the datasets too, and is a sledgehammer for a
permission.

**A report nobody has granted is readable by the whole project.** It becomes
restricted by its first grant and not a moment before.

That is the opposite of the usual instinct and it is deliberate.
Closed-until-granted makes an upgrade an outage for every report in every
deployment, first noticed when a burst delivers nothing at 06:00 and a customer
asks where their statement went. The cost is that restricting a report is an act
somebody has to take, which is the cost every allow-list has.

```bash
# Grant it to a group, or to one person.
curl -X POST localhost:8787/v1/reports/receivables/grants \
  -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"kind":"group","subject":"finance"}'

# Who may open it, and whether anybody has narrowed it at all.
curl localhost:8787/v1/reports/receivables/grants -H "Authorization: Bearer $TOKEN"
# {"grants":[{"report":"receivables","kind":"group","subject":"finance"}],"restricted":true}
```

Grants are recorded against the report and never inside it. A definition is
content-addressed and versioned, so access in the YAML would make every
permission change a new version of the report and every grant an editor's job.
Who may read something is an administrator's decision about people, not an
author's decision about content — and because a grant is not in the definition,
an editor cannot edit their way past one.

**A report somebody may not open is hidden and answers 404.** Not 403: telling
somebody a report exists and that they are not on its list is a fact about the
project nobody granted them, and across a list of names it is an enumeration
oracle for whatever an operator called their most sensitive report.

**Project administrators are not subject to grants.** They are who adds and
removes them, and a deployment where an admin can be locked out of a report has
a recovery path that ends at a psql prompt. The same reasoning this document
already gives for an org owner entering any project.

## Confining a member

A grant decides whether somebody opens a report. Confinement decides which rows
they see inside it, and it is the answer for the regional manager who signs in
to read their own region and must not read the others.

A scope is set on a **group**, or on **one person** where they need an
exception:

```bash
# Everybody in this group reads the west region and nothing else.
curl -X POST localhost:8787/v1/groups \
  -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"name":"west","scope":{"region":"west"}}'
```

The fields a scope names are the ones the dataset's `rowLevelSecurity`
predicates read, exactly as an embed token's scope is — the same mechanism,
bound rather than interpolated, reaching the same place in the compiled query.

**Only viewers are confined.** An editor or an admin carrying a scope is still
exempt, because they build and repair reports and have to see the whole of one.
A viewer with no scope is unchanged from before the feature existed, so a
deployment that sets none sees no difference at all — it is opt-in per person.

**A person's own scope overrides their groups' rather than merging with it**,
and two groups that confine the same field differently are refused rather than
resolved. Picking one silently is how somebody reads a region nobody granted
them; picking the union is how adding a group widens a confinement that was
meant to narrow. Neither is an answer, so the sign-in fails and names both
groups.

**A change takes effect on the next principal lookup**, within five seconds —
the same delay disabling an account already has. Confinement is resolved where
the principal is built rather than minted into the token, because there are six
places a portal token is issued and a scope threaded through all six is one
somebody eventually forgets to thread. A missed site would be a viewer reading
every region.

**Both readings fail closed.** A confinement that cannot be read refuses the
request rather than serving an unconfined one, and grants that cannot be read
refuse the report and hide the catalogue. Reading a failure as "nothing is
restricted" would open every restricted report in the deployment at the one
moment nobody is watching the logs.

None of this is mounted without a store. A file-backed deployment has nowhere to
record a grant, so nobody is restricted and nobody is confined — which is not a
degraded mode, it is the deployment working exactly as it did before.

## One process, or several

A process serves one project by default — `CRONOS_ORG` and `CRONOS_PROJECT` —
and that was the whole of the isolation: the blast radius of a bad definition,
a runaway query or a leaked signing key was one customer's project, because
there was physically nothing else in the process.

`CRONOS_PROJECTS=acme/finance,globex/ops` serves several, with definitions under
`$CRONOS_DEFINITIONS/<org>/<project>` and a database store, because a
definitions directory holds one project. Each gets its own definitions in
memory, its own connection pools and its own scheduler, resolved per request
from the caller's own principal. Nothing is shared but the store, which has
scoped every statement by organisation and project since it existed.

That trade is worth naming: with several, isolation is a property of the code
rather than of the operating system. One process per project is still supported
and is still the stronger answer.

**A token now has to name a project the server serves.** It always carried one;
the embed handler simply never read it, because one signing key meant one
deployment meant one project. Any token signed with the right key opened any
report on that server whatever project it claimed. That is no longer true, and
a host minting tokens with the wrong organisation or project will find them
refused.

A **share link** is an embed token, so this rule decides what it can be. The
recipient is by definition not a project member, which means a link to a report
whose dataset is row-scoped would show them nothing — and the only way to make
it show something would be to mark the token as a member, which is this rule
with the check removed. So such a share is refused, and the message says which
dataset made it one. Sharing a per-customer report means saying which customer,
which is a question only the person sharing can answer.

For an end customer, a dataset whose row-level security references
`{{ .scope.x }}` **cannot be read without that scope**. If the value is absent the predicate matches nothing; it
is never dropped, and never treated as "no constraint". The alternative — an
absent scope meaning unrestricted — turns one missing token claim into a full
table disclosure.

"Matches nothing" is the literal `FALSE`, replacing the whole predicate — not a
comparison against a null. `customer_id = NULL` happens to return no rows, but
that is a property of the comparison the *author* chose: wrap the same hole in
`COALESCE`, or write `NOT IN`, and the null stops being safe. Replacing the
predicate cannot be written around.

**`.scope` may only appear in `rowLevelSecurity`.** A `{{ .scope.x }}` in the
dataset's own `query:` is rejected on save and at compile. It reads as a
sensible optimisation — push the predicate down so the source can index it —
but the fail-closed rule works a predicate at a time, and it can only replace
text it knows is a predicate. A scope hole in the query body would bind an
empty string and run: the exact disclosure the rule exists to prevent, arrived
at by an author trying to be helpful. Pushdown will be a deliberate feature
with the same `FALSE` semantics, not a side effect of where someone typed.

The consequence is a modelling rule, and it is easy to get wrong: **a dataset
read by a schedule must not carry a `.scope` predicate.** Publishing a schedule
now refuses one that does, and says what would have happened — the rule is
enforced rather than documented. Scheduled runs and
bursts execute as the schedule's owner, a project member with no embed token, so
a scope predicate matches nothing and the burst silently delivers zero
documents. Internal datasets — burst targets, admin reports — rely on project
membership alone. That is sufficient: project isolation is already structural.

Read paths therefore split cleanly:

| Read by | Scope | Protected by |
| :--- | :--- | :--- |
| A project member, in the app | none | Membership + project ownership |
| A schedule or burst | none | Membership of the schedule's owner |
| An embedded end-customer | from the signed token | All three checks |

## Cache keys

Any cache key includes **organization, project, principal and definition
version**. A cache that omits any of them is a cross-tenant leak with extra
steps, and this is the single most likely place for one to appear — the query
result cache for API-backed datasources, where the upstream call is expensive
enough that caching is mandatory.

## Storage

```sql
organizations   (id, slug, name, created_at)
projects        (id, org_id, slug, name, created_at)   unique (org_id, slug)
users           (id, email, name, created_at)
org_members     (org_id, user_id, role)                unique (org_id, user_id)
project_members (project_id, user_id, role)            unique (project_id, user_id)
```

Every resource table carries `project_id`, and every composite index leads with
it — the project filter is on every query, so it belongs first in every index.

Names are unique within a project, not globally: two projects may both have a
report called `monthly-statement`, and they are unrelated.

## Not in v1

- **Cross-project sharing.** A dataset belongs to one project. Sharing needs an
  explicit grant model, and guessing at it now would bake the wrong one in.
- **Nested organizations.** Two levels is enough structure for the buyers in
  [product.md](product.md); a tree is a support burden with no named demand.
- **Custom roles.** Three project roles cover create/edit/view, now narrowed
  further per report by grants. A fourth role arrives when somebody can name a
  permission that grants and confinement together cannot express.

## Pricing

Organizations and projects are free structure. Pricing them per unit recreates
the growth penalty the product exists to avoid — it is the per-viewer problem
wearing a different hat, and a customer who has to ration projects will put
everything in one and lose the isolation the model is for.
