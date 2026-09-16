//go:build e2e

package doomrend_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassReaper uint8 = 30

	AbilityDoomrend  uint32 = 800172 // Doomrend Rank 1, requires Soul Infusion (CasterAuraSpell 803031)
	AuraSoulInfusion uint32 = 803031 // granted by the server at 3 Reaped Souls
	AuraReapedSoul   uint32 = 500363
	reapedSoulsCap          = 3
	botLevel                = 13 // level reported in #419
	auraWaitTimeout         = 3 * time.Second
	settle                  = time.Second
	globalCooldown          = 1400 * time.Millisecond

	missMiss  uint8 = 1 // SPELL_MISS_MISS
	missDodge uint8 = 3 // SPELL_MISS_DODGE
	missParry uint8 = 4 // SPELL_MISS_PARRY
)

// Doomrend must keep Soul Infusion when it misses, is dodged or parried.
// Main project issue #419; the 2026-07-31 changelog says Soul Infusion spells
// refund their cost on a miss, dodge or parry.
//
// The heroic training dummy is left at level 83 so a level 13 Reaper misses often.
//
//	go test -tags=e2e ./e2e/classes/reaper/doomrend -run Avoided -count=1 -v
func TestReaper_DoomrendKeepsSoulInfusionWhenAvoided(t *testing.T) {
	bot, dummy := setup(t, "DoomA", false)
	results := watchDoomrendResults(t, bot)

	const attempts = 30
	for attempt := 1; attempt <= attempts; attempt++ {
		grantSoulInfusion(t, bot, dummy)
		miss, ok := castDoomrend(t, bot, dummy, results)
		if !ok {
			continue
		}
		if miss != missMiss && miss != missDodge && miss != missParry {
			t.Logf("cast %d: result %d, not an avoided attack, retrying", attempt, miss)
			time.Sleep(globalCooldown)
			continue
		}

		time.Sleep(settle)
		if !bot.HasAura(AuraSoulInfusion) {
			t.Errorf("E2E_FAIL: Soul Infusion consumed by a Doomrend that was avoided (result %d) (#419)", miss)
		} else if souls := bot.AuraStacks(AuraReapedSoul); souls != reapedSoulsCap {
			t.Errorf("E2E_FAIL: %d Reaped Soul(s) left after an avoided Doomrend, want %d (#419)", souls, reapedSoulsCap)
		} else {
			t.Logf("E2E_PASS: avoided Doomrend (result %d) kept Soul Infusion and %d Reaped Souls", miss, souls)
		}
		return
	}
	t.Fatalf("no Doomrend was missed, dodged or parried in %d casts", attempts)
}

// Control: a Doomrend that lands still consumes Soul Infusion and its Reaped Souls.
//
//	go test -tags=e2e ./e2e/classes/reaper/doomrend -run Lands -count=1 -v
func TestReaper_DoomrendConsumesSoulInfusionWhenItLands(t *testing.T) {
	bot, dummy := setup(t, "DoomH", true)
	results := watchDoomrendResults(t, bot)

	const attempts = 10
	for attempt := 1; attempt <= attempts; attempt++ {
		grantSoulInfusion(t, bot, dummy)
		miss, ok := castDoomrend(t, bot, dummy, results)
		if !ok || miss != 0 {
			t.Logf("cast %d: result %d, not a hit, retrying", attempt, miss)
			time.Sleep(globalCooldown)
			continue
		}

		time.Sleep(settle)
		if bot.HasAura(AuraSoulInfusion) {
			t.Errorf("E2E_FAIL: Soul Infusion still active after a Doomrend hit")
		} else if souls := bot.AuraStacks(AuraReapedSoul); souls != 0 {
			t.Errorf("E2E_FAIL: %d Reaped Soul(s) left after a Doomrend hit", souls)
		} else {
			t.Logf("E2E_PASS: Doomrend hit consumed Soul Infusion and its Reaped Souls")
		}
		return
	}
	t.Fatalf("no Doomrend landed in %d casts", attempts)
}

