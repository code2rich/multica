package service

import (
	"encoding/json"
	"fmt"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// EnvSecretService encrypts and decrypts synchronized AgentWaker environment
// values at rest, using the existing application-layer secretbox (AES-256-GCM)
// infrastructure. The plaintext lives only in daemon memory (during the explicit
// apply request) and in the single secret apply envelope; everything stored in
// the database is sealed by this service. Task preparation decrypts only for
// the owning agent execution.
//
// The key is loaded from MULTICA_AGENT_ENV_SECRET_KEY at boot (see router.go),
// mirroring the lark/slack/wechat integration key pattern. If the key is absent
// the service is nil and AgentWaker env sync is unavailable — but the rest of
// the apply (capabilities, roles, skills, bindings) still works because env is
// the only encrypted surface.
type EnvSecretService struct {
	box *secretbox.Box
}

// NewEnvSecretService returns a service backed by the supplied box. The box
// must be non-nil; callers gate construction on secretbox.LoadKey succeeding.
func NewEnvSecretService(box *secretbox.Box) (*EnvSecretService, error) {
	if box == nil {
		return nil, fmt.Errorf("env_secret: a non-nil secretbox.Box is required (set MULTICA_AGENT_ENV_SECRET_KEY)")
	}
	return &EnvSecretService{box: box}, nil
}

// SealEnv encrypts a map of env values into a single sealed blob suitable for
// the agent.custom_env_encrypted column. The map is JSON-marshaled before
// encryption so decryption recovers the full key→value structure.
func (s *EnvSecretService) SealEnv(values map[string]string) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("env_secret: marshal values: %w", err)
	}
	sealed, err := s.box.Seal(plain)
	if err != nil {
		return nil, fmt.Errorf("env_secret: seal: %w", err)
	}
	return sealed, nil
}

// OpenEnv decrypts a sealed env blob back into the key→value map. Returns an
// empty map when the blob is nil/empty (agent has no synchronized values).
func (s *EnvSecretService) OpenEnv(sealed []byte) (map[string]string, error) {
	if len(sealed) == 0 {
		return map[string]string{}, nil
	}
	plain, err := s.box.Open(sealed)
	if err != nil {
		return nil, fmt.Errorf("env_secret: open: %w", err)
	}
	var out map[string]string
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("env_secret: unmarshal values: %w", err)
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}
