//go:build e2e

package summonspets_test

import (
	"testing"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWildwalker        uint8  = 31
	specSpiritBeast        uint32 = 59
	spellSpiritBeastMaster uint32 = 92148  // passive, teaches Harness Animal Spirit
	spellHarnessSpirit     uint32 = 574301 // tame
	spellCallAnimalSpirit  uint32 = 574302
	spellDismissSpirit     uint32 = 574303
	spellRecallSpirit      uint32 = 500860 // revive
)

// Main project issue #107: a level 10 Wildwalker can tame an animal spirit but has no call, dismiss or revive
// spell for it, so a dead pet is lost.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run WildwalkerPetSpells -count=1 -v
func TestWildwalker_PetSpells(t *testing.T) {
	bot := newBot(t, "WwPet", e2eharness.RaceTauren, classWildwalker, 10)
	bot.SetSpecialization(t, specSpiritBeast)
	waitKnows(t, bot, spellHarnessSpirit)
	t.Logf("Spirit Beast Master %v, spellbook count of Harness Animal Spirit %d", bot.World.KnowsSpell(spellSpiritBeastMaster),
		bot.SpellbookCount(spellHarnessSpirit))
	missing := 0
	for _, s := range []struct {
		name string
		id   uint32
	}{{"Call Animal Spirit", spellCallAnimalSpirit}, {"Dismiss Animal Spirit", spellDismissSpirit},
		{"Recall Animal Spirit", spellRecallSpirit}} {
		if bot.World.KnowsSpell(s.id) {
			t.Logf("knows %s %d", s.name, s.id)
		} else {
			t.Logf("does not know %s %d", s.name, s.id)
			missing++
		}
	}
	if missing > 0 {
		t.Errorf("E2E_FAIL: a level 10 Spirit Beast Wildwalker lacks %d pet spell(s) (#107)", missing)
	}
}
