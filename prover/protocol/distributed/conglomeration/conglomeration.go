package conglomeration

import (
	"errors"
	"fmt"

	"github.com/consensys/linea-monorepo/prover/protocol/distributed"
	"github.com/consensys/linea-monorepo/prover/protocol/ifaces"
	"github.com/consensys/linea-monorepo/prover/protocol/wizard"
)

type AggregateInput struct {
	Module         distributed.ModuleName
	SegmentID      int
	LookupPermProj wizard.Proof
	GlobalLocal    wizard.Proof
}

type compTemplate struct {
	columnCommitLsName []ifaces.ColID
	columnProofLsName  []ifaces.ColID
	columnCommitLsSize []int
	columnProofLsSize  []int
	queryParamsLs      []ifaces.QueryID
	queryNoParamsLs    []ifaces.QueryID
}

func ConglomerateWizardIOP(
	publicInputModule distributed.DistributedModule,
	distModules []distributed.DistributedModule,
	maxNumSegment int,
) func(build *wizard.Builder) {

	compTemplate, err := getCompTemplate(distModules)
	if err != nil {
		panic(err)
	}

	return nil
}

func getCompTemplate(distModules []distributed.DistributedModule) (res *compTemplate, err error) {

	for _, distModule := range distModules {

		var (
			comps        = []*wizard.CompiledIOP{distModule.GlobalLocal, distModule.LookupPermProj}
			subCompNames = []string{"global-local", "lookup-permutation-projection"}
		)

		for i, comp := range comps {

			var (
				locColumnCommitLsName = comp.Columns.AllKeysCommitted()
				locColumnProofLsName  = comp.Columns.AllKeysProof()
				locTmpl               = &compTemplate{
					columnCommitLsName: comp.Columns.AllKeysCommitted(),
					columnProofLsName:  comp.Columns.AllKeysProof(),
					columnCommitLsSize: make([]int, len(locColumnCommitLsName)),
					columnProofLsSize:  make([]int, len(locColumnProofLsName)),
					queryParamsLs:      comp.QueriesParams.AllUnignoredKeys(),
					queryNoParamsLs:    comp.QueriesNoParams.AllUnignoredKeys(),
				}
			)

			for i, cName := range locTmpl.columnCommitLsName {
				locTmpl.columnCommitLsSize[i] = comp.Columns.GetSize(cName)
			}

			for i, cName := range locTmpl.columnProofLsName {
				locTmpl.columnProofLsSize[i] = comp.Columns.GetSize(cName)
			}

			if res == nil {
				res = locTmpl
			}

			if !res.Eq(locTmpl) {
				err = errors.Join(
					err,
					fmt.Errorf("module did not match the template name=%v.%v", distModule.Name, subCompNames[i]),
				)
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return res, nil

}

func (tmpl *compTemplate) Eq(res *compTemplate) bool {
	return listsAreEquals(tmpl.columnCommitLsName, res.columnCommitLsName) &&
		listsAreEquals(tmpl.columnProofLsName, res.columnProofLsName) &&
		listsAreEquals(tmpl.columnCommitLsSize, res.columnCommitLsSize) &&
		listsAreEquals(tmpl.columnProofLsSize, res.columnProofLsSize) &&
		listsAreEquals(tmpl.queryNoParamsLs, res.queryNoParamsLs) &&
		listsAreEquals(tmpl.queryParamsLs, res.queryParamsLs)
}

func listsAreEquals[T comparable](a, b []T) bool {

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
