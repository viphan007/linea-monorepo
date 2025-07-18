import { useMemo } from "react";
import styles from "./gas-fees-list-item.module.scss";
import { CurrencyOption } from "@/stores";
import { useFormattedDigit } from "@/hooks/useFormattedDigit";

type Props = {
  name: string;
  fee: bigint;
  fiatValue: number | null;
  currency: CurrencyOption;
};

export default function GasFeesListItem({ name, fee, fiatValue, currency }: Props) {
  const formattedFees = useFormattedDigit(fee, 18);

  const feeText = useMemo(() => {
    if (fee === 0n) return <>Free</>;
    return <>{formattedFees} ETH</>;
  }, [fee, formattedFees]);

  return (
    <li className={styles["list-item"]}>
      <span>{name}</span>
      <div className={styles["fee-row"]}>
        <span className={styles["fee-value"]}>{feeText}</span>
        {fiatValue && (
          <span className={styles["fee-fiat-value"]}>
            {fiatValue.toLocaleString("en-US", {
              style: "currency",
              currency: currency.label,
              maximumFractionDigits: 2,
            })}
          </span>
        )}
      </div>
    </li>
  );
}
