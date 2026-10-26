package auth

import (
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/TillmanBuildsTech/grokbot-node/internal/state"
)

// Credentials is the persisted account OAuth state.
type Credentials struct {
	AccessToken    string `json:"accessToken"`
	RefreshToken   string `json:"refreshToken"`
	SelectedTeamID any    `json:"selectedTeamId,omitempty"`
	SavedAtMs      int64  `json:"savedAtMs,omitempty"`
}

// Valid reports whether credentials have the minimal required shape.
func (c Credentials) Valid() bool {
	return c.AccessToken != "" && c.RefreshToken != ""
}

// Identity is the stable machine identity for this node.
type Identity struct {
	MachineID string `json:"machineId"`
	Label     string `json:"label"`
}

// UUIDv4 returns a random RFC-4122 v4 UUID using crypto/rand.
func UUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// LoadOrCreateMachine returns the persisted identity or creates one.
func LoadOrCreateMachine(path string) (Identity, error) {
	var id Identity
	found, err := state.ReadJSON(path, &id)
	if err != nil {
		return Identity{}, err
	}
	if found && id.MachineID != "" {
		return id, nil
	}
	host, _ := os.Hostname()
	id = Identity{MachineID: UUIDv4(), Label: host}
	if err := state.WriteJSON(path, id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// SaveCredentials persists credentials with a save timestamp.
func SaveCredentials(path string, c Credentials) (Credentials, error) {
	c.SavedAtMs = time.Now().UnixMilli()
	if err := state.WriteJSON(path, c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

// LoadCredentials reads credentials, erroring if absent or malformed.
func LoadCredentials(path string) (Credentials, error) {
	var c Credentials
	found, err := state.ReadJSON(path, &c)
	if err != nil {
		return Credentials{}, err
	}
	if !found || !c.Valid() {
		return Credentials{}, fmt.Errorf("no valid account credentials; run: grokbotd login")
	}
	return c, nil
}
