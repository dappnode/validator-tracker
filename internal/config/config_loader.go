package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/dappnode/validator-tracker/internal/logger"
)

type Config struct {
	BeaconEndpoint     string
	Web3SignerEndpoint string
	Network            string
	SignerDnpName      string
	BeaconchaUrl       string
	DappmanagerUrl     string
	NotifierUrl        string
}

func LoadConfig() Config {
	network := os.Getenv("NETWORK")
	if network == "" {
		network = "hoodi" // default
	}

	// Build the dynamic endpoints
	beaconEndpoint := fmt.Sprintf("http://beacon-chain.%s.dncore.dappnode:3500", network)
	web3SignerEndpoint := fmt.Sprintf("http://web3signer.web3signer-%s.dappnode:9000", network)
	dappmanagerEndpoint := "http://dappmanager.dappnode"
	notifierEndpoint := "http://notifier.dappnode:8080f"

	// Allow override via environment variables
	if envBeacon := os.Getenv("BEACON_ENDPOINT"); envBeacon != "" {
		beaconEndpoint = envBeacon
	}
	if envWeb3Signer := os.Getenv("WEB3SIGNER_ENDPOINT"); envWeb3Signer != "" {
		web3SignerEndpoint = envWeb3Signer
	}
	if envDappmanager := os.Getenv("DAPPMANAGER_ENDPOINT"); envDappmanager != "" {
		dappmanagerEndpoint = envDappmanager
	}
	if envNotifier := os.Getenv("NOTIFIER_URL"); envNotifier != "" {
		notifierEndpoint = envNotifier
	}

	// Normalize network name for logs
	network = strings.ToLower(network)
	if network != "hoodi" && network != "holesky" && network != "mainnet" && network != "gnosis" && network != "lukso" {
		logger.Fatal("Unknown network: %s", network)
	}

	var dnpName string
	if network == "mainnet" {
		dnpName = "web3signer.dnp.dappnode.eth"
	} else {
		dnpName = fmt.Sprintf("web3signer-%s.dnp.dappnode.eth", network)
	}

	var beaconchaUrl string
	switch network {
	case "mainnet":
		beaconchaUrl = "https://beaconcha.in"
	case "holesky":
		beaconchaUrl = "https://holesky.beaconcha.in"
	case "hoodi":
		beaconchaUrl = "https://hoodi.beaconcha.in"
	case "gnosis":
		beaconchaUrl = "https://gnosischa.in"
	case "lukso":
		beaconchaUrl = "https://explorer.consensus.mainnet.lukso.network"
	default:
		logger.Fatal("Unsupported network for beaconcha URL: %s", network)
	}

	return Config{
		BeaconEndpoint:     beaconEndpoint,
		Web3SignerEndpoint: web3SignerEndpoint,
		Network:            network,
		SignerDnpName:      dnpName,
		BeaconchaUrl:       beaconchaUrl,
		DappmanagerUrl:     dappmanagerEndpoint,
		NotifierUrl:        notifierEndpoint,
	}
}
