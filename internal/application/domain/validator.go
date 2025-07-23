package domain

// --------------------------------------------------------

// Domain types used for anything related to validators
type Epoch uint64
type Slot uint64
type ValidatorIndex uint64

// --------------------------------------------------------

// Attestation-related types
type ValidatorDuty struct {
	Slot                  Slot
	CommitteeIndex        CommitteeIndex
	ValidatorCommitteeIdx uint64
	ValidatorIndex        ValidatorIndex
}

type Attestation struct {
	DataSlot        Slot
	CommitteeBits   []byte
	AggregationBits []byte
}

type CommitteeSizeMap map[CommitteeIndex]int
type CommitteeIndex uint64

// --------------------------------------------------------

// Proposer-related types
type ProposerDuty struct {
	Slot           Slot
	ValidatorIndex ValidatorIndex
}
