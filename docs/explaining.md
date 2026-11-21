# Explaining reports

A reader who opens a report they did not build can ask what it means: what a
metric measures, what the numbers say, whether a change is large. They can keep
going in the same conversation, with follow-up questions about a chart and
advice on what to do about a figure. cronos answers with a language model the
deployment configures. It is off until a deployment turns it on.

## Turning it on

| Variable | |
| :--- | :--- |
| `CRONOS_AI_API_KEY` | The provider's API key. Setting it is the opt-in: without it the endpoint is not mounted and the portal offers nothing. |
| `CRONOS_AI_PROVIDER` | `anthropic`, the default and the only adapter today. Anything else is refused at startup. |
| `CRONOS_AI_MODEL` | The model id. `claude-opus-5-5` by default; `claude-sonnet-5-5` costs less per answer. |
| `CRONOS_AI_BASE_URL` | A gateway or proxy in front of the provider's API. Unset is the provider's own. |

The startup line says which model answers (`explain=claude-opus-5-5`), or
nothing when none is configured. cronos reads only these variables: an
`ANTHROPIC_*` variable in the process's environment is ignored, so a value left
over in a container cannot redirect readers' figures.

## What is sent

`POST /v1/reports/{name}/explain` takes the request the report was rendered
with — its filters and parameters — and optionally a `block` (its place on the
report, from zero) and a `question`. The server then renders the report again
**as the person asking**: through their grants, their row scope and anything
pinned for them. The model is given that rendered view and nothing else.

A follow-up also sends `history`: the earlier questions in the conversation,
oldest first, each with its `block`, its `question` and the `answer` the reader
was shown. The server keeps no conversation. It takes at most twelve earlier
exchanges, each answer at most 4,000 characters, and refuses more with `400`
before anything is sent. The report is rendered again on every request and
sent once, at the top, so a follow-up is always read against the figures on
screen; the portal starts a new conversation when the filters change. A
client can send history the model never wrote, which changes nothing about
what the model is shown of the data.

From the view it sends, as plain text:

- the report's title, description and the filter values in force;
- each block's title and kind;
- a stat's value, its change and its trend;
- a chart's points, at most 50 per series — the first and last 25 of a longer
  one, with how many were left out;
- a table's columns and at most 20 rows, with the total that matched;
- that a block is a map, and none of its places.

Never sent: SQL, dataset definitions, datasource names or credentials, secrets,
or any row the reader was not shown. A request cannot supply numbers of its own
for the model to explain; it can only name a report the caller may already open.
Labels and cell values are clipped to 120 characters and the whole of it to
32 KB.

The report goes to the model as data, inside delimiters it is told never to
follow instructions from. Angle brackets in a value are replaced before sending,
so a cell cannot close those delimiters early.

## What comes back

A short answer, at most about 250 words, that uses only the figures supplied
and says so when they cannot answer the question. Answers follow two
frameworks, so they cover the same ground in the same order:

- **An explanation** is organised by 5W1H: *What* is measured and what the
  numbers show, *Who* they cover, *When* (the period), *Where* (the regions,
  channels or products they are split by), *Why* and *How* to read it. A label
  the report says nothing about is left out, except *Why*, which always
  appears and usually says that the report does not show a cause.
- **Advice** (how to improve a figure, what to look into next) is a MECE list:
  two to four groups of options that do not overlap and together cover the
  main levers for that figure, each with a concrete action tied to a figure in
  the report, then *Check first:* the one thing to look at before acting. It is
  labelled as ideas to test, not findings.

A short factual follow-up gets a short direct answer. The portal labels every
answer as generated. A request the model declines is answered `422`; a provider
that fails or does not answer within a minute, `502`, with the provider's own
error kept out of the response and out of the log line.

## Audit and limits

Each explanation is recorded as `report.explain`, allowed or refused, with the
block, the length of the question and the number of earlier exchanges, but
not the text of any of them. The render behind it is
recorded as `report.read` like any other.

Explanations are limited per reader to six a minute, with a burst of ten: each
one is a render and a paid call to somebody else's API. In a conversation the
report is marked for the provider's prompt cache, so a follow-up is not billed
the whole report again.

## Not covered

The embedded `<cronos-report>` element does not offer explanations. A host
application embedding a report has its own customers' consent to consider, and
the element has its own size budget.
