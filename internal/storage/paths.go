package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateID(id string) error {
	if id == "" || len(id) > 128 {
		return fmt.Errorf("案件编号长度无效")
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return fmt.Errorf("案件编号包含非法字符")
		}
	}
	return nil
}

func (s *Store) caseDir(caseID string) (string, error) {
	if err := validateID(caseID); err != nil {
		return "", err
	}
	return filepath.Join(s.root, "cases", caseID), nil
}

func ensureDir(path string) error { return os.MkdirAll(path, 0750) }

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func digestBytes(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func trimLine(line []byte) []byte { return []byte(strings.TrimSpace(string(line))) }
