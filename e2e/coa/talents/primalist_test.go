//go:build e2e

package talents_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specPrimalistGrovekeeper uint32 = 58

	spellHammerOfLife       uint32 = 803973
	talentHammerOfLife      uint32 = 31209
	spellHammerOfLifeHeal   uint32 = 806073
	spellHammerOfLifeDamage uint32 = 807651
	spellGroveTraining      uint32 = 92150
	talentGroveTraining     uint32 = 4060
	auraAftershock          uint32 = 301086
)

// Main project issue #317 (persistence): Hammer of Life is not kept across sessions.
//
//	go test -tags=e2e ./e2e/coa/talents -run HammerOfLifePersists -count=1 -v
func TestPrimalist_HammerOfLifePersists(t *testing.T) {
	bot := newBot(t, "PrHamS", e2eharness.RaceTauren, classPrimalist, 12)
	takeTalent(t, bot, specPrimalistGrovekeeper, talentHammerOfLife, spellHammerOfLife)
	bot.Save(t)
	bot.Relog(t)
	if !waitSpell(bot, spellHammerOfLife, 5*time.Second) {
		t.Errorf("E2E_FAIL: Hammer of Life %d not known after a relog (#317)", spellHammerOfLife)
		return
	}
	t.Logf("E2E_PASS: Hammer of Life kept after a relog")
}

// Main project issue #317 (effect): Hammer of Life's melee attacks do not heal allies or damage nearby enemies.
//
//	go test -tags=e2e ./e2e/coa/talents -run HammerOfLifeEffect -count=1 -v
func TestPrimalist_HammerOfLifeEffect(t *testing.T) {
	bot := newBot(t, "PrHamE", e2eharness.RaceTauren, classPrimalist, 12)
	takeTalent(t, bot, specPrimalistGrovekeeper, talentHammerOfLife, spellHammerOfLife)
	// The heroic training dummy's armor keeps low-level hits at 2-4 damage, whose 20% rounds to 0: use a creature.
	dummy := spawnTarget(t, bot, creatureDefiasThug, 12)
	bot.GM(t, ".npc set faction 7")
	damage := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, spellHammerOfLifeDamage)
	heals := watchSpellLog(t, bot, smsgSpellHealLog, spellHammerOfLifeHeal)
	swings := watchSwings(t, bot)
	bot.CombatReady(t)
	bot.Attack(t, dummy)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(damage.snapshot())+len(heals.snapshot()) > 0 {
			t.Logf("E2E_PASS: Hammer of Life fired (damage %+v, heals %+v)", damage.snapshot(), heals.snapshot())
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	if swings.swings() < 3 {
		t.Fatalf("precondition: only %d melee swing(s) in 20 s", swings.swings())
	}
	t.Errorf("E2E_FAIL: %d melee swings with Hammer of Life, no %d damage or %d heal (#317)", swings.swings(),
		spellHammerOfLifeDamage, spellHammerOfLifeHeal)
}

// Main project issue #319: Grove Training (10% chance on melee damage or healing) never grants Aftershock.
//
//	go test -tags=e2e ./e2e/coa/talents -run GroveTraining -count=1 -v
func TestPrimalist_GroveTrainingProcsAftershock(t *testing.T) {
	bot := newBot(t, "PrGrove", e2eharness.RaceTauren, classPrimalist, 12)
	bot.SetSpecialization(t, specPrimalistGrovekeeper)
	bot.SetTalentRank(t, talentGroveTraining, 1)
	if !waitSpell(bot, spellGroveTraining, 5*time.Second) {
		t.Fatalf("precondition: Grove Training %d not known at level 12 in specialization %d", spellGroveTraining,
			specPrimalistGrovekeeper)
	}
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 12)
	swings := watchSwings(t, bot)
	bot.CombatReady(t)
	bot.Attack(t, dummy)
	// 10% per landed hit; swings also count misses and dodges. Over five runs the first proc came after 4 to 24
	// swings and once not within 40, so allow 100 swings.
	deadline := time.Now().Add(360 * time.Second)
	for time.Now().Before(deadline) && swings.swings() < 100 {
		if bot.HasAura(auraAftershock) {
			t.Logf("E2E_PASS: Grove Training granted Aftershock after %d swing(s)", swings.swings())
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	if swings.swings() < 20 {
		t.Fatalf("precondition: only %d melee swing(s)", swings.swings())
	}
	t.Errorf("E2E_FAIL: %d melee swings with Grove Training, Aftershock %d never granted (#319)", swings.swings(),
		auraAftershock)
}
