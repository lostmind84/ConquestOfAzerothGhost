package e2eharness

import (
	"fmt"
	"testing"
	"time"
)

// SetSpecialization changes the character's active specialization using the Ascension/CoA .localspec command.
// If an expectedSpell is provided, it uses the "spell known" technique to wait until the server confirms the switch
// by granting the baseline ability for that specialization.
func (b *ScenarioBot) SetSpecialization(t *testing.T, specID uint32, expectedSpell ...uint32) {
	t.Helper()

	b.GM(t, fmt.Sprintf(".localspec %d", specID))

	if len(expectedSpell) > 0 && expectedSpell[0] != 0 {
		spell := expectedSpell[0]
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if b.World.KnowsSpell(spell) {
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for bot to learn baseline spec spell %d after .localspec %d", spell, specID)
	} else {
		time.Sleep(500 * time.Millisecond)
	}
}

// SetTalentRank sets a CoA talent entry to the given rank using .localtalent.
func (b *ScenarioBot) SetTalentRank(t *testing.T, entryID, rank uint32) {
	t.Helper()
	b.GM(t, fmt.Sprintf(".localtalent %d %d", entryID, rank))
	time.Sleep(200 * time.Millisecond)
}

// ActivateCharacterAdvancement sends CMSG_SET_ACTIVE_MOVER for the bot's own character.
//
// The real client sends this once its loading screen ends, and mod-ascension-compat waits for it before
// pushing the character's CoA talent state (SMSG_CHARACTER_ADVANCEMENT_ACTIVE_SPEC 0x0725 followed by
// SMSG_CHARACTER_ADVANCEMENT_KNOWN_ENTRIES 0x0726); a second one for the same login is a no-op server-side.
// Unlike bot/bot.go (the AI bot, not this harness), the e2eharness login path (NewSolo/LoginBots, Relog)
// never sends CMSG_SET_ACTIVE_MOVER, so a test that needs the character-advancement packets must call this
// explicitly once, after registering any packet hook that watches for them.
func (b *ScenarioBot) ActivateCharacterAdvancement(t *testing.T) {
	t.Helper()
	if err := b.World.SetActiveMover(b.World.CharGUID()); err != nil {
		t.Fatalf("SetActiveMover: %v", err)
	}
}
