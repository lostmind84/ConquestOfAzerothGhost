//go:build e2e

// Issue #3971 ("CoA talent authority", branch fix/coa-talent-authority): the server enforces the
// CharacterAdvancement point budget for `.localtalent` / the client's known-entries upload, instead of
// trusting whatever rank the client asks for, and keeps the client's local talent UI in sync with
// SMSG/CMSG_CHARACTER_ADVANCEMENT_* packets. See modules/mod-ascension-compat/src/AscensionCompat.cpp
// (SetTalentRank, ResetPaidTalents, ApplyKnownEntriesUpload, SendCharacterAdvancementState,
// OnPlayerActiveMover, HandleLocalTalentCommand, HandleLocalTalentResetCommand) and
// AscensionCoATalentState.cpp (KnownEntriesPayload / ParseKnownEntriesUpload wire format).
//
//	go test -tags=e2e ./e2e/coa/talents -run 'TestTalentStateSurvivesRelog|TestTalentBudgetIsEnforced|TestTalentResetRemovesPaidRanks|TestKnownEntriesAreSent|TestKnownEntriesUploadIsApplied' -count=1 -v
package talents_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Reaper (class 30) CoA talent catalog rows used below, read from the DBC-derived catalog
// (`python3 modules/mod-ascension-compat/tests/coa_talent_catalog.py /srv/coa/server-data/dbc` on the
// azerothcore-wotlk-coa-talent worktree). All are single-rank (SpellCount 1) with RequiredLevel 0, so one
// rank costs exactly one point (AECost/TECost 1) and the only gate below level 60 is the point budget.
const (
	// Class tree (SpecId 0, AE cost 1).
	entryReaperClassA uint32 = 5464
	spellReaperClassA uint32 = 805185
	entryReaperClassB uint32 = 5495
	spellReaperClassB uint32 = 572340
	entryReaperClassC uint32 = 5600
	spellReaperClassC uint32 = 800845
	// entryReaperClassD is a fourth, otherwise-unused class-tree entry so TestKnownEntriesAreSent does not
	// share talent state assumptions with the budget/reset tests above.
	entryReaperClassD uint32 = 6708
	spellReaperClassD uint32 = 706790

	// Specialization 56 tree (TE cost 1). entryReaperSpecA/spellReaperSpecA is the same row
	// TestTalentReplacements (replacement_test.go, "ShudderScythe") exercises, independently confirming the
	// entry/spell pairing.
	specReaper       uint32 = 56
	entryReaperSpecA uint32 = 5562
	spellReaperSpecA uint32 = 805708
	entryReaperSpecB uint32 = 5762
	spellReaperSpecB uint32 = 707707

	// Automatic (AECost/TECost 0) entry granted once level >= 10 with specialization 56 active. It is not a
	// key in the catalog's CoAAutomaticDependencies table, so it has no prerequisite: used both as the
	// SetSpecialization sync anchor and as the automatic entry TestTalentResetRemovesPaidRanks checks
	// `.localtalent reset` leaves alone.
	entryReaperAutoSpec56 uint32 = 4055
	spellReaperAutoSpec56 uint32 = 92145
)

// Opcodes added by issue #3971 (AscensionCompat.cpp); not yet in client/opcodes.go.
const (
	smsgCharacterAdvancementActiveSpec   uint16 = 0x0725
	smsgCharacterAdvancementKnownEntries uint16 = 0x0726
	cmsgCharacterAdvancementKnownEntries uint16 = 0x0727
)

// knownEntryRecordSize is one SMSG/CMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES record: u32 EntryId, u32 Rank,
// u32, u8, u32, u32 (AscensionCoATalentState::KnownEntriesPayload / ParseKnownEntriesUpload).
const knownEntryRecordSize = 21

// advancementPacket is one SMSG_CHARACTER_ADVANCEMENT_* packet captured by watchAdvancement, in arrival order.
type advancementPacket struct {
	opcode uint16
	data   []byte
}

// advancementWatcher captures SMSG_CHARACTER_ADVANCEMENT_ACTIVE_SPEC (0x0725) and
// SMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES (0x0726) packets in arrival order.
type advancementWatcher struct {
	mu      sync.Mutex
	packets []advancementPacket
}

