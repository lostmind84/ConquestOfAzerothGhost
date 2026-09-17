//go:build e2e

package summonspets_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specBarbarianAncestors   uint32 = 3
	talentAncestorsCall      uint32 = 31153
	spellAncestorsCallTalent uint32 = 804729 // passive, teaches 804834
	spellAncestorsCallTroll  uint32 = 804834 // SUMMON_PET creature 51265
)

var ancestorsCallRanks = []uint32{804834, 804836, 804846, 804869, 804872, 804898, 804901, 804905, 805043}

// Main project issues #112 and #168: the Ancestor's Call talent gives no working summon; casting Ancestor's Call
// plays an animation but no Honored Ancestor appears, and two Ancestor's Call buttons exist.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run AncestorsCall -count=1 -v
func TestBarbarian_AncestorsCallSummons(t *testing.T) {
	bot := newBot(t, "BbAnc", e2eharness.RaceTroll, classBarbarian, 20)
	bot.SetSpecialization(t, specBarbarianAncestors)
	bot.SetTalentRank(t, talentAncestorsCall, 1)
	waitKnows(t, bot, spellAncestorsCallTalent)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !bot.World.KnowsSpell(spellAncestorsCallTroll); {
		time.Sleep(100 * time.Millisecond)
	}
	var known []uint32
	for _, id := range ancestorsCallRanks {
		if bot.World.KnowsSpell(id) {
			known = append(known, id)
		}
	}
	t.Logf("known Ancestor's Call spells: %v", known)
	if !bot.World.KnowsSpell(spellAncestorsCallTroll) {
		t.Fatalf("E2E_FAIL: the Ancestor's Call talent did not teach %d (#112)", spellAncestorsCallTroll)
	}
	bot.CombatReadyFull(t)
	res, err := bot.TryCast(t, spellAncestorsCallTroll, 0, 10*time.Second)
	if err != nil {
		t.Fatalf("Ancestor's Call: %v", err)
	}
	t.Logf("cast result: success %v %s", res.Success, e2eharness.SpellFailReasonName(res.FailReason))
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if bot.PlayerPetGUID() != 0 {
			t.Logf("E2E_PASS: Ancestor's Call summoned pet 0x%X (#112, #168)", bot.PlayerPetGUID())
			return
		}
	}
	t.Errorf("E2E_FAIL: Ancestor's Call summoned nothing (#112, #168)")
}
