#!/usr/bin/env bash
set -euo pipefail

DOMAIN=${1:?usage: check_dns.sh <domain>}
AUTOCONFIG_HOST=${AUTOCONFIG_HOST:-autoconfig.$DOMAIN}
AUTODISCOVER_HOST=${AUTODISCOVER_HOST:-autodiscover.$DOMAIN}
IMAP_HOST=${IMAP_HOST:-imap.$DOMAIN}
SMTP_HOST=${SMTP_HOST:-smtp.$DOMAIN}

if ! command -v dig >/dev/null 2>&1; then
	echo "dig is required" >&2
	exit 1
fi

failures=0

check_address() {
	local host=$1
	local records
	records=$(dig +short A "$host"; dig +short AAAA "$host")
	if [[ -n "$records" ]]; then
		echo "PASS A/AAAA $host -> $(echo "$records" | tr '\n' ' ')"
	else
		echo "FAIL A/AAAA $host is missing"
		failures=$((failures + 1))
	fi
}

check_srv() {
	local name=$1
	local records
	records=$(dig +short SRV "$name")
	if [[ -n "$records" ]]; then
		echo "PASS SRV $name -> $(echo "$records" | tr '\n' ' ')"
	else
		echo "FAIL SRV $name is missing"
		failures=$((failures + 1))
	fi
}

check_address "$AUTOCONFIG_HOST"
check_address "$AUTODISCOVER_HOST"

check_srv "_autodiscover._tcp.$DOMAIN"
check_srv "_imaps._tcp.$DOMAIN"
check_srv "_submission._tcp.$DOMAIN"
check_srv "_submissions._tcp.$DOMAIN"

echo "expected mail hosts: imap=$IMAP_HOST smtp=$SMTP_HOST"

if ((failures > 0)); then
	echo "dns checks failed: $failures"
	exit 1
fi
echo "dns checks passed"
