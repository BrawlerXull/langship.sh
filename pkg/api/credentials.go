package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lyzrai/flow/pkg/secrets"
	"github.com/lyzrai/flow/pkg/storage"
)

// credentialBody is the create/update wire format. Fields are tagged
// per credential type — only the matching ones are read for a given
// `type`. Secrets (gcpServiceAccountJson, kv values) come in as
// plaintext and get sealed before persisting.
type credentialBody struct {
	Name string                 `json:"name"`
	Type storage.CredentialType `json:"type"`

	// AWS
	AwsRegion              string `json:"awsRegion,omitempty"`
	AwsAccountID           string `json:"awsAccountId,omitempty"`
	AwsCrossAccountRoleArn string `json:"awsCrossAccountRoleArn,omitempty"`

	// GCP
	GcpProjectID          string `json:"gcpProjectId,omitempty"`
	GcpLocation           string `json:"gcpLocation,omitempty"`
	GcpServiceAccountJson string `json:"gcpServiceAccountJson,omitempty"`

	// KV
	Kv map[string]string `json:"kv,omitempty"`
}

// validateAndBuild converts the wire body into a storage.Credential,
// sealing any secret fields. Sets ID + CreatedAt if missing (caller
// pulls those forward on update).
func (b *credentialBody) validateAndBuild() (storage.Credential, error) {
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return storage.Credential{}, errors.New("name is required")
	}
	now := time.Now().UTC()
	c := storage.Credential{
		Name:      name,
		Type:      b.Type,
		CreatedAt: now,
		UpdatedAt: now,
	}
	switch b.Type {
	case storage.CredentialAWS:
		c.AwsRegion = strings.TrimSpace(b.AwsRegion)
		c.AwsAccountID = strings.TrimSpace(b.AwsAccountID)
		c.AwsCrossAccountRoleArn = strings.TrimSpace(b.AwsCrossAccountRoleArn)
		if c.AwsRegion == "" || c.AwsAccountID == "" || c.AwsCrossAccountRoleArn == "" {
			return storage.Credential{}, errors.New("aws credential needs region, accountId, and crossAccountRoleArn")
		}
	case storage.CredentialGCP:
		c.GcpProjectID = strings.TrimSpace(b.GcpProjectID)
		c.GcpLocation = strings.TrimSpace(b.GcpLocation)
		if c.GcpProjectID == "" {
			return storage.Credential{}, errors.New("gcp credential needs projectId")
		}
		if b.GcpServiceAccountJson != "" {
			sealed, err := secrets.SealString(b.GcpServiceAccountJson)
			if err != nil {
				return storage.Credential{}, fmt.Errorf("seal gcp SA: %w", err)
			}
			c.GcpServiceAccountSealed = sealed
		}
	case storage.CredentialKV:
		if len(b.Kv) == 0 {
			return storage.Credential{}, errors.New("kv credential needs at least one key/value")
		}
		sealed, err := secrets.SealMap(b.Kv)
		if err != nil {
			return storage.Credential{}, fmt.Errorf("seal kv: %w", err)
		}
		c.KvSealed = sealed
	default:
		return storage.Credential{}, fmt.Errorf("unknown credential type %q", b.Type)
	}
	return c, nil
}

func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "agent not found")
		return
	}
	out := make([]PublicCredential, 0, len(a.Credentials))
	for _, c := range a.Credentials {
		out = append(out, publicCredential(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	if !secrets.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, errors.New("FLOW_SECRET_KEY is not set; refusing to store credentials"))
		return
	}
	id := r.PathValue("id")
	a, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "agent not found")
		return
	}

	var body credentialBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cred, err := body.validateAndBuild()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Reject duplicate names within the agent.
	for _, c := range a.Credentials {
		if strings.EqualFold(c.Name, cred.Name) {
			writeError(w, http.StatusConflict, fmt.Errorf("credential %q already exists; PUT to update", cred.Name))
			return
		}
	}
	cred.ID = newID()
	a.Credentials = append(a.Credentials, cred)
	a.UpdatedAt = time.Now().UTC()
	if err := s.agents.Update(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicCredential(cred))
}

func (s *Server) handleUpdateCredential(w http.ResponseWriter, r *http.Request) {
	if !secrets.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, errors.New("FLOW_SECRET_KEY is not set"))
		return
	}
	id := r.PathValue("id")
	name := r.PathValue("name")
	a, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "agent not found")
		return
	}

	var body credentialBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		body.Name = name
	}
	updated, err := body.validateAndBuild()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	idx := -1
	for i, c := range a.Credentials {
		if strings.EqualFold(c.Name, name) {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, fmt.Errorf("credential %q not found", name))
		return
	}

	// Preserve immutable fields; carry forward unchanged secrets when the
	// update body left them empty (matches the "edit without re-entering
	// the secret" UX).
	prev := a.Credentials[idx]
	updated.ID = prev.ID
	updated.CreatedAt = prev.CreatedAt
	if updated.Type == storage.CredentialGCP && updated.GcpServiceAccountSealed == "" {
		updated.GcpServiceAccountSealed = prev.GcpServiceAccountSealed
	}
	if updated.Type == storage.CredentialKV && len(updated.KvSealed) == 0 {
		updated.KvSealed = prev.KvSealed
	}

	a.Credentials[idx] = updated
	a.UpdatedAt = time.Now().UTC()
	if err := s.agents.Update(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, publicCredential(updated))
}

func (s *Server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")
	a, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "agent not found")
		return
	}
	idx := -1
	for i, c := range a.Credentials {
		if strings.EqualFold(c.Name, name) {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, fmt.Errorf("credential %q not found", name))
		return
	}
	a.Credentials = append(a.Credentials[:idx], a.Credentials[idx+1:]...)
	a.UpdatedAt = time.Now().UTC()
	if err := s.agents.Update(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
