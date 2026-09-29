(
set -o pipefail
sslvpn_state=$(LC_ALL=C systemctl show webvpn.service --no-pager -p Id -p LoadState -p ActiveState -p SubState -p MainPID) || exit $?
printf '%s\n' "$sslvpn_state" | awk -F= '
$1 == "Id" { unit=$2; haveUnit++ }
$1 == "LoadState" { load=$2; haveLoad++ }
$1 == "ActiveState" { active=$2; haveActive++ }
$1 == "SubState" { substate=$2; haveSub++ }
$1 == "MainPID" { pid=$2; havePid++ }
END {
  if (haveUnit!=1 || haveLoad!=1 || haveActive!=1 || haveSub!=1 || havePid!=1 || unit!="webvpn.service" || load!="loaded" || substate=="" || pid!~/^[0-9]+$/ || active!~/^(active|inactive|failed|activating|deactivating|reloading|maintenance)$/) {
    print "SERVICE_DOWN=unknown"
    print "UNIT=webvpn.service REASON=missing_or_unrecognized_service_state"
    exit 2
  }
  running=(active=="active" || active=="reloading") && pid+0>0
  printf "SERVICE_DOWN=%d\n", running ? 0 : 1
  printf "UNIT=%s ACTIVE_STATE=%s SUB_STATE=%s MAIN_PID=%s\n", unit, active, substate, pid
}'
)
