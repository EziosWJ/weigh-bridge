package weighing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// A ticket number must never be reused after an older database backup is
// restored: the restored wb_number_sequence is behind the numbers that were
// already issued. The database floor scan only knows rows that still exist, so
// the last issued serial is also kept in a small file next to the business
// images. Restoring only the database therefore cannot lower the counter
// (ADR 0033). If the whole data directory is restored or lost, the
// configuration-management "流水号水位" entry lets an administrator raise the
// serial after verifying the printed tickets.
const (
	watermarkFile     = "ticket-watermark.json"
	watermarkLockFile = "ticket-watermark.lock"
)

var errWatermarkCorrupt = errors.New("流水号水位文件损坏或为空，请检查数据目录与备份后再发号")

type watermarkKey struct {
	Kind     string `json:"kind"`
	SiteCode string `json:"siteCode"`
	Date     string `json:"businessDate"`
}

func watermarkPath(dir string) string { return filepath.Join(dir, watermarkFile) }

// readWatermarks reads the whole floor map. A missing file means no serial has
// been recorded yet; an empty or unparsable file fails closed instead of
// silently resetting all floors, because that file is the only protection when
// a database-only backup is restored.
func readWatermarks(dir string) (map[string]int64, error) {
	values := map[string]int64{}
	if dir == "" {
		return values, nil
	}
	data, err := os.ReadFile(watermarkPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errWatermarkCorrupt
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("%w: %v", errWatermarkCorrupt, err)
	}
	return values, nil
}

func (k watermarkKey) key() string { return k.Kind + "|" + k.SiteCode + "|" + k.Date }

// watermarkFloor reports the highest serial this installation has issued for
// the key, including numbers the current database no longer contains.
func watermarkFloor(dir string, key watermarkKey) (int64, error) {
	if dir == "" {
		return 0, nil
	}
	values, err := readWatermarks(dir)
	if err != nil {
		return 0, err
	}
	return values[key.key()], nil
}

// raiseWatermark records an issued serial. Writers serialize on a dedicated
// lock file and publish an atomically renamed temp file, so a crash can never
// leave a truncated or empty floor file behind. The merge is monotonic:
// concurrent issuers cannot lower a recorded serial.
func raiseWatermark(dir string, key watermarkKey, serial int64) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, watermarkLockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	values, err := readWatermarks(dir)
	if err != nil {
		return err
	}
	if serial <= values[key.key()] {
		return nil
	}
	values[key.key()] = serial
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ticket-watermark-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmpName, watermarkPath(dir)); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.Sync()
}
