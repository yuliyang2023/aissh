sh -c '
date
printf "\n=== 服务状态 ===\n"
mysql_service_found=0
for mysql_unit in mysqld.service mysql.service mariadb.service; do
  mysql_load_state=$(systemctl show "$mysql_unit" -p LoadState --value 2>/dev/null)
  if [ "$mysql_load_state" = loaded ]; then
    mysql_service_found=1
    systemctl show "$mysql_unit" -p Id -p ActiveState -p SubState -p MainPID -p NRestarts
  fi
done
if [ "$mysql_service_found" = 0 ]; then printf "未发现常见 MySQL 服务单元，请结合进程状态判断\n"; fi
printf "\n=== 进程资源（CPU 为存活期平均占用率，RSS 单位 KiB）===\n"
ps -C mysqld,mariadbd -o pid,comm,etime,pcpu,pmem,rss,nlwp,stat || printf "未发现数据库进程，或无法读取进程信息\n"
printf "\n=== TCP 监听端口 ===\n"
if command -v ss >/dev/null 2>&1; then
  ss -lntp | awk "NR == 1 || /mysqld|mariadbd/"
else
  printf "缺少 ss，无法读取监听端口\n"
fi
printf "\n=== 数据库指标（查询及连接计数为累计值）===\n"
if command -v mysql >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
  if mysql_metrics=$(timeout 5s mysql --protocol=socket --host=localhost --connect-timeout=3 --batch --skip-column-names --skip-reconnect --execute="SHOW GLOBAL STATUS" </dev/null 2>&1); then
    printf "%s\n" "$mysql_metrics" | awk "/^(Uptime|Threads_connected|Threads_running|Max_used_connections|Connections|Queries|Slow_queries|Aborted_connects|Aborted_clients|Innodb_row_lock_current_waits)[[:space:]]/"
  else
    printf "数据库指标读取失败（可能为连接、认证问题或超时）：\n%s\n" "$mysql_metrics"
  fi
else
  printf "缺少 mysql 客户端或 timeout，未采集数据库内部指标\n"
fi
'
