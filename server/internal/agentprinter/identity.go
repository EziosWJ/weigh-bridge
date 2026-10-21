package agentprinter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// IdentityDirectory binds pre-existing root-level attempt files to the first
// configured identity, then isolates later Server/Agent/scale identities.
func IdentityDirectory(root, server, agent string, scale int64) (string, error) {
	canonical, err := CanonicalServerURL(server)
	if err != nil {
		return "", err
	}
	identity := canonical + "\x00" + agent + "\x00" + strconv.FormatInt(scale, 10)
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	meta := filepath.Join(root, ".attempt-identity.json")
	if b, e := os.ReadFile(meta); e == nil {
		var old string
		if json.Unmarshal(b, &old) != nil {
			return "", fmt.Errorf("invalid attempt identity metadata")
		}
		if old == identity {
			return root, nil
		}
	} else if !os.IsNotExist(e) {
		return "", e
	} else {
		// Existing flat attempts have no identity metadata. Bind them once to
		// this first configured identity, preserving every historical dedupe key.
		b, _ := json.Marshal(identity)
		f, e := os.OpenFile(meta, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			_, e = f.Write(b)
			if e == nil {
				e = f.Sync()
			}
			_ = f.Close()
			if e != nil {
				return "", e
			}
			return root, nil
		}
		if !os.IsExist(e) {
			return "", e
		}
		b, e = os.ReadFile(meta)
		if e != nil {
			return "", e
		}
		var old string
		if json.Unmarshal(b, &old) == nil && old == identity {
			return root, nil
		}
	}
	h := sha256.Sum256([]byte(identity))
	dir := filepath.Join(root, "identity-"+hex.EncodeToString(h[:12]))
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	nested := filepath.Join(dir, ".attempt-identity.json")
	if b, e := os.ReadFile(nested); e == nil {
		var old string
		if json.Unmarshal(b, &old) != nil || old != identity {
			return "", fmt.Errorf("attempt identity directory mismatch")
		}
	} else if os.IsNotExist(e) {
		b, _ := json.Marshal(identity)
		if e = os.WriteFile(nested, b, 0600); e != nil {
			return "", e
		}
	} else {
		return "", e
	}
	return dir, nil
}

// CanonicalServerURL preserves the API base path while making equivalent URL
// spellings share the same durable attempt ledger. Query-based routing is not
// part of the Agent protocol and must not create a new print identity.
func CanonicalServerURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("Server 地址必须为不含用户信息、查询参数或片段的 HTTP(S) API 基址")
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}
