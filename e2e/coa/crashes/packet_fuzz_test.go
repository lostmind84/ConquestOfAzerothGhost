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
	cmsgRecoverVendoredItem         uint16 = 0x05E0
	cmsgQueryCustomStore            uint16 = 0x06B9
	cmsgPurchaseCustomStoreItem     uint16 = 0x06BB
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
	assertMalformedPacketKeepsWorldAlive(t, "SortF1", cmsgAscensionCharacterSortOrder, []byte{0xC2})
}

// F3: the recovery request has a complete shape but an invalid UTF-8 ID.
func TestPacketFuzz_ItemRecoveryInvalidUTF8(t *testing.T) {
	assertMalformedPacketKeepsWorldAlive(t, "RecovF3", cmsgRecoverVendoredItem,
		[]byte{0x00, 0x10, 0x01, 0x0A, 0x01, 0xFC, 0x00})
}

// F4/F5: both custom-store consumers read a store ID from this empty body.
func TestPacketFuzz_StoreQueryTruncated(t *testing.T) {
	assertMalformedPacketKeepsWorldAlive(t, "StoreQ", cmsgQueryCustomStore, nil)
}

// F4/F5: both custom-store consumers read two uint32 values from this body.
func TestPacketFuzz_StorePurchaseTruncated(t *testing.T) {
	assertMalformedPacketKeepsWorldAlive(t, "StoreP", cmsgPurchaseCustomStoreItem, []byte{0, 0, 0, 0})
}

func assertMalformedPacketKeepsWorldAlive(t *testing.T, prefix string, opcode uint16, payload []byte) {
	t.Helper()
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: prefix,
		Count:  2,
		Race:   e2eharness.RaceOrc,
		Class:  13, // Witch Doctor — a CoA class, never leave a default Warrior on a slot
	})
	attacker, probe := bots[0], bots[1]

	if !e2eharness.SessionAlive(probe.Session) {
		e2eharness.HarnessFailf(t, "precondition: probe session not alive before the test")
	}

	if err := attacker.World.SendRawPacket(opcode, payload); err != nil {
		e2eharness.HarnessFailf(t, "sending malformed opcode 0x%04X failed: %v", opcode, err)
	}

	time.Sleep(settleDelay)
	e2eharness.ProbeWorldAlive(t, probe, 0)
}
