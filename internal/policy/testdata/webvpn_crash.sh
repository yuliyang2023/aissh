(
set -o pipefail
sslvpn_until=$(date '+%Y-%m-%d %H:%M:%S') &&
sslvpn_since=$(date -d '1 minute ago' '+%Y-%m-%d %H:%M:%S') &&
LC_ALL=C journalctl -u webvpn.service --since "$sslvpn_since" --until "$sslvpn_until" --no-pager -o short-iso 2>&1 | awk -v start="$sslvpn_since" -v end="$sslvpn_until" '
/^-- / { next }
/^[[:space:]]*$/ { next }
{
  text=tolower($0)
  if ($0 !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T/ && $0 !~ /^[[:space:]]/) { invalid++; next }
  if (text ~ /suppressed[[:space:]]+[0-9]+[[:space:]]+messages/) { invalid++; next }
  fatal=text ~ /webvpn\[[0-9]+\]:[[:space:]]+(panic:|fatal error:|runtime: out of memory|sigsegv|sigabrt)/
  failed=text ~ /systemd\[[0-9]+\]:/ && text ~ /webvpn/ && ((text ~ /main process exited/ && text !~ /status=0([/ ,]|$)/) || text ~ /entered failed state|failed with result/)
  if (fatal || failed) { hits++; last=$1 }
}
END {
  if (NR==0 || invalid>0) {
    print "CRASH_RECORDS=unknown"
    printf "UNIT=webvpn.service REASON=incomplete_or_unrecognized_journal DIAGNOSTIC_LINES=%d\n", invalid
    exit 2
  }
  printf "CRASH_RECORDS=%d\n", hits
  printf "UNIT=webvpn.service WINDOW_START=%s WINDOW_END=%s LAST_MATCH=%s\n", start, end, last=="" ? "none" : last
}'
)
