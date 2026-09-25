package catalog

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

type SecretStore interface {
	Get(ref string) (string, error)
	Set(ref, secret string) error
	Delete(ref string) error
}

type OSSecrets struct{ Service string }

func (s OSSecrets) service() string {
	if s.Service != "" {
		return s.Service
	}
	return "com.wrdtech.aissh.credentials"
}
func (s OSSecrets) Get(ref string) (string, error) {
	if err := ValidateID(ref); err != nil {
		return "", err
	}
	value, err := keyring.Get(s.service(), ref)
	if err != nil {
		return "", errors.New("无法读取系统凭据库：请解锁钥匙串，或在桌面端重新保存此凭据")
	}
	return value, nil
}
func (s OSSecrets) Set(ref, value string) error {
	if err := ValidateID(ref); err != nil {
		return err
	}
	// The macOS security tool has a 4096-byte input line limit. Keep the
	// cross-platform limit below it even after the keyring's base64 encoding.
	if len(value) == 0 || len(value) > 2048 {
		return errors.New("密码或 Token 长度必须为 1–2048 字节")
	}
	if err := keyring.Set(s.service(), ref, value); err != nil {
		return fmt.Errorf("无法保存到系统凭据库；请检查钥匙串或 Secret Service 是否已解锁")
	}
	return nil
}
func (s OSSecrets) Delete(ref string) error {
	if err := ValidateID(ref); err != nil {
		return err
	}
	err := keyring.Delete(s.service(), ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errors.New("无法删除系统凭据，请解锁系统凭据库后重试")
	}
	return nil
}
