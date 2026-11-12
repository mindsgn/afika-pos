/** 1 point is earned per R10 spent; 1 point is worth 10 cents when redeemed (i.e. 1% back). */
export const EARN_PER_CENTS = 1000;
export const POINT_VALUE_CENTS = 10;

export const pointsEarned = (totalCents: number): number =>
  Math.max(0, Math.floor(totalCents / EARN_PER_CENTS));

export const pointsValue = (points: number): number => points * POINT_VALUE_CENTS;

export const maxRedeemablePoints = (balance: number, payableCents: number): number =>
  Math.max(0, Math.min(balance, Math.floor(payableCents / POINT_VALUE_CENTS)));

/** Points to claw back for a (partial) refund, proportional to the refunded share of the sale. */
export const pointsToReverse = (earned: number, refundAmount: number, saleTotal: number): number =>
  saleTotal <= 0 ? 0 : Math.floor((earned * refundAmount) / saleTotal);
