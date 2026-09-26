package widgets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

type SavedVersion struct {
	Version    int        `json:"version"`
	Time       string     `json:"time"`
	Definition Definition `json:"definition"`
}
type Saved struct {
	Key      string         `json:"key"`
	HostID   string         `json:"hostId"`
	Title    string         `json:"title"`
	Versions []SavedVersion `json:"versions"`
}

var savedKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

func StoreKey(host, id string) string {
	sum := sha256.Sum256([]byte(host + "\x00" + id))
	return hex.EncodeToString(sum[:])
}
func LoadSaved(dir, key string) (Saved, error) {
	var result Saved
	if !savedKey.MatchString(key) {
		return result, errors.New("无效组件编号")
	}
	f, err := os.Open(filepath.Join(dir, key+".json"))
	if err != nil {
		return result, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 3<<20+1))
	if err != nil || len(b) > 3<<20 {
		return result, errors.New("组件存档超过限制")
	}
	err = json.Unmarshal(b, &result)
	if err == nil && (result.Key != key || len(result.Versions) > 10) {
		err = errors.New("组件存档无效")
	}
	return result, err
}
func Save(dir, host string, d Definition) error {
	if err := d.View.Validate(); err != nil {
		return err
	}
	if err := d.Contract.Validate(); err != nil {
		return err
	}
	if len(d.Command) > 64<<10 || len(d.Title) > 256 {
		return errors.New("组件定义超过限制")
	}
	key := StoreKey(host, d.ID)
	s, err := LoadSaved(dir, key)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	s.Key = key
	s.HostID = host
	s.Title = d.Title
	if len(s.Versions) > 0 {
		last := s.Versions[len(s.Versions)-1]
		a, _ := json.Marshal(last.Definition)
		b, _ := json.Marshal(d)
		if string(a) == string(b) {
			return nil
		}
	}
	version := 1
	if len(s.Versions) > 0 {
		version = s.Versions[len(s.Versions)-1].Version + 1
	}
	s.Versions = append(s.Versions, SavedVersion{Version: version, Time: time.Now().UTC().Format(time.RFC3339Nano), Definition: d})
	if len(s.Versions) > 10 {
		s.Versions = s.Versions[len(s.Versions)-10:]
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	total := int64(len(b))
	for _, e := range entries {
		if e.Name() != key+".json" {
			if stat, err := e.Info(); err == nil {
				total += stat.Size()
			}
		}
	}
	if total > 100<<20 {
		return errors.New("组件存档已达到 100 MiB 上限")
	}
	f, err := os.CreateTemp(dir, ".widget-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(dir, key+".json"))
}
func ListSaved(dir, host string) ([]Saved, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Saved{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Saved{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		key := e.Name()[:len(e.Name())-5]
		s, err := LoadSaved(dir, key)
		if err != nil {
			return nil, err
		}
		if s.HostID == host {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result, nil
}
