package globalcs

import (
	"math/big"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/consensys/linea-monorepo/prover/protocol/coin"
	"github.com/consensys/linea-monorepo/prover/protocol/variables"

	"github.com/sirupsen/logrus"

	"github.com/consensys/linea-monorepo/prover/maths/common/mempool"
	sv "github.com/consensys/linea-monorepo/prover/maths/common/smartvectors"
	"github.com/consensys/linea-monorepo/prover/maths/fft"
	"github.com/consensys/linea-monorepo/prover/maths/fft/fastpoly"
	"github.com/consensys/linea-monorepo/prover/maths/field"
	"github.com/consensys/linea-monorepo/prover/protocol/column"
	"github.com/consensys/linea-monorepo/prover/protocol/ifaces"
	"github.com/consensys/linea-monorepo/prover/protocol/wizard"
	"github.com/consensys/linea-monorepo/prover/protocol/wizardutils"
	"github.com/consensys/linea-monorepo/prover/symbolic"
	"github.com/consensys/linea-monorepo/prover/utils"
	"github.com/consensys/linea-monorepo/prover/utils/collection"
	ppool "github.com/consensys/linea-monorepo/prover/utils/parallel/pool"
	"github.com/consensys/linea-monorepo/prover/utils/profiling"
)

const (
	/*
		Explanation for Manual Garbage Collection Thresholds
	*/
	// These two thresholds work well for the real-world traces at the moment of writing and a 340GiB memory limit,
	// but this approach can be generalized and further improved.

	// When ctx.domainSize>=524288, proverEvaluationQueries() experiences a heavy workload,
	// consistently hitting the GOMEMLIMIT of 340GiB.
	// This results in numerous auto GCs during CPU-intensive small tasks, significantly degrading performance.
	// In the benchmark input files, GC_DOMAIN_SIZE >= 524288 means only the first call of proverEvaluationQueries().
	// With ctx.domainSize<=262144, manual GC is not necessary as auto GCs triggered by GOMEMLIMIT suffice.
	GC_DOMAIN_SIZE int = 524288

	// Auto GCs are triggered during ReEvaluate and Batch evaluation
	// when len(handles) exceeds approximately 4000, causing performance degradation.
	// This threshold is set to perform manual GCs before ReEvaluate and Batch evaluation
	// only when len(handles) reaches a size substantial enough to trigger auto GC during ReEvaluate and Batch evaluation.
	// Note that the value of GC_HANDLES_SIZE 4000 is derived from experience and analytics on the benchmark input files.
	GC_HANDLES_SIZE int = 4000
)

// QuotientCtx collects all the internal fields needed to compute the quotient
type QuotientCtx struct {

	// DomainSize is the domain over which the global constraints are computed
	DomainSize int

	// Ratio lists the ratio found in the global constraints
	//
	// See [mergingCtx.Ratios]
	Ratios []int

	// ColumnsForRatio[k] stores all the columns involved in the aggregate
	// expressions for ratio Ratios[k]
	ColumnsForRatio [][]ifaces.Column

	// RootsPerRatio[k] stores all the root columns involved in the aggregate
	// expressions for ration Ratios[k]. By root column we mean the underlying
	// column that are actually committed to. For instance, if Shift(A, 1) is
	// in ColumnsForRatio[k], we will have A in RootPerRatio[k]
	RootsForRatio [][]ifaces.Column

	// AllInvolvedColumns stores the union of the ColumnForRatio[k] for all k
	AllInvolvedColumns []ifaces.Column

	// AllInvolvedRoots stores the union of the RootsForRatio[k] for all k
	AllInvolvedRoots []ifaces.Column

	// AggregateExpressions[k] stores the aggregate expression for Ratios[k]
	AggregateExpressions []*symbolic.Expression

	// AggregateExpressionsBoard[k] stores the topological sorting of
	// AggregateExpressions[k]
	AggregateExpressionsBoard []symbolic.ExpressionBoard

	// QuotientShares[k] stores for each k, the list of the Ratios[k] shares
	// of the quotient for the AggregateExpression[k]
	QuotientShares [][]ifaces.Column

	// MaxNbExprNode stores the largest number of node AggregateExpressionBoard[*]
	// has. This is used to dimension the memory pool during the prover time.
	MaxNbExprNode int
}

