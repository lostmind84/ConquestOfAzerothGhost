//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellFlamesOfXoroth uint32 = 801059 // Rank 1, CasterAuraSpell Demonfire, frontal cone
	spellSkulltakerR2   uint32 = 802416 // BaseLevel 8, 70% weapon damage + 10
	auraDemonfire       uint32 = 500906 // stacks to 6
	xorothLevel                = 8      // level in #227 and #1383
)

// Main project issue #227: Flames of Xoroth deals 0 damage (level 8, reported before the #229 cone fix).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run FlamesOfXorothDealsDamage -count=1 -v
func TestKnightOfXoroth_FlamesOfXorothDealsDamage(t *testing.T) {
	bot, dummy := xorothSetup(t, "KoxFl")
	log := watchDamage(t, bot, spellFlamesOfXoroth)

	for attempt := 1; attempt <= 5; attempt++ {
		grantDemonfire(t, bot, 1)
		log.reset()
		if res := castLanded(t, bot, spellFlamesOfXoroth, dummy, 3); !res.Success {
			t.Fatalf("Flames of Xoroth refused with Demonfire: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(settle)
		for _, ev := range log.snapshot() {
			if ev.target == dummy && ev.damage > 0 {
				t.Logf("E2E_PASS: Flames of Xoroth hit the dummy for %d (#227)", ev.damage)
				return
			}
		}
		t.Logf("attempt %d: no damage log on the dummy (%d logs)", attempt, len(log.snapshot()))
		time.Sleep(gcd)
	}
	t.Errorf("E2E_FAIL: Flames of Xoroth never damaged a dummy in front of the caster (#227)")
}

// Main project issue #1383: with 6 Demonfire, Flames of Xoroth (AoE) out-damages Skulltaker (single
// target) at level 8. Whether that is wrong is a balance question; this test only records the numbers.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run SkulltakerVsFlames -count=1 -v
func TestKnightOfXoroth_SkulltakerVsFlamesAtSixDemonfire(t *testing.T) {
	bot, dummy := xorothSetup(t, "KoxSk")
	log := watchDamage(t, bot, spellFlamesOfXoroth, spellSkulltakerR2)

	const samples = 5
	avg := map[uint32]float64{}
	for _, spell := range []uint32{spellSkulltakerR2, spellFlamesOfXoroth} {
		var total, hits uint32
		for i := 0; i < samples*3 && hits < samples; i++ {
			grantDemonfire(t, bot, 6)
			log.reset()
			if res := castLanded(t, bot, spell, dummy, 3); !res.Success {
				t.Fatalf("spell %d refused: %s", spell, e2eharness.SpellFailReasonName(res.FailReason))
			}
			time.Sleep(settle)
			for _, ev := range log.snapshot() {
				if ev.target == dummy && ev.spellID == spell && ev.damage > 0 {
					total += ev.damage
					hits++
					break
				}
			}
			time.Sleep(gcd)
		}
		if hits == 0 {
			t.Fatalf("spell %d never damaged the dummy", spell)
		}
		avg[spell] = float64(total) / float64(hits)
		t.Logf("spell %d at 6 Demonfire: average %.1f over %d hits", spell, avg[spell], hits)
	}
	t.Logf("E2E_MEASURE: Skulltaker %.1f, Flames of Xoroth %.1f per target at level %d, 6 Demonfire (#1383)",
		avg[spellSkulltakerR2], avg[spellFlamesOfXoroth], xorothLevel)
}

func xorothSetup(t *testing.T, prefix string) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceOrc, classKnightXoroth, xorothLevel)
	// New Knights of Xoroth start with Charred Blade 629983 (two-hand sword) equipped.
	bot.Learn(t, spellFlamesOfXoroth)
	bot.Learn(t, spellSkulltakerR2)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, xorothLevel-2)
	bot.CombatReadyFull(t)
	bot.Face(t, dummy)
	return bot, dummy
}

// grantDemonfire sets the Demonfire stack count with GM auras (.aura may add more than one stack).
func grantDemonfire(t *testing.T, bot *e2eharness.ScenarioBot, stacks int) {
	t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		if bot.HasAura(auraDemonfire) {
			bot.CancelAura(t, auraDemonfire)
			bot.TryWaitAuraGone(t, auraDemonfire, 2*time.Second)
		}
		for i := 0; i < stacks*2 && bot.AuraStacks(auraDemonfire) < stacks; i++ {
			bot.ApplyAura(t, auraDemonfire)
			time.Sleep(150 * time.Millisecond)
		}
		if bot.AuraStacks(auraDemonfire) == stacks {
			return
		}
	}
	t.Fatalf("precondition: could not set Demonfire to %d stacks (have %d)", stacks, bot.AuraStacks(auraDemonfire))
}
