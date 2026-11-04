import type { InstallKind } from "@mivo/api-client";
import { Trans, useTranslation } from "react-i18next";

import { settingsNamespace } from "../locales";

/** Props of {@link PermissionHelp}. */
export interface PermissionHelpProps {
  readonly installKind: InstallKind;
  /** Account the server runs as; empty when unknown. */
  readonly serviceUser: string;
}

const dockerExample = "-v /volume1/video:/media/video:ro";

/** Fallback shown where the account name is unknown. */
function useUserName(serviceUser: string): string {
  const { t } = useTranslation(settingsNamespace);
  return serviceUser === "" ? t("libraries.help.unknownUser") : serviceUser;
}

function SynologySteps({ user }: { readonly user: string }) {
  return (
    <Trans
      i18nKey="libraries.help.synology"
      ns={settingsNamespace}
      values={{ user }}
      components={{ strong: <strong />, code: <code /> }}
    />
  );
}

function DockerSteps() {
  return (
    <Trans
      i18nKey="libraries.help.docker"
      ns={settingsNamespace}
      values={{ example: dockerExample }}
      components={{ code: <code /> }}
    />
  );
}

function StandardSteps({ user }: { readonly user: string }) {
  return (
    <Trans
      i18nKey="libraries.help.standard"
      ns={settingsNamespace}
      values={{ user }}
      components={{ code: <code /> }}
    />
  );
}

/**
 * How to let the server read a folder, written for the installation at hand. The server cannot
 * change permissions itself, so this is a plain explanation, not an action.
 */
export function PermissionHelp({ installKind, serviceUser }: PermissionHelpProps) {
  const { t } = useTranslation(settingsNamespace);
  const user = useUserName(serviceUser);
  return (
    <section
      aria-label={t("libraries.help.title")}
      className="mt-2 rounded-md border border-divider bg-surface p-3 text-sm text-text-muted"
    >
      <h3 className="mb-1 font-medium text-text">{t("libraries.help.title")}</h3>
      <p>
        {installKind === "synology" && <SynologySteps user={user} />}
        {installKind === "docker" && <DockerSteps />}
        {/* Later API versions may add install kinds; the generic advice still applies. */}
        {installKind !== "synology" && installKind !== "docker" && <StandardSteps user={user} />}
      </p>
    </section>
  );
}
