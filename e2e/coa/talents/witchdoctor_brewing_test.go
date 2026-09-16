//go:build e2e

package talents_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specWitchDoctorBrewing uint32 = 6

	spellJungleSecrets     uint32 = 707212
	talentJungleSecrets    uint32 = 31137
	spellJungleSecretsHeal uint32 = 712348
	spellShadowEffigy      uint32 = 505339
	spellFrogBones         uint32 = 801663
	talentFrogBones        uint32 = 29741
	auraPotionFrogBones    uint32 = 802971
)

var (
	loasBrewRanks   = []uint32{801670, 501198, 501199, 501200, 501201, 501202, 501203, 501204, 501205}
	potionTossRanks = []uint32{801661, 573430, 573431, 573432, 573433, 573434, 573435}
)

// highestKnown returns the highest rank the bot knows, learning the first rank when it knows none.
func highestKnown(t *testing.T, bot *e2eharness.ScenarioBot, ranks []uint32) uint32 {
	t.Helper()
	for i := len(ranks) - 1; i >= 0; i-- {
		if bot.World.KnowsSpell(ranks[i]) {
			return ranks[i]
		}
	}
	bot.Learn(t, ranks[0])
	return ranks[0]
}

// Main project issue #488: with Jungle Secrets, healing with Loa's Brew does not make the Effigy heal another ally.
//
//	go test -tags=e2e ./e2e/coa/talents -run JungleSecrets -count=1 -v
func TestWitchDoctor_JungleSecretsEffigyHeal(t *testing.T) {
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: "WdJung",
		Bots: []e2eharness.BotSpec{
			{Role: "doctor", Race: e2eharness.RaceTroll, Class: classWitchDoctor, Level: 60},
			{Role: "ally", Race: e2eharness.RaceOrc, Class: e2eharness.ClassWarrior, Level: 60},
		},
	})
	doctor, ally := bots[0], bots[1]
	e2eharness.FormPartyAtPad(t, e2eharness.PackagePad(t), doctor, ally)
	t.Cleanup(func() { doctor.CleanupOwnedSummons(t) })
	takeTalent(t, doctor, specWitchDoctorBrewing, talentJungleSecrets, spellJungleSecrets)
	if !doctor.World.KnowsSpell(spellShadowEffigy) {
		doctor.Learn(t, spellShadowEffigy)
	}
	brew := highestKnown(t, doctor, loasBrewRanks)
	doctor.CombatReadyFull(t)
	ally.GM(t, ".gm off")

	if res := castLanded(t, doctor, spellShadowEffigy, 0, 3); !res.Success {
		t.Fatalf("Shadow Effigy refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	heals := watchSpellLog(t, doctor, smsgSpellHealLog, spellJungleSecretsHeal)
	allyGUID := ally.World.CharGUID()
	for attempt := 1; attempt <= 3; attempt++ {
		// Heal the wounded ally: the Effigy copy goes to another party member near it, here the doctor.
		_ = doctor.World.SetTarget(allyGUID)
		doctor.GM(t, ".damage 2500")
		time.Sleep(500 * time.Millisecond)
		if res := castLanded(t, doctor, brew, allyGUID, 3); !res.Success {
			t.Fatalf("Loa's Brew %d refused: %s", brew, e2eharness.SpellFailReasonName(res.FailReason))
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, ev := range heals.snapshot() {
				if ev.target != allyGUID {
					t.Logf("E2E_PASS: Loa's Brew on the ally made the Effigy heal %#x for %d (%d)", ev.target, ev.amount,
						ev.spellID)
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		hp, maxHP := doctor.UnitHP(allyGUID)
		t.Logf("attempt %d: no %d heal on another party member (ally %d/%d, logs %+v)", attempt,
			spellJungleSecretsHeal, hp, maxHP, heals.snapshot())
	}
	t.Errorf("E2E_FAIL: three Loa's Brews with an Effigy up, no Jungle Secrets heal on another party member (#488)")
}

// Main project issue #519: Frog Bones potions do not grant their absorption shield.
//
//	go test -tags=e2e ./e2e/coa/talents -run FrogBones -count=1 -v
func TestWitchDoctor_FrogBonesPotionShield(t *testing.T) {
	bot := newBot(t, "WdBone", e2eharness.RaceTroll, classWitchDoctor, 60)
	takeTalent(t, bot, specWitchDoctorBrewing, talentFrogBones, spellFrogBones)
	potion := highestKnown(t, bot, potionTossRanks)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellFrogBones, 0, 3); !res.Success {
		t.Fatalf("Frog Bones refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	if res := castLanded(t, bot, potion, bot.World.CharGUID(), 3); !res.Success {
		t.Fatalf("Potion Toss %d refused after brewing Frog Bones: %s", potion,
			e2eharness.SpellFailReasonName(res.FailReason))
	}
	if !waitAura(bot, auraPotionFrogBones, 3*time.Second) {
		t.Errorf("E2E_FAIL: Potion Toss with Frog Bones brewed, no %d absorb on the target (#519)", auraPotionFrogBones)
		return
	}
	t.Logf("E2E_PASS: Potion Toss applied the Frog Bones absorb %d", auraPotionFrogBones)
}