// watchAdvancement registers the packet hook. Register it before any action that may trigger these packets
// (ActivateCharacterAdvancement, .localtalent, .localspec, the 0x0727 upload) — a hook added afterward can
// miss the reply.
func watchAdvancement(bot *e2eharness.ScenarioBot) (*advancementWatcher, func()) {
	w := &advancementWatcher{}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgCharacterAdvancementActiveSpec && opcode != smsgCharacterAdvancementKnownEntries {
			return
		}
		cp := append([]byte(nil), data...)
		w.mu.Lock()
		w.packets = append(w.packets, advancementPacket{opcode, cp})
		w.mu.Unlock()
	})
	return w, cancel
}

func (w *advancementWatcher) snapshot() []advancementPacket {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]advancementPacket(nil), w.packets...)
}

// waitKnownEntries waits until at least `count` SMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES packets have
// arrived and returns the parsed entryId -> rank set of the most recent one.
func (w *advancementWatcher) waitKnownEntries(t *testing.T, count int, timeout time.Duration) map[uint32]uint32 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var last []byte
		n := 0
		for _, p := range w.snapshot() {
			if p.opcode == smsgCharacterAdvancementKnownEntries {
				n++
				last = p.data
			}
		}
		if n >= count {
			return parseKnownEntries(t, last)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d SMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES packet(s), got %d", count, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// parseKnownEntries decodes an SMSG/CMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES body (u32 count, then `count`
// 21-byte records) into entryId -> rank.
func parseKnownEntries(t *testing.T, data []byte) map[uint32]uint32 {
	t.Helper()
	if len(data) < 4 {
		t.Fatalf("known-entries payload too short: %d byte(s)", len(data))
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	want := 4 + int(count)*knownEntryRecordSize
	if len(data) != want {
		t.Fatalf("known-entries payload is %d byte(s), want %d for count %d", len(data), want, count)
	}
	out := make(map[uint32]uint32, count)
	for i := uint32(0); i < count; i++ {
		off := 4 + int(i)*knownEntryRecordSize
		out[binary.LittleEndian.Uint32(data[off:off+4])] = binary.LittleEndian.Uint32(data[off+4 : off+8])
	}
	return out
}

// buildKnownEntriesUpload encodes a CMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES body: the client's complete
// wanted set (ranks keyed by entry id), using the same 21-byte record layout as the SMSG the server sends.
func buildKnownEntriesUpload(ranks map[uint32]uint32) []byte {
	ids := make([]uint32, 0, len(ranks))
	for id := range ranks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(ids)))
	for _, id := range ids {
		_ = binary.Write(buf, binary.LittleEndian, id)
		_ = binary.Write(buf, binary.LittleEndian, ranks[id])
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
		buf.WriteByte(0)
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	}
	return buf.Bytes()
}

