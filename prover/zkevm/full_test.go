package zkevm

import (
	"fmt"
	"os"
	"testing"

	"github.com/consensys/linea-monorepo/prover/backend/files"
	"github.com/consensys/linea-monorepo/prover/config"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/cleanup"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/innerproduct"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/lookup"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/mimc"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/permutation"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/selfrecursion"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/specialqueries"
	"github.com/consensys/linea-monorepo/prover/protocol/compiler/vortex"
)

func TestProfileColumnInFull(t *testing.T) {

	os.Chdir("..")

	config, err := config.NewConfigFromFile("config/config-integration-full.toml")
	if err != nil {
		panic(err)
	}

	var (
		zkevm = fullZKEVMWithSuite(
			&config.TracesLimits,
			compilationSuite{
				// logdata.Log("initial-wizard"),
				mimc.CompileMiMC,
				compiler.Arcane(1<<10, 1<<19, false),
				vortex.Compile(
					2,
					vortex.ForceNumOpenedColumns(256),
					vortex.WithSISParams(&sisInstance),
				),
				// logdata.Log("post-vortex-1"),

				// First round of self-recursion
				selfrecursion.SelfRecurse,
				// logdata.Log("post-selfrecursion-1"),
				cleanup.CleanUp,
				mimc.CompileMiMC,
				compiler.Arcane(1<<10, 1<<18, false),
				vortex.Compile(
					2,
					vortex.ForceNumOpenedColumns(256),
					vortex.WithSISParams(&sisInstance),
				),
				// logdata.Log("post-vortex-2"),

				// Second round of self-recursion
				selfrecursion.SelfRecurse,
				// logdata.Log("post-selfrecursion-2"),
				cleanup.CleanUp,
				mimc.CompileMiMC,
				compiler.Arcane(1<<10, 1<<16, false),
				vortex.Compile(
					8,
					vortex.ForceNumOpenedColumns(64),
					vortex.WithSISParams(&sisInstance),
				),

				// Fourth round of self-recursion
				// logdata.Log("post-vortex-3"),
				selfrecursion.SelfRecurse,
				// logdata.Log("post-selfrecursion-3"),
				cleanup.CleanUp,
				mimc.CompileMiMC,
				specialqueries.RangeProof,
				specialqueries.CompileFixedPermutations,
				permutation.CompileGrandProduct,
				lookup.CompileLogDerivative,
				innerproduct.Compile,
			},
		)
		comp    = zkevm.WizardIOP
		file    = files.MustOverwrite("full-zkevm-final.csv")
		columns = comp.Columns.AllKeysCommitted()
	)

	fmt.Fprintf(file, "id; size; name\n")

	for i, cname := range columns {
		col := comp.Columns.GetHandle(cname)
		fmt.Fprintf(file, "%v; %v; %v\n", i, col.Size(), col.GetColID())
	}

	defer file.Close()

}
