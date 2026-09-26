(
export LC_ALL=C
if mon_service=$(timeout -k 1s 1s systemctl show mariadb.service --no-pager -p ActiveState -p SubState -p MainPID 2>&1); then mon_service_rc=0; else mon_service_rc=$?; fi
if mon_tcp=$(timeout -k 1s 1s ss -ltnp 'sport = :3306' 2>&1); then mon_tcp_rc=0; else mon_tcp_rc=$?; fi
mon_query="SELECT 'sql_ok', 1; SELECT 'version', VERSION(); SELECT 'host', @@hostname; SHOW GLOBAL STATUS WHERE Variable_name IN ('Uptime','Threads_connected','Threads_running','Max_used_connections','Connections','Aborted_connects','Aborted_clients','Questions','Slow_queries','Bytes_received','Bytes_sent','Innodb_buffer_pool_read_requests','Innodb_buffer_pool_reads','Innodb_buffer_pool_pages_total','Innodb_buffer_pool_pages_free','Innodb_row_lock_current_waits','Innodb_row_lock_time','Innodb_log_waits'); SHOW GLOBAL VARIABLES WHERE Variable_name IN ('max_connections','long_query_time','slow_query_log','read_only');"
if mon_sql=$(timeout -k 1s 2s mysql --host=localhost --protocol=SOCKET --socket=/var/lib/mysql/mysql.sock --connect-timeout=2 --skip-reconnect --batch --skip-column-names --execute="$mon_query" </dev/null 2>&1); then mon_sql_rc=0; else mon_sql_rc=$?; fi
{
  printf '@service\t%s\n%s\n' "$mon_service_rc" "$mon_service"
  printf '@tcp\t%s\n%s\n' "$mon_tcp_rc" "$mon_tcp"
  printf '@sql\t%s\n%s\n' "$mon_sql_rc" "$mon_sql"
} | awk '
BEGIN { FS="\t"; sr=-1; nr=-1; qr=-1; count=0; comma="" }
function j(s, i,c,r) {
  r="\""
  for(i=1;i<=length(s);i++) {
    c=substr(s,i,1)
    if(c=="\\") r=r "\\\\"
    else if(c=="\"") r=r "\\\""
    else if(c=="\n") r=r "\\n"
    else if(c=="\r") r=r "\\r"
    else if(c=="\t") r=r "\\t"
    else if(c ~ /[[:cntrl:]]/) r=r "?"
    else r=r c
  }
  return r "\""
}
function emit(k,x) { printf "%s%s:%s",comma,j(k),x; comma="," }
function note(s) { errors=errors (errors=="" ? "" : "\n") s }
function num(k,x) {
  if(qr!=0 || !(k in v)) return "null"
  x=v[k]
  if(x !~ /^(0|[1-9][0-9]*)([.][0-9]+)?$/) return "null"
  if(x+0>9007199254740991) { overflow=1; return "null" }
  return x
}
function str(k) { return qr==0 && (k in v) ? j(v[k]) : "null" }
function row(label,k,unit) { return "{\"指标\":" j(label) ",\"数值\":" j((k in v) ? v[k] : "未知") ",\"单位\":" j(unit) "}" }
$1=="@service" { section="service"; sr=$2+0; next }
$1=="@tcp" { section="tcp"; nr=$2+0; next }
$1=="@sql" { section="sql"; qr=$2+0; next }
section=="service" {
  if(sr==0) { p=index($0,"="); if(p>0) svc[substr($0,1,p-1)]=substr($0,p+1) }
  else se=se (se=="" ? "" : "\n") $0
  next
}
section=="tcp" {
  if(nr==0) {
    split($0,a,/[[:space:]]+/)
    if($0 ~ /^LISTEN[[:space:]]/ && a[4] ~ /:3306$/) { count++; addresses=addresses (addresses=="" ? "" : ", ") a[4] }
  } else ne=ne (ne=="" ? "" : "\n") $0
  next
}
section=="sql" {
  if(qr==0 && NF==2) v[$1]=$2
  if($0 ~ /ERROR|mysql:|timeout:/) qe=qe (qe=="" ? "" : "\n") $0
  next
}
END {
  if(qr==0 && v["sql_ok"]!="1") { qr=65; qe="SQL 结果缺少 SELECT 1 确认标记" }
  if(sr!=0) note("服务状态采集失败，返回码 " sr ": " se)
  if(nr!=0) note("监听采集失败，返回码 " nr ": " ne)
  if(qr!=0) note("SQL 指标未知，返回码 " qr ": " (qe=="" ? "查询失败或超时，不能据此认定服务停止" : qe))
  printf "{"
  emit("service_active",j(sr==0 && ("ActiveState" in svc) ? svc["ActiveState"] : "unknown"))
  emit("service_sub",j(sr==0 && ("SubState" in svc) ? svc["SubState"] : "unknown"))
  emit("pid",sr==0 && ("MainPID" in svc) ? j(svc["MainPID"]) : "null")
  emit("tcp_count",nr==0 ? count : "null")
  emit("tcp_addresses",nr==0 ? j(addresses) : "null")
  emit("sql_ok",qr==0 ? "true" : "false")
  emit("version",str("version")); emit("host",str("host"))
  emit("slow_log",str("slow_query_log")); emit("read_only",str("read_only"))
  emit("slow_seconds",num("long_query_time"))
  emit("limit_connections",num("max_connections")); emit("peak_connections",num("Max_used_connections"))
  emit("connected",num("Threads_connected")); emit("running",num("Threads_running"))
  emit("uptime",num("Uptime")); emit("questions",num("Questions")); emit("connections",num("Connections"))
  emit("slow_queries",num("Slow_queries")); emit("aborted_clients",num("Aborted_clients")); emit("aborted_connects",num("Aborted_connects"))
  emit("bytes_received",num("Bytes_received")); emit("bytes_sent",num("Bytes_sent"))
  emit("buffer_requests",num("Innodb_buffer_pool_read_requests")); emit("buffer_reads",num("Innodb_buffer_pool_reads"))
  emit("buffer_total",num("Innodb_buffer_pool_pages_total")); emit("buffer_free",num("Innodb_buffer_pool_pages_free"))
  emit("lock_waits",num("Innodb_row_lock_current_waits")); emit("lock_time",num("Innodb_row_lock_time")); emit("log_waits",num("Innodb_log_waits"))
  totals="[]"
  if(qr==0) {
    totals="[" row("SQL 查询（累计）","Questions","次")
    totals=totals "," row("连接尝试（累计）","Connections","次")
    totals=totals "," row("慢查询（累计）","Slow_queries","次")
    totals=totals "," row("异常断连（累计）","Aborted_clients","次")
    totals=totals "," row("连接失败（累计）","Aborted_connects","次")
    totals=totals "," row("接收数据（累计）","Bytes_received","B")
    totals=totals "," row("发送数据（累计）","Bytes_sent","B")
    totals=totals "," row("缓冲池读请求（累计）","Innodb_buffer_pool_read_requests","次")
    totals=totals "," row("缓冲池磁盘读取（累计）","Innodb_buffer_pool_reads","次")
    totals=totals "," row("行锁等待时间（累计）","Innodb_row_lock_time","ms")
    totals=totals "," row("日志缓冲等待（累计）","Innodb_log_waits","次") "]"
  }
  emit("totals",totals)
  if(overflow) note("部分计数超过安全整数范围：表格保留精确累计值，相关趋势与速率显示未知。")
  emit("error",j(errors))
  printf "}\n"
}'
)
