package externals

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Prepared struct {
	Kind      string   `json:"kind"`
	Method    string   `json:"method,omitempty"`
	Path      string   `json:"path,omitempty"`
	Body      string   `json:"body,omitempty"`
	SQL       string   `json:"sql,omitempty"`
	SQLValues []any    `json:"sqlValues,omitempty"`
	RedisArgs []string `json:"redisArgs,omitempty"`
	ReadOnly  bool     `json:"readOnly"`
}

var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

func Prepare(d Definition, args map[string]any) (Prepared, error) {
	p := Prepared{Kind: d.Kind, ReadOnly: d.Assessment.Effects.Access == "read"}
	if d.Blocked != "" {
		return p, fmt.Errorf("接口定义尚不完整：%s", d.Blocked)
	}
	if err := ValidateArguments(d.InputSchema, args); err != nil {
		return p, err
	}
	r := d.Request
	switch d.Kind {
	case "http":
		p.Method = strings.ToUpper(r.Method)
		if !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}, p.Method) {
			return p, errors.New("不支持的 HTTP 方法")
		}
		u, err := url.Parse(r.Path)
		if err != nil || !strings.HasPrefix(r.Path, "/") || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(r.Path, "\\\r\n") {
			return p, errors.New("接口路径必须是固定连接内的路径模板")
		}
		for _, seg := range strings.Split(u.Path, "/") {
			if seg == ".." || seg == "." {
				return p, errors.New("接口路径不能包含相对跳转")
			}
		}
		if p.Method == "DELETE" && (!d.Assessment.Effects.Destructive || p.ReadOnly) {
			return p, errors.New("DELETE 必须声明写入和删除效果")
		}
		p.Path = r.Path
		q := url.Values{}
		body := map[string]any{}
		used := map[string]bool{}
		for _, param := range r.Parameters {
			value, exists := args[param.Input]
			if !exists {
				continue
			}
			used[param.Input] = true
			switch param.In {
			case "path":
				s, ok := scalar(value)
				if !ok || s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/%\\?#\r\n") {
					return p, errors.New("路径参数必须是单一路径段")
				}
				marker := "{" + param.Name + "}"
				if !strings.Contains(p.Path, marker) {
					return p, errors.New("路径参数不存在于模板")
				}
				p.Path = strings.ReplaceAll(p.Path, marker, url.PathEscape(s))
			case "query":
				s, ok := scalar(value)
				if !ok {
					return p, errors.New("查询参数暂只支持标量")
				}
				q.Set(param.Name, s)
			case "body":
				body[param.Name] = value
			default:
				return p, errors.New("不支持的参数位置")
			}
		}
		if placeholder.MatchString(p.Path) || strings.ContainsAny(p.Path, "{}") {
			return p, errors.New("缺少必填路径参数")
		}
		if r.BodyInput != "" {
			v, ok := args[r.BodyInput]
			if ok {
				if len(body) > 0 {
					return p, errors.New("请求体映射冲突")
				}
				raw, err := json.Marshal(v)
				if err != nil {
					return p, err
				}
				p.Body = string(raw)
				used[r.BodyInput] = true
			}
		}
		if len(body) > 0 {
			raw, _ := json.Marshal(body)
			p.Body = string(raw)
		}
		for key := range args {
			if !used[key] {
				return p, fmt.Errorf("参数 %s 未映射到实际请求", key)
			}
		}
		if (p.Method == "GET" || p.Method == "HEAD") && p.Body != "" {
			return p, errors.New("GET/HEAD 不支持请求体")
		}
		if len(q) > 0 {
			p.Path += "?" + q.Encode()
		}
	case "postgres", "mysql":
		p.SQL = r.Query
		if len(p.SQL) == 0 || len(p.SQL) > 16384 || strings.Contains(p.SQL, ";") || strings.Contains(p.SQL, "--") || strings.Contains(p.SQL, "/*") {
			return p, errors.New("SQL 模板只支持不含注释的单条语句")
		}
		upper := strings.ToUpper(strings.TrimSpace(p.SQL))
		if p.ReadOnly && !strings.HasPrefix(upper, "SELECT ") {
			return p, errors.New("只读 SQL 模板必须为 SELECT")
		}
		if !p.ReadOnly && !(strings.HasPrefix(upper, "INSERT ") || strings.HasPrefix(upper, "UPDATE ") || strings.HasPrefix(upper, "DELETE ")) {
			return p, errors.New("写入 SQL 仅支持 INSERT/UPDATE/DELETE")
		}
		if !p.ReadOnly && d.Kind == "mysql" {
			return p, errors.New("MySQL 写入暂未启用：需先确认表使用可事务回滚的存储引擎")
		}
		if strings.HasPrefix(upper, "DELETE ") && !d.Assessment.Effects.Destructive {
			return p, errors.New("DELETE SQL 必须声明删除效果")
		}
		used := map[string]bool{}
		for _, name := range r.SQLParameters {
			v, ok := args[name]
			if !ok {
				return p, fmt.Errorf("缺少 SQL 参数 %s", name)
			}
			if _, ok := scalar(v); !ok && v != nil {
				return p, errors.New("SQL 参数必须是标量")
			}
			p.SQLValues = append(p.SQLValues, v)
			used[name] = true
		}
		for name := range args {
			if !used[name] {
				return p, fmt.Errorf("SQL 参数 %s 未绑定", name)
			}
		}
	case "redis":
		command := strings.ToUpper(r.RedisCommand)
		reads := []string{"PING", "INFO", "DBSIZE", "GET", "TTL", "PTTL", "TYPE", "STRLEN", "SCARD", "LLEN", "HLEN", "ZCARD"}
		writes := []string{"SET", "DEL", "UNLINK", "EXPIRE", "PERSIST"}
		if p.ReadOnly && !slices.Contains(reads, command) || !p.ReadOnly && !slices.Contains(writes, command) {
			return p, errors.New("Redis 命令不在所选效果的适配器白名单中")
		}
		if slices.Contains([]string{"DEL", "UNLINK"}, command) && !d.Assessment.Effects.Destructive {
			return p, errors.New("删除命令必须声明删除效果")
		}
		p.RedisArgs = []string{command}
		used := map[string]bool{}
		for _, name := range r.RedisParameters {
			value, ok := args[name]
			if !ok {
				return p, fmt.Errorf("缺少 Redis 参数 %s", name)
			}
			s, ok := scalar(value)
			if !ok || len(s) > 4096 {
				return p, errors.New("Redis 参数无效")
			}
			p.RedisArgs = append(p.RedisArgs, s)
			used[name] = true
		}
		for name := range args {
			if !used[name] {
				return p, fmt.Errorf("Redis 参数 %s 未绑定", name)
			}
		}
		count := map[string]int{"PING": 1, "INFO": 1, "DBSIZE": 1, "GET": 2, "TTL": 2, "PTTL": 2, "TYPE": 2, "STRLEN": 2, "SCARD": 2, "LLEN": 2, "HLEN": 2, "ZCARD": 2, "SET": 3, "DEL": 2, "UNLINK": 2, "EXPIRE": 3, "PERSIST": 2}
		if command == "EXPIRE" && len(p.RedisArgs) == 3 {
			seconds, err := strconv.ParseInt(p.RedisArgs[2], 10, 64)
			if err != nil || seconds < 1 {
				return p, errors.New("EXPIRE 秒数必须为正整数，禁止通过非正数立即删除")
			}
		}
		if len(p.RedisArgs) != count[command] {
			return p, errors.New("Redis 接口参数数量不匹配；写接口仅允许单键操作")
		}
	default:
		return p, errors.New("不支持的适配器")
	}
	return p, nil
}
func scalar(v any) (string, bool) {
	switch v := v.(type) {
	case string:
		return v, true
	case bool:
		return fmt.Sprint(v), true
	case float64:
		return fmt.Sprint(v), true
	case int:
		return fmt.Sprint(v), true
	case json.Number:
		return string(v), true
	}
	return "", false
}
