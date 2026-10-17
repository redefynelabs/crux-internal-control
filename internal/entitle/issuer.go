package entitle

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// ErrNoSigner: the control plane has no signing key, so it cannot issue anything.
var ErrNoSigner = errors.New("entitle: no signing key is configured (CP_ENTITLEMENT_KEY_FILE)")

// Draft is what a person chooses in the editor.
type Draft struct {
	// License is "subscription" or "lifetime". Empty issues a file without the field, as before.
	License         string           `json:"license"`
	UpdatesUntil    *time.Time       `json:"updates_until"`
	Suspended       bool             `json:"suspended"`
	ControlPlaneURL string           `json:"control_plane_url"`
	Template        string           `json:"template"`
	Apps            []string         `json:"apps"`
	Features        []string         `json:"features"`
	Labels          map[string]Label `json:"labels"`
	ExpiresAt       *time.Time       `json:"expires_at"`
}

// Issuer signs entitlements for installations and keeps them.
type Issuer struct {
	store  *store.Store
	signer *Signer
	now    func() time.Time
}

// NewIssuer returns an issuer. signer may be nil: reading still works, issuing answers ErrNoSigner.
func NewIssuer(st *store.Store, signer *Signer) *Issuer {
	return &Issuer{store: st, signer: signer, now: func() time.Time { return time.Now().UTC() }}
}

// Signer returns the key holder, nil when there is none.
func (i *Issuer) Signer() *Signer { return i.signer }

// Issue signs the next version for an installation. Requests named in approve become approved, in the same transaction.
// The declined requests of the installation travel in the file, unless the draft now grants what they asked for.
func (i *Issuer) Issue(ctx context.Context, installationID string, d Draft, by string, approve []string) (store.Issued, error) {
	if i.signer == nil {
		return store.Issued{}, ErrNoSigner
	}
	inst, err := i.store.GetInstallation(ctx, installationID)
	if err != nil {
		return store.Issued{}, err
	}
	version, err := i.store.NextVersion(ctx, installationID)
	if err != nil {
		return store.Issued{}, err
	}
	p := Payload{
		Version: version, IssuedAt: i.now().Truncate(time.Second), ExpiresAt: d.ExpiresAt,
		Customer: Customer{ID: inst.ID, Name: inst.Name},
		Template: d.Template, Apps: d.Apps, Features: d.Features, Labels: d.Labels,
		UpdatesUntil: d.UpdatesUntil, Suspended: d.Suspended, ControlPlaneURL: d.ControlPlaneURL,
	}
	if d.License != "" {
		p.License = &License{Type: d.License}
	}
	if p.UpdatesUntil != nil {
		t := p.UpdatesUntil.UTC().Truncate(time.Second)
		p.UpdatesUntil = &t
	}
	if p.ExpiresAt != nil {
		t := p.ExpiresAt.UTC().Truncate(time.Second)
		p.ExpiresAt = &t
	}
	if err := p.Validate(); err != nil {
		return store.Issued{}, err
	}
	declined, err := i.store.DeclinedFor(ctx, installationID)
	if err != nil {
		return store.Issued{}, err
	}
	for _, r := range declined {
		if !slices.Contains(p.Apps, r.Name) && !slices.Contains(p.Features, r.Name) {
			p.Declined = append(p.Declined, Declined{Name: r.Name, Note: r.DecidedNote})
		}
	}
	file, payload, err := i.signer.Sign(p)
	if err != nil {
		return store.Issued{}, err
	}
	if err := i.store.SaveIssued(ctx, installationID, version, payload, string(file), by, approve); err != nil {
		return store.Issued{}, err
	}
	return store.Issued{Version: version, Payload: payload, File: string(file), IssuedAt: p.IssuedAt}, nil
}

// Reissue signs the newest file again with the current list of declined requests, so a decline reaches the customer.
// It does nothing when nothing was issued yet: the decline then travels in the first file.
func (i *Issuer) Reissue(ctx context.Context, installationID, by string) error {
	if i.signer == nil {
		return nil
	}
	last, err := i.store.LatestIssued(ctx, installationID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var p Payload
	if err := json.Unmarshal(last.Payload, &p); err != nil {
		return err
	}
	_, err = i.Issue(ctx, installationID, DraftOf(p), by, nil)
	return err
}

// DraftOf is the draft that would issue the payload again unchanged.
func DraftOf(p Payload) Draft {
	d := Draft{Template: p.Template, Apps: p.Apps, Features: p.Features, Labels: p.Labels, ExpiresAt: p.ExpiresAt,
		UpdatesUntil: p.UpdatesUntil, Suspended: p.Suspended, ControlPlaneURL: p.ControlPlaneURL}
	if p.License != nil {
		d.License = p.License.Type
	}
	return d
}

// Latest returns the newest payload issued to an installation, or false when none was.
func (i *Issuer) Latest(ctx context.Context, installationID string) (Payload, bool, error) {
	last, err := i.store.LatestIssued(ctx, installationID)
	if errors.Is(err, store.ErrNotFound) {
		return Payload{}, false, nil
	}
	if err != nil {
		return Payload{}, false, err
	}
	var p Payload
	if err := json.Unmarshal(last.Payload, &p); err != nil {
		return Payload{}, false, err
	}
	return p, true, nil
}

// ForDemo issues a demo's first file under the given template: the given apps and features, or every one of the catalog
// when both are empty.
func (i *Issuer) ForDemo(ctx context.Context, installationID, template string, until time.Time, apps, features []string) error {
	if len(apps) == 0 && len(features) == 0 {
		apps, features = Names()
	}
	_, err := i.Issue(ctx, installationID, Draft{Template: template, Apps: apps, Features: features, ExpiresAt: &until}, "", nil)
	return err
}
