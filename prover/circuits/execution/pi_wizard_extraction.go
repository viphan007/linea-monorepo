package execution

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
	"github.com/consensys/linea-monorepo/prover/circuits/internal"
	"github.com/consensys/linea-monorepo/prover/protocol/wizard"
	"github.com/consensys/linea-monorepo/prover/zkevm/prover/publicInput"
)

// checkPublicInputs checks that the values in fi are consistent with the
// wizard.VerifierCircuit
func checkPublicInputs(
	api frontend.API,
	wvc *wizard.WizardVerifierCircuit,
	gnarkFuncInp FunctionalPublicInputSnark,
	wizardFuncInp publicInput.FunctionalInputExtractor,
) {

	var (
		finalRollingHash   = internal.CombineBytesIntoElements(api, gnarkFuncInp.FinalRollingHash)
		initialRollingHash = internal.CombineBytesIntoElements(api, gnarkFuncInp.InitialRollingHash)
		execDataHash       = execDataHash(api, wvc, wizardFuncInp)
	)

	// As we have this issue, the execDataHash will not match what we have in the
	// functional input (the txnrlp is incorrect). It should be converted into
	// an [api.AssertIsEqual] once this is resolved.
	//
	shouldBeEqual(api, execDataHash, gnarkFuncInp.DataChecksum)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameL2MessageHash),
		// TODO: this operation is done a second time when computing the final
		// public input which is wasteful although not dramatic (~8000 unused
		// constraints)
		gnarkFuncInp.L2MessageHashes.CheckSumMiMC(api),
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalStateRootHash),
		gnarkFuncInp.InitialStateRootHash,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameInitialBlockNumber),
		gnarkFuncInp.InitialBlockNumber,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameInitialBlockTimestamp),
		gnarkFuncInp.InitialBlockTimestamp,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameInitialRollingHash_0),
		initialRollingHash[0],
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameInitialRollingHash_1),
		initialRollingHash[1],
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameInitialRollingHashNumber),
		gnarkFuncInp.InitialRollingHashNumber,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalStateRootHash),
		gnarkFuncInp.FinalStateRootHash,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalBlockNumber),
		gnarkFuncInp.FinalBlockNumber,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalBlockTimestamp),
		gnarkFuncInp.FinalBlockTimestamp,
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalRollingHash_0),
		finalRollingHash[0],
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalRollingHash_1),
		finalRollingHash[1],
	)

	api.AssertIsEqual(
		findPubInput(api, wvc, publicInput.PubInputNameFinalRollingHashNumber),
		gnarkFuncInp.FinalRollingHashNumber,
	)

	var (
		twoPow128     = new(big.Int).SetInt64(1)
		twoPow112     = new(big.Int).SetInt64(1)
		_             = twoPow128.Lsh(twoPow128, 128)
		_             = twoPow112.Lsh(twoPow112, 112)
		bridgeAddress = api.Add(
			api.Mul(
				twoPow128,
				findPubInput(api, wvc, publicInput.PubInputNameL2MessageServiceAddrHi),
			),
			findPubInput(api, wvc, publicInput.PubInputNameL2MessageServiceAddrLo),
		)
	)

	api.AssertIsEqual(
		api.Div(
			findPubInput(api, wvc, publicInput.PubInputNameChainID),
			twoPow112,
		),
		gnarkFuncInp.ChainID,
	)

	api.AssertIsEqual(bridgeAddress, gnarkFuncInp.L2MessageServiceAddr)

}

// execDataHash hash the execution-data with its length so that we can guard
// against padding attack (although the padding attacks are not possible to
// being with due to the encoding of the plaintext)
func execDataHash(
	api frontend.API,
	wvc *wizard.WizardVerifierCircuit,
	wFuncInp publicInput.FunctionalInputExtractor,
) frontend.Variable {

	hsh, err := mimc.NewMiMC(api)
	if err != nil {
		panic(err)
	}

	hsh.Write(
		findPubInput(api, wvc, publicInput.PubInputNameDataNbBytes),
		findPubInput(api, wvc, publicInput.PubInputNameDataChecksum),
	)

	return hsh.Sum()
}

// shouldBeEqual is a placeholder dummy function that generate fake constraints
// as a replacement for what should be an api.AssertIsEqual. If we just commented
// out the api.AssertIsEqual we might have an unconstrained variable.
func shouldBeEqual(api frontend.API, a, b frontend.Variable) {
	_ = api.Sub(a, b)
}

func findPubInput(api frontend.API, wvc *wizard.WizardVerifierCircuit, name string) frontend.Variable {

	for _, wp := range wvc.Spec.PublicInputs {
		if wp.Name == name {
			return wp.Acc.GetFrontendVariable(api, wvc)
		}
	}

	panic(fmt.Sprintf("could not find public input with name %v, the available list is %v", name, wvc.Spec.PublicInputs))
}
