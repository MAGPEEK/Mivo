import { describe, expect, it } from "vitest";

import { RecoveryBudget, recoveryProgressSeconds } from "./recoveryBudget";

describe("RecoveryBudget", () => {
  it("allows one recovery per position until playback moves on or it is reset", () => {
    const budget = new RecoveryBudget();

    expect(budget.tryRecover(50)).toBe(true);
    expect(budget.tryRecover(50 + recoveryProgressSeconds - 0.1)).toBe(false);
    expect(budget.tryRecover(50 + recoveryProgressSeconds)).toBe(true);
    budget.reset();
    expect(budget.tryRecover(0)).toBe(true);
  });
});
