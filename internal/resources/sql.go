package resources

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

type pqDialer struct {
	ctx  context.Context
	dial dialFunc
}

func (d pqDialer) Dial(network, address string) (net.Conn, error) {
	return d.dial(d.ctx, network, address)
}
func (d pqDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(d.ctx, timeout)
	defer cancel()
	return d.dial(ctx, network, address)
}
func (d pqDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dial(ctx, network, address)
}

func executeSQL(ctx context.Context, r catalog.Resource, op Operation, c catalog.Credential, secret string, dial dialFunc) (string, error) {
	var db *sql.DB
	if r.Kind == "postgres" {
		u := &url.URL{Scheme: "postgres", Host: r.Endpoint, Path: "/" + r.Database, User: url.UserPassword(c.Username, secret)}
		q := u.Query()
		q.Set("sslmode", "disable")
		if r.TLS {
			q.Set("sslmode", "verify-full")
		}
		q.Set("connect_timeout", "8")
		u.RawQuery = q.Encode()
		connector, err := pq.NewConnector(u.String())
		if err != nil {
			return "", errors.New("数据库连接配置无效")
		}
		connector.Dialer(pqDialer{ctx, dial})
		db = sql.OpenDB(connector)
	} else {
		name := "aissh-" + catalog.NewID()
		mysql.RegisterDialContext(name, func(ctx context.Context, addr string) (net.Conn, error) { return dial(ctx, "tcp", addr) })
		defer mysql.DeregisterDialContext(name)
		config := mysql.NewConfig()
		config.User = c.Username
		config.Passwd = secret
		config.Net = name
		config.Addr = r.Endpoint
		config.DBName = r.Database
		config.Timeout = 8 * time.Second
		config.ReadTimeout = 30 * time.Second
		config.WriteTimeout = 10 * time.Second
		config.MultiStatements = false
		config.AllowNativePasswords = true
		if r.TLS {
			hostname, _, _ := net.SplitHostPort(r.Endpoint)
			config.TLS = &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}
		}
		connector, err := mysql.NewConnector(config)
		if err != nil {
			return "", errors.New("数据库连接配置无效")
		}
		db = sql.OpenDB(connector)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, op.Query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	result := struct {
		Columns   []string `json:"columns"`
		Rows      [][]any  `json:"rows"`
		Truncated bool     `json:"truncated"`
	}{Columns: columns, Rows: [][]any{}}
	bytesUsed := 0
	for rows.Next() {
		if len(result.Rows) >= 100 || bytesUsed >= 128*1024 {
			result.Truncated = true
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return "", err
		}
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}
		encoded, _ := json.Marshal(values)
		bytesUsed += len(encoded)
		if bytesUsed > 128*1024 {
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	return string(encoded), err
}
