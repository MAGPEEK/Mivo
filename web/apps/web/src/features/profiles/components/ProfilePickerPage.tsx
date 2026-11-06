import type { Me } from "@mivo/api-client";
import { useId, useRef, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { SignOutButton, useHeadingFocus, useSessionSnapshot } from "../../auth";
import { useProfilePicker, type ProfilePicker } from "../hooks/useProfilePicker";
import { canAddProfile } from "../lib/profileModel";
import { PinPanel } from "./PinPanel";
import { AddProfileTile, ProfileTile } from "./ProfileTile";

/** Props of {@link ProfilePickerPage}. */
export interface ProfilePickerPageProps {
  /**
   * Path of the managed-profile editor. Without it the picker shows no "Add profile" tile;
   * the editor (M7-33) passes its route here.
   */
  readonly addProfilePath?: string | undefined;
}

function PickerFailure({ picker }: { readonly picker: ProfilePicker }) {
  const { t } = useTranslation("profiles");
  let text = "";
  if (picker.failure !== undefined) {
    text = picker.failure.kind === "network" ? t("picker.network") : t("picker.failed");
  }
  return (
    <p role="alert" className="mt-4 text-sm text-accent empty:hidden">
      {text}
    </p>
  );
}

interface PickerProps {
  readonly me: Me;
  readonly addProfilePath: string | undefined;
}

type TileRefs = RefObject<Map<string, HTMLButtonElement>>;

interface ProfileListProps extends PickerProps {
  readonly picker: ProfilePicker;
  readonly tiles: TileRefs;
}

function ProfileList({ me, addProfilePath, picker, tiles }: ProfileListProps) {
  const { t } = useTranslation("profiles");
  return (
    <ul aria-label={t("picker.listLabel")} className="my-6 flex flex-wrap justify-center gap-6">
      {me.profiles.map((profile) => (
        <li key={profile.id}>
          <ProfileTile
            profile={profile}
            busy={picker.pending}
            onChoose={() => {
              picker.choose(profile);
            }}
            tileRef={(element) => {
              if (element === null) {
                tiles.current.delete(profile.id);
              } else {
                tiles.current.set(profile.id, element);
              }
            }}
          />
        </li>
      ))}
      {addProfilePath !== undefined && canAddProfile(me) && (
        <li>
          <AddProfileTile
            busy={picker.pending}
            onAdd={() => {
              picker.addProfile(addProfilePath);
            }}
          />
        </li>
      )}
    </ul>
  );
}

function Picker({ me, addProfilePath }: PickerProps) {
  const { t } = useTranslation("profiles");
  const headingId = useId();
  const headingRef = useHeadingFocus();
  const picker = useProfilePicker(me);
  const tiles = useRef(new Map<string, HTMLButtonElement>());
  const cancelPin = () => {
    const tile =
      picker.pinProfile === undefined ? undefined : tiles.current.get(picker.pinProfile.id);
    picker.cancelPin();
    tile?.focus();
  };
  return (
    <section
      aria-labelledby={headingId}
      aria-busy={picker.pending}
      className="grid min-h-dvh place-items-center px-4 py-8 text-center"
    >
      <div className="w-full max-w-3xl">
        <h1
          id={headingId}
          ref={headingRef}
          tabIndex={-1}
          className="text-3xl font-semibold text-text focus:outline-none"
        >
          {t("picker.title")}
        </h1>
        <ProfileList me={me} addProfilePath={addProfilePath} picker={picker} tiles={tiles} />
        {picker.pinProfile !== undefined && (
          <PinPanel
            key={picker.pinProfile.id}
            profile={picker.pinProfile}
            pin={picker.pin}
            remember={picker.remember}
            onRememberChange={picker.setRemember}
            onCancel={cancelPin}
          />
        )}
        <PickerFailure picker={picker} />
        <div className="mt-8 flex justify-center">
          <SignOutButton />
        </div>
      </div>
    </section>
  );
}

/**
 * "Who's watching?" (US-01): the account's profiles as tiles; a PIN profile opens the PIN pad,
 * any other is selected at once. Afterwards the guard returns to `next` or Home.
 */
export function ProfilePickerPage({ addProfilePath }: ProfilePickerPageProps) {
  const { me } = useSessionSnapshot();
  return me === undefined ? null : <Picker me={me} addProfilePath={addProfilePath} />;
}
