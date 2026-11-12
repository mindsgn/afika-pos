// types/send.ts

export type SendMethod = "bank" | "ethereum" | "bitcoin" | "phone" | "email";

export type SendState =
  | "method"
  | "token"
  | "amount"
  | "recipient"
  | "review"
  | "auth"
  | "sending"
  | "sent"
  | "error";

export interface RecipientBank {
  name: string;
  bank: string;
  accountNumber: string;
  branchCode: string;
  accountType: string;
}

export interface SendContext {
  method: SendMethod | null;
  amountLocal: string;
  amountUsd: string;
  destination: string;
  bankDetails?: RecipientBank;
}
