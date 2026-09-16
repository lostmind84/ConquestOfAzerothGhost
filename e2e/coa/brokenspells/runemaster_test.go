//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellRuneshroud      uint32 = 500288
	auraRuneshroudMarker uint32 = 808089 // "Runeshroud or Waveforged", CasterAuraSpell of Palm Sigils
	spellPalmSigilFire   uint32 = 534803
	runemasterLevel             = 4 // #410 and #343
)

// Main project issue #410: Palm Sigil: Fire is refused ("you can't do that yet") inside Runeshroud.
// Reported on 7f0761f, which already has 9fb23e840 (#162) syncing the 808089 marker.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run PalmSigilFire -count=1 -v
func TestRunemaster_PalmSigilFireInRuneshroud(t *testing.T) {
	bot := runeshroudSetup(t, "RmPalm")
	t.Logf("marker %d present: %v", auraRuneshroudMarker, bot.HasAura(auraRuneshroudMarker))

	res, err := bot.TryCast(t, spellPalmSigilFire, 0, castTimeout)
	if err != nil {
		t.Fatalf("Palm Sigil: Fire: %v", err)
	}
	if !res.Success {
		t.Errorf("E2E_FAIL: Palm Sigil: Fire refused in Runeshroud: %s (#410)", e2eharness.SpellFailReasonName(res.FailReason))
		return
	}
	t.Logf("E2E_PASS: Palm Sigil: Fire cast in Runeshroud")
}

// Main project issue #343: Runeshroud does not reduce movement speed.
// The slow lives in hidden aura 808090 (-32% run speed), invisible to the bot, so the test reads the
// run speed the server forces on the client.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run RuneshroudSlows -count=1 -v
func TestRunemaster_RuneshroudSlows(t *testing.T) {
	bot := newBot(t, "RmSlow", e2eharness.RaceBloodElf, classRunemaster, runemasterLevel)
	speeds := watchRunSpeed(t, bot)
	bot.Learn(t, spellRuneshroud)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellRuneshroud, 0, 2); !res.Success {
		t.Fatalf("Runeshroud refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellRuneshroud, 3*time.Second)
	time.Sleep(settle)

	speed, ok := speeds.last()
	if !ok || speed >= baseRunSpeed {
		t.Errorf("E2E_FAIL: run speed %.2f (forced: %v) in Runeshroud, want below %.2f (#343)", speed, ok, baseRunSpeed)
		return
	}
	t.Logf("E2E_PASS: run speed %.2f in Runeshroud (%.0f%% of %.2f)", speed, 100*speed/baseRunSpeed, baseRunSpeed)
}

func runeshroudSetup(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceBloodElf, classRunemaster, runemasterLevel)
	bot.Learn(t, spellRuneshroud)
	bot.Learn(t, spellPalmSigilFire)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellRuneshroud, 0, 2); !res.Success {
		t.Fatalf("Runeshroud refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellRuneshroud, 3*time.Second)
	time.Sleep(gcd)
	return bot
}
