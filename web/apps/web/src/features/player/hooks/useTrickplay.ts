import { trickplayTile, type TrickplayLayout, type TrickplayTile } from "@mivo/player-core";
import { useEffect, useMemo, useRef, useState } from "react";

import { useApiClient } from "../../../shared/api";
import { createTrickplayApi, type TrickplayApi } from "../api/trickplayApi";
import { browserWaits, loadTrickplay, type TrickplayLoaderWaits } from "../lib/trickplayLoader";
import {
  imageSheetLoader,
  resolveSheetUrl,
  SheetPreloader,
  type SheetLoader,
} from "../lib/trickplaySheets";

/** Replaceable services, for tests; the defaults use the API client, timers and `Image`. */
export interface TrickplayServices {
  readonly api?: TrickplayApi;
  readonly waits?: TrickplayLoaderWaits;
  readonly loadSheet?: SheetLoader;
}

/** A preview tile that can be drawn: its sheet has arrived in the browser. */
export interface PreviewTile {
  readonly tile: TrickplayTile;
  /** The layout the tile belongs to, for the sheet's size. */
  readonly layout: TrickplayLayout;
  /** Absolute, same-origin URL of the sheet. */
  readonly sheetUrl: string;
}

interface LoadedLayout {
  readonly fileId: string;
  readonly layout: TrickplayLayout;
}

/** The manifest of `fileId`, asked for once; `undefined` while missing or not generated yet. */
function useTrickplayLayout(
  fileId: string | undefined,
  services: TrickplayServices,
): TrickplayLayout | undefined {
  const client = useApiClient();
  const { api: injectedApi, waits = browserWaits } = services;
  const api = useMemo(() => injectedApi ?? createTrickplayApi(client), [injectedApi, client]);
  const [loaded, setLoaded] = useState<LoadedLayout | undefined>(undefined);
  useEffect(() => {
    if (fileId === undefined) {
      return;
    }
    const controller = new AbortController();
    void loadTrickplay({
      fileId,
      api,
      waits,
      signal: controller.signal,
      onLayout: (layout) => {
        setLoaded({ fileId, layout });
      },
    });
    return () => {
      controller.abort();
    };
  }, [fileId, api, waits]);
  return loaded?.fileId === fileId ? loaded?.layout : undefined;
}

/** Loads the sheet of the wanted tile lazily, and reports which sheets are in the browser. */
function useLoadedSheets(wanted: string | undefined, loadSheet: SheetLoader): ReadonlySet<string> {
  const [loaded, setLoaded] = useState<ReadonlySet<string>>(new Set());
  const preloader = useRef<SheetPreloader | undefined>(undefined);
  useEffect(() => {
    const created = new SheetPreloader(loadSheet, (url) => {
      setLoaded((current) => new Set(current).add(url));
    });
    preloader.current = created;
    return () => {
      created.dispose();
      preloader.current = undefined;
    };
  }, [loadSheet]);
  useEffect(() => {
    if (wanted !== undefined) {
      preloader.current?.request(wanted);
    }
  }, [wanted, loadSheet]);
  return loaded;
}

/**
 * The preview tile for file position `seconds` (M4-16d). The manifest of `fileId` is asked for
 * once when the file plays (and generated on a 404); sheets are only loaded once `seconds` is
 * defined, i.e. when the viewer hovers or scrubs, one at a time lazily. Returns `undefined`
 * while there is nothing to draw: no trickplay, not generated yet, a sheet that has not
 * arrived or one that failed.
 */
export function useTrickplayTile(
  fileId: string | undefined,
  seconds: number | undefined,
  services: TrickplayServices = {},
): PreviewTile | undefined {
  const layout = useTrickplayLayout(fileId, services);
  const tile =
    layout === undefined || seconds === undefined ? undefined : trickplayTile(layout, seconds);
  const sheetUrl =
    tile === undefined ? undefined : resolveSheetUrl(tile.sheetUrl, window.location.href);
  const loaded = useLoadedSheets(sheetUrl, services.loadSheet ?? imageSheetLoader);
  if (layout === undefined || tile === undefined || sheetUrl === undefined) {
    return undefined;
  }
  return loaded.has(sheetUrl) ? { tile, layout, sheetUrl } : undefined;
}
