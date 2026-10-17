package entitle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Manifest is what an installation verifies before it offers an update: the release, signed with the same key as
// entitlements. The images are digests, so what is pulled is exactly what was published.
type Manifest struct {
	Version        string            `json:"version"`
	Channel        string            `json:"channel"`
	PublishedAt    time.Time         `json:"published_at"`
	HasMigrations  bool              `json:"has_migrations"`
	MinFromVersion string            `json:"min_from_version,omitempty"`
	Images         map[string]string `json:"images"`
	Notes          string            `json:"notes"`
}

// SignManifest makes the signed document for a release, in the same envelope as an entitlement.
func (s *Signer) SignManifest(m Manifest) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	canon, err := Canonical(raw)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(s.priv, canon)
	return json.MarshalIndent(Bundle{Payload: canon, Signature: base64.StdEncoding.EncodeToString(sig)}, "", "  ")
}

// CompareVersions orders two release versions like 1.4.2 or v1.4.2-rc1: numbers by value, a pre-release before its release.
// Anything that is not numbers separated by dots compares as text.
func CompareVersions(a, b string) int {
	pa, qa, oka := splitVersion(a)
	pb, qb, okb := splitVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int64
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case qa == qb:
		return 0
	case qa == "":
		return 1
	case qb == "":
		return -1
	}
	return strings.Compare(qa, qb)
}

func splitVersion(v string) (nums []int64, pre string, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	if core == "" {
		return nil, "", false
	}
	for _, part := range strings.Split(core, ".") {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n < 0 {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	return nums, pre, true
}

// ValidVersion says whether v looks like a release version.
func ValidVersion(v string) bool {
	_, _, ok := splitVersion(v)
	return ok && len(v) <= 40
}

// Activation codes are 20 characters from an alphabet without look-alikes, shown as ACT-XXXXX-XXXXX-XXXXX-XXXXX: 100 bits,
// so they cannot be guessed. Only the hash is kept.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewActivationCode returns a code to show once and the hash to store.
func NewActivationCode() (code, hash, hint string, err error) {
	raw := make([]byte, 20)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := range raw {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", "", "", err
		}
		raw[i] = codeAlphabet[n.Int64()]
	}
	s := string(raw)
	code = "ACT-" + s[0:5] + "-" + s[5:10] + "-" + s[10:15] + "-" + s[15:20]
	h, err := HashActivationCode(code)
	return code, h, s[15:20], err
}

// HashActivationCode normalizes what a person typed (case, dashes, spaces, the ACT prefix) and hashes it.
func HashActivationCode(code string) (string, error) {
	c := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
	if len(c) == 23 && strings.HasPrefix(c, "ACT") {
		c = c[3:] // the prefix is optional; a code that happens to begin with ACT keeps its letters
	}
	if len(c) != 20 || strings.Trim(c, codeAlphabet) != "" {
		return "", errors.New("entitle: not an activation code")
	}
	return hashHex(c), nil
}
