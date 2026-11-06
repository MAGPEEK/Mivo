import type { ReactNode } from "react";

/** Props of {@link Notice}. */
export interface NoticeProps {
  /** `success` after sign-out, `warning` for a lost session, `problem` while throttled. */
  readonly tone: "info" | "success" | "warning" | "problem";
  readonly children: ReactNode;
  /** `status` announces politely; omit it for static text. */
  readonly role?: "status";
}

const toneClasses = {
  info: "border-direct-stream",
  success: "border-direct-play",
  warning: "border-transcode",
  problem: "border-accent",
} as const;

/** A short message with a coloured edge above or inside a form. */
export function Notice({ tone, children, role }: NoticeProps) {
  return (
    <div
      role={role}
      className={`mb-4 rounded-sm border-l-4 bg-background px-3 py-2 text-sm text-text ${toneClasses[tone]}`}
    >
      {children}
    </div>
  );
}
