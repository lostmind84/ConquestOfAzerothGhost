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
