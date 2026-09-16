//go:build e2e

package talents_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// replacementCase is one "transforms your X into Y" talent: taking the talent in its specialization must put one
// rank of the replacement spell in the spellbook.
type replacementCase struct {
	issue        int
	name         string
	class        uint8
	race         uint8
	spec         uint32
	talentEntry  uint32
	talentSpell  uint32
	original     uint32   // base spell the talent transforms
	replacements []uint32 // any rank of the transformed spell
}

// Main project issues #475, #484, #496, #514, #515, #554, #555 and #556 are generated reports saying the
// "transform" part of these talents has no server handler. Rows come from Spell.dbc and the talent catalog.
//
//	go test -tags=e2e ./e2e/coa/talents -run TalentReplacements -count=1 -v
func TestTalentReplacements(t *testing.T) {
	cases := []replacementCase{
		{475, "GrimReaper", classReaper, e2eharness.RaceOrc, 56, 7267, 504269, 803985, []uint32{807234}},
		{496, "ShudderScythe", classReaper, e2eharness.RaceOrc, 56, 5562, 805708, 500376, []uint32{572382}},
		{484, "TempestSovereign", classStormbringer, e2eharness.RaceOrc, 14, 31165, 560020, 500040,
			[]uint32{804017, 503352, 503353, 503354, 503355, 503356, 503357, 503358, 503359}},
		{514, "Timerend", classChronomancer, e2eharness.RaceOrc, 32, 3997, 707430, 520175,
			[]uint32{801291, 501831, 501832, 501833, 501834, 501835, 572578}},
		{515, "ArtificersWand", classChronomancer, e2eharness.RaceOrc, 33, 12853, 804478, 804418,
			[]uint32{561284, 561354, 561355, 561356, 561357}},
		{556, "SanguineEssence", classBloodmage, e2eharness.RaceOrc, 25, 7750, 505188, 562720, []uint32{680692}},
		{555, "Transgression", classBloodmage, e2eharness.RaceOrc, 26, 31175, 504728, 562720, []uint32{801076}},
		{554, "AccursedForm", classBloodmage, e2eharness.RaceOrc, 27, 29485, 504710, 562720, []uint32{562572}},
	}
	for i, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			bot := newBot(t, fmt.Sprintf("TRep%d", i), c.race, c.class, 60)
			bot.SetSpecialization(t, c.spec)
			if !bot.World.KnowsSpell(c.original) {
				bot.Learn(t, c.original)
			}
			bot.SetTalentRank(t, c.talentEntry, 1)
			if !waitSpell(bot, c.talentSpell, 5*time.Second) {
				t.Fatalf("precondition: talent spell %d not learned after .localtalent %d 1", c.talentSpell, c.talentEntry)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				for _, id := range c.replacements {
					if bot.World.KnowsSpell(id) {
						t.Logf("E2E_PASS: #%d talent %d replaced %d with %d", c.issue, c.talentSpell, c.original, id)
						return
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
			t.Errorf("E2E_FAIL: #%d talent %d taken, no rank of the replacement %v is known (original %d known: %v)",
				c.issue, c.talentSpell, c.replacements, c.original, bot.World.KnowsSpell(c.original))
		})
	}
}
