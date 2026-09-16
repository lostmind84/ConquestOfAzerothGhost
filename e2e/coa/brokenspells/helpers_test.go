//go:build e2e

// Package brokenspells_test reproduces the "broken spells" reports of the CoA server issue tracker.
// Each file covers one issue (or a group of duplicates) and names it in its failure messages.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -count=1 -v -p 1
package brokenspells_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWitchHunter  uint8 = 15
	classKnightXoroth uint8 = 17
	classGuardian     uint8 = 18
	classWitchDoctor  uint8 = 13
	classFelsworn     uint8 = 14
	classBloodmage    uint8 = 20
	classPyromancer   uint8 = 24
	classStarcaller   uint8 = 26
	classSunCleric    uint8 = 27
	classReaper       uint8 = 30
	classRunemaster   uint8 = 32

	itemWalkingStick uint32 = 2495 // staff

	smsgSpellNonMeleeDamageLog uint16 = 0x0250

	castTimeout = 5 * time.Second
	settle      = time.Second
	gcd         = 1600 * time.Millisecond
)

// newBot creates one bot of the given class and level on the package pad.
func newBot(t *testing.T, prefix string, race, class uint8, level int) *e2eharness.ScenarioBot {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: prefix, Race: race, Class: class, Level: level})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	t.Cleanup(func() { bot.CleanupOwnedSummons(t) })
	return bot
}

// spawnTarget spawns a creature, sets its level and faces it. The heroic training dummy is level 83
// and avoids most attacks from a low-level bot unless its level is lowered.
func spawnTarget(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32, level int) uint64 {
	t.Helper()
	guid := bot.Spawn(t, entry, 10*time.Second)
	_ = bot.World.SetTarget(guid)
	if level > 0 {
		bot.GM(t, fmt.Sprintf(".npc set level %d", level))
	}
	bot.Face(t, guid)
	return guid
}

// equip adds an item and auto-equips it from the backpack, retrying until it shows on the character.
func equip(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32) {
	t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		bot.EquipEntry(t, entry, 1)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if _, ok := bot.EquippedSlot(entry); ok {
				return
			}
			for slot := uint8(23); slot < 39; slot++ {
				_ = bot.World.AutoEquipItem(255, slot)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	t.Fatalf("precondition: item %d not equipped", entry)
}

// knownRank returns the first spell of ranks the bot knows, learning ranks[0] when it knows none.
func knownRank(t *testing.T, bot *e2eharness.ScenarioBot, ranks ...uint32) uint32 {
	t.Helper()
	for i := len(ranks) - 1; i >= 0; i-- {
		if bot.World.KnowsSpell(ranks[i]) || bot.SpellbookCount(ranks[i]) > 0 {
			return ranks[i]
		}
	}
	bot.Learn(t, ranks[0])
	return ranks[0]
}

const (
	smsgForceRunSpeedChange uint16  = 0x00E2
	baseRunSpeed            float32 = 7.0
)

// runSpeedLog keeps the last run speed the server forced on the bot.
type runSpeedLog struct {
	mu    sync.Mutex
	speed float32
	seen  bool
}

func watchRunSpeed(t *testing.T, bot *e2eharness.ScenarioBot) *runSpeedLog {
	t.Helper()
	log := &runSpeedLog{}
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgForceRunSpeedChange {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != self {
			return
		}
		var body struct {
			Counter uint32
			Unk     uint8
			Speed   float32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		log.mu.Lock()
		log.speed, log.seen = body.Speed, true
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *runSpeedLog) last() (float32, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.speed, l.seen
}

// damageEvent is one SMSG_SPELLNONMELEEDAMAGELOG from the bot.
type damageEvent struct {
	target  uint64
	spellID uint32
	damage  uint32
}

// damageLog collects the bot's spell damage logs for the given spell IDs (all spells when empty).
type damageLog struct {
	mu     sync.Mutex
	events []damageEvent
}

func watchDamage(t *testing.T, bot *e2eharness.ScenarioBot, spellIDs ...uint32) *damageLog {
	t.Helper()
	log := &damageLog{}
	self := bot.World.CharGUID()
	want := map[uint32]bool{}
	for _, id := range spellIDs {
		want[id] = true
	}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellNonMeleeDamageLog {
			return
		}
		r := bytes.NewReader(data)
		target := readPackedGUID(r)
		if readPackedGUID(r) != self {
			return
		}
		var body struct {
			SpellID uint32
			Damage  uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		if len(want) > 0 && !want[body.SpellID] {
			return
		}
		log.mu.Lock()
		log.events = append(log.events, damageEvent{target, body.SpellID, body.Damage})
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *damageLog) reset() {
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
}

func (l *damageLog) snapshot() []damageEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]damageEvent(nil), l.events...)
}

// spellbookEvents records SMSG_LEARNED_SPELL and SMSG_REMOVED_SPELL for one spell.
type spellbookEvents struct {
	mu      sync.Mutex
	learned int
	removed int
}

func watchSpellbook(t *testing.T, bot *e2eharness.ScenarioBot, spellID uint32) *spellbookEvents {
	t.Helper()
	ev := &spellbookEvents{}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgLearnedSpell && opcode != client.SmsgRemovedSpell {
			return
		}
		if len(data) < 4 || binary.LittleEndian.Uint32(data) != spellID {
			return
		}
		ev.mu.Lock()
		if opcode == client.SmsgLearnedSpell {
			ev.learned++
		} else {
			ev.removed++
		}
		ev.mu.Unlock()
	})
	t.Cleanup(cancel)
	return ev
}

func (e *spellbookEvents) counts() (learned, removed int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.learned, e.removed
}

// castLanded casts until the spell succeeds (misses and "not ready" are retried).
func castLanded(t *testing.T, bot *e2eharness.ScenarioBot, spellID uint32, target uint64, attempts int) e2eharness.SpellCastResult {
	t.Helper()
	var res e2eharness.SpellCastResult
	for i := 0; i < attempts; i++ {
		var err error
		res, err = bot.TryCast(t, spellID, target, castTimeout)
		if err != nil {
			t.Fatalf("cast %d: %v", spellID, err)
		}
		if res.Success {
			return res
		}
		t.Logf("cast %d attempt %d refused: %s", spellID, i+1, e2eharness.SpellFailReasonName(res.FailReason))
		time.Sleep(gcd)
	}
	return res
}

func readPackedGUID(r *bytes.Reader) uint64 {
	mask, err := r.ReadByte()
	if err != nil {
		return 0
	}
	var guid uint64
	for i := uint8(0); i < 8; i++ {
		if mask&(1<<i) != 0 {
			b, err := r.ReadByte()
			if err != nil {
				return 0
			}
			guid |= uint64(b) << (i * 8)
		}
	}
	return guid
}
