//go:build e2e

package xprates_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	CreatureHarvestGolem  uint32 = 36 // spawns at level 11 or 12, Westfall
	golemLevel                   = 12 // pinned: kill XP depends on the creature level
	QuestBountyOnMurlocs  uint32 = 46 // Alliance, min level 7
	botLevel                     = 15
	experienceWaitTimeout        = 5 * time.Second
)

type xpGains struct {
	kill  uint32
	quest uint32
}

// Rate.XP.Kill and Rate.XP.Quest must scale kill and quest experience.
// Main project issue #193 reports that changing them has no effect.
//
// The same creature kill and quest reward are measured on a fresh level 15
// character with both rates at 1, then at 10; each gain must be exactly ten
// times larger. Absolute values depend on the world database and CoA level
// scaling, so only the ratios are checked.
//
// Rates are changed in the live worldserver.conf and applied with
// `.reload config`, so E2E_WORLDSERVER_CONF is required (see SetWorldConfig).
//
//	go test -tags=e2e ./e2e/coa/xprates -run TestCoA_XPRates -count=1 -v
func TestCoA_XPRates(t *testing.T) {
	if e2eharness.WorldserverConfPath == "" {
		t.Skip("E2E_WORLDSERVER_CONF not set: this test changes worldserver settings")
	}

	base := measureXP(t, "1")
	scaled := measureXP(t, "10")

	for _, c := range []struct {
		name         string
		base, scaled uint32
	}{
		{"Rate.XP.Kill (creature kill)", base.kill, scaled.kill},
		{"Rate.XP.Quest (quest reward)", base.quest, scaled.quest},
	} {
		// Kill experience is rated before SPELL_AURA_MOD_XP_PCT (the default PvE ruleset's War Mode marker gives
		// +15%) and truncated to an integer after it, so the rate 10 value can exceed ten times the rate 1 value
		// by up to 9.
		if c.scaled < 10*c.base || c.scaled >= 10*c.base+10 {
			t.Errorf("E2E_FAIL: %s: %d XP at rate 1, %d XP at rate 10, want %d to %d",
				c.name, c.base, c.scaled, 10*c.base, 10*c.base+9)
			continue
		}
		t.Logf("E2E_PASS: %s: %d XP at rate 1, %d XP at rate 10", c.name, c.base, c.scaled)
	}
}

func measureXP(t *testing.T, rate string) xpGains {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "XP" + rate,
		Race:   e2eharness.RaceHuman,
		Level:  botLevel,
	})
	bot.SetWorldConfig(t, map[string]string{"Rate.XP.Kill": rate, "Rate.XP.Quest": rate})
	bot.TeleportPad(t, e2eharness.PackagePad(t))

	golem := bot.Spawn(t, CreatureHarvestGolem, 15*time.Second)
	if err := bot.World.SetTarget(golem); err != nil {
		e2eharness.HarnessFailf(t, "select creature: %v", err)
	}
	bot.GM(t, fmt.Sprintf(".npc set level %d", golemLevel))
	waitUnitLevel(t, bot, golem, golemLevel)
	bot.GM(t, ".gm off") // kill as a regular player, no GM mode
	before := bot.PlayerXP()
	bot.DamageKill(t, []uint64{golem}, 10_000_000, 10*time.Second)
	kill := bot.WaitXPGain(t, before, experienceWaitTimeout)
	t.Logf("rate %s: creature %d kill granted %d XP", rate, CreatureHarvestGolem, kill)

	bot.GM(t, ".gm on")
	if err := bot.World.SetTarget(bot.World.CharGUID()); err != nil {
		e2eharness.HarnessFailf(t, "select self: %v", err)
	}
	before = bot.PlayerXP()
	for _, cmd := range []string{".quest add %d", ".quest complete %d", ".quest reward %d"} {
		bot.GM(t, fmt.Sprintf(cmd, QuestBountyOnMurlocs))
	}
	quest := bot.WaitXPGain(t, before, experienceWaitTimeout)
	t.Logf("rate %s: quest %d reward granted %d XP", rate, QuestBountyOnMurlocs, quest)

	return xpGains{kill: kill, quest: quest}
}

func waitUnitLevel(t *testing.T, bot *e2eharness.ScenarioBot, guid uint64, level uint32) {
	t.Helper()
	var got uint32
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if obj := bot.World.GetObject(guid); obj != nil {
			if got = obj.Value(client.UnitFieldLevel); got == level {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	e2eharness.Preconditionf(t, "creature 0x%X level %d, want %d", guid, got, level)
}
