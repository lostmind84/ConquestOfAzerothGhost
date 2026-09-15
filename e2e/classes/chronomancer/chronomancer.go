// Package chronomancer defines constants for the custom Chronomancer class (ID 22).
package chronomancer

// Class ID in Conquest of Azeroth
const (
	ClassID = 22
)

// Specialization IDs for Chronomancer (Class 22)
const (
	SpecTime      uint32 = 31
	SpecInfinite  uint32 = 32
	SpecArtificer uint32 = 33
)

// Baseline / signature Abilities automatically granted upon selecting each specialization
const (
	// Spec 31: Time
	AbilityTimeBaseline    uint32 = 92119  // "Aeon of Resilience" -- Learned at Level 10

	// Spec 32: Infinite
	AbilityInfiniteBaseline uint32 = 92118 // Baseline Infinite ability (level 10)

	// Spec 33: Artificer
	AbilityArtificerBaseline uint32 = 92120 // Baseline Artificer ability (level 10)
)

// Common Chronomancer Abilities & Auras
const (
	AbilityEpoch       uint32 = 501784 // Epoch (Rank 7)
)

// CoA Hidden Auras — these passive auras should be applied automatically by the
// server when a Chronomancer is created or switches specialization. They modify
// class/spec balance values (e.g. healing coefficients). Currently bugged: the
// server does not apply them unless done manually with `.aura`.
const (
	AuraChronomancerClass     uint32 = 887110 // CoA Aura - Chronomancer Class
	AuraChronomancerInfinite  uint32 = 887153 // CoA Aura - Chronomancer Infinite
	AuraChronomancerTime      uint32 = 887154 // CoA Aura - Chronomancer Time
	AuraChronomancerArtificer uint32 = 887155 // CoA Aura - Chronomancer Artificer
)
