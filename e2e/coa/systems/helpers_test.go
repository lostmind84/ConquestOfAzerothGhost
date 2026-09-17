//go:build e2e

// Package systems_test reproduces the "systems and modes" reports of the CoA server issue tracker
// (rest, professions, instances, duels, rulesets, GM access). Each test names its issue in its failure messages.
//
//	go test -tags=e2e ./e2e/coa/systems -count=1 -v -p 1
package systems_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classRanger      uint8 = 21
	classNecromancer uint8 = 23

	creatureHarvestGolem uint32 = 36

	factionHostile = 14

	castTimeout = 5 * time.Second
	gcd         = 1600 * time.Millisecond
)

// newBot creates one bot of the given class and level on the package pad.
func newBot(t *testing.T, prefix string, race, class uint8, level int) *e2eharness.ScenarioBot {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: prefix, Race: race, Class: class, Level: level})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	t.Cleanup(func() { bot.CleanupOwnedSummons(t) })
	return bot
}

// spawnEnemy spawns a hostile creature of the given level and targets it.
func spawnEnemy(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32, level int) uint64 {
	t.Helper()
	guid := bot.Spawn(t, entry, 10*time.Second)
	_ = bot.World.SetTarget(guid)
	bot.GM(t, fmt.Sprintf(".npc set level %d", level))
	bot.GM(t, fmt.Sprintf(".npc set faction %d", factionHostile))
	bot.Face(t, guid)
	return guid
}

// waitKnows waits until the bot knows a spell.
func waitKnows(t *testing.T, bot *e2eharness.ScenarioBot, spellID uint32, timeout time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if bot.World.KnowsSpell(spellID) {
			return true
		}
	}
	return false
}
