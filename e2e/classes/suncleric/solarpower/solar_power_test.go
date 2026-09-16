//go:build e2e

package solarpower_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassSunCleric uint8 = 27

	AuraSolarPower     uint32 = 500149 // 0..20 stacks, removed at 0
	SpellSunflare      uint32 = 800231 // Rank 1, learned at level 1
	SpellVowOfRadiance uint32 = 803489 // learned at level 2: direct damage or healing generates 2 Solar Power
	castAttempts              = 4
)

// Main project issue #323: a level 1 Sun Cleric sees no Solar Power icon.
//
// Solar Power is aura 500149 and only Vows generate it; the first Vow (Vow of Radiance) is
// granted at level 2. These tests check what the server sends: no Solar Power at level 1,
// Solar Power stacks once Vow of Radiance is active. The icon itself is client UI.
//
//	go test -tags=e2e ./e2e/classes/suncleric/solarpower -count=1 -v
func TestSunCleric_NoSolarPowerSourceAtLevel1(t *testing.T) {
	bot, dummy := setup(t, "SolarA", 1)
	if bot.World.KnowsSpell(SpellVowOfRadiance) {
		t.Errorf("E2E_FAIL: a level 1 Sun Cleric already knows Vow of Radiance")
	}
	castSunflare(t, bot, dummy, func() bool { return false })
	if stacks := bot.AuraStacks(AuraSolarPower); stacks != 0 {
		t.Errorf("E2E_FAIL: level 1 Sunflare without a Vow gave %d Solar Power", stacks)
	} else {
		t.Logf("E2E_PASS: level 1 Sun Cleric has no Vow and no Solar Power after Sunflare (#323)")
	}
}

func TestSunCleric_VowOfRadianceGeneratesSolarPower(t *testing.T) {
	bot, dummy := setup(t, "SolarB", 2)
	if !bot.World.KnowsSpell(SpellVowOfRadiance) {
		t.Fatalf("a level 2 Sun Cleric does not know Vow of Radiance")
	}
	_ = bot.World.SetTarget(bot.World.CharGUID())
	res, err := bot.TryCast(t, SpellVowOfRadiance, bot.World.CharGUID(), 5*time.Second)
	if err != nil || !res.Success {
		t.Fatalf("Vow of Radiance failed: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, SpellVowOfRadiance, 3*time.Second)
	_ = bot.World.SetTarget(dummy)

	castSunflare(t, bot, dummy, func() bool { return bot.AuraStacks(AuraSolarPower) > 0 })
	if stacks := bot.AuraStacks(AuraSolarPower); stacks == 0 {
		t.Errorf("E2E_FAIL: Sunflare under Vow of Radiance gave no Solar Power (#323)")
	} else {
		t.Logf("E2E_PASS: Sunflare under Vow of Radiance gave %d Solar Power", stacks)
	}
}

func setup(t *testing.T, prefix string, level int) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: prefix,
		Race:   e2eharness.RaceHuman,
		Class:  ClassSunCleric,
		Level:  level,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.CheatPower(t)
	dummy := bot.Spawn(t, e2eharness.CreatureHeroicTrainingDummy, 10*time.Second)
	_ = bot.World.SetTarget(dummy)
	bot.GM(t, fmt.Sprintf(".npc set level %d", level)) // a level 83 dummy resists a low-level caster
	bot.Face(t, dummy)
	return bot, dummy
}

// castSunflare casts Sunflare up to castAttempts times on the dummy, calling after() after each one
// until it returns true.
func castSunflare(t *testing.T, bot *e2eharness.ScenarioBot, dummy uint64, after func() bool) {
	t.Helper()
	for attempt := 1; attempt <= castAttempts; attempt++ {
		time.Sleep(1600 * time.Millisecond) // global cooldown of the previous cast
		res, err := bot.TryCast(t, SpellSunflare, dummy, 6*time.Second)
		if err != nil || !res.Success {
			t.Fatalf("Sunflare failed: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(700 * time.Millisecond)
		if after() {
			return
		}
	}
}
