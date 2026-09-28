//go:build e2e

package crashes_test

import (
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Regression cases for malformed CoA extension packets found by local fuzzing.
// These handlers run on the worldserver socket thread, so each parser must reject
// malformed payloads without letting a ByteBufferException escape.
//
// Each test sends the malformed packet from an "attacker" session and then checks a
// separate "probe" session is still alive. A worldserver crash drops every session, so a
// dead probe means the whole server went down.
//
//	E2E_PLAINTEXT_HEADERS=0 go test -tags=e2e ./e2e/coa/crashes -count=1 -v -p 1 -run PacketFuzz

const (
	cmsgAscensionCharacterSortOrder uint16 = 0x0772
)

func TestPacketFuzz_ProbeHealthy(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "SoPrb",
		Race:   e2eharness.RaceOrc,
		Class:  13,
	})
	e2eharness.ProbeWorldAlive(t, bot, 0)
}

// F1: a 1-byte, non-UTF-8 CMSG_ASCENSION_CHARACTER_SORT_ORDER body previously let
// ByteBufferInvalidValueException escape from the socket thread.
func TestPacketFuzz_CharacterSortOrderInvalidUTF8(t *testing.T) {
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: "SortF1",
		Count:  2,
		Race:   e2eharness.RaceOrc,
		Class:  13, // Witch Doctor — a CoA class, never leave a default Warrior on a slot
	})
	attacker, probe := bots[0], bots[1]

	if !e2eharness.SessionAlive(probe.Session) {
		e2eharness.HarnessFailf(t, "precondition: probe session not alive before the test")
	}

	// 0xC2 is a UTF-8 two-byte lead with no continuation byte: an invalid string.
	if err := attacker.World.SendRawPacket(cmsgAscensionCharacterSortOrder, []byte{0xC2}); err != nil {
		e2eharness.HarnessFailf(t, "sending the malformed sort-order packet failed: %v", err)
	}

	time.Sleep(settleDelay)
	e2eharness.ProbeWorldAlive(t, probe, 0)
}
