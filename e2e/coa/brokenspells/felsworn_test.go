//go:build e2e

package brokenspells_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellIllidansGuile     uint32 = 806109 // 8 s school immunity; E2 triggers 807235 after 300 ms
	spellIllidansGuileLock uint32 = 807235 // root (-101% speed), pacify and silence, stun, 8 s
	talentIllidansGuile    uint32 = 29924  // CoA talent entry of 806109
	spellBloodFury         uint32 = 20572  // instant self buff used as "any action"
	smsgForceMoveRoot      uint16 = 0x00E8
	felswornGuileLevel            = 30 // 806109 BaseLevel 28
)

// Main project issue #1513: Illidan's Guile says "you cannot attack, move or cast spells" for its duration,
// but the Felsworn can run and cast while it lasts.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run IllidansGuileLocksCaster -count=1 -v
func TestFelsworn_IllidansGuileLocksCaster(t *testing.T) {
	bot := newBot(t, "FsGui", e2eharness.RaceBloodElf, classFelsworn, felswornGuileLevel)
	bot.SetTalentRank(t, talentIllidansGuile, 1)
	bot.Learn(t, spellIllidansGuile)
	bot.Learn(t, spellBloodFury)
	bot.CombatReadyFull(t)

	var rooted atomic.Int32
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode == smsgForceMoveRoot && readSelfGUIDPacket(data, self) {
			rooted.Add(1)
		}
	})
	t.Cleanup(cancel)

	if res := castLanded(t, bot, spellIllidansGuile, self, 3); !res.Success {
		t.Fatalf("Illidan's Guile refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellIllidansGuile, 3*time.Second)
	time.Sleep(time.Second)

	res, err := bot.TryCast(t, spellBloodFury, self, castTimeout)
	if err != nil {
		t.Fatalf("cast %d: %v", spellBloodFury, err)
	}
	lock := bot.HasAura(spellIllidansGuileLock)
	t.Logf("during Illidan's Guile: lock aura %d=%v, SMSG_FORCE_MOVE_ROOT=%d, Blood Fury success=%v (%s)",
		spellIllidansGuileLock, lock, rooted.Load(), res.Success, e2eharness.SpellFailReasonName(res.FailReason))

	failed := false
	if rooted.Load() == 0 {
		t.Errorf("E2E_FAIL: Illidan's Guile did not root the caster (#1513)")
		failed = true
	}
	if res.Success {
		t.Errorf("E2E_FAIL: a spell was cast during Illidan's Guile (#1513)")
		failed = true
	}
	if failed {
		return
	}
	if !bot.TryWaitAuraGone(t, spellIllidansGuileLock, 10*time.Second) {
		t.Fatalf("E2E_FAIL: the Illidan's Guile lock %d did not expire (#1513)", spellIllidansGuileLock)
	}
	if res := castLanded(t, bot, spellBloodFury, self, 3); !res.Success {
		t.Fatalf("E2E_FAIL: still unable to cast after Illidan's Guile: %s (#1513)", e2eharness.SpellFailReasonName(res.FailReason))
	}
	t.Logf("E2E_PASS: Illidan's Guile roots the caster and blocks casting until it ends (#1513)")
}
