//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellShadowEffigy    uint32 = 505339
	creatureShadowEffigy uint32 = 50119
	spellSpiritualRecall uint32 = 583092 // Effect 110 (destroy all totems), BasePoints 49
	witchDoctorLevel            = 30
)

// Main project issue #384: Spiritual Recall does not destroy wards, idols or effigies and returns no mana.
// The DBC gives Spiritual Recall no cooldown (Category 0), so the cooldown part of the report is not tested.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run SpiritualRecall -count=1 -v
func TestWitchDoctor_SpiritualRecallDestroysEffigy(t *testing.T) {
	bot := newBot(t, "WdRec", e2eharness.RaceTroll, classWitchDoctor, witchDoctorLevel)
	bot.Learn(t, spellShadowEffigy)
	bot.Learn(t, spellSpiritualRecall)
	bot.CombatReady(t)

	castLanded(t, bot, spellShadowEffigy, 0, 2)
	effigy := bot.WaitUnitAny(t, 5*time.Second, creatureShadowEffigy)
	if effigy == 0 {
		t.Fatalf("precondition: no Shadow Effigy %d appeared", creatureShadowEffigy)
	}

	bot.CombatReady(t) // god mode only: the refund must be visible, so no .cheat power
	time.Sleep(gcd)    // let the Shadow Effigy mana cost reach the client
	manaBefore, _ := bot.PlayerPower()
	if res := castLanded(t, bot, spellSpiritualRecall, 0, 2); !res.Success {
		t.Fatalf("Spiritual Recall refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	manaAfter, _ := bot.PlayerPower()

	if obj := bot.World.GetObject(effigy); obj != nil && obj.IsAlive() {
		t.Errorf("E2E_FAIL: Shadow Effigy still present after Spiritual Recall (mana %d -> %d) (#384)", manaBefore, manaAfter)
		return
	}
	if manaAfter <= manaBefore {
		t.Errorf("E2E_FAIL: Shadow Effigy removed but no mana restored (%d -> %d) (#384)", manaBefore, manaAfter)
		return
	}
	t.Logf("E2E_PASS: Shadow Effigy removed by Spiritual Recall, mana %d -> %d", manaBefore, manaAfter)
}
