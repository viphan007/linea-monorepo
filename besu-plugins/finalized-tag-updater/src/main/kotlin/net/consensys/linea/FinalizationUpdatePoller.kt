package net.consensys.zkevm.ethereum.finalization

import io.vertx.core.Vertx
import linea.consensus.HardForkIdProvider
import linea.contract.l1.Web3JLineaRollupSmartContractClientReadOnly
import linea.domain.BlockParameter
import net.consensys.linea.async.AsyncRetryer
import net.consensys.linea.async.toSafeFuture
import net.consensys.zkevm.PeriodicPollingService
import org.apache.logging.log4j.LogManager
import org.apache.logging.log4j.Logger
import org.hyperledger.besu.datatypes.HardforkId
import org.hyperledger.besu.datatypes.HardforkId.MainnetHardforkId.PARIS
import tech.pegasys.teku.infrastructure.async.SafeFuture
import java.util.concurrent.CompletableFuture
import java.util.concurrent.atomic.AtomicReference
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

data class FinalizationUpdatePollerConfig(
  val pollingInterval: Duration = 12.seconds,
  val blockTag: BlockParameter,
) {
  init {
    require(pollingInterval >= 0.seconds) {
      "pollingInterval must be greater than 0"
    }
  }
}

class FinalizationUpdatePoller(
  private val vertx: Vertx,
  private val config: FinalizationUpdatePollerConfig,
  private val lineaRollup: Web3JLineaRollupSmartContractClientReadOnly,
  private val hardForkIdProvider: HardForkIdProvider,
  private val finalizationHandler: (ULong) -> CompletableFuture<*>,
  private val log: Logger = LogManager.getLogger(FinalizationUpdatePoller::class.java),
) : PeriodicPollingService(
  vertx = vertx,
  pollingIntervalMs = config.pollingInterval.inWholeMilliseconds,
  log = log,
) {
  private val lastFinalizationRef: AtomicReference<ULong> = AtomicReference(null)

  override fun action(): SafeFuture<*> {
    return AsyncRetryer.retry(
      vertx,
      backoffDelay = config.pollingInterval,
    ) {
      val hardForkId = hardForkIdProvider.getHardForkId()
      log.debug("Current HardForkId=$hardForkId")

      if (hardForkId is HardforkId.MainnetHardforkId && hardForkId.ordinal >= PARIS.ordinal) {
        log.info(
          "Detected network had been switched to Paris or later forks, " +
            "will stop updating safe/finalized block from the plugin",
        )
        super.stop().thenApply { hardForkId }
      } else {
        lineaRollup.finalizedL2BlockNumber(config.blockTag)
          .thenCompose { lineaFinalizedBlockNumber ->
            val prevFinalizedBlockNumber = lastFinalizationRef.get()
            lastFinalizationRef.set(lineaFinalizedBlockNumber.toULong())
            if (prevFinalizedBlockNumber != lineaFinalizedBlockNumber.toULong()) {
              finalizationHandler(lineaFinalizedBlockNumber.toULong()).thenApply { hardForkId }
            } else {
              CompletableFuture.completedFuture(hardForkId)
            }
          }
          .toSafeFuture()
      }
    }
  }

  override fun handleError(error: Throwable) {
    if (error.cause is UnsupportedOperationException) {
      log.error(
        "\"setFinalizedBlock\" and \"setSafeBlock\" methods are not supported in the hosting Besu client, " +
          "the poller will stop now, please check the Besu client's settings",
      )
      super.stop()
    } else {
      log.warn("Error when polling/handling Linea finalized block number", error)
    }
  }

  fun finalizedBlockNumber(): ULong {
    return lastFinalizationRef.get() ?: throw IllegalStateException("No finalization update available")
  }
}
