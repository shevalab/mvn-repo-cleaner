package resolve

import (
	"os"
	"strconv"
	"strings"
)

func openDir(dir string) (*os.File, error) {
	return os.Open(dir)
}

// compareVersions compares two Maven version strings. It splits on '.', '-' and
// trims leading zeros for numeric components; non-numeric parts compare
// lexically. A trailing qualifier sorts by a rough Maven ordering.
func compareVersions(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ca, cb string
		if i < len(pa) {
			ca = pa[i]
		}
		if i < len(pb) {
			cb = pb[i]
		}
		if ca == cb {
			continue
		}
		an, aok := toInt(ca)
		bn, bok := toInt(cb)
		switch {
		case aok && bok:
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		case aok && !bok:
			return 1 // numeric > qualifier
		case !aok && bok:
			return -1
		default:
			if ca < cb {
				return -1
			}
			return 1
		}
	}
	if len(pa) < len(pb) {
		return -1
	}
	if len(pa) > len(pb) {
		return 1
	}
	return 0
}

func splitVersion(v string) []string {
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, strings.TrimSpace(f))
	}
	return out
}

func toInt(s string) (int, bool) {
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return 0, true
	}
	if trimmed[0] < '0' || trimmed[0] > '9' {
		return 0, false
	}
	n, err := strconv.Atoi(trimmed)
	return n, err == nil
}

func readDirNames(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}