// createQuotientCtx constructs a [quotientCtx] from a list of ratios and aggregated
// expressions. The function organizes the handles but does not declare anything
// in the current wizard.CompiledIOP.
func createQuotientCtx(comp *wizard.CompiledIOP, ratios []int, aggregateExpressions []*symbolic.Expression) QuotientCtx {

	var (
		allInvolvedHandlesIndex = map[ifaces.ColID]int{}
		allInvolvedRootsSet     = collection.NewSet[ifaces.ColID]()
		_, _, domainSize        = wizardutils.AsExpr(aggregateExpressions[0])
		ctx                     = QuotientCtx{
			DomainSize:                domainSize,
			Ratios:                    ratios,
			AggregateExpressions:      aggregateExpressions,
			AggregateExpressionsBoard: make([]symbolic.ExpressionBoard, len(ratios)),
			AllInvolvedColumns:        []ifaces.Column{},
			AllInvolvedRoots:          []ifaces.Column{},
			ColumnsForRatio:           make([][]ifaces.Column, len(ratios)),
			RootsForRatio:             make([][]ifaces.Column, len(ratios)),
			QuotientShares:            generateQuotientShares(comp, ratios, domainSize),
		}
	)

	for k, expr := range ctx.AggregateExpressions {

		var (
			board               = expr.Board()
			uniqueRootsForRatio = collection.NewSet[ifaces.ColID]()
		)

		ctx.AggregateExpressionsBoard[k] = board
		ctx.MaxNbExprNode = max(ctx.MaxNbExprNode, board.CountNodes())

		// This loop scans the metadata looking for columns with the goal of
		// populating the collections composing quotientCtx.
		for _, metadata := range board.ListVariableMetadata() {

			// Scan in column metadata only
			col, ok := metadata.(ifaces.Column)
			if !ok {
				continue
			}

			var (
				rootCol = column.RootParents(col)
			)

			// Append the handle (we trust that there are no duplicate of handles
			// within a constraint). This works because the symbolic package has
			// automatic simplification routines that ensure that an expression
			// does not refer to duplicates of the same variable.
			ctx.ColumnsForRatio[k] = append(ctx.ColumnsForRatio[k], col)

			if !uniqueRootsForRatio.Exists(rootCol.GetColID()) {
				ctx.RootsForRatio[k] = append(ctx.RootsForRatio[k], rootCol)
			}

			// Get the name of the
			if _, alreadyThere := allInvolvedHandlesIndex[col.GetColID()]; alreadyThere {
				continue
			}

			allInvolvedHandlesIndex[col.GetColID()] = len(ctx.AllInvolvedColumns)
			ctx.AllInvolvedColumns = append(ctx.AllInvolvedColumns, col)

			// If the handle is simply a shift or a natural columns tracks its root
			if !allInvolvedRootsSet.Exists(rootCol.GetColID()) {
				allInvolvedRootsSet.Insert(rootCol.GetColID())
				ctx.AllInvolvedRoots = append(ctx.AllInvolvedRoots, rootCol)
			}
		}
	}

	return ctx
}

// generateQuotientShares declares and returns the quotient share columns
func generateQuotientShares(comp *wizard.CompiledIOP, ratios []int, domainSize int) [][]ifaces.Column {

	var (
		quotientShares = make([][]ifaces.Column, len(ratios))
		currRound      = comp.NumRounds() - 1
	)

	for i, ratio := range ratios {
		quotientShares[i] = make([]ifaces.Column, ratio)
		for k := range quotientShares[i] {
			quotientShares[i][k] = comp.InsertCommit(
				currRound,
				ifaces.ColID(deriveName(comp, QUOTIENT_POLY_TMPL, ratio, k)),
				domainSize,
			)
		}
	}

	return quotientShares
}

