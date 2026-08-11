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
| `admin` | Manage project members, datasources, and settings |
| `editor` | Create and edit datasets, reports and schedules |
| `viewer` | Run and view; export if the report allows it |

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

A dataset whose row-level security references `{{ .scope.x }}` **cannot be read
without that scope**. If the value is absent the predicate matches nothing; it
is never dropped, and never treated as "no constraint". The alternative — an
absent scope meaning unrestricted — turns one missing token claim into a full
table disclosure.

The consequence is a modelling rule, and it is easy to get wrong: **a dataset
read by a schedule must not carry a `.scope` predicate.** Scheduled runs and
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
- **Custom roles.** Three project roles cover create/edit/view. Custom roles
  arrive when a design partner can name the permission they are missing.

## Pricing

Organizations and projects are free structure. Pricing them per unit recreates
the growth penalty the product exists to avoid — it is the per-viewer problem
wearing a different hat, and a customer who has to ration projects will put
everything in one and lose the isolation the model is for.
