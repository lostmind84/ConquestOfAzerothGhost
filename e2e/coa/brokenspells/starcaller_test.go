//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specMoonPriest         uint32 = 43 // CoA talent entry 4045, spell 92134
	spellMoonPriest        uint32 = 92134
	spellHuntressShotR1    uint32 = 680220 // CastingTimeIndex 5
	spellMoonArrowR1       uint32 = 801972 // CastingTimeIndex 5
	spellStarfireShotR1    uint32 = 801978 // CastingTimeIndex 19, needs Scattered Stars on the target
	auraScatteredStars     uint32 = 804378
	spellHuntressSaber     uint32 = 524643
	auraHuntressSaberMount uint32 = 704772
	itemWornShortbow       uint32 = 2504
)

// Main project issue #345: with the Moon Priest specialization, Huntress Shot applies only 1 Scattered
// Stars stack. Huntress Shot's tooltip: 1 stack, plus 1 with Lunar Eclipse (Moon Priest).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run MoonPriest -count=1 -v
func TestStarcaller_MoonPriestHuntressShotStacks(t *testing.T) {
	bot, dummy := starcallerSetup(t, "ScMoon", 10)
	bot.SetSpecialization(t, specMoonPriest, spellMoonPriest)
	bot.Learn(t, spellHuntressShotR1)

	for attempt := 1; attempt <= 5; attempt++ {
		castLanded(t, bot, spellHuntressShotR1, dummy, 3)
		time.Sleep(2 * time.Second) // projectile travel
		if stacks := bot.UnitAuraStacks(dummy, auraScatteredStars); stacks > 0 {
			if stacks < 2 {
				t.Errorf("E2E_FAIL: one Moon Priest Huntress Shot applied %d Scattered Stars, want 2 (#345)", stacks)
			} else {
				t.Logf("E2E_PASS: one Moon Priest Huntress Shot applied %d Scattered Stars", stacks)
			}
			return
		}
		t.Logf("attempt %d: no Scattered Stars on the dummy (miss?)", attempt)
		time.Sleep(gcd)
	}
	t.Fatalf("no Huntress Shot applied Scattered Stars in 5 casts")
}

// Main project issue #1324: Huntress Shot, Moon Arrow and Starfire Shot cast in 1.85 s; the reporter
// remembers 1.25 s. No server-side source gives the expected value, so this only records the cast bars.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run StarcallerCastTimes -count=1 -v
func TestStarcaller_StarcallerCastTimes(t *testing.T) {
	bot, dummy := starcallerSetup(t, "ScCast", 26)
	shots := []uint32{
		knownRank(t, bot, huntressShotRanks...),
		knownRank(t, bot, moonArrowRanks...),
		knownRank(t, bot, starfireShotRanks...),
	}
	for _, id := range shots {
		if !bot.World.KnowsSpell(id) {
			t.Logf("spell %d not known at level 26, skipped", id)
			continue
		}
		res, measured := bot.CastAndMeasure(t, id, dummy, castTimeout)
		t.Logf("E2E_MEASURE: spell %d server cast bar %d ms, measured %s, success=%v reason=%s (#1324)",
			id, res.CastTimeMs, measured, res.Success, e2eharness.SpellFailReasonName(res.FailReason))
		time.Sleep(gcd)
	}
}

// Main project issue #210: attacking dismounts the Huntress Saber, which should allow attacking and
// casting while mounted (tooltip of 524643).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run HuntressSaber -count=1 -v
func TestStarcaller_HuntressSaberKeptWhileAttacking(t *testing.T) {
	bot, dummy := starcallerSetup(t, "ScSabr", 40)
	bot.Learn(t, spellHuntressSaber)
	moonArrow := knownRank(t, bot, moonArrowRanks...)

	castLanded(t, bot, spellHuntressSaber, 0, 2)
	bot.WaitAura(t, auraHuntressSaberMount, 3*time.Second)
	time.Sleep(gcd)

	bot.Face(t, dummy)
	res, err := bot.TryCast(t, moonArrow, dummy, castTimeout)
	if err != nil {
		t.Fatalf("Moon Arrow: %v", err)
	}
	t.Logf("Moon Arrow while mounted: success=%v reason=%s", res.Success, e2eharness.SpellFailReasonName(res.FailReason))
	bot.Attack(t, dummy)
	time.Sleep(4 * time.Second)
	_ = bot.World.AttackStop()

	if !bot.HasAura(auraHuntressSaberMount) {
		t.Errorf("E2E_FAIL: Huntress Saber mount %d removed by casting or attacking (#210)", auraHuntressSaberMount)
		return
	}
	t.Logf("E2E_PASS: still on the Huntress Saber after casting and attacking")
}

// Rank chains from Spell.dbc names (Rank 1 first).
var (
	huntressShotRanks = []uint32{spellHuntressShotR1, 680586, 680587, 680588, 680589}
	moonArrowRanks    = []uint32{spellMoonArrowR1, 803918, 803919, 803920, 803921, 803922, 803923, 803924, 803925}
	starfireShotRanks = []uint32{spellStarfireShotR1, 804463, 804464, 804465, 804466, 804467, 804468, 804469}
)

func starcallerSetup(t *testing.T, prefix string, level int) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceNightElf, classStarcaller, level)
	equip(t, bot, itemWornShortbow)
	x, y, z, mapID := bot.Pos()
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, level-2)
	bot.Teleport(t, x+15, y, z, mapID) // ranged shots need distance from the dummy
	bot.CombatReadyFull(t)
	bot.Face(t, dummy)
	return bot, dummy
}

// Main project issue #345, Lunar Eclipse variant: Moon Priest's extra Scattered Stars belong to Huntress
// Shot's Lunar effect, so they are only expected after activating Lunar Eclipse (4 Lunar Phase stacks).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run MoonPriestLunarEclipse -count=1 -v
func TestStarcaller_MoonPriestLunarEclipseHuntressShotStacks(t *testing.T) {
	const (
		spellLunarEclipse uint32 = 800386
		auraLunarPhase    uint32 = 802985
		auraEclipseReady  uint32 = 704519
	)
	bot, dummy := starcallerSetup(t, "ScLuna", 10)
	bot.SetSpecialization(t, specMoonPriest, spellMoonPriest)
	bot.Learn(t, spellHuntressShotR1)
	bot.Learn(t, spellLunarEclipse)

	for attempt := 1; attempt <= 5; attempt++ {
		for i := 0; i < 8 && bot.AuraStacks(auraLunarPhase) < 4; i++ {
			bot.ApplyAura(t, auraLunarPhase)
		}
		if !bot.HasAura(auraEclipseReady) {
			bot.ApplyAura(t, auraEclipseReady)
		}
		if res := castLanded(t, bot, spellLunarEclipse, 0, 2); !res.Success {
			t.Fatalf("Lunar Eclipse refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		bot.WaitAura(t, spellLunarEclipse, 2*time.Second)
		before := bot.UnitAuraStacks(dummy, auraScatteredStars)
		castLanded(t, bot, spellHuntressShotR1, dummy, 3)
		time.Sleep(2 * time.Second)
		after := bot.UnitAuraStacks(dummy, auraScatteredStars)
		if after > before {
			t.Logf("E2E_MEASURE: Huntress Shot in Lunar Eclipse with Moon Priest: Scattered Stars %d -> %d (#345)", before, after)
			return
		}
		t.Logf("attempt %d: no new Scattered Stars (miss?)", attempt)
		time.Sleep(gcd)
	}
	t.Fatalf("no Huntress Shot in Lunar Eclipse applied Scattered Stars in 5 attempts")
}