// waitSpellGone waits until the bot no longer knows spellID (mirrors waitSpell in helpers_test.go).
func waitSpellGone(bot *e2eharness.ScenarioBot, spellID uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !bot.World.KnowsSpell(spellID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !bot.World.KnowsSpell(spellID)
}

// captureChatDuring runs action and returns the printable text of SMSG_MESSAGECHAT payloads received for a
// moment afterward. The packet layout is msgType-dependent (see client.WorldClient.handleChatMessage), so —
// like e2e/coa/questsworld/helpers_test.go's gmOutput — this scrapes printable ASCII instead of decoding the
// struct, which is enough to check for a substring the server is known to send.
func captureChatDuring(t *testing.T, bot *e2eharness.ScenarioBot, action func()) string {
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
	action()
	time.Sleep(1500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	return fmt.Sprint(out)
}

// gmChatOutput runs a GM command and returns its chat output (see captureChatDuring).
func gmChatOutput(t *testing.T, bot *e2eharness.ScenarioBot, cmd string) string {
	t.Helper()
	return captureChatDuring(t, bot, func() { bot.GM(t, cmd) })
}

// TestTalentStateSurvivesRelog covers issue #3971: 2 class-tree ranks, 1 specialization-tree rank and the
// active local specialization must all still be there after a full logout/login.
func TestTalentStateSurvivesRelog(t *testing.T) {
	bot := newBot(t, "TAut1", e2eharness.RaceOrc, classReaper, 20)

	bot.SetSpecialization(t, specReaper, spellReaperAutoSpec56)
	bot.SetTalentRank(t, entryReaperClassA, 1)
	if !waitSpell(bot, spellReaperClassA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassA, spellReaperClassA)
	}
	bot.SetTalentRank(t, entryReaperClassB, 1)
	if !waitSpell(bot, spellReaperClassB, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassB, spellReaperClassB)
	}
	bot.SetTalentRank(t, entryReaperSpecA, 1)
	if !waitSpell(bot, spellReaperSpecA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperSpecA, spellReaperSpecA)
	}

	bot.Relog(t)

	gotA := bot.World.KnowsSpell(spellReaperClassA)
	gotB := bot.World.KnowsSpell(spellReaperClassB)
	gotSpec := bot.World.KnowsSpell(spellReaperSpecA)
	if !gotA || !gotB || !gotSpec {
		t.Errorf("E2E_FAIL: talent spells not all known after relog (class %d known=%v, class %d known=%v, spec %d known=%v) (#3971)",
			spellReaperClassA, gotA, spellReaperClassB, gotB, spellReaperSpecA, gotSpec)
	}

	var data string
	if err := bot.CharDB.QueryRow(
		"SELECT data FROM character_settings WHERE guid = ? AND source = ?", bot.GUID, "core.ascension_active_spec",
	).Scan(&data); err != nil {
		t.Fatalf("character_settings query after relog: %v", err)
	}
	want := fmt.Sprint(specReaper)
	got := strings.TrimSpace(data)
	if got != want {
		t.Errorf("E2E_FAIL: character_settings core.ascension_active_spec = %q after relog, want %q (#3971)", got, want)
	}

	if gotA && gotB && gotSpec && got == want {
		t.Logf("E2E_PASS: 2 class-tree + 1 specialization-tree talent rank and specialization %s survived the relog", want)
	}
}

// TestTalentBudgetIsEnforced covers issue #3971: at level 12 (CoA talent budget 2 class / 1 specialization
// point — see modules/mod-ascension-compat/tests/talent_state/run.py, "one class point at 10, one
// specialization point at 11, alternating"), a rank above what the tree's point budget allows is refused
// with a chat reply naming the shortfall; lowering another rank to 0 (always allowed) frees a point and lets
// the previously-refused rank through.
func TestTalentBudgetIsEnforced(t *testing.T) {
	bot := newBot(t, "TAut2", e2eharness.RaceOrc, classReaper, 12)

	// Class tree: budget 2. The first two single-rank entries succeed, the third is refused.
	bot.SetTalentRank(t, entryReaperClassA, 1)
	if !waitSpell(bot, spellReaperClassA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassA, spellReaperClassA)
	}
	bot.SetTalentRank(t, entryReaperClassB, 1)
	if !waitSpell(bot, spellReaperClassB, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassB, spellReaperClassB)
	}

	txt := gmChatOutput(t, bot, fmt.Sprintf(".localtalent %d 1", entryReaperClassC))
	if waitSpell(bot, spellReaperClassC, 3*time.Second) {
		t.Errorf("E2E_FAIL: class-tree talent entry %d granted over the level 12 budget of 2 (#3971)", entryReaperClassC)
	} else if !strings.Contains(txt, "needs") || !strings.Contains(txt, "point(s), but") {
		t.Errorf("E2E_FAIL: over-budget class talent reply %q missing \"needs\" / \"point(s), but\" (#3971)", txt)
	} else {
		t.Logf("E2E_PASS: over-budget class talent entry %d refused: %q", entryReaperClassC, txt)
	}

	bot.SetTalentRank(t, entryReaperClassA, 0)
	if !waitSpellGone(bot, spellReaperClassA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 0 did not remove spell %d", entryReaperClassA, spellReaperClassA)
	}
	bot.SetTalentRank(t, entryReaperClassC, 1)
	if !waitSpell(bot, spellReaperClassC, 5*time.Second) {
		t.Errorf("E2E_FAIL: class-tree talent entry %d still refused after freeing a point (#3971)", entryReaperClassC)
	} else {
		t.Logf("E2E_PASS: class-tree talent entry %d granted after freeing a point", entryReaperClassC)
	}

	// Specialization 56 tree: budget 1. Same shape as the class tree above.
	bot.SetSpecialization(t, specReaper, spellReaperAutoSpec56)
	bot.SetTalentRank(t, entryReaperSpecA, 1)
	if !waitSpell(bot, spellReaperSpecA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperSpecA, spellReaperSpecA)
	}

	txt = gmChatOutput(t, bot, fmt.Sprintf(".localtalent %d 1", entryReaperSpecB))
	if waitSpell(bot, spellReaperSpecB, 3*time.Second) {
		t.Errorf("E2E_FAIL: specialization-tree talent entry %d granted over the level 12 budget of 1 (#3971)", entryReaperSpecB)
	} else if !strings.Contains(txt, "needs") || !strings.Contains(txt, "point(s), but") {
		t.Errorf("E2E_FAIL: over-budget specialization talent reply %q missing \"needs\" / \"point(s), but\" (#3971)", txt)
	} else {
		t.Logf("E2E_PASS: over-budget specialization talent entry %d refused: %q", entryReaperSpecB, txt)
	}

	bot.SetTalentRank(t, entryReaperSpecA, 0)
	if !waitSpellGone(bot, spellReaperSpecA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 0 did not remove spell %d", entryReaperSpecA, spellReaperSpecA)
	}
	bot.SetTalentRank(t, entryReaperSpecB, 1)
	if !waitSpell(bot, spellReaperSpecB, 5*time.Second) {
		t.Errorf("E2E_FAIL: specialization-tree talent entry %d still refused after freeing a point (#3971)", entryReaperSpecB)
	} else {
		t.Logf("E2E_PASS: specialization-tree talent entry %d granted after freeing a point", entryReaperSpecB)
	}
}

