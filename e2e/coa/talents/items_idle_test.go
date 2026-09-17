//go:build e2e

package talents_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	// Twisted Onslaught (enchant 105115): aura 42 with ProcFlags 0x10110 (melee and ranged abilities, harmful magic
	// spells) and a 15% chance; the proc applies 1968746 (Agility and 20 Energy over 15 s).
	spellTwistedOnslaught     uint32 = 1968716
	spellTwistedOnslaughtProc uint32 = 1968746
	smsgSpellGo               uint16 = 0x0132
)

// Main project issue #1427: item procs such as Twisted Onslaught trigger on a Felsworn standing idle.
//
//	go test -tags=e2e ./e2e/coa/talents -run TwistedOnslaughtIdle -count=1 -v
func TestFelsworn_TwistedOnslaughtIdle(t *testing.T) {
	bot := newBot(t, "FsIdle", e2eharness.RaceBloodElf, classFelsworn, 60)
	var mu sync.Mutex
	casts := map[uint32]int{}
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgSpellGo {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != self {
			return
		}
		readPackedGUID(r)
		var body struct {
			Count uint8
			Spell uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) == nil {
			mu.Lock()
			casts[body.Spell]++
			mu.Unlock()
		}
	})
	t.Cleanup(cancel)
	_ = bot.World.SetTarget(self)
	bot.GM(t, fmt.Sprintf(".aura %d", spellTwistedOnslaught))
	bot.GM(t, ".gm off")
	if _, ok := serverAuras(t, bot, self)[spellTwistedOnslaught]; !ok {
		t.Fatalf("precondition: Twisted Onslaught aura %d not applied", spellTwistedOnslaught)
	}
	// No specialization, then each Felsworn specialization (7 Felblood, 8 Slaying, 9 Demonology).
	for _, spec := range []uint32{0, 7, 8, 9} {
		if spec != 0 {
			bot.SetSpecialization(t, spec)
			if _, ok := serverAuras(t, bot, self)[spellTwistedOnslaught]; !ok {
				_ = bot.World.SetTarget(self)
				bot.GM(t, fmt.Sprintf(".aura %d", spellTwistedOnslaught))
			}
		}
		bot.GM(t, ".gm off")
		deadline := time.Now().Add(40 * time.Second)
		for time.Now().Before(deadline) {
			if bot.HasAura(spellTwistedOnslaughtProc) {
				mu.Lock()
				t.Errorf("E2E_FAIL: Twisted Onslaught procced on an idle Felsworn (spec %d); own casts: %v (#1427)", spec,
					casts)
				mu.Unlock()
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		mu.Lock()
		t.Logf("spec %d: no proc in 40 s idle; own casts so far: %v", spec, casts)
		mu.Unlock()
	}
	t.Logf("E2E_PASS: no Twisted Onslaught proc while idle in any specialization")
}
