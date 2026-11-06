import { productId } from "@mivo/ui-tokens";

// The password recovery of ADR 0032 §6; the binary and the Docker container are named after
// the product id.

/** The recovery command for the account `name`, run on the server machine. */
export function recoveryCommand(name: string): string {
  return `${productId} admin reset-password --name ${name}`;
}

/** The same command through Docker, with an ellipsis for the name. */
export function dockerRecoveryCommand(): string {
  return `docker exec -it ${productId} ${recoveryCommand("…")}`;
}
