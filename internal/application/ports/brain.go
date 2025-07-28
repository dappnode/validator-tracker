package ports

// BrainAdapter exposes the same method as Web3SignerAdapter for validator pubkeys
type BrainAdapter interface {
	GetValidatorPubkeys() ([]string, error)
}