func setup(t *testing.T, prefix string, lowLevelDummy bool) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: prefix,
		Race:   e2eharness.RaceOrc,
		Class:  ClassReaper,
		Level:  botLevel,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, AbilityDoomrend)

	dummy := bot.Spawn(t, e2eharness.CreatureHeroicTrainingDummy, 10*time.Second)
	_ = bot.World.SetTarget(dummy)
	if lowLevelDummy {
		bot.GM(t, fmt.Sprintf(".npc set level %d", botLevel-3)) // a level 83 dummy avoids most attacks
	}
	bot.CombatReady(t)
	bot.Face(t, dummy)
	return bot, dummy
}

// castDoomrend casts Doomrend and returns the target's SMSG_SPELL_GO result (0 = hit).
func castDoomrend(t *testing.T, bot *e2eharness.ScenarioBot, dummy uint64, results <-chan uint8) (uint8, bool) {
	t.Helper()
	for len(results) > 0 {
		<-results
	}
	res, err := bot.TryCast(t, AbilityDoomrend, dummy, 5*time.Second)
	if err != nil {
		t.Fatalf("cast Doomrend: %v", err)
	}
	if !res.Success {
		t.Fatalf("Doomrend was refused with Soul Infusion: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	select {
	case miss := <-results:
		return miss, true
	case <-time.After(2 * time.Second):
		t.Logf("no SMSG_SPELL_GO target list seen for Doomrend")
		return 0, false
	}
}

// watchDoomrendResults reports, for each of the bot's Doomrend SMSG_SPELL_GO, the miss
// condition of its first target (0 when the target is in the hit list).
func watchDoomrendResults(t *testing.T, bot *e2eharness.ScenarioBot) <-chan uint8 {
	out := make(chan uint8, 8)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgSpellGo {
			return
		}
		miss, ok := parseSpellGo(data, bot.World.CharGUID(), AbilityDoomrend)
		if !ok {
			return
		}
		select {
		case out <- miss:
		default:
		}
	})
	t.Cleanup(cancel)
	return out
}

// parseSpellGo reads the 3.3.5a SMSG_SPELL_GO header and target lists (Spell::SendSpellGo).
func parseSpellGo(data []byte, caster uint64, spellID uint32) (uint8, bool) {
	r := bytes.NewReader(data)
	if _, err := packedGUID(r); err != nil { // cast item or caster
		return 0, false
	}
	realCaster, err := packedGUID(r)
	if err != nil || realCaster != caster {
		return 0, false
	}
	var header struct {
		CastCount uint8
		SpellID   uint32
		CastFlags uint32
		Time      uint32
		HitCount  uint8
	}
	if binary.Read(r, binary.LittleEndian, &header) != nil || header.SpellID != spellID {
		return 0, false
	}
	if header.HitCount > 0 {
		return 0, true
	}
	var missCount uint8
	if binary.Read(r, binary.LittleEndian, &missCount) != nil || missCount == 0 {
		return 0, false
	}
	var miss struct {
		GUID   uint64
		Reason uint8
	}
	if binary.Read(r, binary.LittleEndian, &miss) != nil {
		return 0, false
	}
	return miss.Reason, true
}

func packedGUID(r io.ByteReader) (uint64, error) {
	mask, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	var guid uint64
	for i := 0; i < 8; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		guid |= uint64(b) << (8 * i)
	}
	return guid, nil
}

// grantSoulInfusion adds Reaped Souls up to the cap, waits for the server to grant Soul Infusion
// and selects target again.
func grantSoulInfusion(t *testing.T, bot *e2eharness.ScenarioBot, target uint64) {
	t.Helper()
	_ = bot.World.SetTarget(bot.World.CharGUID()) // `.aura` applies to the current selection
	for bot.AuraStacks(AuraReapedSoul) < reapedSoulsCap {
		before := bot.AuraStacks(AuraReapedSoul)
		bot.GM(t, fmt.Sprintf(".aura %d", AuraReapedSoul))
		deadline := time.Now().Add(auraWaitTimeout)
		for bot.AuraStacks(AuraReapedSoul) == before {
			if time.Now().After(deadline) {
				t.Fatalf("Reaped Soul stayed at %d stack(s) after .aura", before)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	bot.WaitAura(t, AuraSoulInfusion, auraWaitTimeout)
	_ = bot.World.SetTarget(target)
}