// TestTalentResetRemovesPaidRanks covers issue #3971: `.localtalent reset` removes every paid rank (class
// and specialization trees alike) but leaves automatic (cost 0) entries alone.
func TestTalentResetRemovesPaidRanks(t *testing.T) {
	bot := newBot(t, "TAut3", e2eharness.RaceOrc, classReaper, 20)

	bot.SetSpecialization(t, specReaper, spellReaperAutoSpec56)
	if !bot.World.KnowsSpell(spellReaperAutoSpec56) {
		t.Fatalf("precondition: automatic entry %d spell %d not granted by specialization %d",
			entryReaperAutoSpec56, spellReaperAutoSpec56, specReaper)
	}
	bot.SetTalentRank(t, entryReaperClassA, 1)
	if !waitSpell(bot, spellReaperClassA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassA, spellReaperClassA)
	}
	bot.SetTalentRank(t, entryReaperClassB, 1)
	if !waitSpell(bot, spellReaperClassB, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassB, spellReaperClassB)
	}
	bot.SetTalentRank(t, entryReaperSpecA, 1)
	if !waitSpell(bot, spellReaperSpecA, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperSpecA, spellReaperSpecA)
	}

	txt := gmChatOutput(t, bot, ".localtalent reset")

	goneA := waitSpellGone(bot, spellReaperClassA, 5*time.Second)
	goneB := waitSpellGone(bot, spellReaperClassB, 5*time.Second)
	goneSpec := waitSpellGone(bot, spellReaperSpecA, 5*time.Second)
	if !goneA || !goneB || !goneSpec {
		t.Errorf("E2E_FAIL: .localtalent reset left a paid rank behind (class %d gone=%v, class %d gone=%v, spec %d gone=%v) (#3971)",
			spellReaperClassA, goneA, spellReaperClassB, goneB, spellReaperSpecA, goneSpec)
	}
	if !strings.Contains(txt, "Reset 3 CoA talent rank") {
		t.Errorf("E2E_FAIL: .localtalent reset reply %q does not report 3 removed rank(s) (#3971)", txt)
	}
	if !bot.World.KnowsSpell(spellReaperAutoSpec56) {
		t.Errorf("E2E_FAIL: .localtalent reset removed automatic entry %d spell %d (#3971)",
			entryReaperAutoSpec56, spellReaperAutoSpec56)
	}
	if goneA && goneB && goneSpec && bot.World.KnowsSpell(spellReaperAutoSpec56) {
		t.Logf("E2E_PASS: reset removed the 3 paid ranks and kept automatic entry %d: %q", entryReaperAutoSpec56, txt)
	}
}

