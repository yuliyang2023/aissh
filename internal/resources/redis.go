package resources

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
)

func validateRedis(args []string) error {
	if len(args) == 0 {
		return errors.New("请输入 Redis 只读命令")
	}
	count := map[string]int{"PING": 1, "INFO": 1, "DBSIZE": 1, "GET": 2, "TTL": 2, "PTTL": 2, "TYPE": 2, "STRLEN": 2, "SCARD": 2, "LLEN": 2, "HLEN": 2, "ZCARD": 2}
	n, ok := count[strings.ToUpper(args[0])]
	if !ok || len(args) != n {
		return errors.New("只支持 PING、INFO、DBSIZE、GET、TTL、PTTL、TYPE、STRLEN、SCARD、LLEN、HLEN、ZCARD")
	}
	for _, arg := range args {
		if len(arg) > 4096 {
			return errors.New("Redis 参数过长")
		}
	}
	return nil
}
func executeRedis(ctx context.Context, r catalog.Resource, op Operation, c catalog.Credential, secret string, dial dialFunc) (string, error) {
	conn, err := dial(ctx, "tcp", r.Endpoint)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if r.TLS {
		host, _, _ := net.SplitHostPort(r.Endpoint)
		secure := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			return "", err
		}
		conn = secure
	}
	_ = conn.SetDeadline(time.Now().Add(25 * time.Second))
	// Context cancellation also closes an SSH pipe, whose deadlines are owned
	// by its SSH subprocess rather than a local TCP socket.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	reader := bufio.NewReader(io.LimitReader(conn, 256*1024))
	call := func(args []string) (any, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "*%d\r\n", len(args))
		for _, arg := range args {
			fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
		}
		if _, err := io.WriteString(conn, b.String()); err != nil {
			return nil, err
		}
		return readRESP(reader, 0)
	}
	if secret != "" {
		auth := []string{"AUTH", secret}
		if c.Username != "" {
			auth = []string{"AUTH", c.Username, secret}
		}
		if _, err := call(auth); err != nil {
			return "", errors.New("Redis 认证失败，请更新凭据")
		}
	}
	if r.Database != "" {
		n, err := strconv.Atoi(r.Database)
		if err != nil || n < 0 {
			return "", errors.New("Redis 数据库应为非负整数")
		}
		if _, err := call([]string{"SELECT", r.Database}); err != nil {
			return "", err
		}
	}
	result, err := call(op.Args)
	if err != nil {
		return "", err
	}
	if s, ok := result.(string); ok {
		return s, nil
	}
	data, err := json.MarshalIndent(result, "", "  ")
	return string(data), err
}
func readRESP(reader *bufio.Reader, depth int) (any, error) {
	if depth > 8 {
		return nil, errors.New("Redis 响应嵌套过深")
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || !strings.HasSuffix(line, "\r\n") {
		return nil, errors.New("无效的 Redis 响应")
	}
	value := line[1 : len(line)-2]
	switch line[0] {
	case '+':
		return value, nil
	case '-':
		return nil, fmt.Errorf("Redis: %s", value)
	case ':':
		return strconv.ParseInt(value, 10, 64)
	case '$':
		n, err := strconv.Atoi(value)
		if err != nil || n < -1 || n > 128*1024 {
			return nil, errors.New("Redis 响应过大或无效")
		}
		if n == -1 {
			return nil, nil
		}
		data := make([]byte, n+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		if string(data[n:]) != "\r\n" {
			return nil, errors.New("无效的 Redis 数据")
		}
		return string(data[:n]), nil
	case '*':
		n, err := strconv.Atoi(value)
		if err != nil || n < -1 || n > 100 {
			return nil, errors.New("Redis 响应过大或无效")
		}
		if n == -1 {
			return nil, nil
		}
		items := make([]any, n)
		for i := range items {
			items[i], err = readRESP(reader, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return items, nil
	}
	return nil, errors.New("未知的 Redis 响应")
}
