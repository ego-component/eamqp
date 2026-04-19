package exampleconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ego-component/eamqp"
	"github.com/gotomicro/ego/core/econf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveConfigPath(t *testing.T) {
	t.Run("prefers command line config", func(t *testing.T) {
		path, err := ResolveConfigPath([]string{"example", "--config", "/tmp/local.toml"}, nil, t.TempDir())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/local.toml", path)
	})

	t.Run("supports equals form", func(t *testing.T) {
		path, err := ResolveConfigPath([]string{"example", "--config=/tmp/local.toml"}, nil, t.TempDir())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/local.toml", path)
	})

	t.Run("falls back to environment", func(t *testing.T) {
		getenv := func(key string) string {
			if key == "EAMQP_EXAMPLE_CONFIG" {
				return "/tmp/env.toml"
			}
			return ""
		}

		path, err := ResolveConfigPath([]string{"example"}, getenv, t.TempDir())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/env.toml", path)
	})

	t.Run("finds repository default from root", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "examples", "config", "local.toml")
		require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o755))
		require.NoError(t, os.WriteFile(cfg, []byte("[amqp.default]\naddr = \"amqp://localhost\"\n"), 0o644))

		path, err := ResolveConfigPath([]string{"example"}, nil, dir)

		require.NoError(t, err)
		assert.Equal(t, cfg, path)
	})

	t.Run("finds default from example directory", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "examples", "config", "local.toml")
		wd := filepath.Join(dir, "examples", "producer")
		require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o755))
		require.NoError(t, os.MkdirAll(wd, 0o755))
		require.NoError(t, os.WriteFile(cfg, []byte("[amqp.default]\naddr = \"amqp://localhost\"\n"), 0o644))

		path, err := ResolveConfigPath([]string{"example"}, nil, wd)

		require.NoError(t, err)
		assert.Equal(t, cfg, path)
	})
}

func TestArgsWithoutConfig(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "keeps role without config",
			args: []string{"example", "publish"},
			want: []string{"publish"},
		},
		{
			name: "removes config flag pair before role",
			args: []string{"example", "--config", "examples/config/local.toml", "publish"},
			want: []string{"publish"},
		},
		{
			name: "removes config equals form before role",
			args: []string{"example", "--config=examples/config/local.toml", "publish"},
			want: []string{"publish"},
		},
		{
			name: "removes config after role",
			args: []string{"example", "publish", "-config=examples/config/local.toml"},
			want: []string{"publish"},
		},
		{
			name: "keeps multiple business args",
			args: []string{"example", "--config", "examples/config/local.toml", "subscriber", "sub1"},
			want: []string{"subscriber", "sub1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ArgsWithoutConfig(tt.args))
		})
	}
}

func TestLoadConfigFile(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
	}{
		{
			name: "toml",
			file: "local.toml",
			body: "[amqp.default]\naddr = \"amqp://guest:guest@127.0.0.1:5672/\"\nheartbeat = \"15s\"\nonFail = \"error\"\n",
		},
		{
			name: "yaml",
			file: "local.yaml",
			body: "amqp:\n  default:\n    addr: amqp://guest:guest@127.0.0.1:5672/\n    heartbeat: 15s\n    onFail: error\n",
		},
		{
			name: "json",
			file: "local.json",
			body: `{"amqp":{"default":{"addr":"amqp://guest:guest@127.0.0.1:5672/","heartbeat":"15s","onFail":"error"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			econf.Reset()
			defer econf.Reset()

			path := filepath.Join(t.TempDir(), tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))

			require.NoError(t, LoadConfigFile(path))

			cfg := eamqp.DefaultConfig()
			require.NoError(t, econf.UnmarshalKey(DefaultComponentKey, &cfg))
			assert.Equal(t, "amqp://guest:guest@127.0.0.1:5672/", cfg.Addr)
			assert.Equal(t, 15*time.Second, cfg.Heartbeat)
			assert.Equal(t, "error", cfg.OnFail)
		})
	}
}
