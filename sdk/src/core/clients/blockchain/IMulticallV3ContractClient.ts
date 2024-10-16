import { Multicall3 } from "../../../clients/blockchain/typechain/MulticallV3";

export interface IMulticallV3ContractClient {
  multicall(calls: { target: string; calldata: string }[]): Promise<Multicall3.ResultStructOutput[]>;
}