// TestKnownEntriesAreSent covers issue #3971: the first CMSG_SET_ACTIVE_MOVER makes the server send
// SMSG_CHARACTER_ADVANCEMENT_ACTIVE_SPEC (0x0725) then SMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES (0x0726);
// `.localtalent` afterward resends a fresh 0x0726 reflecting the change. The e2eharness login path does not
// send CMSG_SET_ACTIVE_MOVER itself (see ActivateCharacterAdvancement), so the packet hook is registered
// right after login and the mover is activated explicitly.
func TestKnownEntriesAreSent(t *testing.T) {
	bot := newBot(t, "TAut4", e2eharness.RaceOrc, classReaper, 20)

	watcher, cancel := watchAdvancement(bot)
	defer cancel()
	bot.ActivateCharacterAdvancement(t)

	known := watcher.waitKnownEntries(t, 1, 5*time.Second)
	pkts := watcher.snapshot()
	if len(pkts) < 2 || pkts[0].opcode != smsgCharacterAdvancementActiveSpec ||
		pkts[1].opcode != smsgCharacterAdvancementKnownEntries {
		t.Fatalf("precondition: expected 0x0725 then 0x0726 after ActivateCharacterAdvancement, got %+v", pkts)
	}
	if len(pkts[0].data) != 8 ||
		binary.LittleEndian.Uint32(pkts[0].data[0:4]) != 0 || binary.LittleEndian.Uint32(pkts[0].data[4:8]) != 1 {
		t.Errorf("E2E_FAIL: SMSG_CHARACTER_ADVANCEMENT_ACTIVE_SPEC body % x, want u32(0) u32(1) (#3971)", pkts[0].data)
	}
	if len(known) != 0 {
		t.Errorf("E2E_FAIL: initial known-entries set is %v, want empty before any talent or specialization (#3971)", known)
	}

	bot.SetTalentRank(t, entryReaperClassD, 1)
	if !waitSpell(bot, spellReaperClassD, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 1 did not grant spell %d", entryReaperClassD, spellReaperClassD)
	}
	known = watcher.waitKnownEntries(t, 2, 5*time.Second)
	if rank, ok := known[entryReaperClassD]; !ok || rank != 1 {
		t.Errorf("E2E_FAIL: known-entries after rank 1 = %v, want entry %d at rank 1 (#3971)", known, entryReaperClassD)
	}

	bot.SetTalentRank(t, entryReaperClassD, 0)
	if !waitSpellGone(bot, spellReaperClassD, 5*time.Second) {
		t.Fatalf("precondition: talent entry %d rank 0 did not remove spell %d", entryReaperClassD, spellReaperClassD)
	}
	known = watcher.waitKnownEntries(t, 3, 5*time.Second)
	if _, ok := known[entryReaperClassD]; ok {
		t.Errorf("E2E_FAIL: known-entries after rank 0 still has entry %d: %v (#3971)", entryReaperClassD, known)
	} else {
		t.Logf("E2E_PASS: 0x0725/0x0726 sent on activation, entry %d appeared at rank 1 and disappeared at rank 0",
			entryReaperClassD)
	}
}

