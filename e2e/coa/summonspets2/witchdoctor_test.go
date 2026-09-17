//go:build e2e

package summonspets2_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWitchDoctor uint8 = 13

	spellSentryWard      uint32 = 674303 // summons creature 51104 for 60 s
	spellSentryReveal    uint32 = 505173 // "Revealed by a Sentry Totem!"
	spellStealth         uint32 = 1784
	creatureSentryWard   uint32 = 51104
	witchDoctorSentryLvl        = 30

	spellCallOfSseratus   uint32 = 681222 // "Summon 4 Serpent Wards near you"
	spellTrueSpiritTalent uint32 = 802268
	spellTrueSpiritReady  uint32 = 681233 // The True Spirit's Spirit Glaive / Spirit Eclipse buff
	creatureMassSerpent   uint32 = 50587
	callOfSseratusWards          = 4
	stealthedTargetOffset        = 20
)

// Main project issue #1507: a Sentry Ward does not reveal stealthed or invisible enemies.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run SentryWardReveals -count=1 -v
func TestWitchDoctor_SentryWardReveals(t *testing.T) {
	bot := newBot(t, "WdSent", e2eharness.RaceTroll, classWitchDoctor, witchDoctorSentryLvl)
	if !bot.World.KnowsSpell(spellSentryWard) {
		bot.Learn(t, spellSentryWard)
		waitKnows(t, bot, spellSentryWard)
	}
	bot.CombatReadyFull(t)
	x, y, z, m := bot.Pos()
	bot.Teleport(t, x+stealthedTargetOffset, y, z, m)
	boar := spawnTarget(t, bot, creatureMottledBoar, witchDoctorSentryLvl, factionHostile)
	bot.GM(t, ".npc set react 0")
	bot.GM(t, ".aura 1784")
	bot.Teleport(t, x, y, z, m)
	bot.GM(t, ".gm off")
	t.Cleanup(func() { bot.GM(t, ".gm on") })
	hidden := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !hidden; time.Sleep(200 * time.Millisecond) {
		hidden = bot.World.GetObject(boar) == nil
	}
	if !hidden {
		t.Fatalf("precondition: the stealthed boar %d yd away stayed visible", stealthedTargetOffset)
	}
	if res := castLanded(t, bot, spellSentryWard, 0, 3); !res.Success {
		t.Fatalf("precondition: Sentry Ward refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	if len(ownedUnits(bot, creatureSentryWard)) == 0 {
		time.Sleep(settle)
		if len(ownedUnits(bot, creatureSentryWard)) == 0 {
			t.Fatalf("precondition: no Sentry Ward appeared")
		}
	}
	for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if bot.World.GetObject(boar) != nil {
			t.Logf("E2E_PASS: the Sentry Ward revealed the stealthed boar (reveal aura %v) (#1507)",
				bot.UnitHasAura(boar, spellSentryReveal))
			return
		}
	}
	t.Errorf("E2E_FAIL: the stealthed boar stayed hidden 6 s next to a Sentry Ward (#1507)")
}

// Main project issue #1481: Call of Sseratus summons far more than 4 Serpent Wards, and they grant The True
// Spirit's buff without the talent.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run CallOfSseratus -count=1 -v
func TestWitchDoctor_CallOfSseratus(t *testing.T) {
	bot := newBot(t, "WdSser", e2eharness.RaceTroll, classWitchDoctor, witchDoctorSentryLvl)
	if !bot.World.KnowsSpell(spellCallOfSseratus) {
		bot.Learn(t, spellCallOfSseratus)
		waitKnows(t, bot, spellCallOfSseratus)
	}
	if bot.World.KnowsSpell(spellTrueSpiritTalent) || bot.HasAura(spellTrueSpiritTalent) {
		t.Fatalf("precondition: the bot already has The True Spirit")
	}
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellCallOfSseratus, 0, 3); !res.Success {
		t.Fatalf("precondition: Call of Sseratus refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(2 * time.Second)
	wards := len(ownedUnits(bot, creatureMassSerpent))
	t.Logf("Call of Sseratus: %d Serpent Wards, True Spirit buff %v", wards, bot.HasAura(spellTrueSpiritReady))
	if wards != callOfSseratusWards {
		t.Errorf("E2E_FAIL: Call of Sseratus summoned %d Serpent Wards, the tooltip says %d (#1481)", wards, callOfSseratusWards)
	} else {
		t.Logf("E2E_PASS: Call of Sseratus summoned %d Serpent Wards (#1481)", wards)
	}
	if bot.HasAura(spellTrueSpiritReady) {
		t.Errorf("E2E_FAIL: the wards granted The True Spirit's buff %d without the talent (#1481)", spellTrueSpiritReady)
	} else {
		t.Logf("E2E_PASS: no True Spirit buff without the talent (#1481)")
	}
}
