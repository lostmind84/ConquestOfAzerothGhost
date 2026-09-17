//go:build e2e

// Package talents_test reproduces the "talents and passives with no effect" reports of the CoA server issue
// tracker. Each file covers one issue (or a group of related reports) and names it in its failure messages.
//
//	go test -tags=e2e ./e2e/coa/talents -count=1 -v -p 1
package talents_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWitchDoctor  uint8 = 13
	classWitchHunter  uint8 = 15
	classStormbringer uint8 = 16
	classGuardian     uint8 = 18
	classTemplar      uint8 = 19
	classBloodmage    uint8 = 20
	classChronomancer uint8 = 22
	classNecromancer  uint8 = 23
	classStarcaller   uint8 = 26
	classSunCleric    uint8 = 27
	classVenomancer   uint8 = 29
	classReaper       uint8 = 30
	classPrimalist    uint8 = 31

	smsgSpellNonMeleeDamageLog uint16 = 0x0250
	smsgSpellHealLog           uint16 = 0x0150
	smsgAttackerStateUpdate    uint16 = 0x014A

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

// spawnTarget spawns a creature, sets its level and faces it.
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

// selfValue reads one of the bot's own update fields (0 when the object is not tracked yet).
func selfValue(bot *e2eharness.ScenarioBot, field uint16) uint32 {
	if obj := bot.World.GetObject(bot.World.CharGUID()); obj != nil {
		return obj.Value(field)
	}
	return 0
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

// waitSpell waits until the bot knows spellID.
func waitSpell(bot *e2eharness.ScenarioBot, spellID uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bot.World.KnowsSpell(spellID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return bot.World.KnowsSpell(spellID)
}

// waitAura waits until the bot has the aura, without failing the test.
func waitAura(bot *e2eharness.ScenarioBot, spellID uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bot.HasAura(spellID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return bot.HasAura(spellID)
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

// spellEvent is one SMSG_SPELLNONMELEEDAMAGELOG or SMSG_SPELLHEALLOG sent by the bot.
type spellEvent struct {
	target  uint64
	spellID uint32
	amount  uint32
}

// spellLog collects the bot's spell damage or heal logs for the given spell IDs (all spells when empty).
type spellLog struct {
	mu     sync.Mutex
	events []spellEvent
}

func watchSpellLog(t *testing.T, bot *e2eharness.ScenarioBot, opcode uint16, spellIDs ...uint32) *spellLog {
	t.Helper()
	log := &spellLog{}
	self := bot.World.CharGUID()
	want := map[uint32]bool{}
	for _, id := range spellIDs {
		want[id] = true
	}
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != opcode {
			return
		}
		r := bytes.NewReader(data)
		target := readPackedGUID(r)
		if readPackedGUID(r) != self {
			return
		}
		var body struct {
			SpellID uint32
			Amount  uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		if len(want) > 0 && !want[body.SpellID] {
			return
		}
		log.mu.Lock()
		log.events = append(log.events, spellEvent{target, body.SpellID, body.Amount})
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *spellLog) reset() {
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
}

func (l *spellLog) snapshot() []spellEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]spellEvent(nil), l.events...)
}

// swingCounter counts the bot's melee swings from SMSG_ATTACKERSTATEUPDATE.
type swingCounter struct {
	mu    sync.Mutex
	count int
}

func watchSwings(t *testing.T, bot *e2eharness.ScenarioBot) *swingCounter {
	t.Helper()
	c := &swingCounter{}
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgAttackerStateUpdate || len(data) < 5 {
			return
		}
		r := bytes.NewReader(data[4:]) // hit info
		if readPackedGUID(r) != self {
			return
		}
		c.mu.Lock()
		c.count++
		c.mu.Unlock()
	})
	t.Cleanup(cancel)
	return c
}

func (c *swingCounter) swings() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
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