// TestKnownEntriesUploadIsApplied covers issue #3971: CMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES (0x0727)
// carries the client's complete wanted set; the server diffs it against the spellbook (removals first),
// refuses the whole upload over budget, and always answers with a fresh 0x0726 reflecting its own state.
func TestKnownEntriesUploadIsApplied(t *testing.T) {
	bot := newBot(t, "TAut5", e2eharness.RaceOrc, classReaper, 12) // budget: 2 class points

	watcher, cancel := watchAdvancement(bot)
	defer cancel()
	bot.ActivateCharacterAdvancement(t)
	if known := watcher.waitKnownEntries(t, 1, 5*time.Second); len(known) != 0 {
		t.Fatalf("precondition: initial known-entries set is %v, want empty", known)
	}

	// 2 valid paid class entries at rank 1: within the level-12 budget of 2, both should be learned.
	if err := bot.World.SendPacketRaw(cmsgCharacterAdvancementKnownEntries, buildKnownEntriesUpload(map[uint32]uint32{
		entryReaperClassA: 1, entryReaperClassB: 1,
	})); err != nil {
		t.Fatalf("send known-entries upload: %v", err)
	}
	if !waitSpell(bot, spellReaperClassA, 5*time.Second) || !waitSpell(bot, spellReaperClassB, 5*time.Second) {
		t.Fatalf("precondition: 2-entry known-entries upload did not grant spells %d and %d",
			spellReaperClassA, spellReaperClassB)
	}
	known := watcher.waitKnownEntries(t, 2, 5*time.Second)
	if known[entryReaperClassA] != 1 || known[entryReaperClassB] != 1 || len(known) != 2 {
		t.Errorf("E2E_FAIL: known-entries after 2-entry upload = %v, want {%d:1, %d:1} (#3971)",
			known, entryReaperClassA, entryReaperClassB)
	} else {
		t.Logf("E2E_PASS: known-entries upload granted entries %d and %d", entryReaperClassA, entryReaperClassB)
	}

	// Re-upload with only one of the two: the omitted entry must be removed.
	if err := bot.World.SendPacketRaw(cmsgCharacterAdvancementKnownEntries, buildKnownEntriesUpload(map[uint32]uint32{
		entryReaperClassA: 1,
	})); err != nil {
		t.Fatalf("send known-entries upload: %v", err)
	}
	if !waitSpellGone(bot, spellReaperClassB, 5*time.Second) {
		t.Errorf("E2E_FAIL: known-entries upload omitting entry %d did not remove spell %d (#3971)",
			entryReaperClassB, spellReaperClassB)
	}
	if !bot.World.KnowsSpell(spellReaperClassA) {
		t.Errorf("E2E_FAIL: known-entries re-upload also removed entry %d (#3971)", entryReaperClassA)
	}
	known = watcher.waitKnownEntries(t, 3, 5*time.Second)
	if known[entryReaperClassA] != 1 || len(known) != 1 {
		t.Errorf("E2E_FAIL: known-entries after 1-entry upload = %v, want {%d:1} (#3971)", known, entryReaperClassA)
	} else {
		t.Logf("E2E_PASS: known-entries upload removed the omitted entry %d, kept %d", entryReaperClassB, entryReaperClassA)
	}

	// Over-budget upload (3 entries, cost 3 > budget 2): refused whole, nothing changes.
	txt := captureChatDuring(t, bot, func() {
		if err := bot.World.SendPacketRaw(cmsgCharacterAdvancementKnownEntries, buildKnownEntriesUpload(map[uint32]uint32{
			entryReaperClassA: 1, entryReaperClassB: 1, entryReaperClassC: 1,
		})); err != nil {
			t.Fatalf("send known-entries upload: %v", err)
		}
	})
	if !strings.Contains(txt, "That build spends") {
		t.Errorf("E2E_FAIL: over-budget known-entries upload reply %q missing \"That build spends\" (#3971)", txt)
	}
	if waitSpell(bot, spellReaperClassB, 3*time.Second) || waitSpell(bot, spellReaperClassC, 3*time.Second) {
		t.Errorf("E2E_FAIL: over-budget known-entries upload granted a spell anyway (#3971)")
	}
	known = watcher.waitKnownEntries(t, 4, 5*time.Second)
	if known[entryReaperClassA] != 1 || len(known) != 1 {
		t.Errorf("E2E_FAIL: known-entries after refused over-budget upload = %v, want unchanged {%d:1} (#3971)",
			known, entryReaperClassA)
	} else {
		t.Logf("E2E_PASS: over-budget known-entries upload refused (%q), state unchanged", txt)
	}
}

// Reaper specialization 55 and 57 rows (TE cost 1, RequiredLevel 0), for the specialization-selection test.
const (
	entryReaperSpec55A uint32 = 5182
	spellReaperSpec55A uint32 = 572213
	entryReaperSpec57A uint32 = 4111
	spellReaperSpec57A uint32 = 707909
)

