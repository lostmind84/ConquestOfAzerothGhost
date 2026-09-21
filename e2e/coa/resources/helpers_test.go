//go:build e2e

package resources_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWitchHunter   uint8 = 15
	classKnightXoroth  uint8 = 17
	classBloodmage     uint8 = 20

	// UNIT_FIELD_POWER1 + POWER_RAGE. Rage is the display power of both the Bloodmage and the
	// Knight of Xoroth (ChrClasses.dbc power type 1), so this is the bar the reports describe.
	unitFieldRage uint16 = client.UnitFieldPower1 + 1

	castTimeout = 5 * time.Second
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

// spawnTarget spawns a creature, lowers its level and faces it: a level 83 heroic training dummy
// avoids most attacks from a low-level bot.
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

// rage reads the player's Rage field straight from the tracked object.
func rage(bot *e2eharness.ScenarioBot) uint32 {
	self := bot.World.GetObject(bot.World.CharGUID())
	if self == nil {
		return 0
	}
	return self.Value(unitFieldRage)
}

// peakRage samples the Rage field over the window and returns its highest value. Rage decays by
// 2 per 2 s out of combat (Player::Regenerate, Rate.Rage.Loss = 1), so a single late read would
// under-report what a cast granted.
func peakRage(bot *e2eharness.ScenarioBot, window time.Duration) uint32 {
	highest := rage(bot)
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if current := rage(bot); current > highest {
			highest = current
		}
		time.Sleep(100 * time.Millisecond)
	}
	return highest
}

// gmChatLines runs a GM command and returns the system-chat text the server answered with.
//
// Passive auras flagged SPELL_ATTR0_DO_NOT_DISPLAY are never sent to the client (Aura::
// CanBeSentToClient), so a bot cannot see them in its own aura list. `.list auras id <id>` reads the
// server's applied-aura map instead and prints each effect's amount, which is the only channel this
// harness has for a hidden passive.
func gmChatLines(t *testing.T, bot *e2eharness.ScenarioBot, cmd string, window time.Duration) []string {
	t.Helper()
	var mu sync.Mutex
	var lines []string
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != client.SmsgMessageChat {
			return
		}
		mu.Lock()
		lines = append(lines, printableText(data))
		mu.Unlock()
	})
	defer cancel()
	bot.GM(t, cmd)
	time.Sleep(window)
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), lines...)
}

// printableText keeps the printable ASCII of a packet payload, so a chat line can be matched without
// parsing every field layout SMSG_MESSAGECHAT uses.
func printableText(data []byte) string {
	var b strings.Builder
	for _, c := range data {
		if c >= 0x20 && c < 0x7F {
			b.WriteByte(c)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