// Run implements the [wizard.ProverAction] interface and embeds the logic to
// compute the quotient shares.
func (ctx *QuotientCtx) Run(run *wizard.ProverRuntime) {

	// threadSafeVector is a wrapper around the smart vector that makes it lockable
	type threadSafeVector struct {
		*sync.Mutex
		sv.SmartVector
	}

	var (
		// Tracks the time spent on garbage collection
		totalTimeGc = int64(0)

		// Initial step is to compute the FFTs for all committed vectors
		coeffs        = sync.Map{} // (ifaces.ColID <=> sv.SmartVector)
		pool          = mempool.CreateFromSyncPool(symbolic.MaxChunkSize).Prewarm(runtime.GOMAXPROCS(0) * ctx.MaxNbExprNode)
		largePool     = mempool.CreateFromSyncPool(ctx.DomainSize).Prewarm(len(ctx.AllInvolvedColumns))
		timeIFFT      time.Duration
		timeFFT       time.Duration
		timeExecRatio = map[int]time.Duration{}
		timeOmega     time.Duration
	)

	if ctx.DomainSize >= GC_DOMAIN_SIZE {
		// Force the GC to run
		tGc := time.Now()
		runtime.GC()
		totalTimeGc += time.Since(tGc).Milliseconds()
	}

	timeIFFT += profiling.TimeIt(func() {
		// Compute once the FFT of the natural columns
		ppool.ExecutePoolChunky(len(ctx.AllInvolvedRoots), func(k int) {
			pol := ctx.AllInvolvedRoots[k]
			name := pol.GetColID()

			// gets directly a shallow copy in the map of the runtime
			var witness sv.SmartVector
			witness, isNatural := run.Columns.TryGet(name)

			// can happen if the column is verifier defined. In that case, no
			// need to protect with a lock. This will not touch run.Columns.
			if !isNatural {
				witness = pol.GetColAssignment(run)
			}

			witness = sv.FFTInverse(witness, fft.DIF, false, 0, 0, nil, fft.WithNbTasks(2))
			coeffs.Store(name, threadSafeVector{&sync.Mutex{}, witness})
		})
	})

	// Take the max quotient degree
	maxRatio := utils.Max(ctx.Ratios...)

	/*
		For the quotient, we precompute the values of (wQ^N - 1)^-1 for w in H, the
		larger domain.

		Those values are D-periodic, thus we only compute a single period.
		(Where D is the ratio of the sizes of the larger and the smaller domain)

		The first value is ignored because it correspond to the case where w^N = 1
		(i.e. w is in the smaller subgroup)
	*/
	annulatorInvVals := fastpoly.EvalXnMinusOneOnACoset(ctx.DomainSize, ctx.DomainSize*maxRatio)
	annulatorInvVals = field.ParBatchInvert(annulatorInvVals, runtime.GOMAXPROCS(0))

	/*
		Also returns the evaluations of
	*/

	for i := 0; i < maxRatio; i++ {

		// use sync map to store the coset evaluated polynomials
		computedReeval := sync.Map{}

		timeOmega += profiling.TimeIt(func() {

			// The following computes the quotient polynomial and assigns it
			// Omega is a root of unity which generates the domain of evaluation of the
			// constraint. Its size coincide with the size of the domain of evaluation.
			// For each value of `i`, X will evaluate to gen*omegaQ^numCoset*omega^i.
			// Gen is a generator of F^*
			var (
				omega        = fft.GetOmega(ctx.DomainSize)
				omegaQNumCos = fft.GetOmega(ctx.DomainSize * maxRatio)
				omegaI       = field.NewElement(field.MultiplicativeGen)
			)

			omegaQNumCos.Exp(omegaQNumCos, big.NewInt(int64(i)))
			omegaI.Mul(&omegaI, &omegaQNumCos)

			// Precomputations of the powers of omega, can be optimized if useful
			omegas := make([]field.Element, ctx.DomainSize)
			for i := range omegas {
				omegas[i] = omegaI
				omegaI.Mul(&omegaI, &omega)
			}
		})

		for j, ratio := range ctx.Ratios {

			if _, ok := timeExecRatio[ratio]; !ok {
				timeExecRatio[ratio] = time.Duration(0)
			}

			// For instance, if deg = 2 and max deg 8, we enter only if
			// i = 0 or 4 because this corresponds to the cosets we are
			// interested in.
			if i%(maxRatio/ratio) != 0 {
				continue
			}

			// With the above example, if we are in the ratio = 2 and maxRatio = 8
			// and i = 1 (it can only be 0 <= i < ratio).
			var (
				share     = i * ratio / maxRatio
				handles   = ctx.ColumnsForRatio[j]
				roots     = ctx.RootsForRatio[j]
				board     = ctx.AggregateExpressionsBoard[j]
				metadatas = board.ListVariableMetadata()
			)

			if ctx.DomainSize >= GC_DOMAIN_SIZE {
				// Force the GC to run
				tGc := time.Now()
				runtime.GC()
				totalTimeGc += time.Since(tGc).Milliseconds()
			}

			timeFFT += profiling.TimeIt(func() {

				ppool.ExecutePoolChunky(len(roots), func(k int) {
					localPool := mempool.WrapsWithMemCache(largePool)
					defer localPool.TearDown()

					root := roots[k]
					name := root.GetColID()
					if _, found := computedReeval.Load(name); found {
						// it was already computed in a previous iteration of `j`
						return
					}

					// else it's the first value of j that sees it. so we compute the
					// coset reevaluation.

					v, ok := coeffs.Load(name)
					if !ok {
						utils.Panic("handle %v not found in the coeffs (a)\n", name)
					}

					// lock the vector to ensure we don't do twice the same compute
					// and that we don't (over)write an entry in the map with a (leaking) pool vector
					v.(threadSafeVector).Lock()
					defer v.(threadSafeVector).Unlock()

					// check again if it was already computed
					// (can happen if 2 go routines hit the lock at the same time)
					if _, ok := computedReeval.Load(name); ok {
						return
					}

					reevaledRoot := sv.FFT(v.(threadSafeVector).SmartVector, fft.DIT, false, ratio, share, localPool, fft.WithNbTasks(2))
					computedReeval.Store(name, reevaledRoot)
				})

				ppool.ExecutePoolChunky(len(handles), func(k int) {
					localPool := mempool.WrapsWithMemCache(largePool)
					defer localPool.TearDown()

					pol := handles[k]
					// short-path, the column is a purely Shifted(Natural) or a Natural
					// (this excludes repeats and/or interleaved columns)
					root := column.RootParents(pol)
					rootName := root.GetColID()

					reevaledRoot, found := computedReeval.Load(rootName)
					if !found {
						// it is expected to computed in the above loop
						utils.Panic("did not find the reevaluation of %v", rootName)
					}

					// Now, we can reuse a soft-rotation of the smart-vector to save memory
					if !pol.IsComposite() {
						// in this case, the right vector was the root so we are done
						return
					}

					if shifted, isShifted := pol.(column.Shifted); isShifted {
						polName := pol.GetColID()
						res := sv.SoftRotate(reevaledRoot.(sv.SmartVector), shifted.Offset)
						computedReeval.Store(polName, res)
						return
					}

					polName := pol.GetColID()
					if _, ok := computedReeval.Load(polName); ok {
						// already computed
						return
					}

					v, ok := coeffs.Load(polName)
					if !ok {
						utils.Panic("handle %v not found in the coeffs\n", polName)
					}

					// lock the vector to ensure we don't do twice the same compute
					// and that we don't (over)write an entry in the map with a (leaking) pool vector
					v.(threadSafeVector).Lock()
					defer v.(threadSafeVector).Unlock()

					// check again if it was already computed
					// (can happen if 2 go routines hit the lock at the same time)
					if _, ok := computedReeval.Load(polName); ok {
						return
					}

					res := sv.FFT(v.(threadSafeVector).SmartVector, fft.DIT, false, ratio, share, localPool, fft.WithNbTasks(2))
					computedReeval.Store(polName, res)
				})
			})

			if len(handles) >= GC_HANDLES_SIZE {
				// Force the GC to run
				tGc := time.Now()
				runtime.GC()
				totalTimeGc += time.Since(tGc).Milliseconds()
			}

			timeExecRatio[ratio] += profiling.TimeIt(func() {

				// Evaluates the constraint expression on the coset
				evalInputs := make([]sv.SmartVector, len(metadatas))

				for k, metadataInterface := range metadatas {
					switch metadata := metadataInterface.(type) {
					case ifaces.Column:
						value, ok := computedReeval.Load(metadata.GetColID())
						if !ok {
							utils.Panic("did not find the reevaluation of %v", metadata.GetColID())
						}
						evalInputs[k] = value.(sv.SmartVector)
					case coin.Info:
						evalInputs[k] = sv.NewConstant(run.GetRandomCoinField(metadata.Name), ctx.DomainSize)
					case variables.X:
						evalInputs[k] = metadata.EvalCoset(ctx.DomainSize, i, maxRatio, true)
					case variables.PeriodicSample:
						evalInputs[k] = metadata.EvalCoset(ctx.DomainSize, i, maxRatio, true)
					case ifaces.Accessor:
						evalInputs[k] = sv.NewConstant(metadata.GetVal(run), ctx.DomainSize)
					default:
						utils.Panic("Not a variable type %v", reflect.TypeOf(metadataInterface))
					}
				}

				if len(handles) >= GC_HANDLES_SIZE {
					// Force the GC to run
					tGc := time.Now()
					runtime.GC()
					totalTimeGc += time.Since(tGc).Milliseconds()
				}

				// Note that this will panic if the expression contains "no commitment"
				// This should be caught already by the constructor of the constraint.
				quotientShare := ctx.AggregateExpressionsBoard[j].Evaluate(evalInputs, pool)
				quotientShare = sv.ScalarMul(quotientShare, annulatorInvVals[i])
				run.AssignColumn(ctx.QuotientShares[j][share].GetColID(), quotientShare)
			})

		}

		// Forcefully clean the memory for the computed reevals
		computedReeval.Range(func(k, v interface{}) bool {

			if pooled, ok := v.(*sv.Pooled); ok {
				pooled.Free(largePool)
			}

			computedReeval.Delete(k)
			return true
		})
	}

	logrus.Infof("[global-constraint] msg=\"computed the quotient\" timeIFFT=%v timeOmega=%v timeFFT=%v timeExecExpression=%v totalTimeGC=%v", timeIFFT, timeOmega, timeFFT, timeExecRatio, totalTimeGc)

}
