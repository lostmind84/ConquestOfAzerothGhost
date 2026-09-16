//go:build e2e

// Package summonspets_test reproduces the "summons and pets" reports of the CoA server issue tracker.
// Each file covers one class and names the issue in its failure messages.
//
//	go test -tags=e2e ./e2e/coa/summonspets -count=1 -v -p 1
package summonspets_test

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
	classBarbarian    uint8 = 12
	classWitchHunter  uint8 = 15
	classKnightXoroth uint8 = 17
	classNecromancer  uint8 = 23
	classTinker       uint8 = 28

	smsgSpellNonMeleeDamageLog uint16 = 0x0250

	factionHostile = 14

	creatureMottledBoar uint32 = 3098 // beast that fights back

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

// spawnTarget spawns a creature, sets its level (when > 0) and faction (when > 0) and faces it.
// Training dummies are neutral to everyone, so pet assist code gated on hostility needs a faction.
func spawnTarget(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32, level, faction int) uint64 {
	t.Helper()
	guid := bot.Spawn(t, entry, 10*time.Second)
	_ = bot.World.SetTarget(guid)
	if level > 0 {
		bot.GM(t, fmt.Sprintf(".npc set level %d", level))
	}
	if faction > 0 {
		bot.GM(t, fmt.Sprintf(".npc set faction %d", faction))
	}
	bot.Face(t, guid)
	return guid
}

// knownRank returns the highest of ranks the bot knows, learning ranks[0] when it knows none.
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

// waitKnows waits until the bot knows a spell (talents learned with .localtalent).
func waitKnows(t *testing.T, bot *e2eharness.ScenarioBot, spellID uint32) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if bot.World.KnowsSpell(spellID) {
			time.Sleep(500 * time.Millisecond)
			return
		}
	}
	t.Fatalf("precondition: spell %d not learned", spellID)
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

// killUnit selects a unit and kills it with the GM .die command.
func killUnit(t *testing.T, bot *e2eharness.ScenarioBot, guid uint64) {
	t.Helper()
	_ = bot.World.SetTarget(guid)
	time.Sleep(200 * time.Millisecond)
	bot.GM(t, ".die")
}

// unitDead reports whether a unit is dead or no longer visible.
func unitDead(bot *e2eharness.ScenarioBot, guid uint64) bool {
	if bot.World.GetObject(guid) == nil {
		return true
	}
	hp, _ := bot.UnitHP(guid)
	return hp == 0
}

// ownedUnits returns the living units of the given entries whose summoner or creator is the bot.
func ownedUnits(bot *e2eharness.ScenarioBot, entries ...uint32) []uint64 {
	var out []uint64
	for _, u := range bot.UnitsByEntry(80, entries...) {
		o := bot.World.GetObject(u.GUID)
		if o == nil || u.Health == 0 {
			continue
		}
		if o.GUIDField(client.UnitFieldSummonedBy) == bot.GUID || o.GUIDField(client.UnitFieldCreatedBy) == bot.GUID {
			out = append(out, u.GUID)
		}
	}
	return out
}

// petSpellsEvent is one SMSG_PET_SPELLS: the pet GUID (0 clears the pet bar) and its spell action IDs.
type petSpellsEvent struct {
	pet     uint64
	react   uint8
	command uint8
	spells  []uint32
}

type petSpellsLog struct {
	mu     sync.Mutex
	events []petSpellsEvent
}

// watchPetSpells records SMSG_PET_SPELLS, the packet that builds the client pet action bar.
func watchPetSpells(t *testing.T, bot *e2eharness.ScenarioBot) *petSpellsLog {
	t.Helper()
	log := &petSpellsLog{}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgPetSpells || len(data) < 8 {
			return
		}
		ev := petSpellsEvent{pet: binary.LittleEndian.Uint64(data)}
		if len(data) >= 16 {
			ev.react, ev.command = data[14], data[15]
		}
		// guid(8) family(2) duration(4) react(1) command(1) unk(1) flags(1) actionbar(10*4) count(1) spells(count*4)
		const bar = 18
		for i := 0; i < 10 && ev.pet != 0 && len(data) >= bar+(i+1)*4; i++ {
			packed := binary.LittleEndian.Uint32(data[bar+i*4:])
			if id := packed & 0x00FFFFFF; id > 7 {
				ev.spells = append(ev.spells, id)
			}
		}
		log.mu.Lock()
		log.events = append(log.events, ev)
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *petSpellsLog) forPet(pet uint64) (petSpellsEvent, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ev := range l.events {
		if ev.pet == pet {
			return ev, true
		}
	}
	return petSpellsEvent{}, false
}

// damageEvent is one SMSG_SPELLNONMELEEDAMAGELOG.
type damageEvent struct {
	target   uint64
	attacker uint64
	spellID  uint32
	damage   uint32
}

type damageLog struct {
	mu     sync.Mutex
	events []damageEvent
}

// watchSpellDamage records every spell damage log the bot receives for the given spells (all when empty).
func watchSpellDamage(t *testing.T, bot *e2eharness.ScenarioBot, spellIDs ...uint32) *damageLog {
	t.Helper()
	log := &damageLog{}
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
		attacker := readPackedGUID(r)
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
		log.events = append(log.events, damageEvent{target, attacker, body.SpellID, body.Damage})
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *damageLog) snapshot() []damageEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]damageEvent(nil), l.events...)
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
