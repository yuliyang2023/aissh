(
set -o pipefail
sslvpn_until=$(date '+%Y-%m-%d %H:%M:%S') &&
sslvpn_since=$(date -d '5 minutes ago' '+%Y-%m-%d %H:%M:%S') &&
LC_ALL=C journalctl -u webvpn.service --since "$sslvpn_since" --until "$sslvpn_until" --no-pager -o short-iso 2>&1 | awk -v start="$sslvpn_since" -v end="$sslvpn_until" '
function field(key, line, pattern) {
  pattern="\"" key "\"[[:space:]]*:[[:space:]]*\""
  if (!match(line, pattern)) return ""
  line=substr(line, RSTART+RLENGTH)
  if (!index(line, "\"")) return ""
  sub(/".*/, "", line)
  return line
}
function sourceIP(addr, ip) {
  if (addr ~ /^\[[[:alnum:]:.%_-]+\]:[0-9]+$/) {
    ip=addr
    sub(/^\[/, "", ip)
    sub(/\]:[0-9]+$/, "", ip)
    if (!index(ip, ":")) return ""
    ip=tolower(ip)
    if (ip ~ /^::ffff:[0-9.]+$/) sub(/^::ffff:/, "", ip)
    return ip
  }
  if (addr ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:[0-9]+$/) {
    sub(/:[0-9]+$/, "", addr)
    return addr
  }
  return ""
}
/^-- / { next }
/^[[:space:]]*$/ { next }
{
  if ($0 !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T/ && $0 !~ /^[[:space:]]/) { invalid++; next }
  if (tolower($0) ~ /suppressed[[:space:]]+[0-9]+[[:space:]]+messages/) { invalid++; next }
  if ($0 !~ /[[:space:]]INFO[[:space:]]+\[SSLVPN\][[:space:]]+用户认证成功[[:space:]]+\{/) next
  authRecords++
  uid=field("user_id", $0)
  cid=field("connection_id", $0)
  ip=sourceIP(field("remote_addr", $0))
  if (uid !~ /^[0-9]+$/ || length(cid)!=36 || cid !~ /^[0-9a-fA-F-]+$/ || ip=="") { invalid++; next }
  group=uid SUBSEP ip
  unique=group SUBSEP cid
  if (unique in seen) next
  seen[unique]=1
  distinct++
  if (!(group in counts)) {
    order[++groups]=group
    users[group]=uid
    ips[group]=ip
    first[group]=$1
  }
  counts[group]++
  last[group]=$1
  if (counts[group]>maximum) maximum=counts[group]
}
END {
  if (NR==0 || invalid>0) {
    print "AUTH_MAX_5M=unknown"
    printf "UNIT=webvpn.service REASON=incomplete_journal_or_invalid_auth_fields INVALID_LINES=%d\n", invalid
    exit 2
  }
  for (i=1; i<=groups; i++) if (counts[order[i]]>=5) affected++
  printf "AUTH_MAX_5M=%d\n", maximum
  printf "UNIT=webvpn.service WINDOW_START=%s WINDOW_END=%s\n", start, end
  printf "AUTH_RECORDS=%d DISTINCT_CONNECTIONS=%d AFFECTED_USER_SOURCE_PAIRS=%d DETAILS_SHOWN=%d DETAILS_OMITTED=0 ORDER=first_auth_in_window\n", authRecords, distinct, affected, affected
  for (i=1; i<=groups; i++) {
    group=order[i]
    if (counts[group]>=5) printf "USER_ID=%s SOURCE_IP=%s DISTINCT_AUTH_CONNECTIONS=%d FIRST_AUTH=%s LAST_AUTH=%s\n", users[group], ips[group], counts[group], first[group], last[group]
  }
}'
)
