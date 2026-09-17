//go:build e2e

package talents_test

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classCultist uint8 = 25

	itemDiscerningEyeOfTheBeast uint32 = 1642992
	spellDiscerningEyeEnergize  uint32 = 59914

	smsgSpellEnergizeLog uint16 = 0x0151
)

// Main project issue #421: Discerning Eye of the Beast (equip: restore 2% mana on a kill that yields experience)
// restores no mana.
//
//	go test -tags=e2e ./e2e/coa/talents -run DiscerningEye -count=1 -v
func TestItem_DiscerningEyeRestoresManaOnKill(t *testing.T) {
	bot := newBot(t, "ItEye", e2eharness.RaceHuman, classCultist, 10)
	equip(t, bot, itemDiscerningEyeOfTheBeast)

	var mu sync.Mutex
	energized := 0
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgSpellEnergizeLog {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r)
		if readPackedGUID(r) != self {
			return
		}
		var spellID uint32
		if binary.Read(r, binary.LittleEndian, &spellID) == nil && spellID == spellDiscerningEyeEnergize {
			mu.Lock()
			energized++
			mu.Unlock()
		}
	})
	defer cancel()

	xp := bot.PlayerXP()
	for kill := 1; kill <= 2; kill++ {
		thug := spawnTarget(t, bot, creatureDefiasThug, 10)
		bot.DamageKill(t, []uint64{thug}, 100000, 10*time.Second)
	}
	time.Sleep(settle)
	if bot.PlayerXP() == xp {
		t.Fatalf("precondition: the kills gave no experience (%d)", xp)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := energized
		mu.Unlock()
		if got > 0 {
			t.Logf("E2E_PASS: Discerning Eye of the Beast restored mana %d time(s)", got)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("E2E_FAIL: two experience-yielding kills with Discerning Eye of the Beast, no %d mana restore (#421)",
		spellDiscerningEyeEnergize)
}
