package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(f, []byte(content), 0o600))
	return f
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTemp(t, `
modules:
  default:
    auth:
      username: admin
      password: secret
    insecure: true
    collectors:
      thermal: true
      power: true
      systems: true
      memory: true
`)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	m := cfg.Modules["default"]
	assert.Equal(t, "admin", m.Auth.Username)
	assert.Equal(t, "secret", m.Auth.Password)
	assert.True(t, m.Insecure)
	assert.True(t, m.Collectors.Thermal)
	assert.True(t, m.Collectors.Power)
	assert.True(t, m.Collectors.Systems)
	assert.True(t, m.Collectors.Memory)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "this: is: not: valid: yaml: ][")
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_MissingDefaultModule(t *testing.T) {
	path := writeTemp(t, `
modules:
  other:
    auth:
      username: admin
      password: secret
`)

	_, err := Load(path)
	require.ErrorContains(t, err, "default module missing")
}

func TestLoad_CollectorFlagsDefaultToFalse(t *testing.T) {
	path := writeTemp(t, `
modules:
  default:
    auth:
      username: admin
      password: secret
`)

	cfg, err := Load(path)
	require.NoError(t, err)

	cc := cfg.Modules["default"].Collectors
	assert.False(t, cc.Thermal)
	assert.False(t, cc.Power)
	assert.False(t, cc.Systems)
	assert.False(t, cc.Memory)
}

func TestLoad_MultipleModules(t *testing.T) {
	path := writeTemp(t, `
modules:
  default:
    collectors:
      thermal: true
  detailed:
    collectors:
      thermal: true
      power: true
      systems: true
      memory: true
`)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Len(t, cfg.Modules, 2)
	assert.True(t, cfg.Modules["detailed"].Collectors.Memory)
}
