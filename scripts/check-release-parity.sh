#!/usr/bin/env bash
#
# Every command ships in every channel that should carry it.
#
# The rule this enforces was broken the moment it was written: cronos-import
# was added to the container image and to nothing else, so the documentation
# said "it ships with cronos" and it did not, for any deployment that installs
# binaries. A container is one way to run cronos and not the only one, and a
# tool present in one channel is a tool half the readers cannot find.
#
#   1. Community commands ship in the BSL archive, the Linux packages, AND the
#      image.
#   2. cronosd-ee ships in the EE archive and in none of the others, because the
#      published image and the published packages are the community edition and
#      cronosd-ee is licensed separately.
#
# Read out of cmd/ rather than a list kept here, so adding a command is what
# fails this rather than remembering to update it.
#
# The release config is .goreleaser.yaml now rather than a shell script, so this
# reads YAML with grep. That is worth one sentence of justification: the checks
# below are all "does this exact line exist", the file is ours and the lines are
# stable, and the alternative is a YAML parser as a dependency of a gate whose
# job is to run everywhere. `goreleaser check` already validates the shape; this
# checks the meaning.
set -euo pipefail
cd "$(dirname "$0")/.."

CONFIG=.goreleaser.yaml
IMAGE=Dockerfile
status=0
ok()  { printf '  \033[32mok\033[0m %s\n' "$*"; }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$*" >&2; status=1; }

# buildID names the goreleaser build whose main is ./cmd/<name>. The id is not
# always the command — cronosd is built by `bsl` and cronosd-ee by `ee` — so
# every check below has to resolve it rather than assume it.
buildID() {
	awk -v want="./cmd/$1" '
		/^  - id: / { id = $NF }
		$1 == "main:" && $2 == want { print id; exit }
	' "$CONFIG"
}

# inList reports whether id appears in the `ids: [...]` of the named section.
inList() {
	local section="$1" id="$2"
	awk -v section="$section" -v id="$id" '
		$0 ~ "^  - id: " section "$" { inside = 1; next }
		inside && /^  - id: / { exit }
		inside && $1 == "ids:" { if (index($0, id)) found = 1 }
		END { exit !found }
	' "$CONFIG"
}

echo "==> release parity"

for dir in cmd/*/; do
	cmd=$(basename "$dir")

	id=$(buildID "$cmd")
	[ -n "$id" ] || { bad "$cmd is in cmd/ and $CONFIG builds nothing from it"; continue; }

	if [ "$cmd" = "cronosd-ee" ]; then
		# Its own archive, and deliberately in nothing else. Shipping it beside
		# the community binaries would put commercially licensed code inside the
		# artifact somebody downloads expecting the community edition.
		inList enterprise "$id" ||
			bad "$cmd is not in the enterprise archive in $CONFIG"
		if inList community "$id"; then
			bad "$cmd is in the community archive — that archive carries the BSL LICENSE"
		fi
		if inList linux "$id"; then
			bad "$cmd is in the Linux packages — those are the community edition"
		fi
		if grep -q "cmd/${cmd}\b" "$IMAGE"; then
			bad "$cmd is built by $IMAGE — the published image is the community edition"
		fi
		continue
	fi

	# The tarball, for a host that wants binaries.
	inList community "$id" ||
		bad "$cmd is in cmd/ but not in the community archive in $CONFIG"

	# The Linux packages, for a host that has a package mirror. A command in the
	# tarball and not the .deb is the same split cronos-import already had once.
	inList linux "$id" ||
		bad "$cmd is in cmd/ but not in the Linux packages in $CONFIG"

	# Built into the image, and copied out of the build stage into it. Both,
	# because building it and forgetting the COPY produces an image that is
	# missing the command and says nothing.
	grep -q "cmd/${cmd}\b" "$IMAGE" ||
		bad "$cmd is in cmd/ but $IMAGE does not build it"
	grep -q "COPY --from=server /out/${cmd}\b" "$IMAGE" ||
		bad "$cmd is built by $IMAGE but never copied into the final image"
done

# The archives carry the licence that covers the binary beside them. This is the
# distribution half of scripts/check-license-boundary.sh, and the half a build
# graph cannot see.
grep -q 'src: ee/LICENSE' "$CONFIG" ||
	bad "the enterprise archive does not carry ee/LICENSE"

[ "$status" -eq 0 ] && ok "every command ships in the channels that should carry it"
exit $status
