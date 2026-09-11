#!/bin/sh
# Runs after install and after upgrade.
#
# Creates the account the unit runs as and the directory it is allowed to write
# to, and does neither if they are already there — an upgrade must not reset a
# deployment's ownership or take away a directory somebody moved.
set -e

if ! getent passwd cronos >/dev/null 2>&1; then
	# A system account with no login and no home: it runs one binary and reads
	# one environment file, and a shell on it is a shell somebody can get.
	useradd --system --no-create-home --shell /usr/sbin/nologin cronos \
		2>/dev/null || adduser --system --no-create-home --shell /usr/sbin/nologin cronos \
		2>/dev/null || true
fi

# /var/lib/cronos, whose parent is root-owned, so nothing unprivileged can have
# put something else there first.
install -d -o cronos -g cronos -m 0750 /var/lib/cronos

# The definitions directory is inside one the service account owns, and that
# changes what creating it means.
#
# `install -d` follows a symlink at the target and applies -o/-g/-m to whatever
# it points at. So the cronos account could replace this with a link to /etc,
# wait for the next `apt upgrade` to re-run this script as root, and be handed
# ownership of /etc — and from there /etc/cron.d or /etc/ld.so.preload is root.
# That defeats every hardening line in the unit.
#
# So: created only when nothing is there, never followed, and never re-permissioned
# on an upgrade. A path that exists and is not a plain directory is left exactly
# as it is and reported, because silently repairing it is how the same trick works
# a second time.
if [ -L /var/lib/cronos/definitions ]; then
	echo "cronos: /var/lib/cronos/definitions is a symlink — refusing to touch it." >&2
	echo "cronos: remove it and reinstall if that is not deliberate." >&2
elif [ -e /var/lib/cronos/definitions ]; then
	: # already there and not a link; leave its ownership alone.
else
	install -d -o cronos -g cronos -m 0750 /var/lib/cronos/definitions
fi

# The environment file holds the signing key, so it is readable by the service
# account and nobody else.
# /etc/cronos is root-owned and the service account cannot write into it, so
# this one cannot be redirected the way the state directory could. Guarded
# anyway: -f is false for a symlink to a directory, and a symlink to a file
# would hand its target away on every upgrade.
if [ -L /etc/cronos/env ]; then
	echo "cronos: /etc/cronos/env is a symlink — refusing to change its ownership." >&2
elif [ -f /etc/cronos/env ]; then
	chown root:cronos /etc/cronos/env
	chmod 0640 /etc/cronos/env
fi

# Not enabled and not started, still. Starting a server the moment a package
# lands means starting one nobody has decided the address or the tenancy of, on
# a machine whose operator may be part-way through a script.
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi

cat <<'MESSAGE'
cronos installed. To configure it:

  systemctl enable --now cronos
  journalctl -u cronos -n 5     # names the setup token file
  cat /var/lib/cronos/setup-token

Then open http://<this-host>:8787/setup, paste the token, and set the
administrator. cronos writes /var/lib/cronos/config.yaml and restarts into it.

Configuring by hand instead: put the variables in /etc/cronos/env, which
overrides anything setup wrote. See /usr/share/doc/cronos/deploying.md.
MESSAGE