// TestKnownEntriesUploadSelectsOneSpecialization covers issue #3971: a character without an active
// specialization can select one through the known-entries upload, as the first `.localtalent` does, but a set
// spanning two specialization trees is refused as a whole (the server would otherwise switch and persist
// the first specialization, then fail on the second). buildKnownEntriesUpload sorts records by entry id, so
// the specialization-57 entry (4111) precedes the specialization-55 entry (5182): a leaked switch would be to
// 57, and the specialization-55 upload that follows would then be refused.
func TestKnownEntriesUploadSelectsOneSpecialization(t *testing.T) {
	bot := newBot(t, "TAut6", e2eharness.RaceOrc, classReaper, 12)
	watcher, cancel := watchAdvancement(bot)
	defer cancel()
	bot.ActivateCharacterAdvancement(t)
	watcher.waitKnownEntries(t, 1, 5*time.Second)

	txt := captureChatDuring(t, bot, func() {
		if err := bot.World.SendPacketRaw(cmsgCharacterAdvancementKnownEntries, buildKnownEntriesUpload(map[uint32]uint32{
			entryReaperSpec55A: 1, entryReaperSpec57A: 1,
		})); err != nil {
			t.Fatalf("send known-entries upload: %v", err)
		}
	})
	if !strings.Contains(txt, "mixes specializations") {
		t.Errorf("E2E_FAIL: two-specialization upload reply %q missing \"mixes specializations\" (#3971)", txt)
	}
	if waitSpell(bot, spellReaperSpec55A, 3*time.Second) || waitSpell(bot, spellReaperSpec57A, 3*time.Second) {
		t.Errorf("E2E_FAIL: two-specialization upload granted a spell anyway (#3971)")
	}

	// One specialization tree only: the upload selects it and the rank lands.
	if err := bot.World.SendPacketRaw(cmsgCharacterAdvancementKnownEntries, buildKnownEntriesUpload(map[uint32]uint32{
		entryReaperSpec55A: 1,
	})); err != nil {
		t.Fatalf("send known-entries upload: %v", err)
	}
	if !waitSpell(bot, spellReaperSpec55A, 5*time.Second) {
		t.Fatalf("E2E_FAIL: single-specialization upload did not grant spell %d (#3971)", spellReaperSpec55A)
	}
	known := watcher.waitKnownEntries(t, 3, 5*time.Second)
	if known[entryReaperSpec55A] != 1 {
		t.Errorf("E2E_FAIL: known-entries after the specialization-selecting upload = %v, want entry %d at rank 1 (#3971)",
			known, entryReaperSpec55A)
	}
	// The server now holds specialization 55: a specialization-57 rank is refused by name.
	reply := gmChatOutput(t, bot, fmt.Sprintf(".localtalent %d 1", entryReaperSpec57A))
	if !strings.Contains(reply, "active local specialization is 55") {
		t.Errorf("E2E_FAIL: after the specialization-55 upload, .localtalent %d reply %q does not name specialization 55 (#3971)",
			entryReaperSpec57A, reply)
	} else if known[entryReaperSpec55A] == 1 {
		t.Logf("E2E_PASS: two-specialization upload refused (%q); single-specialization upload selected 55", txt)
	}
}

// TestTalentBridgeMessageIsSent covers the ASC_LOCAL_CAD bridge (server PR #4027, format from #4030): after a
// talent change the server whispers the character its active specialization and its held entry ranks on the
// addon channel, so the patch-B local layer can trust the server instead of its SavedVariable and spellbook.
// captureChatDuring replaces the tab after the prefix with a space.
func TestTalentBridgeMessageIsSent(t *testing.T) {
	bot := newBot(t, "TAut7", e2eharness.RaceOrc, classReaper, 12)
	bot.SetSpecialization(t, specReaper, spellReaperAutoSpec56)

	txt := captureChatDuring(t, bot, func() { bot.SetTalentRank(t, entryReaperClassA, 1) })
	if !waitSpell(bot, spellReaperClassA, 5*time.Second) {
		t.Fatalf("precondition: .localtalent %d 1 did not grant spell %d", entryReaperClassA, spellReaperClassA)
	}
	header := fmt.Sprintf("ASC_LOCAL_CAD 1:%d:1:1:", specReaper)
	paid := fmt.Sprintf("%d,1", entryReaperClassA)
	automatic := fmt.Sprintf("%d,1", entryReaperAutoSpec56)
	if !strings.Contains(txt, header) || !strings.Contains(txt, paid) || !strings.Contains(txt, automatic) {
		t.Errorf("E2E_FAIL: bridge message after .localtalent = %q, want %q with %q and %q (#3971)", txt, header,
			paid, automatic)
	} else {
		t.Logf("E2E_PASS: ASC_LOCAL_CAD carried specialization %d, paid entry %d and automatic entry %d",
			specReaper, entryReaperClassA, entryReaperAutoSpec56)
	}
}
