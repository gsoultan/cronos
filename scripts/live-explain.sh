#!/usr/bin/env bash
#
# Explaining a report, against the built cronosd and a stand-in for the model.
#
# The property that matters is what leaves for the provider. An explanation is
# a render followed by a paid call to somebody else's API, and the render has to
# be the asker's: a viewer confined to one customer must send that customer's
# figures and nobody else's. Unit tests prove the handler does this; this proves
# the binary is wired to do it — the route mounted from configuration, the
# confinement resolved from the store, the request shaped as the Messages API
# expects.
#
# The model is a local stand-in that records what it was sent, so this needs no
# key and spends nothing. Point CRONOS_AI_BASE_URL at the real API to try it for
# real.
#
#   ./scripts/live-explain.sh
#
# Needs go and python3. Leaves nothing behind.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8798
MODEL_PORT=8799
API="http://127.0.0.1:$PORT"

work=$(mktemp -d)
cleanup() {
	[ -n "${server:-}" ] && { kill "$server" 2>/dev/null || true; }
	[ -n "${model:-}" ] && { kill "$model" 2>/dev/null || true; }
	rm -rf "$work" || true
	return 0
}
trap 'cleanup 2>/dev/null' EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok() { printf '  \033[32mok\033[0m %s\n' "$*"; }
die() { printf '  \033[31mFAILED\033[0m %s\n' "$*" >&2; exit 1; }
field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }

boot() {
	CRONOS_ADDR=":$PORT" \
		CRONOS_DEFINITIONS="$work/defs" \
		CRONOS_STORE_DRIVER=sqlite \
		CRONOS_STORE_DSN="file:$work/cronos.db" \
		CRONOS_SIGNING_KEY=0123456789abcdef0123456789abcdef \
		CRONOS_SEED=demo/seed.sql CRONOS_SEED_SOURCE=warehouse \
		"$@" ./bin/cronosd >>"$work/log" 2>&1 &
	server=$!
	for _ in $(seq 1 60); do
		curl -sf "$API/v1/health" >/dev/null 2>&1 && return 0
		sleep 0.25
	done
	cat "$work/log"
	die "cronos never came up"
}
stop() { kill "$server"; wait "$server" 2>/dev/null || true; server=; }

explain() {
	curl -s -w '\n%{http_code}' -X POST -H "Authorization: Bearer $1" \
		-H 'content-type: application/json' -d "${3:-{\}}" "$API/v1/reports/$2/explain"
}

# --- off unless asked for -----------------------------------------------------

