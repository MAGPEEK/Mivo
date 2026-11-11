import type { AvatarColor } from "@mivo/api-client";
import { avatarColorNames } from "@mivo/ui-tokens";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { avatarBackgroundClass } from "../../../../shared/ui";
import { usersNamespace } from "../../locales";

/** Props of {@link AvatarColorField}. */
export interface AvatarColorFieldProps {
  readonly color: AvatarColor;
  readonly onChange: (color: AvatarColor) => void;
}

// The radio itself is visually hidden, so the swatch shows its checked and focused state.
const swatchClass =
  "grid size-10 cursor-pointer place-items-center rounded-full border-2 border-transparent has-checked:border-text has-focus-visible:outline-2 has-focus-visible:outline-offset-2 has-focus-visible:outline-accent";

/**
 * The avatar colour as one swatch per token colour. A colour of a newer server that this
 * client does not know stays stored until another one is chosen.
 */
export function AvatarColorField({ color, onChange }: AvatarColorFieldProps) {
  const { t } = useTranslation(usersNamespace);
  const name = useId();
  return (
    <fieldset className="mb-4">
      <legend className="mb-1 text-sm font-medium text-text">
        {t("profiles.dialog.color.legend")}
      </legend>
      <div className="flex flex-wrap gap-1">
        {avatarColorNames.map((option) => (
          <label key={option} title={t(`profiles.dialog.color.${option}`)} className={swatchClass}>
            <input
              type="radio"
              name={name}
              value={option}
              checked={color === option}
              onChange={() => {
                onChange(option);
              }}
              className="sr-only"
            />
            <span
              aria-hidden="true"
              className={`size-7 rounded-full ${avatarBackgroundClass(option)}`}
            />
            <span className="sr-only">{t(`profiles.dialog.color.${option}`)}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
