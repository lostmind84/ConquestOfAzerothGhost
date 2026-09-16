//go:build e2e

package brokenspells_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellScytheRush       uint32 = 500359
	auraScytheRushMarker  uint32 = 500377 // 20 s "Cannot be affected by Scythe Rush"
	spellUnderwalk        uint32 = 800797
	spellSpectralHand     uint32 = 520046 // pickpocket, requires Underwalk
	creatureDefiasThug    uint32 = 38
	scytheRushReaperLevel        = 14 // level in #423
	spectralHandLevel            = 13 // level in #286
)

// Main project issue #423: Scythe Rush can be used on the same target again within 20 seconds.
// Reported before 9fb68e05d (#328) restored the Scythe Rush chain that applies the 500377 marker.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run ScytheRush -count=1 -v
func TestReaper_ScytheRushOncePerTargetEvery20Seconds(t *testing.T) {
	bot := newBot(t, "RpRush", e2eharness.RaceOrc, classReaper, scytheRushReaperLevel)
	bot.Learn(t, spellScytheRush)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, scytheRushReaperLevel-2)
	bot.CombatReadyFull(t)

	stepBack := func() {
		x, y, z, mapID := bot.Pos()
		bot.Teleport(t, x+12, y, z, mapID)
		time.Sleep(settle)
		bot.Face(t, dummy)
	}

	// The marker comes with the Runic Power energize of a rush that hits; retry misses.
	landed := false
	for attempt := 1; attempt <= 5 && !landed; attempt++ {
		stepBack()
		before, _ := bot.PlayerPower()
		if res := castLanded(t, bot, spellScytheRush, dummy, 3); !res.Success {
			t.Fatalf("first Scythe Rush refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(settle)
		after, _ := bot.PlayerPower()
		marker := bot.UnitHasAura(dummy, auraScytheRushMarker)
		t.Logf("rush %d: power %d -> %d, dummy marker %d visible: %v", attempt, before, after, auraScytheRushMarker, marker)
		landed = after > before || marker
		if !landed {
			time.Sleep(gcd)
		}
	}
	if !landed {
		t.Fatalf("precondition: no Scythe Rush generated Runic Power or applied the marker in 5 casts")
	}

	stepBack()
	time.Sleep(gcd)
	res, err := bot.TryCast(t, spellScytheRush, dummy, castTimeout)
	if err != nil {
		t.Fatalf("second Scythe Rush: %v", err)
	}
	if res.Success {
		t.Errorf("E2E_FAIL: second Scythe Rush on the same target within 20 s succeeded (#423)")
		return
	}
	t.Logf("E2E_PASS: second Scythe Rush refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
}

// Main project issue #286: Spectral Hand opens the target's pickpocket loot but the coins cannot be taken.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run SpectralHand -count=1 -v
func TestReaper_SpectralHandTakesCoins(t *testing.T) {
	bot := newBot(t, "RpHand", e2eharness.RaceOrc, classReaper, spectralHandLevel)
	bot.Learn(t, spellUnderwalk)
	bot.Learn(t, spellSpectralHand)
	bot.SetMoney(t, 0)

	gold := make(chan uint32, 4)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgLootResponse || len(data) < 13 {
			return
		}
		var head struct {
			GUID uint64
			Type uint8
			Gold uint32
		}
		if binary.Read(bytes.NewReader(data), binary.LittleEndian, &head) == nil {
			select {
			case gold <- head.Gold:
			default:
			}
		}
	})
	defer cancel()

	thug := bot.Spawn(t, creatureDefiasThug, 10*time.Second)
	_ = bot.World.SetTarget(thug)
	bot.GM(t, ".npc set faction 7") // neutral creature, stays out of combat
	bot.CombatReady(t)
	castLanded(t, bot, spellUnderwalk, 0, 2)
	bot.WaitAura(t, spellUnderwalk, 3*time.Second)
	bot.Face(t, thug)

	for attempt := 1; attempt <= 3; attempt++ {
		if res := castLanded(t, bot, spellSpectralHand, thug, 2); !res.Success {
			t.Fatalf("Spectral Hand refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		select {
		case coins := <-gold:
			if coins == 0 {
				t.Logf("attempt %d: pickpocket loot has no coins", attempt)
				_ = bot.World.LootRelease(thug)
				continue
			}
			_ = bot.World.LootMoney()
			deadline := time.Now().Add(3 * time.Second)
			for bot.PlayerMoney() < coins && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			if got := bot.PlayerMoney(); got < coins {
				t.Errorf("E2E_FAIL: pickpocket offered %d copper, money is %d after taking it (#286)", coins, got)
				return
			}
			t.Logf("E2E_PASS: took %d copper with Spectral Hand", coins)
			return
		case <-time.After(3 * time.Second):
			t.Fatalf("no SMSG_LOOT_RESPONSE after Spectral Hand")
		}
	}
	t.Fatalf("precondition: no pickpocket loot with coins in 3 attempts")
}
