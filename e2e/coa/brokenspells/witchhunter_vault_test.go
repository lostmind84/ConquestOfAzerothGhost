//go:build e2e

package brokenspells_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const vaultReportLevel = 12 // level in #1467

// Main project issue #1467: casting Vault plays the jump animation but does not move the character.
// The module moves a player with SMSG_MOVE_KNOCK_BACK (#217), so a Vault that moves the caster sends one.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run VaultDisplaces -count=1 -v
func TestWitchHunter_VaultDisplacesCaster(t *testing.T) {
	bot := newBot(t, "WhVlt", e2eharness.RaceOrc, classWitchHunter, vaultReportLevel)
	bot.Learn(t, spellVault)
	bot.CombatReadyFull(t)

	self := bot.World.CharGUID()
	var knockbacks atomic.Int32
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode == client.SmsgMoveKnockBack && readSelfGUIDPacket(data, self) {
			knockbacks.Add(1)
		}
	})
	defer cancel()

	if res := castLanded(t, bot, spellVault, 0, 3); !res.Success {
		t.Fatalf("Vault refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	t.Logf("SMSG_MOVE_KNOCK_BACK for the caster after Vault: %d", knockbacks.Load())
	if knockbacks.Load() == 0 {
		t.Errorf("E2E_FAIL: Vault was cast but the server did not move the caster (#1467)")
		return
	}
	t.Logf("E2E_PASS: Vault moves the caster (#1467)")
}
