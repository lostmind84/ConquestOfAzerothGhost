//go:build e2e

package brokenspells_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellXorothianWarsteed uint32  = 804703 // E0 SPELL_AURA_MOD_INCREASE_MOUNTED_SPEED BasePoints 19 (+20%)
	auraPathOfHellfire     uint32  = 804787
	warsteedReportLevel            = 45 // level in #1465
	mountedSpeedTolerance  float32 = 0.05
)

// Main project issue #1465: Xorothian Warsteed's tooltip promises +20% movement speed, but the player does
// not feel it (indoor use is intended, per the issue's comment).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run WarsteedMountedSpeed -count=1 -v
func TestKnightOfXoroth_WarsteedMountedSpeed(t *testing.T) {
	bot := newBot(t, "KoxWs", e2eharness.RaceOrc, classKnightXoroth, warsteedReportLevel)
	bot.Learn(t, spellXorothianWarsteed)
	speed := watchRunSpeed(t, bot)
	bot.CombatReadyFull(t)

	if res := castLanded(t, bot, spellXorothianWarsteed, bot.World.CharGUID(), 3); !res.Success {
		t.Fatalf("Xorothian Warsteed refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellXorothianWarsteed, 5*time.Second)
	time.Sleep(500 * time.Millisecond) // Path of Hellfire needs 3 s without damage

	got, seen := speed.last()
	want := baseRunSpeed * 1.2
	t.Logf("run speed after mounting: %.3f (seen=%v, Path of Hellfire stacks %d), expected %.3f",
		got, seen, bot.AuraStacks(auraPathOfHellfire), want)
	if !seen || got < want-mountedSpeedTolerance {
		t.Errorf("E2E_FAIL: Xorothian Warsteed run speed %.3f, expected %.3f (+20%%) (#1465)", got, want)
		return
	}
	t.Logf("E2E_PASS: Xorothian Warsteed raises run speed to %.3f (#1465)", got)
}

// readSelfGUIDPacket reports whether a packed-GUID-prefixed packet is about the bot.
func readSelfGUIDPacket(data []byte, self uint64) bool {
	return readPackedGUID(bytes.NewReader(data)) == self
}
