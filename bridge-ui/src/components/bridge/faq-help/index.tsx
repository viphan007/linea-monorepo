import Link from "next/link";
import styles from "./faq-help.module.scss";
import clsx from "clsx";

export default function FaqHelp() {
  return (
    <div className={clsx(styles["faq-help"])}>
      Need help?{" "}
      <Link data-testid="faq-page-link" href="/faq">
        Check our FAQ
      </Link>
    </div>
  );
}
