package agentprinter

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var installationIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// InstallationDirectory isolates new-system print attempts by the persistent
// Server installation identifier. Changing the Server address, factory code,
// scale code or Agent label does not change this path. Earlier installations'
// ledgers are left untouched and are not imported into the new empty system.
func InstallationDirectory(root, installationID string) (string, error) {
	if !installationIDPattern.MatchString(installationID) {
		return "", fmt.Errorf("服务端安装实例标识无效")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir := filepath.Join(root, "installation-"+installationID)
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("打印账本目录无效")
	}
	d, err := os.Open(root)
	if err != nil {
		return "", err
	}
	err = d.Sync()
	_ = d.Close()
	if err != nil {
		return "", err
	}
	return dir, nil
}
