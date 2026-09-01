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
#   1. Community commands ship in the BSL archive AND the image.
#   2. cronosd-ee ships in the EE archive and NOT in the image, because the
#      published image is the community edition and cronosd-ee is licensed
#      separately.
#
# Read out of cmd/ rather than a list kept here, so adding a command is what
# fails this rather than remembering to update it.
set -euo pipefail
cd "$(dirname "$0")/.."

DIST=scripts/dist.sh
IMAGE=Dockerfile
status=0
ok()   { printf '  \033[32mok\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$*" >&2; status=1; }

echo "==> release parity"

for dir in cmd/*/; do
	cmd=$(basename "$dir")

	if [ "$cmd" = "cronosd-ee" ]; then
		# The EE binary: its own archive, and deliberately not the image.
		grep -q "EE_CMDS=.*\b${cmd}\b" "$DIST" ||
			bad "$cmd is not in EE_CMDS in $DIST"
		if grep -q "cmd/${cmd}\b" "$IMAGE"; then
			bad "$cmd is built by $IMAGE — the published image is the community edition"
		fi
		continue
	fi

	# Built into the archive.
	grep -q "BSL_CMDS=.*\b${cmd}\b" "$DIST" ||
		bad "$cmd is in cmd/ but not in BSL_CMDS in $DIST — it would ship in the image and not the tarball"
	# Built into the image, and copied out of the build stage into it. Both,
	# because building it and forgetting the COPY produces an image that is
	# missing the command and says nothing.
	grep -q "cmd/${cmd}\b" "$IMAGE" ||
		bad "$cmd is in cmd/ but $IMAGE does not build it"
	grep -q "COPY --from=server /out/${cmd}\b" "$IMAGE" ||
		bad "$cmd is built by $IMAGE but never copied into the final image"
done

[ "$status" -eq 0 ] && ok "every command ships in the channels that should carry it"
exit $status
