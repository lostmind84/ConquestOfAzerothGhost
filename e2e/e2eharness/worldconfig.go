package e2eharness

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// WorldserverConfPath is the path, as seen by the test process, of the
// worldserver.conf the target worldserver reads (E2E_WORLDSERVER_CONF).
// Empty disables tests that change server settings.
var WorldserverConfPath = os.Getenv("E2E_WORLDSERVER_CONF")

// SetWorldConfig writes settings into WorldserverConfPath and applies them with
// `.reload config`. The original file is restored and reloaded on cleanup.
//
// It changes the live server for every player, so the test is skipped unless
// E2E_WORLDSERVER_CONF is set. Only settings that World::LoadConfigSettings
// re-reads on reload take effect; startup-only settings need a restart.
// A killed test process (no cleanup) leaves the modified file in place.
func (b *ScenarioBot) SetWorldConfig(t *testing.T, settings map[string]string) {
	t.Helper()
	path := WorldserverConfPath
	if path == "" {
		t.Skip("E2E_WORLDSERVER_CONF not set: this test changes worldserver settings")
	}
	info, err := os.Stat(path)
	if err != nil {
		Preconditionf(t, "worldserver config: %v", err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		Preconditionf(t, "read worldserver config: %v", err)
	}
	updated, err := SetConfigValues(string(original), settings)
	if err != nil {
		Preconditionf(t, "%s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		Preconditionf(t, "write worldserver config: %v", err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(path, original, info.Mode().Perm()); err != nil {
			t.Errorf("restore %s: %v", path, err)
			return
		}
		if err := b.World.SendGMCommand(".reload config"); err != nil {
			t.Logf("WARNING: restored %s but .reload config failed (%v); run it manually", path, err)
			return
		}
		FlushWorld(t, b.World)
		t.Logf("restored %s and reloaded config", path)
	})
	MustGM(t, b.World, ".reload config")
	// The reload runs inside the command handler; the next command's ack proves it ran.
	FlushWorld(t, b.World)
	t.Logf("world config reloaded with %s", formatSettings(settings))
}

// SetConfigValues returns content with each `Key = value` line replaced. Every key
// must appear exactly once as an uncommented setting. Line endings are preserved.
func SetConfigValues(content string, settings map[string]string) (string, error) {
	for _, key := range sortedKeys(settings) {
		re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(key) + `[ \t]*=[^\r\n]*`)
		if n := len(re.FindAllStringIndex(content, -1)); n != 1 {
			return "", fmt.Errorf("setting %q found %d times, want exactly 1", key, n)
		}
		line := key + " = " + settings[key]
		content = re.ReplaceAllLiteralString(content, line)
	}
	return content, nil
}

func formatSettings(settings map[string]string) string {
	parts := make([]string, 0, len(settings))
	for _, key := range sortedKeys(settings) {
		parts = append(parts, key+"="+settings[key])
	}
	return strings.Join(parts, " ")
}

func sortedKeys(settings map[string]string) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
