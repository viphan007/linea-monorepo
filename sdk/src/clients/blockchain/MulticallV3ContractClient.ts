import { Block, JsonRpcProvider, Signer, TransactionReceipt, TransactionRequest, TransactionResponse } from "ethers";
import { IChainQuerier } from "../../core/clients/blockchain/IChainQuerier";
import { MulticallV3, MulticallV3__factory } from "./typechain";
import { SDKMode } from "../../sdk/config";
import { MULTICALL_ADDRESS } from "../../core/constants";
import { BaseError } from "../../core/errors/Base";
import { Multicall3 } from "./typechain/MulticallV3";
import { IMulticallV3ContractClient } from "../../core/clients/blockchain/IMulticallV3ContractClient";

export class MulticallV3ContractClient implements IMulticallV3ContractClient {
  private readonly contract: MulticallV3;

  /**
   * Creates an instance of MulticallContractClient.
   *
   * @param {IChainQuerier} chainQuerier - The chain querier for interacting with the blockchain.
   */
  constructor(
    private readonly chainQuerier: IChainQuerier<
      TransactionReceipt,
      Block,
      TransactionRequest,
      TransactionResponse,
      JsonRpcProvider
    >,
    private readonly mode: SDKMode,
    private readonly signer?: Signer,
  ) {
    this.contract = this.getContract(MULTICALL_ADDRESS, this.signer);
  }

  /**
   * Retrieves the `MulticallV3` contract instance.
   *
   * @param {string} contractAddress - Address of the MulticallV3 contract.
   * @param {Signer} [signer] - L2 ethers signer instance.
   * @returns {MulticallV3} The `MulticallV3` contract instance.
   * @private
   */
  private getContract(contractAddress: string, signer?: Signer): MulticallV3 {
    if (this.mode === "read-only") {
      return MulticallV3__factory.connect(contractAddress, this.chainQuerier.getProvider());
    }

    if (!signer) {
      throw new BaseError("Please provide a signer.");
    }

    return MulticallV3__factory.connect(contractAddress, signer);
  }

  /**
   * Calls multiple contract methods in a single transaction.
   *
   * @param {Array<{ target: string; calldata: string }>} calls - The list of calls to be made.
   * @returns {Promise<Multicall3.ResultStructOutput[]>} The results of the calls.
   */
  public async multicall(calls: { target: string; calldata: string }[]): Promise<Multicall3.ResultStructOutput[]> {
    const callsList = calls.map((call) => ({
      target: call.target,
      allowFailure: true,
      callData: call.calldata,
    }));
    const result = await this.contract.aggregate3.staticCall(callsList);
    return result;
  }
}
