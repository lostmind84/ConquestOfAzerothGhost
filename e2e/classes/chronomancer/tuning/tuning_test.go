//go:build e2e

package tuning_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/classes/chronomancer"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	SpellEpochR8 uint32 = 504575 // Epoch Rank 8, the rank a level 60 Chronomancer owns

	smsgSpellHealLog uint16 = 0x150
	healsWanted             = 4
	untunedMin              = 798
	untunedMax              = 841
	tunedMin                = 638 // 798 * 0.80, rounded down
	tunedMax                = 673 // 841 * 0.80, rounded up
)

// Main project issue #250: CoA tuning auras are not applied, so Epoch heals for more than
// its tooltip, which multiplies by (100 + 887010 effect 3) / 100.
//
// The tuning auras are passive and hidden (Aura::CanBeSentToClient), so the client never sees
// them and the heal amount is the evidence. Epoch rank 8 at level 60 rolls 798..841
// (BasePoints 797, DieSides 44, no per-level gain at its SpellLevel 60). The class aura's
// effect 3 is -0.4 healing per level above 10, capped at 60: -20%, so a naked non-critical
// heal must land in 638..673 with tuning and in 798..841 without it.
//
//	go test -tags=e2e ./e2e/classes/chronomancer/tuning -count=1 -v
func TestChronomancer_NormalTuningAppliesToHealing(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "Tune",
		Race:   e2eharness.RaceHuman,
		Class:  chronomancer.ClassID,
		Level:  60,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.CheatPower(t)
	bot.SetSpecialization(t, chronomancer.SpecTime, chronomancer.AbilityTimeBaseline)
	time.Sleep(time.Second)

	heals := make(chan uint32, 16)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellHealLog {
			return
		}
		if amount, ok := parseHealLog(data, bot.World.CharGUID(), SpellEpochR8); ok {
			select {
			case heals <- amount:
			default:
			}
		}
	})
	t.Cleanup(cancel)

	for _, amount := range nonCriticalHeals(t, bot, heals) {
		switch {
		case amount >= tunedMin && amount <= tunedMax:
			t.Logf("E2E_PASS: Epoch healed %d, within the tuned range %d..%d", amount, tunedMin, tunedMax)
		case amount >= untunedMin && amount <= untunedMax:
			t.Errorf("E2E_FAIL: Epoch healed %d, the untuned range %d..%d: class tuning not applied (#250)",
				amount, untunedMin, untunedMax)
		default:
			t.Errorf("E2E_FAIL: Epoch healed %d, outside both the tuned and untuned ranges", amount)
		}
	}
}

// nonCriticalHeals casts Epoch on self until it has healsWanted non-critical heal amounts.
func nonCriticalHeals(t *testing.T, bot *e2eharness.ScenarioBot, heals chan uint32) []uint32 {
	t.Helper()
	var got []uint32
	for attempt := 0; len(got) < healsWanted && attempt < 4*healsWanted; attempt++ {
		res, err := bot.TryCast(t, SpellEpochR8, bot.World.CharGUID(), 6*time.Second)
		if err != nil || !res.Success {
			t.Logf("Epoch cast refused: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
			time.Sleep(time.Second)
			continue
		}
		select {
		case amount := <-heals:
			if amount > 0 {
				got = append(got, amount)
			}
		case <-time.After(3 * time.Second):
			t.Logf("no heal log for Epoch")
		}
		time.Sleep(1600 * time.Millisecond)
	}
	if len(got) < healsWanted {
		t.Fatalf("only %d non-critical Epoch heals recorded", len(got))
	}
	return got
}

// parseHealLog reads SMSG_SPELLHEALLOG (Unit::SendHealSpellLog) and returns the heal amount,
// or 0 with ok=true for a critical heal.
func parseHealLog(data []byte, caster uint64, spellID uint32) (uint32, bool) {
	r := bytes.NewReader(data)
	if _, err := packedGUID(r); err != nil {
		return 0, false
	}
	source, err := packedGUID(r)
	if err != nil || source != caster {
		return 0, false
	}
	var log struct {
		SpellID  uint32
		Heal     uint32
		Overheal uint32
		Absorb   uint32
		Critical uint8
	}
	if binary.Read(r, binary.LittleEndian, &log) != nil || log.SpellID != spellID {
		return 0, false
	}
	if log.Critical != 0 {
		return 0, true
	}
	return log.Heal, true
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