say "Without a key"
go build -o bin/cronosd ./cmd/cronosd || die "build"
mkdir -p "$work/defs"
cp demo/definitions/*.yaml "$work/defs/"
boot env

ADMIN=$(curl -s -X POST -H 'content-type: application/json' \
	-d '{"email":"ada@acme.example","name":"Ada Rahayu","password":"a-password-they-chose","org":"acme","project":"finance"}' \
	"$API/v1/setup" | field token)
[ -n "$ADMIN" ] || { cat "$work/log"; die "no session came back from /v1/setup"; }

[ "$(curl -s -H "Authorization: Bearer $ADMIN" "$API/v1/catalog" | field explain)" = False ] ||
	die "the catalogue offered explanations with no model configured"
code=$(explain "$ADMIN" billing-summary | tail -1)
[ "$code" = 404 ] || die "the route answered $code with no model configured"
ok "the catalogue says no, and there is no route to press"
stop

# --- a stand-in model ---------------------------------------------------------

cat >"$work/model.py" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

log = sys.argv[2]

class Model(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers["content-length"]))
        with open(log, "a") as f:
            f.write(json.dumps({"path": self.path, "beta": self.headers.get("anthropic-beta", ""),
                                "body": json.loads(body)}) + "\n")
        reply = {"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
                 "content": [{"type": "text", "text": "Total billed is what was invoiced in the period."}],
                 "stop_reason": "end_turn", "usage": {"input_tokens": 1, "output_tokens": 1}}
        out = json.dumps(reply).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)
    def log_message(self, *a):
        pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), Model).serve_forever()
PY
python3 "$work/model.py" "$MODEL_PORT" "$work/sent.jsonl" &
model=$!
sleep 0.5

say "With a key"
boot env CRONOS_AI_API_KEY=sk-live-check CRONOS_AI_BASE_URL="http://127.0.0.1:$MODEL_PORT"
grep -q 'explain=claude-opus-5-5' "$work/log" || die "the startup line does not name the model"
[ "$(curl -s -H "Authorization: Bearer $ADMIN" "$API/v1/catalog" | field explain)" = True ] ||
	die "the catalogue does not offer explanations"
ok "the startup line names the model and the catalogue offers it"

out=$(explain "$ADMIN" billing-summary)
[ "$(tail -1 <<<"$out")" = 200 ] || die "an administrator's explanation: $out"
grep -q "Total billed is what was invoiced" <<<"$out" || die "the answer did not come back: $out"
python3 - "$work/sent.jsonl" <<'PY' || die "the request to the model was not what the Messages API expects"
import json, sys
sent = [json.loads(l) for l in open(sys.argv[1])]
assert len(sent) == 1, sent
s = sent[0]
assert s["path"] == "/v1/messages", s["path"]
assert "server-side-fallback-2026-07-01" in s["beta"], s["beta"]
b = s["body"]
assert b["model"] == "claude-opus-5-5" and b["fallbacks"] == "default", b
user = b["messages"][0]["content"][0]["text"]
assert "Report: Billing summary" in user and "[Block 0] Total billed" in user, user
for leak in ("SELECT", "warehouse", "dsn", "postgres"):
    assert leak not in user, ("sent " + leak, user)
PY
ok "the model was sent the report as the reader sees it, and no query or source"

# --- a confined viewer --------------------------------------------------------

say "A viewer confined to one customer"
BUDI=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
	-d '{"email":"budi@acme.example","name":"Budi","role":"viewer","password":"another-password-here"}' \
	"$API/v1/people" | field id)
GROUP=$(curl -s -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
	-d '{"name":"Customer c-1","scope":{"customer_id":"c-1"}}' "$API/v1/groups" | field id)
[ -n "$BUDI" ] && [ -n "$GROUP" ] || die "the viewer or the group could not be created"
curl -sf -o /dev/null -X POST -H "Authorization: Bearer $ADMIN" -H 'content-type: application/json' \
	-d "{\"user\":\"$BUDI\"}" "$API/v1/groups/$GROUP/members" || die "Budi could not join the group"
budi=$(curl -s -X POST -H 'content-type: application/json' \
	-d '{"email":"budi@acme.example","password":"another-password-here"}' "$API/v1/auth/login" | field token)
[ -n "$budi" ] || die "Budi could not sign in"
sleep 6 # past the five seconds a confinement is cached for

total() {
	curl -s -X POST -H "Authorization: Bearer $1" -H 'content-type: application/json' -d '{}' \
		"$API/v1/reports/billing-summary" | python3 -c 'import json,sys; print(json.load(sys.stdin)["blocks"][0]["value"])'
}
mine=$(total "$budi")
everyone=$(total "$ADMIN")
[ "$mine" != "$everyone" ] || die "Budi sees the whole project's total ($everyone): the confinement is not applied"

: >"$work/sent.jsonl"
out=$(explain "$budi" billing-summary '{"block":0,"question":"Is this a lot?"}')
[ "$(tail -1 <<<"$out")" = 200 ] || die "Budi's explanation: $out"
python3 - "$work/sent.jsonl" "$mine" "$everyone" <<'PY' || die "Budi's explanation was not built from Budi's rows"
import json, sys
user = json.loads(open(sys.argv[1]).readline())["body"]["messages"][0]["content"][0]["text"]
mine, everyone = sys.argv[2], sys.argv[3]
assert "Value: " + mine in user, ("not sent", mine, user)
assert everyone not in user, ("the whole project's total was sent", everyone, user)
assert "<question>\nIs this a lot?\n</question>" in user, user
PY
ok "Budi's explanation carried Budi's total ($mine) and not the project's ($everyone)"

grep -q '"action":"report.explain"' "$work/log" || grep -q 'report.explain' "$work/log" ||
	die "no explanation reached the audit log"
grep -q 'Is this a lot' "$work/log" && die "the question's text reached the log"
ok "each one was audited, without the question's text"

# --- the limit ----------------------------------------------------------------

say "The limit"
limited=
for _ in $(seq 1 12); do
	[ "$(explain "$budi" billing-summary | tail -1)" = 429 ] && { limited=1; break; }
done
[ -n "$limited" ] || die "twelve explanations at once were all answered"
ok "a reader asking in a loop is told to wait"

say "All checks passed"
