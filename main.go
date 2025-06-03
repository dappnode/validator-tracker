// main.go
//
// This program checks whether a given validator (by index) performed its attestation duties
// correctly in the latest finalized epoch, using a Lighthouse (DappNode) Beacon API endpoint.
// It scans the next 5 slots after the assigned duty slot (since attestations can appear a few slots later).
//
// Usage:
//   go run main.go <validator_index>
//
// Example:
//   go run main.go 500502

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Replace with your Beacon API base URL
const baseURL = "http://beacon-chain.lighthouse-hoodi.dappnode:3500"

// --- Structs for JSON unmarshalling ---

// FinalityCheckpointsResponse models the response for
// GET /eth/v1/beacon/states/{state_id}/finality_checkpoints
type FinalityCheckpointsResponse struct {
	Data struct {
		Finalized struct {
			Epoch string `json:"epoch"`
		} `json:"finalized"`
	} `json:"data"`
}

// AttesterDutiesResponse models the response for
// POST /eth/v1/validator/duties/attester/{epoch}
type AttesterDutiesResponse struct {
	Data []struct {
		ValidatorIndex        string `json:"validator_index"`
		CommitteeIndex        string `json:"committee_index"`
		CommitteeLength       string `json:"committee_length"`
		CommitteesAtSlot      string `json:"committees_at_slot"`
		ValidatorCommitteeIdx string `json:"validator_committee_index"`
		Slot                  string `json:"slot"`
	} `json:"data"`
}

// CommitteeEntry models one committee entry in
// GET /eth/v1/beacon/states/{state_id}/committees
type CommitteeEntry struct {
	Index      string   `json:"index"`
	Slot       string   `json:"slot"`
	Validators []string `json:"validators"`
}

// CommitteesResponse models the response for
// GET /eth/v1/beacon/states/{state_id}/committees
type CommitteesResponse struct {
	Data []CommitteeEntry `json:"data"`
}

// BlockAttestationsResponse models the response for
// GET /eth/v2/beacon/blocks/{block_id}/attestations
type BlockAttestationsResponse struct {
	Data []struct {
		AggregationBits string `json:"aggregation_bits"` // hex string, e.g. "0x..."
		CommitteeBits   string `json:"committee_bits"`   // hex string, e.g. "0x..."
		Data            struct {
			Slot string `json:"slot"` // the slot that this attestation is for
		} `json:"data"`
	} `json:"data"`
}

// --- Helper functions ---

// parseHexBitvector converts a hex-encoded bitvector (with "0x" prefix) into a byte slice.
// Bits are interpreted in little-endian order within each byte.
func parseHexBitvector(hexstr string) ([]byte, error) {
	trimmed := strings.TrimPrefix(hexstr, "0x")
	if len(trimmed)%2 != 0 {
		// must be even length
		trimmed = "0" + trimmed
	}
	return hex.DecodeString(trimmed)
}

// getBitLE returns the bit (0 or 1) at position bitIndex in the little-endian bitvector stored in data.
// bitIndex = 0 refers to least-significant bit of data[0], bitIndex=8 refers to LSB of data[1], etc.
func getBitLE(data []byte, bitIndex int) int {
	byteIdx := bitIndex / 8
	if byteIdx < 0 || byteIdx >= len(data) {
		return 0
	}
	b := data[byteIdx]
	return int((b >> (bitIndex % 8)) & 1)
}

// httpGetJSON sends a GET request to the given URL and unmarshals the JSON response into outStruct.
func httpGetJSON(url string, outStruct interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, string(body))
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}
	if err := json.Unmarshal(data, outStruct); err != nil {
		return fmt.Errorf("unmarshal JSON: %v\nJSON was: %s", err, string(data))
	}
	return nil
}

// httpPostJSON sends a POST request with a JSON payload (body) to the given URL and unmarshals the JSON response into outStruct.
func httpPostJSON(url string, body []byte, outStruct interface{}) error {
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respData, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", url, resp.StatusCode, string(respData))
	}
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}
	if err := json.Unmarshal(respData, outStruct); err != nil {
		return fmt.Errorf("unmarshal JSON: %v\nJSON was: %s", err, string(respData))
	}
	return nil
}

