package ports

type Web3SignerAdapter interface {
	GetValidatorPubkeys() ([]string, error)
}
