//go:build e2e

package talents_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Hit chance talents. A spell's miss chance against a same-level creature is 4% (Unit::MagicSpellHitResult), so hit
// chance granting at least that much leaves no miss at all. Each test casts
// Ice Lance (instant, no cooldown; `.cheat cooldown` also skips the global cooldown) without the talent, then with
// it, and counts casts that produced no damage log. The target is a normal-rank dummy: the Heroic Training Dummy is a
// boss, which counts as three levels above the caster (17% base miss chance).

const (
	classFelsworn uint8 = 14

	spellIceLance uint32 = 30455

	creatureExpertsTrainingDummy uint32 = 32666 // rank 0

	// Witch Doctor (#312): Village Wisdom effect 1 is aura 333 (hit chance, all attacks), 5%.
	spellVillageWisdom  uint32 = 680882
	talentVillageWisdom uint32 = 6074

	// Felsworn (#1490): Wrath of Sargeras rank 1 effect 1 is aura 55 (spell hit chance), 2%. Accuracy (aura 333, 2%)
	// brings the baseline to 2% so that the talent's 2% can remove every miss.
	spellAccuracy         uint32 = 13832
	specFelswornFelblood  uint32 = 7
	spellWrathOfSargeras  uint32 = 806103
	talentWrathOfSargeras uint32 = 31297
)

// countMisses casts Ice Lance at the dummy `casts` times and returns how many landed casts dealt no damage.
func countMisses(t *testing.T, bot *e2eharness.ScenarioBot, dummy uint64, casts int) int {
	t.Helper()
	logs := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, spellIceLance)
	landed := 0
	for i := 0; i < casts; i++ {
		res, err := bot.TryCast(t, spellIceLance, dummy, castTimeout)
		if err != nil {
			t.Fatalf("Ice Lance cast %d: %v", i+1, err)
		}
		if res.Success {
			landed++
		}
	}
	time.Sleep(3 * time.Second) // missile travel
	hits := 0
	for _, ev := range logs.snapshot() {
		if ev.target == dummy {
			hits++
		}
	}
	if landed < casts*9/10 {
		t.Fatalf("precondition: only %d of %d Ice Lance casts succeeded", landed, casts)
	}
	return landed - hits
}

// hitChanceScenario prepares a bot with Ice Lance, cheats and a training dummy at dummyLevel.
func hitChanceScenario(t *testing.T, prefix string, race, class uint8, level, dummyLevel int) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := newBot(t, prefix, race, class, level)
	bot.Learn(t, spellIceLance)
	dummy := spawnTarget(t, bot, creatureExpertsTrainingDummy, dummyLevel)
	bot.CombatReadyFull(t)
	bot.GM(t, ".cheat cooldown on")
	return bot, dummy
}

func checkNoMisses(t *testing.T, bot *e2eharness.ScenarioBot, dummy uint64, casts int, takeTalent func(), issue string) {
	t.Helper()
	before := countMisses(t, bot, dummy, casts)
	t.Logf("without the talent: %d misses in %d casts", before, casts)
	if before == 0 {
		t.Fatalf("inconclusive: no miss without the talent either, the baseline miss chance is not what the test expects")
	}
	takeTalent()
	time.Sleep(settle)
	after := countMisses(t, bot, dummy, casts)
	if after > 0 {
		t.Errorf("E2E_FAIL: %d misses in %d casts with the talent, expected none (%s)", after, casts, issue)
		return
	}
	t.Logf("E2E_PASS: no miss in %d casts with the talent (%d without)", casts, before)
}

// Main project issue #312: Village Wisdom does not grant its 5% hit chance.
//
//	go test -tags=e2e ./e2e/coa/talents -run VillageWisdom -count=1 -v
func TestWitchDoctor_VillageWisdomHitChance(t *testing.T) {
	bot, dummy := hitChanceScenario(t, "WdVill", e2eharness.RaceTroll, classWitchDoctor, 60, 60)
	bot.SetSpecialization(t, specWitchDoctorShadowhunting)
	time.Sleep(settle)
	checkNoMisses(t, bot, dummy, 250, func() {
		takeTalent(t, bot, specWitchDoctorShadowhunting, talentVillageWisdom, spellVillageWisdom)
	}, "#312")
}

// Main project issue #1490: Wrath of Sargeras does not grant its 2% spell hit chance.
//
//	go test -tags=e2e ./e2e/coa/talents -run WrathOfSargeras -count=1 -v
func TestFelsworn_WrathOfSargerasSpellHit(t *testing.T) {
	bot, dummy := hitChanceScenario(t, "FsWrath", e2eharness.RaceBloodElf, classFelsworn, 60, 60)
	bot.SetSpecialization(t, specFelswornFelblood)
	_ = bot.World.SetTarget(bot.World.CharGUID())
	bot.GM(t, fmt.Sprintf(".aura %d", spellAccuracy))
	_ = bot.World.SetTarget(dummy)
	time.Sleep(settle)
	checkNoMisses(t, bot, dummy, 300, func() {
		takeTalent(t, bot, specFelswornFelblood, talentWrathOfSargeras, spellWrathOfSargeras)
	}, "#1490")
}
