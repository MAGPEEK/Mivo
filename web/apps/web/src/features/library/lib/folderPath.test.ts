import { describe, expect, it } from "vitest";

import { folderBasename, folderCrumbs, parseFolderPath } from "./folderPath";

const libraryFolderId = "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d";

describe("parseFolderPath", () => {
  it.each([
    { splat: undefined, path: "", segments: [] },
    { splat: "", path: "", segments: [] },
    { splat: libraryFolderId, path: libraryFolderId, segments: [libraryFolderId] },
    {
      splat: `${libraryFolderId}/Animation/Short Films`,
      path: `${libraryFolderId}/Animation/Short Films`,
      segments: [libraryFolderId, "Animation", "Short Films"],
    },
    { splat: "id/Folder/", path: "id/Folder", segments: ["id", "Folder"] },
    { splat: "id/..foo", path: "id/..foo", segments: ["id", "..foo"] },
    { splat: "id/a:b", path: "id/a:b", segments: ["id", "a:b"] },
    { splat: "id/100%", path: "id/100%", segments: ["id", "100%"] },
  ])("accepts $splat", ({ splat, path, segments }) => {
    expect(parseFolderPath(splat)).toEqual({ valid: true, path, segments });
  });

  it.each([
    ["a parent segment", "id/../other"],
    ["a leading parent segment", "../id"],
    ["a trailing parent segment", "id/.."],
    ["a current-folder segment", "id/./x"],
    ["an absolute path", "/etc/passwd"],
    ["an absolute path with the root id", `/${libraryFolderId}/x`],
    ["a Windows drive", "C:/Windows"],
    ["a lower-case Windows drive", "c:/Windows"],
    ["an empty segment", "id//x"],
    ["a backslash", "id\\x"],
    ["a backslash parent", "id/..\\x"],
    ["a control character", "id/a\u0000b"],
    ["a line break", "id/a\nb"],
    ["more than 4096 characters", `id/${"x".repeat(4096)}`],
  ])("rejects %s", (_, splat) => {
    expect(parseFolderPath(splat)).toEqual({ valid: false });
  });
});

describe("folderBasename", () => {
  it.each([
    ["/media/movies", "movies"],
    ["/media/movies/", "movies"],
    ["C:\\Media\\Filme", "Filme"],
    ["\\\\nas\\share\\tv", "tv"],
    ["movies", "movies"],
  ])("takes the last segment of %s", (path, name) => {
    expect(folderBasename(path)).toBe(name);
  });
});

describe("folderCrumbs", () => {
  it("is empty at the top", () => {
    expect(folderCrumbs([], "movies")).toEqual([]);
  });

  it("names the first segment and links every ancestor", () => {
    expect(folderCrumbs(["id", "Animation", "Short"], "movies")).toEqual([
      { name: "movies", path: "id" },
      { name: "Animation", path: "id/Animation" },
      { name: "Short", path: "id/Animation/Short" },
    ]);
  });
});