func main() {
	// 1) Parse validator index from command-line
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: go run main.go <validator_index>\n")
		os.Exit(1)
	}
	validatorIndexStr := os.Args[1]
	validatorIndex, err := strconv.ParseUint(validatorIndexStr, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid validator index: %v\n", err)
		os.Exit(1)
	}

	// 2) Fetch finalized epoch (use “head” to learn the latest finalized checkpoint)
	finalityURL := fmt.Sprintf("%s/eth/v1/beacon/states/head/finality_checkpoints", baseURL)
	var finResp FinalityCheckpointsResponse
	if err := httpGetJSON(finalityURL, &finResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching finality checkpoints: %v\n", err)
		os.Exit(1)
	}
	finalEpoch, err := strconv.ParseUint(finResp.Data.Finalized.Epoch, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing finalized epoch: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Latest finalized epoch: %d\n", finalEpoch)

	// 3) Fetch attester duties for that epoch, for this validator
	dutiesURL := fmt.Sprintf("%s/eth/v1/validator/duties/attester/%d", baseURL, finalEpoch)
	reqBody, _ := json.Marshal([]string{validatorIndexStr})

	var dutiesResp AttesterDutiesResponse
	if err := httpPostJSON(dutiesURL, reqBody, &dutiesResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching attester duties: %v\n", err)
		os.Exit(1)
	}
	if len(dutiesResp.Data) == 0 {
		fmt.Fprintf(os.Stderr, "No attester duties found for validator %d in epoch %d\n", validatorIndex, finalEpoch)
		os.Exit(1)
	}
	// We expect exactly one duty entry for this validator in the given epoch
	duty := dutiesResp.Data[0]
	commIdx, _ := strconv.ParseUint(duty.CommitteeIndex, 10, 64)
	slot, _ := strconv.ParseUint(duty.Slot, 10, 64)
	valCommIdx, _ := strconv.ParseUint(duty.ValidatorCommitteeIdx, 10, 64)

	fmt.Printf("Validator %d duties for epoch %d: slot=%d, committee_index=%d, validator_committee_index=%d\n",
		validatorIndex, finalEpoch, slot, commIdx, valCommIdx)

	// 4) Fetch committees for the finalized state at that slot
	commURL := fmt.Sprintf("%s/eth/v1/beacon/states/finalized/committees?slot=%d", baseURL, slot)
	var commResp CommitteesResponse
	if err := httpGetJSON(commURL, &commResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching committees: %v\n", err)
		os.Exit(1)
	}
	if len(commResp.Data) == 0 {
		fmt.Fprintf(os.Stderr, "No committees found for slot %d (finalized state)\n", slot)
		os.Exit(1)
	}

	// Build a map of committee_index -> validator list
	commMap := make(map[uint64][]string)
	for _, entry := range commResp.Data {
		idx, _ := strconv.ParseUint(entry.Index, 10, 64)
		commMap[idx] = entry.Validators
	}
	if _, ok := commMap[commIdx]; !ok {
		fmt.Fprintf(os.Stderr, "Committee index %d not found in committees for slot %d\n", commIdx, slot)
		os.Exit(1)
	}

	// Compute offset: sum sizes of all committees with index < commIdx
	var indices []uint64
	for idx := range commMap {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	var offset uint64
	for _, idx := range indices {
		if idx == commIdx {
			break
		}
		offset += uint64(len(commMap[idx]))
	}
	fmt.Printf("Computed aggregation bit offset for committee %d: %d\n", commIdx, offset)

	// 5) Scan the next 5 slots (slot+1 .. slot+5) for attestations
	//    Because attestations may be included a few slots after the duty slot.
	const scanWindow = 5
	attested := false

	for b := slot + 1; b <= slot+scanWindow; b++ {
		blockAttURL := fmt.Sprintf("%s/eth/v2/beacon/blocks/%d/attestations", baseURL, b)
		var blockAttResp BlockAttestationsResponse

		err := httpGetJSON(blockAttURL, &blockAttResp)
		if err != nil {
			// If the block doesn't exist or has no attestations endpoint, skip quietly
			fmt.Fprintf(os.Stderr, "  [slot %d] warning: could not fetch attestations: %v\n", b, err)
			continue
		}
		if len(blockAttResp.Data) == 0 {
			// No attestations in this block; keep scanning
			continue
		}

		// Inspect each attestation in the block
		for _, att := range blockAttResp.Data {
			// 1) Parse the attestation’s declared slot, and skip if it’s not our duty slot
			attSlot, err := strconv.ParseUint(att.Data.Slot, 10, 64)
			if err != nil {
				// If parsing fails, skip
				continue
			}
			if attSlot != slot {
				// This attestation is for some other slot, so skip
				continue
			}

			// 2) Check committee_bits
			cbBytes, err := parseHexBitvector(att.CommitteeBits)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    error parsing committee_bits at slot %d: %v\n", b, err)
				continue
			}
			if getBitLE(cbBytes, int(commIdx)) == 0 {
				continue
			}

			// 3) Parse aggregation_bits and check our bit
			aggBytes, err := parseHexBitvector(att.AggregationBits)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    error parsing aggregation_bits at slot %d: %v\n", b, err)
				continue
			}
			targetBit := int(offset + valCommIdx)
			if getBitLE(aggBytes, targetBit) == 1 {
				fmt.Printf("  ▶️ Found validator bit for slot %d in block %d.\n", slot, b)
				attested = true
				break
			}
		}

		if attested {
			break
		}
	}

	// 6) Report final result
	if attested {
		fmt.Printf("✅ Validator %d DID attest correctly for slot %d (epoch %d).\n", validatorIndex, slot, finalEpoch)
	} else {
		fmt.Printf("❌ Validator %d did NOT attest for slot %d within the next %d slots (epoch %d).\n",
			validatorIndex, slot, scanWindow, finalEpoch)
	}
}
