package prover

import (
	"encoding/json"
	"github.com/consensys/linea-monorepo/prover/backend/aggregation"
	"github.com/consensys/linea-monorepo/prover/backend/blobdecompression"
	"github.com/consensys/linea-monorepo/prover/backend/execution"
	"github.com/consensys/linea-monorepo/prover/config"
	"github.com/consensys/linea-monorepo/prover/lib/compressor/blob"
	"github.com/consensys/linea-monorepo/prover/utils/test_utils"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var repoRoot string

func init() {
	var err error
	repoRoot, err = blob.GetRepoRootPath()
	require.NoError(test_utils.FakeTestingT{}, err)
}

func TestAggregationError(t *testing.T) {
	// load failing config and request
	cfg, err := config.NewConfigFromFile("config/config-integration-full.toml")
	require.NoError(t, err)
	f, err := os.Open("../testdata/sepolia-data/prover-aggregation/requests/4454961-4457351-645da7ecbd6d96b7c23c2a5a337b90e6060b36c0aea3957174253fef01c10f16-getZkAggregatedProof.json")
	require.NoError(t, err)
	var req aggregation.Request
	require.NoError(t, json.NewDecoder(f).Decode(&req))
	require.NoError(t, f.Close())

	_, err = aggregation.Prove(cfg, &req)
	require.NoError(t, err)
}

func TestExecution(t *testing.T) {
	cfg, err := config.NewConfigFromFile("config/config-integration-full.toml")
	require.NoError(t, err)

	for _, req := range readRequests[execution.Request](t, "../testdata/sepolia-data/prover-execution/requests/") {

		resp, err := execution.Prove(cfg, req.body, false)
		require.NoError(t, err)

		f, err := os.OpenFile(repoRoot+"/prover/integration/full-mode/tmp/prover-execution/responses/"+req.blockRange+"-getZkProof.json", os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(f).Encode(resp))
		require.NoError(t, f.Close())
	}
}

func TestDecompression(t *testing.T) {
	cfg, err := config.NewConfigFromFile("config/config-integration-full.toml")
	require.NoError(t, err)

	for _, req := range readRequests[blobdecompression.Request](t, "../testdata/sepolia-data/prover-compression/requests/") {

		resp, err := blobdecompression.Prove(cfg, req.body)
		require.NoError(t, err)

		f, err := os.OpenFile(repoRoot+"/prover/integration/full-mode/tmp/prover-compression/responses/"+req.blockRange+"-getZkProof.json", os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(f).Encode(resp))
		require.NoError(t, f.Close())
	}
}

type request[T any] struct {
	body       *T
	blockRange string
}

func readRequests[T any](t *testing.T, dirPath string) []request[T] {
	dir, err := os.ReadDir(dirPath)
	require.NoError(t, err)
	res := make([]request[T], 0, len(dir))
	for _, fPath := range dir {
		if fPath.IsDir() {
			continue
		}
		dashIndex := strings.Index(fPath.Name(), "-")                   // first dash
		dashIndex += 1 + strings.Index(fPath.Name()[dashIndex+1:], "-") //second dash
		blockRange := fPath.Name()[:dashIndex]

		f, err := os.Open(filepath.Join(dirPath + fPath.Name()))
		require.NoError(t, err)
		var req T
		require.NoError(t, json.NewDecoder(f).Decode(&req))
		require.NoError(t, f.Close())

		res = append(res, request[T]{body: &req, blockRange: blockRange})
	}
	return res
}
