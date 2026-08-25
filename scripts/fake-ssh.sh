#!/bin/sh

last=''
for value in "$@"; do
  last=$value
done

case "$last" in
  *"HOST="*|*"ps -eo"*|*"df -hP"*|*"ip -s link"*|*"systemctl --failed"*)
    printf 'HOST=fake-server\n'
    printf 'UPTIME= 10:30 up 12 days, 2 users, load averages: 0.20 0.15 0.10\n'
    printf 'LOAD=0.20 0.15 0.10\n'
    printf 'MEM=512/2048 MB (25.0%%)\n'
    printf 'DISK=8G/40G (20%%)\n'
    printf 'PROCS=87\n'
    exit 0
    ;;
esac

printf '\033[32mConnected to fake-server\033[0m\r\n'
printf 'root@fake-server:~# '
while IFS= read -r line; do
  case "$line" in
    exit|logout)
      printf 'logout\r\n'
      exit 0
      ;;
    missing-command)
      printf 'bash: missing-command: command not found\r\n'
      ;;
    clear)
      printf '\033[2J\033[H'
      ;;
    *)
      printf 'executed: %s\r\n' "$line"
      ;;
  esac
  printf 'root@fake-server:~# '
done
