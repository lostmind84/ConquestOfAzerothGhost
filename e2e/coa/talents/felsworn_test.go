//go:build e2e

package talents_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #376: Felsworn talents are reset by logging out and back in.
//
//	go test -tags=e2e ./e2e/coa/talents -run FelswornTalentsSurviveRelog -count=1 -v
func TestFelswornTalentsSurviveRelog(t *testing.T) {
	bot := newBot(t, "FsRelog", e2eharness.RaceBloodElf, classFelsworn, 60)
	takeTalent(t, bot, specFelswornFelblood, talentWrathOfSargeras, spellWrathOfSargeras)
	if _, ok := serverAuras(t, bot, bot.World.CharGUID())[spellWrathOfSargeras]; !ok {
		t.Fatalf("precondition: Wrath of Sargeras aura %d not applied before the relog", spellWrathOfSargeras)
	}
	bot.Relog(t) // a normal logout, which saves the character
	time.Sleep(2 * time.Second)
	known := bot.World.KnowsSpell(spellWrathOfSargeras)
	_, aura := serverAuras(t, bot, bot.World.CharGUID())[spellWrathOfSargeras]
	t.Logf("after relog: Wrath of Sargeras known %v, aura %v", known, aura)
	if !known || !aura {
		t.Errorf("E2E_FAIL: Wrath of Sargeras lost by the relog (known %v, aura %v) (#376)", known, aura)
		return
	}
	t.Logf("E2E_PASS: Felsworn talent kept across the relog")
}
