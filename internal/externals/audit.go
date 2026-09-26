package externals

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Consume serializes grants across desktop and SSH processes. A failed or
// uncertain attempt consumes its use too; it is never replayed automatically.
func (s *Store) Consume(id string, revision uint64, grantID string, inv Invocation) error {
	l, err := s.lock(id)
	if err != nil {
		return err
	}
	defer l.Unlock()
	st, err := s.Read(id)
	if err != nil {
		return err
	}
	if st.Revision != revision {
		return errors.New("执行前目录或规则已改变，请重新预览")
	}
	i := slices.IndexFunc(st.Grants, func(g Grant) bool { return g.ID == grantID })
	if i < 0 || !ActiveGrant(st.Grants[i], time.Now()) {
		return errors.New("授权已撤销、到期或耗尽")
	}
	inv.Outcome = "started"
	inv.Time = Now()
	// Audit must be durable before dispatch. Crashes after this point leave an
	// explicit uncertain attempt, never an invented successful result.
	if err := s.appendAudit(id, inv); err != nil {
		return err
	}
	if st.Grants[i].MaxUses > 0 {
		st.Grants[i].Used++
		if err := s.write(st); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) appendAudit(id string, inv Invocation) error {
	path := filepath.Join(s.dir(id), "invocations.jsonl")
	if info, err := os.Stat(path); err == nil && info.Size() > 8*1024*1024 {
		if err := os.Rename(path, path+".1"); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(inv); err != nil {
		return err
	}
	return f.Sync()
}
func (s *Store) Record(id string, inv Invocation) error {
	l, err := s.lock(id)
	if err != nil {
		return err
	}
	defer l.Unlock()
	inv.Time = Now()
	return s.appendAudit(id, inv)
}
func (s *Store) Invocations(id string) ([]Invocation, error) {
	l, err := s.lock(id)
	if err != nil {
		return nil, err
	}
	defer l.Unlock()
	out := []Invocation{}
	f, err := os.Open(filepath.Join(s.dir(id), "invocations.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		var inv Invocation
		if err := json.Unmarshal(scanner.Bytes(), &inv); err != nil {
			return nil, err
		}
		out = append(out, inv)
		if len(out) > 200 {
			out = out[1:]
		}
	}
	slices.Reverse(out)
	return out, scanner.Err()
}
