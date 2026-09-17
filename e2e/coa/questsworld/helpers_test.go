//go:build e2e

// Package questsworld_test reproduces the "quests, world and creatures" reports of the CoA server issue tracker.
// Each test names its issue in its failure messages.
//
//	go test -tags=e2e ./e2e/coa/questsworld -count=1 -v -p 1
package questsworld_test

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
	settle = time.Second

	smsgGossipMessage uint16 = 0x017D
)

// newBot creates one bot of the given race, class and level on the package pad.
func newBot(t *testing.T, prefix string, race, class uint8, level int) *e2eharness.ScenarioBot {
	t.Helper()
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: prefix, Race: race, Class: class, Level: level})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	return bot
}

// unitLevel reads UNIT_FIELD_LEVEL of a visible unit (0 when not visible).
func unitLevel(bot *e2eharness.ScenarioBot, guid uint64) uint32 {
	o := bot.World.GetObject(guid)
	if o == nil {
		return 0
	}
	return o.Value(client.UnitFieldLevel)
}

// waitUnitLevel polls a unit's level until cond holds or the timeout ends, and returns the last level seen.
func waitUnitLevel(bot *e2eharness.ScenarioBot, guid uint64, timeout time.Duration, cond func(uint32) bool) uint32 {
	var level uint32
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if level = unitLevel(bot, guid); cond(level) {
			return level
		}
	}
	return level
}

// giverQuest is one entry of SMSG_QUESTGIVER_QUEST_LIST, or the quest of SMSG_QUESTGIVER_QUEST_DETAILS.
type giverQuest struct {
	id    uint32
	icon  uint32
	level int32
	title string
}

func cString(r *bytes.Reader) string {
	var b []byte
	for {
		c, err := r.ReadByte()
		if err != nil || c == 0 {
			return string(b)
		}
		b = append(b, c)
	}
}

// questgiverQuests sends CMSG_QUESTGIVER_HELLO and returns the quests of the answer: a quest list, a gossip menu
// or the details of a single quest.
func questgiverQuests(t *testing.T, bot *e2eharness.ScenarioBot, giver uint64) []giverQuest {
	t.Helper()
	var (
		mu     sync.Mutex
		quests []giverQuest
		done   = make(chan struct{}, 1)
	)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		r := bytes.NewReader(data)
		var guid uint64
		switch opcode {
		case client.SmsgQuestgiverQuestList:
			_ = binary.Read(r, binary.LittleEndian, &guid)
			cString(r)
			var delay, emote uint32
			var count uint8
			_ = binary.Read(r, binary.LittleEndian, &delay)
			_ = binary.Read(r, binary.LittleEndian, &emote)
			_ = binary.Read(r, binary.LittleEndian, &count)
			mu.Lock()
			for i := uint8(0); i < count; i++ {
				var q giverQuest
				var flags uint32
				var repeatable uint8
				_ = binary.Read(r, binary.LittleEndian, &q.id)
				_ = binary.Read(r, binary.LittleEndian, &q.icon)
				_ = binary.Read(r, binary.LittleEndian, &q.level)
				_ = binary.Read(r, binary.LittleEndian, &flags)
				_ = binary.Read(r, binary.LittleEndian, &repeatable)
				q.title = cString(r)
				quests = append(quests, q)
			}
			mu.Unlock()
		case smsgGossipMessage:
			var menu, text, options uint32
			_ = binary.Read(r, binary.LittleEndian, &guid)
			_ = binary.Read(r, binary.LittleEndian, &menu)
			_ = binary.Read(r, binary.LittleEndian, &text)
			_ = binary.Read(r, binary.LittleEndian, &options)
			for i := uint32(0); i < options; i++ {
				var index, money uint32
				var icon, coded uint8
				_ = binary.Read(r, binary.LittleEndian, &index)
				_ = binary.Read(r, binary.LittleEndian, &icon)
				_ = binary.Read(r, binary.LittleEndian, &coded)
				_ = binary.Read(r, binary.LittleEndian, &money)
				cString(r)
				cString(r)
			}
			var count uint32
			_ = binary.Read(r, binary.LittleEndian, &count)
			mu.Lock()
			for i := uint32(0); i < count; i++ {
				var q giverQuest
				var flags uint32
				var repeatable uint8
				_ = binary.Read(r, binary.LittleEndian, &q.id)
				_ = binary.Read(r, binary.LittleEndian, &q.icon)
				_ = binary.Read(r, binary.LittleEndian, &q.level)
				_ = binary.Read(r, binary.LittleEndian, &flags)
				_ = binary.Read(r, binary.LittleEndian, &repeatable)
				q.title = cString(r)
				quests = append(quests, q)
			}
			mu.Unlock()
		case client.SmsgQuestgiverQuestDetails:
			var informer uint64
			var q giverQuest
			_ = binary.Read(r, binary.LittleEndian, &guid)
			_ = binary.Read(r, binary.LittleEndian, &informer)
			_ = binary.Read(r, binary.LittleEndian, &q.id)
			q.title = cString(r)
			q.level = -1
			mu.Lock()
			quests = append(quests, q)
			mu.Unlock()
		default:
			if opcode >= 0x017D && opcode <= 0x01A0 {
				t.Logf("questgiver answer opcode 0x%04X (%d bytes)", opcode, len(data))
			}
			return
		}
		select {
		case done <- struct{}{}:
		default:
		}
	})
	defer cancel()
	if err := bot.World.QuestgiverHello(giver); err != nil {
		t.Fatalf("questgiver hello: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	return append([]giverQuest(nil), quests...)
}

func hasQuest(quests []giverQuest, id uint32) bool {
	for _, q := range quests {
		if q.id == id {
			return true
		}
	}
	return false
}

func describeQuests(quests []giverQuest) string {
	s := ""
	for _, q := range quests {
		s += fmt.Sprintf(" [%d %q level %d icon %d]", q.id, q.title, q.level, q.icon)
	}
	return s
}

// gmOutput runs a GM command and returns the printable text of the chat messages received for a moment after it.
func gmOutput(t *testing.T, bot *e2eharness.ScenarioBot, cmd string) string {
	t.Helper()
	var (
		mu  sync.Mutex
		out []string
	)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgMessageChat {
			return
		}
		var b []byte
		for _, c := range data {
			if c >= 0x20 && c < 0x7F {
				b = append(b, c)
			} else if len(b) > 0 && b[len(b)-1] != ' ' {
				b = append(b, ' ')
			}
		}
		mu.Lock()
		out = append(out, string(b))
		mu.Unlock()
	})
	defer cancel()
	bot.GM(t, cmd)
	time.Sleep(1500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	return fmt.Sprint(out)
}
