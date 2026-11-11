import type { Profile } from "@mivo/api-client";
import { productName } from "@mivo/ui-tokens";
import { useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  EmptyState,
  InfoIcon,
  Skeleton,
  primaryButtonClass,
  secondaryButtonClass,
} from "../../../../shared/ui";
import { usersNamespace } from "../../locales";
import { useOwnProfiles, type OwnProfilesState } from "../hooks/useOwnProfiles";
import { useProfileRemoval } from "../hooks/useProfileRemoval";
import { newProfileTarget, type AccountSearch } from "../lib/accountPaths";
import { OwnProfileList } from "./OwnProfileList";
import { ProfileDialog } from "./ProfileDialog";
import { RemoveProfileDialog } from "./RemoveProfileDialog";

/** Props of {@link OwnProfilesPage}. */
export interface OwnProfilesPageProps {
  /** The URL search: which profile editor is open. */
  readonly search: AccountSearch;
  /** Opens or closes an editor by changing the URL search. */
  readonly onSearchChange: (next: AccountSearch) => void;
}

interface ContentProps {
  readonly state: OwnProfilesState;
  readonly onEdit: (profile: Profile) => void;
  readonly onDelete: (profile: Profile) => void;
}

function ProfilesContent({ state, onEdit, onDelete }: ContentProps) {
  const { t } = useTranslation(usersNamespace);
  switch (state.status) {
    case "loading":
      return (
        <div role="status" className="w-60">
          <Skeleton label={t("profiles.page.loading")} className="h-5 w-full" />
        </div>
      );
    case "unavailable":
      return (
        <EmptyState
          icon={<InfoIcon className="size-10" />}
          title={t("profiles.page.unavailable.title")}
          description={t("profiles.page.unavailable.hint", { productName })}
        />
      );
    case "primaryRequired":
      return <p>{t("profiles.page.primaryRequired")}</p>;
    case "forbidden":
      return <p role="alert">{t("profiles.problems.forbidden")}</p>;
    case "error":
      return (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p>{t("profiles.page.loadFailed")}</p>
          <button type="button" onClick={state.retry} className={secondaryButtonClass}>
            {t("users.retry")}
          </button>
        </div>
      );
    case "ready":
      return (
        <OwnProfileList
          accountName={state.owner.name}
          profiles={state.profiles}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      );
  }
}

interface EditorProps extends OwnProfilesPageProps {
  readonly state: OwnProfilesState;
  readonly onSaved: (name: string) => void;
}

// The editor follows the URL: `profile=new`, or the ID of one of the account's profiles.
function ProfileEditor({ state, search, onSearchChange, onSaved }: EditorProps) {
  const target = search.profile;
  if (state.status !== "ready" || target === undefined) {
    return null;
  }
  const profile = state.profiles.find((candidate) => candidate.id === target);
  if (profile === undefined && target !== newProfileTarget) {
    return null;
  }
  const close = () => {
    onSearchChange({});
  };
  return (
    <ProfileDialog
      key={target}
      owner={state.owner}
      profile={profile}
      onClose={close}
      onSaved={(name) => {
        onSaved(name);
        close();
      }}
    />
  );
}

interface PageHeaderProps {
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
  /** Present once the profiles are loaded and one can be added. */
  readonly onAdd: (() => void) | undefined;
}

function PageHeader({ headingRef, onAdd }: PageHeaderProps) {
  const { t } = useTranslation(usersNamespace);
  return (
    <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 ref={headingRef} tabIndex={-1} className="text-3xl font-semibold">
          {t("profiles.page.heading")}
        </h1>
        <p className="mt-1 text-sm text-text-muted">{t("profiles.page.description")}</p>
      </div>
      {onAdd !== undefined && (
        <button type="button" onClick={onAdd} className={primaryButtonClass}>
          {t("profiles.add")}
        </button>
      )}
    </div>
  );
}

/**
 * The own account page: the account's profiles with their editor (US-10). Only the primary
 * profile manages them; a managed profile is told to switch.
 */
export function OwnProfilesPage({ search, onSearchChange }: OwnProfilesPageProps) {
  const { t } = useTranslation(usersNamespace);
  const state = useOwnProfiles();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [announcement, setAnnouncement] = useState("");
  // The deleted entry takes the focused button with it; the heading is the stable landmark left.
  const removal = useProfileRemoval(() => {
    setAnnouncement(t("profiles.announce.removed"));
    headingRef.current?.focus();
  });
  const open = (profile: string) => {
    onSearchChange({ profile });
  };
  return (
    <div className="max-w-3xl">
      <PageHeader
        headingRef={headingRef}
        onAdd={
          state.status === "ready"
            ? () => {
                open(newProfileTarget);
              }
            : undefined
        }
      />
      <p role="status" className="sr-only">
        {announcement}
      </p>
      <ProfilesContent
        state={state}
        onEdit={(profile) => {
          open(profile.id);
        }}
        onDelete={removal.ask}
      />
      <ProfileEditor
        state={state}
        search={search}
        onSearchChange={onSearchChange}
        onSaved={(name) => {
          setAnnouncement(t("profiles.announce.saved", { name }));
        }}
      />
      <RemoveProfileDialog removal={removal} />
    </div>
  );
}
