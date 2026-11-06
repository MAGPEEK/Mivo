import { productName } from "@mivo/ui-tokens";
import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { useSystemInfo } from "../../system";
import { useHeadingFocus } from "../hooks/useHeadingFocus";

/** Props of {@link AuthCard}. */
export interface AuthCardProps {
  readonly title: string;
  /** `wide` for the setup pages with more text. */
  readonly size?: "narrow" | "wide";
  /** Shown between the server line and the heading, e.g. steps or a notice. */
  readonly header?: ReactNode;
  readonly children: ReactNode;
}

const sizeClasses = { narrow: "max-w-md", wide: "max-w-xl" } as const;

function ServerLine() {
  const { t } = useTranslation("auth");
  const systemInfo = useSystemInfo();
  const line =
    systemInfo.data === undefined
      ? t("server.versionUnknown", { productName })
      : t("server.version", { productName, version: systemInfo.data.version });
  return (
    <div className="mb-5 flex items-center gap-3">
      <span
        aria-hidden="true"
        className="grid size-10 place-items-center rounded-md bg-accent text-lg font-extrabold text-on-accent"
      >
        {productName.slice(0, 1)}
      </span>
      <span className="text-xs text-text-muted">{line}</span>
    </div>
  );
}

/**
 * The centred card of the sign-in, setup and related pages: server line, optional header,
 * the page heading (focused on arrival, so screen readers start there) and the content.
 */
export function AuthCard({ title, size = "narrow", header, children }: AuthCardProps) {
  const headingId = useId();
  const headingRef = useHeadingFocus();
  return (
    <div className="grid min-h-dvh place-items-center px-4 py-8 max-sm:place-items-start">
      <section
        aria-labelledby={headingId}
        className={`w-full rounded-lg border border-divider bg-surface p-7 max-sm:border-0 max-sm:bg-transparent max-sm:p-2 ${sizeClasses[size]}`}
      >
        <ServerLine />
        {header}
        <h1
          id={headingId}
          ref={headingRef}
          tabIndex={-1}
          className="mb-1 text-2xl font-semibold text-text focus:outline-none"
        >
          {title}
        </h1>
        {children}
      </section>
    </div>
  );
}
